import type { Fix, TableHealth } from './types'

export const inputCls = 'h-control-h rounded-md border border-input bg-background px-2 text-sm'
export const labelCls = 'inline-flex items-center gap-density-1 text-xs text-muted-foreground'
export const thCls = 'px-density-2 py-density-1 text-left font-semibold text-foreground'
export const tdCls = 'px-density-2 py-density-1 align-top text-xs'
export const monoTd = tdCls + ' font-mono'

export function EmptyState({ title, body }: { title: string; body: string }) {
  return (
    <div className="bg-muted/20 p-density-4 text-center">
      <div className="font-medium text-foreground">{title}</div>
      <p className="mx-auto mt-density-1 max-w-xl text-sm text-muted-foreground">{body}</p>
    </div>
  )
}

export function Mark({ ok }: { ok: boolean }) {
  return ok ? <span className="text-green-600">✓</span> : <span className="text-red-600">✗</span>
}

export function tableKey(t: TableHealth): string {
  return `${t.schema}.${t.tableName}`
}

export function defaultSelectedFixIndexes(fixes: Fix[]): number[] {
  const selected: number[] = []
  fixes.forEach((f, i) => {
    if (!f.kind.includes('DROP')) selected.push(i)
  })
  return selected
}

export function fixTone(kind: string): string {
  if (kind.includes('DROP')) return ' text-red-600'
  if (kind.includes('REBUILD')) return ' text-amber-600'
  return ''
}
