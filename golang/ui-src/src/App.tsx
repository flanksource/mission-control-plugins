import { useEffect, useMemo, useState } from "preact/hooks";
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { Activity, RefreshCw, Trash2 } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import {
  callOp,
  configIDFromURL,
  type GolangSession,
  type RunningPod,
} from "./api";
import { DashboardTab } from "./components/DashboardTab";
import { GoroutinesTab } from "./components/GoroutinesTab";
import { ProfilerTab } from "./components/ProfilerTab";
import { SessionMenu } from "./components/SessionMenu";
import { SessionSummary } from "./components/SessionSummary";
import type { ActiveTab, SessionStartTarget } from "./components/types";
import { Empty, ErrorText, TabButton } from "./components/ui";
import { flattenTargets, sessionMatchesTarget } from "./components/utils";

const sessionsKey = (configID: string) => ["golang", configID, "sessions"] as const;
const podsKey = (configID: string) => ["golang", configID, "pods"] as const;

export function App() {
  const configID = configIDFromURL();
  const [selectedTarget, setSelectedTarget] = useState<SessionStartTarget | null>(null);
  const [selectedSessionID, setSelectedSessionID] = useState<string | null>(null);
  const [tab, setTab] = useState<ActiveTab>("dashboard");
  const qc = useQueryClient();

  const podsQ = useQuery({
    queryKey: podsKey(configID),
    queryFn: () => callOp<RunningPod[]>("pods-list"),
    enabled: !!configID,
    refetchInterval: 15_000,
  });

  const sessionsQ = useQuery({
    queryKey: sessionsKey(configID),
    queryFn: () => callOp<GolangSession[]>("sessions-list"),
    enabled: !!configID,
    refetchInterval: 5_000,
  });

  const targets = useMemo(() => flattenTargets(podsQ.data ?? []), [podsQ.data]);
  const sessions = sessionsQ.data ?? [];
  const selectedTargetSession = useMemo(
    () => (selectedTarget ? sessions.find((s) => sessionMatchesTarget(s, selectedTarget)) ?? null : null),
    [sessions, selectedTarget],
  );
  const selectedSession = useMemo(
    () => (selectedTarget ? selectedTargetSession : sessions.find((s) => s.id === selectedSessionID) ?? null),
    [sessions, selectedSessionID, selectedTarget, selectedTargetSession],
  );

  useEffect(() => {
    if (selectedTarget || targets.length === 0) return;
    const activeTarget = targets.find((target) => sessions.some((session) => sessionMatchesTarget(session, target)));
    setSelectedTarget(activeTarget ?? targets[0]);
  }, [selectedTarget, targets, sessions]);

  useEffect(() => {
    setSelectedSessionID(selectedTargetSession?.id ?? null);
  }, [selectedTargetSession]);

  const startSession = useMutation({
    mutationFn: (target: SessionStartTarget) =>
      callOp<GolangSession>("session-create", {
        namespace: target.namespace,
        pod: target.pod,
        container: target.container,
        useGops: target.useGops,
        usePprof: target.usePprof,
        gopsPort: target.gopsPort,
        pprofPort: target.pprofPort,
      }),
    onSuccess: (session, target) => {
      setSelectedTarget(target);
      setSelectedSessionID(session.id);
      qc.setQueryData<GolangSession[]>(sessionsKey(configID), (old = []) => [
        session,
        ...old.filter((item) => item.id !== session.id),
      ]);
      qc.invalidateQueries({ queryKey: sessionsKey(configID) });
    },
  });

  const deleteSession = useMutation({
    mutationFn: (id: string) => callOp("session-delete", { id }),
    onSuccess: (_, id) => {
      if (selectedSessionID === id) setSelectedSessionID(null);
      qc.setQueryData<GolangSession[]>(sessionsKey(configID), (old = []) => old.filter((item) => item.id !== id));
      qc.invalidateQueries({ queryKey: sessionsKey(configID) });
    },
  });

  if (!configID) {
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-2 bg-background p-8 text-center text-sm text-muted-foreground">
        <Activity className="h-8 w-8" />
        <p>No config_id in the iframe URL.</p>
      </div>
    );
  }

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <header className="flex shrink-0 items-center justify-between gap-3 border-b bg-card px-4 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <Activity className="h-4 w-4 text-primary" />
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold">Golang Diagnostics</div>
            <div className="truncate text-xs text-muted-foreground">
              {selectedSession ? `${selectedSession.namespace}/${selectedSession.pod}` : "No active session"}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" title="Refresh" onClick={() => refreshAll(qc)}>
            <RefreshCw className="h-4 w-4" />
          </Button>
          <SessionMenu
            targets={targets}
            sessions={sessions}
            selectedTarget={selectedTarget}
            selectedSession={selectedSession}
            sessionsLoading={sessionsQ.isLoading}
            targetsLoading={podsQ.isLoading}
            targetsError={podsQ.error}
            creating={startSession.isPending}
            deletingId={deleteSession.isPending ? String(deleteSession.variables ?? "") || null : null}
            onSelectTarget={(target) => {
              const session = sessions.find((item) => sessionMatchesTarget(item, target));
              setSelectedTarget(target);
              setSelectedSessionID(session?.id ?? null);
            }}
            onStartSession={(target) => startSession.mutate(target)}
            onDeleteSession={(id) => deleteSession.mutate(id)}
          />
        </div>
      </header>

      <div className="min-h-0 flex-1 p-3">
        <section className="flex h-full min-h-0 flex-col rounded-md border bg-card">
          <div className="flex shrink-0 items-center justify-between gap-3 border-b px-3 py-2">
            <SessionSummary session={selectedSession} />
            <Button
              size="sm"
              variant="destructive"
              loading={deleteSession.isPending}
              disabled={!selectedSession}
              onClick={() => selectedSession && deleteSession.mutate(selectedSession.id)}
            >
              <Trash2 className="h-4 w-4" />
              Stop
            </Button>
          </div>
          <div className="flex shrink-0 gap-1 border-b px-3 py-2">
            <TabButton tab="dashboard" current={tab} onClick={setTab}>Dashboard</TabButton>
            <TabButton tab="goroutines" current={tab} onClick={setTab}>Goroutines</TabButton>
            <TabButton tab="profiler" current={tab} onClick={setTab}>Profiler</TabButton>
          </div>
          <div className="min-h-0 flex-1 overflow-hidden">
            {startSession.error && <div className="p-3"><ErrorText error={startSession.error} /></div>}
            {selectedSession ? (
              <>
                {tab === "dashboard" && <DashboardTab session={selectedSession} />}
                {tab === "goroutines" && <GoroutinesTab session={selectedSession} />}
                {tab === "profiler" && <ProfilerTab session={selectedSession} />}
              </>
            ) : (
              <Empty>Select a target and start a session.</Empty>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

function refreshAll(qc: QueryClient) {
  qc.invalidateQueries({ queryKey: ["golang"] });
}
