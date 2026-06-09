package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

// HTTPHandler powers the iframe's streaming endpoints. The unary
// operations (stats/query/explain/trace-*) are handled by the host's
// /api/plugins/<name>/operations/<op> route which has HostClient access;
// SSE has to live in the plugin's HTTP server because operations are
// unary RPCs.
//
// /trace-stream/<id> tails an existing trace's event buffer.
func (p *SQLServerPlugin) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/trace-stream/", p.httpTraceStream)
	mux.Handle("/version", sdk.VersionHandler(sdk.BuildInfo{
		Name:       "sql-server",
		Version:    Version,
		BuildDate:  BuildDate,
		UIChecksum: uiChecksum,
	}))
	return mux
}

func (p *SQLServerPlugin) httpTraceStream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/trace-stream/")
	}
	if id == "" || id == r.URL.Path {
		http.Error(w, "trace id required", http.StatusBadRequest)
		return
	}
	configID := r.URL.Query().Get("config_id")
	if configID == "" {
		configID = sdk.ConfigItemIDFromContext(r.Context())
	}
	trace, err := p.traces.GetForConfig(id, configID)
	if err != nil {
		http.Error(w, fmt.Sprintf("trace %q not found", id), http.StatusNotFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Content-Encoding", "identity")
	w.Header().Set("X-Accel-Buffering", "no")

	writeComment := func(comment string) bool {
		if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	writeDone := func() bool {
		if _, err := fmt.Fprintf(w, "event: done\ndata: {}\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Tail loop: drain once immediately, then ship new events as SSE frames on
	// each tick. Heartbeats keep the EventSource/proxy connection established
	// even when SQL Server has no matching events.
	since := r.URL.Query().Get("since")
	drain := func() bool {
		events := trace.EventsSince(since)
		for _, e := range events {
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return false
			}
			flusher.Flush()
			since = e.Key()
		}
		if !trace.Running() {
			_ = writeDone()
			return false
		}
		return true
	}

	if !writeComment("connected") || !drain() {
		return
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if !writeComment("heartbeat") {
				return
			}
		case <-tick.C:
			if !drain() {
				return
			}
		}
	}
}
