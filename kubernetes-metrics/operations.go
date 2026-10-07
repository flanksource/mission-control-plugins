package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/flanksource/incident-commander/plugin/sdk"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"
)

type invalidError struct{ message string }

func (e *invalidError) Error() string { return "EINVALID: " + e.message }
func invalid(format string, args ...any) error {
	return &invalidError{message: fmt.Sprintf(format, args...)}
}

// lookupError carries the HTTP status for a config item the host could not
// find or would not return, so callers do not retry it as an upstream failure.
type lookupError struct {
	status int
	code   string
	err    error
}

func (e *lookupError) Error() string { return e.code + ": " + e.err.Error() }
func (e *lookupError) Unwrap() error { return e.err }

type resourceCurrent struct {
	Usage   *float64 `json:"usage"`
	Request *float64 `json:"request"`
	Limit   *float64 `json:"limit"`
}

type currentResult struct {
	At     time.Time       `json:"at"`
	Pods   int             `json:"pods"`
	CPU    resourceCurrent `json:"cpu"`
	Memory resourceCurrent `json:"memory"`
}

type point struct {
	At    time.Time `json:"at"`
	Value *float64  `json:"value"`
}

type resourceHistory struct {
	Usage []point `json:"usage"`
	Limit []point `json:"limit"`
}

type historyResult struct {
	Range  string          `json:"range"`
	Step   string          `json:"step"`
	CPU    resourceHistory `json:"cpu"`
	Memory resourceHistory `json:"memory"`
}

type historyParams struct {
	Range string `json:"range"`
	Step  string `json:"step"`
}

func parseHistoryParams(raw []byte) (historyParams, time.Duration, time.Duration, error) {
	var params historyParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, 0, 0, invalid("invalid history params: %v", err)
	}
	duration, ok := map[string]time.Duration{"15m": 15 * time.Minute, "1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour}[params.Range]
	if !ok {
		return params, 0, 0, invalid("range must be one of 15m, 1h, 6h, 24h")
	}
	if params.Step == "" {
		params.Step = "15s"
	}
	step, err := time.ParseDuration(params.Step)
	if err != nil || step <= 0 {
		return params, 0, 0, invalid("step must be a positive duration")
	}
	// Range endpoints are inclusive: 999 intervals produce at most 1000 points.
	minimum := ((duration/time.Second + 998) / 999) * time.Second
	if step < minimum {
		step = minimum
	}
	params.Step = step.String()
	return params, duration, step, nil
}

func (*KubernetesMetricsPlugin) current(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	q, err := resolveQueries(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	api, closeClient, err := prometheusClient(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	at := time.Now().UTC()
	out := currentResult{At: at}
	var pods *float64
	// The queries are independent; run them concurrently so a slow Prometheus
	// costs one round trip instead of seven.
	g, gctx := errgroup.WithContext(ctx)
	for _, query := range []struct {
		expr  string
		value **float64
	}{
		{q.pods, &pods},
		{q.cpuUsage, &out.CPU.Usage}, {q.memoryUsage, &out.Memory.Usage},
		{q.cpuRequest, &out.CPU.Request}, {q.memoryRequest, &out.Memory.Request},
		{q.cpuLimit, &out.CPU.Limit}, {q.memoryLimit, &out.Memory.Limit},
	} {
		g.Go(func() (err error) {
			*query.value, err = instant(gctx, api, query.expr, at)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if pods != nil {
		out.Pods = int(*pods)
	}
	// An empty owner join alone cannot distinguish missing KSM from zero pods.
	if out.Pods == 0 && q.zeroReplicas != "" && (out.CPU.Usage == nil || out.Memory.Usage == nil) {
		zero, err := instant(ctx, api, q.zeroReplicas, at)
		if err != nil {
			return nil, err
		}
		if zero != nil && *zero == 0 {
			if out.CPU.Usage == nil {
				out.CPU.Usage = zero
			}
			if out.Memory.Usage == nil {
				out.Memory.Usage = zero
			}
		}
	}
	return out, nil
}

func (*KubernetesMetricsPlugin) history(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	params, duration, step, err := parseHistoryParams(req.ParamsJSON)
	if err != nil {
		return nil, err
	}
	q, err := resolveQueries(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	api, closeClient, err := prometheusClient(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	end := time.Now().UTC()
	r := v1.Range{Start: end.Add(-duration), End: end, Step: step}
	out := historyResult{Range: params.Range, Step: params.Step}
	g, gctx := errgroup.WithContext(ctx)
	for _, query := range []struct {
		expr   string
		points *[]point
	}{
		{q.cpuUsage, &out.CPU.Usage}, {q.cpuLimit, &out.CPU.Limit},
		{q.memoryUsage, &out.Memory.Usage}, {q.memoryLimit, &out.Memory.Limit},
	} {
		g.Go(func() (err error) {
			*query.points, err = series(gctx, api, query.expr, r)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

func sampleValue(v model.SampleValue) *float64 {
	n := float64(v)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}

func instant(ctx context.Context, api v1.API, expr string, at time.Time) (*float64, error) {
	value, warnings, err := api.Query(ctx, expr, at)
	if err != nil {
		return nil, fmt.Errorf("prometheus query: %w", err)
	}
	if len(warnings) > 0 {
		return nil, fmt.Errorf("prometheus query warnings: %v", warnings)
	}
	vector, ok := value.(model.Vector)
	if !ok || len(vector) > 1 {
		return nil, fmt.Errorf("prometheus query: expected at most one aggregate sample, got %T", value)
	}
	if len(vector) == 0 {
		return nil, nil
	}
	return sampleValue(vector[0].Value), nil
}

func series(ctx context.Context, api v1.API, expr string, r v1.Range) ([]point, error) {
	value, warnings, err := api.QueryRange(ctx, expr, r)
	if err != nil {
		return nil, fmt.Errorf("prometheus range query: %w", err)
	}
	if len(warnings) > 0 {
		return nil, fmt.Errorf("prometheus range query warnings: %v", warnings)
	}
	matrix, ok := value.(model.Matrix)
	if !ok || len(matrix) > 1 {
		return nil, fmt.Errorf("prometheus range query: expected at most one aggregate series, got %T", value)
	}
	points := make([]point, 0)
	for _, stream := range matrix {
		for _, sample := range stream.Values {
			points = append(points, point{At: sample.Timestamp.Time().UTC(), Value: sampleValue(sample.Value)})
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })
	return points, nil
}
