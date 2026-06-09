import { useEffect, useMemo, useState } from 'preact/hooks'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { callOp, configIDFromURL } from '../lib/api'
import { HealthFixJobsPanel } from '../components/Health/HealthFixJobsPanel'
import { HealthScanPanel } from '../components/Health/HealthScanPanel'
import { HealthSummary } from '../components/Health/HealthSummary'
import { ManualMaintenancePanel } from '../components/Health/ManualMaintenancePanel'
import { FixesPanel } from '../components/Health/FixesPanel'
import { TablesPanel } from '../components/Health/TablesPanel'
import { PermissionsPanel } from '../components/Health/PermissionsPanel'
import type { Fix, FixJob, HealthView, PermissionReport } from '../components/Health/types'
import { defaultSelectedFixIndexes, tableKey } from '../components/Health/shared'

export function HealthTab() {
  const configID = configIDFromURL()
  const qc = useQueryClient()
  const [database, setDatabase] = useState('')
  const [table, setTable] = useState('')
  const [scanMode, setScanMode] = useState('LIMITED')
  const [limit, setLimit] = useState(20)
  const [selectedFixes, setSelectedFixes] = useState<Set<number>>(new Set())
  const [selectedTables, setSelectedTables] = useState<Set<string>>(new Set())
  const [bulkRebuildIndexes, setBulkRebuildIndexes] = useState(true)
  const [bulkUpdateStats, setBulkUpdateStats] = useState(true)

  const permissions = useQuery({
    queryKey: ['permissions', configID],
    queryFn: () => callOp<PermissionReport>('permissions', configID, {}),
    staleTime: 60_000,
    retry: 0,
  })

  const health = useQuery({
    queryKey: ['defrag-health', configID, database, table, scanMode, limit],
    queryFn: () =>
      callOp<HealthView>('defrag-health', configID, {
        database,
        table,
        scanMode,
        limit,
      }),
    enabled: false,
    retry: 0,
  })

  const jobs = useQuery({
    queryKey: ['defrag-fix-jobs', configID],
    queryFn: () => callOp<FixJob[]>('defrag-fix-jobs', configID, {}),
    refetchInterval: 3_000,
  })

  const applyFixes = useMutation({
    mutationFn: (fixes: Fix[]) =>
      callOp<FixJob>('defrag-fix', configID, {
        database: health.data?.health.database || database,
        fixes,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['defrag-fix-jobs'] }),
  })

  const bulk = useMutation({
    mutationFn: (tables: { schema: string; table: string }[]) =>
      callOp<FixJob>('defrag-bulk-rebuild', configID, {
        database: health.data?.health.database || database,
        tables,
        rebuildIndexes: bulkRebuildIndexes,
        updateStats: bulkUpdateStats,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['defrag-fix-jobs'] }),
  })

  const stopJobs = useMutation({
    mutationFn: (id?: string) => callOp('defrag-fix-stop', configID, id ? { id } : {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['defrag-fix-jobs'] }),
  })

  const view = health.data
  const fixes = view?.fixes ?? []
  const tables = view?.health.tables ?? []

  useEffect(() => {
    // Do not pre-select DROP INDEX recommendations; they are destructive and
    // must be an explicit operator choice even though rollback SQL is persisted.
    setSelectedFixes(new Set(defaultSelectedFixIndexes(fixes)))
    setSelectedTables(new Set<string>())
  }, [view])

  const applyableFixIndexes = useMemo(() => defaultSelectedFixIndexes(fixes), [fixes])
  const selectedFixList = useMemo(
    () => [...selectedFixes].map(i => fixes[i]).filter(Boolean),
    [selectedFixes, fixes],
  )
  const selectedTableRefs = useMemo(
    () =>
      tables
        .filter(t => selectedTables.has(tableKey(t)))
        .map(t => ({ schema: t.schema, table: t.tableName })),
    [selectedTables, tables],
  )

  const detailedNeedsTable = scanMode === 'DETAILED' && table.trim() === ''

  return (
    <section className="grid gap-density-2">
      <PermissionsPanel query={permissions} />

      <HealthScanPanel
        database={database}
        setDatabase={setDatabase}
        configID={configID}
        table={table}
        setTable={setTable}
        scanMode={scanMode}
        setScanMode={setScanMode}
        limit={limit}
        setLimit={setLimit}
        query={health}
        detailedNeedsTable={detailedNeedsTable}
      />

      {view && <HealthSummary view={view} />}
      {view && (
        <TablesPanel
          tables={tables}
          usageReliable={view.usageReliable}
          selected={selectedTables}
          onToggle={key => toggleSet(selectedTables, setSelectedTables, key)}
          onToggleAll={() =>
            setSelectedTables(
              selectedTables.size === tables.length
                ? new Set<string>()
                : new Set(tables.map(tableKey)),
            )
          }
        />
      )}
      {view && (
        <ManualMaintenancePanel
          bulkRebuildIndexes={bulkRebuildIndexes}
          setBulkRebuildIndexes={setBulkRebuildIndexes}
          bulkUpdateStats={bulkUpdateStats}
          setBulkUpdateStats={setBulkUpdateStats}
          selectedTableRefs={selectedTableRefs}
          bulk={bulk}
        />
      )}
      {view && (
        <FixesPanel
          fixes={fixes}
          selected={selectedFixes}
          onToggle={i => toggleSet(selectedFixes, setSelectedFixes, i)}
          onToggleAll={() =>
            setSelectedFixes(
              selectedFixes.size === applyableFixIndexes.length
                ? new Set<number>()
                : new Set(applyableFixIndexes),
            )
          }
          onApply={() => applyFixes.mutate(selectedFixList)}
          applying={applyFixes.isPending}
          applyError={applyFixes.error as Error | null}
        />
      )}

      <HealthFixJobsPanel
        jobs={jobs.data ?? []}
        isFetching={jobs.isFetching}
        error={jobs.error as Error | null}
        onRefresh={() => jobs.refetch()}
        onStop={id => stopJobs.mutate(id)}
        onStopAll={() => stopJobs.mutate(undefined)}
      />
    </section>
  )
}

function toggleSet<T>(value: Set<T>, setValue: (next: Set<T>) => void, item: T) {
  const next = new Set(value)
  if (next.has(item)) next.delete(item)
  else next.add(item)
  setValue(next)
}
