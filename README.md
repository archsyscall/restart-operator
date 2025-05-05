# restart-operator

A Kubernetes operator for scheduling recurring restarts of workloads running in a Kubernetes cluster.

## Overview

The Restart Operator allows you to define cron-based schedules for automatically restarting Kubernetes workloads. It's perfect for applications that require periodic restarts to clear memory, refresh connections, or apply configuration changes without requiring manual intervention.

## Features

- **Cron-based scheduling**: Use standard cron expressions to define restart schedules
- **Multiple workload support**: Works with Deployments, StatefulSets, and DaemonSets
- **Namespace scoping**: Target resources in the same or different namespaces
- **Status tracking**: Keep track of the last successful restart and the next scheduled restart
- **Cross-platform**: Works on both ARM64 and AMD64 architectures

## Installation

```bash
helm repo add archsyscall https://archsyscall.github.io/restart-operator
helm repo update
helm install restart-operator archsyscall/restart-operator
```

## Usage

1. Create a `RestartSchedule` resource in your cluster:

```yaml
apiVersion: restart-operator.k8s/v1alpha1
kind: RestartSchedule
metadata:
  name: nightly-app-restart
  namespace: default
spec:
  schedule: "0 3 * * *"
  targetRef:
    kind: Deployment
    name: my-application
```

2. Check the status of your restart schedule:

```bash
kubectl get restartschedule
```

Example output:
```
NAME                 TARGET-KIND   TARGET-NAME        SCHEDULE    LAST-RESTART           AGE
nightly-app-restart   Deployment    my-application     0 3 * * *   2025-05-03T03:00:00Z   2d
```

## How It Works

The operator:

1. Watches for `RestartSchedule` resources
2. Validates the cron schedule and target resource
3. Creates a scheduled job using the cron expression
4. When the schedule triggers, adds a restart annotation to the target resource's pod template
5. Kubernetes sees the template change and initiates a rolling update
6. Updates the status with the last restart time and next scheduled restart

The restart is performed by adding/updating an annotation (`restart-operator.k8s/restartedAt`) on the pod template spec, which triggers Kubernetes to perform a rolling restart of the workload without modifying any other configuration.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.