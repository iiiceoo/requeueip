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
	poolSystem = consts.APP + "_" + "ippool"
	ipTotalKey = "ip_total"
	ipUsageKey = "ip_usage"
)

var (
	ipTotalGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: poolSystem,
		Name:      ipTotalKey,
		Help:      "The number of total IP addresses.",
	}, []string{"namespace", "name", "version", "subnet"})

	ipUsageGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: poolSystem,
		Name:      ipUsageKey,
		Help:      "The number of used IP addresses.",
	}, []string{"namespace", "name", "version", "subnet"})
)

func init() {
	metrics.Registry.MustRegister(ipTotalGauge)
	metrics.Registry.MustRegister(ipUsageGauge)
}

func IPPoolIPTotal(namespace, name, version, subnet string) prometheus.Gauge {
	return ipTotalGauge.WithLabelValues(namespace, name, version, subnet)
}

func IPPoolIPUsage(namespace, name, version, subnet string) prometheus.Gauge {
	return ipUsageGauge.WithLabelValues(namespace, name, version, subnet)
}
