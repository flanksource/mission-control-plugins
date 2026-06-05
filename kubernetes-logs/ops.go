package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

type logsParams struct {
	ConfigID  string `json:"config_id"`
	Container string `json:"container,omitempty"`
	TailLines int64  `json:"tailLines,omitempty"`
}

// logs is the unary gRPC operation handler. Streaming follow mode remains HTTP
// only because the current plugin SDK exposes operation Invoke as unary gRPC.
func (p *KubernetesLogsPlugin) logs(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params logsParams
	if len(strings.TrimSpace(string(req.ParamsJSON))) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}

	configID := params.ConfigID
	if configID == "" {
		configID = req.ConfigItemID
	}
	if configID == "" {
		return nil, fmt.Errorf("config_id required")
	}

	tailLines := params.TailLines
	if tailLines <= 0 {
		tailLines = 200
	}

	cli, err := p.clients.For(ctx, req.Host, configID)
	if err != nil {
		return nil, err
	}
	pods, err := resolvePods(ctx, cli, req.Host, configID)
	if err != nil {
		return nil, err
	}

	out := make([]sseLogLine, 0)
	for _, pod := range pods {
		containers := containerNames(pod, params.Container)
		lines, err := fetchHTTPLogs(ctx, cli, pod.Namespace, pod.Name, containers, tailLines)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return out, nil
}
