package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

type TraceStartParams struct {
	Gadget      string         `json:"gadget"`
	Namespace   string         `json:"namespace,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	Name        string         `json:"name,omitempty"`
	Pod         string         `json:"pod,omitempty"`
	Container   string         `json:"container,omitempty"`
	DurationSec int            `json:"durationSec,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Args        []string       `json:"args,omitempty"`
	ArgString   string         `json:"argString,omitempty"`
}

type TraceStopParams struct {
	ID string `json:"id"`
}

type TraceEventsParams struct {
	ID string `json:"id"`
}

func (p *InspektorGadgetPlugin) tracesList(_ context.Context, _ sdk.InvokeCtx) (any, error) {
	return supportedGadgets(defaultIGTag), nil
}

func (p *InspektorGadgetPlugin) traceList(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	pods, err := p.currentPods(ctx, req)
	if err != nil {
		return nil, err
	}
	sessions := p.sessions.List()
	out := make([]*TraceSession, 0, len(sessions))
	for i := range sessions {
		if traceTargetInPods(sessions[i].Target, pods) {
			out = append(out, &sessions[i])
		}
	}
	return out, nil
}

func (p *InspektorGadgetPlugin) traceEvents(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params TraceEventsParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	sess, err := p.sessionForConfig(ctx, req, params.ID)
	if err != nil {
		return nil, err
	}
	return sess.Events(), nil
}

func (p *InspektorGadgetPlugin) traceStop(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params TraceStopParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	sess, err := p.sessionForConfig(ctx, req, params.ID)
	if err != nil {
		return nil, err
	}
	sess.Stop()
	return sess.Snapshot(), nil
}

func (p *InspektorGadgetPlugin) traceStart(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params TraceStartParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if params.Gadget == "" {
		params.Gadget = "trace_exec"
	}
	if p.sessions.RunningCount() >= p.settings.MaxSessions {
		return nil, fmt.Errorf("maximum running sessions reached (%d)", p.settings.MaxSessions)
	}
	gadget, ok := gadgetByID(params.Gadget, defaultIGTag)
	if !ok {
		return nil, fmt.Errorf("unsupported gadget %q", params.Gadget)
	}

	cli, err := p.clients.Client(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	restCfg, err := p.clients.RESTConfig(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	target, err := p.createTraceTarget(ctx, req, params)
	if err != nil {
		return nil, err
	}
	pods, selector, err := listRunningPodsAndSelectorForTarget(ctx, cli, TargetRef{Namespace: target.Namespace, Kind: target.Kind, Name: target.Name, Selector: target.Selector})
	if err != nil {
		return nil, fmt.Errorf("resolve pods: %w", err)
	}
	target, err = resolveTraceTarget(target, pods, selector)
	if err != nil {
		return nil, err
	}

	options, err := normalizeTraceOptions(params)
	if err != nil {
		return nil, err
	}
	runParams := buildGadgetParams(target, options)
	duration := p.duration(params.DurationSec)
	diagnostics := TraceDiagnostics{
		Runtime:      "inspektor-gadget-gadget-service-grpc",
		Connection:   "kubernetes-api-portforward",
		DurationSec:  int(duration.Seconds()),
		MaxSessions:  p.settings.MaxSessions,
		ResolvedPods: pods,
		UserOptions:  options,
	}
	session, runCtx := newTraceSession(gadget, target, runParams, diagnostics, p.settings.MaxEvents)
	p.sessions.Add(session)

	go func() {
		ctx, cancel := context.WithTimeout(runCtx, duration)
		defer cancel()
		session.MarkRunning()
		err := p.runner.Run(ctx, TraceRunRequest{
			Image:           gadget.Image,
			Params:          runParams,
			RESTConfig:      restCfg,
			GadgetNamespace: p.settings.GadgetNamespace,
			Timeout:         duration,
		}, session.AddEvent)
		session.MarkDone(err)
	}()

	return session.Snapshot(), nil
}

func (p *InspektorGadgetPlugin) sessionForConfig(ctx context.Context, req sdk.InvokeCtx, id string) (*TraceSession, error) {
	sess, ok := p.sessions.Get(id)
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	if req.ConfigItemID == "" {
		return sess, nil
	}
	pods, err := p.currentPods(ctx, req)
	if err != nil {
		return nil, err
	}
	if !traceTargetInPods(sess.Target, pods) {
		return nil, fmt.Errorf("session %q does not belong to the current config item", id)
	}
	return sess, nil
}

func (p *InspektorGadgetPlugin) currentPods(ctx context.Context, req sdk.InvokeCtx) ([]RunningPod, error) {
	target, err := targetFromConfig(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	cli, err := p.clients.Client(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	pods, err := listRunningPodsForTarget(ctx, cli, target)
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func traceTargetInPods(target TraceTarget, pods []RunningPod) bool {
	for _, pod := range pods {
		if pod.Namespace != target.Namespace {
			continue
		}
		if target.Pod != "" && pod.Name != target.Pod {
			continue
		}
		if target.Pod == "" {
			if len(target.Selector) > 0 && !labelsMatch(pod.Labels, target.Selector) {
				continue
			}
			if len(target.Selector) == 0 && target.Name != "" && target.Kind != "" && !sameWorkloadOwner(target, pod) {
				continue
			}
		}
		if target.Container == "" || stringInSlice(target.Container, pod.Containers) {
			return true
		}
	}
	return false
}

func normalizeTraceOptions(params TraceStartParams) (map[string]any, error) {
	options := map[string]any{}
	for k, v := range params.Options {
		if strings.TrimSpace(k) != "" && v != nil {
			options[strings.TrimSpace(k)] = v
		}
	}
	for k, v := range params.Arguments {
		if strings.TrimSpace(k) != "" && v != nil {
			options[strings.TrimSpace(k)] = v
		}
	}
	if params.ArgString != "" {
		parsed, err := parseArgLines(strings.Split(params.ArgString, "\n"))
		if err != nil {
			return nil, err
		}
		for k, v := range parsed {
			options[k] = v
		}
	}
	parsed, err := parseArgLines(params.Args)
	if err != nil {
		return nil, err
	}
	for k, v := range parsed {
		options[k] = v
	}
	return options, nil
}

func resolveTraceTarget(target TraceTarget, pods []RunningPod, selector map[string]string) (TraceTarget, error) {
	if len(pods) == 0 {
		return TraceTarget{}, fmt.Errorf("no ready pods found for %s/%s in namespace %s", target.Kind, target.Name, target.Namespace)
	}

	if isPodKind(target.Kind) || target.Pod != "" {
		pod := pods[0]
		if target.Pod == "" {
			target.Pod = pod.Name
		} else {
			selected, ok := runningPodByName(pods, target.Pod)
			if !ok {
				return TraceTarget{}, fmt.Errorf("pod %s/%s is not a ready pod for %s/%s", target.Namespace, target.Pod, target.Kind, target.Name)
			}
			pod = selected
		}
		target.Node = pod.Node
		if target.Container == "" && len(pod.Containers) == 1 {
			target.Container = pod.Containers[0]
		}
		return target, nil
	}

	if len(selector) > 0 {
		target.Selector = selector
	} else if len(target.Selector) == 0 && len(pods) == 1 {
		target.Selector = pods[0].Labels
	}
	target.Node = strings.Join(uniquePodNodes(pods), ",")
	if target.Container == "" {
		target.Container = commonSingleContainer(pods)
	}
	return target, nil
}

func isPodKind(kind string) bool {
	switch normalizeKind(kind) {
	case "pod", "pods", "po":
		return true
	default:
		return false
	}
}

func runningPodByName(pods []RunningPod, name string) (RunningPod, bool) {
	for _, pod := range pods {
		if pod.Name == name {
			return pod, true
		}
	}
	return RunningPod{}, false
}

func uniquePodNodes(pods []RunningPod) []string {
	seen := map[string]bool{}
	nodes := make([]string, 0, len(pods))
	for _, pod := range pods {
		if pod.Node == "" || seen[pod.Node] {
			continue
		}
		seen[pod.Node] = true
		nodes = append(nodes, pod.Node)
	}
	sort.Strings(nodes)
	return nodes
}

func commonSingleContainer(pods []RunningPod) string {
	if len(pods) == 0 || len(pods[0].Containers) != 1 {
		return ""
	}
	container := pods[0].Containers[0]
	for _, pod := range pods[1:] {
		if len(pod.Containers) != 1 || pod.Containers[0] != container {
			return ""
		}
	}
	return container
}

func labelsMatch(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func sameWorkloadOwner(target TraceTarget, pod RunningPod) bool {
	return normalizeKind(target.Kind) == normalizeKind(pod.OwnerKind) && target.Name == pod.OwnerName
}

func stringInSlice(value string, values []string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func parseArgLines(lines []string) (map[string]any, error) {
	out := map[string]any{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "--")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid gadget argument %q", line)
		}
		if !ok {
			out[key] = true
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out, nil
}

func (p *InspektorGadgetPlugin) createTraceTarget(ctx context.Context, req sdk.InvokeCtx, params TraceStartParams) (TraceTarget, error) {
	if params.Pod != "" {
		ns := params.Namespace
		base := TargetRef{}
		if ns == "" {
			var err error
			base, err = targetFromConfig(ctx, req.Host, req.ConfigItemID)
			if err != nil {
				return TraceTarget{}, err
			}
			ns = base.Namespace
		}
		return TraceTarget{Namespace: ns, Kind: "pod", Name: params.Pod, Pod: params.Pod, Container: params.Container}, nil
	}
	if params.Kind != "" && params.Name != "" && params.Namespace != "" {
		return TraceTarget{Namespace: params.Namespace, Kind: normalizeKind(params.Kind), Name: params.Name, Container: params.Container}, nil
	}
	base, err := targetFromConfig(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return TraceTarget{}, err
	}
	if params.Kind != "" && params.Name != "" {
		base.Kind = normalizeKind(params.Kind)
		base.Name = params.Name
	}
	if params.Namespace != "" {
		base.Namespace = params.Namespace
	}
	return TraceTarget{Namespace: base.Namespace, Kind: base.Kind, Name: base.Name, Container: params.Container}, nil
}

func (p *InspektorGadgetPlugin) duration(requested int) time.Duration {
	max := p.settings.MaxDurationSec
	if max <= 0 {
		max = 300
	}
	if requested <= 0 || requested > max {
		requested = max
	}
	return time.Duration(requested) * time.Second
}
