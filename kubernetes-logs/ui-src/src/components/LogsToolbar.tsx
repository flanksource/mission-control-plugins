import { Button, Select } from '@flanksource/clicky-ui'

type Option = {
  value: string
  label: string
}

type LogsToolbarProps = {
  pod: string
  podOptions: Option[]
  container: string
  tailLines: number
  follow: boolean
  canReload: boolean
  onPodChange: (value: string) => void
  onContainerChange: (value: string) => void
  onTailLinesChange: (value: number) => void
  onFollowChange: (value: boolean) => void
  onReload: () => void
}

export function LogsToolbar({
  pod,
  podOptions,
  container,
  tailLines,
  follow,
  canReload,
  onPodChange,
  onContainerChange,
  onTailLinesChange,
  onFollowChange,
  onReload,
}: LogsToolbarProps) {
  return (
    <div className="flex flex-wrap items-center gap-density-2">
      <label className="flex items-center gap-density-1 text-xs text-muted-foreground">
        Pod
        <div className="min-w-[260px]">
          <Select
            value={pod}
            options={podOptions}
            onChange={(e: React.ChangeEvent<HTMLSelectElement>) => onPodChange(e.currentTarget.value)}
            disabled={podOptions.length === 0}
          />
        </div>
      </label>
      <label className="flex items-center gap-density-1 text-xs text-muted-foreground">
        Container
        <input
          className="h-control-h rounded-md border border-input bg-background px-2 text-sm"
          placeholder="(all)"
          value={container}
          onChange={e => onContainerChange(e.currentTarget.value)}
        />
      </label>
      <label className="flex items-center gap-density-1 text-xs text-muted-foreground">
        Tail
        <input
          type="number"
          min={1}
          max={5000}
          className="h-control-h w-20 rounded-md border border-input bg-background px-2 text-sm"
          value={tailLines}
          onChange={e => onTailLinesChange(Number(e.currentTarget.value) || 200)}
        />
      </label>
      <label className="flex items-center gap-density-1.5 text-xs text-muted-foreground">
        <button
          type="button"
          role="switch"
          aria-checked={follow}
          onClick={() => onFollowChange(!follow)}
          className={`mr-2 relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${
            follow ? 'bg-primary' : 'bg-muted'
          }`}
        >
          <span
            className={`pointer-events-none block h-4 w-4 rounded-full bg-background shadow-sm ring-0 transition-transform ${
              follow ? 'translate-x-4' : 'translate-x-0.5'
            }`}
          />
        </button>
        Follow
      </label>
      <Button
        variant="outline"
        size="icon"
        className="ml-auto border-border bg-card text-foreground hover:bg-muted hover:text-foreground"
        onClick={onReload}
        disabled={!canReload}
        aria-label="Refresh logs"
        title="Refresh logs"
      >
        <svg
          className="h-4 w-4"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" />
          <path d="M3 21v-5h5" />
          <path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" />
          <path d="M21 3v5h-5" />
        </svg>
      </Button>
    </div>
  )
}
