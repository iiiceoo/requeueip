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

package consts

const (
	requeueip = "requeueip.io"
)

const (
	RAPIGroup        = requeueip
	RAPIVersion      = "v1"
	RAPIGroupVersion = RAPIGroup + "/" + RAPIVersion
)

const (
	APP        = "requeueip"
	RPrefix    = requeueip
	RFinalizer = RPrefix + "/" + "protection"
)

const (
	RDaemon     = APP + "d"
	RController = APP + "-" + "controller"
)

const (
	LabelRefNamespace   = RPrefix + "/" + "ns-ref"
	LabelRefSubnet      = RPrefix + "/" + "subnet-ref"
	LabelRefIPPool      = RPrefix + "/" + "ippool-ref"
	LabelRefWorkloadUID = RPrefix + "/" + "workload-uid"
	LabelRefPodUID      = RPrefix + "/" + "pod-uid"
)

const (
	LabelRefSTSUID = RPrefix + "/" + "sts-uid"
	LabelRefPod    = RPrefix + "/" + "pod-ref"
)

const LabelIPVersion = RPrefix + "/" + "ip-version"

const (
	AnnoMultusDefaultNetwork = "v1.multus-cni.io/default-network"
	AnnoIPv4Subnets          = RPrefix + "/" + "ipv4-subnets"
	AnnoIPv6Subnets          = RPrefix + "/" + "ipv6-subnets"
	AnnoIPv4IPPools          = RPrefix + "/" + "ipv4-pools"
	AnnoIPv6IPPools          = RPrefix + "/" + "ipv6-pools"
	AnnoScaleDownDelay       = RPrefix + "/" + "scale-down-delay"
)

const (
	KindIPPoolClaim = "IPPoolClaim"
	KindSubnet      = "Subnet"
	KindIPBlock     = "IPBlock"
	KindIPPool      = "IPPool"
	KindIP          = "IP"
)

const (
	KindNamespace   = "Namespace"
	KindReplicaSet  = "ReplicaSet"
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
	KindPod         = "Pod"
)
