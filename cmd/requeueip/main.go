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

package main

import (
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"

	"github.com/iiiceoo/requeueip/cmd/requeueip/run"
	rversion "github.com/iiiceoo/requeueip/internal/version"
)

// The minimum CNI spec version supported by RequesteIP IPAM CNI.
const min = "0.0.3"

func main() {
	text, _ := rversion.Get().Text()
	skel.PluginMainFuncs(skel.CNIFuncs{
		Add: run.CmdAdd,
		Del: run.CmdDel,
	}, version.VersionsStartingFrom(min), text)
}
