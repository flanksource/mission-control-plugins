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

The UI uses `@flanksource/mission-control-sdk`'s `createEmbeddedPluginClient()`
for operation calls, token authentication, and fetch-based SSE parsing. Follow
streams do not reconnect automatically; use **Refresh logs** to restart one.

### Third-party embedding

Use `/api/plugins/kubernetes-logs/ui/?config_id=<uuid>&embed=token` as the iframe
URL. Cross-origin parents always require token mode, even if `embed=token` is
omitted. Top-level pages and same-origin parents use the Mission Control session
cookie unless token mode is explicitly requested. In token mode, all operation
requests (including follow streams) omit cookies and send
`X-Flanksource-Plugin-Invocation`; no request is made before a token arrives.

The SDK posts `{ type: 'mc.tab.ready' }` when the client is created. The parent
must reply with `{ type: 'mc.token', token, expiresInSeconds }`, using Mission Control's origin
as `postMessage`'s `targetOrigin`, never `'*'`. Only messages from the parent
window are accepted, and tokens stay in memory.

Mint each iframe's plugin/config-scoped token on the host backend by calling
`GET /api/plugins/kubernetes-logs/ui-token?config_id=<uuid>` with the user's
federated JWT. Refresh it before its five-minute expiry and whenever the iframe
posts `{ type: 'mc.token.request' }`; the SDK's `createPluginEmbed()` host helper
can manage this lifecycle. The host owns pre-expiry renewal. On HTTP 401, the
iframe waits for a new token and retries once; HTTP 403 is surfaced without
requesting renewal. Unanswered token requests repeat every five seconds, and
each wait fails after 30 seconds. If no refresh arrives before expiry, in-flight
requests and follow streams are cancelled. Stop refreshing when the iframe is
removed.

## Connection resolution

The plugin asks Mission Control for the connection used by the scraper that
created the selected config item. If Mission Control cannot resolve one, the
plugin falls back to its own in-cluster service account.
