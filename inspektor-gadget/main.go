// Inspektor Gadget plugin: eBPF workload diagnostics for Kubernetes resources
// through the Mission Control plugin runtime.
//
// Build: task -d plugins/inspektor-gadget build
// Apply: kubectl apply -f plugins/inspektor-gadget/Plugin.yaml
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
	OpStatus       = "status"
	OpTracesList   = "traces-list"
	OpTraceStart   = "trace-start"
	OpTraceStop    = "trace-stop"
	OpTraceList    = "trace-list"
	OpTraceEvents  = "trace-events"
	pluginName     = "inspektor-gadget"
	defaultIGTag   = "v0.53.0"
	defaultMaxRuns = 5
)

//go:generate go run ./internal/gen-checksum

//go:embed all:ui
var uiAssets embed.FS

var (
	Version   = ""
	BuildDate = ""
)

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
			fmt.Fprintln(os.Stderr, "inspektor-gadget: --serve-tls-cert and --serve-tls-key must be set together")
			os.Exit(1)
		}
		if *tlsClientCA != "" && *tlsCert == "" {
			fmt.Fprintln(os.Stderr, "inspektor-gadget: --serve-tls-client-ca requires --serve-tls-cert and --serve-tls-key")
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
			fmt.Fprintf(os.Stderr, "inspektor-gadget: serve grpc: %v\n", err)
			os.Exit(1)
		}
		return
	}

	sdk.Serve(plugin, sdk.WithStaticAssets(sub))
}

type InspektorGadgetPlugin struct {
	clients  clientCache
	sessions *SessionRegistry
	runner   TraceRunner
	settings PluginSettings
}

type PluginSettings struct {
	GadgetNamespace string
	MaxDurationSec  int
	MaxEvents       int
	MaxSessions     int
}

func defaultSettings() PluginSettings {
	return PluginSettings{
		GadgetNamespace: "gadget",
		MaxDurationSec:  900,
		MaxEvents:       10000,
		MaxSessions:     defaultMaxRuns,
	}
}

func newPlugin() *InspektorGadgetPlugin {
	settings := defaultSettings()
	return &InspektorGadgetPlugin{
		sessions: NewSessionRegistry(settings.MaxEvents),
		runner:   NewTraceRunner(),
		settings: settings,
	}
}

func (p *InspektorGadgetPlugin) Manifest() *pluginpb.PluginManifest {
	return &pluginpb.PluginManifest{
		Name:         pluginName,
		Version:      sdk.FormatVersion(Version, BuildDate, uiChecksum),
		Description:  "Run Inspektor Gadget eBPF traces against Kubernetes workloads through the configured Kubernetes API connection.",
		Capabilities: []string{"tabs", "operations"},
		Tabs: []*pluginpb.TabSpec{
			{Name: "Gadget", Icon: "lucide:radar", Path: "/", Scope: "config"},
		},
		Operations: operationDefs(),
	}
}

func (p *InspektorGadgetPlugin) Configure(_ context.Context, settings map[string]any) error {
	next := p.settings
	if v, ok := settings["gadgetNamespace"].(string); ok && v != "" {
		next.GadgetNamespace = v
	}
	if v, ok := numberSetting(settings, "maxDurationSec"); ok && v > 0 {
		next.MaxDurationSec = v
	}
	if v, ok := numberSetting(settings, "maxEvents"); ok && v > 0 {
		next.MaxEvents = v
	}
	if v, ok := numberSetting(settings, "maxSessions"); ok && v > 0 {
		next.MaxSessions = v
	}
	p.settings = next
	p.sessions.SetMaxEvents(next.MaxEvents)
	return nil
}

func (p *InspektorGadgetPlugin) Operations() []sdk.Operation {
	handlers := map[string]func(context.Context, sdk.InvokeCtx) (any, error){
		OpStatus:      p.status,
		OpTracesList:  p.tracesList,
		OpTraceStart:  p.traceStart,
		OpTraceStop:   p.traceStop,
		OpTraceList:   p.traceList,
		OpTraceEvents: p.traceEvents,
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
		mk(OpStatus, "Check Inspektor Gadget deployment readiness through the Kubernetes API."),
		mk(OpTracesList, "List supported Inspektor Gadget traces."),
		mk(OpTraceStart, "Start a bounded Inspektor Gadget trace session for this resource."),
		mk(OpTraceStop, "Stop a running trace session."),
		mk(OpTraceList, "List active and recent trace sessions."),
		mk(OpTraceEvents, "Return buffered events for a trace session."),
	}
}

func numberSetting(settings map[string]any, key string) (int, bool) {
	switch v := settings[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
