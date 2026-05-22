package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

func (p *InspektorGadgetPlugin) httpInvoke(operation string, handler func(context.Context, sdk.InvokeCtx) (any, error)) http.Handler {
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

func (p *InspektorGadgetPlugin) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sessions/", p.httpSession)
	mux.Handle("/version", sdk.VersionHandler(sdk.BuildInfo{
		Name:       pluginName,
		Version:    Version,
		BuildDate:  BuildDate,
		UIChecksum: uiChecksum,
	}))
	return mux
}

func (p *InspektorGadgetPlugin) httpSession(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/sessions/")
	id, tail, _ := strings.Cut(rest, "/")
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}
	sess, err := p.sessionForConfig(r.Context(), sdk.InvokeCtx{
		ConfigItemID: configItemIDFromRequest(r),
		Host:         sdk.HostClientFromContext(r.Context()),
	}, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	switch tail {
	case "events":
		streamSessionEvents(w, r, sess)
	case "export":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".ndjson"))
		for _, event := range sess.Events() {
			b, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(w, "%s\n", b); err != nil {
				return
			}
		}
	default:
		http.NotFound(w, r)
	}
}

func configItemIDFromRequest(r *http.Request) string {
	if id := sdk.ConfigItemIDFromContext(r.Context()); id != "" {
		return id
	}
	return r.URL.Query().Get("config_id")
}

func streamSessionEvents(w http.ResponseWriter, r *http.Request, sess *TraceSession) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for _, event := range sess.Events() {
		writeSSEJSON(w, flusher, event)
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-sess.events:
			if !ok {
				writeSSE(w, flusher, "done", "{}")
				return
			}
			writeSSEJSON(w, flusher, event)
		}
	}
}

func writeSSE(w http.ResponseWriter, f http.Flusher, event, data string) {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return
	}
	f.Flush()
}

func writeSSEJSON(w http.ResponseWriter, f http.Flusher, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return
	}
	f.Flush()
}
