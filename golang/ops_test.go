package main

import (
	"github.com/flanksource/incident-commander/plugin/sdk"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("sessions", func() {
	ginkgo.It("lists only sessions for the current config item", func() {
		p := newPlugin()
		current := NewSession("config-a", "default", "pod", "app-a", "app-a-0", "app", nil)
		other := NewSession("config-b", "default", "pod", "app-b", "app-b-0", "app", nil)
		p.sessions.Add(current)
		p.sessions.Add(other)

		result, err := p.sessionsList(ginkgo.GinkgoT().Context(), sdk.InvokeCtx{ConfigItemID: "config-a"})

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]Session{current.Snapshot()}))
	})
})
