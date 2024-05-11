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

	// The IP version of IPPool.
	Version string `json:"version"`

	// +kubebuilder:validation:Required

	// The subnet to which IPPool belongs.
	Subnet string `json:"subnet"`

	// +kubebuilder:validation:Required

	// The IP ranges of IPPool, which represents a set of consecutive IP addresses.
	Ranges []string `json:"ranges"`
}

// IPPoolStatus defines the observed state of IPPool.
type IPPoolStatus struct {
	// +kubebuilder:validation:Required

	// The current available IP ranges of IPPool.
	Free []string `json:"free"`

	// +kubebuilder:validation:Optional

	// The count status of IPPool.
	Count *Count `json:"count,omitempty"`
}

// Count represents the count status of IPPool.
type Count struct {
	// +kubebuilder:validation:Required

	// The number of total IP addresses.
	Total string `json:"total"`

	// +kubebuilder:validation:Required

	// The number of used IP addresses.
	Used string `json:"used"`

	// +kubebuilder:validation:Required

	// The number of available IP addresses.
	Free string `json:"free"`
}

// +kubebuilder:resource:categories={requeueip},path="ippools",scope="Namespaced",shortName={rp},singular="ippool"
// +kubebuilder:printcolumn:JSONPath=".spec.version",description="The IP version of IPPool.",name="VERSION",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.subnet",description="The subnet to which IPPool belongs.",name="SUBNET",type=string
// +kubebuilder:printcolumn:JSONPath=".status.count.used",description="The number of used IP addresses.",name="USED",type=string
// +kubebuilder:printcolumn:JSONPath=".status.count.total",description="The number of total IP addresses.",name="TOTAL",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",description="The age of IPPool.",name="AGE",type=date
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
