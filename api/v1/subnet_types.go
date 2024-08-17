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

	// The IP version of Subnet.
	Version *string `json:"version,omitempty"`

	// +kubebuilder:validation:Required

	// The CIDR of Subnet.
	CIDR string `json:"cidr"`

	// +kubebuilder:validation:Optional

	// The IP ranges excluded from Subnet.
	Excluded []string `json:"excluded,omitempty"`

	// +kubebuilder:validation:Optional

	// The minimum unit of IP address assignments from Subnet. Defaults to 30
	// for IPv4 and 126 for IPv6.
	BlockSize *int32 `json:"blockSize,omitempty"`
}

// SubnetStatus defines the observed state of Subnet.
type SubnetStatus struct {
	// +kubebuilder:validation:Required

	// The current available IP ranges of Subnet.
	Free []string `json:"free"`

	// +kubebuilder:validation:Optional

	// The IP block count status of Subnet.
	BlockCount *BlockCount `json:"blockCount,omitempty"`
}

// BlockCount represents the IP block count status of Subnet.
type BlockCount struct {
	// +kubebuilder:validation:Required

	// The number of total IP blocks.
	Total string `json:"total"`

	// +kubebuilder:validation:Required

	// The number of used IP blocks.
	Used string `json:"used"`

	// +kubebuilder:validation:Required

	// The number of available IP blocks.
	Free string `json:"free"`
}

// +kubebuilder:resource:categories={requeueip},path="subnets",scope="Cluster",shortName={rn},singular="subnet"
// +kubebuilder:printcolumn:JSONPath=".spec.version",description="The IP version of Subnet.",name="VERSION",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.cidr",description="The CIDR of Subnet.",name="CIDR",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.blockSize",description="The minimum unit of IP address assignments from Subnet.",name="BLOCK-SIZE",type=integer
// +kubebuilder:printcolumn:JSONPath=".status.blockCount.used",description="The number of used IP blocks.",name="BLOCK-USED",type=string
// +kubebuilder:printcolumn:JSONPath=".status.blockCount.total",description="The number of total IP blocks.",name="BLOCK-TOTAL",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",description="The age of Subnet.",name="AGE",type=date
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
