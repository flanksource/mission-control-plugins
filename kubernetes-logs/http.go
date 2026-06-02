package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

// httpLogs fetches pod logs. By default it behaves like `kubectl logs`:
// return the requested tail as a finite JSON array and close. With
// follow=true it behaves like `kubectl logs -f --tail=0` and streams new lines
// as Server-Sent Events until the client disconnects.
func (p *KubernetesLogsPlugin) httpLogs(w http.ResponseWriter, r *http.Request) {
	configID := r.URL.Query().Get("config_id")
	podName := r.URL.Query().Get("pod")
	namespace := r.URL.Query().Get("namespace")
	container := r.URL.Query().Get("container")
	follow := r.URL.Query().Get("follow") == "true"
	tailLines, _ := strconv.ParseInt(r.URL.Query().Get("tailLines"), 10, 64)
	if follow {
		tailLines = 0
	} else if tailLines <= 0 {
		tailLines = 200
	}
	if configID == "" || podName == "" || namespace == "" {
		http.Error(w, "config_id, pod and namespace required", http.StatusBadRequest)
		return
	}

	cli, err := p.clients.For(r.Context(), sdk.HostClientFromContext(r.Context()), configID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	pod, err := cli.CoreV1().Pods(namespace).Get(r.Context(), podName, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	containers := containerNames(*pod, container)

	if !follow {
		lines, err := fetchHTTPLogs(r.Context(), cli, namespace, podName, containers, tailLines)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lines)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Content-Encoding", "identity")
	w.Header().Set("X-Accel-Buffering", "no")
	followHTTPLogs(r.Context(), cli, w, flusher, namespace, podName, containers, tailLines)
}

func fetchHTTPLogs(ctx context.Context, cli kubernetes.Interface, namespace, pod string, containers []string, tailLines int64) ([]sseLogLine, error) {
	var out []sseLogLine
	for _, container := range containers {
		opts := &corev1.PodLogOptions{
			Container:  container,
			Follow:     false,
			TailLines:  &tailLines,
			Timestamps: true,
		}
		stream, err := cli.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
		if err != nil {
			return nil, err
		}
		lines, err := readLines(stream)
		_ = stream.Close()
		if err != nil {
			return nil, err
		}
		baseLabels := httpLogLabels(namespace, pod, container)
		for _, raw := range lines {
			out = append(out, parseSSELine(pod, container, baseLabels, raw))
		}
	}
	return out, nil
}

func followHTTPLogs(ctx context.Context, cli kubernetes.Interface, w http.ResponseWriter, flusher http.Flusher, namespace, pod string, containers []string, tailLines int64) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	write := func(fn func()) {
		mu.Lock()
		defer mu.Unlock()
		fn()
	}

	write(func() { writeSSEComment(w, flusher, "connected") })
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				write(func() { writeSSEComment(w, flusher, "heartbeat") })
			}
		}
	}()

	for _, container := range containers {
		container := container
		wg.Add(1)
		go func() {
			defer wg.Done()
			opts := &corev1.PodLogOptions{
				Container:  container,
				Follow:     true,
				TailLines:  &tailLines,
				Timestamps: true,
			}
			stream, err := cli.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
			if err != nil {
				write(func() { writeSSE(w, flusher, "error", err.Error()) })
				return
			}
			defer func() { _ = stream.Close() }()

			baseLabels := httpLogLabels(namespace, pod, container)
			streamLines(ctx, stream, func(raw string) {
				write(func() { writeSSEJSON(w, flusher, parseSSELine(pod, container, baseLabels, raw)) })
			})
		}()
	}
	wg.Wait()
	close(done)
}

func httpLogLabels(namespace, pod, container string) map[string]string {
	labels := map[string]string{
		"namespace": namespace,
		"pod":       pod,
	}
	if container != "" {
		labels["container"] = container
	}
	return labels
}

// sseLogLine is the wire shape sent to the iframe, either as JSON rows for
// one-shot logs or as SSE data payloads in follow mode. Pod/container/line stay
// flat for backwards compat with the existing renderer; timestamp and labels
// are added so the UI can show kubelet-reported time and any tags
// (`namespace`, `pod`, `container`) without the iframe having to re-derive
// them from the URL.
type sseLogLine struct {
	Pod       string            `json:"pod"`
	Container string            `json:"container"`
	Line      string            `json:"line"`
	Timestamp string            `json:"timestamp,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// parseSSELine splits the leading RFC3339Nano timestamp produced by
// PodLogOptions.Timestamps from the message body and surfaces both, plus
// the per-stream labels.
func parseSSELine(pod, container string, baseLabels map[string]string, raw string) sseLogLine {
	out := sseLogLine{Pod: pod, Container: container, Labels: baseLabels}
	if idx := strings.IndexByte(raw, ' '); idx > 0 {
		if ts, err := time.Parse(time.RFC3339Nano, raw[:idx]); err == nil {
			out.Timestamp = ts.Format(time.RFC3339Nano)
			out.Line = raw[idx+1:]
			return out
		}
	}
	out.Line = raw
	return out
}

func streamLines(ctx context.Context, r interface{ Read([]byte) (int, error) }, fn func(string)) {
	scanner := bufio.NewScanner(struct{ Reader }{Reader{r}})
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		fn(strings.TrimRight(scanner.Text(), "\r"))
	}
}

// Reader adapts Read([]byte) to io.Reader so we can use bufio.Scanner.
type Reader struct {
	R interface{ Read([]byte) (int, error) }
}

func (r Reader) Read(p []byte) (int, error) { return r.R.Read(p) }

func writeSSEComment(w http.ResponseWriter, f http.Flusher, data string) {
	if _, err := fmt.Fprintf(w, ": %s\n\n", data); err != nil {
		return
	}
	f.Flush()
}

func writeSSE(w http.ResponseWriter, f http.Flusher, event, data string) {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return
	}

	for line := range strings.SplitSeq(data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return
		}
	}

	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return
	}

	f.Flush()
}

func writeSSEJSON(w http.ResponseWriter, f http.Flusher, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return
	}
	f.Flush()
}
