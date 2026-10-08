import { useEffect, useState } from 'react'
import type { QueryParams } from '@flanksource/mission-control-sdk'
import {
  type ErrorDiagnostics,
  type LogsTableInput,
  normalizeErrorDiagnostics,
} from '@flanksource/clicky-ui'
import { diagnosticsFromError, pluginClient } from '../api/kubernetesLogs'

const MAX_LOGS = 5000

function appendLog(prev: LogsTableInput[], entry: LogsTableInput) {
  const next = [...prev, entry]
  return next.length > MAX_LOGS ? next.slice(next.length - MAX_LOGS) : next
}

export function useLogStream(query: QueryParams | null, enabled: boolean, nonce: number) {
  const [logs, setLogs] = useState<LogsTableInput[]>([])
  const [status, setStatus] = useState('')
  const [error, setError] = useState<ErrorDiagnostics | null>(null)

  useEffect(() => {
    setLogs([])
    setError(null)
    if (!enabled || !query) {
      setStatus('')
      return
    }

    const controller = new AbortController()
    setStatus('following')

    const follow = async () => {
      for await (const { event, data } of pluginClient.stream('logs', query, { signal: controller.signal })) {
        if (controller.signal.aborted) return
        if (event === 'error') {
          setError(normalizeErrorDiagnostics(data) ?? { message: data, context: [] })
        } else if (event === 'message') {
          try {
            const entry = JSON.parse(data) as LogsTableInput
            setLogs(prev => appendLog(prev, entry))
          } catch {
            setLogs(prev => appendLog(prev, data))
          }
        }
      }
      if (!controller.signal.aborted) setStatus('stream closed')
    }
    void follow().catch(err => {
      if (!controller.signal.aborted) {
        setError(diagnosticsFromError(err))
        setStatus('stream closed')
      }
    })

    return () => controller.abort()
  }, [enabled, query, nonce])

  return { logs, status, error }
}
