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

type GopsProcess struct {
	PID     int    `json:"pid"`
	Port    int    `json:"port"`
	Command string `json:"command,omitempty"`
}

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

func parseGopsDiscovery(raw string) []GopsProcess {
	var out []GopsProcess
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var proc GopsProcess
		for _, field := range strings.Fields(line) {
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
		if idx := strings.Index(line, " cmd="); idx >= 0 {
			proc.Command = strings.TrimSpace(line[idx+5:])
		}
		if proc.PID > 0 && proc.Port > 0 {
			out = append(out, proc)
		}
	}
	return out
}

func selectGopsProcess(processes []GopsProcess, pid int) (GopsProcess, bool) {
	return selectGopsProcessForTarget(processes, pid, TargetRef{})
}

func selectGopsProcessForTarget(processes []GopsProcess, pid int, target TargetRef) (GopsProcess, bool) {
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
	if len(processes) == 1 {
		return processes[0], true
	}

	best := processes[0]
	bestScore := gopsProcessScore(best, target)
	for _, proc := range processes[1:] {
		score := gopsProcessScore(proc, target)
		if score > bestScore || (score == bestScore && proc.PID < best.PID) {
			best = proc
			bestScore = score
		}
	}
	return best, true
}

func gopsProcessScore(proc GopsProcess, target TargetRef) int {
	score := 0
	cmd := strings.ToLower(proc.Command)
	name := strings.ToLower(strings.TrimSpace(target.Name))

	// In Kubernetes containers PID 1 is normally the workload process. Prefer it
	// when multiple helpers/plugins in the same container also expose gops.
	if proc.PID == 1 {
		score += 100
	}
	if name != "" && strings.Contains(cmd, name) {
		score += 60
	}
	if isMissionControlPluginCommand(cmd) {
		score -= 40
	} else if cmd != "" {
		score += 20
	}
	return score
}

func isMissionControlPluginCommand(cmd string) bool {
	return strings.Contains(cmd, "/.mission-control/plugins/") || strings.Contains(cmd, "-mc-plugin")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
