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

const None = "<none>"

const (
	claimSystem = consts.APP + "_" + "ippool_claim"
	replicasKey = "replicas"
	poolSizeKey = "pool_size"
)

var (
	replicasGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: claimSystem,
		Name:      replicasKey,
		Help: "The total number of IP addresses of the IPPool to be synced. " +
			"It should always be consistent with the replica of the owner workload.",
	}, []string{
		"namespace", "name", "version", "subnet",
		"owner_kind", "owner_name", "owner_uid",
	})

	poolSizeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: claimSystem,
		Name:      poolSizeKey,
		Help:      "The current size of the IPPool created based on the claim.",
	}, []string{
		"namespace", "name", "version", "subnet",
		"owner_kind", "owner_name", "owner_uid",
	})
)

func init() {
	metrics.Registry.MustRegister(replicasGauge)
	metrics.Registry.MustRegister(poolSizeGauge)
}

func IPPoolClaimReplicas(namespace, name, version, subnet, ownerKind, ownerName, ownerUID string) prometheus.Gauge {
	return replicasGauge.WithLabelValues(
		namespace, name, version, subnet,
		ownerKind, ownerName, ownerUID,
	)
}

func IPPoolClaimPoolSize(namespace, name, version, subnet, ownerKind, ownerName, ownerUID string) prometheus.Gauge {
	return poolSizeGauge.WithLabelValues(
		namespace, name, version, subnet,
		ownerKind, ownerName, ownerUID,
	)
}

func DeleteIPPoolClaim(namespace, name string) {
	labels := map[string]string{
		"namespace": namespace,
		"name":      name,
	}
	replicasGauge.DeletePartialMatch(labels)
	poolSizeGauge.DeletePartialMatch(labels)
}
