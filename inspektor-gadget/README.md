# Inspektor Gadget Mission Control Plugin

The Inspektor Gadget plugin adds eBPF-based Kubernetes workload diagnostics to Mission Control. 
It can install/check the Inspektor Gadget deployment and run bounded gadget trace sessions against selected Kubernetes resources from the Mission Control UI.

## What it does

- Adds a **Gadget** tab to Kubernetes catalog items:
  - `Kubernetes::Pod`
  - `Kubernetes::Deployment`
  - `Kubernetes::StatefulSet`
  - `Kubernetes::DaemonSet`
  - `Kubernetes::ReplicaSet`
  - `Kubernetes::Job`
  - `Kubernetes::CronJob`
- Checks whether Inspektor Gadget is deployed and ready.
- Can generate or apply the Inspektor Gadget Kubernetes manifest.
- Resolves the selected workload to pods, containers, nodes, or selectors.
- Starts eBPF gadget runs through the Inspektor Gadget gadget-service API.
- Uses Kubernetes API port-forwarding to reach gadget-service pods.
- Buffers trace events in the plugin process for UI/API retrieval.

## Supported gadget categories

The plugin exposes a curated list of Inspektor Gadget gadgets, including:

- Runtime traces: exec, signals, OOM kills, malloc, deadlock, tty snoop
- Network traces: DNS, TCP, TCP drops/retransmits, SNI, SSL, bind, tcpdump
- File traces: open, fsnotify, fsslower, mount, file top/snapshot
- Security traces: seccomp audit/advice, Linux capabilities, LSM, module load
- Profiles and snapshots: CPU, block I/O, TCP RTT, process/socket/file snapshots
- Top-style gadgets: process, TCP, file, block I/O, CPU throttling, BPF stats

## Operations

| Operation      | Purpose                                                         |
| -------------- | --------------------------------------------------------------- |
| `status`       | Check Inspektor Gadget deployment readiness.                    |
| `install-plan` | Return the Kubernetes manifest that would be applied.           |
| `install`      | Apply the Inspektor Gadget manifest through the Kubernetes API. |
| `traces-list`  | List supported gadgets.                                         |
| `trace-start`  | Start a bounded trace session for the selected resource.        |
| `trace-stop`   | Stop a running trace session.                                   |
| `trace-list`   | List active and recent trace sessions.                          |
| `trace-events` | Return buffered events for a trace session.                     |

## Kubernetes access

The plugin needs Kubernetes API access to inspect/install Inspektor Gadget, resolve workload pods, port-forward to gadget-service, and run traces.

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

The plugin supports these `spec.properties` settings:

| Property          | Default   | Description                                                    |
| ----------------- | --------- | -------------------------------------------------------------- |
| `gadgetNamespace` | `gadget`  | Namespace where Inspektor Gadget is installed.                 |
| `gadgetTag`       | `v0.52.0` | Inspektor Gadget image tag used for install and gadget images. |
| `maxDurationSec`  | `900`     | Maximum trace duration in seconds.                             |
| `maxEvents`       | `10000`   | Maximum buffered events per session.                           |
| `maxSessions`     | `5`       | Maximum concurrent trace sessions.                             |

## Build

From the repository root:

```sh
task build:plugin:inspektor-gadget
```
