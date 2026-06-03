import { invoke } from "@flanksource/plugin-ui-sdk";

export interface RunningPod {
  namespace: string;
  name: string;
  node?: string;
  containers: string[];
  containerPorts?: Record<string, number[]>;
  ownerKind?: string;
  ownerName?: string;
}

export interface TargetOption {
  namespace: string;
  pod: string;
  container: string;
  owner: string;
  ports: number[];
}

export interface GolangSession {
  id: string;
  configItemId?: string;
  namespace: string;
  kind: string;
  name: string;
  pod: string;
  container: string;
  pid?: number;
  gopsRemotePort?: number;
  pprofRemotePort?: number;
  pprofBasePath?: string;
  gopsAvailable: boolean;
  pprofAvailable: boolean;
  startedAt: string;
  diagnostics?: string[];
}

export interface RuntimeSnapshot {
  sessionId: string;
  version?: string;
  stats?: string;
  memstats?: string;
  error?: string;
}

export interface GoroutineSnapshot {
  sessionId: string;
  source: string;
  dump: string;
  error?: string;
}

export type ProfileKind = "cpu" | "trace" | "heap";
export type ProfileSource = "auto" | "pprof" | "gops";
export type ProfileState = "running" | "completed" | "failed" | "stopped";

export interface ProfileRun {
  id: string;
  sessionId: string;
  kind: ProfileKind;
  source?: string;
  preference?: ProfileSource;
  state: ProfileState;
  seconds?: number;
  startedAt: string;
  completedAt?: string;
  elapsedMs: number;
  bytes?: number;
  error?: string;
  url?: string;
}

export interface FlamegraphNode {
  name: string;
  value: number;
  self?: number;
  children?: FlamegraphNode[];
}

export interface ProfileFlamegraph {
  sampleType: string;
  unit: string;
  total: number;
  sampleTypes: string[];
  root: FlamegraphNode;
}

export function configIDFromURL(): string {
  return new URLSearchParams(window.location.search).get("config_id") ?? "";
}

export async function callOp<T>(op: string, params: Record<string, unknown> = {}): Promise<T> {
  const res = await invoke(op, params);
  return responseJSON<T>(res);
}

export async function fetchProfileFlamegraph(sessionID: string, runID: string, sampleIndex?: string): Promise<ProfileFlamegraph> {
  const res = await invoke("profiles", undefined, {
    method: "GET",
    query: { path: `${sessionID}/${runID}/flamegraph-data`, si: sampleIndex },
  });
  return responseJSON<ProfileFlamegraph>(res);
}

export async function fetchProfileTop(sessionID: string, runID: string, sampleIndex?: string): Promise<string> {
  const res = await invoke("profiles", undefined, {
    method: "GET",
    query: { path: `${sessionID}/${runID}/top`, si: sampleIndex },
  });
  if (!res.ok) throw new Error(await res.text() || res.statusText);
  return res.text();
}

export async function fetchProfileBlob(sessionID: string, runID: string): Promise<Blob> {
  const res = await invoke("profiles", undefined, {
    method: "GET",
    query: { path: `${sessionID}/${runID}` },
  });
  if (!res.ok) throw new Error(await res.text() || res.statusText);
  return res.blob();
}

async function responseJSON<T>(res: Response): Promise<T> {
  if (!res.ok) throw new Error(await res.text() || res.statusText);
  return (await res.json()) as T;
}
