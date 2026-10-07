package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/flanksource/incident-commander/plugin/sdk"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/config"
)

func prometheusClient(ctx context.Context, host sdk.HostClient) (v1.API, func(), error) {
	conn, err := host.GetConnectionByType(ctx, sdk.ConnectionType("prometheus"))
	if err != nil {
		return nil, nil, fmt.Errorf("resolve spec.connections.types.prometheus: %w", err)
	}
	if conn == nil || conn.Url == "" {
		return nil, nil, fmt.Errorf("spec.connections.types.prometheus: readable Prometheus connection with URL is required")
	}
	if conn.Type != "prometheus" {
		return nil, nil, fmt.Errorf("spec.connections.types.prometheus: expected prometheus connection, got %q", conn.Type)
	}
	address, err := url.Parse(conn.Url)
	if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Host == "" {
		return nil, nil, fmt.Errorf("spec.connections.types.prometheus: invalid HTTP(S) URL")
	}
	properties := conn.GetProperties().GetFields()
	property := func(key string) string { return properties[key].GetStringValue() }
	cfg := config.DefaultHTTPClientConfig
	cfg.TLSConfig = config.TLSConfig{
		CA: property("ca"), Cert: property("cert"), Key: config.Secret(property("key")),
		InsecureSkipVerify: properties["insecureTLS"].GetBoolValue(),
	}
	switch {
	case conn.Username != "" || conn.Password != "":
		cfg.BasicAuth = &config.BasicAuth{Username: conn.Username, Password: config.Secret(conn.Password)}
	case property("bearer") != "":
		cfg.Authorization = &config.Authorization{Type: "Bearer", Credentials: config.Secret(property("bearer"))}
	case property("clientID") != "" || property("clientSecret") != "" || property("tokenURL") != "":
		if property("clientID") == "" || property("clientSecret") == "" || property("tokenURL") == "" {
			return nil, nil, fmt.Errorf("spec.connections.types.prometheus: OAuth requires clientID, clientSecret and tokenURL")
		}
		oauth := &config.OAuth2{ClientID: property("clientID"), ClientSecret: config.Secret(property("clientSecret")), TokenURL: property("tokenURL"), TLSConfig: cfg.TLSConfig}
		if scopes := property("scopes"); scopes != "" {
			oauth.Scopes = strings.Split(scopes, ",")
		}
		if params := property("params"); params != "" {
			if err := json.Unmarshal([]byte(params), &oauth.EndpointParams); err != nil {
				return nil, nil, fmt.Errorf("spec.connections.types.prometheus: invalid OAuth params: %w", err)
			}
		}
		cfg.OAuth2 = oauth
	}
	client, err := config.NewClientFromConfig(cfg, "kubernetes-metrics")
	if err != nil {
		return nil, nil, fmt.Errorf("spec.connections.types.prometheus: %w", err)
	}
	client.Timeout = 30 * time.Second
	prom, err := api.NewClient(api.Config{Address: conn.Url, Client: client})
	if err != nil {
		client.CloseIdleConnections()
		return nil, nil, fmt.Errorf("prometheus client: %w", err)
	}
	return v1.NewAPI(prom), client.CloseIdleConnections, nil
}
