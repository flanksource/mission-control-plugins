package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

func (p *ArthasPlugin) httpInvoke(operation string, handler func(context.Context, sdk.InvokeCtx) (any, error)) http.Handler {
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
			ConfigItemID: sdk.ConfigItemIDFromContext(r.Context()),
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
