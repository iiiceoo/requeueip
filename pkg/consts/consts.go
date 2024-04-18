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
	sauto4    = "sauto4.io"
	requeueip = "requeueip" + "." + sauto4
)

const (
	RAPIGroup        = requeueip
	RAPIVersion      = "v1"
	RAPIGroupVersion = RAPIGroup + "/" + RAPIVersion
)

const (
	RPrefix    = requeueip
	RFinalizer = RPrefix + "/" + "protection"
)

const mb = "managed-by"

const (
	ManagedBySubnet = "subnet" + "." + RPrefix + "/" + mb
	ManagedByIPPool = "ippool" + "." + RPrefix + "/" + mb
)
