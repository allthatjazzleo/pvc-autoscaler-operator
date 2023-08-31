package pvc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client is a controller client. It is a subset of client.Client.
type Client interface {
	client.Reader
	client.Writer
	client.StatusClient
}

type PVCAutoScaler struct {
	client Client
	now    func() time.Time
}

func NewPVCAutoScaler(client Client) *PVCAutoScaler {
	return &PVCAutoScaler{
		client: client,
		now:    time.Now,
	}
}

// ProcessPVCResize patches the PVC request storage size and update annotation for resize time
//
// Returns true if the status was patched.
//
// Returns false and does not patch if:
// 1. The PVCs do not need resizing
// 2. The status already has >= calculated size.
// 3. The maximum size has been reached. It will patch up to the maximum size.
//
// Returns an error if patching unsuccessful.
func (scaler PVCAutoScaler) ProcessPVCResize(ctx context.Context, crd *v1alpha1.PodDiskInspector, results []PVCDiskUsage, reporter kube.Reporter) error {
	var (
		status        = crd.Status.PVCScalingStatus
		pvcCandidates = make(map[string]v1alpha1.ScalingStatus)
		merr          error
	)

	for _, pvcCandidate := range results {
		// Prevent patching if PVC size not at threshold
		if pvcCandidate.PVCScalingSpec == nil {
			continue
		}
		if pvcCandidate.PercentUsed < int(pvcCandidate.PVCScalingSpec.UsedSpacePercentage) {
			continue
		}

		// Calc new size first to catch errors with the increase quantity
		newSize, err := scaler.calcNextCapacity(pvcCandidate.Capacity, pvcCandidate.PVCScalingSpec.IncreaseQuantity)
		if err != nil {
			merr = errors.Join(merr, fmt.Errorf("increaseQuantity must be a percentage string (e.g. 10%%) or a storage quantity (e.g. 100Gi): %w", err))
		}

		// Handle max size
		if max := pvcCandidate.PVCScalingSpec.MaxSize; !max.IsZero() {
			// If already reached max size, don't patch
			if pvcCandidate.Capacity.Cmp(max) >= 0 {
				continue
			}
			// Cap new size to the max size
			if newSize.Cmp(max) >= 0 {
				newSize = max
			}
		}

		// Prevent continuous reconcile loops
		key := client.ObjectKey{Namespace: pvcCandidate.Namespace, Name: pvcCandidate.Name}
		if _, found := pvcCandidates[key.String()]; found {
			continue
		}
		if scalingStatus, found := status[key.String()]; found {
			// If already patched, don't patch again
			if scalingStatus.RequestedSize.Value() == newSize.Value() {
				reporter.Debug("PVC already patched before", "pvc", pvcCandidate.Name, "namespace", pvcCandidate.Namespace, "newSize", newSize.String())
				continue
			}

			// If cooldown period has not passed, don't patch
			if pvcCandidate.PVCScalingSpec.Cooldown.Duration != 0 {
				cooldown := pvcCandidate.PVCScalingSpec.Cooldown.Duration
				if !scalingStatus.RequestedAt.IsZero() && scaler.now().Before(scalingStatus.RequestedAt.Add(cooldown)) {
					reporter.Debug("PVC cooldown period has not passed", "pvc", pvcCandidate.Name, "namespace", pvcCandidate.Namespace, "requestedAt", scalingStatus.RequestedAt.String(), "cooldown", cooldown.String())
					continue
				}
			}
		}

		reporter.Info("Patching pvc", "pvc", pvcCandidate.Name, "namespace", pvcCandidate.Namespace, "newSize", newSize.String())

		currentRequests := pvcCandidate.pvc.Spec.Resources.Requests
		currentRequests[corev1.ResourceStorage] = newSize
		patch := corev1.PersistentVolumeClaim{
			ObjectMeta: pvcCandidate.pvc.ObjectMeta,
			TypeMeta:   pvcCandidate.pvc.TypeMeta,
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.ResourceRequirements{
					Requests: currentRequests,
				},
			},
		}
		if err := scaler.client.Patch(ctx, &patch, client.Merge); err != nil {
			reporter.Error(err, "PVC patch failed", "pvc", pvcCandidate.Name, "namespace", pvcCandidate.Namespace, "newSize", newSize.String())
			reporter.RecordError("PVCAutoScaleResize", err)
			merr = errors.Join(merr, err)
			continue
		}
		reporter.Info("PVC patch succeeded", "pvc", pvcCandidate.Name, "namespace", pvcCandidate.Namespace, "newSize", newSize.String())

		pvcCandidates[key.String()] = v1alpha1.ScalingStatus{
			RequestedSize: newSize,
			RequestedAt:   metav1.NewTime(scaler.now()),
		}
	}

	// Update crd status
	if len(pvcCandidates) > 0 {
		if err := scaler.client.Get(ctx, client.ObjectKeyFromObject(crd), crd); err != nil {
			merr = errors.Join(merr, err)
			return merr
		}
		if crd.Status.PVCScalingStatus == nil {
			crd.Status.PVCScalingStatus = make(map[string]v1alpha1.ScalingStatus)
		}
		for key, scalingStatus := range pvcCandidates {
			crd.Status.PVCScalingStatus[key] = scalingStatus
		}

		if err := scaler.client.Status().Update(ctx, crd); err != nil {
			merr = errors.Join(merr, err)
			return merr
		}
	}

	return merr
}

func (scaler PVCAutoScaler) calcNextCapacity(current resource.Quantity, increase string) (resource.Quantity, error) {
	var (
		merr     error
		quantity resource.Quantity
	)

	// Try to calc by percentage first
	v := intstr.FromString(increase)
	percent, err := intstr.GetScaledValueFromIntOrPercent(&v, 100, false)
	if err == nil {
		addtl := math.Round(float64(current.Value()) * (float64(percent) / 100.0))
		quantity = *resource.NewQuantity(current.Value()+int64(addtl), current.Format)
		return quantity, nil
	}

	merr = errors.Join(merr, err)

	// Then try to calc by resource quantity
	addtl, err := resource.ParseQuantity(increase)
	if err != nil {
		return quantity, errors.Join(merr, err)
	}
	current.Add(addtl)

	return current, nil
}
