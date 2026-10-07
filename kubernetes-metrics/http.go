package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

func (p *KubernetesMetricsPlugin) httpOperation(operation string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := sdk.InvokeCtx{
			Operation: operation, ConfigItemID: sdk.ConfigItemIDFromContext(r.Context()),
			Host: sdk.HostClientFromContext(r.Context()),
		}
		var result any
		var err error
		if operation == "history" {
			if r.Method == http.MethodGet {
				req.ParamsJSON, err = json.Marshal(historyParams{Range: r.URL.Query().Get("range"), Step: r.URL.Query().Get("step")})
			} else {
				req.ParamsJSON, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
				if err != nil {
					err = invalid("cannot read history params: %v", err)
				}
			}
			if err == nil {
				result, err = p.history(r.Context(), req)
			}
		} else {
			result, err = p.current(r.Context(), req)
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			status, code := http.StatusBadGateway, "HANDLER_ERROR"
			var validation *invalidError
			var lookup *lookupError
			switch {
			case errors.As(err, &validation):
				status, code = http.StatusBadRequest, "EINVALID"
			case errors.As(err, &lookup):
				status, code = lookup.status, lookup.code
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error_code": code, "error_message": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}
