import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ErrorDetails, LogsTable } from '@flanksource/clicky-ui'
import type { DataTableColumn, LogsTableRow } from '@flanksource/clicky-ui/data'
import { buildLogsUrl, diagnosticsFromError, fetchLogs, listPods } from './api/kubernetesLogs'
import { LogsToolbar } from './components/LogsToolbar'
import { getConfigId } from './hooks/useConfigId'
import { useLogStream } from './hooks/useLogStream'
import { parseSelectedPod, selectedPodValue } from './utils/pods'

const LOG_COLUMNS: DataTableColumn<LogsTableRow>[] = [
  {
    key: 'timestamp',
    label: 'Timestamp',
    kind: 'timestamp',
    shrink: true,
    minWidth: 180,
    timestamp: { autoRangeFilter: false },
  },
  {
    key: 'message',
    label: 'Message',
    grow: true,
    minWidth: 360,
    cellClassName: 'font-mono text-xs',
  },
  { key: 'tags', label: 'Tags', kind: 'tags', grow: true, tags: { maxVisible: 4 } },
]

export function LogsApp() {
  const configId = getConfigId()

  const [selectedPod, setSelectedPod] = useState('')
  const [container, setContainer] = useState('')
  const [tailLines, setTailLines] = useState(200)
  const [follow, setFollow] = useState(false)
  const [streamNonce, setStreamNonce] = useState(0)

  const podsQuery = useQuery({
    queryKey: ['kubernetes-logs', 'pods', configId],
    queryFn: () => listPods(configId),
    enabled: Boolean(configId),
  })

  const pods = podsQuery.data ?? []

  useEffect(() => {
    if (!podsQuery.isFetched && !podsQuery.isError) return
    window.parent?.postMessage({ type: 'mc.tab.ready' }, '*')
  }, [podsQuery.isFetched, podsQuery.isError])

  useEffect(() => {
    if (!podsQuery.data) return
    if (podsQuery.data.length === 0) {
      setSelectedPod('')
      return
    }

    const currentStillExists = podsQuery.data.some(p => selectedPodValue(p) === selectedPod)
    if (!currentStillExists) {
      setSelectedPod(selectedPodValue(podsQuery.data[0]))
    }
  }, [podsQuery.data, selectedPod])

  const selectedPodRef = useMemo(() => parseSelectedPod(selectedPod), [selectedPod])

  const logsUrl = useMemo(() => {
    if (!selectedPodRef) return null
    return buildLogsUrl({ configId, selectedPod: selectedPodRef, container, tailLines, follow })
  }, [configId, selectedPodRef, container, tailLines, follow])

  const logsQuery = useQuery({
    queryKey: ['kubernetes-logs', 'logs', logsUrl],
    queryFn: ({ signal }) => fetchLogs(logsUrl!, signal),
    enabled: Boolean(logsUrl && !follow),
  })

  const stream = useLogStream(logsUrl, Boolean(logsUrl && follow), streamNonce)

  const podOptions = useMemo(
    () =>
      pods.map(p => ({
        value: selectedPodValue(p),
        label: `${p.namespace}/${p.pod} (${p.phase})`,
      })),
    [pods],
  )

  const logs = follow ? stream.logs : (logsQuery.data ?? [])
  const error = podsQuery.error
    ? diagnosticsFromError(podsQuery.error)
    : logsQuery.error
      ? diagnosticsFromError(logsQuery.error)
      : stream.error

  const fullscreenTitle = selectedPod ? `Logs · ${selectedPod.replaceAll('|', '/')}` : 'Logs'

  return (
    <div className="flex h-screen flex-col gap-density-2 p-density-3">
      <LogsToolbar
        pod={selectedPod}
        podOptions={podOptions}
        container={container}
        tailLines={tailLines}
        follow={follow}
        canReload={Boolean(selectedPodRef)}
        onPodChange={setSelectedPod}
        onContainerChange={setContainer}
        onTailLinesChange={setTailLines}
        onFollowChange={setFollow}
        onReload={() => {
          if (follow) {
            setStreamNonce(n => n + 1)
          } else {
            void logsQuery.refetch()
          }
        }}
      />
      {error && <ErrorDetails diagnostics={error} />}
      <div className="min-h-0 flex-1">
        <LogsTable
          logs={logs}
          columns={LOG_COLUMNS}
          fullscreenTitle={fullscreenTitle}
          showFullscreenControl={false}
          hideableColumns={false}
          className="h-full"
        />
      </div>
    </div>
  )
}
