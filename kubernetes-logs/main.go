// Reference plugin: stream logs from a Kubernetes Pod (or its ancestor —
// Deployment / StatefulSet / DaemonSet / ReplicaSet / Job / CronJob), using
// the Plugin CRD's kubernetes connection.
//
// Build: go build -o $MISSION_CONTROL_PLUGIN_PATH/kubernetes-logs ./kubernetes-logs
// Apply: kubectl apply -f kubernetes-logs/Plugin.yaml
package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
)

//go:generate go run ./internal/gen-checksum

//go:embed all:ui
var uiAssets embed.FS

// Version and BuildDate are injected at link time via the Taskfile's
// build:plugin:kubernetes-logs target. Empty values trip the SDK's
// RegisterPlugin guard, so dev binaries built without ldflags fail loudly.
var (
	Version   = ""
	BuildDate = ""
)

func main() {
	sub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		panic(err)
	}
	sdk.Serve(&KubernetesLogsPlugin{}, sdk.WithStaticAssets(sub))
}

type KubernetesLogsPlugin struct {
	clients clientCache
}

func (p *KubernetesLogsPlugin) Manifest() *pluginpb.PluginManifest {
	// Suffix the UI bundle checksum onto the version so the host can detect
	// a UI rebuild and the iframe URL the frontend constructs changes
	// (`?config_id=…&_v=<sha>`), forcing browsers to bypass any stale cache.
	return &pluginpb.PluginManifest{
		Name:         "kubernetes-logs",
		Version:      sdk.FormatVersion(Version, BuildDate, uiChecksum),
		Description:  "Stream logs from a Pod or any of its workload ancestors (Deployment / StatefulSet / DaemonSet / Job).",
		Capabilities: []string{"tabs", "operations"},
		Tabs: []*pluginpb.TabSpec{
			{Name: "Logs", Icon: "lucide:scroll-text", Path: "/", Scope: "config"},
		},
	}
}

func (p *KubernetesLogsPlugin) Configure(_ context.Context, _ map[string]any) error {
	return nil
}

func (p *KubernetesLogsPlugin) Operations() []sdk.Operation {
	return []sdk.Operation{
		{
			Def: &pluginpb.OperationDef{
				Name:        "list-pods",
				Description: "Resolve a config item to its Pods, walking workload ancestors.",
				Scope:       "config",
				ResultMime:  sdk.ClickyResultMimeType,
				Http: []*pluginpb.HTTPBinding{
					{Method: http.MethodGet},
				},
			},
			Handler:     p.listPods,
			HTTPHandler: http.HandlerFunc(p.httpListPods),
		},
		{
			Def: &pluginpb.OperationDef{
				Name:        "logs",
				Description: "Fetch Kubernetes pod logs, or stream new lines when follow=true.",
				Scope:       "config",
				ResultMime:  "application/json",
				Http: []*pluginpb.HTTPBinding{
					{Method: http.MethodGet},
				},
			},
			HTTPHandler: http.HandlerFunc(p.httpLogs),
		},
	}
}

func (p *KubernetesLogsPlugin) listPods(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	cli, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	pods, err := resolvePods(ctx, cli, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	return podRows(pods), nil
}
