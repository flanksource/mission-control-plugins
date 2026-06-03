package main

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/flanksource/incident-commander/plugin/sdk"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = ginkgo.Describe("http", func() {
	ginkgo.It("passes config_id query parameters into operation invoke context", func() {
		plugin := newPlugin()
		handler := plugin.httpInvoke("trace-list", func(_ context.Context, req sdk.InvokeCtx) (any, error) {
			Expect(req.ConfigItemID).To(Equal("config-123"))
			return map[string]string{"ok": "true"}, nil
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/trace-list?config_id=config-123", nil)
		handler.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
	})

	ginkgo.It("returns permission errors as HTTP 403 for the UI", func() {
		plugin := newPlugin()
		handler := plugin.httpInvoke("status", func(context.Context, sdk.InvokeCtx) (any, error) {
			return nil, status.Error(codes.PermissionDenied, "cannot read connection")
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/status", nil)
		handler.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusForbidden))
		Expect(rec.Body.String()).To(ContainSubstring("cannot read connection"))
	})

	ginkgo.It("exports buffered events as an attachment", func() {
		plugin := newPlugin()
		gadget, ok := gadgetByID("trace_exec", defaultIGTag)
		Expect(ok).To(BeTrue())
		session, _ := newTraceSession(gadget, TraceTarget{Namespace: "default", Pod: "pod"}, nil, TraceDiagnostics{}, 10)
		session.AddEvent(TraceEvent{Raw: `{"comm":"sh"}`})
		plugin.sessions.Add(session)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/sessions/"+session.ID+"/export", nil)
		plugin.HTTPHandler().ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/x-ndjson"))
		Expect(rec.Header().Get("Content-Disposition")).To(ContainSubstring(session.ID + ".ndjson"))
		Expect(rec.Body.String()).To(ContainSubstring(`"raw":"{\"comm\":\"sh\"}"`))
	})
})
