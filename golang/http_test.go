package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("HTTP handler", func() {
	ginkgo.It("returns a useful error for missing pprof session", func() {
		p := newPlugin()
		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/pprof?path=missing", nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPPprof).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusNotFound))
		Expect(rec.Body.String()).To(ContainSubstring("session not found"))
	})

	ginkgo.It("validates profile paths", func() {
		p := newPlugin()
		sess := NewSession("", "default", "pod", "app", "app-0", "app", nil)
		p.sessions.Add(sess)
		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/profiles?path="+sess.ID+"/unknown", nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPProfiles).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	ginkgo.It("proxies the pprof index with a trailing slash and rewrites relative links", func() {
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			Expect(r.URL.Query().Get("path")).To(BeEmpty())
			Expect(r.URL.Query().Get("config_id")).To(BeEmpty())
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<a href='heap?debug=1'>heap</a><a href="goroutine?debug=2">goroutine</a>`)
		}))
		defer server.Close()

		p := newPlugin()
		sess := NewSession("config-a", "default", "pod", "app", "app-0", "app", nil)
		sess.PprofAvailable = true
		sess.PprofLocal = serverPort(server)
		p.sessions.Add(sess)
		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/pprof?config_id=config-a&path="+sess.ID+"/", nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPPprof).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(gotPath).To(Equal("/debug/pprof/"))
		Expect(rec.Body.String()).To(ContainSubstring("href='?config_id=config-a&debug=1&path=" + sess.ID + "%2Fheap'"))
		Expect(rec.Body.String()).To(ContainSubstring("href=\"?config_id=config-a&debug=2&path=" + sess.ID + "%2Fgoroutine\""))
	})

	ginkgo.It("forwards pprof link query parameters without plugin routing parameters", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/debug/pprof/heap"))
			Expect(r.URL.Query().Get("debug")).To(Equal("1"))
			Expect(r.URL.Query().Get("path")).To(BeEmpty())
			Expect(r.URL.Query().Get("config_id")).To(BeEmpty())
			_, _ = w.Write([]byte("heap"))
		}))
		defer server.Close()

		p := newPlugin()
		sess := NewSession("config-a", "default", "pod", "app", "app-0", "app", nil)
		sess.PprofAvailable = true
		sess.PprofLocal = serverPort(server)
		p.sessions.Add(sess)
		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/pprof?config_id=config-a&path="+sess.ID+"/heap&debug=1", nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPPprof).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("heap"))
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

func serverPort(server *httptest.Server) int {
	_, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	Expect(err).ToNot(HaveOccurred())
	port, err := strconv.Atoi(portStr)
	Expect(err).ToNot(HaveOccurred())
	return port
}

func httpOp(p *GolangPlugin, name string) http.Handler {
	for _, op := range p.Operations() {
		if op.Def.Name == name {
			return op.HTTPHandler
		}
	}
	return http.NotFoundHandler()
}
