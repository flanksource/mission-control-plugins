import { useEffect, useMemo, useState } from "preact/hooks";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, RefreshCw, RotateCcw, Undo2 } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { callOp, configIDFromURL } from "../lib/api";
import { Card, ErrorBox } from "./StatsTab";
import { DatabasePicker } from "./DatabasePicker";
import type { FixJob, RollbackEntry, RollbacksResponse } from "../components/Health/types";
import { EmptyState, monoTd, tdCls, thCls } from "../components/Health/shared";

export function RollbacksTab() {
  const configID = configIDFromURL();
  const qc = useQueryClient();
  const [database, setDatabase] = useState("");
  const [restoreJobID, setRestoreJobID] = useState<string | null>(null);

  const rollbacks = useQuery({
    queryKey: ["defrag-rollbacks", configID, database],
    queryFn: () => callOp<RollbacksResponse>("defrag-rollbacks", configID, { database }),
    retry: 0,
  });

  const jobs = useQuery({
    queryKey: ["defrag-fix-jobs", configID],
    queryFn: () => callOp<FixJob[]>("defrag-fix-jobs", configID, {}),
    refetchInterval: restoreJobID ? 2_000 : 5_000,
  });

  const restore = useMutation({
    mutationFn: (entry: RollbackEntry) =>
      callOp<FixJob>("defrag-rollback-restore", configID, {
        database: entry.database || rollbacks.data?.database || database,
        id: entry.id,
      }),
    onSuccess: (job) => {
      setRestoreJobID(job.id);
      void qc.invalidateQueries({ queryKey: ["defrag-fix-jobs"] });
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

  return (
    <section className="grid gap-density-2">
      <Card title="Dropped-index rollbacks" icon={<Undo2 size={14} />}>
        <div className="flex flex-wrap items-center gap-density-2">
          <DatabasePicker
            configID={configID}
            value={database}
            onChange={setDatabase}
            emptyLabel="Default database"
            disabled={rollbacks.isFetching}
          />
          <Button
            variant="outline"
            size="sm"
            onClick={() => rollbacks.refetch()}
            disabled={rollbacks.isFetching}
          >
            <RefreshCw size={12} className={rollbacks.isFetching ? "spin" : ""} /> Load
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
          onRestore={(entry) => restore.mutate(entry)}
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
                <th className={thCls}>Index</th>
                <th className={thCls}>Table</th>
                <th className={thCls}>Reason</th>
                <th className={thCls}>Dropped</th>
                <th className={thCls}>Restored</th>
                <th className={thCls}>Rollback SQL</th>
                <th className={thCls}></th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id} className="border-t border-border">
                  <td className={monoTd}>{entry.objectName}</td>
                  <td className={monoTd}>{entry.schema}.{entry.table}</td>
                  <td className={tdCls}>{entry.reason || "—"}</td>
                  <td className={tdCls} title={formatDate(entry.createdAt)}>{formatAge(entry.createdAt)}</td>
                  <td className={tdCls} title={formatDate(entry.restoredAt)}>
                    {entry.restoredAt ? formatAge(entry.restoredAt) : <span className="text-amber-600">not restored</span>}
                  </td>
                  <td className={monoTd}><code>{entry.rollbackSql}</code></td>
                  <td className={tdCls + " text-right"}>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={restoring || Boolean(entry.restoredAt)}
                      onClick={() => onRestore(entry)}
                    >
                      {restoring ? <Loader2 size={12} className="spin" /> : <RotateCcw size={12} />} Restore
                    </Button>
                  </td>
                </tr>
              ))}
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
