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

// IPPoolSpec defines the desired state of IPPool.
type IPPoolSpec struct {
	// +kubebuilder:validation:Enum=IPv4;IPv6
	// +kubebuilder:validation:Required
	Version string `json:"version"`

	// +kubebuilder:validation:Required
	Subnet string `json:"subnet"`

	// +kubebuilder:validation:Required
	Ranges []string `json:"ranges"`
}

// IPPoolStatus defines the observed state of IPPool.
type IPPoolStatus struct {
	// +kubebuilder:validation:Required
	Free []string `json:"free"`

	// +kubebuilder:validation:Optional
	Count *Count `json:"count,omitempty"`
}

// +kubebuilder:resource:categories={requeueip},path="ippools",scope="Namespaced",shortName={rp},singular="ippool"
// +kubebuilder:printcolumn:JSONPath=".spec.version",description="version",name="VERSION",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.subnet",description="subnet",name="SUBNET",type=string
// +kubebuilder:printcolumn:JSONPath=".status.count.used",description="used",name="USED",type=string
// +kubebuilder:printcolumn:JSONPath=".status.count.total",description="total",name="TOTAL",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",description="The age of IPPool",name="AGE",type=date
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// IPPool is the Schema for the IPPools API.
type IPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPPoolSpec   `json:"spec,omitempty"`
	Status IPPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IPPoolList contains a list of IPPool.
type IPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPPool{}, &IPPoolList{})
}
