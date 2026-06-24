package main

import (
	"strings"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("gops discovery parsing", func() {
	ginkgo.It("parses pid port command rows and diagnostics", func() {
		result := parseGopsDiscoveryResult("diag=checking /tmp/gops\npid=12 port=4567 cmd=/app/server --flag\npid=x port=1\npid=13 port=8901 cmd=\n")
		got := result.Processes
		Expect(got).To(HaveLen(2))
		Expect(got[0].PID).To(Equal(12))
		Expect(got[0].Port).To(Equal(4567))
		Expect(got[0].Command).To(Equal("/app/server --flag"))
		Expect(got[1].PID).To(Equal(13))
		Expect(got[1].Port).To(Equal(8901))
		Expect(result.Diagnostics).To(Equal([]string{"checking /tmp/gops"}))
	})

	ginkgo.It("keeps the discovery script exit status successful", func() {
		Expect(strings.TrimSpace(buildGopsDiscoveryScript(nil))).To(HaveSuffix("exit 0"))
	})

	ginkgo.It("selects a requested pid", func() {
		procs := []GopsProcess{{PID: 10, Port: 1000}, {PID: 20, Port: 2000}}
		got, ok := selectGopsProcess(procs, 20)
		Expect(ok).To(BeTrue())
		Expect(got.Port).To(Equal(2000))
	})

	ginkgo.It("selects the lowest pid when no pid is requested", func() {
		procs := []GopsProcess{
			{PID: 42, Port: 6061, Command: "/app/worker serve"},
			{PID: 17, Port: 39217, Command: "/app/helper serve"},
			{PID: 30, Port: 3000, Command: "/app/api serve"},
		}
		got, ok := selectGopsProcess(procs, 0)
		Expect(ok).To(BeTrue())
		Expect(got.PID).To(Equal(17))
		Expect(got.Port).To(Equal(39217))
	})
})
