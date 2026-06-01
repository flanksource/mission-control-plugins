package main

import (
	"net/http"
	"net/http/httptest"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("HTTP handler", func() {
	ginkgo.It("validates profile paths", func() {
		p := newPlugin()
		sess := NewSession("", "default", "pod", "app", "app-0", "app", nil)
		p.sessions.Add(sess)
		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/profiles?path="+sess.ID+"/unknown", nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPProfiles).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	ginkgo.It("serves completed profile runs from the registry", func() {
		p := newPlugin()
		sess := NewSession("", "default", "pod", "app", "app-0", "app", nil)
		p.sessions.Add(sess)
		run, _ := NewProfileRun(sess.ID, "heap", "pprof", 30)
		run.MarkDone([]byte("profile-bytes"), "pprof", nil)
		p.profiles.Add(run)

		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/profiles?path="+sess.ID+"/"+run.ID, nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPProfiles).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("profile-bytes"))
		Expect(rec.Header().Get("X-Diagnostics-Source")).To(Equal("pprof"))
		Expect(rec.Header().Get("Content-Disposition")).To(ContainSubstring(pluginName))
	})

	ginkgo.It("renders completed profile runs as static SVG", func() {
		p := newPlugin()
		sess := NewSession("", "default", "pod", "app", "app-0", "app", nil)
		p.sessions.Add(sess)
		run, _ := NewProfileRun(sess.ID, "heap", "pprof", 30)
		run.MarkDone(generateHeapProfile(), "pprof", nil)
		p.profiles.Add(run)

		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/profiles?path="+sess.ID+"/"+run.ID+"/flamegraph&si=inuse_space", nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPProfiles).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(Equal("image/svg+xml"))
		Expect(rec.Body.String()).To(ContainSubstring("<svg"))
	})

	ginkgo.It("does not download running profile runs", func() {
		p := newPlugin()
		sess := NewSession("", "default", "pod", "app", "app-0", "app", nil)
		p.sessions.Add(sess)
		run, _ := NewProfileRun(sess.ID, "cpu", "auto", 30)
		p.profiles.Add(run)

		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/profiles?path="+sess.ID+"/"+run.ID, nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPProfiles).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusConflict))
		Expect(rec.Body.String()).To(ContainSubstring("not completed"))
	})
})

func httpOp(p *GolangPlugin, name string) http.Handler {
	for _, op := range p.Operations() {
		if op.Def.Name == name {
			return op.HTTPHandler
		}
	}
	return http.NotFoundHandler()
}
