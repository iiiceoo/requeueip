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
	Version string `json:"version"`

	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	Subnets []string `json:"subnets"`

	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	Replicas int32 `json:"replicas"`
}

// +kubebuilder:resource:categories={requeueip},path="ippoolclaims",scope="Namespaced",shortName={rpc},singular="ippoolclaim"
// +kubebuilder:printcolumn:JSONPath=".spec.version",description="version",name="VERSION",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.subnet",description="subnet",name="SUBNET",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.replicas",description="replicas",name="REPLICAS",type=integer
// +kubebuilder:object:root=true

// IPPoolClaim is the Schema for the IPPoolClaims API.
type IPPoolClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec IPPoolClaimSpec `json:"spec,omitempty"`
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
