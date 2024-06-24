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

	"gopkg.in/yaml.v2"
)

var arg = new(controllerArg)

type controllerArg struct {
	v              int8
	file           string
	pyroscopeAddr  string
	workers        int
	probeAddr      string
	metricsAddr    string
	webhookHost    string
	webhookPort    int
	webhookCertDir string
}

func (ca *controllerArg) String() string {
	return fmt.Sprintf("%+v", *ca)
}

func init() {
	runCmd.Flags().Int8Var(&arg.v, "v", 0, "Number for the log level verbosity.")
	runCmd.Flags().StringVar(&arg.file, "config", "/etc/requeueip/controller.yaml", "Path to config file.")
	runCmd.Flags().
		StringVar(&arg.pyroscopeAddr, "pyroscope-address", "", "The address where the Pyroscope server runs (push mode).")
	runCmd.Flags().IntVar(&arg.workers, "workers", 3, "Maximum number of concurrent rconciles that each controller can run.")
	runCmd.Flags().StringVar(&arg.probeAddr, "health-probe-address", ":8081", "The address that probe endpoint binds to.")
	runCmd.Flags().StringVar(&arg.metricsAddr, "metrics-address", ":8443", "The address that metrics endpoint binds to.")
	runCmd.Flags().StringVar(&arg.webhookHost, "webhook-host", "", "The host that webhook endpoint binds to.")
	runCmd.Flags().IntVar(&arg.webhookPort, "webhook-port", 9443, "The port that webhook endpoint listens on.")
	runCmd.Flags().
		StringVar(&arg.webhookCertDir, "webhook-cert-dir", "/etc/requeueip/webhook", "The directory that contains the server key and certificate for API Server and webhook TLS communication.")
}

var config = new(controllerConfig)

type controllerConfig struct {
}

func (cc *controllerConfig) String() string {
	return fmt.Sprintf("%+v", *cc)
}

func (cc *controllerConfig) load(path string) error {
	bb, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(bb, cc); err != nil {
		return err
	}

	return nil
}
