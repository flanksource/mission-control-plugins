import { useEffect, useRef, useState } from "preact/hooks";
import { ChevronDown, Play, Trash2 } from "lucide-react";
import { Badge, Button } from "@flanksource/clicky-ui";
import type { GolangSession, TargetOption } from "../api";
import { Field } from "./ui";
import type { SessionStartTarget } from "./types";
import { errorMessage, parsePortInput, sessionMatchesTarget } from "./utils";

const DEFAULT_GOPS_PORT = 6060
const DEFAULT_PPROF_PORT = 6060;

type SessionMenuProps = {
  targets: TargetOption[];
  sessions: GolangSession[];
  selectedTarget: TargetOption | null;
  selectedSession: GolangSession | null;
  sessionsLoading: boolean;
  targetsLoading: boolean;
  targetsError: unknown;
  creating: boolean;
  deletingId: string | null;
  onSelectTarget: (target: TargetOption) => void;
  onStartSession: (target: SessionStartTarget) => void;
  onDeleteSession: (id: string) => void;
};

export function SessionMenu({
  targets,
  sessions,
  selectedTarget,
  selectedSession,
  sessionsLoading,
  targetsLoading,
  targetsError,
  creating,
  deletingId,
  onSelectTarget,
  onStartSession,
  onDeleteSession,
}: SessionMenuProps) {
  const [open, setOpen] = useState(false);
  const [gopsPort, setGopsPort] = useState(String(DEFAULT_GOPS_PORT));
  const [pprofPort, setPprofPort] = useState(String(DEFAULT_PPROF_PORT));
  const menuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [open]);

  const label = selectedSession
    ? `${selectedSession.pod} / ${selectedSession.container}`
    : selectedTarget
      ? `${selectedTarget.pod} / ${selectedTarget.container}`
      : "Targets";

  return (
    <div ref={menuRef} className="relative">
      <Button variant="outline" className="max-w-[22rem] justify-between" onClick={() => setOpen((value) => !value)}>
        <span className="min-w-0 truncate">{label}</span>
        <ChevronDown className="h-3 w-3 shrink-0" />
      </Button>

      {open && (
        <div className="absolute right-0 top-full z-30 mt-2 w-[34rem] max-w-[calc(100vw-2rem)] rounded-md border bg-background p-1 shadow-lg">
          <div className="flex items-center justify-between px-2 py-1">
            <span className="text-[11px] font-semibold uppercase text-muted-foreground">Targets</span>
            <Badge variant="outline" size="sm">{targets.length}</Badge>
          </div>
          <div className="grid grid-cols-2 gap-2 border-y px-2 py-2">
            <Field label="gops port">
              <input
                className="h-8 rounded-md border bg-background px-2 text-xs"
                type="number"
                min={1}
                max={65535}
                value={gopsPort}
                onInput={(event) => setGopsPort((event.target as HTMLInputElement).value)}
              />
            </Field>
            <Field label="pprof port">
              <input
                className="h-8 rounded-md border bg-background px-2 text-xs"
                type="number"
                min={1}
                max={65535}
                value={pprofPort}
                onInput={(event) => setPprofPort((event.target as HTMLInputElement).value)}
              />
            </Field>
          </div>
          {sessionsLoading || targetsLoading ? (
            <div className="px-2 py-3 text-xs text-muted-foreground">Loading targets…</div>
          ) : targetsError ? (
            <div className="px-2 py-3 text-xs text-red-600">{errorMessage(targetsError)}</div>
          ) : targets.length === 0 ? (
            <div className="px-2 py-3 text-xs text-muted-foreground">No ready pods resolved for this resource.</div>
          ) : (
            <div className="max-h-96 overflow-auto">
              {targets.map((target) => {
                const session = sessions.find((item) => sessionMatchesTarget(item, target));
                const selected =
                  selectedTarget?.namespace === target.namespace &&
                  selectedTarget?.pod === target.pod &&
                  selectedTarget?.container === target.container;
                return (
                  <div
                    key={`${target.namespace}/${target.pod}/${target.container}`}
                    className={`grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded px-2 py-2 hover:bg-accent ${
                      selected ? "bg-primary/10" : ""
                    }`}
                  >
                    <button
                      type="button"
                      className="min-w-0 text-left text-xs"
                      onClick={() => {
                        onSelectTarget(target);
                        if (session) setOpen(false);
                      }}
                    >
                      <span className="block truncate font-medium">{target.pod}</span>
                      <span className="block truncate text-muted-foreground">
                        {target.owner} / {target.container}
                      </span>
                      <span className="mt-1 flex flex-wrap items-center gap-1">
                        <Badge variant="outline" size="sm">{target.namespace}</Badge>
                        {target.ports.length > 0 && <Badge variant="outline" size="sm">ports {target.ports.join(", ")}</Badge>}
                        {session && <Badge tone="success" variant="soft" size="sm">active</Badge>}
                      </span>
                    </button>
                    {session ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        loading={deletingId === session.id}
                        onClick={() => onDeleteSession(session.id)}
                      >
                        <Trash2 className="h-3 w-3" /> Stop
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        variant="secondary"
                        loading={creating}
                        onClick={() => {
                          onSelectTarget(target);
                          onStartSession({
                            ...target,
                            gopsPort: parsePortInput(gopsPort),
                            pprofPort: parsePortInput(pprofPort),
                          });
                          setOpen(false);
                        }}
                      >
                        <Play className="h-3 w-3" /> Start
                      </Button>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
