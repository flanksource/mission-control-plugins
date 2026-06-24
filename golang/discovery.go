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

// gopsDiscoveryScript locates gops port files inside a container, printing a
// "pid=<pid> port=<port> cmd=<cmdline>" line per live agent. It resolves config
// dirs from each process environ the way gops does
// (GOPS_CONFIG_DIR > $XDG_CONFIG_HOME/gops > $HOME/.config/gops) so it works for
// any user/image, and keeps only files whose port is currently listening (in
// /proc/net/tcp{,6}) so orphaned files from a non-graceful exit are ignored.
// __GOPS_DIRS__ is replaced with the shell-quoted fallback directories.
const gopsDiscoveryScript = `set +e
have_tcp=0
listening=""
for tcpf in /proc/net/tcp /proc/net/tcp6; do
  [ -r "$tcpf" ] || continue
  have_tcp=1
  while read -r sl laddr raddr st rest; do
    [ "$st" = "0A" ] || continue
    listening="$listening ${laddr##*:} "
  done < "$tcpf"
done

patterns=""
add_dir() {
  [ -n "$1" ] || return 0
  case " $patterns " in *" $1 "*) return 0 ;; esac
  patterns="$patterns $1"
}
env_val() {
  tr '\000' '\n' < "$2" 2>/dev/null | while IFS= read -r kv; do
    case "$kv" in "$1="*) printf '%s' "${kv#$1=}"; break ;; esac
  done
}

for dir in __GOPS_DIRS__; do add_dir "$dir"; done

for envf in /proc/[0-9]*/environ; do
  [ -r "$envf" ] || continue
  g=$(env_val GOPS_CONFIG_DIR "$envf")
  x=$(env_val XDG_CONFIG_HOME "$envf")
  h=$(env_val HOME "$envf")
  if [ -n "$g" ]; then add_dir "$g"
  elif [ -n "$x" ]; then add_dir "$x/gops"
  elif [ -n "$h" ]; then add_dir "$h/.config/gops"
  fi
done

for dir in $patterns; do
  [ -n "$dir" ] || continue
  for expanded in $dir; do
    [ -d "$expanded" ] || continue
    for f in "$expanded"/*; do
      [ -f "$f" ] || continue
      pid=$(basename "$f")
      case "$pid" in ""|*[!0-9]*) continue ;; esac
      port=$(cat "$f" 2>/dev/null | tr -dc '0-9')
      [ -n "$port" ] || continue
      if [ "$have_tcp" = 1 ]; then
        hexport=$(printf '%04X' "$port" 2>/dev/null)
        case "$listening" in *" $hexport "*) ;; *) continue ;; esac
      else
        [ -d "/proc/$pid" ] || continue
      fi
      cmd=""
      if [ -r "/proc/$pid/cmdline" ]; then
        cmd=$(tr '\000' ' ' <"/proc/$pid/cmdline" 2>/dev/null)
      fi
      printf 'pid=%s port=%s cmd=%s\n' "$pid" "$port" "$cmd"
    done
  done
done
`

// buildGopsDiscoveryScript fills gopsDiscoveryScript with the shell-quoted
// fallback directories (GOPS_CONFIG_DIR plus the configured dirs).
func buildGopsDiscoveryScript(dirs []string) string {
	quoted := make([]string, 0, len(dirs)+1)
	quoted = append(quoted, `"${GOPS_CONFIG_DIR:-}"`)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		quoted = append(quoted, shellQuote(dir))
	}
	return strings.ReplaceAll(gopsDiscoveryScript, "__GOPS_DIRS__", strings.Join(quoted, " "))
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
