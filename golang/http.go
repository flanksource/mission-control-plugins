package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

func (p *GolangPlugin) httpInvoke(operation string, handler func(context.Context, sdk.InvokeCtx) (any, error)) http.Handler {
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

func (p *GolangPlugin) httpProfile(w http.ResponseWriter, r *http.Request) {
	rest := operationSubpath(r, OpHTTPProfiles)
	id, tail, _ := strings.Cut(rest, "/")
	if id == "" || tail == "" {
		http.Error(w, "expected /profiles/{sessionID}/{runID|heap|cpu|trace}", http.StatusBadRequest)
		return
	}
	sess, ok := p.sessions.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !sessionMatchesConfig(sess, configItemIDFromRequest(r)) {
		http.Error(w, "session does not belong to the current config item", http.StatusForbidden)
		return
	}

	runIDOrKind, subPath, _ := strings.Cut(tail, "/")
	if run, ok := p.profiles.Get(runIDOrKind); ok {
		if run.SessionID != sess.ID {
			http.Error(w, "profile run does not belong to session", http.StatusForbidden)
			return
		}
		snapshot := run.Snapshot()
		if snapshot.State != "completed" {
			http.Error(w, "profile run is not completed", http.StatusConflict)
			return
		}
		if subPath != "" {
			if p.serveStaticProfileView(w, r, run, subPath) {
				return
			}
			p.proxyProfileViewer(w, r, sess, run, subPath)
			return
		}
		data := run.Data()
		if len(data) == 0 {
			http.Error(w, "profile run has no data", http.StatusNotFound)
			return
		}
		writeProfileDownload(w, sess.ID, run.ID, snapshot.Kind, snapshot.Source, data)
		return
	}

	kind := normalizeProfileKind(runIDOrKind)
	if kind == "" {
		http.Error(w, "expected /profiles/{sessionID}/{runID|heap|cpu|trace}", http.StatusBadRequest)
		return
	}
	data, source, err := collectProfile(r.Context(), sess, kind, p.settings.MaxProfileSec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeProfileDownload(w, sess.ID, kind, kind, source, data)
}

func (p *GolangPlugin) serveStaticProfileView(w http.ResponseWriter, r *http.Request, run *ProfileRun, subPath string) bool {
	view := strings.Trim(strings.TrimPrefix(subPath, "ui/"), "/")
	sampleIndex := r.URL.Query().Get("si")
	if sampleIndex == "" {
		sampleIndex = r.URL.Query().Get("sample_index")
	}
	switch view {
	case "flamegraph", "graph":
		out, err := renderPprofCLI(r.Context(), run, "svg", sampleIndex)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write(out)
		return true
	case "top":
		out, err := renderPprofCLI(r.Context(), run, "top", sampleIndex)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(out)
		return true
	default:
		return false
	}
}

func renderPprofCLI(ctx context.Context, run *ProfileRun, format, sampleIndex string) ([]byte, error) {
	snap := run.Snapshot()
	if snap.Kind == "trace" {
		return nil, fmt.Errorf("trace profiles cannot be rendered with go tool pprof")
	}
	data := run.Data()
	if len(data) == 0 {
		return nil, fmt.Errorf("profile run has no data")
	}
	tmp, err := os.CreateTemp("", "golang-profile-*."+profileExtension(snap.Kind))
	if err != nil {
		return nil, fmt.Errorf("create temp profile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("write temp profile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp profile: %w", err)
	}

	args := []string{"tool", "pprof", "-" + format}
	if sampleIndex != "" {
		args = append(args, "-sample_index="+sampleIndex)
	}
	args = append(args, tmpPath)
	cmd := exec.CommandContext(ctx, "go", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("go tool pprof -%s: %w: %s", format, err, msg)
		}
		return nil, fmt.Errorf("go tool pprof -%s: %w", format, err)
	}
	return stdout.Bytes(), nil
}

func (p *GolangPlugin) proxyProfileViewer(w http.ResponseWriter, r *http.Request, _ *Session, run *ProfileRun, subPath string) {
	if p.viewers == nil {
		http.Error(w, "profile viewer registry is not initialised", http.StatusInternalServerError)
		return
	}
	addr, err := p.viewers.Get(r.Context(), run)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	target, _ := url.Parse("http://" + addr)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorLog = log.New(os.Stderr, "[WARN] golang profile viewer: ", 0)
	r.URL.Path = "/" + strings.TrimLeft(subPath, "/")
	r.URL.RawPath = ""
	proxy.ServeHTTP(w, r)
}

func configItemIDFromRequest(r *http.Request) string {
	if id := sdk.ConfigItemIDFromContext(r.Context()); id != "" {
		return id
	}
	return r.URL.Query().Get("config_id")
}

func operationSubpath(r *http.Request, operation string) string {
	if p := strings.TrimLeft(r.URL.Query().Get("path"), "/"); p != "" {
		return p
	}
	return strings.Trim(strings.TrimPrefix(r.URL.Path, "/__mc/operations/"+operation), "/")
}

func writeProfileDownload(w http.ResponseWriter, sessionID, name, kind, source string, data []byte) {
	filename := fmt.Sprintf("%s-%s-%s.%s", pluginName, sessionID, name, profileExtension(kind))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.Header().Set("X-Diagnostics-Source", source)
	w.Header().Set("Content-Type", profileContentType(kind))
	_, _ = w.Write(data)
}

func profileExtension(kind string) string {
	if kind == "trace" {
		return "trace"
	}
	return "pprof"
}

func profileContentType(kind string) string {
	if kind == "trace" {
		return "application/octet-stream"
	}
	return "application/vnd.google.protobuf"
}
