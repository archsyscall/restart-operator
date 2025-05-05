# Restart Operator Helm Chart

This Helm chart installs the Kubernetes Restart Operator, which enables scheduled restarts of Kubernetes workloads using cron expressions.

## Introduction

The Restart Operator allows users to define restart schedules for their Kubernetes workloads (Deployments, StatefulSets, and DaemonSets) using a custom RestartSchedule resource.

## Prerequisites

- Kubernetes 1.16+
- Helm 3.0+

## Installing the Chart

To install the chart with the release name `restart-operator`:

```bash
helm install restart-operator ./charts/restart-operator
```

## Configuration

The following table lists the configurable parameters of the Restart Operator chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of operator replicas | `1` |
| `image.repository` | Image repository | `archsyscall/restart-operator` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Image tag | `latest` |
| `imagePullSecrets` | Image pull secrets | `[]` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full chart name | `""` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` |
| `securityContext` | Security context | See values.yaml |
| `resources` | Pod resources | See values.yaml |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Pod tolerations | `[]` |
| `affinity` | Pod affinity | `{}` |
| `operator.watchNamespace` | Namespace to watch (empty for all) | `""` |
| `operator.logLevel` | Log level | `info` |
| `operator.leaderElection.enabled` | Enable leader election | `true` |
| `operator.leaderElection.resourceName` | Leader election resource name | `restart-operator-leader-election` |
| `operator.metrics.enabled` | Enable metrics | `true` |
| `operator.metrics.port` | Metrics port | `8080` |
| `operator.healthProbe.port` | Health probe port | `8081` |
| `rbac.create` | Create RBAC resources | `true` |

## Usage Example

Create a RestartSchedule resource to restart a deployment every day at 2 AM:

```yaml
apiVersion: restart-operator.k8s/v1alpha1
kind: RestartSchedule
metadata:
  name: nightly-restart
  namespace: default
spec:
  schedule: "0 2 * * *"
  targetRef:
    kind: Deployment
    name: my-deployment
```

## Uninstalling the Chart

To uninstall/delete the `restart-operator` deployment:

```bash
helm delete restart-operator
```

This will remove all the Kubernetes components associated with the chart and delete the release.