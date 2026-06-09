import { RefreshCw, Square, Wrench } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { Card, ErrorBox } from "../../pages/StatsTab";
import type { FixJob } from "./types";
import { EmptyState, monoTd, tdCls, thCls } from "./shared";

export function HealthFixJobsPanel({
  jobs,
  isFetching,
  error,
  onRefresh,
  onStop,
  onStopAll,
}: {
  jobs: FixJob[];
  isFetching: boolean;
  error: Error | null;
  onRefresh: () => void;
  onStop: (id: string) => void;
  onStopAll: () => void;
}) {
  return (
    <Card title="Health-fix jobs" icon={<Wrench size={14} />}>
      <div className="mb-density-1 flex gap-density-1">
        <Button variant="outline" size="sm" onClick={onRefresh}>
          <RefreshCw size={12} className={isFetching ? "spin" : ""} /> Refresh
        </Button>
        <Button variant="outline" size="sm" onClick={onStopAll}>
          <Square size={12} /> Stop all
        </Button>
      </div>
      {error && <ErrorBox error={error} />}
      <JobsTable jobs={jobs} onStop={onStop} />
    </Card>
  );
}

function JobsTable({
  jobs,
  onStop,
}: {
  jobs: FixJob[];
  onStop: (id: string) => void;
}) {
  if (!jobs.length) {
    return (
      <EmptyState
        title="No health-fix jobs yet"
        body="Jobs appear here after applying recommended fixes or queuing manual maintenance."
      />
    );
  }
  return (
    <table className="w-full border-collapse text-xs">
      <thead>
        <tr className="bg-muted/30">
          {"ID Status Database Started Progress Actions".split(" ").map((h) => (
            <th key={h} className={thCls}>
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {jobs.map((j) => (
          <tr key={j.id} className="border-t border-border">
            <td className={monoTd}>{j.id}</td>
            <td
              className={
                tdCls +
                " " +
                (j.status === "failed"
                  ? "text-destructive"
                  : j.status === "running"
                    ? "text-emerald-600"
                    : "")
              }
            >
              {j.status}
            </td>
            <td className={monoTd}>{j.database}</td>
            <td className={monoTd}>{new Date(j.startedAt).toLocaleString()}</td>
            <td className={tdCls}>
              {j.summary.applied}/{j.summary.total} applied
              {j.summary.failed ? (
                <span className="text-destructive">
                  {" "}
                  · {j.summary.failed} failed
                </span>
              ) : null}
              {j.error ? (
                <div className="text-destructive">{j.error}</div>
              ) : null}
            </td>
            <td className={tdCls}>
              {j.status === "running" && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onStop(j.id)}
                >
                  <Square size={12} /> Stop
                </Button>
              )}
              {(j.results ?? []).some((r) => r.error) && (
                <details className="mt-density-1">
                  <summary>errors</summary>
                  <ul className="m-0 pl-4 text-destructive">
                    {(j.results ?? [])
                      .filter((r) => r.error)
                      .map((r, i) => (
                        <li key={i}>
                          {r.fix.target}: {r.error}
                        </li>
                      ))}
                  </ul>
                </details>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
