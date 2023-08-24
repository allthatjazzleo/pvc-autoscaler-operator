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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PodDiskInspectorSpec defines the desired state of PodDiskInspector
type PodDiskInspectorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Image is the docker reference in "repository:tag" format. E.g. busybox:latest.
	// This is for the sidecar container running the disk health check process.
	// +kubebuilder:validation:MinLength:=1
	Image string `json:"image"`

	// Your cluster must support and use the ExpandInUsePersistentVolumes feature gate. This allows volumes to
	// expand while a pod is attached to it, thus eliminating the need to restart pods.
	// If you cluster does not support ExpandInUsePersistentVolumes, you will need to manually restart pods after
	// resizing is complete.
	// +optional
	PVCScaling *PVCScalingSpec `json:"pvcScaling"`
}

// PodDiskInspectorStatus defines the observed state of PodDiskInspector
type PodDiskInspectorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PodDiskInfo contains information about a pod's disk
	// +optional
	PodDiskInfo []PodDiskInfo `json:"podDiskInfo,omitempty"`
}

// PodDiskInfo contains information about a pod's disk
type PodDiskInfo struct {
	// PodName is the name of the pod
	PodName string `json:"podName,omitempty"`

	// PVCScalingStatus contains information about a pod's PVC scaling status
	PVCScalingStatus []PVCScalingStatus `json:"pvcScalingStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PodDiskInspector is the Schema for the poddiskinspectors API
type PodDiskInspector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodDiskInspectorSpec   `json:"spec,omitempty"`
	Status PodDiskInspectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodDiskInspectorList contains a list of PodDiskInspector
type PodDiskInspectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodDiskInspector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodDiskInspector{}, &PodDiskInspectorList{})
}
