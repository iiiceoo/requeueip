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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/iiiceoo/requeueip/pkg/consts"
)

const (
	subnetSystem  = consts.APP + "_" + "subnet"
	blockTotalKey = "block_total"
	blockUsageKey = "block_usage"
)

var (
	blockTotalGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: subnetSystem,
		Name:      blockTotalKey,
		Help:      "The number of total IP blocks.",
	}, []string{"name", "version", "cidr", "block_size"})

	blockUsageGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: subnetSystem,
		Name:      blockUsageKey,
		Help:      "The number of used IP blocks.",
	}, []string{"name", "version", "cidr", "block_size"})
)

func init() {
	metrics.Registry.MustRegister(blockTotalGauge)
	metrics.Registry.MustRegister(blockUsageGauge)
}

func SubnetBlockTotal(name, version, cidr, blockSize string) prometheus.Gauge {
	return blockTotalGauge.WithLabelValues(name, version, cidr, blockSize)
}

func SubnetBlockUsage(name, version, cidr, blockSize string) prometheus.Gauge {
	return blockUsageGauge.WithLabelValues(name, version, cidr, blockSize)
}

func DeleteSubnet(name string) {
	labels := map[string]string{"name": name}
	blockTotalGauge.DeletePartialMatch(labels)
	blockUsageGauge.DeletePartialMatch(labels)
}
