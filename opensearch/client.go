package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

type resolvedOpenSearch struct {
	URLs               []string
	Username           string
	Password           string
	Index              string
	InsecureSkipVerify bool
}

type openSearchClient struct {
	conn   resolvedOpenSearch
	client *http.Client
}

func resolveOpenSearch(ctx context.Context, host sdk.HostClient, configItemID string) (resolvedOpenSearch, error) {
	if host == nil {
		return resolvedOpenSearch{}, fmt.Errorf("no host client available")
	}
	conn, err := host.GetConnectionByType(ctx, sdk.ConnectionType("opensearch"))
	if err != nil {
		return resolvedOpenSearch{}, fmt.Errorf("get opensearch connection: %w", err)
	}
	if conn == nil {
		return resolvedOpenSearch{}, fmt.Errorf("host returned no opensearch connection")
	}
	out := resolvedOpenSearch{
		Username: conn.Username,
		Password: conn.Password,
	}
	if conn.Url != "" {
		out.URLs = append(out.URLs, conn.Url)
	}
	if conn.Properties != nil {
		props := conn.Properties.AsMap()
		if raw, ok := props["urls"].(string); ok {
			for _, part := range strings.Split(raw, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					out.URLs = append(out.URLs, trimmed)
				}
			}
		}
		if values, ok := props["urls"].([]any); ok {
			for _, value := range values {
				if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
					out.URLs = append(out.URLs, strings.TrimSpace(s))
				}
			}
		}
		if index, ok := props["index"].(string); ok {
			out.Index = index
		}
		if insecure, ok := props["insecureSkipVerify"].(bool); ok {
			out.InsecureSkipVerify = insecure
		}
		if insecure, ok := props["insecure_tls"].(bool); ok {
			out.InsecureSkipVerify = insecure
		}
	}
	out.URLs = uniqueStrings(out.URLs)
	if len(out.URLs) == 0 {
		return resolvedOpenSearch{}, fmt.Errorf("opensearch connection has no urls")
	}
	return out, nil
}

func newOpenSearchClient(conn resolvedOpenSearch) *openSearchClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if conn.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &openSearchClient{
		conn: conn,
		client: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
	}
}

func (c *openSearchClient) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.conn.URLs[0], "/"), nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("opensearch returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *openSearchClient) Search(ctx context.Context, index string, limit int, query map[string]any) (map[string]any, error) {
	if limit <= 0 {
		limit = 100
	}
	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}
	base := strings.TrimRight(c.conn.URLs[0], "/")
	url := base + "/" + strings.Trim(index, "/") + "/_search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("size", fmt.Sprintf("%d", limit))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read search response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("opensearch search returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	return result, nil
}

func (c *openSearchClient) authorize(req *http.Request) {
	if c.conn.Username != "" || c.conn.Password != "" {
		req.SetBasicAuth(c.conn.Username, c.conn.Password)
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
