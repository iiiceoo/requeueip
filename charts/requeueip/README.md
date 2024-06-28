# requeueip

![Version: 0.0.1](https://img.shields.io/badge/Version-0.0.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.1](https://img.shields.io/badge/AppVersion-0.0.1-informational?style=flat-square)

RequeueIP is an IPAM CNI plugin that assigns static IP addresses for K8s workloads.

**Homepage:** <https://github.com/iiiceoo/requeueip>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clusterDomain | string | `"cluster.local"` |  |
| controller.image.pullPolicy | string | `"IfNotPresent"` |  |
| controller.image.registry | string | `"ghcr.io/requeueip"` |  |
| controller.image.repository | string | `"requeueip-controller"` |  |
| controller.image.tag | string | `""` |  |
| controller.imagePullSecrets | list | `[]` |  |
| controller.livenessProbe.failureThreshold | int | `6` |  |
| controller.livenessProbe.initialDelaySeconds | int | `5` |  |
| controller.livenessProbe.periodSeconds | int | `10` |  |
| controller.livenessProbe.successThreshold | int | `1` |  |
| controller.livenessProbe.timeoutSeconds | int | `1` |  |
| controller.metricsService.port | int | `8443` |  |
| controller.metricsService.type | string | `"ClusterIP"` |  |
| controller.readinessProbe.failureThreshold | int | `3` |  |
| controller.readinessProbe.initialDelaySeconds | int | `10` |  |
| controller.readinessProbe.periodSeconds | int | `10` |  |
| controller.readinessProbe.successThreshold | int | `1` |  |
| controller.readinessProbe.timeoutSeconds | int | `1` |  |
| controller.replicas | int | `1` |  |
| controller.resources.limits.cpu | string | `"500m"` |  |
| controller.resources.limits.memory | string | `"500Mi"` |  |
| controller.resources.requests.cpu | string | `"100m"` |  |
| controller.resources.requests.memory | string | `"200Mi"` |  |
| daemon.image.pullPolicy | string | `"IfNotPresent"` |  |
| daemon.image.registry | string | `"ghcr.io/requeueip"` |  |
| daemon.image.repository | string | `"requeueipd"` |  |
| daemon.image.tag | string | `""` |  |
| daemon.imagePullSecrets | list | `[]` |  |
| daemon.livenessProbe.failureThreshold | int | `6` |  |
| daemon.livenessProbe.initialDelaySeconds | int | `5` |  |
| daemon.livenessProbe.periodSeconds | int | `10` |  |
| daemon.livenessProbe.successThreshold | int | `1` |  |
| daemon.livenessProbe.timeoutSeconds | int | `1` |  |
| daemon.metricsService.port | int | `8443` |  |
| daemon.metricsService.type | string | `"ClusterIP"` |  |
| daemon.readinessProbe.failureThreshold | int | `3` |  |
| daemon.readinessProbe.initialDelaySeconds | int | `10` |  |
| daemon.readinessProbe.periodSeconds | int | `10` |  |
| daemon.readinessProbe.successThreshold | int | `1` |  |
| daemon.readinessProbe.timeoutSeconds | int | `1` |  |
| daemon.resources.limits.cpu | string | `"500m"` |  |
| daemon.resources.limits.memory | string | `"500Mi"` |  |
| daemon.resources.requests.cpu | string | `"100m"` |  |
| daemon.resources.requests.memory | string | `"200Mi"` |  |
| daemon.socket | string | `"/var/run/requeueip/cni.sock"` |  |
| daemon.updateStrategy.rollingUpdate.maxUnavailable | int | `1` |  |
| daemon.updateStrategy.type | string | `"RollingUpdate"` |  |
| fullnameOverride | string | `""` |  |
| multus.config.chrootDir | string | `"/hostroot"` |  |
| multus.config.cniConfigDir | string | `"/host/etc/cni/net.d"` |  |
| multus.config.cniVersion | string | `"0.3.1"` |  |
| multus.config.logLevel | string | `"verbose"` |  |
| multus.config.logToStderr | bool | `true` |  |
| multus.config.multusAutoconfigDir | string | `"/host/etc/cni/net.d"` |  |
| multus.config.multusConfigFile | string | `"auto"` |  |
| multus.config.socketDir | string | `"/host/run/multus"` |  |
| multus.enabled | bool | `true` |  |
| multus.image.pullPolicy | string | `"IfNotPresent"` |  |
| multus.image.registry | string | `"ghcr.io/k8snetworkplumbingwg"` |  |
| multus.image.repository | string | `"multus-cni"` |  |
| multus.image.tag | string | `"snapshot-thick"` |  |
| multus.imagePullSecrets | list | `[]` |  |
| multus.nad.config.cniVersion | string | `"0.3.1"` |  |
| multus.nad.config.name | string | `"static-network"` |  |
| multus.nad.config.plugins[0].datastore_type | string | `"kubernetes"` |  |
| multus.nad.config.plugins[0].ipam.ipv4 | bool | `true` |  |
| multus.nad.config.plugins[0].ipam.ipv6 | bool | `false` |  |
| multus.nad.config.plugins[0].ipam.type | string | `"requeueip"` |  |
| multus.nad.config.plugins[0].kubernetes.kubeconfig | string | `"/etc/cni/net.d/calico-kubeconfig"` |  |
| multus.nad.config.plugins[0].log_file_path | string | `"/var/log/calico/cni/cni.log"` |  |
| multus.nad.config.plugins[0].log_level | string | `"info"` |  |
| multus.nad.config.plugins[0].policy.type | string | `"k8s"` |  |
| multus.nad.config.plugins[0].type | string | `"calico"` |  |
| multus.nad.config.plugins[1].capabilities.portMappings | bool | `true` |  |
| multus.nad.config.plugins[1].snat | bool | `true` |  |
| multus.nad.config.plugins[1].type | string | `"portmap"` |  |
| multus.nad.config.plugins[2].capabilities.bandwidth | bool | `true` |  |
| multus.nad.config.plugins[2].type | string | `"bandwidth"` |  |
| multus.nad.name | string | `"calico-requeueip"` |  |
| multus.resources.limits.cpu | string | `"2000m"` |  |
| multus.resources.limits.memory | string | `"1024Mi"` |  |
| multus.resources.requests.cpu | string | `"250m"` |  |
| multus.resources.requests.memory | string | `"128Mi"` |  |
| multus.updateStrategy.rollingUpdate.maxUnavailable | int | `1` |  |
| multus.updateStrategy.type | string | `"RollingUpdate"` |  |
| nameOverride | string | `""` |  |
| tls.auto.caValidityDuration | int | `3650` |  |
| tls.auto.certValidityDuration | int | `3650` |  |
| tls.auto.extraDNSNames | list | `[]` |  |
| tls.auto.extraIPAddresses | list | `[]` |  |
| webhookService.port | int | `9443` |  |
| webhookService.type | string | `"ClusterIP"` |  |

