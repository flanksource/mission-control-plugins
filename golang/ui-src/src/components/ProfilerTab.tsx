import { useEffect, useState } from "preact/hooks";
import type { ComponentChildren } from "preact";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, ExternalLink, FileText, Flame, Play, Square, TerminalSquare } from "lucide-react";
import { Button, SplitPane } from "@flanksource/clicky-ui";
import { callOp, pluginURL, type GolangSession, type ProfileKind, type ProfileRun, type ProfileSource } from "../api";
import { Empty, ErrorText, Field, GopsRequiredOverlay, InfoCard, KV, RunBadge } from "./ui";
import { errorMessage, fmtBytes, fmtDuration } from "./utils";

const PROFILE_KINDS: ProfileKind[] = ["cpu", "trace", "heap"];
const PROFILE_SOURCES: ProfileSource[] = ["auto", "pprof", "gops"];

export function ProfilerTab({ session }: { session: GolangSession }) {
  const [kind, setKind] = useState<ProfileKind>("cpu");
  const [source, setSource] = useState<ProfileSource>("auto");
  const [seconds, setSeconds] = useState(30);
  const [selectedRunID, setSelectedRunID] = useState<string | null>(null);
  const qc = useQueryClient();

  const runsQ = useQuery({
    queryKey: ["golang", session.id, "profile-runs"],
    queryFn: () => callOp<ProfileRun[]>("profile-runs-list", { sessionId: session.id }),
    refetchInterval: 2_000,
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

  const preview = profilePreview(kind, source, seconds, session);
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
            {PROFILE_SOURCES.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
        </Field>
        <Field label="Duration seconds">
          <input className="h-8 rounded-md border bg-background px-2 text-xs" type="number" min={1} max={300} value={seconds} onInput={(event) => setSeconds(Number((event.target as HTMLInputElement).value))} />
        </Field>
      </div>
      <div className="rounded-md border bg-muted/30 p-2">
        <div className="mb-1 text-xs text-muted-foreground">Request preview</div>
        <pre className="overflow-auto font-mono text-xs">{preview}</pre>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Button size="sm" loading={start.isPending} onClick={() => start.mutate({ sessionId: session.id, kind, source, seconds })}>
          <Play className="h-4 w-4" />
          Start
        </Button>
        <Button size="sm" variant="secondary" loading={start.isPending} onClick={() => start.mutate({ sessionId: session.id, kind, source, seconds })}>
          <Play className="h-4 w-4" />
          Timed sample
        </Button>
        <Button size="sm" variant="outline" onClick={() => runsQ.refetch()}>
          Status
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
  const available = session.gopsAvailable || session.pprofAvailable;

  return (
    <div className="relative h-full min-h-0">
      {!available && <GopsRequiredOverlay>gops or pprof is required to capture profiles.</GopsRequiredOverlay>}
      <div className={`h-full min-h-0 ${!available ? "pointer-events-none blur-sm" : ""}`}>
        <SplitPane left={controls} right={output} defaultSplit={38} minLeft={28} minRight={36} />
      </div>
    </div>
  );
}

type ProfilerView = "flamegraph" | "top" | "raw";

function ProfilerOutputView({ session, run }: { session: GolangSession; run: ProfileRun | null }) {
  const renderable = !!run && run.state === "completed" && run.kind !== "trace";
  const [view, setView] = useState<ProfilerView>(renderable ? "flamegraph" : "raw");

  useEffect(() => {
    if (!renderable && view !== "raw") setView("raw");
  }, [renderable, view]);

  if (!run) {
    return (
      <section className="flex h-full min-h-0 flex-col gap-3 p-3">
        <Empty>No profile run selected.</Empty>
      </section>
    );
  }

  const downloadName = profileDownloadName(session, run);
  const renderURL = (path: string) => pluginURL(`profiles/${session.id}/${run.id}/${path}`);
  const downloadURL = pluginURL(`profiles/${session.id}/${run.id}`);

  return (
    <section className="flex h-full min-h-0 flex-col gap-2 p-3">
      <div className="flex items-center justify-between gap-2">
        <ProfilerOutputSwitch value={view} onChange={setView} disabled={!renderable} kind={run.kind} />
        <div className="flex items-center gap-1">
          {renderable && (
            <a
              className="inline-flex h-7 items-center gap-1 rounded-md border px-2 text-xs hover:bg-accent"
              href={renderURL("flamegraph")}
              target="_blank"
              rel="noreferrer"
              title="Open pprof viewer in a new tab"
            >
              <ExternalLink className="h-3 w-3" />
              Open viewer
            </a>
          )}
          {run.state === "completed" && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => downloadBlob(downloadURL, downloadName).catch((err) => alert(errorMessage(err)))}
            >
              <Download className="h-4 w-4" />
              Download
            </Button>
          )}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden rounded-md border bg-background">
        {view === "flamegraph" && renderable ? (
          <iframe
            key={`flame-${run.id}`}
            title="Profile flamegraph"
            src={renderURL("flamegraph")}
            className="h-full w-full border-0"
          />
        ) : view === "top" && renderable ? (
          <iframe
            key={`top-${run.id}`}
            title="Profile top functions"
            src={renderURL("top")}
            className="h-full w-full border-0"
          />
        ) : (
          <ProfilerRawView run={run} />
        )}
      </div>
    </section>
  );
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
          <KV k="Duration" v={run.seconds ? `${run.seconds}s` : "default"} />
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

async function downloadBlob(url: string, fallbackName: string): Promise<void> {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) throw new Error((await res.text()) || res.statusText);
  const filename = parseContentDispositionFilename(res.headers.get("Content-Disposition")) ?? fallbackName;
  const blob = await res.blob();
  const objectURL = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectURL;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
}

function parseContentDispositionFilename(header: string | null): string | undefined {
  if (!header) return undefined;
  const utf8 = header.match(/filename\*\s*=\s*UTF-8''([^;]+)/i);
  if (utf8?.[1]) {
    try { return decodeURIComponent(utf8[1].trim()); } catch { /* fallthrough */ }
  }
  const quoted = header.match(/filename\s*=\s*"([^"]+)"/i);
  if (quoted?.[1]) return quoted[1];
  const bare = header.match(/filename\s*=\s*([^;]+)/i);
  return bare?.[1]?.trim();
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
  if (run.state === "completed") return "Profile is complete. Use Download to inspect it with go tool pprof or go tool trace.";
  return "Profile run did not produce output.";
}

