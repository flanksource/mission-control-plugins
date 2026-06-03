import { Button } from '@flanksource/clicky-ui'
import { Loader2, Play, Radar, RefreshCw, Wrench } from 'lucide-react'
import type { Status } from '../types'
import { pluginBuildDate, pluginVersion } from '../version'

type HeaderProps = {
  status: Status | null
  busy: string
  canStart: boolean
  onInstall: () => void
  onStartTrace: () => void
  onRefresh: () => void
}

export function Header({
  status,
  busy,
  canStart,
  onInstall,
  onStartTrace,
  onRefresh,
}: HeaderProps) {
  const problems = status?.problems?.join(' ')

  return (
    <header className={`header-card ${status?.ready ? 'ok' : 'warn'}`}>
      <div className="brand">
        <Radar size={18} />
        <span>Inspektor Gadget</span>
        <span className="version">
          v{pluginVersion}
          {pluginBuildDate ? ` ${pluginBuildDate}` : ''}
        </span>
      </div>

      <div className="status-strip">
        <strong>
          {status?.ready ? 'Ready' : status?.installed ? 'Installed, not ready' : 'Not installed'}
        </strong>
        <span>namespace {status?.namespace || 'gadget'}</span>
        <span>expected {status?.expectedTag || ''}</span>
        {status?.version && <span>image {status.version}</span>}
        {status?.desired !== undefined && (
          <span>
            pods {status.readyPods || 0}/{status.desired}
          </span>
        )}
        {!status?.ready && (
          <button className="secondary" onClick={onInstall} disabled={busy === 'install'}>
            {busy === 'install' ? <Loader2 className="spin" size={14} /> : <Wrench size={14} />}
            Install
          </button>
        )}
      </div>

      {problems ? (
        <div className="header-problems" title={problems}>
          {problems}
        </div>
      ) : null}

      <div className="header-actions">
        <Button size="sm" onClick={onStartTrace} disabled={!canStart}>
          <Play size={14} />
          Start trace
        </Button>
        <Button variant="outline" size="icon" onClick={onRefresh} title="Refresh">
          <RefreshCw size={16} />
        </Button>
      </div>
    </header>
  )
}
