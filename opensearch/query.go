package main

import (
	"fmt"
	"sort"
	"strings"
)

const (
	clauseFilter  = "filter"
	clauseMust    = "must"
	clauseShould  = "should"
	clauseMustNot = "must_not"
)

type QueryBuildOptions struct {
	From   string
	To     string
	Params map[string]any
}

type TraceQueryPreview struct {
	Profile string                   `json:"profile"`
	Request TraceQueryPreviewRequest `json:"request"`
}

type TraceQueryPreviewRequest struct {
	Index string         `json:"index"`
	Limit int            `json:"limit"`
	Query map[string]any `json:"query"`
}

func buildQueryPreview(profile TracingProfile, req TraceQueryRequest) (TraceQueryPreview, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	query, err := buildTraceQuery(QueryBuildOptions{From: req.From, To: req.To, Params: req.Params}, profile)
	if err != nil {
		return TraceQueryPreview{}, err
	}
	return TraceQueryPreview{
		Profile: profile.Name,
		Request: TraceQueryPreviewRequest{
			Index: profile.Index,
			Limit: limit,
			Query: query,
		},
	}, nil
}

func buildTraceQuery(opts QueryBuildOptions, profile TracingProfile) (map[string]any, error) {
	queryParams, err := mergeQueryParams(profile, opts.Params)
	if err != nil {
		return nil, err
	}

	boolClauses := map[string][]map[string]any{
		clauseFilter:  {},
		clauseMust:    {},
		clauseShould:  {},
		clauseMustNot: {},
	}

	names := make([]string, 0, len(queryParams))
	for name := range queryParams {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		param, ok := profile.Params[name]
		if !ok {
			return nil, fmt.Errorf("filter %q is not supported by tracing profile %q", name, profile.Name)
		}
		clauses, err := buildParamClauses(param, queryParams[name])
		if err != nil {
			return nil, fmt.Errorf("build filter %q: %w", name, err)
		}
		bucket := firstNonEmpty(param.Clause, clauseFilter)
		if _, ok := boolClauses[bucket]; !ok {
			return nil, fmt.Errorf("unsupported clause %q for filter %q", bucket, name)
		}
		boolClauses[bucket] = append(boolClauses[bucket], clauses...)
	}

	rangeQuery := map[string]any{}
	if strings.TrimSpace(opts.From) != "" {
		rangeQuery["gte"] = strings.TrimSpace(opts.From)
	}
	if strings.TrimSpace(opts.To) != "" {
		rangeQuery["lte"] = strings.TrimSpace(opts.To)
	}
	if len(rangeQuery) > 0 {
		rangeQuery["format"] = "strict_date_optional_time||epoch_millis"
		boolClauses[clauseFilter] = append(boolClauses[clauseFilter], map[string]any{
			"range": map[string]any{profile.DateField: rangeQuery},
		})
	}

	query := map[string]any{
		"sort": []map[string]any{{profile.DateField: map[string]any{"order": "desc"}}},
	}
	if len(profile.SelectFields) > 0 {
		query["stored_fields"] = []string{"*"}
		query["fields"] = profile.SelectFields
	}
	if profile.DateField != "" {
		query["docvalue_fields"] = []map[string]any{{"field": profile.DateField, "format": "date_time"}}
	}
	if len(profile.SourceExcludes) > 0 {
		query["_source"] = map[string]any{"excludes": profile.SourceExcludes}
	}

	boolQuery := map[string]any{}
	for _, bucket := range []string{clauseFilter, clauseMust, clauseShould, clauseMustNot} {
		if len(boolClauses[bucket]) > 0 {
			boolQuery[bucket] = boolClauses[bucket]
		}
	}
	if len(boolQuery) > 0 {
		if len(boolClauses[clauseShould]) > 0 {
			boolQuery["minimum_should_match"] = 1
		}
		query["query"] = map[string]any{"bool": boolQuery}
	} else {
		query["query"] = map[string]any{"match_all": map[string]any{}}
	}
	return query, nil
}

func mergeQueryParams(profile TracingProfile, overrides map[string]any) (map[string]any, error) {
	params := cloneMap(profile.Defaults)
	for key, value := range overrides {
		if isEmptyParamValue(value) {
			continue
		}
		if param, ok := profile.Params[key]; ok && param.Internal {
			return nil, fmt.Errorf("filter %q is internal to tracing profile %q", key, profile.Name)
		}
		params[key] = value
	}
	var missing []string
	for name, param := range profile.Params {
		if !param.Required {
			continue
		}
		if isEmptyParamValue(params[name]) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("tracing profile %q requires param(s): %s", profile.Name, strings.Join(missing, ", "))
	}
	return params, nil
}

func buildParamClauses(param TracingParam, value any) ([]map[string]any, error) {
	values := normalizeParamValues(value)
	if len(values) == 0 {
		return nil, nil
	}
	formatted := make([]any, 0, len(values))
	for _, item := range values {
		formatted = append(formatted, formatParamValue(param, item))
	}
	switch firstNonEmpty(param.Operator, "term") {
	case "term":
		if len(formatted) > 1 {
			return []map[string]any{{"terms": map[string]any{param.Field: formatted}}}, nil
		}
		return []map[string]any{{"term": map[string]any{param.Field: formatted[0]}}}, nil
	case "terms":
		return []map[string]any{{"terms": map[string]any{param.Field: formatted}}}, nil
	case "match_phrase":
		out := make([]map[string]any, 0, len(formatted))
		for _, item := range formatted {
			out = append(out, map[string]any{"match_phrase": map[string]any{param.Field: item}})
		}
		return out, nil
	case "wildcard":
		out := make([]map[string]any, 0, len(formatted))
		for _, item := range formatted {
			out = append(out, map[string]any{"wildcard": map[string]any{param.Field: item}})
		}
		return out, nil
	case "query_string":
		out := make([]map[string]any, 0, len(formatted))
		for _, item := range formatted {
			out = append(out, map[string]any{"query_string": map[string]any{"fields": []string{param.Field}, "query": item}})
		}
		return out, nil
	case "exists":
		return []map[string]any{{"exists": map[string]any{"field": param.Field}}}, nil
	default:
		return nil, fmt.Errorf("unsupported operator %q", param.Operator)
	}
}

func formatParamValue(param TracingParam, value any) any {
	s, ok := value.(string)
	if !ok {
		return value
	}
	if param.Template != "" {
		return strings.ReplaceAll(param.Template, "{value}", s)
	}
	return s
}

func normalizeParamValues(value any) []any {
	switch v := value.(type) {
	case nil:
		return nil
	case []any:
		return v
	case []string:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		return out
	case string:
		if strings.Contains(v, ",") {
			parts := strings.Split(v, ",")
			out := make([]any, 0, len(parts))
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					out = append(out, trimmed)
				}
			}
			return out
		}
		return []any{v}
	default:
		return []any{v}
	}
}

func isEmptyParamValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	case []string:
		return len(v) == 0
	default:
		return false
	}
}
