import { Database, Gauge, HardDrive, ListChecks, RefreshCw, Server, Table2, TrendingUp } from "lucide-react";
import { formatBytes, formatNumber, formatPercent } from "../lib/format";

export interface HealthSummaryView {
  health: {
    database: string;
    scanMode: string;
    tables?: HealthSummaryTable[];
  };
  engineEdition: number;
  onlineRebuild: boolean;
  productMajorVersion: number;
  uptimeDays: number;
  usageReliable: boolean;
  warnings?: string[];
}

interface HealthSummaryTable {
  rows: number;
  totalBytes: number;
  dataBytes: number;
  indexBytes: number;
  unusedBytes: number;
  maxFragmentation: number;
  fragHealthy: boolean;
  statsHealthy: boolean;
}

interface SummaryMetric {
  label: string;
  value: string;
  hint?: string;
  tone?: "neutral" | "good" | "warn" | "bad";
  icon: typeof Database;
}

export function HealthSummary({ view }: { view: HealthSummaryView }) {
  const tables = view.health.tables ?? [];
  const totals = tables.reduce(
    (acc, t) => {
      acc.rows += t.rows;
      acc.totalBytes += t.totalBytes;
      acc.dataBytes += t.dataBytes;
      acc.indexBytes += t.indexBytes;
      acc.unusedBytes += t.unusedBytes;
      if (!t.fragHealthy) acc.fragmented++;
      if (!t.statsHealthy) acc.staleStats++;
      if (t.maxFragmentation > acc.worstFrag) acc.worstFrag = t.maxFragmentation;
      return acc;
    },
    {
      rows: 0,
      totalBytes: 0,
      dataBytes: 0,
      indexBytes: 0,
      unusedBytes: 0,
      fragmented: 0,
      staleStats: 0,
      worstFrag: 0,
    },
  );

  const metrics: SummaryMetric[] = [
    {
      label: "Database",
      value: view.health.database || "—",
      hint: `scan ${view.health.scanMode}`,
      icon: Database,
    },
    { label: "Tables", value: formatNumber(tables.length), hint: `${formatNumber(totals.rows)} rows`, icon: Table2 },
    { label: "Total size", value: formatBytes(totals.totalBytes), hint: "reserved", icon: HardDrive },
    { label: "Data size", value: formatBytes(totals.dataBytes), hint: "table data", icon: HardDrive },
    { label: "Index size", value: formatBytes(totals.indexBytes), hint: "all indexes", icon: ListChecks },
    { label: "Unused", value: formatBytes(totals.unusedBytes), hint: "allocated free", icon: HardDrive },
    {
      label: "Fragmented",
      value: formatNumber(totals.fragmented),
      hint: "tables",
      icon: TrendingUp,
      tone: totals.fragmented > 0 ? "bad" : "good",
    },
    {
      label: "Stale stats",
      value: formatNumber(totals.staleStats),
      hint: "tables",
      icon: RefreshCw,
      tone: totals.staleStats > 0 ? "warn" : "good",
    },
    { label: "Worst frag", value: formatPercent(totals.worstFrag), hint: "max index", icon: Gauge },
    {
      label: "Rebuild mode",
      value: view.onlineRebuild ? "Online" : "Offline",
      hint: `edition ${view.engineEdition || "?"}`,
      icon: Server,
      tone: view.onlineRebuild ? "good" : "warn",
    },
  ];

  return (
    <section className="rounded-lg border border-border bg-card p-density-2">
      <div className="mb-density-2 flex flex-wrap items-center justify-between gap-density-2">
        <h4 className="m-0 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Health summary</h4>
        <div className="text-xs text-muted-foreground">
          SQL {view.productMajorVersion || "?"} · uptime {view.uptimeDays || "?"}d
          {!view.usageReliable && " · usage counters may be reset/noisy"}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-density-2 sm:grid-cols-2 lg:grid-cols-5">
        {metrics.map((metric) => (
          <SummaryTile key={metric.label} metric={metric} />
        ))}
      </div>

      {view.warnings?.length ? (
        <ul className="mt-density-2 m-0 pl-4 text-xs text-amber-600">
          {view.warnings.map((w, i) => <li key={i}>{w}</li>)}
        </ul>
      ) : null}
    </section>
  );
}

function SummaryTile({ metric }: { metric: SummaryMetric }) {
  const Icon = metric.icon;
  const toneClass =
    metric.tone === "bad"
      ? "text-red-600"
      : metric.tone === "warn"
        ? "text-amber-600"
        : metric.tone === "good"
          ? "text-emerald-600"
          : "text-foreground";

  return (
    <div className="rounded-md border border-border/70 bg-background/60 p-density-2 shadow-sm">
      <div className="mb-density-1 flex items-center gap-density-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        <Icon size={13} />
        {metric.label}
      </div>
      <div className={`truncate font-mono text-lg font-semibold leading-tight ${toneClass}`}>{metric.value}</div>
      {metric.hint && <div className="mt-1 truncate text-[11px] text-muted-foreground">{metric.hint}</div>}
    </div>
  );
}
