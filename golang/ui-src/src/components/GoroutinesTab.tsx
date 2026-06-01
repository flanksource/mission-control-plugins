import { useMemo, useState } from "preact/hooks";
import { useQuery } from "@tanstack/react-query";
import { Badge, countGoroutinesByState, parseGoroutineDump, type ParsedGoroutine } from "@flanksource/clicky-ui";
import { callOp, type GolangSession, type GoroutineSnapshot } from "../api";
import { Empty, ErrorText, GopsRequiredOverlay, LoadingOverlay, RefetchIndicator, useDelayedTruthy } from "./ui";

export function GoroutinesTab({ session }: { session: GolangSession }) {
  const [query, setQuery] = useState("");
  const [hideRuntimeOnly, setHideRuntimeOnly] = useState(true);
  const goroutinesQ = useQuery({
    queryKey: ["golang", session.id, "goroutines"],
    queryFn: () => callOp<GoroutineSnapshot>("goroutines", { sessionId: session.id }),
    enabled: session.gopsAvailable,
    refetchInterval: session.gopsAvailable ? 5_000 : false,
    refetchIntervalInBackground: false,
  });
  const dump = goroutinesQ.data?.dump ?? "";
  const parsed = useMemo(() => parseGoroutineDump(dump), [dump]);
  const filtered = useMemo(() => filterGoroutines(parsed, query, hideRuntimeOnly), [parsed, query, hideRuntimeOnly]);
  const counts = useMemo(() => countGoroutinesByState(parsed), [parsed]);
  const loading = session.gopsAvailable && goroutinesQ.isFetching && !goroutinesQ.data;
  const refetching = session.gopsAvailable && goroutinesQ.isFetching && !!goroutinesQ.data;
  const showRefetching = useDelayedTruthy(refetching);
  const blocked = !session.gopsAvailable || loading;

  return (
    <div className="relative h-full min-h-0">
      {!session.gopsAvailable && <GopsRequiredOverlay>gops is required to inspect goroutines.</GopsRequiredOverlay>}
      {loading && <LoadingOverlay>Loading goroutines…</LoadingOverlay>}
      {showRefetching && <RefetchIndicator>Refreshing goroutines…</RefetchIndicator>}
      <div className={`flex h-full min-h-0 flex-col gap-3 p-4 ${blocked ? "pointer-events-none blur-sm" : ""}`}>
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">Goroutines</h3>
          <p className="text-xs text-muted-foreground">
            {goroutinesQ.data?.source ? `source: ${goroutinesQ.data.source}` : "Load the current stack dump."}
          </p>
        </div>

      </div>
      <div className="flex flex-wrap items-center gap-2">
        <input
          className="h-8 min-w-72 rounded-md border bg-background px-2 text-xs"
          value={query}
          onInput={(event) => setQuery((event.target as HTMLInputElement).value)}
          placeholder="Filter function, file, or state"
        />
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={hideRuntimeOnly}
            onChange={(event) => setHideRuntimeOnly((event.target as HTMLInputElement).checked)}
          />
          Hide runtime-only stacks
        </label>
        {[...counts.entries()].map(([state, count]) => (
          <Badge key={state} variant="outline" size="sm">{state}: {count}</Badge>
        ))}
      </div>
      {goroutinesQ.data?.error ? (
        <ErrorText error={goroutinesQ.data.error} />
      ) : goroutinesQ.error ? (
        <ErrorText error={goroutinesQ.error} />
      ) : dump && parsed.length === 0 ? (
        <pre className="min-h-0 flex-1 overflow-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">{dump}</pre>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto rounded-md border p-2">
          {filtered.length === 0 ? (
            <Empty>No goroutines match the current filter.</Empty>
          ) : (
            <div className="flex flex-col divide-y">
              {filtered.map((goroutine) => (
                <GoroutineDetails key={goroutine.id} goroutine={goroutine} query={query} hideRuntimeOnly={hideRuntimeOnly} />
              ))}
            </div>
          )}
        </div>
      )}
      </div>
    </div>
  );
}
function GoroutineDetails({
  goroutine,
  query,
  hideRuntimeOnly,
}: {
  goroutine: ParsedGoroutine;
  query: string;
  hideRuntimeOnly: boolean;
}) {
  const frames = hideRuntimeOnly
    ? goroutine.frames.filter((frame) => !frame.runtime || frame.kind === "created_by")
    : goroutine.frames;
  const open = goroutine.state === "running" || !!query;
  return (
    <details open={open} className="py-1">
      <summary className="cursor-pointer list-none px-2 py-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-xs font-semibold">g{goroutine.id}</span>
          <Badge variant="outline" size="sm">{goroutine.rawState}</Badge>
          <span className="text-[11px] text-muted-foreground">{frames.length} frames</span>
          {goroutine.topFunction && <span className="truncate text-[11px] text-muted-foreground">{goroutine.topFunction}</span>}
        </div>
      </summary>
      <div className="space-y-0.5 px-2 pb-2 pl-5">
        {frames.map((frame, index) => (
          <div key={`${goroutine.id}-${index}`} className={frame.runtime ? "text-muted-foreground" : "text-foreground"}>
            <div className="break-all font-mono text-[11px] font-semibold leading-4">
              {frame.displayName}
              {frame.location && <span className="ml-2 text-[10px] font-normal opacity-80">{frame.location}</span>}
            </div>
          </div>
        ))}
      </div>
    </details>
  );
}

function filterGoroutines(goroutines: ParsedGoroutine[], query: string, hideRuntimeOnly: boolean): ParsedGoroutine[] {
  const q = query.trim().toLowerCase();
  return goroutines.filter((goroutine) => {
    if (hideRuntimeOnly && goroutine.userFrameCount === 0) return false;
    if (!q) return true;
    return goroutine.searchText.includes(q) || goroutine.rawState.toLowerCase().includes(q);
  });
}

