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

// IPPoolClaimSpec defines the desired state of IPPoolClaim.
type IPPoolClaimSpec struct {
	// +kubebuilder:validation:Enum=IPv4;IPv6
	// +kubebuilder:validation:Required

	// Version specifies the IP version (IPv4 or IPv6) for the IPPool.
	Version string `json:"version"`

	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required

	// Subnets is a list of candidate Subnet names for IP address assignments.
	Subnets []string `json:"subnets"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required

	// Replicas is the expected number of IP addresses to allocate from the
	// IPPool, which must match the replica count of the owner workload.
	Replicas int32 `json:"replicas"`
}

// IPPoolClaimStatus defines the observed state of IPPoolClaim.
type IPPoolClaimStatus struct {
	// +kubebuilder:validation:Optional

	// Subnet is the selected Subnet name from the candidate list.
	Subnet *string `json:"subnet,omitempty"`

	// +kubebuilder:validation:Optional

	// PoolSize is the current size of the IPPool created based on the claim.
	PoolSize *int32 `json:"poolSize,omitempty"`
}

// +kubebuilder:resource:categories={requeueip},path="ippoolclaims",scope="Namespaced",shortName={rpc},singular="ippoolclaim"
// +kubebuilder:printcolumn:JSONPath=".spec.version",description="The IP version of the IPPool to be synced.",name="VERSION",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.replicas",description="The total number of IP addresses of the IPPool to be synced.",name="REPLICAS",type=integer
// +kubebuilder:printcolumn:JSONPath=".status.subnet",description="The Subnet selected from the candidate Subnets.",name="SUBNET",type=string
// +kubebuilder:printcolumn:JSONPath=".status.poolSize",description="The current size of the IPPool created based on the claim.",name="POOL-SIZE",type=integer
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",description="The age of IPPoolClaim.",name="AGE",type=date
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// IPPoolClaim is the Schema for the IPPoolClaims API.
type IPPoolClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPPoolClaimSpec   `json:"spec,omitempty"`
	Status IPPoolClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IPPoolClaimList contains a list of IPPoolClaim.
type IPPoolClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPPoolClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPPoolClaim{}, &IPPoolClaimList{})
}
