package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

type TraceQueryRequest struct {
	Profile string         `json:"profile"`
	From    string         `json:"from,omitempty"`
	To      string         `json:"to,omitempty"`
	Limit   int            `json:"limit,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
}

type ConnectionCheckResult struct {
	OK      bool     `json:"ok"`
	URLs    []string `json:"urls"`
	Index   string   `json:"index,omitempty"`
	Message string   `json:"message,omitempty"`
}

func (p *OpenSearchPlugin) profilesList(_ context.Context, _ sdk.InvokeCtx) (any, error) {
	return p.settings.profileSummaries()
}

func (p *OpenSearchPlugin) queryPreview(_ context.Context, req sdk.InvokeCtx) (any, error) {
	params, profile, err := p.decodeQuery(req.ParamsJSON)
	if err != nil {
		return nil, err
	}
	return buildQueryPreview(profile, params)
}

func (p *OpenSearchPlugin) traceQuery(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	params, profile, err := p.decodeQuery(req.ParamsJSON)
	if err != nil {
		return nil, err
	}
	preview, err := buildQueryPreview(profile, params)
	if err != nil {
		return nil, err
	}
	conn, err := resolveOpenSearch(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	if profile.Index == "" {
		profile.Index = conn.Index
	}
	if profile.Index == "" {
		return nil, fmt.Errorf("no OpenSearch index configured for profile %q", profile.Name)
	}
	client := newOpenSearchClient(conn)
	raw, err := client.Search(ctx, profile.Index, preview.Request.Limit, preview.Request.Query)
	if err != nil {
		return nil, err
	}
	return parseSearchResult(raw, profile), nil
}

func (p *OpenSearchPlugin) connectionCheck(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	conn, err := resolveOpenSearch(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	err = newOpenSearchClient(conn).Check(ctx)
	result := ConnectionCheckResult{
		OK:    err == nil,
		URLs:  conn.URLs,
		Index: conn.Index,
	}
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	result.Message = "connected"
	return result, nil
}

func (p *OpenSearchPlugin) decodeQuery(data []byte) (TraceQueryRequest, TracingProfile, error) {
	var req TraceQueryRequest
	if len(data) > 0 {
		if err := json.Unmarshal(data, &req); err != nil {
			return TraceQueryRequest{}, TracingProfile{}, fmt.Errorf("decode params: %w", err)
		}
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	profile, err := p.settings.resolveProfile(req.Profile)
	if err != nil {
		return TraceQueryRequest{}, TracingProfile{}, err
	}
	if req.Profile == "" {
		req.Profile = profile.Name
	}
	return req, profile, nil
}
