import type { GolangSession, RunningPod, TargetOption } from "../api";

export function sessionMatchesTarget(session: GolangSession, target: TargetOption): boolean {
  return session.namespace === target.namespace && session.pod === target.pod && session.container === target.container;
}

export function flattenTargets(pods: RunningPod[]): TargetOption[] {
  const out: TargetOption[] = [];
  for (const pod of pods) {
    for (const container of pod.containers ?? []) {
      out.push({
        namespace: pod.namespace,
        pod: pod.name,
        container,
        owner: pod.ownerKind ? `${pod.ownerKind}/${pod.ownerName}` : "pod",
        ports: pod.containerPorts?.[container] ?? [],
      });
    }
  }
  return out;
}

export function parsePortInput(value: string): number | undefined {
  const port = Number(value);
  return Number.isInteger(port) && port > 0 && port <= 65535 ? port : undefined;
}

export function portText(port?: number): string {
  return port ? `:${port}` : "unknown";
}

export function fmtBytes(n?: number): string {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let value = n;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function fmtDuration(ms?: number): string {
  if (!ms) return "0s";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
