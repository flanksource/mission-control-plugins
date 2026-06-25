package main

import "strings"

// eventMatchesTarget reports whether an event came from a workload that
// matches the trace target. It relies on the K8s enrichment that the gadget
// service's KubeManager adds to events from container processes. Host
// processes and untracked containers have no K8s metadata and are dropped
// when the target is scoped to a specific workload.
func eventMatchesTarget(event TraceEvent, target TraceTarget) bool {
	if event.Error != "" {
		return true
	}
	if event.Data == nil {
		return true
	}
	if !targetHasScope(target) {
		return true
	}
	k8s, _ := event.Data["k8s"].(map[string]any)
	if k8s == nil {
		return false
	}
	if target.Namespace != "" && stringValue(k8s, "namespace") != target.Namespace {
		return false
	}
	if target.Pod != "" && stringValue(k8s, "podName") != target.Pod {
		return false
	}
	if target.Container != "" && stringValue(k8s, "containerName") != target.Container {
		return false
	}
	if len(target.Selector) > 0 {
		if !serializedLabelsMatch(stringValue(k8s, "podLabels"), target.Selector) {
			return false
		}
	}
	return true
}

func targetHasScope(target TraceTarget) bool {
	return target.Namespace != "" || target.Pod != "" || target.Container != "" || len(target.Selector) > 0
}

func stringValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// serializedLabelsMatch reports whether the comma-joined "k=v,k2=v2" string
// the gadget service emits in k8s.podLabels satisfies every entry in the
// desired selector. Values are compared as strings; an empty serialization
// is treated as a miss.
func serializedLabelsMatch(serialized string, selector map[string]string) bool {
	if serialized == "" {
		return false
	}
	actual := parseLabelString(serialized)
	for key, want := range selector {
		if actual[key] != want {
			return false
		}
	}
	return true
}

func parseLabelString(serialized string) map[string]string {
	out := make(map[string]string)
	for _, pair := range strings.Split(serialized, ",") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}
