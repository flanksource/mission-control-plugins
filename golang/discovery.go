package main

import (
	"bytes"
	"context"
	"fmt"
	"sort"
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

// GopsDiscoveryResult is the parsed output from the in-container discovery
// script.
type GopsDiscoveryResult struct {
	Processes   []GopsProcess
	Diagnostics []string
}

// discoverGopsProcesses executes the discovery script in the selected container
// and returns the gops pid/port entries that are valid for running processes,
// plus diagnostics describing which paths were inspected.
func discoverGopsProcesses(ctx context.Context, restCfg *rest.Config, namespace, pod, container string, dirs []string) ([]GopsProcess, []string, error) {
	script := buildGopsDiscoveryScript(dirs)
	var stdout, stderr bytes.Buffer
	if err := golangk8s.ExecInPod(ctx, restCfg, golangk8s.ExecOptions{
		Namespace: namespace,
		Pod:       pod,
		Container: container,
		Command:   []string{"sh", "-s"},
		Stdin:     strings.NewReader(script),
		Stdout:    &stdout,
		Stderr:    &stderr,
	}); err != nil {
		result := parseGopsDiscoveryResult(stdout.String())
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			result.Diagnostics = append(result.Diagnostics, "stderr: "+msg)
		}
		if len(result.Processes) > 0 {
			result.Diagnostics = append(result.Diagnostics, "exec error after candidates were found: "+err.Error())
			return result.Processes, result.Diagnostics, nil
		}
		return nil, result.Diagnostics, fmt.Errorf("discover gops ports: %w", err)
	}

	result := parseGopsDiscoveryResult(stdout.String())
	return result.Processes, result.Diagnostics, nil
}

// gopsDiscoveryScript locates gops port files inside a container, printing a
// "pid=<pid> port=<port> cmd=<cmdline>" line per agent. It resolves config dirs
// from each process environ the way gops does (GOPS_CONFIG_DIR >
// $XDG_CONFIG_HOME/gops > $HOME/.config/gops, then the uid's /etc/passwd home),
// so it works for any user/image. A file is reported only when its PID still
// exists and, when tcp tables are readable, that PID owns a LISTEN socket for
// the recorded port, so files orphaned by a non-graceful exit are ignored. Dirs
// are kept newline-delimited so paths with spaces survive. __GOPS_DIRS__ is
// replaced with the shell-quoted fallback directories; callers still verify each
// reported agent by probing it.
const gopsDiscoveryScript = `set +e
NL='
'
diag() { printf 'diag=%s\n' "$*"; }

have_tcp=0
listen_count=0
listening=""
for tcpf in /proc/net/tcp /proc/net/tcp6; do
  [ -r "$tcpf" ] || continue
  have_tcp=1
  while read -r sl laddr raddr st txrx trtm retrnsmt uid timeout inode rest; do
    [ "$st" = "0A" ] || continue
    [ -n "$inode" ] || continue
    listen_count=$((listen_count + 1))
    listening="$listening${laddr##*:} $inode$NL"
  done < "$tcpf"
done
if [ "$have_tcp" = 1 ]; then
  diag "tcp listen sockets visible: $listen_count"
else
  diag "tcp listen socket tables are not readable; only PID existence will be validated"
fi

patterns=""
add_dir() {
  [ -n "$1" ] || return 0
  case "$NL$patterns$NL" in *"$NL$1$NL"*) return 0 ;; esac
  patterns="$patterns$NL$1"
}
env_val() {
  tr '\000' '\n' < "$2" 2>/dev/null | while IFS= read -r kv; do
    case "$kv" in "$1="*) printf '%s' "${kv#$1=}"; break ;; esac
  done
}
passwd_home() {
  while IFS=: read -r pn pp pu pg pc ph ps; do
    [ "$pu" = "$1" ] || continue
    printf '%s' "$ph"; break
  done < /etc/passwd 2>/dev/null
}
passwd_home_for_pid() {
  uid=$(while read -r sk sv srest; do [ "$sk" = "Uid:" ] && { printf '%s' "$sv"; break; }; done < "/proc/$1/status" 2>/dev/null)
  [ -n "$uid" ] || return 1
  passwd_home "$uid"
}
add_process_config_dir() {
  g=$(env_val GOPS_CONFIG_DIR "/proc/$1/environ")
  x=$(env_val XDG_CONFIG_HOME "/proc/$1/environ")
  h=$(env_val HOME "/proc/$1/environ")
  if [ -n "$g" ]; then add_dir "$g"; return 0; fi
  if [ -n "$x" ]; then
    case "$x" in
      /*) add_dir "$x/gops"; return 0 ;;
    esac
    hd=$(passwd_home_for_pid "$1") && [ -n "$hd" ] && { add_dir "$hd/.config/gops"; return 0; }
    [ -n "$h" ] && add_dir "$h/.config/gops"
    return 0
  fi
  if [ -n "$h" ]; then add_dir "$h/.config/gops"; return 0; fi
  hd=$(passwd_home_for_pid "$1") && [ -n "$hd" ] && add_dir "$hd/.config/gops"
}
listening_inodes() {
  printf '%s' "$listening" | while IFS=' ' read -r lp linode; do
    [ "$lp" = "$1" ] && [ -n "$linode" ] && printf '%s\n' "$linode"
  done
}
pid_has_socket_inode() {
  for fd in "/proc/$1/fd"/*; do
    [ -e "$fd" ] || continue
    target=$(readlink "$fd" 2>/dev/null) || continue
    [ "$target" = "socket:[$2]" ] && return 0
  done
  return 1
}
pid_listens_on_port() {
  hexport=$(printf '%04X' "$2" 2>/dev/null) || return 1
  for inode in $(listening_inodes "$hexport"); do
    pid_has_socket_inode "$1" "$inode" && return 0
  done
  return 1
}

for dir in __GOPS_DIRS__; do add_dir "$dir"; done

# Mirror gops' config-dir resolution per process:
# GOPS_CONFIG_DIR > $XDG_CONFIG_HOME/gops > $HOME/.config/gops > <passwd home>/.config/gops.
env_seen=0
env_readable=0
env_unreadable=0
for envf in /proc/[0-9]*/environ; do
  case "$envf" in *'['*) continue ;; esac
  env_seen=$((env_seen + 1))
  if [ ! -r "$envf" ]; then
    env_unreadable=$((env_unreadable + 1))
    continue
  fi
  env_readable=$((env_readable + 1))
  spid=${envf#/proc/}; spid=${spid%/environ}
  add_process_config_dir "$spid"
done

diag "process environ files: seen=$env_seen readable=$env_readable unreadable=$env_unreadable"
diag "resolved gops config dirs:"
IFS=$NL
for dir in $patterns; do
  [ -n "$dir" ] && diag "  $dir"
done

for dir in $patterns; do
  [ -n "$dir" ] || continue
  for expanded in $dir; do
    if [ ! -d "$expanded" ]; then
      diag "gops config dir not found: $expanded"
      continue
    fi
    diag "checking gops config dir: $expanded"
    file_count=0
    for f in "$expanded"/*; do
      [ -f "$f" ] || continue
      file_count=$((file_count + 1))
      pid=$(basename "$f")
      case "$pid" in ""|*[!0-9]*) diag "ignored non-pid file: $f"; continue ;; esac
      port=$(cat "$f" 2>/dev/null | tr -dc '0-9')
      [ -n "$port" ] || { diag "ignored $f: no numeric port"; continue; }
      [ -d "/proc/$pid" ] || { diag "ignored $f: pid $pid is not running"; continue; }
      if [ "$have_tcp" = 1 ]; then
        pid_listens_on_port "$pid" "$port" || { diag "ignored $f: pid $pid does not own a LISTEN socket on port $port"; continue; }
      fi
      cmd=""
      if [ -r "/proc/$pid/cmdline" ]; then
        cmd=$(tr '\000' ' ' <"/proc/$pid/cmdline" 2>/dev/null)
      fi
      diag "candidate gops file: path=$f pid=$pid port=$port"
      printf 'pid=%s port=%s cmd=%s\n' "$pid" "$port" "$cmd"
    done
    [ "$file_count" = 0 ] && diag "no files in gops config dir: $expanded"
  done
done
exit 0
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

// parseGopsDiscoveryResult parses discovery script output lines in the forms
// "pid=<pid> port=<port> cmd=<cmdline>" and "diag=<message>".
func parseGopsDiscoveryResult(raw string) GopsDiscoveryResult {
	var result GopsDiscoveryResult
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if diag, ok := strings.CutPrefix(line, "diag="); ok {
			if diag = strings.TrimSpace(diag); diag != "" {
				result.Diagnostics = append(result.Diagnostics, diag)
			}
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
			result.Processes = append(result.Processes, proc)
		}
	}

	return result
}

// orderGopsCandidates returns the discovered processes in the order they should
// be tried: only the requested PID when one is given, otherwise every process
// sorted by ascending PID so the lowest PID (usually the main process) is first.
// Returning all of them lets callers probe each, so a stale-but-listening file
// can't shadow a real agent that happens to sort lower.
func orderGopsCandidates(processes []GopsProcess, pid int) []GopsProcess {
	if pid > 0 {
		for _, proc := range processes {
			if proc.PID == pid {
				return []GopsProcess{proc}
			}
		}
		return nil
	}
	out := append([]GopsProcess(nil), processes...)
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// selectGopsProcess returns the requested PID when provided; otherwise it picks
// the lowest PID from the discovered gops processes as the deterministic default.
func selectGopsProcess(processes []GopsProcess, pid int) (GopsProcess, bool) {
	ordered := orderGopsCandidates(processes, pid)
	if len(ordered) == 0 {
		return GopsProcess{}, false
	}
	return ordered[0], true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
