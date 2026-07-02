// Arthas plugin: JVM diagnostics for Kubernetes workloads surfaced as a
// Mission Control plugin.
//
// Build: task build:plugin:arthas
// Apply: kubectl apply -f plugins/arthas/Plugin.yaml
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
	"github.com/flanksource/mission-control-plugins/arthas/internal/arthas"
)

const (
	OpSessionsList          = "sessions-list"
	OpSessionCreate         = "session-create"
	OpSessionCreationStatus = "session-creation-status"
	OpSessionDelete         = "session-delete"
	OpPodsList              = "pods-list"
	OpExec                  = "exec"
)

//go:generate go run ./internal/gen-checksum

//go:embed ui/*
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
			fmt.Fprintln(os.Stderr, "arthas: --serve-tls-cert and --serve-tls-key must be set together")
			os.Exit(1)
		}
		if *tlsClientCA != "" && *tlsCert == "" {
			fmt.Fprintln(os.Stderr, "arthas: --serve-tls-client-ca requires --serve-tls-cert and --serve-tls-key")
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
			fmt.Fprintf(os.Stderr, "arthas: serve grpc: %v\n", err)
			os.Exit(1)
		}
		return
	}

	sdk.Serve(plugin, sdk.WithStaticAssets(sub))
}

type ArthasPlugin struct {
	clients        clientCache
	sessions       *arthas.SessionRegistry
	sessionCreates *SessionCreateJobRegistry
}

func newPlugin() *ArthasPlugin {
	return &ArthasPlugin{sessions: arthas.NewSessionRegistry(), sessionCreates: NewSessionCreateJobRegistry()}
}

func (p *ArthasPlugin) Manifest() *pluginpb.PluginManifest {
	return &pluginpb.PluginManifest{
		Name:         "arthas",
		Version:      sdk.FormatVersion(Version, BuildDate, uiChecksum),
		Description:  "Attach Arthas to JVMs running in Kubernetes workloads and inspect threads, memory, MBeans, logging, and the Arthas web console.",
		Capabilities: []string{"tabs", "operations"},
		Tabs: []*pluginpb.TabSpec{
			{Name: "Arthas", Icon: "lucide:bug", Path: "/", Scope: "config"},
		},
		Operations: operationDefs(),
	}
}

func (p *ArthasPlugin) Configure(_ context.Context, _ map[string]any) error {
	return nil
}

func (p *ArthasPlugin) Operations() []sdk.Operation {
	defs := operationDefs()
	handlers := map[string]func(context.Context, sdk.InvokeCtx) (any, error){
		OpSessionsList:  p.sessionsList,
		OpSessionCreate: p.sessionCreate,
		OpSessionCreationStatus: func(_ context.Context, req sdk.InvokeCtx) (any, error) {
			return p.sessionCreationStatus(req)
		},
		OpSessionDelete: p.sessionDelete,
		OpPodsList:      p.podsList,
		OpExec:          p.exec,
	}
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
		mk(OpSessionsList, "List active Arthas sessions in this plugin process."),
		mk(OpSessionCreate, "Start attaching Arthas to the selected Kubernetes workload or pod asynchronously."),
		mk(OpSessionCreationStatus, "Get the status of an asynchronous Arthas session creation."),
		mk(OpSessionDelete, "Stop and remove an Arthas session."),
		mk(OpPodsList, "List ready target pods for the selected Kubernetes workload."),
		mk(OpExec, "Execute one Arthas command through the session HTTP API."),
	}
}
