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

package run

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"

	"github.com/iiiceoo/requeueip/pkg/consts"
)

// The top-level network config - IPAM plugins are passed the full configuration
// of the calling plugin.
type Net struct {
	Name       string      `json:"name"`
	CNIVersion string      `json:"cniVersion"`
	IPAM       *IPAMConfig `json:"ipam"`
}

type IPAMConfig struct {
	Name           string
	Type           string `json:"type"`
	LogLevel       int8   `json:"logLevel"`
	LogPath        string `json:"logPath"`
	UnixSocketPath string `json:"unixSocketPath"`
}

type IPAMEnvArgs struct {
	types.CommonArgs

	PodNamespace types.UnmarshallableString `json:"K8S_POD_NAMESPACE"`
	PodName      types.UnmarshallableString `json:"K8S_POD_NAME"`
	PodUID       types.UnmarshallableString `json:"K8S_POD_UID"`
}

// LoadIPAMConfig creates IPAMConfig using json encoded configuration provided
// as `bytes`.
func LoadIPAMConfig(bytes []byte, envArgs string) (*IPAMConfig, string, error) {
	n := Net{}
	if err := json.Unmarshal(bytes, &n); err != nil {
		return nil, "", err
	}
	if n.IPAM == nil {
		return nil, "", fmt.Errorf("IPAM config missing 'ipam' key")
	}

	if n.IPAM.LogPath == "" {
		n.IPAM.LogPath = consts.CNILogPath
	}
	if n.IPAM.UnixSocketPath == "" {
		n.IPAM.UnixSocketPath = consts.CNIUnixSocketPath
	}

	// Copy net name into IPAM so not to drag Net struct around。
	n.IPAM.Name = n.Name

	return n.IPAM, n.CNIVersion, nil
}
