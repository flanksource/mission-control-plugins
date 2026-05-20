# Arthas Mission Control Plugin

The Arthas plugin adds JVM diagnostics to Mission Control for Kubernetes workloads. It attaches [Arthas](https://arthas.aliyun.com/) to a Java process running in a selected pod and exposes the Arthas web console, HTTP API, and diagnostic operations from the Mission Control UI.

## What it does

- Adds an **Arthas** tab to Kubernetes catalog items:
  - `Kubernetes::Pod`
  - `Kubernetes::Deployment`
  - `Kubernetes::StatefulSet`
  - `Kubernetes::DaemonSet`
  - `Kubernetes::ReplicaSet`
  - `Kubernetes::Job`
  - `Kubernetes::CronJob`
- Resolves the selected workload to a running pod and container.
- Copies/installs `arthas-boot.jar` into the target pod when needed.
- Attaches Arthas to the JVM in the target container.
- Opens Kubernetes port-forwards to the Arthas HTTP console and optional MCP endpoint.
- Proxies the Arthas UI/API through Mission Control.
- Tracks active sessions inside the plugin process.

## Operations

| Operation        | Purpose                                                         |
| ---------------- | --------------------------------------------------------------- |
| `sessions-list`  | List active Arthas sessions.                                    |
| `session-create` | Attach Arthas to the selected workload or pod.                  |
| `session-delete` | Stop and remove a plugin session and close port-forwards.       |
| `pods-list`      | List ready pods that can be targeted for the selected workload. |
| `exec`           | Execute an Arthas command through the Arthas HTTP API.          |

## Kubernetes access

The plugin needs Kubernetes API access to resolve pods, exec into containers, and create port-forwards.

At runtime it first tries to resolve a Mission Control Kubernetes connection via:

```go
host.GetConnectionByType(ctx, sdk.ConnectionTypeKubernetes)
```

The Plugin CRD must map that connection type, for example:

```yaml
spec:
  connections:
    types:
      kubernetes: connection://mission-control/kubernetes-vps-d1140a41
```

If no usable host connection is available, it falls back to `rest.InClusterConfig()`, using the service account of the plugin process.

## Configuration

This plugin currently does not consume `spec.properties`; `Configure()` is a no-op.

## Build

From the repository root:

```sh
task build:plugin:arthas
```
