# requeueip

![Version: 0.0.1](https://img.shields.io/badge/Version-0.0.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.1](https://img.shields.io/badge/AppVersion-0.0.1-informational?style=flat-square)

RequeueIP is an IPAM CNI plugin that assigns static IP addresses for K8s workloads.

**Homepage:** <https://github.com/iiiceoo/requeueip>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clusterDomain | string | `"cluster.local"` |  |
| controller.config | object | `{}` |  |
| controller.image.pullPolicy | string | `"IfNotPresent"` |  |
| controller.image.registry | string | `"ghcr.io/sauto4/requeueip"` |  |
| controller.image.repository | string | `"requeueip-controller"` |  |
| controller.image.tag | string | `""` |  |
| controller.imagePullSecrets | list | `[]` |  |
| controller.livenessProbe.failureThreshold | int | `6` |  |
| controller.livenessProbe.initialDelaySeconds | int | `5` |  |
| controller.livenessProbe.periodSeconds | int | `10` |  |
| controller.livenessProbe.successThreshold | int | `1` |  |
| controller.livenessProbe.timeoutSeconds | int | `1` |  |
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
| fullnameOverride | string | `""` |  |
| metricsService.port | int | `8443` |  |
| metricsService.type | string | `"ClusterIP"` |  |
| nameOverride | string | `""` |  |
| tls.auto.caValidityDuration | int | `3650` |  |
| tls.auto.certValidityDuration | int | `3650` |  |
| tls.auto.extraDNSNames | list | `[]` |  |
| tls.auto.extraIPAddresses | list | `[]` |  |
| webhookService.port | int | `9443` |  |
| webhookService.type | string | `"ClusterIP"` |  |

