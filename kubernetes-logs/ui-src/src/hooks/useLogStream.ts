import { useEffect, useState } from 'react'
import {
  type ErrorDiagnostics,
  type LogsTableInput,
  normalizeErrorDiagnostics,
} from '@flanksource/clicky-ui'

const MAX_LOGS = 5000

function appendLog(prev: LogsTableInput[], entry: LogsTableInput) {
  const next = [...prev, entry]
  return next.length > MAX_LOGS ? next.slice(next.length - MAX_LOGS) : next
}

export function useLogStream(url: string | null, enabled: boolean, nonce: number) {
  const [logs, setLogs] = useState<LogsTableInput[]>([])
  const [status, setStatus] = useState('')
  const [error, setError] = useState<ErrorDiagnostics | null>(null)

  useEffect(() => {
    setLogs([])
    setError(null)
    if (!enabled || !url) {
      setStatus('')
      return
    }

    const es = new EventSource(url, { withCredentials: true })
    setStatus('following')

    es.onmessage = ev => {
      try {
        setLogs(prev => appendLog(prev, JSON.parse(ev.data) as LogsTableInput))
      } catch {
        setLogs(prev => appendLog(prev, ev.data))
      }
    }

    es.addEventListener('error', ev => {
      const data = 'data' in ev ? String(ev.data ?? '') : ''
      if (data) {
        setError(normalizeErrorDiagnostics(data) ?? { message: data, context: [] })
      } else {
        setStatus('stream closed')
      }
    })

    return () => {
      es.close()
    }
  }, [enabled, url, nonce])

  return { logs, status, error }
}
