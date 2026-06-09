// SQL Server plugin: stats / trace / defrag / console / processes
// diagnostics surfaced as a Mission Control plugin.
//
// Build: task build:plugin:sql-server
// Apply: kubectl apply -f plugins/sql-server/Plugin.yaml
package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	pluginpb "github.com/flanksource/incident-commander/plugin/api"
	"github.com/flanksource/incident-commander/plugin/sdk"
	"github.com/flanksource/mission-control-plugins/sql-server/internal/sqldefrag"
	"github.com/flanksource/mission-control-plugins/sql-server/internal/sqltrace"
)

// Operation names — exported as constants so the frontend's API client can
// reference them by symbol when we generate ts bindings.
const (
	OpStats                 = "stats"
	OpQuery                 = "query"
	OpExplain               = "explain"
	OpSchema                = "schema"
	OpDatabasesList         = "databases-list"
	OpProcessesList         = "processes-list"
	OpProcessKill           = "process-kill"
	OpTraceStart            = "trace-start"
	OpTraceList             = "trace-list"
	OpTraceGet              = "trace-get"
	OpTraceStop             = "trace-stop"
	OpTraceDelete           = "trace-delete"
	OpTraceStream           = "trace-stream"
	OpPermissions           = "permissions"
	OpRollbackList          = "rollback-list"
	OpRollbackRestore       = "rollback-restore"
	OpDefragHealth          = "defrag-health"
	OpDefragFix             = "defrag-fix"
	OpDefragBulkRebuild     = "defrag-bulk-rebuild"
	OpDefragFixJobs         = "defrag-fix-jobs"
	OpDefragRollbacks       = "defrag-rollbacks"
	OpDefragRollbackRestore = "defrag-rollback-restore"
	OpDefragFixStop         = "defrag-fix-stop"
	OpDefragInstall         = "defrag-install"
	OpDefragRun             = "defrag-run"
	OpDefragStatus          = "defrag-status"
	OpDefragStats           = "defrag-stats"
	OpDefragHistory         = "defrag-history"
	OpDefragSessions        = "defrag-sessions"
	OpDefragTerminate       = "defrag-terminate"
	OpDefragJobs            = "defrag-jobs"
	OpDefragStop            = "defrag-stop"
)

//go:generate go run ./internal/gen-checksum

//go:embed ui/*
var uiAssets embed.FS

// Version and BuildDate are injected at link time:
//
//	go build -ldflags "-X main.Version=$VERSION -X 'main.BuildDate=$DATE'" ./plugins/sql-server
//
// The Taskfile (build:plugin:sql-server) sets both. Leaving them at "dev"
// causes the SDK's RegisterPlugin to fail fast — every plugin MUST ship
// with a real version.
var (
	Version   = ""
	BuildDate = ""
)

func main() {
	sub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		panic(err)
	}
	sdk.Serve(newPlugin(), sdk.WithStaticAssets(sub))
}

type SQLServerPlugin struct {
	clients    connectionCache
	traces     *sqltrace.Registry
	defragJobs *sqldefrag.JobRegistry
	fixJobs    *sqldefrag.FixJobRegistry
}

func newPlugin() *SQLServerPlugin {
	return &SQLServerPlugin{
		traces:     sqltrace.NewRegistry(),
		defragJobs: sqldefrag.NewJobRegistry(nil),
		fixJobs:    sqldefrag.NewFixJobRegistry(),
	}
}

func (p *SQLServerPlugin) Manifest() *pluginpb.PluginManifest {
	return &pluginpb.PluginManifest{
		Name:         "sql-server",
		Version:      sdk.FormatVersion(Version, BuildDate, uiChecksum),
		Description:  "Inspect SQL Server health (stats, processes), capture Extended Events traces, run AdaptiveIndexDefrag, and execute ad-hoc queries.",
		Capabilities: []string{"tabs", "operations"},
		Tabs: []*pluginpb.TabSpec{
			{Name: "SQL Server", Icon: "lucide:database", Path: "/", Scope: "config"},
		},
		Operations: operationDefs(),
	}
}

func (p *SQLServerPlugin) Configure(_ context.Context, _ map[string]any) error {
	return nil
}

func (p *SQLServerPlugin) Operations() []sdk.Operation {
	defs := operationDefs()
	handlers := map[string]func(context.Context, sdk.InvokeCtx) (any, error){
		OpStats:                 p.stats,
		OpQuery:                 p.query,
		OpExplain:               p.explain,
		OpSchema:                p.schema,
		OpDatabasesList:         p.databasesList,
		OpProcessesList:         p.processesList,
		OpProcessKill:           p.processKill,
		OpTraceStart:            p.traceStart,
		OpTraceList:             p.traceList,
		OpTraceGet:              p.traceGet,
		OpTraceStop:             p.traceStop,
		OpTraceDelete:           p.traceDelete,
		OpPermissions:           p.permissions,
		OpRollbackList:          p.rollbackList,
		OpRollbackRestore:       p.rollbackRestore,
		OpDefragHealth:          p.defragHealth,
		OpDefragFix:             p.defragFix,
		OpDefragBulkRebuild:     p.defragBulkRebuild,
		OpDefragFixJobs:         p.defragFixJobsList,
		OpDefragRollbacks:       p.defragRollbacks,
		OpDefragRollbackRestore: p.defragRollbackRestore,
		OpDefragFixStop:         p.defragFixStop,
		OpDefragInstall:         p.defragInstall,
		OpDefragRun:             p.defragRun,
		OpDefragStatus:          p.defragStatus,
		OpDefragStats:           p.defragStats,
		OpDefragHistory:         p.defragHistory,
		OpDefragSessions:        p.defragSessions,
		OpDefragTerminate:       p.defragTerminate,
		OpDefragJobs:            p.defragJobsList,
		OpDefragStop:            p.defragStop,
	}
	httpHandlers := map[string]http.Handler{
		OpTraceStream: http.HandlerFunc(p.httpTraceStream),
	}
	out := make([]sdk.Operation, 0, len(defs))
	for _, d := range defs {
		h := handlers[d.Name]
		hh := httpHandlers[d.Name]
		if h == nil && hh == nil {
			continue
		}
		out = append(out, sdk.Operation{Def: d, Handler: h, HTTPHandler: hh})
	}
	return out
}

func operationDefs() []*pluginpb.OperationDef {
	mk := func(name, desc string) *pluginpb.OperationDef {
		return &pluginpb.OperationDef{Name: name, Description: desc, Scope: "config", ResultMime: sdk.ClickyResultMimeType}
	}
	return []*pluginpb.OperationDef{
		mk(OpStats, "Snapshot of SQL Server instance/CPU/memory/disk/IO health."),
		mk(OpQuery, "Execute an ad-hoc SQL statement and return rows + columns."),
		mk(OpExplain, "Return SHOWPLAN (XML or text) for the given statement."),
		mk(OpSchema, "List tables and columns in the database (powers Console autocomplete)."),
		mk(OpDatabasesList, "List ONLINE databases on the instance."),
		mk(OpProcessesList, "List active user sessions on the instance (sp_who2 style)."),
		mk(OpProcessKill, "KILL the given SPID. Not recoverable."),
		mk(OpTraceStart, "Start an Extended Events trace and return the trace handle."),
		mk(OpTraceList, "List active and recently-stopped traces."),
		mk(OpTraceGet, "Fetch a trace's buffered events. Pass {since:<lastKey>} to tail incrementally."),
		mk(OpTraceStop, "Stop a running trace. Returns the final TraceResult."),
		mk(OpTraceDelete, "Stop and remove a trace from the registry."),
		{
			Name:        OpTraceStream,
			Description: "Stream events from a running Extended Events trace.",
			Scope:       "config",
			ResultMime:  "text/event-stream",
			Http: []*pluginpb.HTTPBinding{
				{Method: http.MethodGet},
			},
		},
		mk(OpPermissions, "Diagnose SQL Server permissions for stats, traces, health scans, fixes, and defrag; returns missing GRANT statements."),
		mk(OpRollbackList, "List recorded DROP INDEX rollback entries from dbo.MCAuditLog for a single database."),
		mk(OpRollbackRestore, "Restore a dropped index by running rollback SQL from dbo.MCAuditLog asynchronously. Returns a job handle."),
		mk(OpDefragHealth, "Scan index health: fragmentation, stale stats, duplicate/unused indexes, and table/index sizes; returns recommended fixes."),
		mk(OpDefragFix, "Apply selected health fixes (rebuild, reorganize, update stats, audited DROP INDEX) asynchronously. Returns a job handle."),
		mk(OpDefragBulkRebuild, "Force whole-table rebuild/update-statistics fixes for selected tables asynchronously. Returns a job handle."),
		mk(OpDefragFixJobs, "List health-fix jobs and rollback-restore jobs the plugin has started."),
		mk(OpDefragRollbacks, "List recorded DROP INDEX rollback entries for a database."),
		mk(OpDefragRollbackRestore, "Restore a dropped index from a recorded rollback entry asynchronously. Returns a job handle."),
		mk(OpDefragFixStop, "Stop a running health-fix job, or all running health-fix jobs when id is omitted."),
		mk(OpDefragInstall, "Install Microsoft TigerToolbox AdaptiveIndexDefrag into the maintenance DB."),
		mk(OpDefragRun, "Run AdaptiveIndexDefrag asynchronously. Returns a job handle."),
		mk(OpDefragStatus, "Read AdaptiveIndexDefrag installation/configuration status."),
		mk(OpDefragStats, "Read fragmentation stats from the dba_indexDefragLog tables."),
		mk(OpDefragHistory, "List recent AdaptiveIndexDefrag history rows."),
		mk(OpDefragSessions, "List currently-running AdaptiveIndexDefrag sessions on the instance."),
		mk(OpDefragTerminate, "KILL existing AdaptiveIndexDefrag sessions on the instance."),
		mk(OpDefragJobs, "List defrag jobs the plugin has started."),
		mk(OpDefragStop, "Stop all running defrag jobs (this plugin process only)."),
	}
}
