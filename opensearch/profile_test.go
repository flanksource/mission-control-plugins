package main

import "testing"

func TestResolveProfileIncludesDefaults(t *testing.T) {
	profile, err := defaultSettings().resolveProfile("jaeger")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Index != "jaeger-span*" {
		t.Fatalf("index = %q", profile.Index)
	}
	if _, ok := profile.Params["trace_id"]; !ok {
		t.Fatalf("trace_id param missing")
	}
}

func TestBuildTraceQueryRejectsUnknownFilter(t *testing.T) {
	profile, err := defaultSettings().resolveProfile("otel")
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildTraceQuery(QueryBuildOptions{Params: map[string]any{"missing": "x"}}, profile)
	if err == nil {
		t.Fatalf("expected unsupported filter error")
	}
}

func TestBuildTraceQueryUsesTermsForCSV(t *testing.T) {
	profile := TracingProfile{
		Name:      "test",
		Index:     "traces-*",
		DateField: "@timestamp",
		Params: map[string]TracingParam{
			"trace_id": {Field: "trace_id", Operator: "term"},
		},
	}.withDefaults()
	query, err := buildTraceQuery(QueryBuildOptions{Params: map[string]any{"trace_id": "a,b"}}, profile)
	if err != nil {
		t.Fatal(err)
	}
	boolQuery := query["query"].(map[string]any)["bool"].(map[string]any)
	filter := boolQuery["filter"].([]map[string]any)
	if _, ok := filter[0]["terms"]; !ok {
		t.Fatalf("expected terms query, got %#v", filter[0])
	}
}
