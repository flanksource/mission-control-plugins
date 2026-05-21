import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Database, FileJson, Play, Search } from "lucide-react";
import { Button, FilterBar, Select } from "@flanksource/clicky-ui/components";
import { DataTable, JsonView, type DataTableColumn } from "@flanksource/clicky-ui/data";
import { InlineError } from "@flanksource/clicky-ui/rpc";
import "@flanksource/clicky-ui/styles.css";
import { pluginBuildDate, pluginVersion } from "./version";
import "./styles.css";

const FIELD_CLASS = "flex min-w-40 flex-col gap-1";
const LABEL_CLASS = "text-[11px] font-semibold text-slate-500";
const INPUT_CLASS =
  "h-8 w-full rounded-md border border-slate-300 bg-white px-2 text-[13px] text-slate-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50";

type ProfileParam = {
  name: string;
  operator: string;
  description?: string;
  required?: boolean;
};

type TracingColumn = {
  name: string;
  field: string;
  detail?: boolean;
};

type ProfileSummary = {
  name: string;
  format: string;
  backend: string;
  index: string;
  params: ProfileParam[];
  columns: TracingColumn[];
  defaults: Record<string, unknown>;
};

type Trace = {
  timestamp?: string;
  trace_id?: string;
  span_id?: string;
  parent_id?: string;
  service_name?: string;
  operation_name?: string;
  status?: string;
  duration_ms?: number;
  cells?: Record<string, unknown>;
  attributes?: Record<string, unknown>;
  raw?: Record<string, unknown>;
  children?: Trace[];
};

type TraceRow = Record<string, unknown> & {
  _id: string;
  _depth: number;
  _trace: Trace;
};

type TraceResult = {
  profile: string;
  index: string;
  total: number;
  traces: Trace[];
};

type QueryPreview = {
  profile: string;
  request: {
    index: string;
    limit: number;
    query: Record<string, unknown>;
  };
};

type OperationError = Error & {
  status?: number;
  url?: string;
  responseBody?: string;
};

function configIDFromURL(): string {
  return new URLSearchParams(window.location.search).get("config_id") ?? "";
}

function operationURL(op: string): string {
  const base = window.location.pathname.replace(/\/ui\/.*$/, "");
  const url = new URL(base + "/operations/" + op, window.location.origin);
  const configID = configIDFromURL();
  if (configID) url.searchParams.set("config_id", configID);
  return url.toString();
}

async function callOp<T>(op: string, params: Record<string, unknown> = {}): Promise<T> {
  const url = operationURL(op);
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(params),
  });
  if (!res.ok) {
    const responseBody = await res.text();
    const err = new Error(responseBody || `${op} returned ${res.status}`) as OperationError;
    err.status = res.status;
    err.url = url;
    err.responseBody = responseBody;
    throw err;
  }
  return (await res.json()) as T;
}

function App() {
  const [profiles, setProfiles] = useState<ProfileSummary[]>([]);
  const [profileName, setProfileName] = useState("");
  const [params, setParams] = useState<Record<string, string>>({});
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [limit, setLimit] = useState(100);
  const [result, setResult] = useState<TraceResult | null>(null);
  const [preview, setPreview] = useState<QueryPreview | null>(null);
  const [selected, setSelected] = useState<TraceRow | null>(null);
  const [pendingAction, setPendingAction] = useState<"preview" | "query" | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    callOp<ProfileSummary[]>("profiles-list")
      .then((items) => {
        setProfiles(items);
        const first = items[0];
        if (first) {
          setProfileName(first.name);
          setParams(defaultParams(first));
        }
      })
      .catch(setError);
  }, []);

  const profile = useMemo(
    () => profiles.find((item) => item.name === profileName),
    [profiles, profileName],
  );

  const rows = useMemo(() => flattenTraces(result?.traces ?? []), [result]);
  const columns = useMemo(() => buildColumns(profile?.columns ?? []), [profile?.columns]);
  const loading = pendingAction !== null;

  function selectProfile(name: string) {
    const next = profiles.find((item) => item.name === name);
    setProfileName(name);
    setParams(next ? defaultParams(next) : {});
    setResult(null);
    setSelected(null);
    setPreview(null);
  }

  async function runPreview() {
    if (!profile) return;
    setPendingAction("preview");
    setError(null);
    try {
      const next = await callOp<QueryPreview>("query-preview", requestBody(profileName, params, from, to, limit));
      setPreview(next);
    } catch (err) {
      setError(err);
    } finally {
      setPendingAction(null);
    }
  }

  async function runQuery() {
    if (!profile) return;
    setPendingAction("query");
    setError(null);
    try {
      const next = await callOp<TraceResult>("trace-query", requestBody(profileName, params, from, to, limit));
      const nextRows = flattenTraces(next.traces);
      setResult(next);
      setSelected(nextRows[0] ?? null);
      setPreview(null);
    } catch (err) {
      setError(err);
    } finally {
      setPendingAction(null);
    }
  }

  return (
    <div className="flex min-h-screen min-w-80 flex-col bg-slate-50 font-sans text-slate-950">
      <div className="border-b border-slate-200 bg-white p-3">
        <FilterBar
          className="w-full items-end rounded-md"
          leading={
            <div className="flex min-w-44 flex-col gap-1">
              <label className={LABEL_CLASS}>Profile</label>
              <Select
                value={profileName}
                onChange={(e) => selectProfile(e.target.value)}
                options={profiles.map((item) => ({ value: item.name, label: item.name }))}
                disabled={loading || profiles.length === 0}
              />
            </div>
          }
          trailing={
            <div className="flex items-end gap-2 max-[860px]:w-full">
              <Button
                type="button"
                variant="outline"
                onClick={runPreview}
                disabled={loading || !profile}
                loading={pendingAction === "preview"}
                loadingLabel="Previewing"
              >
                <FileJson size={15} />
                Preview
              </Button>
              <Button
                type="button"
                onClick={runQuery}
                disabled={loading || !profile}
                loading={pendingAction === "query"}
                loadingLabel="Querying"
              >
                <Play size={15} />
                Query
              </Button>
              <div className="self-center whitespace-nowrap text-xs text-slate-500">
                v{pluginVersion}
                {pluginBuildDate ? ` ${pluginBuildDate}` : ""}
              </div>
            </div>
          }
        >
          <TraceTextField label="From" value={from} onChange={setFrom} placeholder="now-1h" />
          <TraceTextField label="To" value={to} onChange={setTo} placeholder="now" />
          <TraceNumberField label="Limit" value={limit} onChange={setLimit} />

          {profile?.params.map((param) => (
            <TraceTextField
              key={param.name}
              label={`${param.name}${param.required ? " *" : ""}`}
              value={params[param.name] ?? ""}
              placeholder={param.description || param.operator}
              onChange={(value) => setParams((prev) => ({ ...prev, [param.name]: value }))}
            />
          ))}
        </FilterBar>
      </div>

      {error !== null && (
        <div className="px-3 pt-3">
          <InlineError title="OpenSearch query failed" error={error} />
        </div>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(280px,34vw)] max-[860px]:grid-cols-1">
        <main className="min-w-0 overflow-auto p-3">
          <div className="mb-2.5 flex items-center gap-2 text-slate-950">
            <Search size={16} />
            <strong>{result ? `${result.total} matches in ${result.index}` : preview ? "Query preview" : "Trace query"}</strong>
          </div>

          {preview ? (
            <div className="rounded-md border border-slate-200 bg-white p-2.5">
              <JsonView data={preview.request} name="request" defaultOpenDepth={3} />
            </div>
          ) : result ? (
            <DataTable<TraceRow>
              data={rows}
              columns={columns}
              emptyMessage="No traces matched the current query."
              autoFilter
              showGlobalFilter
              globalFilterPlaceholder="Search traces..."
              getRowId={(row) => row._id}
              onRowClick={setSelected}
              renderExpandedRow={(row) => (
                <div className="border-t border-slate-200 bg-white p-2.5">
                  <JsonView data={row._trace} name="trace" defaultOpenDepth={2} />
                </div>
              )}
              resizableColumns
              persistColumnWidths
              columnResizeStorageKey={`opensearch:${profileName}:widths`}
              hideableColumns
              persistColumnVisibility
              columnVisibilityStorageKey={`opensearch:${profileName}:columns`}
              defaultDensity="compact"
              persistDensity
              densityStorageKey="opensearch:trace-table:density"
              showDensityControl
            />
          ) : (
            <div className="grid justify-items-center gap-2 p-8 text-center text-slate-500">
              <Database size={28} />
              <div>No query results yet.</div>
            </div>
          )}
        </main>

        <aside className="min-w-0 overflow-auto border-l border-slate-200 bg-white p-3 max-[860px]:border-l-0 max-[860px]:border-t">
          <div className="mb-2 text-[13px] font-bold">Details</div>
          {selected ? (
            <JsonView data={selected._trace} name="trace" defaultOpenDepth={2} />
          ) : (
            <div className="text-slate-500">Select a row to inspect the trace document.</div>
          )}
        </aside>
      </div>
    </div>
  );
}

function TraceTextField({
  label,
  value,
  placeholder,
  onChange,
}: {
  label: string;
  value: string;
  placeholder?: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className={FIELD_CLASS}>
      <label className={LABEL_CLASS}>{label}</label>
      <input className={INPUT_CLASS} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

function TraceNumberField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
}) {
  return (
    <div className="flex min-w-24 flex-col gap-1">
      <label className={LABEL_CLASS}>{label}</label>
      <input
        className={INPUT_CLASS}
        type="number"
        min={1}
        max={10000}
        value={value}
        onChange={(e) => onChange(Number(e.target.value) || 100)}
      />
    </div>
  );
}

function buildColumns(columns: TracingColumn[]): Array<DataTableColumn<TraceRow>> {
  return columns
    .filter((col) => !col.detail)
    .map((col) => {
      const column: DataTableColumn<TraceRow> = {
        key: col.name,
        label: col.name,
        sortable: true,
        filterable: true,
        grow: /operation|message|name/i.test(col.name),
        shrink: /status|duration/i.test(col.name),
        align: /duration|count|size|ms$/i.test(col.name) ? "right" : "left",
        render: (value, row) => renderCell(value, col, row),
        filterValue: (value) => (value == null ? "" : String(value)),
        sortValue: sortValue,
        cellClassName: cellClass(col),
      };

      if (/time|timestamp/i.test(col.name) || /time|timestamp/i.test(col.field)) {
        column.kind = "timestamp";
        column.timestamp = { mode: "auto" };
      } else if (/status/i.test(col.name)) {
        column.kind = "status";
        column.status = { showLabel: true };
      }

      return column;
    });
}

function defaultParams(profile: ProfileSummary): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(profile.defaults ?? {})) {
    if (value != null) out[key] = String(value);
  }
  return out;
}

function requestBody(profile: string, params: Record<string, string>, from: string, to: string, limit: number) {
  const cleanParams: Record<string, string> = {};
  for (const [key, value] of Object.entries(params)) {
    if (value.trim()) cleanParams[key] = value.trim();
  }
  return { profile, params: cleanParams, from: from.trim(), to: to.trim(), limit };
}

function flattenTraces(traces: Trace[]): TraceRow[] {
  const out: TraceRow[] = [];
  const walk = (trace: Trace, depth: number) => {
    const row: TraceRow = {
      ...(trace.cells ?? {}),
      _id: `${trace.trace_id || "trace"}:${trace.span_id || out.length}`,
      _depth: depth,
      _trace: trace,
    };
    out.push(row);
    for (const child of trace.children ?? []) walk(child, depth + 1);
  };
  for (const trace of traces) walk(trace, 0);
  return out;
}

function renderCell(value: unknown, col: TracingColumn, row: TraceRow): React.ReactNode {
  if (value == null) return "";
  const text = formatValue(value);
  const content = /operation|name/i.test(col.name) && row._depth > 0 ? `${"  ".repeat(row._depth)}${text}` : text;
  return (
    <span className={cellClass(col)} title={text}>
      {content}
    </span>
  );
}

function formatValue(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value % 1 === 0 ? String(value) : value.toFixed(2);
  }
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function sortValue(value: unknown): unknown {
  if (typeof value === "number") return value;
  if (typeof value === "string") {
    const timestamp = Date.parse(value);
    if (Number.isFinite(timestamp) && /[-:T]/.test(value)) return timestamp;
    const numeric = Number(value);
    if (Number.isFinite(numeric) && value.trim() !== "") return numeric;
  }
  return value;
}

function cellClass(col: TracingColumn): string {
  if (/id$/i.test(col.name) || col.name.includes("ID")) {
    return "inline-block max-w-[360px] truncate align-bottom font-mono";
  }
  if (/time|duration/i.test(col.name)) return "font-mono";
  return "inline-block max-w-[360px] truncate align-bottom";
}

createRoot(document.getElementById("root")!).render(<App />);
