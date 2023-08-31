package pvc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/healthcheck"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/kube"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const UsedSpacePercentage = "pvc-autoscaler-operator.kubernetes.io/used-space-percentage"
const IncreaseQuantity = "pvc-autoscaler-operator.kubernetes.io/increase-quantity"
const Cooldown = "pvc-autoscaler-operator.kubernetes.io/cooldown"
const MaxSize = "pvc-autoscaler-operator.kubernetes.io/max-size"

var ErrNoPodsFound = errors.New("no pods found")

// DiskUsager fetches disk usage statistics
type DiskUsager interface {
	DiskUsage(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error)
}

type PVCDiskUsage struct {
	Name           string // pvc name
	Namespace      string // pvc namespace
	PercentUsed    int
	Capacity       resource.Quantity
	PVCScalingSpec *v1alpha1.PVCScalingSpec
	pvc            *corev1.PersistentVolumeClaim
}

type DiskUsageCollector struct {
	diskClient DiskUsager
	client     client.Reader
}

func NewDiskUsageCollector(diskClient DiskUsager, lister client.Reader) *DiskUsageCollector {
	return &DiskUsageCollector{diskClient: diskClient, client: lister}
}

// CollectDiskUsage retrieves the disk usage information for all pods has
// "pvc-autoscaler-operator.kubernetes.io/enabled" annotation set to "true",
// "pvc-autoscaler-operator.kubernetes.io/operator-name" annotation set to the name of the operator and
// "pvc-autoscaler-operator.kubernetes.io/operator-namespace" annotation set to the namespace of the operator.=
// It returns a slice of PVCDiskUsage objects representing the disk usage information for each PVC or an error
// if fetching disk usage via all pods was unsuccessful.
func (c DiskUsageCollector) CollectDiskUsage(ctx context.Context, crd *v1alpha1.PodDiskInspector) ([]PVCDiskUsage, error) {
	var pods corev1.PodList
	fieldValue := client.ObjectKey{Name: crd.Name, Namespace: crd.Namespace}
	if err := c.client.List(ctx, &pods,
		client.MatchingFields{kube.ControllerField: fieldValue.String()},
	); err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, ErrNoPodsFound
	}

	var (
		found = make([][]PVCDiskUsage, len(pods.Items))
		errs  = make([]error, len(pods.Items))
		eg    errgroup.Group
	)

	for i := range pods.Items {
		i := i
		eg.Go(func() error {
			pod := pods.Items[i]
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			resp, err := c.diskClient.DiskUsage(cctx, "http://"+pod.Status.PodIP)
			if err != nil {
				errs[i] = fmt.Errorf("pod %s: %w", pod.Name, err)
				return nil
			}

			nestedErr := make([]error, len(resp))
			for _, diskUsageResponse := range resp {
				name := diskUsageResponse.PvcName
				namespace := pod.Namespace
				defaultSpec := crd.Spec.PVCScaling.DeepCopy()

				// Find matching PVC to capture its actual capacity
				key := client.ObjectKey{Namespace: namespace, Name: name}
				var pvc corev1.PersistentVolumeClaim
				if err = c.client.Get(ctx, key, &pvc); err != nil {
					nestedErr = append(nestedErr, fmt.Errorf("get pvc %s: %w", key, err))
					continue
				}

				// override default spec with pod annoations if present
				OverideSpec(defaultSpec, pod.GetAnnotations())

				// override default spec with pvc annoations if present
				OverideSpec(defaultSpec, pvc.GetAnnotations())

				item := PVCDiskUsage{
					Name:           name,
					Namespace:      namespace,
					PercentUsed:    int(math.Round((float64(diskUsageResponse.AllBytes-diskUsageResponse.FreeBytes) / float64(diskUsageResponse.AllBytes)) * 100)),
					Capacity:       pvc.Status.Capacity[corev1.ResourceStorage],
					PVCScalingSpec: defaultSpec,
					pvc:            &pvc,
				}
				found[i] = append(found[i], item)
			}
			if len(nestedErr) > 0 {
				errs[i] = errors.Join(nestedErr...)
			}

			return nil
		})
	}

	_ = eg.Wait()

	errs = lo.Filter(errs, func(item error, _ int) bool {
		return item != nil
	})
	if len(errs) == len(pods.Items) {
		return nil, errors.Join(errs...)
	}

	return lo.Flatten(found), nil
}

func OverideSpec(defaultSpec *v1alpha1.PVCScalingSpec, annotations map[string]string) *v1alpha1.PVCScalingSpec {
	if annotations[UsedSpacePercentage] != "" {
		if num, err := strconv.ParseInt(annotations[UsedSpacePercentage], 10, 32); err == nil {
			defaultSpec.UsedSpacePercentage = int32(num)
		}
	}
	if annotations[IncreaseQuantity] != "" {
		defaultSpec.IncreaseQuantity = annotations[IncreaseQuantity]
	}
	if annotations[Cooldown] != "" {
		if pd, err := time.ParseDuration(annotations[Cooldown]); err == nil {
			defaultSpec.Cooldown = v1.Duration{Duration: pd}
		}
	}
	if annotations[MaxSize] != "" {
		defaultSpec.MaxSize = resource.MustParse(annotations[MaxSize])
	}
	return defaultSpec
}
