package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type TraceResult struct {
	Profile string  `json:"profile"`
	Index   string  `json:"index"`
	Total   int     `json:"total"`
	Traces  []Trace `json:"traces"`
}

type Trace struct {
	Timestamp     string         `json:"timestamp,omitempty"`
	TraceID       string         `json:"trace_id,omitempty"`
	SpanID        string         `json:"span_id,omitempty"`
	ParentID      string         `json:"parent_id,omitempty"`
	ServiceName   string         `json:"service_name,omitempty"`
	OperationName string         `json:"operation_name,omitempty"`
	Status        string         `json:"status,omitempty"`
	DurationMS    float64        `json:"duration_ms,omitempty"`
	Attributes    map[string]any `json:"attributes,omitempty"`
	Cells         map[string]any `json:"cells,omitempty"`
	Raw           map[string]any `json:"raw,omitempty"`
	Children      []Trace        `json:"children,omitempty"`
}

func parseSearchResult(result map[string]any, profile TracingProfile) TraceResult {
	hitsObj, _ := result["hits"].(map[string]any)
	hits, _ := hitsObj["hits"].([]any)
	traces := make([]Trace, 0, len(hits))
	for _, hitRaw := range hits {
		hit, _ := hitRaw.(map[string]any)
		source, _ := hit["_source"].(map[string]any)
		if source == nil {
			source = map[string]any{}
		}
		fields, _ := hit["fields"].(map[string]any)
		merged := cloneAnyMap(source)
		for k, v := range fields {
			if _, ok := merged[k]; !ok {
				merged[k] = unwrapFieldValue(v)
			}
		}
		trace := traceFromDocument(merged, profile)
		trace.Raw = merged
		trace.Cells = evalCells(trace, profile.Columns)
		traces = append(traces, trace)
	}
	return TraceResult{
		Profile: profile.Name,
		Index:   profile.Index,
		Total:   totalHits(hitsObj, len(traces)),
		Traces:  buildTraceTree(traces),
	}
}

func traceFromDocument(doc map[string]any, profile TracingProfile) Trace {
	attrs := flattenDocument(doc)
	trace := Trace{
		Timestamp:     stringValue(firstPath(doc, profile.DateField, "@timestamp", "timestamp", "startTimeMillis")),
		TraceID:       stringValue(firstPath(doc, profile.TraceIDField, "trace_id", "traceID")),
		SpanID:        stringValue(firstPath(doc, profile.SpanIDField, "span_id", "spanID")),
		ParentID:      parentID(doc, profile),
		ServiceName:   stringValue(firstPath(doc, profile.ServiceField, "service_name", "process.serviceName")),
		OperationName: stringValue(firstPath(doc, profile.OperationField, "operation_name", "operationName")),
		DurationMS:    durationMillis(firstPath(doc, "duration_ms", "duration", "durationNano", "duration_nano")),
		Attributes:    attrs,
	}
	trace.Status = statusValue(doc, attrs, profile)
	if ts := normalizeTimestamp(trace.Timestamp); ts != "" {
		trace.Timestamp = ts
	}
	return trace
}

func evalCells(trace Trace, columns []TracingColumn) map[string]any {
	env := map[string]any{
		"timestamp":      trace.Timestamp,
		"trace_id":       trace.TraceID,
		"span_id":        trace.SpanID,
		"parent_id":      trace.ParentID,
		"service_name":   trace.ServiceName,
		"operation_name": trace.OperationName,
		"status":         trace.Status,
		"duration_ms":    trace.DurationMS,
	}
	for k, v := range trace.Attributes {
		env[k] = v
	}
	out := make(map[string]any, len(columns))
	for _, col := range columns {
		if col.Name == "" || col.Field == "" {
			continue
		}
		if v, ok := env[col.Field]; ok {
			out[col.Name] = v
			continue
		}
		if v := lookupPath(trace.Raw, col.Field); v != nil {
			out[col.Name] = v
		}
	}
	return out
}

func parentID(doc map[string]any, profile TracingProfile) string {
	if profile.Format == "jaeger" {
		if refs, ok := doc["references"].([]any); ok {
			for _, raw := range refs {
				ref, _ := raw.(map[string]any)
				if profile.ParentRefType != "" {
					if stringValue(ref["refType"]) != profile.ParentRefType {
						continue
					}
				}
				if id := stringValue(firstPath(ref, "spanID", "span_id")); id != "" {
					return id
				}
			}
		}
	}
	return stringValue(firstPath(doc, profile.ParentIDField, "parent_id", "parentID"))
}

func statusValue(doc map[string]any, attrs map[string]any, profile TracingProfile) string {
	for _, field := range profile.StatusFields {
		if v := stringValue(lookupPath(doc, field)); v != "" {
			return v
		}
		if v := stringValue(attrs[field]); v != "" {
			return v
		}
	}
	for _, field := range []string{"status", "status.code", "tag.error", "error"} {
		if v := stringValue(lookupPath(doc, field)); v != "" {
			return v
		}
		if v := stringValue(attrs[field]); v != "" {
			return v
		}
	}
	return ""
}

func buildTraceTree(flat []Trace) []Trace {
	byID := map[string]*Trace{}
	for i := range flat {
		if flat[i].SpanID != "" {
			byID[flat[i].SpanID] = &flat[i]
		}
	}
	var roots []Trace
	for i := range flat {
		if flat[i].ParentID != "" {
			if parent, ok := byID[flat[i].ParentID]; ok {
				parent.Children = append(parent.Children, flat[i])
				continue
			}
		}
		roots = append(roots, flat[i])
	}
	if len(roots) == 0 {
		return flat
	}
	return roots
}

func flattenDocument(doc map[string]any) map[string]any {
	out := map[string]any{}
	var walk func(string, any)
	walk = func(prefix string, value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				next := key
				if prefix != "" {
					next = prefix + "." + key
				}
				walk(next, child)
			}
		case []any:
			if isJaegerTags(prefix, v) {
				for _, raw := range v {
					tag, _ := raw.(map[string]any)
					key := stringValue(tag["key"])
					if key == "" {
						continue
					}
					out["tag."+key] = firstPath(tag, "value", "vStr", "vDouble", "vBool", "vLong")
				}
				return
			}
			out[prefix] = v
		default:
			out[prefix] = v
		}
	}
	walk("", doc)
	return out
}

func isJaegerTags(prefix string, values []any) bool {
	if prefix != "tags" && prefix != "process.tags" {
		return false
	}
	if len(values) == 0 {
		return false
	}
	first, ok := values[0].(map[string]any)
	if !ok {
		return false
	}
	_, hasKey := first["key"]
	return hasKey
}

func firstPath(doc map[string]any, paths ...string) any {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if v := lookupPath(doc, path); v != nil {
			return v
		}
	}
	return nil
}

func lookupPath(doc map[string]any, dotted string) any {
	if doc == nil || dotted == "" {
		return nil
	}
	if v, ok := doc[dotted]; ok {
		return v
	}
	parts := strings.Split(dotted, ".")
	var cur any = doc
	for _, part := range parts {
		switch typed := cur.(type) {
		case map[string]any:
			cur = typed[part]
		case []any:
			if len(typed) == 0 {
				return nil
			}
			cur = typed[0]
			if m, ok := cur.(map[string]any); ok {
				cur = m[part]
			}
		default:
			return nil
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}

func unwrapFieldValue(v any) any {
	if arr, ok := v.([]any); ok && len(arr) == 1 {
		return arr[0]
	}
	return v
}

func stringValue(v any) string {
	switch t := unwrapFieldValue(v).(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		if math.Trunc(t) == t {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

func durationMillis(v any) float64 {
	switch t := unwrapFieldValue(v).(type) {
	case float64:
		if t > 1_000_000 {
			return math.Round(t/10_000) / 100
		}
		return t
	case int:
		return float64(t)
	case string:
		n, _ := strconv.ParseFloat(t, 64)
		if n > 1_000_000 {
			return math.Round(n/10_000) / 100
		}
		return n
	default:
		return 0
	}
}

func normalizeTimestamp(raw string) string {
	if raw == "" {
		return ""
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if n > 10_000_000_000_000 {
			return time.UnixMilli(n).Format(time.RFC3339Nano)
		}
		return time.Unix(n, 0).Format(time.RFC3339Nano)
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts.Format(time.RFC3339Nano)
	}
	return raw
}

func totalHits(hits map[string]any, fallback int) int {
	total, ok := hits["total"].(map[string]any)
	if !ok {
		return fallback
	}
	switch v := total["value"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return fallback
	}
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
