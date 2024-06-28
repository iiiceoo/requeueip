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
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/iiiceoo/requeueip/pkg/consts"
)

var arg = new(daemonArg)

type daemonArg struct {
	v              int8
	file           string
	pyroscopeAddr  string
	probeAddr      string
	unixSocketPath string
}

func (da *daemonArg) String() string {
	return fmt.Sprintf("%+v", *da)
}

func init() {
	runCmd.Flags().Int8Var(&arg.v, "v", 0, "Number for the log level verbosity.")
	runCmd.Flags().StringVar(&arg.file, "config", "/etc/requeueip/daemon.yaml", "Path to config file.")
	runCmd.Flags().
		StringVar(&arg.pyroscopeAddr, "pyroscope-address", "", "The address where the Pyroscope server runs (push mode).")
	runCmd.Flags().StringVar(&arg.probeAddr, "health-probe-address", ":8081", "The address that probe endpoint binds to.")
	runCmd.Flags().
		StringVar(&arg.unixSocketPath, "socket", consts.CNIUnixSocketPath, "The Unix socket path where the RequeueIP daemon listens.")
}

var config = new(daeminConfig)

type daeminConfig struct {
}

func (dc *daeminConfig) String() string {
	return fmt.Sprintf("%+v", *dc)
}

func (dc *daeminConfig) load(path string) error {
	bb, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(bb, dc); err != nil {
		return err
	}

	return nil
}
