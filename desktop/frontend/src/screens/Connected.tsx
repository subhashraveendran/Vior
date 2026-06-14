// Connected screen — the full operations surface that only appears once
// a device is paired. Shows:
//   • device card (name, resolution, transport, connected-since)
//   • virtual display info (resolution, refresh rate, mode)
//   • Files pane (when the Files sidebar item is active)
//   • Permissions status card (Accessibility green/red + one-tap fix)
//   • Disconnect button
//
// Pre-connect (ready / waiting) hides all of this — the App.tsx router
// is responsible for that gating, this component only renders when a
// client is live.
import React, { useEffect, useState } from 'react'
import { Icons } from '../lib/icons'
import FilesPane from '../panes/Files'
import type { ServerStatus, ClientInfo } from '../types'

type DisplayMode = 'extend' | 'mirror'

type ConnectedScreenProps = {
  status: ServerStatus | null
  client: ClientInfo | null
  mode: DisplayMode
  setMode: (m: DisplayMode) => void
  onModeExtend: () => void
  onModeMirror: () => void
  onDisconnect: () => void
  onSendFile: () => void
  errorState: boolean
  onRetry: () => void
  onStop: () => void
  // accessibilityOk: true = granted, false = missing, null = unknown.
  accessibilityOk: boolean | null
  // onFixAccessibility opens the macOS deep-link dialog (HasAccessibility(true))
  // which routes the user to Settings → Privacy → Accessibility.
  onFixAccessibility: () => void
  // showFilesTab swaps the right-hand pane into the FilesPane when the
  // user clicks the Files sidebar item.
  showFilesTab: boolean
}

// Refresh rate isn't on ServerStatus today; show 60Hz as the documented
// default (config.DefaultRefreshRate). Promote to a real field if/when
// the desktop ever supports non-60Hz virtual displays.
const DEFAULT_REFRESH_HZ = 60

function elapsedSince(iso: string | undefined): string {
  if (!iso) return '—'
  const ms = Date.now() - new Date(iso).getTime()
  if (!isFinite(ms) || ms < 0) return '—'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ${s % 60}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

export default function ConnectedScreen({
  status, client, mode, setMode, onDisconnect, onSendFile,
  onModeExtend, onModeMirror,
  errorState, onRetry, onStop,
  accessibilityOk, onFixAccessibility, showFilesTab,
}: ConnectedScreenProps): React.JSX.Element {
  // Tick once a second so the "Connected for Xs" badge feels live.
  const [, setTick] = useState(0)
  useEffect(() => {
    const id = setInterval(() => setTick(n => n + 1), 1000)
    return () => clearInterval(id)
  }, [])

  const connectedFor = elapsedSince(client?.connectedAt)
  const transport = (client?.connectionType || 'wifi').toUpperCase()

  if (showFilesTab) {
    return (
      <div className="connected-v2">
        <ConnectedHeader client={client} transport={transport} connectedFor={connectedFor} errorState={errorState} />
        <div className="connected-scroll">
          <FilesPane onSendFile={onSendFile} client={client} />
        </div>
        {errorState && <DisconnectModal client={client} onStop={onStop} onRetry={onRetry} />}
      </div>
    )
  }

  return (
    <div className="connected-v2">
      <ConnectedHeader client={client} transport={transport} connectedFor={connectedFor} errorState={errorState} />

      <div className="connected-scroll">
        {/* Permissions block — only render when something needs attention.
            When accessibility is granted the card disappears with the
            checkmark animation (handled by the .perm-card-ok keyframe). */}
        {accessibilityOk === false && (
          <div className="perm-card perm-card-bad">
            <div className="perm-card-icon">{Icons.shield(20)}</div>
            <div className="perm-card-body">
              <div className="perm-card-title">Accessibility access needed</div>
              <div className="perm-card-sub">
                Remote taps and trackpad won't reach the Mac until Vior is enabled
                in System Settings → Privacy &amp; Security → Accessibility.
              </div>
            </div>
            <a
              className="btn btn-primary btn-sm"
              href="x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
              onClick={onFixAccessibility}
            >
              {Icons.arrowR(15)} Fix
            </a>
          </div>
        )}
        {accessibilityOk === true && (
          <div className="perm-card perm-card-ok">
            <div className="perm-card-icon">{Icons.check(20)}</div>
            <div className="perm-card-body">
              <div className="perm-card-title">All permissions granted</div>
              <div className="perm-card-sub">Remote, screen mirroring, and file transfer are all live.</div>
            </div>
          </div>
        )}

        {/* Virtual display details — the user usually wants to verify
            "yes, the phone is actually treated as a second screen" at a
            glance without going hunting in System Settings → Displays. */}
        <div className="section">
          <div className="section-head"><div className="label">Virtual display</div></div>
          <div className="vdisp-grid">
            <div className="vdisp-cell">
              <div className="vdisp-val">{client?.width || '—'}×{client?.height || '—'}</div>
              <div className="vdisp-label">Resolution</div>
            </div>
            <div className="vdisp-cell">
              <div className="vdisp-val">{DEFAULT_REFRESH_HZ}<span className="vdisp-unit"> Hz</span></div>
              <div className="vdisp-label">Target refresh</div>
            </div>
            <div className="vdisp-cell">
              <div className="vdisp-val">{status?.frameRate || 30}<span className="vdisp-unit"> fps</span></div>
              <div className="vdisp-label">Target fps</div>
            </div>
            <div className="vdisp-cell">
              <div className="vdisp-val" style={{ textTransform: 'capitalize' }}>{mode}</div>
              <div className="vdisp-label">Mode</div>
            </div>
          </div>
        </div>

        {/* Mode toggle — the only thing in the connected surface that
            actually changes behavior post-connect. Keep it compact. */}
        <div className="section">
          <div className="section-head"><div className="label">Display mode</div></div>
          <div className="seg" style={{ gridTemplateColumns: '1fr 1fr' }}>
            <button className={`seg-btn ${mode === 'extend' ? 'active' : ''}`} onClick={() => { setMode('extend'); onModeExtend() }}>
              <div className="seg-row"><span className="seg-icon">{Icons.layers(17)}</span><span>Extend</span></div>
              <div className="seg-sub">Use as a second display</div>
            </button>
            <button className={`seg-btn ${mode === 'mirror' ? 'active' : ''}`} onClick={() => { setMode('mirror'); onModeMirror() }}>
              <div className="seg-row"><span className="seg-icon">{Icons.display(17)}</span><span>Mirror</span></div>
              <div className="seg-sub">Duplicate this screen</div>
            </button>
          </div>
        </div>

        {/* Quick file send — Files pane lives behind the sidebar tab,
            but a one-shot Send button here lets the user push a file
            without context-switching. */}
        <div className="section">
          <div className="section-head"><div className="label">Files</div></div>
          <button className="btn btn-ghost btn-block" onClick={onSendFile}>
            {Icons.arrowR(18)} Send a file to {client?.name || 'phone'}
          </button>
        </div>

        <div className="section">
          <button
            className="btn btn-danger btn-block"
            onClick={() => {
              if (window.confirm(`Disconnect '${client?.name || 'device'}' and stop the server? All active transfers will be cancelled.`)) {
                onDisconnect()
              }
            }}
          >
            {Icons.close(19)} Disconnect &amp; stop server
          </button>
        </div>
      </div>

      {errorState && <DisconnectModal client={client} onStop={onStop} onRetry={onRetry} />}
    </div>
  )
}

function ConnectedHeader({ client, transport, connectedFor, errorState }: { client: ClientInfo | null; transport: string; connectedFor: string; errorState: boolean }): React.JSX.Element {
  return (
    <div className="dev-head dev-head-v2">
      <span className="dev-icon">{Icons.remote2(20)}</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="dev-name">{client?.name || 'Connected device'}</div>
        <div className="dev-meta">
          {client?.width}×{client?.height} · {transport} · connected {connectedFor}
        </div>
      </div>
      <span className="conn-chip">
        <span className={`dot ${errorState ? 'dot-warn dot-pulse' : 'dot-ok dot-pulse'}`} />
        {errorState ? 'Reconnecting' : 'Connected'}
      </span>
    </div>
  )
}

function DisconnectModal({ client, onStop, onRetry }: { client: ClientInfo | null; onStop: () => void; onRetry: () => void }): React.JSX.Element {
  return (
    <div className="error-backdrop">
      <div className="card error-modal">
        <span className="error-icon">{Icons.alert(26)}</span>
        <div className="modal-title">Connection lost</div>
        <div className="modal-body">Couldn't reach {client?.name || 'device'} after 5 attempts. The device may have left the network.</div>
        <div style={{ display: 'flex', gap: 10, marginTop: 22 }}>
          <button className="btn btn-ghost btn-block" onClick={onStop}>{Icons.power(19)} Stop Server</button>
          <button className="btn btn-primary btn-block" onClick={onRetry}>{Icons.refresh(19)} Retry</button>
        </div>
      </div>
    </div>
  )
}
