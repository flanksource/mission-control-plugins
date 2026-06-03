import { Button } from '@flanksource/clicky-ui'
import { CheckCircle2, CircleAlert, Loader2, Play, Radar, RefreshCw, Wrench } from 'lucide-react'
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
  const statusLabel = status?.ready
    ? 'Ready'
    : status?.installed
      ? 'Installed, not ready'
      : 'Not installed'
  const statusTone = status?.ready ? 'ok' : 'warn'

  return (
    <header className="header-card">
      <div className="header-main">
        <div className="brand">
          <span className="brand-icon" aria-hidden="true">
            <Radar size={18} />
          </span>
          <div className="brand-copy">
            <div className="brand-title-row">
              <h1>Inspektor Gadget</h1>
              <span className="version">
                v{pluginVersion}
                {pluginBuildDate ? ` ${pluginBuildDate}` : ''}
              </span>
            </div>
            <p>eBPF observability for Kubernetes workloads</p>
          </div>
        </div>

        <div className="header-actions">
          <Button size="sm" onClick={onStartTrace} disabled={!canStart}>
            <Play size={14} />
            Start trace
          </Button>
          <Button variant="outline" size="icon" onClick={onRefresh} title="Refresh">
            <RefreshCw size={16} />
          </Button>
        </div>
      </div>

      <div className="status-strip">
        <span className={`status-badge ${statusTone}`}>
          {status?.ready ? <CheckCircle2 size={14} /> : <CircleAlert size={14} />}
          {statusLabel}
        </span>
        <span className="meta-chip">
          <span>Namespace</span>
          <strong>{status?.namespace || 'gadget'}</strong>
        </span>
        {status?.expectedTag && (
          <span className="meta-chip">
            <span>Expected</span>
            <strong>{status.expectedTag}</strong>
          </span>
        )}
        {status?.version && (
          <span className="meta-chip">
            <span>Image</span>
            <strong>{status.version}</strong>
          </span>
        )}
        {status?.desired !== undefined && (
          <span className="meta-chip">
            <span>Pods</span>
            <strong>
              {status.readyPods || 0}/{status.desired}
            </strong>
          </span>
        )}
        {!status?.ready && (
          <button className="install-button" onClick={onInstall} disabled={busy === 'install'}>
            {busy === 'install' ? <Loader2 className="spin" size={14} /> : <Wrench size={14} />}
            Install
          </button>
        )}
      </div>

      {problems ? (
        <div className="header-problems" title={problems}>
          <CircleAlert size={14} />
          <span>{problems}</span>
        </div>
      ) : null}
    </header>
  )
}
