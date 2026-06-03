package main

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	golangk8s "github.com/flanksource/mission-control-plugins/golang/internal/k8s"
	"k8s.io/client-go/rest"
)

// GopsProcess describes one gops agent port file found inside a container.
type GopsProcess struct {
	PID     int    `json:"pid"`
	Port    int    `json:"port"`
	Command string `json:"command,omitempty"`
}

// discoverGopsProcesses executes the discovery script in the selected container
// and returns the gops pid/port entries that are valid for running processes.
func discoverGopsProcesses(ctx context.Context, restCfg *rest.Config, namespace, pod, container string, dirs []string) ([]GopsProcess, error) {
	script := buildGopsDiscoveryScript(dirs)
	var stdout, stderr bytes.Buffer
	if err := golangk8s.ExecInPod(ctx, restCfg, golangk8s.ExecOptions{
		Namespace: namespace,
		Pod:       pod,
		Container: container,
		Command:   []string{"sh", "-c", script},
		Stdout:    &stdout,
		Stderr:    &stderr,
	}); err != nil {
		return nil, fmt.Errorf("discover gops ports: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return parseGopsDiscovery(stdout.String()), nil
}

// buildGopsDiscoveryScript builds a POSIX shell script that scans GOPS_CONFIG_DIR
// and configured directories for gops port files named by PID.
func buildGopsDiscoveryScript(dirs []string) string {
	quoted := make([]string, 0, len(dirs)+1)
	quoted = append(quoted, `"${GOPS_CONFIG_DIR:-}"`)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		quoted = append(quoted, shellQuote(dir))
	}
	return fmt.Sprintf(`set +e
for dir in %s; do
  [ -n "$dir" ] || continue
  for expanded in $dir; do
    [ -d "$expanded" ] || continue
    for f in "$expanded"/*; do
      [ -f "$f" ] || continue
      pid=$(basename "$f")
      case "$pid" in ""|*[!0-9]*) continue ;; esac
      port=$(cat "$f" 2>/dev/null | tr -dc '0-9')
      [ -n "$port" ] || continue
      [ -d "/proc/$pid" ] || continue
      cmd=""
      if [ -r "/proc/$pid/cmdline" ]; then
        cmd=$(tr '\000' ' ' <"/proc/$pid/cmdline" 2>/dev/null)
      fi
      printf 'pid=%%s port=%%s cmd=%%s\n' "$pid" "$port" "$cmd"
    done
  done
done
`, strings.Join(quoted, " "))
}

// parseGopsDiscovery parses discovery script output lines in the form
// "pid=<pid> port=<port> cmd=<cmdline>" and drops incomplete entries.
func parseGopsDiscovery(raw string) []GopsProcess {
	var out []GopsProcess
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var proc GopsProcess
		for field := range strings.FieldsSeq(line) {
			k, v, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}

			switch k {
			case "pid":
				proc.PID, _ = strconv.Atoi(v)
			case "port":
				proc.Port, _ = strconv.Atoi(v)
			}
		}

		if _, after, ok := strings.Cut(line, " cmd="); ok {
			proc.Command = strings.TrimSpace(after)
		}

		if proc.PID > 0 && proc.Port > 0 {
			out = append(out, proc)
		}
	}

	return out
}

// selectGopsProcess returns the requested PID when provided; otherwise it picks
// the lowest PID from the discovered gops processes as the deterministic default.
func selectGopsProcess(processes []GopsProcess, pid int) (GopsProcess, bool) {
	if pid > 0 {
		for _, proc := range processes {
			if proc.PID == pid {
				return proc, true
			}
		}
		return GopsProcess{}, false
	}
	if len(processes) == 0 {
		return GopsProcess{}, false
	}

	best := processes[0]
	for _, proc := range processes[1:] {
		if proc.PID < best.PID {
			best = proc
		}
	}
	return best, true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
