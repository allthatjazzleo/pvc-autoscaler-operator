package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCScalingController is the canonical controller name.
const PVCScalingController = "PVCScaling"

// PVCScalingSpec is part of a CosmosFullNode but is managed by a separate controller, PVCScalingReconciler.
// This is an effort to reduce complexity in the CosmosFullNodeReconciler.
// The controller only modifies the CosmosFullNode's status subresource relying on the CosmosFullNodeReconciler
// to reconcile appropriately.
type PVCScalingSpec struct {
	// The percentage of used disk space required to trigger scaling.
	// Example, if set to 80, autoscaling will not trigger until used space reaches >=80% of capacity.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:MaxSize=100
	UsedSpacePercentage int32 `json:"usedSpacePercentage"`

	// How much to increase the PVC's capacity.
	// Either a percentage (e.g. 20%) or a resource storage quantity (e.g. 100Gi).
	//
	// If a percentage, the existing capacity increases by the percentage.
	// E.g. PVC of 100Gi capacity + IncreaseQuantity of 20% increases disk to 120Gi.
	//
	// If a storage quantity (e.g. 100Gi), increases by that amount.
	IncreaseQuantity string `json:"increaseQuantity"`

	// How long to wait before scaling again.
	// For AWS EBS, this is 6 hours.
	// +optional
	Cooldown metav1.Duration `json:"cooldown"`

	// A resource storage quantity (e.g. 2000Gi).
	// When increasing PVC capacity reaches >= MaxSize, autoscaling ceases.
	// Safeguards against storage quotas and costs.
	// +optional
	MaxSize resource.Quantity `json:"maxSize"`
}

type PVCScalingStatus struct {
	// The PVC name
	PVCName string `json:"pvcName"`
	// The PVC size requested by the PVCScaling controller.
	RequestedSize resource.Quantity `json:"requestedSize"`
	// The timestamp the PVCScaling controller requested a PVC increase.
	RequestedAt metav1.Time `json:"requestedAt"`
}
