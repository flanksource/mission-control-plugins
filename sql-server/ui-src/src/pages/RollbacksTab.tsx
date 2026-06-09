import { useEffect, useMemo, useState } from "preact/hooks";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, RefreshCw, RotateCcw, Undo2 } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { callOp, configIDFromURL } from "../lib/api";
import { Card, ErrorBox } from "./StatsTab";
import { DatabasePicker } from "../components/DatabasePicker";
import type { FixJob, RollbackEntry, RollbacksResponse } from "../components/Health/types";
import { EmptyState, monoTd, tdCls, thCls } from "../components/Health/shared";

export function RollbacksTab() {
  const configID = configIDFromURL();
  const qc = useQueryClient();
  const [database, setDatabase] = useState("");
  const [limit, setLimit] = useState(50);
  const [restoreJobID, setRestoreJobID] = useState<string | null>(null);

  const rollbacks = useQuery({
    queryKey: ["rollback-list", configID, database, limit],
    queryFn: () => callOp<RollbacksResponse>("rollback-list", configID, { database, limit }),
    retry: 0,
  });

  const jobs = useQuery({
    queryKey: ["defrag-fix-jobs", configID],
    queryFn: () => callOp<FixJob[]>("defrag-fix-jobs", configID, {}),
    refetchInterval: restoreJobID ? 2_000 : 5_000,
  });

  const restore = useMutation({
    mutationFn: (entry: RollbackEntry) =>
      callOp<FixJob>("rollback-restore", configID, {
        database: entry.database || rollbacks.data?.database || database,
        id: entry.id,
      }),
    onSuccess: (job) => {
      setRestoreJobID(job.id);
      void qc.invalidateQueries({ queryKey: ["defrag-fix-jobs", configID] });
    },
  });

  const restoreJob = useMemo(
    () => jobs.data?.find((j) => j.id === restoreJobID),
    [jobs.data, restoreJobID],
  );

  useEffect(() => {
    if (restoreJob && restoreJob.status !== "running") {
      void rollbacks.refetch();
    }
  }, [restoreJob?.status]);

  const entries = rollbacks.data?.rollbacks ?? [];

  const confirmRestore = (entry: RollbackEntry) => {
    const target = `${entry.schema}.${entry.table}.${entry.objectName}`;
    const db = entry.database || rollbacks.data?.database || database;
    if (
      window.confirm(
        `Restore dropped index from audit #${entry.id}?\n\nDatabase: ${db}\nIndex: ${target}\n\nThis will run the saved RollbackSQL from dbo.MCAuditLog.`,
      )
    ) {
      restore.mutate(entry);
    }
  };

  return (
    <section className="grid gap-density-2">
      <Card title="Dropped-index rollbacks" icon={<Undo2 size={14} />}>
        <div className="flex flex-wrap items-end gap-density-2">
          <div className="grid gap-density-1">
            <label className="text-xs font-medium text-muted-foreground">Database</label>
            <DatabasePicker
              configID={configID}
              value={database}
              onChange={setDatabase}
              emptyLabel="Default database"
              disabled={rollbacks.isFetching}
            />
          </div>
          <div className="grid gap-density-1">
            <label className="text-xs font-medium text-muted-foreground" htmlFor="rollback-limit">
              Limit
            </label>
            <input
              id="rollback-limit"
              type="number"
              min={1}
              max={500}
              value={limit}
              onChange={(e) => setLimit(clampLimit(e.currentTarget.value))}
              className="h-control-h w-24 rounded-md border border-input bg-background px-2 text-sm"
              disabled={rollbacks.isFetching}
            />
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => rollbacks.refetch()}
            disabled={rollbacks.isFetching}
          >
            <RefreshCw size={12} className={rollbacks.isFetching ? "spin" : ""} /> Refresh
          </Button>
        </div>
        <p className="mt-density-2 text-xs text-muted-foreground">
          Lists successful DROP INDEX health fixes recorded in dbo.MCAuditLog. Restore runs the saved CREATE INDEX statement asynchronously.
        </p>
      </Card>

      {rollbacks.error && <ErrorBox error={rollbacks.error as Error} />}
      {restore.error && <ErrorBox error={restore.error as Error} />}

      {restoreJob && <RestoreJobCard job={restoreJob} isFetching={jobs.isFetching} />}

      {rollbacks.isLoading ? (
        <Card title="Rollbacks"><p className="m-0 text-sm text-muted-foreground">Loading…</p></Card>
      ) : rollbacks.data ? (
        <RollbacksTable
          database={rollbacks.data.database}
          entries={entries}
          restoring={restore.isPending}
          onRestore={confirmRestore}
        />
      ) : null}
    </section>
  );
}

function RollbacksTable({
  database,
  entries,
  restoring,
  onRestore,
}: {
  database: string;
  entries: RollbackEntry[];
  restoring: boolean;
  onRestore: (entry: RollbackEntry) => void;
}) {
  return (
    <Card title={`Recorded rollbacks · ${database} (${entries.length})`} icon={<Undo2 size={14} />}>
      {entries.length === 0 ? (
        <EmptyState
          title="No dropped-index rollbacks recorded"
          body={`dbo.MCAuditLog in ${database} does not contain any DROP INDEX entries.`}
        />
      ) : (
        <div className="max-h-[560px] overflow-auto rounded-md border border-border">
          <table className="w-full border-collapse text-xs">
            <thead>
              <tr className="bg-muted/30">
                <th className={thCls}>Created</th>
                <th className={thCls}>Object / index</th>
                <th className={thCls}>Table</th>
                <th className={thCls}>SQL preview</th>
                <th className={thCls}>Audit ID</th>
                <th className={thCls}>State</th>
                <th className={thCls}></th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => {
                const restored = Boolean(entry.restoredAt);
                return (
                  <tr key={entry.id} className="border-t border-border align-top">
                    <td className={tdCls} title={formatDate(entry.createdAt)}>
                      <div className="whitespace-nowrap">{formatDate(entry.createdAt)}</div>
                      <div className="text-muted-foreground">{formatAge(entry.createdAt)}</div>
                    </td>
                    <td className={monoTd}>{entry.objectName}</td>
                    <td className={monoTd}>{entry.schema}.{entry.table}</td>
                    <td className={monoTd} title={entry.rollbackSql}>
                      <code className="block max-w-[520px] whitespace-pre-wrap break-words">
                        {sqlPreview(entry.rollbackSql)}
                      </code>
                      {entry.reason && <div className="mt-density-1 font-sans text-muted-foreground">{entry.reason}</div>}
                    </td>
                    <td className={monoTd}>#{entry.id}</td>
                    <td className={tdCls} title={formatDate(entry.restoredAt)}>
                      {restored ? (
                        <span className="text-green-600">restored</span>
                      ) : (
                        <span className="text-amber-600">dropped</span>
                      )}
                    </td>
                    <td className={tdCls + " text-right"}>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={restoring || restored}
                        onClick={() => onRestore(entry)}
                      >
                        {restoring ? <Loader2 size={12} className="spin" /> : <RotateCcw size={12} />} Restore
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

function RestoreJobCard({ job, isFetching }: { job: FixJob; isFetching: boolean }) {
  const tone = job.status === "done" ? "text-green-600" : job.status === "failed" ? "text-red-600" : "text-amber-600";
  return (
    <Card title="Restore job" icon={<RotateCcw size={14} />}>
      <div className="flex flex-wrap items-center gap-density-2 text-sm">
        <span className="font-mono text-xs">{job.id}</span>
        <span className={"font-semibold " + tone}>{job.status}</span>
        {isFetching && <Loader2 size={12} className="spin" />}
      </div>
      {job.status === "done" && <div className="mt-density-1 text-sm text-green-600">Restore completed successfully.</div>}
      {job.error && <div className="mt-density-1 text-sm text-red-600">{job.error}</div>}
      {job.results?.map((r, i) => (
        <div key={i} className="mt-density-1 rounded-md bg-muted/30 p-density-1 text-xs">
          <div>
            {r.applied ? "✓" : "✗"} {r.fix.schema}.{r.fix.table} {r.fix.target}
          </div>
          {r.error && <div className="text-red-600">{r.error}</div>}
          {r.messages?.map((m, j) => <pre key={j} className="m-0 whitespace-pre-wrap font-mono text-muted-foreground">{m}</pre>)}
        </div>
      ))}
    </Card>
  );
}

function clampLimit(value: string): number {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed)) return 50;
  return Math.max(1, Math.min(500, parsed));
}

function sqlPreview(value?: string): string {
  if (!value) return "—";
  const compact = value.replace(/\s+/g, " ").trim();
  return compact.length > 220 ? compact.slice(0, 220) + "…" : compact;
}

function formatDate(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatAge(value?: string): string {
  if (!value) return "—";
  const ms = Date.now() - new Date(value).getTime();
  if (!Number.isFinite(ms)) return formatDate(value);
  const sec = Math.max(0, Math.floor(ms / 1000));
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 48) return `${hr}h ago`;
  return `${Math.floor(hr / 24)}d ago`;
}
