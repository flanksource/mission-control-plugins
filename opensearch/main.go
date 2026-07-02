// OpenSearch plugin: generic profile-based trace querying for Mission Control.
//
// Build: task -d plugins/opensearch build
// Apply: kubectl apply -f plugins/opensearch/Plugin.yaml
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
)

const (
	OpProfilesList    = "profiles-list"
	OpTraceQuery      = "trace-query"
	OpQueryPreview    = "query-preview"
	OpConnectionCheck = "connection-check"
	pluginName        = "opensearch"
)

//go:generate go run ./internal/gen-checksum

//go:embed all:ui
var uiAssets embed.FS

var (
	Version   = ""
	BuildDate = ""
)

func versionWithUIChecksum() string {
	if uiChecksum == "" {
		return Version
	}
	if Version == "" {
		return "ui " + uiChecksum
	}
	return Version + " ui " + uiChecksum
}

func main() {
	serveAddr := flag.String("serve", "", "run as a standalone gRPC server on this address (e.g. :9000) instead of as a go-plugin subprocess")
	tlsCert := flag.String("serve-tls-cert", "", "TLS certificate file for --serve (enables TLS with --serve-tls-key)")
	tlsKey := flag.String("serve-tls-key", "", "TLS private key file for --serve")
	tlsClientCA := flag.String("serve-tls-client-ca", "", "PEM CA bundle to require and verify the host's client certificate (enables mTLS)")
	flag.Parse()

	sub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		panic(err)
	}

	plugin := newPlugin()
	if *serveAddr != "" {
		if (*tlsCert == "") != (*tlsKey == "") {
			fmt.Fprintln(os.Stderr, "opensearch: --serve-tls-cert and --serve-tls-key must be set together")
			os.Exit(1)
		}
		if *tlsClientCA != "" && *tlsCert == "" {
			fmt.Fprintln(os.Stderr, "opensearch: --serve-tls-client-ca requires --serve-tls-cert and --serve-tls-key")
			os.Exit(1)
		}
		opts := []sdk.Option{sdk.WithStaticAssets(sub)}
		if *tlsCert != "" || *tlsKey != "" {
			opts = append(opts, sdk.WithServerTLS(*tlsCert, *tlsKey))
		}
		if *tlsClientCA != "" {
			opts = append(opts, sdk.WithServerClientCA(*tlsClientCA))
		}
		if err := sdk.ServeGRPC(plugin, *serveAddr, opts...); err != nil {
			fmt.Fprintf(os.Stderr, "opensearch: serve grpc: %v\n", err)
			os.Exit(1)
		}
		return
	}

	sdk.Serve(plugin, sdk.WithStaticAssets(sub))
}

type OpenSearchPlugin struct {
	settings PluginSettings
}

type PluginSettings struct {
	DefaultProfile string
	Index          string
	Profiles       map[string]TracingProfile
}

func defaultSettings() PluginSettings {
	return PluginSettings{
		DefaultProfile: "jaeger",
		Profiles:       map[string]TracingProfile{},
	}
}

func newPlugin() *OpenSearchPlugin {
	return &OpenSearchPlugin{settings: defaultSettings()}
}

func (p *OpenSearchPlugin) Manifest() *pluginpb.PluginManifest {
	return &pluginpb.PluginManifest{
		Name:         pluginName,
		Version:      sdk.FormatVersion(Version, BuildDate, uiChecksum),
		Description:  "Query OpenSearch trace indexes through profile-driven filters and table views.",
		Capabilities: []string{"tabs", "operations"},
		Operations:   operationDefs(),
		Tabs: []*pluginpb.TabSpec{
			{
				Name:  "OpenSearch",
				Icon:  "lucide:search",
				Path:  "/",
				Scope: "config",
			},
		},
	}
}

func (p *OpenSearchPlugin) Configure(_ context.Context, settings map[string]any) error {
	next := p.settings
	if next.DefaultProfile == "" {
		next = defaultSettings()
	}
	if v, ok := settings["defaultProfile"].(string); ok && v != "" {
		next.DefaultProfile = v
	}
	if v, ok := settings["index"].(string); ok && v != "" {
		next.Index = v
	}
	if v, ok := settings["profilesYaml"].(string); ok && v != "" {
		profiles, err := parseProfilesYAML([]byte(v))
		if err != nil {
			return err
		}
		next.Profiles = profiles
	}
	p.settings = next
	return nil
}

func (p *OpenSearchPlugin) Operations() []sdk.Operation {
	handlers := map[string]func(context.Context, sdk.InvokeCtx) (any, error){
		OpProfilesList:    p.profilesList,
		OpTraceQuery:      p.traceQuery,
		OpQueryPreview:    p.queryPreview,
		OpConnectionCheck: p.connectionCheck,
	}
	defs := operationDefs()
	out := make([]sdk.Operation, 0, len(defs))
	for _, d := range defs {
		if h, ok := handlers[d.Name]; ok {
			out = append(out, sdk.Operation{Def: d, Handler: h, HTTPHandler: p.httpInvoke(d.Name, h)})
		}
	}
	return out
}

func operationDefs() []*pluginpb.OperationDef {
	mk := func(name, desc string) *pluginpb.OperationDef {
		return &pluginpb.OperationDef{
			Name:        name,
			Description: desc,
			Scope:       "config",
			ResultMime:  sdk.ClickyResultMimeType,
			Http:        []*pluginpb.HTTPBinding{{Method: http.MethodPost}},
		}
	}
	return []*pluginpb.OperationDef{
		mk(OpProfilesList, "List resolved OpenSearch tracing profiles."),
		mk(OpTraceQuery, "Run a profile-based OpenSearch trace query."),
		mk(OpQueryPreview, "Render the OpenSearch query body without executing it."),
		mk(OpConnectionCheck, "Verify the resolved OpenSearch connection responds."),
	}
}
