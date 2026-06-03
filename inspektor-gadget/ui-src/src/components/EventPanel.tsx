import React, { useMemo } from "react";
import { Button, DataTable, HoverCard, cn, type DataTableColumn } from "@flanksource/clicky-ui";
import { K8S, K8SCronjob, K8SDaemonset, K8SDeployment, K8SEndpoint, K8SIngress, K8SJob, K8SNamespace, K8SNode, K8SPod, K8SReplicaset, K8SService, K8SServiceaccount, K8SStatefulset } from "@flanksource/icons/mi";
import { Download, Network, Terminal } from "lucide-react";
import type { EventColumnSpec, EventTableRow, GadgetSpec, Session, TraceEvent } from "../types";
import { widgetLabel } from "../utils/gadgets";

type EventPanelProps = {
  activeSession: Session | null;
  activeGadgetSpec: GadgetSpec | null;
  events: TraceEvent[];
};

export function EventPanel({ activeSession, activeGadgetSpec, events }: EventPanelProps) {
  const eventRows = useMemo(
    () => events.map((event) => ({
      ...event,
      timeLabel: event.time ? new Date(event.time).toLocaleTimeString() : "",
      summary: event.error || summarize(event)
    })),
    [events]
  );
  const displayRows = useMemo(
    () => rowsForWidget(eventRows, activeGadgetSpec),
    [eventRows, activeGadgetSpec]
  );
  const eventColumns: DataTableColumn<EventTableRow>[] = useMemo(
    () => eventTableColumns(activeGadgetSpec, activeSession),
    [activeGadgetSpec, activeSession]
  );
  const eventDefaultSort = useMemo(
    () => eventTableDefaultSort(activeGadgetSpec),
    [activeGadgetSpec]
  );

  return (
    <section className="panel events">
      <div className="panel-heading">
        <div className="flex min-w-0 items-center gap-2">
          {activeGadgetSpec && <WidgetBadge gadget={activeGadgetSpec} rows={displayRows} rawCount={eventRows.length} />}
        </div>
        {activeSession && (
          <Button asChild variant="outline" size="sm">
            <a href={pluginUiPath(`/sessions/${activeSession.id}/export`)} download={`${activeSession.id}.ndjson`}>
              <Download size={14} /> NDJSON
            </a>
          </Button>
        )}
      </div>
      <DataTable
        key={`${activeSession?.id || "none"}-${activeGadgetSpec?.widget || "trace"}-${eventDefaultSort.key}`}
        className="events-table"
        data={displayRows}
        columns={eventColumns}
        autoFilter
        defaultSort={eventDefaultSort}
        getRowId={(row) => row.__rowKey || `${row.sessionId}-${row.sequence}`}
        columnResizeStorageKey={`inspektor-gadget-events-${activeGadgetSpec?.id || "generic"}`}
        emptyMessage={activeSession ? "Waiting for events" : "Select a session"}
        renderExpandedRow={(row) => <pre className="event-json">{JSON.stringify(originalEvent(row), null, 2)}</pre>}
      />
    </section>
  );
}

function pluginUiPath(path: string) {
  const base = window.location.pathname.replace(/\/$/, "");
  const query = window.location.search || "";
  return `${base}${path}${query}`;
}

function WidgetBadge({ gadget, rows, rawCount }: { gadget: GadgetSpec; rows: EventTableRow[]; rawCount: number }) {
  const label = widgetLabel(gadget.widget);
  const detail = gadget.widget === "top" && rawCount !== rows.length
    ? `${rows.length} latest / ${rawCount} samples`
    : `${rows.length} rows`;
  const cpu = gadget.id === "top_process" ? maxMetric(rows, "cpuUsage", parsePercentValue) : null;
  const rss = gadget.id === "top_process" ? maxMetric(rows, "memoryRSS", parseByteValue) : null;
  return (
    <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
      <span className="rounded-full border border-border bg-muted px-2 py-0.5 font-medium text-foreground">{label}</span>
      <span>{detail}</span>
      {cpu != null && <span className="metric-badge metric-badge-amber">max CPU {formatPercent(cpu)}</span>}
      {rss != null && <span className="metric-badge metric-badge-sky">max RSS {formatBytes(rss)}</span>}
    </div>
  );
}

function maxMetric(rows: EventTableRow[], path: string, parse: (value: unknown) => number | null) {
  let max: number | null = null;
  for (const row of rows) {
    const value = parse(valueAtPath(row.data || {}, path));
    if (value != null && (max == null || value > max)) max = value;
  }
  return max;
}

function summarize(event: TraceEvent) {
  const data = event.data || {};
  const proc = data.proc as Record<string, unknown> | undefined;
  const k8s = data.k8s as Record<string, unknown> | undefined;
  const parts = [
    k8s?.namespace,
    k8s?.podName,
    k8s?.containerName,
    proc?.comm || data.comm,
    data.type,
    data.dst || data.name || data.fname || data.args
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" / ") : event.raw || "";
}

function rowsForWidget(rows: EventTableRow[], gadget: GadgetSpec | null): EventTableRow[] {
  if (gadget?.widget !== "top") return rows;
  const byKey = new Map<string, EventTableRow>();
  for (const row of rows) {
    const key = topRowKey(row, gadget);
    const prev = byKey.get(key);
    byKey.set(key, {
      ...row,
      __rowKey: `${row.sessionId}-${gadget.id}-${key}`,
      __sampleCount: (prev?.__sampleCount || 0) + 1
    });
  }
  return Array.from(byKey.values());
}

function topRowKey(row: EventTableRow, gadget: GadgetSpec) {
  const data = row.data || {};
  const workload = formatK8s(data.k8s, null);
  switch (gadget.id) {
    case "top_process":
      return [row.node, workload, data.pid, data.comm].map(stringValue).join("|");
    case "top_tcp":
      return [row.node, workload, data.pid, data.comm, displayEventValue(data.src, "endpoint"), displayEventValue(data.dst, "endpoint")].map(stringValue).join("|");
    case "top_file":
      return [row.node, workload, processKey(data.proc), data.file, data.dev, data.inode].map(stringValue).join("|");
    case "top_blockio":
      return [row.node, workload, processKey(data.proc), data.rw, data.major, data.minor].map(stringValue).join("|");
    case "top_cpu_throttle":
      return [row.node, workload, data.cgroupPath].map(stringValue).join("|");
    case "bpfstats":
      return [row.node, data.gadgetID, data.progID, data.progName].map(stringValue).join("|");
    default:
      return [row.node, workload, row.summary].map(stringValue).join("|");
  }
}

function processKey(value: unknown) {
  if (!value || typeof value !== "object") return stringValue(value);
  const record = value as Record<string, unknown>;
  return [record.pid, record.comm || record.name].map(stringValue).join("/");
}

function eventTableDefaultSort(gadget: GadgetSpec | null): { key: string; dir?: "asc" | "desc" } {
  switch (gadget?.id) {
    case "top_process":
      return { key: "data.cpuUsage", dir: "desc" };
    case "top_cpu_throttle":
      return { key: "data.throttleRatio", dir: "desc" };
    case "top_tcp":
      return { key: "data.sent", dir: "desc" };
    case "top_file":
      return { key: "data.rbytes_raw", dir: "desc" };
    case "top_blockio":
      return { key: "data.bytes", dir: "desc" };
    case "bpfstats":
      return { key: "data.mapMemory", dir: "desc" };
    default:
      return { key: "sequence", dir: "asc" };
  }
}

function eventTableColumns(gadget: GadgetSpec | null, session: Session | null): DataTableColumn<EventTableRow>[] {
  const columns: DataTableColumn<EventTableRow>[] = [
    {
      key: "sequence",
      label: "#",
      align: "right",
      shrink: true,
      minWidth: 56,
      sortable: true
    },
    {
      key: "timeLabel",
      label: "Time",
      shrink: true,
      minWidth: 96,
      sortable: true,
      sortValue: (_value, row) => Date.parse(row.time || "") || 0
    },
    {
      key: "node",
      label: "Node",
      shrink: true,
      minWidth: 130,
      filterable: true
    },
    {
      key: "data.k8s",
      label: "Workload",
      minWidth: 220,
      filterable: true,
      filterValue: (_value, row) => formatK8s(row.data?.k8s, session),
      render: (_value, row) => <K8sCell value={row.data?.k8s} session={session} />
    }
  ];

  for (const spec of gadget?.eventSchema?.columns || []) {
    if (spec.hidden) continue;
    columns.push(eventColumn(spec));
  }

  columns.push({
    key: "summary",
    label: "Summary",
    grow: true,
    minWidth: 280,
    filterable: true,
    cellClassName: "font-mono text-xs truncate max-w-0",
    render: (value) => <code title={String(value || "")}>{String(value || "")}</code>
  });
  return columns;
}

function eventColumn(spec: EventColumnSpec): DataTableColumn<EventTableRow> {
  const key = `data.${spec.path}`;
  const numeric = spec.kind === "number" || spec.kind === "bytes" || spec.kind === "percent";
  return {
    key,
    label: spec.label || spec.path,
    align: numeric ? "right" : "left",
    shrink: spec.kind !== "json" && spec.kind !== "text",
    minWidth: columnMinWidth(spec),
    filterable: true,
    sortValue: (_value, row) => sortValue(eventDataValue(row, spec.path), spec.kind),
    filterValue: (_value, row) => displayEventValue(eventDataValue(row, spec.path), spec.kind),
    cellClassName: spec.kind === "json" ? "font-mono text-xs truncate max-w-0" : undefined,
    render: (_value, row) => {
      const value = eventDataValue(row, spec.path);
      if (spec.kind === "process") return <ProcessCell value={value} />;
      if (spec.kind === "command") return <CommandCell row={row} />;
      if (spec.kind === "endpoint") return <EndpointCell value={value} row={row} path={spec.path} />;
      if (spec.kind === "percent") return <PercentCell value={value} />;
      if (spec.kind === "bytes") return <BytesCell value={value} />;
      const display = displayEventValue(value, spec.kind);
      return <code title={display}>{display}</code>;
    }
  };
}

function columnMinWidth(spec: EventColumnSpec) {
  if (spec.kind === "process") return 170;
  if (spec.kind === "command") return 180;
  if (spec.kind === "endpoint") return 190;
  if (spec.kind === "percent") return 116;
  if (spec.kind === "bytes" || spec.kind === "number") return 96;
  if (spec.kind === "json") return 180;
  if (/(path|file|args|address|destination|source|buffer|parameters)/i.test(spec.label)) return 220;
  return 130;
}

function eventDataValue(row: EventTableRow, path: string) {
  return valueAtPath(row.data || {}, path);
}

function valueAtPath(value: unknown, path: string): unknown {
  let current = value;
  for (const part of path.split(".")) {
    if (!part) continue;
    if (current == null || typeof current !== "object") return undefined;
    current = (current as Record<string, unknown>)[part];
  }
  return current;
}

function sortValue(value: unknown, kind?: string) {
  if (kind === "bytes") return parseByteValue(value) ?? 0;
  if (kind === "percent") return parsePercentValue(value) ?? 0;
  if (kind === "number") {
    const number = Number(value);
    return Number.isFinite(number) ? number : 0;
  }
  return displayEventValue(value, kind);
}

function displayEventValue(value: unknown, kind?: string): string {
  if (value == null || value === "") return "";
  if (kind === "process") return formatProcess(value);
  if (kind === "endpoint") return formatEndpoint(value);
  if (kind === "bytes") return formatBytes(value);
  if (kind === "percent") return formatPercent(value);
  if (kind === "boolean") return Boolean(value) ? "true" : "false";
  if (typeof value === "object") return compactJSON(value);
  return String(value);
}

function EndpointCell({ value, row, path }: { value: unknown; row: EventTableRow; path: string }) {
  const endpoint = endpointParts(value, row, path);
  if (!endpoint.addr && !endpoint.port && !endpoint.proto && !endpoint.k8s) return <span />;
  const title = endpointTitle(endpoint);
  const identity = endpointIdentity(endpoint);
  const primary = identity.name || endpoint.addr || "";
  const secondary = identity.name ? endpointAddressLabel(endpoint) : "";
  const trigger = (
    <span className="inline-flex max-w-full items-center gap-1.5 align-top" title={title}>
      <EndpointIcon kind={identity.kind} />
      <span className="min-w-0 truncate text-foreground">{primary}</span>
      {secondary && <span className="shrink-0 font-mono text-[11px] text-muted-foreground">{secondary}</span>}
      {!identity.name && endpoint.port && <span className="shrink-0 font-mono text-[11px] text-muted-foreground">:{endpoint.port}</span>}
      {identity.kind && <span className="shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground">{identity.kind}</span>}
      {endpoint.proto && <span className="shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground">{endpoint.proto}</span>}
    </span>
  );
  return (
    <HoverCard trigger={trigger} placement="bottom" delay={120} cardClassName="w-[min(28rem,calc(100vw-2rem))]">
      <DetailRows rows={endpointDetails(endpoint)} />
    </HoverCard>
  );
}

function CommandCell({ row }: { row: EventTableRow }) {
  const data = row.data || {};
  const comm = data.comm || data.name;
  const pid = stringValue(data.pid);
  if (!comm && !pid) {
    const display = displayEventValue(data.comm);
    return <code title={display}>{display}</code>;
  }
  const trigger = (
    <span className="inline-flex max-w-full items-center gap-1.5 align-top" title={[comm, pid && `pid ${pid}`].filter(Boolean).join(" / ")}>
      <Terminal size={14} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate text-foreground">{String(comm || "")}</span>
      {pid && <span className="shrink-0 font-mono text-[11px] text-muted-foreground">pid {pid}</span>}
    </span>
  );
  return (
    <HoverCard trigger={trigger} placement="bottom" delay={120} cardClassName="w-[min(24rem,calc(100vw-2rem))]">
      <DetailRows rows={[
        ["Command", stringValue(comm)],
        ["PID", pid],
        ["TID", stringValue(data.tid)],
        ["UID", stringValue(data.uid ?? data.uidRaw)],
        ["GID", stringValue(data.gid ?? data.gidRaw)],
        ["State", stringValue(data.state)],
        ["CPU", formatPercent(data.cpuUsage)],
        ["RSS", formatBytes(data.memoryRSS)],
        ["Virtual", formatBytes(data.memoryVirtual)],
        ["Samples", stringValue(row.__sampleCount)]
      ]} />
    </HoverCard>
  );
}

function PercentCell({ value }: { value: unknown }) {
  const parsed = parsePercentValue(value);
  const display = formatPercent(value);
  const width = parsed == null ? 0 : Math.max(0, Math.min(parsed, 100));
  return (
    <span className="inline-flex min-w-[5.5rem] items-center justify-end gap-2 tabular-nums" title={display}>
      <span className="relative h-1.5 w-10 overflow-hidden rounded-full bg-muted">
        <span className={cn("absolute inset-y-0 left-0 rounded-full", percentColor(parsed))} style={{ width: `${width}%` }} />
      </span>
      <code>{display}</code>
    </span>
  );
}

function BytesCell({ value }: { value: unknown }) {
  const display = formatBytes(value);
  return <code className="tabular-nums" title={display}>{display}</code>;
}

function percentColor(value: number | null) {
  if (value == null) return "bg-muted-foreground/25";
  if (value >= 80) return "bg-red-500";
  if (value >= 50) return "bg-amber-500";
  return "bg-emerald-500";
}

function K8sCell({ value, session }: { value: unknown; session: Session | null }) {
  const record = value && typeof value === "object" ? value as Record<string, unknown> : {};
  const kind = stringValue(record.kind || session?.target?.kind || "pod");
  const namespace = record.namespace || session?.target?.namespace;
  const name = record.podName || record.pod || record.name || session?.target?.pod || session?.target?.name;
  const container = record.containerName || record.container || session?.target?.container;
  if (!namespace && !name && !container) return <span />;
  const trigger = (
    <span className="inline-flex max-w-full items-center gap-1.5 align-top" title={formatK8s(value, session)}>
      <KubernetesIcon kind={kind} />
      <span className="min-w-0 truncate text-foreground">{String(name || "")}</span>
      {container && <span className="shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground">{String(container)}</span>}
    </span>
  );
  return (
    <HoverCard trigger={trigger} placement="bottom" delay={120} cardClassName="w-[min(28rem,calc(100vw-2rem))]">
      <DetailRows rows={[
        ["Kind", kind],
        ["Namespace", stringValue(namespace)],
        ["Name", stringValue(name)],
        ["Container", stringValue(container)],
        ["Labels", stringValue(record.labels)],
        ["Selector", stringValue(record.podSelector)]
      ]} />
    </HoverCard>
  );
}

function ProcessCell({ value }: { value: unknown }) {
  const record = value && typeof value === "object" ? value as Record<string, unknown> : {};
  const comm = record.comm || record.pcomm || record.name;
  const pid = record.pid;
  const pidLabel = stringValue(pid);
  if (!comm && !pidLabel) {
    const display = displayEventValue(value);
    return <code title={display}>{display}</code>;
  }
  const trigger = (
    <span className="inline-flex max-w-full items-center gap-1.5 align-top" title={formatProcess(value)}>
      <Terminal size={14} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate text-foreground">{String(comm || "")}</span>
      {pidLabel && <span className="shrink-0 font-mono text-[11px] text-muted-foreground">pid {pidLabel}</span>}
    </span>
  );
  return (
    <HoverCard trigger={trigger} placement="bottom" delay={120} cardClassName="w-[min(24rem,calc(100vw-2rem))]">
      <DetailRows rows={[
        ["Command", stringValue(comm)],
        ["PID", stringValue(pid)],
        ["TID", stringValue(record.tid)],
        ["PPID", stringValue(record.ppid)],
        ["UID", stringValue(record.uid ?? record.uidRaw)],
        ["GID", stringValue(record.gid ?? record.gidRaw)],
        ["Parent", stringValue(record.pcomm)]
      ]} />
    </HoverCard>
  );
}

function DetailRows({ rows }: { rows: Array<[string, string]> }) {
  const visible = rows.filter(([, value]) => value !== "");
  if (visible.length === 0) return null;
  return (
    <div className="flex flex-col gap-1 text-xs">
      {visible.map(([label, value]) => (
        <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-2" key={label}>
          <span className="font-semibold text-muted-foreground">{label}</span>
          <code className="min-w-0 break-words">{value}</code>
        </div>
      ))}
    </div>
  );
}

function endpointParts(value: unknown, row?: EventTableRow, path?: string) {
  const record = value && typeof value === "object" ? value as Record<string, unknown> : {};
  const rowData = row?.data || {};
  const addr = stringValue(record.addr || record.ip || record.address || (typeof value === "string" || typeof value === "number" ? value : ""));
  const port = stringValue(record.port || (path === "addr" ? rowData.port : undefined));
  const proto = stringValue(record.proto || (path === "addr" ? rowData.proto : undefined));
  const version = stringValue(record.version || (path === "addr" ? rowData.version : undefined));
  const k8s = record.k8s && typeof record.k8s === "object"
    ? record.k8s as Record<string, unknown>
    : path === "addr" && rowData.k8s && typeof rowData.k8s === "object"
      ? rowData.k8s as Record<string, unknown>
      : undefined;
  const labels = record.labels || (path === "addr" ? rowData.labels : undefined);
  return { addr, port, proto, version, k8s, labels, record, raw: value };
}

function endpointDetails(endpoint: ReturnType<typeof endpointParts>): Array<[string, string]> {
  const identity = endpointIdentity(endpoint);
  return [
    ["Endpoint", endpointDisplayLabel(endpoint)],
    ["Address", endpoint.addr],
    ["Port", endpoint.port],
    ["Protocol", endpoint.proto],
    ["IP Version", endpoint.version],
    ["K8s Kind", identity.kind],
    ["K8s Namespace", identity.namespace],
    ["K8s Name", identity.name],
    ["Pod", stringValue(endpoint.k8s?.podName || endpoint.k8s?.pod || endpoint.k8s?.podNameRaw)],
    ["Service", stringValue(endpoint.k8s?.serviceName || endpoint.k8s?.service || endpoint.k8s?.svc)],
    ["Workload", stringValue(endpoint.k8s?.workloadName || endpoint.k8s?.ownerName || endpoint.k8s?.deployment || endpoint.k8s?.replicaSet)],
    ["Endpoint Labels", stringValue(endpoint.labels)],
    ["K8s Labels", stringValue(endpoint.k8s?.labels)],
    ["Selector", stringValue(endpoint.k8s?.podSelector || endpoint.k8s?.selector)],
    ["Endpoint Metadata", stringValue(endpointMetadata(endpoint.record))]
  ];
}

function endpointTitle(endpoint: ReturnType<typeof endpointParts>) {
  return endpointDetails(endpoint).filter(([, value]) => value).map(([label, value]) => `${label}: ${value}`).join("\n");
}

function endpointIdentity(endpoint: ReturnType<typeof endpointParts>) {
  const k8s = endpoint.k8s || {};
  const record = endpoint.record || {};
  const service = stringValue(k8s.serviceName || k8s.service || k8s.svc || k8s.service_name || record.serviceName || record.service || record.svc);
  const pod = stringValue(k8s.podName || k8s.pod || k8s.pod_name || record.podName || record.pod);
  const workload = stringValue(k8s.workloadName || k8s.ownerName || k8s.deployment || k8s.replicaSet || k8s.statefulSet || k8s.daemonSet || record.workloadName || record.ownerName);
  const rawName = stringValue(k8s.name || record.name);
  const rawKind = stringValue(k8s.kind || record.kind).toLowerCase();
  const namespace = stringValue(k8s.namespace || k8s.ns || k8s.serviceNamespace || k8s.podNamespace || record.namespace || record.ns);
  if (service) return { kind: "service", namespace, name: service };
  if (pod) return { kind: "pod", namespace, name: pod };
  if (workload) return { kind: stringValue(k8s.ownerKind || k8s.workloadKind || "workload").toLowerCase(), namespace, name: workload };
  if (rawName && rawKind && rawKind !== "raw") return { kind: rawKind, namespace, name: rawName };
  if (rawName && !rawKind) return { kind: "", namespace, name: rawName };
  return { kind: rawKind === "raw" ? "" : rawKind, namespace, name: "" };
}

function endpointDisplayLabel(endpoint: ReturnType<typeof endpointParts>) {
  const identity = endpointIdentity(endpoint);
  if (!identity.name) return [endpoint.addr, endpoint.port ? `:${endpoint.port}` : "", endpoint.proto ? ` ${endpoint.proto}` : ""].join("");
  const ns = identity.namespace ? `${identity.namespace}/` : "";
  const address = endpointAddressLabel(endpoint);
  return `${ns}${identity.name}${address ? ` ${address}` : ""}${endpoint.proto ? ` ${endpoint.proto}` : ""}`;
}

function endpointAddressLabel(endpoint: ReturnType<typeof endpointParts>) {
  if (!endpoint.addr && !endpoint.port) return "";
  return `${endpoint.addr || ""}${endpoint.port ? `:${endpoint.port}` : ""}`;
}

function EndpointIcon({ kind }: { kind: string }) {
  if (!kind) return <Network size={14} className="shrink-0 text-muted-foreground" />;
  return <KubernetesIcon kind={kind} />;
}

function KubernetesIcon({ kind }: { kind: string }) {
  const Icon = kubernetesIconComponent(kind);
  return <Icon className="h-3.5 max-w-3.5 shrink-0" square />;
}

type FlanksourceIconComponent = React.ComponentType<React.SVGAttributes<SVGElement> & { size?: string | number; square?: boolean }>;

function kubernetesIconComponent(kind: string): FlanksourceIconComponent {
  const normalized = kind.trim().toLowerCase().replace(/^kubernetes::/i, "").replace(/[^a-z0-9]/g, "");
  const icons: Record<string, FlanksourceIconComponent> = {
    cronjob: K8SCronjob,
    daemonset: K8SDaemonset,
    deployment: K8SDeployment,
    deploy: K8SDeployment,
    ds: K8SDaemonset,
    endpoint: K8SEndpoint,
    endpoints: K8SEndpoint,
    ep: K8SEndpoint,
    ingress: K8SIngress,
    job: K8SJob,
    namespace: K8SNamespace,
    ns: K8SNamespace,
    node: K8SNode,
    pod: K8SPod,
    replicaset: K8SReplicaset,
    rs: K8SReplicaset,
    service: K8SService,
    svc: K8SService,
    serviceaccount: K8SServiceaccount,
    statefulset: K8SStatefulset,
    sts: K8SStatefulset,
    workload: K8SDeployment
  };
  return icons[normalized] || K8S;
}

function endpointMetadata(record: Record<string, unknown>) {
  const metadata: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(record)) {
    if (["addr", "ip", "address", "port", "proto", "version", "k8s", "labels", "name", "kind"].includes(key)) continue;
    if (value == null || value === "") continue;
    metadata[key] = value;
  }
  return Object.keys(metadata).length > 0 ? metadata : "";
}

function formatEndpoint(value: unknown) {
  const endpoint = endpointParts(value);
  return endpointDisplayLabel(endpoint);
}

function formatProcess(value: unknown) {
  if (!value || typeof value !== "object") return displayEventValue(value);
  const record = value as Record<string, unknown>;
  const comm = record.comm || record.pcomm || record.name;
  const pid = record.pid;
  const tid = record.tid;
  const uid = record.uid ?? record.uidRaw;
  const parts = [comm, pid ? `pid ${pid}` : "", tid && tid !== pid ? `tid ${tid}` : "", uid !== undefined ? `uid ${uid}` : ""].filter(Boolean);
  return parts.length ? parts.join(" / ") : compactJSON(record);
}

function formatK8s(value: unknown, session: Session | null) {
  const record = value && typeof value === "object" ? value as Record<string, unknown> : {};
  const namespace = record.namespace || session?.target?.namespace;
  const pod = record.podName || record.pod || record.name || session?.target?.pod || session?.target?.name;
  const container = record.containerName || record.container || session?.target?.container;
  return [namespace, pod, container].filter(Boolean).join(" / ");
}

function stringValue(value: unknown) {
  if (value == null) return "";
  if (typeof value === "object") return compactJSON(value);
  return String(value);
}

function formatPercent(value: unknown) {
  const number = parsePercentValue(value);
  if (number == null) return String(value ?? "");
  const digits = Math.abs(number) >= 10 ? 1 : 2;
  return `${number.toFixed(digits)}%`;
}

function parsePercentValue(value: unknown) {
  if (value == null || value === "") return null;
  if (typeof value === "number") return Number.isFinite(value) ? value : null;
  const parsed = Number(String(value).trim().replace(/%$/, ""));
  return Number.isFinite(parsed) ? parsed : null;
}

function formatBytes(value: unknown) {
  const number = parseByteValue(value);
  if (number == null) return String(value ?? "");
  if (Math.abs(number) < 1024) return `${number} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let scaled = number / 1024;
  let unit = units[0];
  for (let i = 1; i < units.length && Math.abs(scaled) >= 1024; i++) {
    scaled /= 1024;
    unit = units[i];
  }
  return `${scaled.toFixed(scaled >= 10 ? 1 : 2)} ${unit}`;
}

function parseByteValue(value: unknown) {
  if (value == null || value === "") return null;
  if (typeof value === "number") return Number.isFinite(value) ? value : null;
  const text = String(value).trim();
  const match = text.match(/^(-?\d+(?:\.\d+)?)\s*([kmgtp]?i?b?|bytes?)?$/i);
  if (!match) return null;
  const number = Number(match[1]);
  if (!Number.isFinite(number)) return null;
  const unit = (match[2] || "b").toLowerCase();
  const multipliers: Record<string, number> = {
    "": 1,
    b: 1,
    byte: 1,
    bytes: 1,
    k: 1024,
    kb: 1024,
    kib: 1024,
    m: 1024 ** 2,
    mb: 1024 ** 2,
    mib: 1024 ** 2,
    g: 1024 ** 3,
    gb: 1024 ** 3,
    gib: 1024 ** 3,
    t: 1024 ** 4,
    tb: 1024 ** 4,
    tib: 1024 ** 4,
    p: 1024 ** 5,
    pb: 1024 ** 5,
    pib: 1024 ** 5
  };
  return number * (multipliers[unit] ?? 1);
}

function compactJSON(value: unknown) {
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function originalEvent(row: EventTableRow): TraceEvent {
  const { timeLabel: _timeLabel, summary: _summary, __rowKey: _rowKey, __sampleCount: _sampleCount, ...event } = row;
  return event;
}

