package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type workloadQueries struct {
	pods, zeroReplicas        string
	cpuUsage, memoryUsage     string
	cpuRequest, memoryRequest string
	cpuLimit, memoryLimit     string
}

func extractKubeRef(item *pluginpb.ConfigItem) (kind, namespace, name string) {
	if item == nil {
		return
	}
	kind, namespace, name = item.Type, item.Tags["namespace"], item.Name
	if namespace == "" {
		namespace = item.Namespace
	}
	if item.Tags["name"] != "" {
		name = item.Tags["name"]
	}
	return
}

func resolveQueries(ctx context.Context, host sdk.HostClient, id string) (workloadQueries, error) {
	if id == "" {
		return workloadQueries{}, invalid("config item %q: config_id is required", id)
	}
	if host == nil {
		return workloadQueries{}, fmt.Errorf("mission control host unavailable for config item %s", id)
	}
	item, err := host.GetConfigItem(ctx, id)
	if err != nil {
		code, message := status.Code(err), status.Convert(err).Message()
		err = fmt.Errorf("get config item %s: %w", id, err)
		switch {
		// Older hosts send this exact GORM lookup error as Internal or Unknown,
		// including RLS-hidden items. Do not classify other host failures as 404.
		case code == codes.NotFound || ((code == codes.Internal || code == codes.Unknown) && message == fmt.Sprintf("config item %s: record not found", id)):
			return workloadQueries{}, &lookupError{status: http.StatusNotFound, code: "ENOTFOUND", err: err}
		case code == codes.PermissionDenied:
			return workloadQueries{}, &lookupError{status: http.StatusForbidden, code: "EFORBIDDEN", err: err}
		}
		return workloadQueries{}, err
	}
	kind, ns, name := extractKubeRef(item)
	kind = strings.TrimPrefix(strings.ToLower(kind), "kubernetes::")
	if ns == "" || name == "" || (kind != "pod" && kind != "deployment" && kind != "statefulset") {
		return workloadQueries{}, invalid("config item %s: expected Pod, Deployment or StatefulSet with namespace and name (got kind=%q namespace=%q name=%q)", id, kind, ns, name)
	}
	return queriesFor(kind, ns, name), nil
}

func queriesFor(kind, namespace, name string) workloadQueries {
	ns, workload := strconv.Quote(namespace), strconv.Quote(name)
	labels := "namespace=" + ns
	var membership string
	q := workloadQueries{}
	switch kind {
	case "pod":
		labels += ",pod=" + workload
		membership = "max by (namespace, pod) (kube_pod_info{" + labels + "})"
	case "statefulset":
		membership = "max by (namespace, pod) (kube_pod_owner{" + labels + ",owner_kind=\"StatefulSet\",owner_name=" + workload + ",owner_is_controller=\"true\"})"
	case "deployment":
		// Deduplicate owner series before joining so multiple KSM scrape targets
		// neither multiply usage nor produce many-to-many matching errors.
		membership = "max by (namespace, pod) (" +
			"max by (namespace, pod, replicaset) (label_replace(kube_pod_owner{" + labels + ",owner_kind=\"ReplicaSet\",owner_is_controller=\"true\"}, \"replicaset\", \"$1\", \"owner_name\", \"(.*)\"))" +
			" * on (namespace, replicaset) group_left() max by (namespace, replicaset) (kube_replicaset_owner{" + labels + ",owner_kind=\"Deployment\",owner_name=" + workload + ",owner_is_controller=\"true\"}))"
	}
	if kind != "pod" {
		// KSM keeps exporting requests and limits for Failed, Succeeded and Evicted
		// pods until they are deleted; only Pending and Running pods count.
		membership = "(" + membership + " and on (namespace, pod) (kube_pod_status_phase{" + labels + ",phase=~\"Pending|Running\"} == 1))"
		selector := "{" + labels + "," + kind + "=" + workload + "}"
		desiredMetric := "kube_deployment_spec_replicas"
		if kind == "statefulset" {
			desiredMetric = "kube_statefulset_replicas"
		}
		q.zeroReplicas = "(max(" + desiredMetric + selector + ") == 0) and (max(kube_" + kind + "_status_replicas" + selector + ") == 0)"
	}
	q.pods = "count(" + membership + ")"
	sum := func(metric string) string {
		// Collapse duplicate series from multiple KSM replicas or kubelet scrape
		// jobs so each container is counted once.
		metric = "max by (namespace, pod, container) (" + metric + ")"
		if kind == "pod" {
			return "sum(" + metric + ")"
		}
		return "sum(" + metric + " * on (namespace, pod) group_left() " + membership + ")"
	}
	q.cpuUsage = sum("rate(container_cpu_usage_seconds_total{" + labels + ",container!=\"\",container!=\"POD\"}[2m])")
	q.memoryUsage = sum("container_memory_working_set_bytes{" + labels + ",container!=\"\",container!=\"POD\"}")
	q.cpuRequest = sum("kube_pod_container_resource_requests{" + labels + ",resource=\"cpu\",unit=\"core\"}")
	q.memoryRequest = sum("kube_pod_container_resource_requests{" + labels + ",resource=\"memory\",unit=\"byte\"}")
	q.cpuLimit = sum("kube_pod_container_resource_limits{" + labels + ",resource=\"cpu\",unit=\"core\"}")
	q.memoryLimit = sum("kube_pod_container_resource_limits{" + labels + ",resource=\"memory\",unit=\"byte\"}")
	return q
}
