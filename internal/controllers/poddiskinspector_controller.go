/*
Copyright 2023.

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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalerv1alpha1 "github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/kube"
)

// PodDiskInspectorReconciler reconciles a PodDiskInspector object
type PodDiskInspectorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

var (
	stopResult    ctrl.Result
	requeueResult = ctrl.Result{RequeueAfter: 3 * time.Second}
)

//+kubebuilder:rbac:groups=autoscaler.allthatjazzleo,resources=poddiskinspectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaler.allthatjazzleo,resources=poddiskinspectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=autoscaler.allthatjazzleo,resources=poddiskinspectors/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PodDiskInspector object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PodDiskInspectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the PodDiskInspector instance
	crd := &autoscalerv1alpha1.PodDiskInspector{}
	err := r.Get(ctx, req.NamespacedName, crd)
	if err != nil {
		if kube.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("crd resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch PodDiskInspector")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodDiskInspectorReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalerv1alpha1.PodDiskInspector{}).
		Complete(r)
}
