# Kubernetes Metrics

Workload-total CPU and memory metrics for catalog items of type
`Kubernetes::Pod`, `Kubernetes::Deployment`, or `Kubernetes::StatefulSet`.
The plugin exposes `current` and `history` operations, with no catalog tab.

## Requirements and connection

Prometheus must scrape **cAdvisor** (kubelet container usage) and
**kube-state-metrics** (pod ownership, requests, limits, and workload replicas),
as kube-prometheus-stack does. CPU is always in **cores** and memory in **bytes**;
these are the native units of the queried metrics, with no millicore/MiB conversion.

Each mapped Prometheus instance must represent a single cluster. Namespace/pod
labels alone cannot distinguish identically named workloads from multiple clusters.
The plugin uses exact controller ownership joins, not pod-name prefixes, and
does not need a Kubernetes connection or Kubernetes API access.

Edit `Plugin.yaml` to point `spec.connections.types.prometheus` at an existing
Mission Control connection, then apply it. The plugin calls
`Host.GetConnectionByType("prometheus")`; there is no default endpoint or fallback.
Basic authentication, bearer authentication (`properties.bearer`), and OAuth
client credentials use the resolved connection. Standard HTTPS verification
is enabled; resolved `ca`, `cert`, `key`, and `insecureTLS` properties are honored
when supplied by the host.

The bundled Permission allows the plugin to read Prometheus connections only
(`types: [prometheus]`), matching the Helm chart. Connections expose their
connection type to permission selectors, not catalog types or labels. Narrow
the selector further by connection name and namespace in production. This
permission does **not** grant users permission to invoke the plugin.

## Build and install

Go 1.26.1 and Task 3.47.0 or later are required. The Dockerfile pins Task 3.47.0
so the optional-UI `if:` guard also works in image builds. From the repository
root:

```sh
make dev PLUGIN=kubernetes-metrics
kubectl apply -f kubernetes-metrics/Plugin.yaml
```

These commands also work in fish. `make dev` injects version/build metadata and
installs a debug binary under `~/.mission-control/plugins/kubernetes-metrics/`,
including the `latest` symlink. To choose the destination, pass
`PLUGIN_PATH=/path/to/plugins` to make. For a release-style local build:

```sh
task build:plugin:kubernetes-metrics VERSION=0.1.0
```

The Dockerfile and Helm chart integrate with the repository's beta and stable
release workflows. Helm defaults to remote gRPC mode on port 9000:

```sh
helm upgrade --install kubernetes-metrics kubernetes-metrics/chart \
  --namespace default \
  --set plugin.prometheusConnection=connection://default/prometheus
```

Set `mode=local` to register a host-executed binary instead. Remote servers
support the same `--serve-tls-cert`, `--serve-tls-key`, and
`--serve-tls-client-ca` flags as the other plugins; pass flags and mounts
through `extraArgs`, `extraVolumes`, and `extraVolumeMounts`.

## Operations

Both operations are non-destructive, config-scoped, and return `application/json`.
Both accept GET and POST through Mission Control's plugin gateway:

```sh
curl --fail-with-body \
  -H "Authorization: Bearer $MC_TOKEN" \
  "$MC_URL/api/plugins/kubernetes-metrics/proxy/current?config_id=$CONFIG_ID"

curl --fail-with-body \
  -H "Authorization: Bearer $MC_TOKEN" \
  "$MC_URL/api/plugins/kubernetes-metrics/proxy/history?config_id=$CONFIG_ID&range=1h&step=15s"

curl --fail-with-body \
  -H "Authorization: Bearer $MC_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"range":"1h","step":"15s"}' \
  "$MC_URL/api/plugins/kubernetes-metrics/proxy/history?config_id=$CONFIG_ID"
```

The curl examples work in bash and fish with `MC_URL`, `MC_TOKEN`, and
`CONFIG_ID` already set. `config_id` is a catalog UUID and stays in the query
string for POST as well. Namespace, kind, and workload name come only from
the catalog item, not operation parameters.

### `current`

No parameters. Returns all six totals at one evaluation timestamp:

```json
{
  "at": "2026-10-07T10:15:30Z",
  "pods": 2,
  "cpu": {"usage": 0.412, "request": 0.5, "limit": 1},
  "memory": {"usage": 734003200, "request": 536870912, "limit": 1073741824}
}
```

Missing samples are `null`, never fabricated zeroes. Requests and limits remain
`null` when no matching pod sets them. A numeric zero returned by Prometheus
is preserved. Non-finite samples are represented as `null`.

`pods` counts distinct matching pod metadata series and is `0` if none exist.
For Deployments and StatefulSets, only `Pending` and `Running` pods are counted,
and only those pods contribute usage, requests, and limits; Failed, Succeeded,
and Evicted pods that still have kube-state-metrics series are excluded.
An empty owner join does not prove a workload was scaled to zero. Only when
both desired and observed Deployment/StatefulSet replica metrics confirm zero
are missing current usage values reported as `0`; otherwise they remain `null`.
A Pod with no usage samples reports `null` rather than inventing usage.

### `history`

`range` is required and must be `15m`, `1h`, `6h`, or `24h`. `step` is an optional
positive Go duration (for example `15s` or `1m`), defaulting to `15s`. The plugin
raises too-small steps to bound each series to at most 1000 points, including
both endpoints, and returns the effective step. A fully populated `1h` range
at `15s` contains 241 points.

```json
{
  "range": "1h",
  "step": "15s",
  "cpu": {
    "usage": [{"at": "2026-10-07T09:15:30Z", "value": 0.38}],
    "limit": [{"at": "2026-10-07T09:15:30Z", "value": 1}]
  },
  "memory": {
    "usage": [{"at": "2026-10-07T09:15:30Z", "value": 712000000}],
    "limit": [{"at": "2026-10-07T09:15:30Z", "value": 1073741824}]
  }
}
```

All four series are returned together, sorted ascending by timestamp. No
samples means `[]`; history does not synthesize samples for zero replicas.
Missing connections, query failures, and Prometheus warnings fail the operation
instead of returning partial or fake results.

Invalid range/step or an unresolved/unsupported workload returns HTTP 400 with
`error_code: EINVALID`. SDK v0.0.6 hard-codes RPC handler errors to
`HANDLER_ERROR`; validation messages still begin with `EINVALID:` over RPC.
A config item the host reports as not found or forbidden returns HTTP 404
(`ENOTFOUND`) or 403 (`EFORBIDDEN`). Current hosts also report missing and
RLS-hidden items as `Internal` or `Unknown` with the exact message
`config item <config_id>: record not found`; the plugin maps only that lookup
message to 404, without distinguishing absence from denied visibility.
Other plugin HTTP failures return 502 with the underlying error message.

## Caller authorization

Mission Control's `EnforceInvokePermission` enforces both `read` on the config
item and the exact `invoke:kubernetes-metrics:current` or
`invoke:kubernetes-metrics:history` action. For example:

```yaml
apiVersion: mission-control.flanksource.com/v1
kind: Scope
metadata:
  name: kubernetes-metrics-workloads
  namespace: default
spec:
  targets:
    - config:
        types:
          - Kubernetes::Pod
          - Kubernetes::Deployment
          - Kubernetes::StatefulSet
---
apiVersion: mission-control.flanksource.com/v1
kind: Role
metadata:
  name: kubernetes-metrics-reader
  namespace: default
spec:
  rules:
    - name: read-workloads
      action: read
      resource:
        scopeRef: kubernetes-metrics-workloads
    - name: current-metrics
      action: invoke:kubernetes-metrics:current
      resource:
        scopeRef: kubernetes-metrics-workloads
    - name: historical-metrics
      action: invoke:kubernetes-metrics:history
      resource:
        scopeRef: kubernetes-metrics-workloads
---
apiVersion: mission-control.flanksource.com/v1
kind: RoleBinding
metadata:
  name: kubernetes-metrics-readers
  namespace: default
spec:
  role: kubernetes-metrics-reader
  subjects:
    teams:
      - platform
```

Narrow the Scope and replace the team before applying. No sampler, metrics-server
fallback, disk/network metrics, or per-pod/container breakdown is included.
