import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bug, ChevronDown, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { toastManager } from "@/components/ui/toast";
import { callOp, configIDFromURL } from "@/lib/api";
import { ArthasDashboardTab } from "./ArthasDashboardTab";
import { ArthasMBeanTab } from "./ArthasMBeanTab";
import { ArthasOgnlTab } from "./ArthasOgnlTab";
import { ArthasProfilerTab } from "./ArthasProfilerTab";

interface ArthasSession {
  id: string;
  namespace: string;
  kind: string;
  name: string;
  pod: string;
  container: string;
  httpLocalPort: number;
  startedAt: string;
  javaVersion?: number;
  jdkProvisioned?: boolean;
  sideloadedJavaHome?: string;
}

interface RunningPod {
  namespace: string;
  name: string;
  containers: string[];
  ownerKind?: string;
  ownerName?: string;
}

interface SessionCreateJob {
  jobId: string;
  status: "pending" | "running" | "failed";
  sessionId?: string;
  session?: ArthasSession;
  error?: string;
  startedAt: string;
  finishedAt?: string;
}

const sessionsKey = (configID: string) => ["arthas", configID, "sessions"] as const;

export function ArthasPage() {
  const configID = configIDFromURL();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [createJobId, setCreateJobId] = useState<string | null>(null);
  const qc = useQueryClient();

  const sessionsQ = useQuery({
    queryKey: sessionsKey(configID),
    queryFn: () => callOp<ArthasSession[]>("sessions-list"),
    enabled: !!configID,
    refetchInterval: 5_000,
  });

  const podsQ = useQuery({
    queryKey: ["arthas", "pods", configID],
    queryFn: () => callOp<RunningPod[]>("pods-list"),
    enabled: !!configID,
    staleTime: 15_000,
  });

  const create = useMutation({
    mutationFn: (body: Record<string, unknown>) => callOp<SessionCreateJob>("session-create", body),
    onSuccess: (job) => {
      if (job.status === "running" && job.session) {
        qc.invalidateQueries({ queryKey: sessionsKey(configID) });
        setSelectedId(job.session.id);
        toastManager.add({ title: `Arthas session started on ${job.session.pod}`, type: "success" });
        return;
      }
      setCreateJobId(job.jobId);
      toastManager.add({ title: "Starting Arthas session…", type: "info" });
    },
  });

  const createStatusQ = useQuery({
    queryKey: ["arthas", "session-creation-status", createJobId],
    queryFn: () => callOp<SessionCreateJob>("session-creation-status", { jobId: createJobId }),
    enabled: !!createJobId,
    refetchInterval: createJobId ? 2_000 : false,
  });

  useEffect(() => {
    const job = createStatusQ.data;
    if (!job || !createJobId) return;
    if (job.status === "running" && job.session) {
      qc.invalidateQueries({ queryKey: sessionsKey(configID) });
      setSelectedId(job.session.id);
      setCreateJobId(null);
      toastManager.add({ title: `Arthas session started on ${job.session.pod}`, type: "success" });
    } else if (job.status === "failed") {
      setCreateJobId(null);
      toastManager.add({ title: job.error ? `Failed to start Arthas session: ${job.error}` : "Failed to start Arthas session", type: "error" });
    }
  }, [configID, createJobId, createStatusQ.data, qc]);

  const del = useMutation({
    mutationFn: (id: string) => callOp("session-delete", { id }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: sessionsKey(configID) });
      if (selectedId === id) setSelectedId(null);
    },
  });

  const sessions = sessionsQ.data ?? [];
  const selected = useMemo(
    () => sessions.find((s) => s.id === selectedId) ?? sessions[0] ?? null,
    [sessions, selectedId],
  );

  if (!configID) {
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-2 p-8 text-center text-sm text-muted-foreground">
        <Bug className="h-8 w-8" />
        <p>No config_id in the iframe URL.</p>
      </div>
    );
  }

  return (
    <div className="flex h-screen bg-background p-3 text-foreground">
      <main className="min-w-0 flex-1">
        {selected ? (
          <SessionDetail
            session={selected}
            sessions={sessions}
            selectedId={selected.id}
            sessionsLoading={sessionsQ.isLoading}
            pods={podsQ.data ?? []}
            podsLoading={podsQ.isLoading}
            podsError={podsQ.error}
            creating={create.isPending || !!createJobId}
            deletingId={del.isPending ? String(del.variables ?? "") || null : null}
            onSelectSession={setSelectedId}
            onCreateSession={(body) => create.mutate(body)}
            onDeleteSession={(id) => del.mutate(id)}
          />
        ) : (
          <EmptyState
            sessions={sessions}
            selectedId={selectedId}
            sessionsLoading={sessionsQ.isLoading}
            pods={podsQ.data ?? []}
            podsLoading={podsQ.isLoading}
            podsError={podsQ.error}
            creating={create.isPending || !!createJobId}
            deletingId={del.isPending ? String(del.variables ?? "") || null : null}
            onSelectSession={setSelectedId}
            onCreateSession={(body) => create.mutate(body)}
            onDeleteSession={(id) => del.mutate(id)}
          />
        )}
      </main>
    </div>
  );
}

type SessionMenuProps = {
  sessions: ArthasSession[];
  selectedId: string | null;
  sessionsLoading: boolean;
  pods: RunningPod[];
  podsLoading: boolean;
  podsError: unknown;
  creating: boolean;
  deletingId: string | null;
  onSelectSession: (id: string) => void;
  onCreateSession: (body: Record<string, unknown>) => void;
  onDeleteSession: (id: string) => void;
};

function SessionMenu({
  sessions,
  selectedId,
  sessionsLoading,
  pods,
  podsLoading,
  podsError,
  creating,
  deletingId,
  onSelectSession,
  onCreateSession,
  onDeleteSession,
}: SessionMenuProps) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const selected = sessions.find((s) => s.id === selectedId) ?? null;
  const targets = useMemo(() => sessionTargets(pods, sessions), [pods, sessions]);

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [open]);

  return (
    <div ref={menuRef} className="relative">
      <Button
        variant="outline"
        className="max-w-[22rem] justify-between"
        onClick={() => setOpen((value) => !value)}
      >
        <span className="min-w-0 truncate">
          {selected ? `${selected.kind}/${selected.name}` : "Sessions"}
        </span>
        <ChevronDown className="h-3 w-3 shrink-0" />
      </Button>

      {open && (
        <div className="absolute right-0 top-full z-30 mt-2 w-[34rem] max-w-[calc(100vw-2rem)] rounded-md border border-border bg-background p-1 shadow-lg">
          <div className="px-2 py-1 text-[11px] font-semibold uppercase text-muted-foreground">Targets</div>
          {sessionsLoading || podsLoading ? (
            <div className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground">
              <Spinner className="h-4 w-4" />
              Loading targets
            </div>
          ) : podsError ? (
            <div className="px-2 py-3 text-xs text-red-600">
              {podsError instanceof Error ? podsError.message : "Failed to load pods"}
            </div>
          ) : targets.length === 0 ? (
            <div className="px-2 py-3 text-xs text-muted-foreground">No ready pods resolved for this resource.</div>
          ) : (
            <div className="max-h-96 overflow-auto">
              {targets.map((target) => (
                <div
                  key={`${target.pod.namespace}/${target.pod.name}/${target.container}`}
                  className={`grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded px-2 py-2 hover:bg-muted ${
                    selectedId === target.session?.id ? "bg-muted" : ""
                  }`}
                >
                  <button
                    type="button"
                    className="min-w-0 text-left text-xs"
                    disabled={!target.session}
                    onClick={() => {
                      if (!target.session) return;
                      onSelectSession(target.session.id);
                      setOpen(false);
                    }}
                  >
                    <span className="block truncate font-medium">{target.pod.name}</span>
                    <span className="block truncate text-muted-foreground">
                      {target.pod.ownerKind ? `${target.pod.ownerKind}/${target.pod.ownerName}` : "pod"}
                      {target.container ? ` / ${target.container}` : ""}
                    </span>
                    {target.session && (
                      <span className="block truncate text-muted-foreground">
                        running since {formatSessionTime(target.session.startedAt)}
                      </span>
                    )}
                  </button>
                  {target.session ? (
                    <Button
                      size="xs"
                      variant="ghost"
                      loading={deletingId === target.session.id}
                      onClick={() => onDeleteSession(target.session!.id)}
                    >
                      <Trash2 className="h-3 w-3" /> Stop
                    </Button>
                  ) : (
                    <Button
                      size="xs"
                      variant="secondary"
                      loading={creating}
                      onClick={() => {
                        onCreateSession({
                          namespace: target.pod.namespace,
                          kind: target.pod.ownerKind || "pod",
                          name: target.pod.ownerName || target.pod.name,
                          pod: target.pod.name,
                          container: target.container,
                        });
                        setOpen(false);
                      }}
                    >
                      <Plus className="h-3 w-3" /> Start
                    </Button>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

type PodSessionTarget = {
  pod: RunningPod;
  container: string;
  session?: ArthasSession;
};

function sessionTargets(pods: RunningPod[], sessions: ArthasSession[]): PodSessionTarget[] {
  const sessionsByTarget = new Map<string, ArthasSession>();
  for (const session of sessions) {
    sessionsByTarget.set(targetKey(session.namespace, session.pod, session.container), session);
  }

  return pods.flatMap((pod) => {
    const containers = pod.containers.length > 0 ? pod.containers : [""];
    return containers.map((container) => ({
      pod,
      container,
      session: sessionsByTarget.get(targetKey(pod.namespace, pod.name, container)),
    }));
  });
}

function targetKey(namespace: string, pod: string, container: string): string {
  return `${namespace}/${pod}/${container}`;
}

function formatSessionTime(startedAt: string): string {
  const date = new Date(startedAt);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function EmptyState(props: SessionMenuProps) {
  return (
    <div className="flex h-full flex-col rounded-md border border-border">
      <div className="m-2 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 px-2 text-sm font-semibold">
          <Bug className="h-4 w-4" />
          Arthas
        </div>
        <SessionMenu {...props} />
      </div>
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 p-8 text-center text-sm text-muted-foreground">
        <Bug className="h-8 w-8" />
        <p>Start an Arthas session to attach to the JVM running inside this Kubernetes resource.</p>
      </div>
    </div>
  );
}

function SessionDetail({
  session,
  ...menuProps
}: {
  session: ArthasSession;
} & SessionMenuProps) {
  const [tab, setTab] = useState("dashboard");

  return (
    <Tabs value={tab} onValueChange={setTab} className="flex h-full flex-col rounded-md border border-border">
      <div className="m-2 flex items-center justify-between gap-2">
        <TabsList className="w-fit">
          <TabsTrigger value="dashboard">Dashboard</TabsTrigger>
          <TabsTrigger value="ognl">OGNL</TabsTrigger>
          <TabsTrigger value="mbeans">MBeans</TabsTrigger>
          <TabsTrigger value="profiler">Profiler</TabsTrigger>
          <TabsTrigger value="info">Info</TabsTrigger>
        </TabsList>
        <SessionMenu {...menuProps} selectedId={session.id} />
      </div>
      <TabsContent value="dashboard" className="min-h-0 flex-1 overflow-y-auto p-0">
        <ArthasDashboardTab sessionId={session.id} />
      </TabsContent>
      <TabsContent value="ognl" className="min-h-0 flex-1 overflow-hidden p-0">
        <ArthasOgnlTab sessionId={session.id} />
      </TabsContent>
      <TabsContent value="mbeans" className="min-h-0 flex-1 overflow-hidden p-0">
        <ArthasMBeanTab sessionId={session.id} />
      </TabsContent>
      <TabsContent value="profiler" className="min-h-0 flex-1 overflow-hidden p-0">
        <ArthasProfilerTab sessionId={session.id} />
      </TabsContent>
      <TabsContent value="info" className="flex-1 overflow-auto p-4 text-sm">
        <dl className="grid grid-cols-[9rem_1fr] gap-2">
          <dt className="text-muted-foreground">Session ID</dt>
          <dd className="font-mono">{session.id}</dd>
          <dt className="text-muted-foreground">Namespace</dt>
          <dd>{session.namespace}</dd>
          <dt className="text-muted-foreground">Target</dt>
          <dd>{session.kind}/{session.name}</dd>
          <dt className="text-muted-foreground">Pod</dt>
          <dd>{session.pod}</dd>
          <dt className="text-muted-foreground">Container</dt>
          <dd>{session.container}</dd>
          <dt className="text-muted-foreground">Java</dt>
          <dd>{session.javaVersion ?? "unknown"}{session.jdkProvisioned ? ` (JDK side-loaded at ${session.sideloadedJavaHome ?? "/tmp/jdk"})` : ""}</dd>
          <dt className="text-muted-foreground">Started</dt>
          <dd>{new Date(session.startedAt).toLocaleString()}</dd>
        </dl>
      </TabsContent>
    </Tabs>
  );
}
