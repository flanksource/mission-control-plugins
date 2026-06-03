import { Button } from '@flanksource/clicky-ui'
import { CheckCircle2, CircleAlert, Play, Radar, RefreshCw } from 'lucide-react'
import type { Status } from '../types'
import { pluginBuildDate, pluginVersion } from '../version'

type HeaderProps = {
  status: Status | null
  canStart: boolean
  onStartTrace: () => void
  onRefresh: () => void
}

export function Header({ status, canStart, onStartTrace, onRefresh }: HeaderProps) {
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
              <span className={`status-badge header-status ${statusTone}`}>
                {status?.ready ? <CheckCircle2 size={11} /> : <CircleAlert size={11} />}
                {statusLabel}
              </span>
            </div>
            <p>eBPF observability for Kubernetes workloads</p>
          </div>
        </div>

        <div className="header-actions">
          <Button variant="outline" size="sm" onClick={onStartTrace} disabled={!canStart}>
            <Play size={14} />
            Start trace
          </Button>
          <Button variant="outline" size="icon" onClick={onRefresh} title="Refresh">
            <RefreshCw size={16} />
          </Button>
        </div>
      </div>

      {!status?.ready && (
        <div className="status-strip">
          Inspektor Gadget must be installed. The plugin expects the <code>gadget</code> daemonset
          to be installed in <code>{status?.namespace || 'gadget'}</code> namespace with pod label:{' '}
          <code>k8s-app=gadget</code>.
        </div>
      )}

      {problems ? (
        <div className="header-problems" title={problems}>
          <CircleAlert size={14} />
          <span>{problems}</span>
        </div>
      ) : null}
    </header>
  )
}
