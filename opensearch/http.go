package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

func (p *OpenSearchPlugin) httpInvoke(operation string, handler func(context.Context, sdk.InvokeCtx) (any, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		params, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(strings.TrimSpace(string(params))) == 0 {
			params = []byte("{}")
		}
		res, err := handler(r.Context(), sdk.InvokeCtx{
			Operation:    operation,
			ParamsJSON:   params,
			ConfigItemID: configItemIDFromRequest(r),
			Host:         sdk.HostClientFromContext(r.Context()),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func (p *OpenSearchPlugin) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name":        pluginName,
			"version":     Version,
			"buildDate":   BuildDate,
			"uiChecksum":  uiChecksum,
			"fullVersion": versionWithUIChecksum(),
		})
	})
	return mux
}

func configItemIDFromRequest(r *http.Request) string {
	if id := sdk.ConfigItemIDFromContext(r.Context()); id != "" {
		return id
	}
	return r.URL.Query().Get("config_id")
}
