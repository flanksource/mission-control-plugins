import {
  type LogsTableInput,
  normalizeErrorDiagnostics,
  type ErrorDiagnostics,
} from '@flanksource/clicky-ui'
import { createEmbeddedPluginClient, type QueryParams } from '@flanksource/mission-control-sdk'
import type { PodRow, SelectedPod } from '../types'
import { getConfigId } from '../hooks/useConfigId'

export const pluginClient = createEmbeddedPluginClient({
  name: 'kubernetes-logs',
  configId: getConfigId() || undefined,
})

export class HttpError extends Error {
  constructor(
    message: string,
    readonly diagnostics: ErrorDiagnostics,
  ) {
    super(message)
    this.name = 'HttpError'
  }
}

async function responseOrThrow(res: Response, label: string): Promise<Response> {
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

export async function listPods(signal?: AbortSignal): Promise<PodRow[]> {
  const res = await responseOrThrow(
    await pluginClient.invoke('list-pods', {}, { signal }),
    'list-pods',
  )
  const rows = (await res.json()) as PodRow[]
  return Array.isArray(rows) ? rows : []
}

export async function fetchLogs(query: QueryParams, signal?: AbortSignal): Promise<LogsTableInput[]> {
  const res = await responseOrThrow(
    await pluginClient.invoke('logs', query, { method: 'GET', proxy: true, signal }),
    'logs',
  )
  const rows = (await res.json()) as LogsTableInput[]
  return Array.isArray(rows) ? rows : []
}

export function buildLogsQuery({
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
  return {
    pod: selectedPod.pod,
    config_id: configId,
    namespace: selectedPod.namespace,
    container,
    tailLines: follow ? 0 : tailLines,
    follow,
  }
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
