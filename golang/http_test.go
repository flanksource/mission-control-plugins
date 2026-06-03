package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/pprof"

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

	ginkgo.It("serves flamegraph data for completed profile runs", func() {
		p := newPlugin()
		sess := NewSession("", "default", "pod", "app", "app-0", "app", nil)
		p.sessions.Add(sess)
		run, _ := NewProfileRun(sess.ID, "heap", "pprof", 30)
		run.MarkDone(generateHeapProfile(), "pprof", nil)
		p.profiles.Add(run)

		req := httptest.NewRequest(http.MethodGet, "/__mc/operations/profiles?path="+sess.ID+"/"+run.ID+"/flamegraph-data", nil)
		rec := httptest.NewRecorder()

		httpOp(p, OpHTTPProfiles).ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/json"))
		var data FlamegraphData
		Expect(json.Unmarshal(rec.Body.Bytes(), &data)).To(Succeed())
		Expect(data.SampleType).ToNot(BeEmpty())
		Expect(data.Total).To(BeNumerically(">", 0))
		Expect(data.Root.Children).ToNot(BeEmpty())
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

func generateHeapProfile() []byte {
	for range 1000 {
		_ = make([]byte, 1024)
	}
	runtime.GC()
	tmp, err := os.CreateTemp("", "profile-test-*.pprof")
	if err != nil {
		return nil
	}
	defer func() {
		Expect(tmp.Close()).To(Succeed())
		Expect(os.Remove(tmp.Name())).To(Succeed())
	}()
	if err := pprof.Lookup("heap").WriteTo(tmp, 0); err != nil {
		return nil
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		return nil
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil
	}
	return data
}
