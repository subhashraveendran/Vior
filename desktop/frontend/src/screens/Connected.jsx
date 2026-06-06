// Connected screen — Display / Files / Mirror Preview tabs, plus the
// Connection lost modal that floats on top when the WS dies.
import React, { useState } from 'react'
import { Icons } from '../lib/icons'
import FilesPane from '../panes/Files'

export default function ConnectedScreen({ status, client, mode, setMode, onDisconnect, onSendFile, errorState, onRetry, onStop }) {
  const [t, setT] = useState('display')
  const tabs = [
    { id: 'display', label: 'Display', icon: Icons.display },
    { id: 'files', label: 'Files', icon: Icons.files },
    { id: 'mirror', label: 'Mirror Preview', icon: Icons.monitor2 },
  ]
  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <div className="dev-head">
        <span className="dev-icon">{Icons.remote2(20)}</span>
        <div style={{ flex: 1 }}>
          <div className="dev-name">{client?.name || 'Connected device'}</div>
          <div className="dev-meta">{client?.width}×{client?.height} · {client?.connectionType || 'wifi'}</div>
        </div>
        <span className="conn-chip">
          <span className={`dot ${errorState ? 'dot-warn dot-pulse' : 'dot-ok dot-pulse'}`} />
          {errorState ? 'Reconnecting' : 'Connected'}
        </span>
      </div>
      <div className="tabs">
        {tabs.map(tb => (
          <button key={tb.id} className={`tab ${t === tb.id ? 'active' : ''}`} onClick={() => setT(tb.id)}>
            {tb.icon(16)} {tb.label}
          </button>
        ))}
      </div>
      <div className="tab-body">
        {t === 'display' && (
          <>
            <div className="stat-grid">
              {[
                ['Resolution', `${client?.width || '—'}×${client?.height || '—'}`, ''],
                ['Frame rate', status?.frameRate || 30, ' fps'],
                ['Uptime', status?.uptime || 0, ' s'],
                ['Clients', status?.clientCount || 1, ''],
              ].map(([l, v, u]) => (
                <div key={l} className="card stat-card">
                  <div className="stat-val">{v}<span className="stat-unit">{u}</span></div>
                  <div className="stat-label">{l}</div>
                </div>
              ))}
            </div>
            <div className="section">
              <div className="section-head"><div className="label">Display mode</div></div>
              <div className="seg" style={{ gridTemplateColumns: '1fr 1fr' }}>
                <button className={`seg-btn ${mode === 'extend' ? 'active' : ''}`} onClick={() => setMode('extend')}>
                  <div className="seg-row"><span className="seg-icon">{Icons.layers(17)}</span><span>Extend</span></div>
                  <div className="seg-sub">Use as a second display</div>
                </button>
                <button className={`seg-btn ${mode === 'mirror' ? 'active' : ''}`} onClick={() => setMode('mirror')}>
                  <div className="seg-row"><span className="seg-icon">{Icons.display(17)}</span><span>Mirror</span></div>
                  <div className="seg-sub">Duplicate this screen</div>
                </button>
              </div>
            </div>
            <button className="btn btn-primary" onClick={onDisconnect} style={{ marginTop: 8, background: '#d04848', borderColor: '#d04848' }}>{Icons.close(19)} Disconnect device</button>
          </>
        )}
        {t === 'files' && <FilesPane onSendFile={onSendFile} client={client} />}
        {t === 'mirror' && (
          <div className="mirror-frame">
            <div className="mirror-tag"><span className="dot dot-ok dot-pulse" /> Streaming · {client?.width}×{client?.height} · {status?.frameRate || 30} fps</div>
          </div>
        )}
      </div>
      {errorState && (
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
      )}
    </div>
  )
}
