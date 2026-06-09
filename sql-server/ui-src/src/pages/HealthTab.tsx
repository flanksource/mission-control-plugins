import { useEffect, useMemo, useState } from "preact/hooks";
import { useMutation, useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import { Hammer, Loader2, RefreshCw, Search, ShieldAlert, Square, Wrench } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { callOp, configIDFromURL } from "../lib/api";
import { formatBytes, formatNumber, formatPercent } from "../lib/format";
import { DatabasePicker } from "./DatabasePicker";
import { HealthSummary } from "./HealthSummary";
import { Card, ErrorBox } from "./StatsTab";

interface PermissionReport {
  login: string;
  maintenanceDatabase: string;
  isSysadmin: boolean;
  engineEdition: number;
  categories: PermissionCategory[];
  warnings?: string[];
}

interface PermissionCategory {
  category: string;
  label: string;
  granted: boolean;
  missingPermissions?: string[];
  grantStatements?: string[];
  note?: string;
}

interface HealthView {
  health: HealthResult;
  fixes?: Fix[];
  engineEdition: number;
  onlineRebuild: boolean;
  productMajorVersion: number;
  uptimeDays: number;
  usageReliable: boolean;
  warnings?: string[];
}

interface HealthResult {
  database: string;
  scanMode: string;
  table?: string;
  tables?: TableHealth[];
}

interface TableHealth {
  schema: string;
  tableName: string;
  rows: number;
  totalBytes: number;
  dataBytes: number;
  indexBytes: number;
  unusedBytes: number;
  maxFragmentation: number;
  fragHealthy: boolean;
  statsHealthy: boolean;
  indexes?: IndexHealth[];
  statistics?: StatHealth[];
}

interface IndexHealth {
  name: string;
  type: string;
  fragmentation: number;
  pageCount: number;
  bytes: number;
  bad: boolean;
  seeks: number;
  scans: number;
  lookups: number;
  updates: number;
  duplicate: boolean;
  duplicateOf?: string;
  unused: boolean;
  dropCandidate: boolean;
  keyColumns?: string[];
  includedColumns?: string[];
}

interface StatHealth {
  name: string;
  lastUpdated?: string;
  rows: number;
  rowsSampled: number;
  modifications: number;
  pctSampled: number;
  pctChanged: number;
  stale: boolean;
}

interface Fix {
  kind: string;
  schema: string;
  table: string;
  target: string;
  detail: string;
  sample?: string;
  sql: string;
  rollback?: string;
}

interface FixJob {
  id: string;
  status: "running" | "done" | "failed" | "stopped";
  database: string;
  startedAt: string;
  finishedAt?: string;
  error?: string;
  fixes?: Fix[];
  results?: FixResult[];
  summary: {
    total: number;
    applied: number;
    failed: number;
    rebuilds: number;
    reorganizes: number;
    updateStats: number;
    dropIndexes: number;
  };
}

interface FixResult {
  fix: Fix;
  applied: boolean;
  error?: string;
  messages?: string[];
}

const inputCls = "h-control-h rounded-md border border-input bg-background px-2 text-sm";
const labelCls = "inline-flex items-center gap-density-1 text-xs text-muted-foreground";
const thCls = "px-density-2 py-density-1 text-left font-semibold text-foreground";
const tdCls = "px-density-2 py-density-1 align-top text-xs";
const monoTd = tdCls + " font-mono";

export function HealthTab() {
  const configID = configIDFromURL();
  const qc = useQueryClient();
  const [database, setDatabase] = useState("");
  const [table, setTable] = useState("");
  const [scanMode, setScanMode] = useState("LIMITED");
  const [limit, setLimit] = useState(20);
  const [selectedFixes, setSelectedFixes] = useState<Set<number>>(new Set());
  const [selectedTables, setSelectedTables] = useState<Set<string>>(new Set());
  const [bulkRebuildIndexes, setBulkRebuildIndexes] = useState(true);
  const [bulkUpdateStats, setBulkUpdateStats] = useState(true);

  const permissions = useQuery({
    queryKey: ["permissions", configID],
    queryFn: () => callOp<PermissionReport>("permissions", configID, {}),
    staleTime: 60_000,
    retry: 0,
  });

  const health = useQuery({
    queryKey: ["defrag-health", configID, database, table, scanMode, limit],
    queryFn: () =>
      callOp<HealthView>("defrag-health", configID, {
        database,
        table,
        scanMode,
        limit,
      }),
    enabled: false,
    retry: 0,
  });

  const jobs = useQuery({
    queryKey: ["defrag-fix-jobs", configID],
    queryFn: () => callOp<FixJob[]>("defrag-fix-jobs", configID, {}),
    refetchInterval: 3_000,
  });

  const applyFixes = useMutation({
    mutationFn: (fixes: Fix[]) =>
      callOp<FixJob>("defrag-fix", configID, {
        database: health.data?.health.database || database,
        fixes,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["defrag-fix-jobs"] }),
  });

  const bulk = useMutation({
    mutationFn: (tables: { schema: string; table: string }[]) =>
      callOp<FixJob>("defrag-bulk-rebuild", configID, {
        database: health.data?.health.database || database,
        tables,
        rebuildIndexes: bulkRebuildIndexes,
        updateStats: bulkUpdateStats,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["defrag-fix-jobs"] }),
  });

  const stopJobs = useMutation({
    mutationFn: (id?: string) => callOp("defrag-fix-stop", configID, id ? { id } : {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["defrag-fix-jobs"] }),
  });

  const view = health.data;
  const fixes = view?.fixes ?? [];
  const tables = view?.health.tables ?? [];

  useEffect(() => {
    // Do not pre-select DROP INDEX recommendations; they are useful diagnostics
    // but should be an explicit operator choice until rollback restore is moved.
    setSelectedFixes(new Set(defaultSelectedFixIndexes(fixes)));
    setSelectedTables(new Set<string>());
  }, [view]);

  const applyableFixIndexes = useMemo(() => defaultSelectedFixIndexes(fixes), [fixes]);
  const selectedFixList = useMemo(
    () => [...selectedFixes].map((i) => fixes[i]).filter(Boolean),
    [selectedFixes, fixes],
  );
  const selectedTableRefs = useMemo(
    () =>
      tables
        .filter((t) => selectedTables.has(tableKey(t)))
        .map((t) => ({ schema: t.schema, table: t.tableName })),
    [selectedTables, tables],
  );

  const detailedNeedsTable = scanMode === "DETAILED" && table.trim() === "";

  return (
    <section className="grid gap-density-2">
      <PermissionsPanel query={permissions} />

      <Card title="Index health scan" icon={<Search size={14} />}>
        <div className="flex flex-wrap items-end gap-density-2">
          <label className={labelCls}>
            database
            <DatabasePicker
              configID={configID}
              value={database}
              onChange={setDatabase}
              emptyLabel="Current / bound database"
            />
          </label>
          <label className={labelCls}>
            table
            <input
              value={table}
              onChange={(e) => setTable(e.currentTarget.value)}
              placeholder="dbo.Table (optional)"
              className={inputCls + " w-[180px]"}
            />
          </label>
          <label className={labelCls}>
            scan
            <select value={scanMode} onChange={(e) => setScanMode(e.currentTarget.value)} className={inputCls}>
              <option value="LIMITED">LIMITED (fast)</option>
              <option value="SAMPLED">SAMPLED</option>
              <option value="DETAILED">DETAILED (table required)</option>
            </select>
          </label>
          <label className={labelCls}>
            top tables
            <input
              type="number"
              min={1}
              max={500}
              value={limit}
              onChange={(e) => setLimit(parseInt(e.currentTarget.value) || 20)}
              className={inputCls + " w-[80px]"}
            />
          </label>
          <Button size="sm" onClick={() => health.refetch()} disabled={health.isFetching || detailedNeedsTable}>
            {health.isFetching ? <Loader2 size={12} className="spin" /> : <Search size={12} />} Scan
          </Button>
        </div>
        {detailedNeedsTable && (
          <p className="mt-density-1 text-xs text-amber-600">DETAILED scan requires a table.</p>
        )}
        {health.error && <ErrorBox error={health.error as Error} />}
      </Card>

      {view && <HealthSummary view={view} />}
      {view && (
        <TablesPanel
          tables={tables}
          usageReliable={view.usageReliable}
          selected={selectedTables}
          onToggle={(key) => toggleSet(selectedTables, setSelectedTables, key)}
          onToggleAll={() =>
            setSelectedTables(selectedTables.size === tables.length ? new Set<string>() : new Set(tables.map(tableKey)))
          }
        />
      )}
      {view && (
        <FixesPanel
          fixes={fixes}
          selected={selectedFixes}
          onToggle={(i) => toggleSet(selectedFixes, setSelectedFixes, i)}
          onToggleAll={() =>
            setSelectedFixes(
              selectedFixes.size === applyableFixIndexes.length ? new Set<number>() : new Set(applyableFixIndexes),
            )
          }
          onApply={() => applyFixes.mutate(selectedFixList)}
          applying={applyFixes.isPending}
          applyError={applyFixes.error as Error | null}
        />
      )}
      {view && (
        <Card title="Bulk rebuild / update stats" icon={<Hammer size={14} />}>
          <div className="flex flex-wrap items-center gap-density-2 text-xs">
            <label className="inline-flex items-center gap-density-1">
              <input
                type="checkbox"
                checked={bulkRebuildIndexes}
                onChange={(e) => setBulkRebuildIndexes(e.currentTarget.checked)}
              />
              rebuild indexes
            </label>
            <label className="inline-flex items-center gap-density-1">
              <input
                type="checkbox"
                checked={bulkUpdateStats}
                onChange={(e) => setBulkUpdateStats(e.currentTarget.checked)}
              />
              update stats
            </label>
            <Button
              size="sm"
              variant="outline"
              disabled={bulk.isPending || selectedTableRefs.length === 0 || (!bulkRebuildIndexes && !bulkUpdateStats)}
              onClick={() => bulk.mutate(selectedTableRefs)}
            >
              {bulk.isPending ? <Loader2 size={12} className="spin" /> : <Hammer size={12} />} Queue for {selectedTableRefs.length} table(s)
            </Button>
            <span className="text-muted-foreground">Select tables in the table list above.</span>
          </div>
          {bulk.error && <ErrorBox error={bulk.error as Error} />}
        </Card>
      )}

      <Card title="Health-fix jobs" icon={<Wrench size={14} />}>
        <div className="mb-density-1 flex gap-density-1">
          <Button variant="outline" size="sm" onClick={() => jobs.refetch()}>
            <RefreshCw size={12} className={jobs.isFetching ? "spin" : ""} /> Refresh
          </Button>
          <Button variant="outline" size="sm" onClick={() => stopJobs.mutate(undefined)}>
            <Square size={12} /> Stop all
          </Button>
        </div>
        {jobs.error && <ErrorBox error={jobs.error as Error} />}
        <JobsTable jobs={jobs.data ?? []} onStop={(id) => stopJobs.mutate(id)} />
      </Card>
    </section>
  );
}

function PermissionsPanel({ query }: { query: UseQueryResult<PermissionReport, Error> }) {
  if (query.error) return <ErrorBox error={query.error as Error} />;
  const report = query.data;
  if (!report) return null;
  const missing = report.categories.filter((c) => !c.granted);
  if (missing.length === 0 && !report.warnings?.length) {
    return null;
  }
  return (
    <Card
      title="Permission diagnostics"
      icon={<ShieldAlert size={14} />}
      className={missing.length ? "border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-950/30" : ""}
    >
      {missing.length ? (
        <div className="grid gap-density-2 md:grid-cols-2">
          {missing.map((c) => (
            <div key={c.category} className="rounded-md border border-amber-300 bg-background/70 p-density-2 text-xs">
              <div className="font-semibold text-amber-700 dark:text-amber-300">{c.label}</div>
              {c.missingPermissions?.length ? (
                <div className="mt-density-1 text-muted-foreground">Missing: {c.missingPermissions.join(", ")}</div>
              ) : null}
              {c.note && <div className="mt-density-1 italic text-muted-foreground">{c.note}</div>}
              {c.grantStatements?.length ? (
                <pre className="mt-density-1 overflow-auto rounded bg-muted/50 p-density-1 text-[11px]">
                  {c.grantStatements.join("\n")}
                </pre>
              ) : null}
            </div>
          ))}
        </div>
      ) : (
        <p className="m-0 text-xs text-muted-foreground">All probed SQL Server capabilities are granted.</p>
      )}
      {report.warnings?.length ? (
        <ul className="mt-density-2 m-0 pl-4 text-xs text-amber-700 dark:text-amber-300">
          {report.warnings.map((w, i) => <li key={i}>{w}</li>)}
        </ul>
      ) : null}
    </Card>
  );
}

function TablesPanel({
  tables,
  usageReliable,
  selected,
  onToggle,
  onToggleAll,
}: {
  tables: TableHealth[];
  usageReliable: boolean;
  selected: Set<string>;
  onToggle: (key: string) => void;
  onToggleAll: () => void;
}) {
  if (!tables.length) {
    return <Card title="Tables"><em className="text-muted-foreground">no matching tables</em></Card>;
  }
  return (
    <Card title={`Tables (${tables.length})`}>
      <div className="mb-density-1 flex items-center gap-density-2 text-xs">
        <label className="inline-flex items-center gap-density-1">
          <input type="checkbox" checked={selected.size === tables.length} onChange={onToggleAll} /> select all for bulk
        </label>
        {!usageReliable && <span className="text-amber-600">unused-index flags need verification: usage counters may have reset</span>}
      </div>
      <div className="overflow-auto">
        <table className="w-full border-collapse text-xs">
          <thead>
            <tr className="bg-muted/30">
              <th className={thCls}></th>
              {"Table Rows Total Index Unused MaxFrag Frag Stats".split(" ").map((h) => <th key={h} className={thCls}>{h}</th>)}
            </tr>
          </thead>
          <tbody>
            {tables.map((t) => {
              const key = tableKey(t);
              return (
                <tr key={key} className="border-t border-border">
                  <td className={tdCls}>
                    <input type="checkbox" checked={selected.has(key)} onChange={() => onToggle(key)} />
                  </td>
                  <td className={monoTd}>
                    <details>
                      <summary className="cursor-pointer">{key}</summary>
                      <IndexDetails table={t} />
                    </details>
                  </td>
                  <td className={monoTd}>{formatNumber(t.rows)}</td>
                  <td className={monoTd}>{formatBytes(t.totalBytes)}</td>
                  <td className={monoTd}>{formatBytes(t.indexBytes)}</td>
                  <td className={monoTd}>{formatBytes(t.unusedBytes)}</td>
                  <td className={monoTd}>{formatPercent(t.maxFragmentation)}</td>
                  <td className={tdCls}><Mark ok={t.fragHealthy} /></td>
                  <td className={tdCls}><Mark ok={t.statsHealthy} /></td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

function IndexDetails({ table }: { table: TableHealth }) {
  return (
    <div className="mt-density-1 grid gap-density-1 text-[11px] text-muted-foreground">
      {(table.indexes ?? []).length > 0 && (
        <div>
          <strong>Indexes</strong>
          <ul className="m-0 mt-1 pl-4">
            {(table.indexes ?? []).map((ix) => (
              <li key={ix.name}>
                <span className={ix.bad || ix.dropCandidate ? "text-amber-600" : ""}>{ix.name}</span> · {formatPercent(ix.fragmentation)} · {formatBytes(ix.bytes)} · reads {formatNumber(ix.seeks + ix.scans + ix.lookups)} · writes {formatNumber(ix.updates)}
                {ix.duplicate && ` · duplicate of ${ix.duplicateOf || "wider index"}`}
                {ix.unused && " · unused?"}
              </li>
            ))}
          </ul>
        </div>
      )}
      {(table.statistics ?? []).length > 0 && (
        <div>
          <strong>Statistics</strong>
          <ul className="m-0 mt-1 pl-4">
            {(table.statistics ?? []).map((s) => (
              <li key={s.name} className={s.stale ? "text-amber-600" : ""}>
                {s.name} · changed {formatPercent(s.pctChanged)} · sampled {formatPercent(s.pctSampled)} · updated {s.lastUpdated ? new Date(s.lastUpdated).toLocaleDateString() : "never"}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function FixesPanel({
  fixes,
  selected,
  onToggle,
  onToggleAll,
  onApply,
  applying,
  applyError,
}: {
  fixes: Fix[];
  selected: Set<number>;
  onToggle: (i: number) => void;
  onToggleAll: () => void;
  onApply: () => void;
  applying: boolean;
  applyError: Error | null;
}) {
  return (
    <Card title={`Recommended fixes (${fixes.length})`} icon={<Wrench size={14} />}>
      {fixes.length === 0 ? (
        <em className="text-muted-foreground">no recommended fixes</em>
      ) : (
        <>
          <div className="mb-density-1 flex items-center gap-density-2 text-xs">
            <label className="inline-flex items-center gap-density-1">
              <input type="checkbox" checked={selected.size > 0 && selected.size === defaultSelectedFixIndexes(fixes).length} onChange={onToggleAll} /> select rebuild/reorg/stats
            </label>
            {fixes.some((f) => f.kind.includes("DROP")) && (
              <span className="text-muted-foreground">DROP INDEX recommendations are diagnostic-only until rollback restore is migrated.</span>
            )}
            <Button size="sm" disabled={applying || selected.size === 0} onClick={onApply}>
              {applying ? <Loader2 size={12} className="spin" /> : <Wrench size={12} />} Apply selected ({selected.size})
            </Button>
          </div>
          <div className="max-h-[360px] overflow-auto rounded-md border border-border">
            <table className="w-full border-collapse text-xs">
              <thead>
                <tr className="bg-muted/30">
                  <th className={thCls}></th>
                  {"Kind Table Target Detail SQL".split(" ").map((h) => <th key={h} className={thCls}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {fixes.map((f, i) => {
                  const isDrop = f.kind.includes("DROP");
                  return (
                  <tr key={`${f.kind}-${f.schema}-${f.table}-${f.target}-${i}`} className="border-t border-border">
                    <td className={tdCls}><input type="checkbox" disabled={isDrop} checked={selected.has(i)} onChange={() => onToggle(i)} /></td>
                    <td className={tdCls + fixTone(f.kind)}>{f.kind}</td>
                    <td className={monoTd}>{f.schema}.{f.table}</td>
                    <td className={monoTd}>{f.target}</td>
                    <td className={tdCls}>{f.detail}</td>
                    <td className={monoTd}><code>{f.sql}</code>{f.rollback && <div className="mt-1 text-muted-foreground">rollback: {f.rollback}</div>}</td>
                  </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}
      {applyError && <ErrorBox error={applyError} />}
    </Card>
  );
}

function JobsTable({ jobs, onStop }: { jobs: FixJob[]; onStop: (id: string) => void }) {
  if (!jobs.length) return <em className="text-muted-foreground">no health-fix jobs yet</em>;
  return (
    <table className="w-full border-collapse text-xs">
      <thead>
        <tr className="bg-muted/30">
          {"ID Status Database Started Progress Actions".split(" ").map((h) => <th key={h} className={thCls}>{h}</th>)}
        </tr>
      </thead>
      <tbody>
        {jobs.map((j) => (
          <tr key={j.id} className="border-t border-border">
            <td className={monoTd}>{j.id}</td>
            <td className={tdCls + " " + (j.status === "failed" ? "text-destructive" : j.status === "running" ? "text-emerald-600" : "")}>{j.status}</td>
            <td className={monoTd}>{j.database}</td>
            <td className={monoTd}>{new Date(j.startedAt).toLocaleString()}</td>
            <td className={tdCls}>
              {j.summary.applied}/{j.summary.total} applied
              {j.summary.failed ? <span className="text-destructive"> · {j.summary.failed} failed</span> : null}
              {j.error ? <div className="text-destructive">{j.error}</div> : null}
            </td>
            <td className={tdCls}>
              {j.status === "running" && (
                <Button variant="outline" size="sm" onClick={() => onStop(j.id)}><Square size={12} /> Stop</Button>
              )}
              {(j.results ?? []).some((r) => r.error) && (
                <details className="mt-density-1">
                  <summary>errors</summary>
                  <ul className="m-0 pl-4 text-destructive">
                    {(j.results ?? []).filter((r) => r.error).map((r, i) => <li key={i}>{r.fix.target}: {r.error}</li>)}
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

function Mark({ ok }: { ok: boolean }) {
  return ok ? <span className="text-green-600">✓</span> : <span className="text-red-600">✗</span>;
}

function tableKey(t: TableHealth): string {
  return `${t.schema}.${t.tableName}`;
}

function toggleSet<T>(value: Set<T>, setValue: (next: Set<T>) => void, item: T) {
  const next = new Set(value);
  if (next.has(item)) next.delete(item);
  else next.add(item);
  setValue(next);
}

function defaultSelectedFixIndexes(fixes: Fix[]): number[] {
  const selected: number[] = [];
  fixes.forEach((f, i) => {
    if (!f.kind.includes("DROP")) selected.push(i);
  });
  return selected;
}

function fixTone(kind: string): string {
  if (kind.includes("DROP")) return " text-red-600";
  if (kind.includes("REBUILD")) return " text-amber-600";
  return "";
}
