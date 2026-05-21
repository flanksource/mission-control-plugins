// OpenSearch plugin: generic profile-based trace querying for Mission Control.
//
// Build: task -d plugins/opensearch build
// Apply: kubectl apply -f plugins/opensearch/Plugin.yaml
package main

import (
	"context"
	"embed"
	"io/fs"

	pluginpb "github.com/flanksource/incident-commander/plugin/proto"
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

func main() {
	sub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		panic(err)
	}
	sdk.Serve(newPlugin(), sdk.WithStaticAssets(sub))
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
		Tabs: []*pluginpb.TabSpec{
			{Name: "OpenSearch", Icon: "lucide:search", Path: "/", Scope: "config"},
		},
		Operations: operationDefs(),
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
			out = append(out, sdk.Operation{Def: d, Handler: h})
		}
	}
	return out
}

func operationDefs() []*pluginpb.OperationDef {
	mk := func(name, desc string) *pluginpb.OperationDef {
		return &pluginpb.OperationDef{Name: name, Description: desc, Scope: "config", ResultMime: sdk.ClickyResultMimeType}
	}
	return []*pluginpb.OperationDef{
		mk(OpProfilesList, "List resolved OpenSearch tracing profiles."),
		mk(OpTraceQuery, "Run a profile-based OpenSearch trace query."),
		mk(OpQueryPreview, "Render the OpenSearch query body without executing it."),
		mk(OpConnectionCheck, "Verify the resolved OpenSearch connection responds."),
	}
}
