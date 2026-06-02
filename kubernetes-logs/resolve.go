package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	pluginpb "github.com/flanksource/incident-commander/plugin/proto"
	"github.com/flanksource/incident-commander/plugin/sdk"
)

// resolvePods turns a config item into the set of pods we should fetch logs
// from. Mission-control tags Kubernetes resources with `name`, `namespace`,
// and the resource Kind in tags/labels. We use the host's GetConfigItem
// to read those, then walk down to pods according to the workload kind.
//
// Supported kinds (case-insensitive):
//
//   - Namespace           — every Pod in the namespace
//   - Pod                 — itself
//   - Deployment          — via ReplicaSet selector
//   - StatefulSet         — via selector
//   - DaemonSet           — via selector
//   - ReplicaSet          — via selector
//   - Job / CronJob       — via selector
//
// Any other kind falls back to "label match by app=<name> in <namespace>",
// which covers the typical Helm chart layout without per-controller
// custom logic.
func resolvePods(ctx context.Context, cli kubernetes.Interface, host sdk.HostClient, configItemID string) ([]corev1.Pod, error) {
	if host == nil || configItemID == "" {
		return nil, fmt.Errorf("config_item_id is required")
	}
	item, err := host.GetConfigItem(ctx, configItemID)
	if err != nil {
		return nil, fmt.Errorf("get config item: %w", err)
	}

	kind, namespace, name := extractKubeRef(item)
	if name == "" {
		return nil, fmt.Errorf("config item %s has no Kubernetes name tag", configItemID)
	}
	if namespace == "" {
		namespace = "default"
	}

	switch strings.ToLower(kind) {
	case "namespace", "kubernetes::namespace":
		return podsByNamespace(ctx, cli, name)

	case "pod", "kubernetes::pod":
		pod, err := cli.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return []corev1.Pod{*pod}, nil

	case "deployment", "kubernetes::deployment":
		dep, err := cli.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return podsByLabelSelector(ctx, cli, namespace, dep.Spec.Selector)

	case "statefulset", "kubernetes::statefulset":
		ss, err := cli.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return podsByLabelSelector(ctx, cli, namespace, ss.Spec.Selector)

	case "daemonset", "kubernetes::daemonset":
		ds, err := cli.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return podsByLabelSelector(ctx, cli, namespace, ds.Spec.Selector)

	case "replicaset", "kubernetes::replicaset":
		rs, err := cli.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return podsByLabelSelector(ctx, cli, namespace, rs.Spec.Selector)

	case "job", "kubernetes::job":
		job, err := cli.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return podsByLabelSelector(ctx, cli, namespace, job.Spec.Selector)

	case "cronjob", "kubernetes::cronjob":
		// Find every Job owned by the CronJob, then every Pod owned by those Jobs.
		jobs, err := cli.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
		if apierrors.IsNotFound(err) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		var pods []corev1.Pod
		for _, j := range jobs.Items {
			if !ownedBy(j.OwnerReferences, "CronJob", name) {
				continue
			}
			ps, err := podsByLabelSelector(ctx, cli, namespace, j.Spec.Selector)
			if err != nil {
				continue
			}
			pods = append(pods, ps...)
		}
		return pods, nil
	}

	// Fallback: try `app=<name>` (the dominant Helm convention).
	return podsBySelector(ctx, cli, namespace, map[string]string{"app": name})
}

// extractKubeRef pulls the Kubernetes kind / namespace / name from a config
// item. Mission-control's Kubernetes scraper writes these as tags.
func extractKubeRef(item *pluginpb.ConfigItem) (kind, namespace, name string) {
	if item == nil {
		return "", "", ""
	}
	kind = item.Type
	namespace = item.Tags["namespace"]
	if namespace == "" {
		namespace = item.Namespace
	}
	name = item.Name
	if tagName := item.Tags["name"]; tagName != "" {
		name = tagName
	}
	return
}

func podsByNamespace(ctx context.Context, cli kubernetes.Interface, namespace string) ([]corev1.Pod, error) {
	list, err := cli.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return sortPods(list.Items), nil
}

func podsBySelector(ctx context.Context, cli kubernetes.Interface, namespace string, sel map[string]string) ([]corev1.Pod, error) {
	if len(sel) == 0 {
		return nil, fmt.Errorf("empty selector")
	}
	return podsBySelectorString(ctx, cli, namespace, labels.SelectorFromSet(sel).String())
}

func podsByLabelSelector(ctx context.Context, cli kubernetes.Interface, namespace string, sel *metav1.LabelSelector) ([]corev1.Pod, error) {
	if sel == nil {
		return nil, fmt.Errorf("empty selector")
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return nil, fmt.Errorf("invalid selector: %w", err)
	}
	if selector.Empty() {
		return nil, fmt.Errorf("empty selector")
	}
	return podsBySelectorString(ctx, cli, namespace, selector.String())
}

func podsBySelectorString(ctx context.Context, cli kubernetes.Interface, namespace, selector string) ([]corev1.Pod, error) {
	list, err := cli.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, err
	}
	return sortPods(list.Items), nil
}

func sortPods(pods []corev1.Pod) []corev1.Pod {
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	return pods
}

func ownedBy(refs []metav1.OwnerReference, kind, name string) bool {
	for _, r := range refs {
		if r.Kind == kind && r.Name == name {
			return true
		}
	}
	return false
}
