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
	refPrefix           = "reference" + "." + RPrefix
	LabelRefNamespace   = refPrefix + "/" + "ns"
	LabelRefSubnet      = refPrefix + "/" + "subnet"
	LabelRefIPPool      = refPrefix + "/" + "ippool"
	LabelRefWorkloadUID = refPrefix + "/" + "workload-uid"
	LabelRefPodUID      = refPrefix + "/" + "pod-uid"
)

const (
	LabelRefSTSUID = RPrefix + "/" + "sts-uid"
	LabelRefPod    = RPrefix + "/" + "pod"
)

const LabelIPVersion = RPrefix + "/" + "ip-version"

const (
	AnnoIPv4Subnets = RPrefix + "/" + "ipv4-subnets"
	AnnoIPv6Subnets = RPrefix + "/" + "ipv6-subnets"
)

const (
	calicoPrefix   = "calico" + "." + RPrefix
	AnnoCalicoSYNC = calicoPrefix + "/" + "sync"
)
