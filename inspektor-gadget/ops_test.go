package main

import (
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("trace target resolution", func() {
	ginkgo.It("keeps pod targets filtered to the selected pod", func() {
		target, err := resolveTraceTarget(TraceTarget{
			Namespace: "default",
			Kind:      "pod",
			Name:      "api-123",
		}, []RunningPod{{
			Namespace:  "default",
			Name:       "api-123",
			Node:       "node-a",
			Containers: []string{"api"},
			Labels:     map[string]string{"app": "api", "pod-template-hash": "123"},
		}}, map[string]string{"app": "api", "pod-template-hash": "123"})

		Expect(err).ToNot(HaveOccurred())
		Expect(target.Pod).To(Equal("api-123"))
		Expect(target.Node).To(Equal("node-a"))
		Expect(target.Container).To(Equal("api"))
		Expect(target.Selector).To(BeNil())
	})

	ginkgo.It("keeps workload targets filtered by selector across pod nodes", func() {
		target, err := resolveTraceTarget(TraceTarget{
			Namespace: "default",
			Kind:      "deployment",
			Name:      "api",
		}, []RunningPod{
			{Namespace: "default", Name: "api-123", Node: "node-a", Containers: []string{"api"}, Labels: map[string]string{"app": "api", "pod-template-hash": "123"}},
			{Namespace: "default", Name: "api-456", Node: "node-b", Containers: []string{"api"}, Labels: map[string]string{"app": "api", "pod-template-hash": "456"}},
		}, map[string]string{"app": "api"})

		Expect(err).ToNot(HaveOccurred())
		Expect(target.Pod).To(BeEmpty())
		Expect(target.Selector).To(Equal(map[string]string{"app": "api"}))
		Expect(target.Node).To(Equal("node-a,node-b"))
		Expect(target.Container).To(Equal("api"))
	})

	ginkgo.It("matches selector based sessions to current workload pods", func() {
		matches := traceTargetInPods(TraceTarget{
			Namespace: "default",
			Selector:  map[string]string{"app": "api"},
		}, []RunningPod{{
			Namespace:  "default",
			Name:       "api-123",
			Containers: []string{"api"},
			Labels:     map[string]string{"app": "api", "pod-template-hash": "123"},
		}})

		Expect(matches).To(BeTrue())
	})
})
