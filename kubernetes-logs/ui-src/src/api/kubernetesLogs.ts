import {
  type LogsTableInput,
  normalizeErrorDiagnostics,
  type ErrorDiagnostics,
} from '@flanksource/clicky-ui'
import type { PodRow, SelectedPod } from '../types'

export const PLUGIN_BASE = '/api/plugins/kubernetes-logs'

export class HttpError extends Error {
  constructor(
    message: string,
    readonly diagnostics: ErrorDiagnostics,
  ) {
    super(message)
    this.name = 'HttpError'
  }
}

async function fetchOrThrow(input: string, init: RequestInit, label: string): Promise<Response> {
  const res = await fetch(input, init)
  if (res.ok) return res

  const fallback = `${label} failed: HTTP ${res.status}`
  let payload: unknown
  try {
    payload = await res.clone().json()
  } catch {
    payload = (await res.text().catch(() => '')) || fallback
  }

  const diagnostics = normalizeErrorDiagnostics(payload, fallback) ?? {
    message: fallback,
    context: [],
  }
  throw new HttpError(diagnostics.message, diagnostics)
}

export async function listPods(configId: string): Promise<PodRow[]> {
  const res = await fetchOrThrow(
    `${PLUGIN_BASE}/invoke/list-pods?config_id=${encodeURIComponent(configId)}`,
    {
      method: 'POST',
      body: '{}',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
    },
    'list-pods',
  )
  const rows = (await res.json()) as PodRow[]
  return Array.isArray(rows) ? rows : []
}

export async function fetchLogs(url: string, signal?: AbortSignal): Promise<LogsTableInput[]> {
  const res = await fetchOrThrow(
    url,
    {
      method: 'GET',
      credentials: 'same-origin',
      signal,
    },
    'logs',
  )
  const rows = (await res.json()) as LogsTableInput[]
  return Array.isArray(rows) ? rows : []
}

export function buildLogsUrl({
  configId,
  selectedPod,
  container,
  tailLines,
  follow,
}: {
  configId: string
  selectedPod: SelectedPod
  container: string
  tailLines: number
  follow: boolean
}) {
  const params = new URLSearchParams({
    pod: selectedPod.pod,
    config_id: configId,
    namespace: selectedPod.namespace,
    container,
    tailLines: String(follow ? 0 : tailLines),
    follow: follow ? 'true' : 'false',
  })
  return `${PLUGIN_BASE}/proxy/logs?${params.toString()}`
}

export function diagnosticsFromError(err: unknown): ErrorDiagnostics {
  if (err instanceof HttpError) return err.diagnostics
  return (
    normalizeErrorDiagnostics(err instanceof Error ? err.message : String(err)) ?? {
      message: String(err),
      context: [],
    }
  )
}
