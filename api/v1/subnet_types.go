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

// SubnetSpec defines the desired state of Subnet.
type SubnetSpec struct {
	// +kubebuilder:validation:Enum=IPv4;IPv6
	// +kubebuilder:validation:Optional
	Version *string `json:"version,omitempty"`

	// +kubebuilder:validation:Required
	CIDR string `json:"cidr"`

	// +kubebuilder:validation:Optional
	BlockSize *int32 `json:"blockSize,omitempty"`
}

// SubnetStatus defines the observed state of Subnet.
type SubnetStatus struct {
	// +kubebuilder:validation:Required
	Free []string `json:"free"`

	// +kubebuilder:validation:Optional
	BlockCount *BlockCount `json:"blockCount,omitempty"`
}

// +kubebuilder:resource:categories={requeueip},path="subnets",scope="Cluster",shortName={rn},singular="subnet"
// +kubebuilder:printcolumn:JSONPath=".spec.version",description="version",name="VERSION",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.cidr",description="cidr",name="CIDR",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.blockSize",description="blockSize",name="BLOCK-SIZE",type=integer
// +kubebuilder:printcolumn:JSONPath=".status.blockCount.used",description="used",name="BLOCK-USED",type=string
// +kubebuilder:printcolumn:JSONPath=".status.blockCount.total",description="total",name="BLOCK-TOTAL",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",description="The age of Subnet",name="AGE",type=date
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Subnet is the Schema for the Subnets API.
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec,omitempty"`
	Status SubnetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubnetList contains a list of Subnet.
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subnet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Subnet{}, &SubnetList{})
}
