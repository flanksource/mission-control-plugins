package main

import "testing"

func TestParseJaegerSearchResult(t *testing.T) {
	profile, err := defaultSettings().resolveProfile("jaeger")
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]any{
		"hits": map[string]any{
			"total": map[string]any{"value": float64(1)},
			"hits": []any{
				map[string]any{
					"_source": map[string]any{
						"traceID":         "t1",
						"spanID":          "s1",
						"operationName":   "GET /health",
						"startTimeMillis": float64(1710000000000),
						"duration":        float64(123000),
						"process": map[string]any{
							"serviceName": "api",
						},
						"tags": []any{
							map[string]any{"key": "otel@status_code", "value": "OK"},
						},
					},
				},
			},
		},
	}
	parsed := parseSearchResult(result, profile)
	if parsed.Total != 1 || len(parsed.Traces) != 1 {
		t.Fatalf("unexpected result: %#v", parsed)
	}
	trace := parsed.Traces[0]
	if trace.TraceID != "t1" || trace.ServiceName != "api" || trace.OperationName != "GET /health" {
		t.Fatalf("unexpected trace: %#v", trace)
	}
	if trace.Status != "OK" {
		t.Fatalf("status = %q", trace.Status)
	}
}
