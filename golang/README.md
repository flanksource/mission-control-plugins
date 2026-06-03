# Golang Diagnostics Plugin

The `golang` plugin inspects Kubernetes workloads that already expose Go
runtime diagnostics. It does not patch deployments, inject agents, restart
pods, or enable mutating runtime actions.

## Workload setup

For gops, start the agent on a localhost-only port. A fixed port is simplest:

```go
agent.Listen(agent.Options{
	Addr:      "127.0.0.1:6061",
	ConfigDir: "/tmp/gops",
})
```

Random ports also work if the port file is readable through Kubernetes exec:

```go
agent.Listen(agent.Options{
	Addr:      "127.0.0.1:0",
	ConfigDir: "/tmp/gops",
})
```

For pprof, mount the standard handlers on a localhost-only admin listener, for
example `127.0.0.1:6060/debug/pprof`.

If no explicit gops port is provided, the plugin tries readable gops port
files first. When multiple gops port files are present, an explicit `pid`
selects that process; otherwise the lowest PID is selected. If no readable port
file is found, the plugin falls back to configured/default gops ports.

For pprof, the plugin probes `/debug/pprof/` on configured and declared
Kubernetes `containerPort` values for the selected container. gops and pprof can
run on different ports; the plugin creates separate pod port-forwards for each
endpoint.

The session-create request can provide `pid`, `gopsPort`, `gopsConfigDir`,
`pprofPort`, and `pprofBasePath` per session, so the same plugin installation
can attach to pods with different diagnostics ports.

The plugin reaches these localhost-only ports with Kubernetes pod port-forward,
so the target process does not need to bind to `0.0.0.0`.

## Caveats

### Gops port auto discovery

- we exec into the pod to discover the appropriate gops port. If the pod doesn't have `sh` installed, this will fail.
- If multiple gops agents are running, we'll pick the one with the lowest `pid`.
- If the `GOPS_CONFIG_DIR` is set to a root owned directory, ensure that the pod has `securityContext.readOnlyRootFilesystem: false`

## Build and test

```sh
task -d plugins/golang test
task build:plugin:golang
kubectl apply -f plugins/golang/Plugin.yaml
```
