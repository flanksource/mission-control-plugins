package main

import (
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("event target filtering", func() {
	ginkgo.It("accepts every event when the target has no scope", func() {
		target := TraceTarget{}
		Expect(eventMatchesTarget(TraceEvent{}, target)).To(BeTrue())
		Expect(eventMatchesTarget(TraceEvent{Data: map[string]any{"k8s": map[string]any{"namespace": "x"}}}, target)).To(BeTrue())
	})

	ginkgo.It("drops host processes that lack k8s metadata when targeting a pod", func() {
		target := TraceTarget{Namespace: "default", Pod: "api-1", Kind: "pod"}
		// Real top_process host-process event: has process fields but no k8s block.
		hostProcess := TraceEvent{Data: map[string]any{"pid": float64(817), "comm": "sshd"}}
		Expect(eventMatchesTarget(hostProcess, target)).To(BeFalse())
		Expect(eventMatchesTarget(TraceEvent{Data: map[string]any{"k8s": map[string]any{}}}, target)).To(BeFalse())
	})

	ginkgo.It("keeps events that match the targeted pod and container", func() {
		target := TraceTarget{Namespace: "default", Pod: "api-1", Container: "api", Kind: "pod"}
		matching := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace":     "default",
				"podName":       "api-1",
				"containerName": "api",
			},
		}}
		Expect(eventMatchesTarget(matching, target)).To(BeTrue())
	})

	ginkgo.It("drops events from other pods in the same namespace", func() {
		target := TraceTarget{Namespace: "default", Pod: "api-1", Kind: "pod"}
		otherPod := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace":     "default",
				"podName":       "api-2",
				"containerName": "api",
			},
		}}
		Expect(eventMatchesTarget(otherPod, target)).To(BeFalse())
	})

	ginkgo.It("drops events from other namespaces", func() {
		target := TraceTarget{Namespace: "default", Pod: "api-1", Kind: "pod"}
		otherNS := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace":     "other",
				"podName":       "api-1",
				"containerName": "api",
			},
		}}
		Expect(eventMatchesTarget(otherNS, target)).To(BeFalse())
	})

	ginkgo.It("drops events from sibling containers in the same pod", func() {
		target := TraceTarget{Namespace: "default", Pod: "api-1", Container: "api", Kind: "pod"}
		sibling := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace":     "default",
				"podName":       "api-1",
				"containerName": "sidecar",
			},
		}}
		Expect(eventMatchesTarget(sibling, target)).To(BeFalse())
	})

	ginkgo.It("accepts events whose pod labels satisfy the workload selector", func() {
		target := TraceTarget{
			Namespace: "default",
			Kind:      "deployment",
			Name:      "api",
			Selector:  map[string]string{"app": "api", "tier": "backend"},
		}
		matched := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace": "default",
				"podName":   "api-1",
				// Real gadget-service output: K8sPodLabelsAsString joined as "k=v,k2=v2".
				"podLabels": "app=api,tier=backend,version=v1",
			},
		}}
		notMatched := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace": "default",
				"podName":   "worker-1",
				"podLabels": "app=worker,tier=backend",
			},
		}}
		partialMatch := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace": "default",
				"podName":   "api-2",
				"podLabels": "app=api", // missing the "tier" key
			},
		}}
		empty := TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace": "default",
				"podName":   "api-3",
				"podLabels": "",
			},
		}}
		Expect(eventMatchesTarget(matched, target)).To(BeTrue())
		Expect(eventMatchesTarget(notMatched, target)).To(BeFalse())
		Expect(eventMatchesTarget(partialMatch, target)).To(BeFalse())
		Expect(eventMatchesTarget(empty, target)).To(BeFalse())
	})

	ginkgo.It("lets gadget-service log and error events bypass scoping", func() {
		target := TraceTarget{Namespace: "default", Pod: "api-1", Kind: "pod"}
		logEvent := TraceEvent{Raw: "gadgettracermanager: shutting down"}
		errorEvent := TraceEvent{Error: "boom"}
		Expect(eventMatchesTarget(logEvent, target)).To(BeTrue())
		Expect(eventMatchesTarget(errorEvent, target)).To(BeTrue())
	})
})
