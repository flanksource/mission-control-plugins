package main

import (
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("gops discovery parsing", func() {
	ginkgo.It("parses pid port and command rows", func() {
		got := parseGopsDiscovery("pid=12 port=4567 cmd=/app/server --flag\npid=x port=1\npid=13 port=8901 cmd=\n")
		Expect(got).To(HaveLen(2))
		Expect(got[0].PID).To(Equal(12))
		Expect(got[0].Port).To(Equal(4567))
		Expect(got[0].Command).To(Equal("/app/server --flag"))
		Expect(got[1].PID).To(Equal(13))
		Expect(got[1].Port).To(Equal(8901))
	})

	ginkgo.It("selects a requested pid", func() {
		procs := []GopsProcess{{PID: 10, Port: 1000}, {PID: 20, Port: 2000}}
		got, ok := selectGopsProcess(procs, 20)
		Expect(ok).To(BeTrue())
		Expect(got.Port).To(Equal(2000))
	})

	ginkgo.It("prefers pid 1 over plugin helper processes", func() {
		procs := []GopsProcess{
			{PID: 23, Port: 39217, Command: "/root/.mission-control/plugins/golang-mc-plugin"},
			{PID: 1, Port: 33055, Command: "/app/incident-commander serve"},
			{PID: 38, Port: 41291, Command: "/root/.mission-control/plugins/kubernetes-logs-mc-plugin"},
		}
		got, ok := selectGopsProcessForTarget(procs, 0, TargetRef{Name: "mission-control"})
		Expect(ok).To(BeTrue())
		Expect(got.PID).To(Equal(1))
		Expect(got.Port).To(Equal(33055))
	})

	ginkgo.It("uses the target name when pid 1 is not present", func() {
		procs := []GopsProcess{
			{PID: 20, Port: 2000, Command: "/app/worker serve"},
			{PID: 30, Port: 3000, Command: "/app/api serve"},
		}
		got, ok := selectGopsProcessForTarget(procs, 0, TargetRef{Name: "api"})
		Expect(ok).To(BeTrue())
		Expect(got.PID).To(Equal(30))
	})
})
