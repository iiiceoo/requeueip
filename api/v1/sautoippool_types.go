/*
Copyright 2024 The RequeueIP Authors.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SautoIPPoolSpec defines the desired state of SautoIPPool.
type SautoIPPoolSpec struct {
	// +kubebuilder:validation:Enum=IPv4;IPv6
	// +kubebuilder:validation:Optional
	Version *string `json:"version,omitempty"`

	// +kubebuilder:validation:Optional
	Ranges []string `json:"ranges,omitempty"`
}

// SautoIPPoolStatus defines the observed state of SautoIPPool.
type SautoIPPoolStatus struct {
	// +kubebuilder:validation:Optional
	Free []string `json:"free,omitempty"`
}

// +kubebuilder:resource:categories={requeueip},path="sautoippools",scope="Namespaced",shortName={sp},singular="sautoippool"
// +kubebuilder:printcolumn:JSONPath=".spec.version",description="version",name="VERSION",type=string
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SautoIPPool is the Schema for the SautoIPPools API.
type SautoIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SautoIPPoolSpec   `json:"spec,omitempty"`
	Status SautoIPPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SautoIPPoolList contains a list of SautoIPPool.
type SautoIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SautoIPPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SautoIPPool{}, &SautoIPPoolList{})
}
