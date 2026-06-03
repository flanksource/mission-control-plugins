package main

import (
	"context"
	"encoding/json"

	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = ginkgo.Describe("client", func() {
	ginkgo.It("returns host connection permission errors to callers", func() {
		host := &fakeHostClient{connectionErr: status.Error(codes.PermissionDenied, "cannot read connection")}

		_, err := buildRestConfig(context.Background(), host)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot access kubernetes connection"))
		Expect(err.Error()).To(ContainSubstring("permission denied"))
	})
})

type fakeHostClient struct {
	connectionErr error
}

func (f *fakeHostClient) GetConfigItem(context.Context, string) (*pluginpb.ConfigItem, error) {
	return nil, nil
}

func (f *fakeHostClient) GetConnectionByType(context.Context, sdk.ConnectionType) (*pluginpb.ResolvedConnection, error) {
	return nil, f.connectionErr
}

func (f *fakeHostClient) GetConnectionForConfig(context.Context, string) (*pluginpb.ResolvedConnection, error) {
	return nil, nil
}

func (f *fakeHostClient) GetConnectionByID(context.Context, string) (*pluginpb.ResolvedConnection, error) {
	return nil, nil
}

func (f *fakeHostClient) GetConnectionByLabel(context.Context, string) (*pluginpb.ResolvedConnection, error) {
	return nil, nil
}

func (f *fakeHostClient) Log(context.Context, string, string, map[string]string) error {
	return nil
}

func (f *fakeHostClient) InvokePlugin(context.Context, string, string, string, json.RawMessage) (*pluginpb.InvokeResponse, error) {
	return nil, nil
}

func (f *fakeHostClient) WriteArtifact(context.Context, *pluginpb.Artifact) (*pluginpb.ArtifactRef, error) {
	return nil, nil
}

func (f *fakeHostClient) ReadArtifact(context.Context, *pluginpb.ArtifactRef) (*pluginpb.Artifact, error) {
	return nil, nil
}
