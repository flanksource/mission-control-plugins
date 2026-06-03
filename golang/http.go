package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/flanksource/incident-commander/plugin/sdk"
	pprofprofile "github.com/google/pprof/profile"
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
			http.Error(w, "unsupported profile view", http.StatusNotFound)
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
	case "flamegraph", "flamegraph-data":
		data, err := buildFlamegraphData(run, sampleIndex)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return true
	case "graph":
		http.Error(w, "graph SVG rendering is not available; use flamegraph-data", http.StatusNotImplemented)
		return true
	case "top":
		out, err := renderPprofTopText(run, sampleIndex)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(out))
		return true
	default:
		return false
	}
}

type FlamegraphData struct {
	SampleType  string         `json:"sampleType"`
	Unit        string         `json:"unit"`
	Total       int64          `json:"total"`
	SampleTypes []string       `json:"sampleTypes"`
	Root        FlamegraphNode `json:"root"`
}

type FlamegraphNode struct {
	Name     string           `json:"name"`
	Value    int64            `json:"value"`
	Self     int64            `json:"self,omitempty"`
	Children []FlamegraphNode `json:"children,omitempty"`
}

type flamegraphBuildNode struct {
	Name     string
	Value    int64
	Self     int64
	Children map[string]*flamegraphBuildNode
}

func buildFlamegraphData(run *ProfileRun, sampleIndex string) (*FlamegraphData, error) {
	prof, idx, err := parseRunProfile(run, sampleIndex)
	if err != nil {
		return nil, err
	}

	root := &flamegraphBuildNode{Name: "root", Children: map[string]*flamegraphBuildNode{}}
	for _, sample := range prof.Sample {
		if idx >= len(sample.Value) {
			continue
		}
		value := sample.Value[idx]
		if value <= 0 {
			continue
		}
		stack := sampleStack(sample)
		if len(stack) == 0 {
			stack = []string{"<unknown>"}
		}
		root.Value += value
		node := root
		for _, frame := range stack {
			child := node.Children[frame]
			if child == nil {
				child = &flamegraphBuildNode{Name: frame, Children: map[string]*flamegraphBuildNode{}}
				node.Children[frame] = child
			}
			child.Value += value
			node = child
		}
		node.Self += value
	}

	sampleTypes := make([]string, len(prof.SampleType))
	for i, t := range prof.SampleType {
		sampleTypes[i] = t.Type
	}
	return &FlamegraphData{
		SampleType:  prof.SampleType[idx].Type,
		Unit:        prof.SampleType[idx].Unit,
		Total:       root.Value,
		SampleTypes: sampleTypes,
		Root:        toFlamegraphNode(root),
	}, nil
}

func renderPprofTopText(run *ProfileRun, sampleIndex string) (string, error) {
	prof, idx, err := parseRunProfile(run, sampleIndex)
	if err != nil {
		return "", err
	}
	total := int64(0)
	type topValue struct{ flat, cum int64 }
	values := map[string]*topValue{}
	for _, sample := range prof.Sample {
		if idx >= len(sample.Value) {
			continue
		}
		value := sample.Value[idx]
		if value <= 0 {
			continue
		}
		total += value
		stack := sampleStack(sample)
		if len(stack) == 0 {
			stack = []string{"<unknown>"}
		}
		leaf := stack[len(stack)-1]
		entry := values[leaf]
		if entry == nil {
			entry = &topValue{}
			values[leaf] = entry
		}
		entry.flat += value
		seen := map[string]struct{}{}
		for _, frame := range stack {
			if _, ok := seen[frame]; ok {
				continue
			}
			seen[frame] = struct{}{}
			entry := values[frame]
			if entry == nil {
				entry = &topValue{}
				values[frame] = entry
			}
			entry.cum += value
		}
	}

	type row struct {
		name string
		flat int64
		cum  int64
	}
	rows := make([]row, 0, len(values))
	for name, value := range values {
		rows = append(rows, row{name: name, flat: value.flat, cum: value.cum})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].flat == rows[j].flat {
			return rows[i].cum > rows[j].cum
		}
		return rows[i].flat > rows[j].flat
	})
	if len(rows) > 80 {
		rows = rows[:80]
	}

	unit := prof.SampleType[idx].Unit
	var b strings.Builder
	fmt.Fprintf(&b, "Showing top %d nodes out of %d\n", len(rows), len(values))
	fmt.Fprintf(&b, "Sample: %s (%s), Total: %s\n\n", prof.SampleType[idx].Type, unit, formatProfileValue(total, unit))
	fmt.Fprintf(&b, "%14s %8s %14s %8s  %s\n", "flat", "flat%", "cum", "cum%", "name")
	for _, row := range rows {
		fmt.Fprintf(&b, "%14s %7.2f%% %14s %7.2f%%  %s\n",
			formatProfileValue(row.flat, unit), percent(row.flat, total),
			formatProfileValue(row.cum, unit), percent(row.cum, total), row.name)
	}
	return b.String(), nil
}

func parseRunProfile(run *ProfileRun, sampleIndex string) (*pprofprofile.Profile, int, error) {
	snap := run.Snapshot()
	if snap.Kind == "trace" {
		return nil, 0, fmt.Errorf("trace profiles cannot be rendered as pprof flamegraphs")
	}
	data := run.Data()
	if len(data) == 0 {
		return nil, 0, fmt.Errorf("profile run has no data")
	}
	prof, err := pprofprofile.ParseData(data)
	if err != nil {
		return nil, 0, fmt.Errorf("parse profile: %w", err)
	}
	if len(prof.SampleType) == 0 {
		return nil, 0, fmt.Errorf("profile has no sample types")
	}
	idx, err := prof.SampleIndexByName(sampleIndex)
	if err != nil {
		return nil, 0, err
	}
	return prof, idx, nil
}

func sampleStack(sample *pprofprofile.Sample) []string {
	stack := make([]string, 0, len(sample.Location))
	for i := len(sample.Location) - 1; i >= 0; i-- {
		loc := sample.Location[i]
		if loc == nil {
			continue
		}
		if len(loc.Line) == 0 {
			stack = append(stack, locationName(loc))
			continue
		}
		for j := len(loc.Line) - 1; j >= 0; j-- {
			line := loc.Line[j]
			if line.Function == nil {
				stack = append(stack, locationName(loc))
				continue
			}
			name := line.Function.Name
			if name == "" {
				name = line.Function.SystemName
			}
			if name == "" {
				name = locationName(loc)
			}
			stack = append(stack, name)
		}
	}
	return stack
}

func locationName(loc *pprofprofile.Location) string {
	if loc.Mapping != nil && loc.Mapping.File != "" {
		return fmt.Sprintf("%s:%#x", loc.Mapping.File, loc.Address)
	}
	if loc.Address != 0 {
		return fmt.Sprintf("%#x", loc.Address)
	}
	return "<unknown>"
}

func toFlamegraphNode(node *flamegraphBuildNode) FlamegraphNode {
	children := make([]*flamegraphBuildNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Value == children[j].Value {
			return children[i].Name < children[j].Name
		}
		return children[i].Value > children[j].Value
	})
	out := FlamegraphNode{Name: node.Name, Value: node.Value, Self: node.Self}
	for _, child := range children {
		out.Children = append(out.Children, toFlamegraphNode(child))
	}
	return out
}

func percent(value, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}

func formatProfileValue(value int64, unit string) string {
	switch unit {
	case "nanoseconds", "ns":
		v := float64(value)
		switch {
		case v >= 1e9:
			return fmt.Sprintf("%.2fs", v/1e9)
		case v >= 1e6:
			return fmt.Sprintf("%.2fms", v/1e6)
		case v >= 1e3:
			return fmt.Sprintf("%.2fµs", v/1e3)
		default:
			return fmt.Sprintf("%dns", value)
		}
	case "bytes":
		v := float64(value)
		for _, suffix := range []string{"B", "KiB", "MiB", "GiB", "TiB"} {
			if v < 1024 || suffix == "TiB" {
				return fmt.Sprintf("%.2f%s", v, suffix)
			}
			v /= 1024
		}
	}
	if unit == "" {
		return fmt.Sprintf("%d", value)
	}
	return fmt.Sprintf("%d%s", value, unit)
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
