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
	ipSystem = consts.APP + "_" + "ip"
	infoKey  = "info"
)

var (
	infoGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: ipSystem,
		Name:      infoKey,
		Help:      "IP assignment information.",
	}, []string{
		"namespace", "name", "version", "ippool",
		"owner_kind", "owner_name", "owner_uid",
	})
)

func init() {
	metrics.Registry.MustRegister(infoGauge)
}

func IPInfo(namespace, name, version, pool, ownerKind, ownerName, ownerUID string) prometheus.Gauge {
	return infoGauge.WithLabelValues(
		namespace, name, version, pool,
		ownerKind, ownerName, ownerUID,
	)
}

func DeletePodIP(namespace, name string) {
	labels := map[string]string{
		"owner_kind": consts.KindPod,
		"namespace":  namespace,
		"owner_name": name,
	}
	infoGauge.DeletePartialMatch(labels)
}

func DeleteSTSIP(namespace, name string) {
	labels := map[string]string{
		"owner_kind": consts.KindStatefulSet,
		"namespace":  namespace,
		"owner_name": name,
	}
	infoGauge.DeletePartialMatch(labels)
}
