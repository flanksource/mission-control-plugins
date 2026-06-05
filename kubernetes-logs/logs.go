package main

import (
	"bufio"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// containerNames returns the containers to pull logs from. When override is
// empty, every container in the pod is included.
func containerNames(pod corev1.Pod, override string) []string {
	if override != "" {
		return []string{override}
	}
	names := make([]string, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	return names
}

func readLines(r io.Reader) ([]string, error) {
	var out []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		out = append(out, strings.TrimRight(scanner.Text(), "\r"))
	}
	return out, scanner.Err()
}
