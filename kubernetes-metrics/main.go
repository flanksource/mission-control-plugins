// Kubernetes metrics exposes workload totals through Mission Control operations.
// Catalog and connection access stay host-authorized; pod selection stays in PromQL.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	Version   = ""
	BuildDate = ""
)

func main() {
	addr := flag.String("serve", "", "run as a standalone gRPC server instead of a go-plugin subprocess")
	tlsCert := flag.String("serve-tls-cert", "", "TLS certificate file for --serve")
	tlsKey := flag.String("serve-tls-key", "", "TLS private key file for --serve")
	clientCA := flag.String("serve-tls-client-ca", "", "PEM CA bundle for verifying the host's client certificate")
	flag.Parse()

	if (*tlsCert == "") != (*tlsKey == "") || (*clientCA != "" && *tlsCert == "") {
		fmt.Fprintln(os.Stderr, "kubernetes-metrics: TLS requires both --serve-tls-cert and --serve-tls-key; client CA also requires TLS")
		os.Exit(1)
	}
	p := &KubernetesMetricsPlugin{}
	if *addr == "" {
		sdk.Serve(p)
		return
	}
	var opts []sdk.Option
	if *tlsCert != "" {
		opts = append(opts, sdk.WithServerTLS(*tlsCert, *tlsKey))
	}
	if *clientCA != "" {
		opts = append(opts, sdk.WithServerClientCA(*clientCA))
	}
	if err := sdk.ServeGRPC(p, *addr, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "kubernetes-metrics: serve grpc: %v\n", err)
		os.Exit(1)
	}
}

type KubernetesMetricsPlugin struct{}

func (p *KubernetesMetricsPlugin) Manifest() *pluginpb.PluginManifest {
	var defs []*pluginpb.OperationDef
	for _, op := range p.Operations() {
		defs = append(defs, op.Def)
	}
	return &pluginpb.PluginManifest{
		Name: "kubernetes-metrics", Version: sdk.FormatVersion(Version, BuildDate, ""),
		Description:  "Workload CPU and memory totals from Prometheus.",
		Capabilities: []string{"operations"}, Operations: defs,
	}
}

func (*KubernetesMetricsPlugin) Configure(context.Context, map[string]any) error { return nil }
func (*KubernetesMetricsPlugin) HTTPHandler() http.Handler                       { return http.NotFoundHandler() }

func (p *KubernetesMetricsPlugin) Operations() []sdk.Operation {
	schema, err := structpb.NewStruct(map[string]any{
		"type": "object", "required": []any{"range"},
		"properties": map[string]any{
			"range": map[string]any{"type": "string", "enum": []any{"15m", "1h", "6h", "24h"}},
			"step":  map[string]any{"type": "string", "default": "15s"},
		},
	})
	if err != nil {
		panic(err)
	}
	return []sdk.Operation{
		{
			Def: &pluginpb.OperationDef{
				Name: "current", Description: "Current workload CPU (cores) and memory (bytes), including requests and limits.",
				Scope: "config", ResultMime: "application/json",
				Http: []*pluginpb.HTTPBinding{{Method: http.MethodGet}, {Method: http.MethodPost}},
			},
			Handler: p.current, HTTPHandler: p.httpOperation("current"),
		},
		{
			Def: &pluginpb.OperationDef{
				Name: "history", Description: "Historical workload CPU and memory usage and limits.",
				Scope: "config", ResultMime: "application/json", ParamsSchema: schema,
				Http: []*pluginpb.HTTPBinding{{Method: http.MethodGet}, {Method: http.MethodPost}},
			},
			Handler: p.history, HTTPHandler: p.httpOperation("history"),
		},
	}
}
