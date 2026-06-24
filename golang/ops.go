package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flanksource/incident-commander/plugin/sdk"
	golangk8s "github.com/flanksource/mission-control-plugins/golang/internal/k8s"
	"k8s.io/client-go/rest"
)

type SessionCreateParams struct {
	Namespace     string `json:"namespace,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Name          string `json:"name,omitempty"`
	Pod           string `json:"pod,omitempty"`
	Container     string `json:"container,omitempty"`
	PID           int    `json:"pid,omitempty"`
	UseGops       *bool  `json:"useGops,omitempty"`
	UsePprof      *bool  `json:"usePprof,omitempty"`
	GopsPort      int    `json:"gopsPort,omitempty"`
	PprofPort     int    `json:"pprofPort,omitempty"`
	PprofBasePath string `json:"pprofBasePath,omitempty"`
	GopsConfigDir string `json:"gopsConfigDir,omitempty"`
	LocalGops     int    `json:"localGops,omitempty"`
	LocalPprof    int    `json:"localPprof,omitempty"`
}

type SessionDeleteParams struct {
	ID string `json:"id"`
}

type SessionIDParams struct {
	SessionID string `json:"sessionId"`
}

type ProfileCollectParams struct {
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	Seconds   int    `json:"seconds,omitempty"`
	Source    string `json:"source,omitempty"`
}

type ProfileRunParams struct {
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	Seconds   int    `json:"seconds,omitempty"`
	Source    string `json:"source,omitempty"`
}

type ProfileRunIDParams struct {
	SessionID string `json:"sessionId,omitempty"`
	RunID     string `json:"runId"`
}

type RuntimeSnapshot struct {
	SessionID string `json:"sessionId"`
	Version   string `json:"version,omitempty"`
	Stats     string `json:"stats,omitempty"`
	MemStats  string `json:"memstats,omitempty"`
	Error     string `json:"error,omitempty"`
}

type GoroutineSnapshot struct {
	SessionID string `json:"sessionId"`
	Source    string `json:"source"`
	Dump      string `json:"dump"`
	Error     string `json:"error,omitempty"`
}

type ProfileResult struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId,omitempty"`
	Kind      string `json:"kind"`
	Source    string `json:"source"`
	Bytes     int    `json:"bytes"`
	URL       string `json:"url"`
	Seconds   int    `json:"seconds,omitempty"`
}

type portCandidate struct {
	Remote int
	Local  int
	Source string
}

func (p *GolangPlugin) podsList(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	target, err := targetFromConfig(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	cli, err := p.clients.Client(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	return listRunningPodsForTarget(ctx, cli, target)
}

func (p *GolangPlugin) sessionsList(_ context.Context, req sdk.InvokeCtx) (any, error) {
	return p.sessions.List(req.ConfigItemID), nil
}

func (p *GolangPlugin) sessionCreate(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params SessionCreateParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if p.sessions.RunningCount() >= p.settings.MaxSessions {
		return nil, fmt.Errorf("maximum running sessions reached (%d)", p.settings.MaxSessions)
	}
	target, err := p.createTarget(ctx, req, params)
	if err != nil {
		return nil, err
	}
	cli, err := p.clients.Client(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	restCfg, err := p.clients.RESTConfig(ctx, req.Host)
	if err != nil {
		return nil, err
	}
	pods, err := listRunningPodsForTarget(ctx, cli, target)
	if err != nil {
		return nil, fmt.Errorf("resolve pods: %w", err)
	}
	pod, container, err := selectPodContainer(pods, params.Pod, params.Container)
	if err != nil {
		return nil, err
	}

	var diagnostics []string
	useGops := params.UseGops == nil || *params.UseGops
	usePprof := params.UsePprof == nil || *params.UsePprof
	gopsPort := params.GopsPort
	pid := params.PID
	if !useGops {
		diagnostics = append(diagnostics, "gops disabled by request")
	}

	dirs := append([]string{}, p.settings.GopsConfigDirs...)
	if params.GopsConfigDir != "" {
		dirs = append([]string{params.GopsConfigDir}, dirs...)
	}

	var discoveredGopsPorts []int
	gopsPortPID := map[int]int{}
	if useGops && gopsPort == 0 {
		discoverCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		procs, discoveryDiagnostics, err := discoverGopsProcesses(discoverCtx, restCfg, pod.Namespace, pod.Name, container, dirs)
		diagnostics = appendPrefixedDiagnostics(diagnostics, "gops discovery: ", discoveryDiagnostics)
		if err != nil {
			diagnostics = append(diagnostics, err.Error())
		} else if candidates := orderGopsCandidates(procs, pid); len(candidates) > 0 {
			for _, proc := range candidates {
				discoveredGopsPorts = append(discoveredGopsPorts, proc.Port)
				if _, ok := gopsPortPID[proc.Port]; !ok {
					gopsPortPID[proc.Port] = proc.PID
				}
			}
			diagnostics = append(diagnostics, fmt.Sprintf("discovered gops: %s", formatGopsCandidates(candidates)))
		} else if params.PID > 0 {
			diagnostics = append(diagnostics, fmt.Sprintf("no gops port file found for pid %d", params.PID))
		} else {
			diagnostics = append(diagnostics, "no gops port file found")
		}
	}

	gopsPorts := []int{}
	if useGops {
		switch {
		case len(discoveredGopsPorts) > 0:
			gopsPorts = append(gopsPorts, discoveredGopsPorts...)
			fallbackPorts := uniquePositiveInts(p.settings.DefaultGopsPorts...)
			if len(fallbackPorts) > 0 {
				gopsPorts = append(gopsPorts, fallbackPorts...)
				diagnostics = append(diagnostics, fmt.Sprintf("trying default gops ports after discovered candidates: %s", formatPorts(fallbackPorts)))
			}
			gopsPorts = uniquePositiveInts(gopsPorts...)
		default:
			gopsPorts = gopsCandidatePorts(gopsPort, p.settings.DefaultGopsPorts)
			if gopsPort == 0 && len(gopsPorts) > 0 {
				diagnostics = append(diagnostics, fmt.Sprintf("trying default gops ports: %s", formatPorts(gopsPorts)))
			}
		}
	}

	pprofPort := params.PprofPort
	pprofBase := p.settings.PprofBasePath
	if params.PprofBasePath != "" {
		pprofBase = normalizePprofBase(params.PprofBasePath)
	}
	pprofPorts := []int{}
	if usePprof {
		pprofPorts = pprofCandidatePorts(pprofPort, p.settings.DefaultPprofPort, pod.ContainerPorts[container])
		if pprofPort == 0 && len(pod.ContainerPorts[container]) > 0 {
			diagnostics = append(diagnostics, fmt.Sprintf("trying %s on declared container ports: %s", pprofBase, formatPorts(pod.ContainerPorts[container])))
		}
	} else {
		diagnostics = append(diagnostics, "pprof disabled by request")
	}

	var gopsCandidates []portCandidate
	for i, remote := range gopsPorts {
		preferred := 0
		if i == 0 {
			preferred = params.LocalGops
		}
		local, err := pickLocalPort(preferred)
		if err != nil {
			return nil, err
		}
		gopsCandidates = append(gopsCandidates, portCandidate{Remote: remote, Local: local, Source: "gops"})
	}
	var pprofCandidates []portCandidate
	for i, remote := range pprofPorts {
		preferred := 0
		if i == 0 {
			preferred = params.LocalPprof
		}
		local, err := pickLocalPort(preferred)
		if err != nil {
			return nil, err
		}
		pprofCandidates = append(pprofCandidates, portCandidate{Remote: remote, Local: local, Source: "pprof"})
	}
	if len(gopsCandidates) == 0 && len(pprofCandidates) == 0 {
		diagnostics = append(diagnostics, "no diagnostics endpoint candidates found; configure gopsPort, pprofPort, default plugin ports, a readable gopsConfigDir, or containerPorts")
	}

	var forwards []*golangk8s.Forwarder
	closeForwards := func() error {
		var errs []error
		for _, fwd := range forwards {
			if err := fwd.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	sess := NewSession(req.ConfigItemID, pod.Namespace, target.Kind, target.Name, pod.Name, container, closeForwards)
	sess.PID = pid
	sess.PprofBasePath = pprofBase
	sess.Diagnostics = diagnostics

	if match, fwd, ok := firstWorkingForward(ctx, restCfg, pod.Namespace, pod.Name, gopsCandidates, func(ctx context.Context, port int) bool {
		return probeGops(ctx, port)
	}); ok {
		forwards = append(forwards, fwd)
		sess.GopsRemote = match.Remote
		sess.GopsLocal = match.Local
		sess.GopsAvailable = true
		if winnerPID, ok := gopsPortPID[match.Remote]; ok {
			sess.PID = winnerPID
		}
	}
	if match, fwd, ok := firstWorkingForward(ctx, restCfg, pod.Namespace, pod.Name, pprofCandidates, func(ctx context.Context, port int) bool {
		return probePprof(ctx, port, pprofBase)
	}); ok {
		forwards = append(forwards, fwd)
		sess.PprofRemote = match.Remote
		sess.PprofLocal = match.Local
		sess.PprofAvailable = true
	}
	if len(gopsCandidates) > 0 && !sess.GopsAvailable {
		sess.Diagnostics = append(sess.Diagnostics, fmt.Sprintf("no gops agent responded on candidate ports: %s", formatCandidatePorts(gopsCandidates)))
	}
	if len(pprofCandidates) > 0 && !sess.PprofAvailable {
		sess.Diagnostics = append(sess.Diagnostics, fmt.Sprintf("no pprof index responded at %s on candidate ports: %s", pprofBase, formatCandidatePorts(pprofCandidates)))
	}
	p.sessions.Add(sess)
	return sess.Snapshot(), nil
}

func (p *GolangPlugin) createTarget(ctx context.Context, req sdk.InvokeCtx, params SessionCreateParams) (TargetRef, error) {
	if params.Pod != "" {
		ns := params.Namespace
		if ns == "" {
			base, err := targetFromConfig(ctx, req.Host, req.ConfigItemID)
			if err != nil {
				return TargetRef{}, err
			}
			ns = base.Namespace
		}
		return TargetRef{Namespace: ns, Kind: "pod", Name: params.Pod}, nil
	}
	if params.Kind != "" && params.Name != "" && params.Namespace != "" {
		return TargetRef{Namespace: params.Namespace, Kind: normalizeKind(params.Kind), Name: params.Name}, nil
	}
	base, err := targetFromConfig(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return TargetRef{}, err
	}
	if params.Kind != "" && params.Name != "" {
		base.Kind = normalizeKind(params.Kind)
		base.Name = params.Name
	}
	if params.Namespace != "" {
		base.Namespace = params.Namespace
	}
	return base, nil
}

func (p *GolangPlugin) sessionDelete(_ context.Context, req sdk.InvokeCtx) (any, error) {
	var params SessionDeleteParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if params.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if _, err := p.getSessionForConfig(params.ID, req.ConfigItemID); err != nil {
		return nil, err
	}
	removed, err := p.sessions.Remove(params.ID)
	if !removed {
		return nil, fmt.Errorf("session %q not found", params.ID)
	}
	p.profiles.RemoveSession(params.ID)
	return map[string]any{"deleted": true, "id": params.ID}, err
}

func (p *GolangPlugin) runtimeSnapshot(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	sess, err := p.sessionFromRequest(req)
	if err != nil {
		return nil, err
	}
	out := RuntimeSnapshot{SessionID: sess.ID}
	if !sess.GopsAvailable {
		out.Error = fmt.Sprintf("session %s has no reachable gops agent", sess.ID)
		return out, nil
	}
	client := GopsClient{Addr: gopsAddr(sess), Timeout: 10 * time.Second}
	var errs []string

	version, err := client.Version(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("version: %v", err))
	} else {
		out.Version = version
	}
	stats, err := client.Stats(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("stats: %v", err))
	} else {
		out.Stats = stats
	}
	memStats, err := client.MemStats(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("memstats: %v", err))
	} else {
		out.MemStats = memStats
	}

	if len(errs) > 0 {
		out.Error = "gops runtime errors: " + strings.Join(errs, "; ")
	}
	if out.Version == "" && out.Stats == "" && out.MemStats == "" && out.Error == "" {
		out.Error = "gops agent returned no runtime data"
	}
	return out, nil
}

func (p *GolangPlugin) goroutines(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	sess, err := p.sessionFromRequest(req)
	if err != nil {
		return nil, err
	}
	if sess.GopsAvailable {
		dump, err := (GopsClient{Addr: gopsAddr(sess), Timeout: 15 * time.Second}).Stack(ctx)
		if err != nil {
			return GoroutineSnapshot{SessionID: sess.ID, Source: "gops", Error: fmt.Sprintf("gops goroutine error: %v", err)}, nil
		}
		return GoroutineSnapshot{SessionID: sess.ID, Source: "gops", Dump: dump}, nil
	}
	if sess.PprofAvailable {
		body, err := getPprof(ctx, sess, "goroutine?debug=2")
		if err != nil {
			return GoroutineSnapshot{SessionID: sess.ID, Source: "pprof", Error: fmt.Sprintf("pprof goroutine error: %v", err)}, nil
		}
		return GoroutineSnapshot{SessionID: sess.ID, Source: "pprof", Dump: string(body)}, nil
	}
	return GoroutineSnapshot{SessionID: sess.ID, Error: fmt.Sprintf("session %s has neither gops nor pprof available", sess.ID)}, nil
}

func (p *GolangPlugin) profileCollect(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params ProfileCollectParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	sess, err := p.getSessionForConfig(params.SessionID, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	kind := normalizeProfileKind(params.Kind)
	if kind == "" {
		return nil, fmt.Errorf("profile kind must be heap, cpu, or trace")
	}
	preference := normalizeProfileSource(params.Source)
	seconds := params.Seconds
	if seconds <= 0 || seconds > p.settings.MaxProfileSec {
		seconds = p.settings.MaxProfileSec
	}
	data, source, err := collectProfileWithSource(ctx, sess, kind, seconds, preference)
	if err != nil {
		return nil, err
	}
	run, _ := NewProfileRun(sess.ID, kind, preference, seconds)
	run.MarkDone(data, source, nil)
	p.profiles.Add(run)
	return ProfileResult{
		SessionID: sess.ID,
		RunID:     run.ID,
		Kind:      kind,
		Source:    source,
		Bytes:     len(data),
		URL:       fmt.Sprintf("profiles/%s/%s", sess.ID, run.ID),
		Seconds:   seconds,
	}, nil
}

func (p *GolangPlugin) profileStart(_ context.Context, req sdk.InvokeCtx) (any, error) {
	var params ProfileRunParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	sess, err := p.getSessionForConfig(params.SessionID, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	kind := normalizeProfileKind(params.Kind)
	if kind == "" {
		return nil, fmt.Errorf("profile kind must be heap, cpu, or trace")
	}
	preference := normalizeProfileSource(params.Source)
	seconds := params.Seconds
	if seconds <= 0 || seconds > p.settings.MaxProfileSec {
		seconds = p.settings.MaxProfileSec
	}
	run, runCtx := NewProfileRun(sess.ID, kind, preference, seconds)
	p.profiles.Add(run)
	go func() {
		timeout := time.Duration(seconds+15) * time.Second
		if timeout < 45*time.Second {
			timeout = 45 * time.Second
		}
		ctx, cancel := context.WithTimeout(runCtx, timeout)
		defer cancel()
		data, source, err := collectProfileWithSource(ctx, sess, kind, seconds, preference)
		run.MarkDone(data, source, err)
	}()
	return run.Snapshot(), nil
}

func (p *GolangPlugin) profileStatus(_ context.Context, req sdk.InvokeCtx) (any, error) {
	var params ProfileRunIDParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if params.RunID == "" {
		return nil, fmt.Errorf("runId is required")
	}
	run, ok := p.profiles.Get(params.RunID)
	if !ok {
		return nil, fmt.Errorf("profile run %q not found", params.RunID)
	}
	if params.SessionID != "" && run.SessionID != params.SessionID {
		return nil, fmt.Errorf("profile run %q does not belong to session %q", params.RunID, params.SessionID)
	}
	if _, err := p.getSessionForConfig(run.SessionID, req.ConfigItemID); err != nil {
		return nil, err
	}
	return run.Snapshot(), nil
}

func (p *GolangPlugin) profileStop(_ context.Context, req sdk.InvokeCtx) (any, error) {
	var params ProfileRunIDParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if params.RunID == "" {
		return nil, fmt.Errorf("runId is required")
	}
	run, ok := p.profiles.Get(params.RunID)
	if !ok {
		return nil, fmt.Errorf("profile run %q not found", params.RunID)
	}
	if params.SessionID != "" && run.SessionID != params.SessionID {
		return nil, fmt.Errorf("profile run %q does not belong to session %q", params.RunID, params.SessionID)
	}
	if _, err := p.getSessionForConfig(run.SessionID, req.ConfigItemID); err != nil {
		return nil, err
	}
	run.Stop()
	return run.Snapshot(), nil
}

func (p *GolangPlugin) profileRunsList(_ context.Context, req sdk.InvokeCtx) (any, error) {
	var params SessionIDParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if params.SessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if _, err := p.getSessionForConfig(params.SessionID, req.ConfigItemID); err != nil {
		return nil, err
	}
	return p.profiles.List(params.SessionID), nil
}

func (p *GolangPlugin) sessionFromRequest(req sdk.InvokeCtx) (*Session, error) {
	var params SessionIDParams
	if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode params: %w", err)
	}
	if params.SessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	return p.getSessionForConfig(params.SessionID, req.ConfigItemID)
}

func (p *GolangPlugin) getSessionForConfig(sessionID, configItemID string) (*Session, error) {
	sess, ok := p.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	if !sessionMatchesConfig(sess, configItemID) {
		return nil, fmt.Errorf("session %q does not belong to the current config item", sessionID)
	}
	return sess, nil
}

func sessionMatchesConfig(sess *Session, configItemID string) bool {
	return configItemID == "" || sess.ConfigItemID == configItemID
}

func selectPodContainer(pods []RunningPod, podName, container string) (RunningPod, string, error) {
	if len(pods) == 0 {
		return RunningPod{}, "", fmt.Errorf("no ready pods found")
	}
	for _, pod := range pods {
		if podName != "" && pod.Name != podName {
			continue
		}
		if container == "" && len(pod.Containers) == 1 {
			container = pod.Containers[0]
		}
		if container == "" {
			return pod, "", fmt.Errorf("container is required for pod %s because it has %d containers", pod.Name, len(pod.Containers))
		}
		for _, c := range pod.Containers {
			if c == container {
				return pod, container, nil
			}
		}
		return pod, "", fmt.Errorf("container %q not found in pod %s", container, pod.Name)
	}
	return RunningPod{}, "", fmt.Errorf("pod %q not found in ready target pods", podName)
}

func probeGops(ctx context.Context, port int) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	version, err := (GopsClient{Addr: fmt.Sprintf("127.0.0.1:%d", port), Timeout: 3 * time.Second}).Version(probeCtx)
	if err != nil {
		return false
	}
	version = strings.TrimSpace(version)
	return version != "" && strings.HasPrefix(version, "go")
}

func probePprof(ctx context.Context, port int, base string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s/", port, normalizePprofBase(base)), nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func firstWorkingForward(ctx context.Context, restCfg *rest.Config, namespace, pod string, candidates []portCandidate, probe func(context.Context, int) bool) (portCandidate, *golangk8s.Forwarder, bool) {
	for _, candidate := range candidates {
		startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		fwd, ready, err := golangk8s.StartPortForward(restCfg, namespace, pod, []golangk8s.PortMapping{{LocalPort: candidate.Local, RemotePort: candidate.Remote}}, io.Discard, io.Discard)
		if err != nil {
			cancel()
			continue
		}
		if err := fwd.Ready(startCtx, ready); err != nil {
			cancel()
			_ = fwd.Close()
			continue
		}
		cancel()
		if probe(ctx, candidate.Local) {
			return candidate, fwd, true
		}
		_ = fwd.Close()
	}
	return portCandidate{}, nil, false
}

func collectProfile(ctx context.Context, sess *Session, kind string, seconds int) ([]byte, string, error) {
	return collectProfileWithSource(ctx, sess, kind, seconds, "auto")
}

func collectProfileWithSource(ctx context.Context, sess *Session, kind string, seconds int, preference string) ([]byte, string, error) {
	if preference == "" {
		preference = "auto"
	}
	if preference == "pprof" {
		if !sess.PprofAvailable {
			return nil, "", fmt.Errorf("pprof is not available for session %s", sess.ID)
		}
		return collectPprofProfile(ctx, sess, kind, seconds)
	}
	if preference == "gops" {
		if !sess.GopsAvailable {
			return nil, "", fmt.Errorf("gops is not available for session %s", sess.ID)
		}
		return collectGopsProfile(ctx, sess, kind)
	}
	if sess.PprofAvailable {
		data, source, err := collectPprofProfile(ctx, sess, kind, seconds)
		if err == nil {
			return data, source, nil
		}
		if !sess.GopsAvailable {
			return nil, "", err
		}
	}
	if !sess.GopsAvailable {
		return nil, "", fmt.Errorf("session %s has neither pprof nor gops available", sess.ID)
	}
	return collectGopsProfile(ctx, sess, kind)
}

func collectPprofProfile(ctx context.Context, sess *Session, kind string, seconds int) ([]byte, string, error) {
	path := kind
	if kind == "cpu" {
		path = fmt.Sprintf("profile?seconds=%d", seconds)
	}
	if kind == "trace" {
		path = fmt.Sprintf("trace?seconds=%d", seconds)
	}
	data, err := getPprof(ctx, sess, path)
	return data, "pprof", err
}

func collectGopsProfile(ctx context.Context, sess *Session, kind string) ([]byte, string, error) {
	client := GopsClient{Addr: gopsAddr(sess), Timeout: 45 * time.Second}
	switch kind {
	case "heap":
		data, err := client.Heap(ctx)
		return data, "gops", err
	case "cpu":
		data, err := client.CPU(ctx)
		return data, "gops", err
	case "trace":
		data, err := client.Trace(ctx)
		return data, "gops", err
	default:
		return nil, "", fmt.Errorf("unsupported profile kind %q", kind)
	}
}

func getPprof(ctx context.Context, sess *Session, path string) ([]byte, error) {
	if !sess.PprofAvailable || sess.PprofLocal == 0 {
		return nil, fmt.Errorf("pprof is not available for session %s", sess.ID)
	}
	base := strings.TrimRight(normalizePprofBase(sess.PprofBasePath), "/")
	path = strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s/%s", sess.PprofLocal, base, path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pprof %s returned HTTP %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func normalizePprofBase(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/debug/pprof"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}

func normalizeProfileKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "heap", "pprof-heap":
		return "heap"
	case "cpu", "profile", "pprof-cpu":
		return "cpu"
	case "trace":
		return "trace"
	default:
		return ""
	}
}

func normalizeProfileSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "auto":
		return "auto"
	case "pprof":
		return "pprof"
	case "gops":
		return "gops"
	default:
		return "auto"
	}
}

func gopsCandidatePorts(discovered int, defaults []int) []int {
	if discovered > 0 {
		return []int{discovered}
	}
	return uniquePositiveInts(defaults...)
}

func pprofCandidatePorts(explicit, configured int, containerPorts []int) []int {
	if explicit > 0 {
		return []int{explicit}
	}
	out := []int{}
	if configured > 0 {
		out = append(out, configured)
	}
	out = append(out, containerPorts...)
	return uniquePositiveInts(out...)
}

func uniquePositiveInts(values ...int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func formatPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, port := range uniquePositiveInts(ports...) {
		parts = append(parts, fmt.Sprint(port))
	}
	return strings.Join(parts, ", ")
}

func formatGopsCandidates(procs []GopsProcess) string {
	parts := make([]string, 0, len(procs))
	for _, proc := range procs {
		parts = append(parts, fmt.Sprintf("pid=%d port=%d", proc.PID, proc.Port))
	}
	return strings.Join(parts, ", ")
}

func appendPrefixedDiagnostics(dst []string, prefix string, lines []string) []string {
	for _, line := range lines {
		if line == "" {
			continue
		}
		dst = append(dst, prefix+line)
	}
	return dst
}

func formatCandidatePorts(candidates []portCandidate) string {
	ports := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		ports = append(ports, candidate.Remote)
	}
	return formatPorts(ports)
}
