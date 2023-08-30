/*
Copyright 2023 allthatjazzleo

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/healthcheck"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/kube"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/pvc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// PVCScalingReconciler reconciles a PodDiskInspector object status and scales PVCs.
type PVCScalingReconciler struct {
	client.Client
	diskClient    *pvc.DiskUsageCollector
	pvcAutoScaler *pvc.PVCAutoScaler
	recorder      record.EventRecorder
}

func NewPVCScaling(
	client client.Client,
	recorder record.EventRecorder,
	httpClient *http.Client,
) *PVCScalingReconciler {
	return &PVCScalingReconciler{
		Client:        client,
		diskClient:    pvc.NewDiskUsageCollector(healthcheck.NewClient(httpClient), client),
		pvcAutoScaler: pvc.NewPVCAutoScaler(client),
		recorder:      recorder,
	}
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;update;patch

// Reconcile reconciles only the pvcScaling spec in PodDiskInspector.
func (r *PVCScalingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get the CRD
	crd := new(v1alpha1.PodDiskInspector)
	reporter := kube.NewEventReporter(log.FromContext(ctx).WithName(v1alpha1.PVCScalingController), r.recorder, nil)

	reporter.Info("Entering reconcile loop", "request", req.NamespacedName)
	if err := r.Client.Get(ctx, req.NamespacedName, crd); err != nil {
		// Ignore not found errors because can't be fixed by an immediate requeue. We'll have to wait for next notification.
		// Also, will get "not found" error if crd is deleted.
		// No need to explicitly delete resources. Kube GC does so automatically because we set the controller reference
		// for each resource.
		return stopResult, client.IgnoreNotFound(err)
	}
	reporter = reporter.UpdateResource(crd)

	r.pvcAutoScale(ctx, reporter, crd)

	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *PVCScalingReconciler) pvcAutoScale(ctx context.Context, reporter kube.Reporter, crd *v1alpha1.PodDiskInspector) {
	if crd.Spec.PVCScaling == nil {
		reporter.Error(errors.New("no default PVCScalingSpec found in PodDiskInspectorSpec"), "Failed to process pvc resize")
		reporter.RecordError("PVCAutoScaleCollectUsage", errors.New("no default PVCScalingSpec found in PodDiskInspectorSpec"))
		return
	}
	usage, err := r.diskClient.CollectDiskUsage(ctx, crd)
	if err != nil {
		reporter.Error(err, "Failed to collect pvc disk usage")
		// This error can be noisy so we record a generic error. Check logs for error details.
		reporter.RecordError("PVCAutoScaleCollectUsage", errors.New("failed to collect pvc disk usage"))
		return
	}
	err = r.pvcAutoScaler.ProcessPVCResize(ctx, crd, usage, reporter)
	if err != nil {
		reporter.Error(err, "Failed to process pvc resize")
		reporter.RecordError("PVCAutoScaleResize", err)
	}
}

func (r *PVCScalingReconciler) findObjectForPod(_ context.Context, pod client.Object) []reconcile.Request {
	enabled := strings.ToLower(strings.TrimSpace(pod.GetAnnotations()[kube.OperatorEnabled]))
	name := pod.GetAnnotations()[kube.OperatorName]
	namespace := pod.GetAnnotations()[kube.OperatorNamespace]

	if enabled == "true" && name != "" && namespace != "" {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      name,
					Namespace: namespace,
				},
			},
		}
	} else {
		return []reconcile.Request{}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCScalingReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	// Index pods.
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&corev1.Pod{},
		kube.ControllerField,
		func(rawObj client.Object) []string {
			// Extract the PodDiskInspector from the pod annotation, if one is provided
			pod := rawObj.(*corev1.Pod)
			name := pod.GetAnnotations()[kube.OperatorName]
			namespace := pod.GetAnnotations()[kube.OperatorNamespace]
			if name == "" || namespace == "" {
				return nil
			}
			value := types.NamespacedName{Name: name, Namespace: namespace}
			return []string{value.String()}
		}); err != nil {
		return fmt.Errorf("pod index field %s: %w", kube.ControllerField, err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PodDiskInspector{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectForPod),
			builder.WithPredicates(&predicate.Funcs{
				CreateFunc: func(_ event.CreateEvent) bool { return true },
				UpdateFunc: func(_ event.UpdateEvent) bool { return true },
			}),
		).
		Complete(r)
}
