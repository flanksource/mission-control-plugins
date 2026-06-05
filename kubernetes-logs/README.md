# Kubernetes Logs Plugin

Reference plugin: streams logs from a Pod, Deployment, StatefulSet, DaemonSet,
ReplicaSet, Job, or CronJob, walking owner references to fan out across every
matching pod.

## What it shows the SDK author

- Reading the catalog item (`Host.GetConfigItem`) to learn `kind / namespace / name`.
- Resolving the Kubernetes connection for the selected catalog item (`Host.GetConnectionForConfig`).
- An iframe UI that calls back into the host's operation API for `list-pods`,
  then calls the plugin's HTTP log endpoint for one-shot logs or follow-mode
  streaming.
- The `list-pods` operation and HTTP log contract (`/proxy/logs`) coexisting on
  the same plugin port.

## Build & install

```sh
mkdir -p $MISSION_CONTROL_PLUGIN_PATH
go build -o $MISSION_CONTROL_PLUGIN_PATH/kubernetes-logs ./kubernetes-logs
kubectl apply -f kubernetes-logs/Plugin.yaml
```

## CLI

```sh
# Resolve which pods a workload maps to:
mission-control plugin kubernetes-logs list-pods --config-id <uuid>
```

## HTTP

```sh
# One-shot HTTP logs, equivalent to kubectl logs --tail=50:
curl \
  "$MISSION_CONTROL_URL/api/plugins/kubernetes-logs/proxy/logs?config_id=<uuid>&namespace=default&pod=<pod>&tailLines=50&follow=false"

# Follow only newly-created lines, equivalent to kubectl logs -f --tail=0:
curl -N \
  "$MISSION_CONTROL_URL/api/plugins/kubernetes-logs/proxy/logs?config_id=<uuid>&namespace=default&pod=<pod>&follow=true"
```

The one-shot endpoint returns `application/json`. Follow mode returns
`text/event-stream`.

## Iframe UI

Open any matching catalog item — the **Logs** tab opens an embedded terminal-style
viewer with pod/container selectors and follow-mode streaming.

## Connection resolution

The plugin asks Mission Control for the connection used by the scraper that
created the selected config item. If Mission Control cannot resolve one, the
plugin falls back to its own in-cluster service account.
