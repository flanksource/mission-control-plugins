import { useMemo } from "preact/hooks";
import { useQuery } from "@tanstack/react-query";
import { ProgressBar } from "@flanksource/clicky-ui";
import type { ComponentChildren } from "preact";
import { callOp, type GolangSession, type RuntimeSnapshot } from "../api";
import { ErrorText, GopsRequiredOverlay, InfoCard, KV, LoadingOverlay, RefetchIndicator, useDelayedTruthy } from "./ui";
import { fmtBytes } from "./utils";

const HEAP_PALETTE = ["bg-emerald-500", "bg-sky-500", "bg-amber-500", "bg-violet-500"];
const STACK_PALETTE = ["bg-indigo-500", "bg-cyan-500"];
const SYS_PALETTE = ["bg-fuchsia-500", "bg-rose-500", "bg-slate-500"];

export function DashboardTab({ session }: { session: GolangSession }) {
  const runtimeQ = useQuery({
    queryKey: ["golang", session.id, "runtime"],
    queryFn: () => callOp<RuntimeSnapshot>("runtime-snapshot", { sessionId: session.id }),
    enabled: session.gopsAvailable,
    refetchInterval: session.gopsAvailable ? 5_000 : false,
  });
  const parsed = useMemo(() => parseRuntime(runtimeQ.data), [runtimeQ.data]);
  const loading = session.gopsAvailable && runtimeQ.isFetching && !runtimeQ.data;
  const refetching = session.gopsAvailable && runtimeQ.isFetching && !!runtimeQ.data;
  const showRefetching = useDelayedTruthy(refetching);
  const blocked = !session.gopsAvailable || loading;

  return (
    <div className="relative h-full min-h-0">
      {!session.gopsAvailable && <GopsRequiredOverlay>gops is required for the runtime dashboard.</GopsRequiredOverlay>}
      {loading && <LoadingOverlay>Loading runtime data…</LoadingOverlay>}
      {showRefetching && <RefetchIndicator>Refreshing runtime data…</RefetchIndicator>}
      <div className={`flex h-full min-h-0 flex-col gap-4 overflow-auto p-4 ${blocked ? "pointer-events-none blur-sm" : ""}`}>
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold">Runtime Dashboard</h3>
        <span className="text-xs text-muted-foreground">
          {runtimeQ.dataUpdatedAt ? `refreshed ${new Date(runtimeQ.dataUpdatedAt).toLocaleTimeString()}` : ""}
        </span>
      </div>

      <section className="grid grid-cols-2 gap-2 lg:grid-cols-2">
        <InfoCard title="Runtime">
          <KV k="Go" v={(runtimeQ.data?.version ?? "").trim() || "unknown"} />
          <KV k="PID" v={session.pid ? String(session.pid) : "unknown"} />
          <KV k="Goroutines" v={firstValue(parsed.stats, ["goroutines", "goroutine-count"])} />
          <KV k="GOMAXPROCS" v={firstValue(parsed.stats, ["gomaxprocs", "gomax-procs"])} />
          <KV k="CPUs" v={firstValue(parsed.stats, ["numcpu", "num-cpu", "cpus"])} />
        </InfoCard>
        <InfoCard title="GC">
          <KV k="Collections" v={firstValue(parsed.mem, ["num-gc", "numgc"])} />
          <KV k="Forced" v={firstValue(parsed.mem, ["num-forced-gc", "numforcedgc"])} />
          <KV k="Next GC" v={humanMemValue(firstValue(parsed.mem, ["next-gc", "nextgc"]))} />
          <KV k="Pause total" v={firstValue(parsed.mem, ["gc-pause-total", "pausetotalns"])} />
          <KV k="CPU fraction" v={firstValue(parsed.mem, ["gc-cpu-fraction", "gccpufraction"])} />
        </InfoCard>
      </section>

      <section>
        <h4 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">Memory</h4>
        <div className="grid grid-cols-1 gap-3">
          <MemoryRegionCard
            title="Heap"
            used={firstValue(parsed.mem, ["heap-alloc", "heapalloc"])}
            total={firstValue(parsed.mem, ["heap-sys", "heapsys"])}
            palette={HEAP_PALETTE}
            rows={[
              memoryRow("allocated", firstValue(parsed.mem, ["heap-alloc", "heapalloc"]), firstValue(parsed.mem, ["heap-sys", "heapsys"])),
              memoryRow("in-use", firstValue(parsed.mem, ["heap-in-use", "heap-inuse", "heapinuse"]), firstValue(parsed.mem, ["heap-sys", "heapsys"]), true),
              memoryRow("idle", firstValue(parsed.mem, ["heap-idle", "heapidle"]), firstValue(parsed.mem, ["heap-sys", "heapsys"]), true),
              memoryRow("released", firstValue(parsed.mem, ["heap-released", "heapreleased"]), firstValue(parsed.mem, ["heap-sys", "heapsys"])),
              countRow("objects", firstValue(parsed.mem, ["heap-objects", "heapobjects"])),
              countRow("mallocs", firstValue(parsed.mem, ["mallocs"])),
              countRow("frees", firstValue(parsed.mem, ["frees"])),
            ]}
          />
          <MemoryRegionCard
            title="Stack"
            used={firstValue(parsed.mem, ["stack-in-use", "stackinuse"])}
            total={firstValue(parsed.mem, ["stack-sys", "stacksys"])}
            palette={STACK_PALETTE}
            rows={[
              memoryRow("in-use", firstValue(parsed.mem, ["stack-in-use", "stackinuse"]), firstValue(parsed.mem, ["stack-sys", "stacksys"]), true),
              memoryRow("system", firstValue(parsed.mem, ["stack-sys", "stacksys"]), firstValue(parsed.mem, ["sys"])),
            ]}
          />
          <MemoryRegionCard
            title="Runtime"
            used={firstValue(parsed.mem, ["alloc"])}
            total={firstValue(parsed.mem, ["sys"])}
            palette={SYS_PALETTE}
            rows={[
              memoryRow("alloc", firstValue(parsed.mem, ["alloc"]), firstValue(parsed.mem, ["sys"]), true),
              memoryRow("total alloc", firstValue(parsed.mem, ["total-alloc", "totalalloc"]), firstValue(parsed.mem, ["sys"])),
              memoryRow("mspan in-use", firstValue(parsed.mem, ["mspan-in-use", "mspaninuse"]), firstValue(parsed.mem, ["mspan-sys", "mspansys"])),
              memoryRow("mcache in-use", firstValue(parsed.mem, ["mcache-in-use", "mcacheinuse"]), firstValue(parsed.mem, ["mcache-sys", "mcachesys"])),
              memoryRow("gc sys", firstValue(parsed.mem, ["gc-sys", "gcsys"]), firstValue(parsed.mem, ["sys"]), true),
              memoryRow("other sys", firstValue(parsed.mem, ["other-sys", "othersys"]), firstValue(parsed.mem, ["sys"]), true),
            ]}
          />
        </div>
      </section>

      {session.diagnostics?.length ? (
        <section>
          <h4 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">Diagnostics</h4>
          <pre className="max-h-28 overflow-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">
            {session.diagnostics.join("\n")}
          </pre>
        </section>
      ) : null}

      {runtimeQ.data?.error ? <ErrorText error={runtimeQ.data.error} /> : null}
      {runtimeQ.error ? <ErrorText error={runtimeQ.error} /> : null}
      </div>
    </div>
  );
}


type MemoryRow = {
  label: string;
  value?: string;
  bytes?: number | null;
  percent?: number | null;
  countOnly?: boolean;
  segment?: boolean;
};

function MemoryRegionCard({
  title,
  used,
  total,
  palette,
  rows,
}: {
  title: string;
  used?: string;
  total?: string;
  palette: string[];
  rows: MemoryRow[];
}) {
  const usedBytes = parseByteValue(used);
  const totalBytes = parseByteValue(total);
  const pct = usedBytes && totalBytes ? Math.min(100, (usedBytes / totalBytes) * 100) : 0;
  const byteRows = rows.filter((row) => row.segment && !row.countOnly && row.bytes && row.bytes > 0);
  const segments = byteRows.map((row, index) => ({
    count: row.bytes ?? 0,
    color: palette[index % palette.length],
    label: row.label,
  }));
  if (segments.length === 0 && usedBytes) {
    segments.push({ count: usedBytes, color: palette[0], label: "used" });
  }
  const segmentTotal = segments.reduce((sum, segment) => sum + segment.count, 0);
  const barTotal = Math.max(totalBytes || 0, segmentTotal, 1);
  return (
    <div className="flex flex-col gap-3 rounded-md border bg-muted/10 p-3">
      <div className="flex items-center justify-between gap-3 text-xs">
        <strong className="uppercase text-muted-foreground">{title}</strong>
        <span className="font-mono text-muted-foreground">
          {humanMemValue(used) || "unknown"} / {humanMemValue(total) || "unknown"} · {pct ? `${pct.toFixed(0)}%` : "unknown"}
        </span>
      </div>
      <ProgressBar total={barTotal} segments={segments} />
      <div className="grid grid-cols-1 gap-x-6 gap-y-1 md:grid-cols-2">
        {rows.filter((row) => row.value).map((row, index) => (
          <div key={`${title}-${row.label}`} className="grid grid-cols-[1fr_auto_auto] items-center gap-3 text-xs">
            <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
              <span className={`h-2 w-2 shrink-0 rounded-full ${row.countOnly ? "bg-muted-foreground/40" : palette[index % palette.length]}`} />
              <span className="truncate">{row.label}</span>
            </span>
            <span className="font-mono font-semibold">{row.value}</span>
            <span className="w-10 text-right font-mono text-muted-foreground">
              {row.percent == null ? "" : `${row.percent.toFixed(0)}%`}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

function parseRuntime(data?: RuntimeSnapshot): { stats: Record<string, string>; mem: Record<string, string> } {
  return {
    stats: parseKeyValue(data?.stats ?? ""),
    mem: parseKeyValue(data?.memstats ?? ""),
  };
}

function parseKeyValue(raw: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of raw.split("\n")) {
    const [key, value] = line.split(/:\s*/, 2);
    if (key && value) out[key.trim().toLowerCase().replace(/\s+/g, "-")] = value.trim();
  }
  return out;
}

function firstValue(values: Record<string, string>, keys: string[]): string | undefined {
  for (const key of keys) {
    const value = values[key];
    if (value) return value;
  }
  return undefined;
}

function memoryRow(label: string, value?: string, total?: string, segment = false): MemoryRow {
  const bytes = parseByteValue(value);
  const totalBytes = parseByteValue(total);
  return {
    label,
    value: humanMemValue(value),
    bytes,
    percent: bytes && totalBytes ? Math.min(100, (bytes / totalBytes) * 100) : null,
    segment,
  };
}

function countRow(label: string, value?: string): MemoryRow {
  return {
    label,
    value: formatCount(value),
    countOnly: true,
  };
}

function humanMemValue(raw?: string): string | undefined {
  if (!raw) return undefined;
  const value = raw.trim();
  const paren = value.match(/\((\d+) bytes\)/);
  if (paren?.[1]) {
    const formatted = fmtBytes(Number(paren[1]));
    if (value.startsWith("when ")) {
      return value.replace(/>=\s*.+$/, `>= ${formatted}`);
    }
    return formatted;
  }
  const direct = value.match(/^(\d+) bytes$/);
  if (direct?.[1]) return fmtBytes(Number(direct[1]));
  return value.replace(/(\d+(?:\.\d+)?)([KMGT]B)\b/g, "$1 $2");
}

function formatCount(raw?: string): string | undefined {
  if (!raw) return undefined;
  const n = Number(raw.trim());
  if (!Number.isFinite(n)) return raw;
  return new Intl.NumberFormat().format(n);
}

function parseByteValue(raw?: string): number | null {
  if (!raw) return null;
  const paren = raw.match(/\((\d+) bytes\)/);
  if (paren) return Number(paren[1]);
  const direct = raw.match(/^(\d+) bytes$/);
  if (direct) return Number(direct[1]);
  const leading = raw.match(/^(\d+)$/);
  if (leading) return Number(leading[1]);
  return null;
}

