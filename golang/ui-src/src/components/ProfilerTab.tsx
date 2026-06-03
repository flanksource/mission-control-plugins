import { useEffect, useMemo, useState } from "preact/hooks";
import type { ComponentChildren } from "preact";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, FileText, Flame, Play, Square, TerminalSquare } from "lucide-react";
import { Button, SplitPane } from "@flanksource/clicky-ui";
import {
  callOp,
  fetchProfileBlob,
  fetchProfileFlamegraph,
  fetchProfileTop,
  type FlamegraphNode,
  type GolangSession,
  type ProfileFlamegraph,
  type ProfileKind,
  type ProfileRun,
  type ProfileSource,
} from "../api";
import { Empty, ErrorText, Field, GopsRequiredOverlay, InfoCard, KV, LoadingOverlay, RefetchIndicator, RunBadge, useDelayedTruthy } from "./ui";
import { fmtBytes, fmtDuration } from "./utils";

const PROFILE_KINDS: ProfileKind[] = ["cpu", "trace", "heap"];
const PROFILE_SOURCES: ProfileSource[] = ["auto", "pprof", "gops"];

export function ProfilerTab({ session }: { session: GolangSession }) {
  const [kind, setKind] = useState<ProfileKind>("cpu");
  const [source, setSource] = useState<ProfileSource>("auto");
  const [seconds, setSeconds] = useState(30);
  const [selectedRunID, setSelectedRunID] = useState<string | null>(null);
  const qc = useQueryClient();

  const available = session.pprofAvailable || session.gopsAvailable;
  const runsQ = useQuery({
    queryKey: ["golang", session.id, "profile-runs"],
    queryFn: () => callOp<ProfileRun[]>("profile-runs-list", { sessionId: session.id }),
    enabled: available,
    refetchInterval: available ? 2_000 : false,
  });
  const runs = runsQ.data ?? [];
  const selected = runs.find((run) => run.id === selectedRunID) ?? runs[0] ?? null;

  const start = useMutation({
    mutationFn: (body: Record<string, unknown>) => callOp<ProfileRun>("profile-start", body),
    onSuccess: (run) => {
      setSelectedRunID(run.id);
      qc.invalidateQueries({ queryKey: ["golang", session.id, "profile-runs"] });
    },
  });

  const stop = useMutation({
    mutationFn: (runID: string) => callOp<ProfileRun>("profile-stop", { sessionId: session.id, runId: runID }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["golang", session.id, "profile-runs"] }),
  });

  const durationApplies = kind !== "heap";
  const preview = profilePreview(kind, source, seconds, session);
  const profileParams = { sessionId: session.id, kind, source, ...(durationApplies ? { seconds } : {}) };
  const controls = (
    <section className="flex min-h-0 flex-col gap-3 p-3">
      <div>
        <h3 className="text-sm font-semibold">Profiler</h3>
        <p className="text-xs text-muted-foreground">Capture Go pprof and gops profiles from the selected process.</p>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Field label="Event">
          <select className="h-8 rounded-md border bg-background px-2 text-xs" value={kind} onChange={(event) => setKind((event.target as HTMLSelectElement).value as ProfileKind)}>
            {PROFILE_KINDS.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
        </Field>
        <Field label="Source">
          <select className="h-8 rounded-md border bg-background px-2 text-xs" value={source} onChange={(event) => setSource((event.target as HTMLSelectElement).value as ProfileSource)}>
            {PROFILE_SOURCES.map((item) => (
              <option key={item} value={item} disabled={(item === "pprof" && !session.pprofAvailable) || (item === "gops" && !session.gopsAvailable)}>
                {item}
              </option>
            ))}
          </select>
        </Field>
        <Field label={durationApplies ? "Duration seconds" : "Duration"}>
          <input
            className="h-8 rounded-md border bg-background px-2 text-xs disabled:cursor-not-allowed disabled:opacity-50"
            type="number"
            min={1}
            max={300}
            value={seconds}
            disabled={!durationApplies}
            title={durationApplies ? undefined : "Heap profiles are snapshots and do not use duration"}
            onInput={(event) => setSeconds(Number((event.target as HTMLInputElement).value))}
          />
        </Field>
      </div>
      <div className="rounded-md border bg-muted/30 p-2">
        <div className="mb-1 text-xs text-muted-foreground">Request preview</div>
        <pre className="overflow-auto font-mono text-xs">{preview}</pre>
        {!durationApplies && <div className="mt-1 text-xs text-muted-foreground">Heap profiles are point-in-time snapshots; duration is ignored.</div>}
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Button size="sm" loading={start.isPending} onClick={() => start.mutate(profileParams)}>
          <Play className="h-4 w-4" />
          {durationApplies ? "Timed sample" : "Snapshot"}
        </Button>
        <Button
          size="sm"
          variant="destructive"
          loading={stop.isPending}
          disabled={!selected || selected.state !== "running"}
          onClick={() => selected && stop.mutate(selected.id)}
        >
          <Square className="h-4 w-4" />
          Stop
        </Button>
      </div>
      {(start.error || stop.error || runsQ.error) && <ErrorText error={start.error ?? stop.error ?? runsQ.error} />}
      <div className="min-h-0 flex-1 overflow-auto rounded-md border">
        {runs.length === 0 ? (
          <Empty>Run a profile to collect output.</Empty>
        ) : (
          runs.map((run) => (
            <button
              key={run.id}
              className={`flex w-full items-center justify-between gap-2 border-b px-3 py-2 text-left text-xs last:border-b-0 hover:bg-accent ${
                selected?.id === run.id ? "bg-primary/10" : ""
              }`}
              onClick={() => setSelectedRunID(run.id)}
            >
              <span className="min-w-0">
                <span className="block truncate font-mono font-semibold">{run.kind}</span>
                <span className="block truncate text-muted-foreground">
                  {run.source || run.preference || "auto"} · {fmtDuration(run.elapsedMs)} · {fmtBytes(run.bytes)}
                </span>
              </span>
              <RunBadge run={run} />
            </button>
          ))
        )}
      </div>
    </section>
  );

  const output = <ProfilerOutputView session={session} run={selected} />;
  const loading = available && runsQ.isFetching && !runsQ.data;
  const refetching = available && runsQ.isFetching && !!runsQ.data;
  const showRefetching = useDelayedTruthy(refetching);
  const blocked = !available || loading;

  return (
    <div className="relative h-full min-h-0">
      {!available && <GopsRequiredOverlay>gops or pprof is required to capture profiles.</GopsRequiredOverlay>}
      {loading && <LoadingOverlay>Loading profile runs…</LoadingOverlay>}
      {showRefetching && <RefetchIndicator>Refreshing profile runs…</RefetchIndicator>}
      <div className={`h-full min-h-0 ${blocked ? "pointer-events-none blur-sm" : ""}`}>
        <SplitPane className="h-full" left={controls} right={output} defaultSplit={38} minLeft={28} minRight={36} />
      </div>
    </div>
  );
}

type ProfilerView = "flamegraph" | "top" | "raw";

function ProfilerOutputView({ session, run }: { session: GolangSession; run: ProfileRun | null }) {
  const renderable = !!run && run.state === "completed" && run.kind !== "trace";
  const [view, setView] = useState<ProfilerView>(renderable ? "flamegraph" : "raw");
  const [sampleIndex, setSampleIndex] = useState("");
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<Error | null>(null);

  useEffect(() => {
    if (!renderable && view !== "raw") setView("raw");
  }, [renderable, view]);

  useEffect(() => {
    setSampleIndex("");
  }, [run?.id]);

  if (!run) {
    return (
      <section className="flex h-full min-h-0 flex-col gap-3 p-3">
        <Empty>No profile run selected.</Empty>
      </section>
    );
  }

  const downloadName = profileDownloadName(session, run);
  const download = async () => {
    setDownloadError(null);
    setDownloading(true);
    try {
      const blob = await fetchProfileBlob(session.id, run.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = downloadName;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      setDownloadError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setDownloading(false);
    }
  };

  return (
    <section className="flex h-full min-h-0 flex-col gap-2 p-3">
      <div className="flex items-center justify-between gap-2">
        <ProfilerOutputSwitch value={view} onChange={setView} disabled={!renderable} kind={run.kind} />
        {run.state === "completed" && (
          <Button size="sm" variant="outline" loading={downloading} onClick={download}>
            <Download className="h-4 w-4" />
            Download
          </Button>
        )}
      </div>
      {downloadError && <ErrorText error={downloadError} />}

      <div className="min-h-0 flex-1 overflow-hidden rounded-md border bg-background">
        {view === "flamegraph" && renderable ? (
          <ProfilerFlamegraphView sessionID={session.id} runID={run.id} sampleIndex={sampleIndex} onSampleIndexChange={setSampleIndex} />
        ) : view === "top" && renderable ? (
          <ProfilerTopView sessionID={session.id} runID={run.id} sampleIndex={sampleIndex} />
        ) : (
          <ProfilerRawView run={run} />
        )}
      </div>
    </section>
  );
}

function ProfilerFlamegraphView({
  sessionID,
  runID,
  sampleIndex,
  onSampleIndexChange,
}: {
  sessionID: string;
  runID: string;
  sampleIndex: string;
  onSampleIndexChange: (value: string) => void;
}) {
  const q = useQuery({
    queryKey: ["golang", sessionID, runID, "flamegraph", sampleIndex || "default"],
    queryFn: () => fetchProfileFlamegraph(sessionID, runID, sampleIndex || undefined),
  });

  if (q.isLoading) return <Empty>Loading flamegraph…</Empty>;
  if (q.error) return <div className="p-3"><ErrorText error={q.error} /></div>;
  if (!q.data || q.data.total <= 0) return <Empty>Profile has no samples for this sample type.</Empty>;

  return (
    <div className="flex h-full min-h-0 flex-col gap-2 p-3">
      <div className="flex items-center justify-between gap-2 text-xs">
        <div className="text-muted-foreground">
          {q.data.sampleType} · {formatFlamegraphValue(q.data.total, q.data.unit)} total
        </div>
        {q.data.sampleTypes.length > 1 && (
          <select
            className="h-7 rounded-md border bg-background px-2 text-xs"
            value={sampleIndex || q.data.sampleType}
            onChange={(event) => onSampleIndexChange((event.target as HTMLSelectElement).value)}
          >
            {q.data.sampleTypes.map((sampleType) => <option key={sampleType} value={sampleType}>{sampleType}</option>)}
          </select>
        )}
      </div>
      <Flamegraph data={q.data} />
    </div>
  );
}

function ProfilerTopView({ sessionID, runID, sampleIndex }: { sessionID: string; runID: string; sampleIndex: string }) {
  const q = useQuery({
    queryKey: ["golang", sessionID, runID, "top", sampleIndex || "default"],
    queryFn: () => fetchProfileTop(sessionID, runID, sampleIndex || undefined),
  });
  if (q.isLoading) return <Empty>Loading top functions…</Empty>;
  if (q.error) return <div className="p-3"><ErrorText error={q.error} /></div>;
  return <pre className="h-full overflow-auto p-3 font-mono text-xs">{q.data}</pre>;
}

function Flamegraph({ data }: { data: ProfileFlamegraph }) {
  const [focused, setFocused] = useState<FlamegraphNode | null>(null);
  const [hovered, setHovered] = useState<FlameFrame | null>(null);
  const [search, setSearch] = useState("");

  useEffect(() => {
    setFocused(null);
    setHovered(null);
    setSearch("");
  }, [data]);

  const root = focused ?? data.root;
  const frames = useMemo(() => buildFlameFrames(root), [root]);
  const maxDepth = frames.reduce((max, frame) => Math.max(max, frame.depth), 0);
  const rowHeight = 22;
  const width = 1000;
  const height = Math.max(1, maxDepth + 1) * rowHeight;
  const needle = search.trim().toLowerCase();

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex items-center gap-2 text-xs">
        <input
          className="h-7 min-w-0 flex-1 rounded-md border bg-background px-2"
          placeholder="Search frames"
          value={search}
          onInput={(event) => setSearch((event.target as HTMLInputElement).value)}
        />
        {focused && <Button size="sm" variant="outline" onClick={() => setFocused(null)}>Reset zoom</Button>}
      </div>
      <div className="rounded-md border bg-muted/20 px-2 py-1 text-xs text-muted-foreground">
        {hovered ? (
          <span>
            <span className="font-mono text-foreground">{hovered.node.name}</span>{" "}
            {formatFlamegraphValue(hovered.node.value, data.unit)} ({((hovered.node.value / root.value) * 100).toFixed(2)}%)
            {hovered.node.self ? ` · self ${formatFlamegraphValue(hovered.node.self, data.unit)}` : ""}
          </span>
        ) : (
          <span>Click a frame to zoom. Wheel/trackpad scroll is handled by the container.</span>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto rounded-md border bg-background">
        <svg viewBox={`0 0 ${width} ${height}`} width="100%" height={height} className="min-w-[900px] select-none text-[10px]">
          {frames.map((frame) => {
            if (frame.w <= 0.05) return null;
            const y = (maxDepth - frame.depth) * rowHeight;
            const match = !needle || frame.node.name.toLowerCase().includes(needle);
            const text = frame.w > 22 ? truncateFrameName(frame.node.name, frame.w) : "";
            return (
              <g key={frame.path} onClick={() => setFocused(frame.node)} onMouseEnter={() => setHovered(frame)} onMouseLeave={() => setHovered(null)} className="cursor-pointer">
                <rect
                  x={frame.x}
                  y={y + 1}
                  width={Math.max(0, frame.w - 0.5)}
                  height={rowHeight - 2}
                  rx={2}
                  fill={colorForFrame(frame.node.name)}
                  opacity={match ? 0.95 : 0.28}
                  stroke={needle && match ? "#facc15" : "rgba(0,0,0,0.2)"}
                  strokeWidth={needle && match ? 1.5 : 0.4}
                />
                {text && (
                  <text x={frame.x + 4} y={y + 15} fill="rgba(255,255,255,0.95)" pointerEvents="none">
                    {text}
                  </text>
                )}
                <title>{`${frame.node.name}\n${formatFlamegraphValue(frame.node.value, data.unit)}`}</title>
              </g>
            );
          })}
        </svg>
      </div>
    </div>
  );
}

type FlameFrame = {
  node: FlamegraphNode;
  x: number;
  w: number;
  depth: number;
  path: string;
};

function buildFlameFrames(root: FlamegraphNode): FlameFrame[] {
  const frames: FlameFrame[] = [];
  const walk = (node: FlamegraphNode, x: number, w: number, depth: number, path: string) => {
    frames.push({ node, x, w, depth, path });
    let childX = x;
    for (let i = 0; i < (node.children?.length ?? 0); i++) {
      const child = node.children![i];
      const childW = node.value > 0 ? w * (child.value / node.value) : 0;
      walk(child, childX, childW, depth + 1, `${path}.${i}`);
      childX += childW;
    }
  };
  walk(root, 0, 1000, 0, "0");
  return frames;
}

function colorForFrame(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = ((hash << 5) - hash + name.charCodeAt(i)) | 0;
  const hue = Math.abs(hash) % 360;
  return `hsl(${hue} 70% 45%)`;
}

function truncateFrameName(name: string, width: number): string {
  const max = Math.max(3, Math.floor(width / 5.8));
  return name.length > max ? `${name.slice(0, max - 1)}…` : name;
}

function formatFlamegraphValue(value: number, unit: string): string {
  if (unit === "nanoseconds" || unit === "ns") {
    if (value >= 1e9) return `${(value / 1e9).toFixed(2)}s`;
    if (value >= 1e6) return `${(value / 1e6).toFixed(2)}ms`;
    if (value >= 1e3) return `${(value / 1e3).toFixed(2)}µs`;
    return `${value}ns`;
  }
  if (unit === "bytes") {
    let v = value;
    for (const suffix of ["B", "KiB", "MiB", "GiB", "TiB"]) {
      if (v < 1024 || suffix === "TiB") return `${v.toFixed(2)}${suffix}`;
      v /= 1024;
    }
  }
  return unit ? `${value}${unit}` : String(value);
}

function ProfilerRawView({ run }: { run: ProfileRun }) {
  return (
    <div className="flex h-full min-h-0 flex-col gap-3 overflow-auto p-3">
      <div className="grid grid-cols-1 gap-2 lg:grid-cols-2">
        <InfoCard title="Run">
          <KV k="Kind" v={run.kind} />
          <KV k="State" v={run.state} />
          <KV k="Source" v={run.source || run.preference || "auto"} />
          <KV k="Bytes" v={fmtBytes(run.bytes)} />
          <KV k="Elapsed" v={fmtDuration(run.elapsedMs)} />
        </InfoCard>
        <InfoCard title="Timing">
          <KV k="Started" v={new Date(run.startedAt).toLocaleString()} />
          <KV k="Completed" v={run.completedAt ? new Date(run.completedAt).toLocaleString() : "running"} />
          <KV k="Duration" v={run.kind === "heap" ? "snapshot" : run.seconds ? `${run.seconds}s` : "default"} />
        </InfoCard>
      </div>
      <pre className="min-h-40 flex-1 overflow-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">
        {run.error || profileHelp(run)}
      </pre>
    </div>
  );
}

function ProfilerOutputSwitch({
  value,
  onChange,
  disabled,
  kind,
}: {
  value: ProfilerView;
  onChange: (value: ProfilerView) => void;
  disabled: boolean;
  kind: string;
}) {
  return (
    <div className="inline-flex items-center gap-1 rounded-md bg-muted p-1" role="tablist" aria-label="Profiler output">
      <ProfilerOutputOption value="flamegraph" current={value} onChange={onChange} disabled={disabled} icon={<Flame className="h-3 w-3" />} label="Flamegraph" />
      <ProfilerOutputOption value="top" current={value} onChange={onChange} disabled={disabled} icon={<FileText className="h-3 w-3" />} label="Top" />
      <ProfilerOutputOption value="raw" current={value} onChange={onChange} disabled={false} icon={<TerminalSquare className="h-3 w-3" />} label={kind === "trace" ? "Trace" : "Raw"} />
    </div>
  );
}

function ProfilerOutputOption({
  value,
  current,
  onChange,
  disabled,
  icon,
  label,
}: {
  value: ProfilerView;
  current: ProfilerView;
  onChange: (value: ProfilerView) => void;
  disabled: boolean;
  icon: ComponentChildren;
  label: string;
}) {
  const checked = value === current;
  const className = `inline-flex h-6 items-center gap-1 rounded px-2 text-xs font-medium ${
    checked ? "bg-background text-foreground shadow" : "text-muted-foreground hover:text-foreground"
  } ${disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`;
  return (
    <button
      type="button"
      role="tab"
      aria-selected={checked}
      disabled={disabled}
      onClick={() => onChange(value)}
      className={className}
    >
      {icon}
      {label}
    </button>
  );
}

function profileDownloadName(session: GolangSession, run: ProfileRun): string {
  const ext = run.kind === "trace" ? "trace" : "pprof";
  return `golang-${session.id}-${run.id}.${ext}`;
}

function profilePreview(kind: ProfileKind, source: ProfileSource, seconds: number, session: GolangSession): string {
  const effective = source === "auto" ? (session.pprofAvailable ? "pprof" : "gops") : source;
  if (effective === "pprof") {
    if (kind === "cpu") return `GET ${session.pprofBasePath ?? "/debug/pprof"}/profile?seconds=${seconds}`;
    if (kind === "trace") return `GET ${session.pprofBasePath ?? "/debug/pprof"}/trace?seconds=${seconds}`;
    return `GET ${session.pprofBasePath ?? "/debug/pprof"}/heap`;
  }
  if (kind === "cpu") return "gops pprof-cpu";
  if (kind === "trace") return "gops trace";
  return "gops pprof-heap";
}

function profileHelp(run: ProfileRun): string {
  if (run.state === "running") return "Profile is still running. Status refreshes automatically.";
  if (run.state === "stopped") return "Profile run was stopped before producing a downloadable profile.";
  if (run.state === "completed") return "Profile is complete. Use Flamegraph or Top to inspect it, or Download for offline analysis.";
  return "Profile run did not produce output.";
}

