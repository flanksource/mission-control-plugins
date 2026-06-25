package main

import (
	"errors"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("sessions", func() {
	ginkgo.It("keeps only the bounded event window", func() {
		gadget, ok := gadgetByID("trace_exec", defaultIGTag)
		Expect(ok).To(BeTrue())
		session, _ := newTraceSession(gadget, TraceTarget{}, nil, TraceDiagnostics{}, 2)

		session.AddEvent(TraceEvent{Raw: "one"})
		session.AddEvent(TraceEvent{Raw: "two"})
		session.AddEvent(TraceEvent{Raw: "three"})

		events := session.Events()
		Expect(events).To(HaveLen(2))
		Expect(events[0].Raw).To(Equal("two"))
		Expect(events[1].Raw).To(Equal("three"))
		Expect(session.Snapshot().EventCount).To(Equal(int64(3)))
	})

	ginkgo.It("records terminal errors", func() {
		gadget, _ := gadgetByID("trace_exec", defaultIGTag)
		session, _ := newTraceSession(gadget, TraceTarget{Namespace: "default", Pod: "pod"}, nil, TraceDiagnostics{}, 10)
		session.MarkRunning()
		session.MarkDone(errors.New("boom"))

		snapshot := session.Snapshot()
		Expect(snapshot.State).To(Equal("failed"))
		Expect(snapshot.Error).To(Equal("boom"))
		Expect(snapshot.StoppedAt).ToNot(BeNil())
	})

	ginkgo.It("drops events that don't match the session target", func() {
		gadget, ok := gadgetByID("top_process", defaultIGTag)
		Expect(ok).To(BeTrue())
		target := TraceTarget{Namespace: "default", Pod: "api-1", Container: "api", Kind: "pod"}
		session, _ := newTraceSession(gadget, target, nil, TraceDiagnostics{}, 100)

		// Event from the targeted pod's container: kept.
		session.AddEvent(TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace":     "default",
				"podName":       "api-1",
				"containerName": "api",
				"pid":           float64(1),
			},
		}})
		// Host process (no k8s metadata): dropped.
		session.AddEvent(TraceEvent{Data: map[string]any{
			"pid":  float64(817),
			"comm": "sshd",
		}})
		// Process in a different pod on the same node: dropped.
		session.AddEvent(TraceEvent{Data: map[string]any{
			"k8s": map[string]any{
				"namespace":     "default",
				"podName":       "worker-1",
				"containerName": "worker",
				"pid":           float64(42),
			},
		}})
		// Plugin-level error events bypass the filter so they reach the UI.
		session.AddEvent(TraceEvent{Error: "boom"})
		// Gadget-service log lines (no Data, just Raw) also bypass the filter
		// so diagnostics from the gadget aren't silently swallowed.
		session.AddEvent(TraceEvent{Raw: "gadgettracermanager: shutting down"})

		events := session.Events()
		Expect(events).To(HaveLen(3))
		Expect(events[0].Data["k8s"]).ToNot(BeNil())
		Expect(events[1].Error).To(Equal("boom"))
		Expect(events[2].Raw).To(Equal("gadgettracermanager: shutting down"))
		Expect(session.Snapshot().EventCount).To(Equal(int64(3)))
	})

	ginkgo.It("lists sessions in a stable newest-first order", func() {
		gadget, _ := gadgetByID("trace_exec", defaultIGTag)
		older, _ := newTraceSession(gadget, TraceTarget{Pod: "pod-a"}, nil, TraceDiagnostics{}, 10)
		newer, _ := newTraceSession(gadget, TraceTarget{Pod: "pod-b"}, nil, TraceDiagnostics{}, 10)
		sameTimeA, _ := newTraceSession(gadget, TraceTarget{Pod: "pod-c"}, nil, TraceDiagnostics{}, 10)
		sameTimeB, _ := newTraceSession(gadget, TraceTarget{Pod: "pod-d"}, nil, TraceDiagnostics{}, 10)
		older.ID = "older"
		newer.ID = "newer"
		sameTimeA.ID = "same-a"
		sameTimeB.ID = "same-b"
		base := time.Now()
		older.StartedAt = base.Add(-time.Minute)
		newer.StartedAt = base
		sameTimeA.StartedAt = base.Add(-30 * time.Second)
		sameTimeB.StartedAt = sameTimeA.StartedAt

		registry := NewSessionRegistry(10)
		registry.Add(older)
		registry.Add(sameTimeB)
		registry.Add(newer)
		registry.Add(sameTimeA)

		sessions := registry.List()
		Expect([]string{sessions[0].ID, sessions[1].ID, sessions[2].ID, sessions[3].ID}).To(Equal([]string{"newer", "same-a", "same-b", "older"}))
	})
})
