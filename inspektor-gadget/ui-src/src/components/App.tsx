import React, { useEffect, useMemo, useRef, useState } from "react";
import { invoke as sdkInvoke, ready } from "@flanksource/plugin-ui-sdk";
import {
  Button,
  cn
} from "@flanksource/clicky-ui";
import {
  ChevronLeft,
  ChevronRight,
  Clock,
  Loader2,
  Square
} from "lucide-react";
import { KeyValue } from "./KeyValue";
import { EventPanel } from "./EventPanel";
import { Header } from "./Header";
import { StartTraceDialog } from "./StartTraceDialog";
import type { GadgetSpec, Session, Status, TraceEvent } from "../types";
import { sessionIconFor, widgetLabel } from "../utils/gadgets";

function configId() {
  return new URLSearchParams(window.location.search).get("config_id") || "";
}

async function invoke<T>(op: string, body: unknown = {}): Promise<T> {
  const res = await sdkInvoke(op, body);
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json() as Promise<T>;
}

export function App() {
  const [status, setStatus] = useState<Status | null>(null);
  const [gadgets, setGadgets] = useState<GadgetSpec[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [selectedGadget, setSelectedGadget] = useState("trace_exec");
  const [container, setContainer] = useState("");
  const [durationSec, setDurationSec] = useState(300);
  const [optionValues, setOptionValues] = useState<Record<string, unknown>>({});
  const [argText, setArgText] = useState("");
  const [events, setEvents] = useState<TraceEvent[]>([]);
  const [selectedSession, setSelectedSession] = useState<string>("");
  const [busy, setBusy] = useState<string>("");
  const [error, setError] = useState<string>("");
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [startDialogOpen, setStartDialogOpen] = useState(false);
  const [sessionsOpen, setSessionsOpen] = useState(true);
  const [sessionsWidth, setSessionsWidth] = useState(320);
  const resizeCleanupRef = useRef<(() => void) | null>(null);

  async function refresh() {
    setError("");
    const [nextStatus, nextGadgets, nextSessions] = await Promise.all([
      invoke<Status>("status"),
      invoke<GadgetSpec[]>("traces-list"),
      invoke<Session[]>("trace-list")
    ]);
    setStatus(nextStatus);
    setGadgets(nextGadgets);
    setSessions(nextSessions);
    if (!selectedSession && nextSessions.length > 0) {
      setSelectedSession(nextSessions[0].id);
    } else if (selectedSession && !nextSessions.some((session) => session.id === selectedSession)) {
      setSelectedSession(nextSessions[0]?.id ?? "");
    }
  }

  useEffect(() => {
    refresh().catch((err) => setError(String(err)));
    ready();
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => {
      invoke<Session[]>("trace-list")
        .then((nextSessions) => {
          setSessions(nextSessions);
          setSelectedSession((current) => {
            if (!current) return nextSessions[0]?.id ?? "";
            return nextSessions.some((session) => session.id === current) ? current : nextSessions[0]?.id ?? "";
          });
        })
        .catch(() => undefined);
    }, 5000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    return () => resizeCleanupRef.current?.();
  }, []);

  const activeSession = useMemo(
    () => sessions.find((session) => session.id === selectedSession) || null,
    [sessions, selectedSession]
  );

  useEffect(() => {
    setEvents([]);
    if (!selectedSession) return;
    let cancelled = false;
    invoke<TraceEvent[]>("trace-events", { id: selectedSession })
      .then((next) => {
        if (cancelled) return;
        setEvents((next ?? []).slice(-1000));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [selectedSession, activeSession?.state]);
  const activeGadgetSpec = useMemo(
    () => gadgets.find((gadget) => gadget.id === activeSession?.gadgetId) || null,
    [gadgets, activeSession?.gadgetId]
  );
  const selectedGadgetSpec = useMemo(
    () => gadgets.find((gadget) => gadget.id === selectedGadget) || null,
    [gadgets, selectedGadget]
  );
  const categories = useMemo(() => {
    return Array.from(new Set(gadgets.map((gadget) => gadget.category)));
  }, [gadgets]);

  useEffect(() => {
    setOptionValues({});
  }, [selectedGadget]);

  async function startTrace() {
    setBusy("start");
    setError("");
    try {
      const options = {
        ...selectedOptionPayload(selectedGadgetSpec, optionValues),
        ...parseArgumentText(argText)
      };
      const session = await invoke<Session>("trace-start", {
        gadget: selectedGadget,
        container: container || undefined,
        durationSec,
        options
      });
      setSelectedSession(session.id);
      setStartDialogOpen(false);
      await refresh();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy("");
    }
  }

  async function stopTrace(sessionId: string) {
    setBusy(`stop:${sessionId}`);
    try {
      await invoke<Session>("trace-stop", { id: sessionId });
      await refresh();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy("");
    }
  }

  function beginSessionsResize(event: React.MouseEvent<HTMLDivElement>) {
    event.preventDefault();
    resizeCleanupRef.current?.();
    const startX = event.clientX;
    const startWidth = sessionsWidth;
    const move = (moveEvent: MouseEvent) => {
      const next = Math.min(520, Math.max(240, startWidth + moveEvent.clientX - startX));
      setSessionsWidth(next);
    };
    const cleanup = () => {
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
    };
    const up = () => {
      cleanup();
      resizeCleanupRef.current = null;
    };
    resizeCleanupRef.current = cleanup;
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  }

  return (
    <div className="app">
      <Header
        status={status}
        canStart={Boolean(configId())}
        onStartTrace={() => setStartDialogOpen(true)}
        onRefresh={() => refresh().catch((err) => setError(String(err)))}
      />
      {error && <div className="error">{error}</div>}

      <main
        className={`workspace ${sessionsOpen ? "" : "sessions-collapsed"}`}
        style={{ "--sessions-width": `${sessionsWidth}px` } as React.CSSProperties}
      >
        <section className={`panel sessions ${sessionsOpen ? "" : "collapsed"}`}>
          <div className="sessions-heading">
            {sessionsOpen && <div className="panel-title">Sessions</div>}
            <Button
              variant="outline"
              size="icon"
              onClick={() => setSessionsOpen((open) => !open)}
              title={sessionsOpen ? "Collapse sessions" : "Expand sessions"}
            >
              {sessionsOpen ? <ChevronLeft size={15} /> : <ChevronRight size={15} />}
            </Button>
          </div>
          {sessionsOpen && (
            <>
              {sessions.length === 0 ? <div className="empty">No sessions</div> : sessions.map((session) => {
                const stoppable = isStoppable(session);
                const stopping = busy === `stop:${session.id}`;
                const SessionIcon = sessionIconFor(session, gadgets);
                return (
                  <div key={session.id} className={cn("session", session.id === selectedSession && "selected")}>
                    <button className="session-main" onClick={() => setSelectedSession(session.id)}>
                      <span className="session-title-row">
                        <SessionIcon size={14} />
                        <span className="session-name">{session.gadgetName || session.gadgetId}</span>
                        <span className={`session-state ${session.state}`}>{session.state}</span>
                        <span className="session-count">{session.eventCount}</span>
                      </span>
                      {stoppable && (
                        <span className="session-countdown">
                          <Clock size={13} />
                          {sessionTimerLabel(session, nowMs)}
                        </span>
                      )}
                    </button>
                    {stoppable && (
                      <Button variant="ghost" size="icon" className="session-stop" onClick={() => stopTrace(session.id)} disabled={stopping} title="Stop trace">
                        {stopping ? <Loader2 className="spin" size={14} /> : <Square size={14} />}
                      </Button>
                    )}
                  </div>
                );
              })}
              {activeSession && (
                <div className="session-details">
                  <div className="panel-title">Run Diagnostics</div>
                  {activeSession.error && (
                    <div className="failure-reason">
                      <span>Failed reason</span>
                      <code>{activeSession.error}</code>
                    </div>
                  )}
                  <KeyValue label="Image" value={activeSession.gadgetImage} mono />
                  <KeyValue label="Tag" value={activeSession.gadgetTag} />
                  <KeyValue label="Widget" value={widgetLabel(activeGadgetSpec?.widget || activeSession.gadgetWidget || activeSession.diagnostics?.gadgetWidget || "table")} />
                  <KeyValue label="Runtime" value={activeSession.diagnostics?.runtime} />
                  <KeyValue label="Connection" value={activeSession.diagnostics?.connection} />
                  <KeyValue label="Duration" value={`${activeSession.diagnostics?.durationSec || 0}s`} />
                  <KeyValue label="Max events" value={String(activeSession.diagnostics?.maxEvents || 0)} />
                  <KeyValue label="Target" value={targetLabel(activeSession)} mono />
                  <KeyValue label="Started" value={new Date(activeSession.startedAt).toLocaleString()} />
                  {activeSession.diagnostics?.startedByEmail && <KeyValue label="User" value={activeSession.diagnostics.startedByEmail} />}
                  {activeSession.diagnostics?.resolvedPods?.length ? (
                    <div className="pod-list">
                      {activeSession.diagnostics.resolvedPods.slice(0, 5).map((pod) => (
                        <code key={`${pod.namespace}/${pod.name}`}>{pod.namespace}/{pod.name}{pod.node ? ` @ ${pod.node}` : ""}</code>
                      ))}
                    </div>
                  ) : null}
                  <details>
                    <summary>Runtime params</summary>
                    <pre>{JSON.stringify(activeSession.diagnostics?.runtimeParams || {}, null, 2)}</pre>
                  </details>
                </div>
              )}
            </>
          )}
        </section>
        {sessionsOpen && <div className="resize-handle" onMouseDown={beginSessionsResize} title="Resize sessions" />}

        <EventPanel
          activeSession={activeSession}
          activeGadgetSpec={activeGadgetSpec}
          events={events}
        />
      </main>
      {startDialogOpen && (
        <StartTraceDialog
          gadgets={gadgets}
          categories={categories}
          selectedGadget={selectedGadget}
          selectedGadgetSpec={selectedGadgetSpec}
          setSelectedGadget={setSelectedGadget}
          container={container}
          setContainer={setContainer}
          durationSec={durationSec}
          setDurationSec={setDurationSec}
          optionValues={optionValues}
          setOptionValues={setOptionValues}
          argText={argText}
          setArgText={setArgText}
          busy={busy}
          canStart={Boolean(configId())}
          onClose={() => setStartDialogOpen(false)}
          onStart={startTrace}
        />
      )}
    </div>
  );
}

function targetLabel(session: Session) {
  const target = session.target;
  return [
    target.namespace,
    target.pod || `${target.name || ""}`,
    target.container,
    target.node
  ].filter(Boolean).join(" / ");
}

function selectedOptionPayload(gadget: GadgetSpec | null, values: Record<string, unknown>) {
  const options: Record<string, unknown> = {};
  for (const option of gadget?.options || []) {
    const value = values[option.name] ?? option.default;
    if (value !== undefined && value !== "") {
      options[option.name] = value;
    }
  }
  return options;
}

function parseArgumentText(text: string) {
  const trimmed = text.trim();
  if (!trimmed) return {};
  if (trimmed.startsWith("{")) {
    const parsed = JSON.parse(trimmed) as Record<string, unknown>;
    if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
      throw new Error("extra arguments JSON must be an object");
    }
    return parsed;
  }
  const options: Record<string, string | boolean> = {};
  for (const rawLine of trimmed.split(/\r?\n/)) {
    let line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    line = line.replace(/^--/, "");
    let idx = line.indexOf("=");
    if (idx < 0) idx = line.indexOf(":");
    if (idx < 0) {
      options[line] = true;
      continue;
    }
    const key = line.slice(0, idx).trim();
    const value = line.slice(idx + 1).trim();
    if (!key) throw new Error(`invalid extra argument: ${rawLine}`);
    options[key] = value;
  }
  return options;
}

function isStoppable(session: Session) {
  return session.state === "starting" || session.state === "running";
}

function sessionTimerLabel(session: Session, nowMs: number) {
  if (!isStoppable(session)) {
    if (session.stoppedAt) return new Date(session.stoppedAt).toLocaleTimeString();
    return session.state;
  }
  const remaining = remainingSeconds(session, nowMs);
  if (remaining === null) return "timed";
  return `${formatDuration(remaining)} left`;
}

function remainingSeconds(session: Session, nowMs: number) {
  const duration = session.diagnostics?.durationSec || 0;
  const startedAt = Date.parse(session.startedAt);
  if (!duration || Number.isNaN(startedAt)) return null;
  return Math.max(0, Math.ceil((startedAt + duration * 1000 - nowMs) / 1000));
}

function formatDuration(totalSeconds: number) {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}
