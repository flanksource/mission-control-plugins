// S3 plugin: attaches to MissionControl::Connection catalog items for S3
// connections. It resolves the selected connection through
// HostClient.GetConnectionByID.
package main

import (
	"context"
	"fmt"

	pluginpb "github.com/flanksource/incident-commander/plugin/proto"
	"github.com/flanksource/incident-commander/plugin/sdk"
)

const OpInspect = "inspect"

var (
	Version   = ""
	BuildDate = ""
)

func main() {
	sdk.Serve(newPlugin())
}

type S3Plugin struct{}

func newPlugin() *S3Plugin {
	return &S3Plugin{}
}

func (p *S3Plugin) Manifest() *pluginpb.PluginManifest {
	return &pluginpb.PluginManifest{
		Name:         "s3",
		Version:      sdk.FormatVersion(Version, BuildDate, ""),
		Description:  "Inspect S3 connection configuration and bucket contents.",
		Capabilities: []string{"operations"},
	}
}

func (p *S3Plugin) Configure(_ context.Context, _ map[string]any) error {
	return nil
}

func (p *S3Plugin) Operations() []sdk.Operation {
	return []sdk.Operation{
		{
			Def: &pluginpb.OperationDef{
				Name:        OpInspect,
				Description: "Inspect the selected S3 MissionControl::Connection.",
				Scope:       "config",
				ResultMime:  sdk.ClickyResultMimeType,
			},
			Handler: p.inspect,
		},
	}
}

func (p *S3Plugin) inspect(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	if req.ConfigItemID == "" {
		return nil, fmt.Errorf("config_item_id is required")
	}
	if req.Host == nil {
		return nil, fmt.Errorf("host client is required")
	}

	conn, err := req.Host.GetConnectionByID(ctx, req.ConfigItemID)
	if err != nil {
		return nil, fmt.Errorf("get connection %s: %w", req.ConfigItemID, err)
	}
	if conn == nil {
		return nil, fmt.Errorf("connection %s not found", req.ConfigItemID)
	}
	if conn.Type != "s3" {
		return nil, fmt.Errorf("s3 plugin requires an s3 connection, got %q", conn.Type)
	}

	props := map[string]any(nil)
	if conn.Properties != nil {
		props = conn.Properties.AsMap()
	}
	return map[string]any{
		"type":           conn.Type,
		"url":            conn.Url,
		"usernameSet":    conn.Username != "",
		"passwordSet":    conn.Password != "",
		"certificateSet": conn.Certificate != "",
		"tokenSet":       conn.Token != "",
		"properties":     props,
	}, nil
}
