package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dutytypes "github.com/flanksource/duty/types"
	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

type fakeHost struct {
	items map[string]*pluginpb.ConfigItem
}

func (f fakeHost) GetConfigItem(_ context.Context, id string) (*pluginpb.ConfigItem, error) {
	return f.items[id], nil
}
func (fakeHost) ListConfigs(context.Context, dutytypes.ResourceSelector, int) (*pluginpb.ConfigItemList, error) {
	panic("not implemented")
}
func (fakeHost) GetConnectionByType(context.Context, sdk.ConnectionType) (*pluginpb.ResolvedConnection, error) {
	panic("not implemented")
}
func (fakeHost) GetConnectionForConfig(context.Context, string) (*pluginpb.ResolvedConnection, error) {
	panic("not implemented")
}
func (fakeHost) GetConnectionByID(context.Context, string) (*pluginpb.ResolvedConnection, error) {
	panic("not implemented")
}
func (fakeHost) GetConnectionByLabel(context.Context, string) (*pluginpb.ResolvedConnection, error) {
	panic("not implemented")
}
func (fakeHost) Log(context.Context, string, string, map[string]string) error {
	panic("not implemented")
}
func (fakeHost) InvokePlugin(context.Context, string, string, string, json.RawMessage) (*pluginpb.InvokeResponse, error) {
	panic("not implemented")
}
func (fakeHost) WriteArtifact(context.Context, *pluginpb.Artifact) (*pluginpb.ArtifactRef, error) {
	panic("not implemented")
}
func (fakeHost) ReadArtifact(context.Context, *pluginpb.ArtifactRef) (*pluginpb.Artifact, error) {
	panic("not implemented")
}

func TestResolvePodsSupportsMatchExpressions(t *testing.T) {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      "app",
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{"api"},
		}},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec:       appsv1.DeploymentSpec{Selector: selector},
	}
	matched := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "prod", Labels: map[string]string{"app": "api"}}}
	ignored := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "prod", Labels: map[string]string{"app": "web"}}}
	cli := fake.NewSimpleClientset(deployment, matched, ignored)

	pods, err := resolvePods(context.Background(), cli, fakeHost{items: map[string]*pluginpb.ConfigItem{
		"deployment": {Name: "api", Type: "Kubernetes::Deployment", Tags: map[string]string{"namespace": "prod"}},
	}}, "deployment")
	if err != nil {
		t.Fatalf("resolvePods returned error: %v", err)
	}
	if len(pods) != 1 || pods[0].Name != "api-1" {
		t.Fatalf("expected api-1 only, got %#v", pods)
	}
}

func TestHTTPLogsNonFollowReturnsJSONOnce(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "prod"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}}},
	}
	cli := fake.NewSimpleClientset(pod)
	plugin := &KubernetesLogsPlugin{clients: clientCache{entries: map[string]kubernetes.Interface{"cfg": cli}}}

	req := httptest.NewRequest(http.MethodGet, "/logs?config_id=cfg&namespace=prod&pod=api-1", nil)
	res := httptest.NewRecorder()
	plugin.httpLogs(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	var rows []sseLogLine
	if err := json.Unmarshal(res.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(rows) != 2 || rows[0].Container != "app" || rows[1].Container != "sidecar" {
		t.Fatalf("expected one log row per container, got %#v", rows)
	}

	opts := logOptions(cli)
	if len(opts) != 2 {
		t.Fatalf("expected 2 log requests, got %d", len(opts))
	}
	for _, opt := range opts {
		if opt.Follow {
			t.Fatalf("expected non-follow log request, got %#v", opt)
		}
		if opt.TailLines == nil || *opt.TailLines != 200 {
			t.Fatalf("expected default tailLines=200, got %#v", opt)
		}
	}
}

func TestHTTPLogsFollowStreamsSSEWithTailZero(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "prod"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	cli := fake.NewSimpleClientset(pod)
	plugin := &KubernetesLogsPlugin{clients: clientCache{entries: map[string]kubernetes.Interface{"cfg": cli}}}

	req := httptest.NewRequest(http.MethodGet, "/logs?config_id=cfg&namespace=prod&pod=api-1&follow=true&tailLines=200", nil)
	res := httptest.NewRecorder()
	plugin.httpLogs(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	if body := res.Body.String(); !strings.Contains(body, "data:") {
		t.Fatalf("expected SSE data event, got %q", body)
	}

	opts := logOptions(cli)
	if len(opts) != 1 {
		t.Fatalf("expected 1 log request, got %d", len(opts))
	}
	if !opts[0].Follow || opts[0].TailLines == nil || *opts[0].TailLines != 0 {
		t.Fatalf("expected follow=true tailLines=0, got %#v", opts[0])
	}
}

func logOptions(cli *fake.Clientset) []*corev1.PodLogOptions {
	var opts []*corev1.PodLogOptions
	for _, action := range cli.Actions() {
		if action.GetVerb() == "get" && action.GetResource().Resource == "pods" && action.GetSubresource() == "log" {
			generic, ok := action.(ktesting.GenericAction)
			if !ok {
				continue
			}
			if opt, ok := generic.GetValue().(*corev1.PodLogOptions); ok {
				opts = append(opts, opt)
			}
		}
	}
	return opts
}
