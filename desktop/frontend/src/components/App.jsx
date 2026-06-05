import React, { useState, useEffect, useCallback, useRef } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  StartServer, StopServer, GetServerStatus,
  GetConnectedClients, GetConfig, UpdateConfig,
  GetVersion, CheckPermissions, PickAndSendFile,
  GetMenuBarVisible, SetMenuBarVisible,
} from '../../wailsjs/go/main/App'

// ── Icons (24x24, 1.7 stroke) ──────────────────────────────────
const I = (p) => (size = 18) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
    stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"
    dangerouslySetInnerHTML={{ __html: p }} />
)
const Icons = {
  display: I('<rect x="2.5" y="4" width="19" height="13" rx="2"/><path d="M8 21h8M12 17v4"/>'),
  files:   I('<path d="M3 7.5a2 2 0 0 1 2-2h4l2 2.2h6a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>'),
  remote:  I('<path d="M5.5 3.2l13.6 7.2a.7.7 0 0 1-.1 1.27l-5.3 1.9-2.4 5.1a.7.7 0 0 1-1.3-.06z"/>'),
  settings:I('<circle cx="12" cy="12" r="3.2"/><path d="M12 3v2.2M12 18.8V21M21 12h-2.2M5.2 12H3M18.4 5.6l-1.6 1.6M7.2 16.8l-1.6 1.6M18.4 18.4l-1.6-1.6M7.2 7.2L5.6 5.6"/>'),
  power:   I('<path d="M12 3v8"/><path d="M6.5 7a8 8 0 1 0 11 0"/>'),
  link:    I('<path d="M9.5 14.5l5-5"/><path d="M8 11l-2 2a3.2 3.2 0 0 0 4.5 4.5l2-2"/><path d="M16 13l2-2A3.2 3.2 0 0 0 13.5 6.5l-2 2"/>'),
  copy:    I('<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>'),
  usb:     I('<circle cx="12" cy="20" r="1.4" fill="currentColor" stroke="none"/><path d="M12 18.6V4M12 4l-2.4 2.6M12 4l2.4 2.6"/><path d="M7 11l2 1.4v2M17 9l-2 1.4v4"/>'),
  refresh: I('<path d="M20 11a8 8 0 1 0-1.5 5.5"/><path d="M20 5v5h-5"/>'),
  close:   I('<path d="M6 6l12 12M18 6L6 18"/>'),
  check:   I('<path d="M4.5 12.5l4.5 4.5L19.5 6.5"/>'),
  arrowR:  I('<path d="M5 12h14M13 6l6 6-6 6"/>'),
  layers:  I('<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/>'),
  download:I('<path d="M12 4v11M7 11l5 5 5-5"/><path d="M5 20h14"/>'),
  monitor2:I('<rect x="2.5" y="4.5" width="19" height="12.5" rx="1.6"/><path d="M2.5 13.5h19"/><path d="M9 21h6"/>'),
  remote2: I('<rect x="2.5" y="4" width="19" height="13" rx="2"/><path d="M8 21h8M12 17v4"/>'),
  shield:  I('<path d="M12 3l7 2.5v5c0 4.5-3 8.2-7 9.5-4-1.3-7-5-7-9.5v-5z"/><path d="M9 12l2 2 4-4"/>'),
  alert:   I('<path d="M12 3.5L21.5 20H2.5z"/><path d="M12 10v4.5M12 17.5h.01"/>'),
  file:    I('<path d="M6 3h7l5 5v13H6z"/><path d="M13 3v5h5"/>'),
  photo:   I('<rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="8.5" cy="10" r="1.6"/><path d="M5 17l4.5-4 3 2.6L16 12l3 3.2"/>'),
}

// ── Glyph (brand) ──
function Glyph({ size = 24 }) {
  const u = size / 24
  return (
    <span className="glyph" style={{ width: size, height: size }}>
      <span className="g-screen" style={{ left: 0, top: 2 * u, width: 18 * u, height: 13 * u, borderWidth: Math.max(1.5, 2 * u), borderRadius: 3.5 * u }} />
      <span className="g-stand" style={{ left: 6 * u, top: 16 * u, width: 6 * u, height: Math.max(1.5, 2 * u), borderRadius: 1 * u }} />
      <span className="g-phone" style={{ right: 0, bottom: 0, width: 9.5 * u, height: 14.5 * u, borderRadius: 3 * u }} />
    </span>
  )
}

// ── Deterministic decorative QR ──
const qrCache = {}
function qrMatrix(seed = 'vior', n = 25) {
  if (qrCache[seed + n]) return qrCache[seed + n]
  let h = 2166136261
  for (let i = 0; i < seed.length; i++) { h ^= seed.charCodeAt(i); h = Math.imul(h, 16777619) }
  const rnd = () => { h ^= h << 13; h ^= h >>> 17; h ^= h << 5; return ((h >>> 0) % 1000) / 1000 }
  const m = Array.from({ length: n }, () => Array(n).fill(false))
  const finder = (r, c) => {
    for (let i = -1; i <= 7; i++) for (let j = -1; j <= 7; j++) {
      const rr = r + i, cc = c + j; if (rr < 0 || cc < 0 || rr >= n || cc >= n) continue
      const edge = i === 0 || i === 6 || j === 0 || j === 6
      const core = i >= 2 && i <= 4 && j >= 2 && j <= 4
      m[rr][cc] = (i >= 0 && i <= 6 && j >= 0 && j <= 6) ? (edge || core) : false
    }
  }
  for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) m[r][c] = rnd() > 0.5
  finder(0, 0); finder(0, n - 7); finder(n - 7, 0)
  qrCache[seed + n] = m
  return m
}
function QR({ size = 196, seed = 'vior' }) {
  const n = 25, m = qrMatrix(seed, n)
  const inner = size - 28, cell = inner / n
  return (
    <div className="qr-box" style={{ width: size, height: size }}>
      <div style={{ display: 'grid', gridTemplateColumns: `repeat(${n}, ${cell}px)`, gridTemplateRows: `repeat(${n}, ${cell}px)` }}>
        {m.flatMap((row, r) => row.map((on, c) => (
          <div key={r + '-' + c} style={{ background: on ? '#0b0d10' : 'transparent' }} />
        )))}
      </div>
    </div>
  )
}

// ── Toast host ──
function ToastHost({ toasts, onClose }) {
  return (
    <div className="toast-host">
      {toasts.map(t => (
        <div key={t.id} className="toast">
          <span className={`dot ${t.tone === 'success' ? 'dot-ok' : t.tone === 'warning' ? 'dot-warn' : t.tone === 'error' ? 'dot-err' : 'dot-idle'}`} style={{ marginTop: 5 }} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="toast-title">{t.title}</div>
            {t.msg && <div className="toast-msg">{t.msg}</div>}
          </div>
          <button onClick={() => onClose(t.id)} style={{ background: 'none', border: 'none', color: 'var(--text-3)', cursor: 'pointer', padding: 2 }}>{Icons.close(15)}</button>
        </div>
      ))}
    </div>
  )
}

// ── Idle screen ──
function IdleScreen({ onStart, showUpdate, onUpdate, onDismiss }) {
  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      {showUpdate && (
        <div className="update-banner">
          <span style={{ color: 'var(--accent)' }}>{Icons.download(19)}</span>
          <div style={{ flex: 1 }}>
            <span className="update-banner-title">Vior 2.1 is available.</span>
            <span className="update-banner-body">H.264 streaming and faster discovery.</span>
          </div>
          <button className="btn btn-primary btn-sm" onClick={onUpdate}>Update</button>
          <button onClick={onDismiss} style={{ background: 'none', border: 'none', color: 'var(--text-3)', cursor: 'pointer', padding: 4 }}>{Icons.close(15)}</button>
        </div>
      )}
      <div className="idle">
        <div className="radar-wrap">
          <div className="radar-circle" style={{ width: 210, height: 210, opacity: 0.7 }} />
          <div className="radar-circle" style={{ width: 158, height: 158, opacity: 0.54 }} />
          <div className="radar-circle" style={{ width: 110, height: 110, opacity: 0.38 }} />
          <span className="radar-ping" />
          <span className="radar-ping" style={{ animationDelay: '1.4s' }} />
          <div className="radar-core"><Glyph size={44} /></div>
        </div>
        <div className="state-pill">
          <span className="dot dot-idle" />
          <span className="label">Not broadcasting</span>
        </div>
        <div className="idle-title">Ready when you are</div>
        <div className="idle-sub">Start the server and Vior on your phone discovers this Mac automatically — over your local network, no setup.</div>
        <button className="btn btn-primary idle-cta-btn" onClick={onStart}>
          {Icons.power(20)} Start Server
        </button>
        <button className="idle-fallback" onClick={onStart}>{Icons.link(15)} Open in a browser instead</button>
      </div>
    </div>
  )
}

// ── Waiting screen ──
function WaitingScreen({ status, onStop, onCopy }) {
  const url = status?.url || ''
  const seed = status?.url || 'vior'
  return (
    <div className="waiting">
      <div className="waiting-left">
        <div className="waiting-tag">
          <span className="dot dot-ok dot-pulse" />
          <span className="waiting-tag-text">Server running · waiting for a device</span>
        </div>
        <div className="waiting-title">Connect your phone</div>
        <div className="waiting-body">Open Vior on your phone — it should appear automatically. Or use the address below.</div>

        <div className="card" style={{ marginTop: 24, padding: 18 }}>
          <div className="label" style={{ marginBottom: 12 }}>Connect manually</div>
          {(status?.urls && status.urls.length > 1) ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {status.urls.map(u => (
                <div key={u} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <span className="mono" style={{ fontSize: 15, fontWeight: 500 }}>{u}</span>
                  <button className="btn btn-ghost btn-sm" onClick={() => navigator.clipboard?.writeText(u)}>{Icons.copy(15)}Copy</button>
                </div>
              ))}
              <div style={{ fontSize: 11.5, color: 'var(--text-3)' }}>Multiple interfaces detected — try each on your phone.</div>
            </div>
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
              <span className="mono" style={{ fontSize: 17, fontWeight: 500 }}>{url || `:${status?.port}`}</span>
              <button className="btn btn-ghost btn-sm" onClick={onCopy}>{Icons.copy(15)}Copy</button>
            </div>
          )}
          {status?.pairCode && (
            <div style={{ marginTop: 14, paddingTop: 14, borderTop: '1px solid var(--border)' }}>
              <div className="label" style={{ marginBottom: 6 }}>Pair code</div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <span className="mono" style={{ fontSize: 22, fontWeight: 600, letterSpacing: '0.18em', color: 'var(--accent)' }}>{status.pairCode}</span>
                <button className="btn btn-ghost btn-sm" onClick={() => navigator.clipboard?.writeText(status.pairCode)} title="Copy pair code">{Icons.copy(14)}Copy</button>
              </div>
              <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 6 }}>Enter this on your phone to authorize the connection.</div>
            </div>
          )}
          <div style={{ height: 1, background: 'var(--border)', margin: '14px 0' }} />
          <div style={{ display: 'flex', gap: 18 }}>
            <a href={url} style={{ display: 'flex', alignItems: 'center', gap: 7, color: 'var(--text-2)', fontSize: 13, fontWeight: 600, textDecoration: 'none' }}>{Icons.link(16)} Open in browser</a>
          </div>
        </div>
        <div style={{ flex: 1 }} />
        <button className="btn btn-ghost" style={{ alignSelf: 'flex-start', marginTop: 18 }} onClick={onStop}>
          {Icons.power(19)} Stop Server
        </button>
      </div>
      <div className="waiting-right">
        <QR size={196} seed={seed} />
        <div style={{ textAlign: 'center' }}>
          <div style={{ fontSize: 14, fontWeight: 600 }}>Scan to connect</div>
          <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 4 }}>Point your phone camera here</div>
        </div>
      </div>
    </div>
  )
}

// ── Connected screen ──
function ConnectedScreen({ status, client, mode, setMode, onDisconnect, onSendFile, errorState, onRetry, onStop }) {
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

// ── Files pane ──
function FilesPane({ onSendFile, client }) {
  const [over, setOver] = useState(false)
  const [files, setFiles] = useState([])
  useEffect(() => {
    const off = EventsOn('file:received', (f) => {
      setFiles(fs => [{ id: f.id, name: f.name, size: f.size, kind: 'in', done: true }, ...fs])
    })
    return () => off && off()
  }, [])
  return (
    <div>
      <div className={`drop ${over ? 'over' : ''}`}
        onDragOver={(e) => { e.preventDefault(); setOver(true) }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => { e.preventDefault(); setOver(false); onSendFile() }}
      >
        <span className="drop-icon">{Icons.download(22)}</span>
        <div className="drop-title">Drop files to send to {client?.name || 'device'}</div>
        <div className="drop-sub">or <b onClick={onSendFile}>browse</b> — up to 2 GB per file</div>
      </div>
      <div className="label" style={{ marginBottom: 12 }}>Recent</div>
      {files.length === 0
        ? <div style={{ padding: '24px 0', textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>No transfers yet.</div>
        : <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {files.map(f => (
              <div key={f.id} className="card file-row">
                <span className="file-icon">{Icons.file(16)}</span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="file-name">{f.name}</div>
                  <div className="file-meta">{f.size} bytes · {f.kind === 'in' ? 'received' : 'sent'}</div>
                </div>
                <span style={{ color: 'var(--ok)' }}>{Icons.check(17)}</span>
              </div>
            ))}
          </div>}
    </div>
  )
}

// ── accent presets ──
const ACCENTS = [
  { hex: '#ff8a4c', on: '#1a0e06', weak: 'rgba(255,138,76,0.14)', line: 'rgba(255,138,76,0.40)', name: 'Orange' },
  { hex: '#4cc2ff', on: '#06121a', weak: 'rgba(76,194,255,0.14)', line: 'rgba(76,194,255,0.40)', name: 'Blue' },
  { hex: '#46d39a', on: '#06140e', weak: 'rgba(70,211,154,0.14)', line: 'rgba(70,211,154,0.40)', name: 'Green' },
  { hex: '#e8e8ea', on: '#0b0d10', weak: 'rgba(232,232,234,0.14)', line: 'rgba(232,232,234,0.40)', name: 'White' },
]
function applyAccent(hex) {
  const p = ACCENTS.find(a => a.hex === hex) || ACCENTS[0]
  const r = document.documentElement.style
  r.setProperty('--accent', p.hex)
  r.setProperty('--accent-2', p.hex)
  r.setProperty('--on-accent', p.on)
  r.setProperty('--accent-weak', p.weak)
  r.setProperty('--accent-line', p.line)
  localStorage.setItem('vior_accent', p.hex)
}

// ── Theme/Appearance popup ──
function AppearancePanel({ accent, setAccent, onClose }) {
  const [style, setStyle] = useState(localStorage.getItem('vior_style') || 'precise')
  const [density, setDensity] = useState(localStorage.getItem('vior_density') || 'regular')
  const [motion, setMotion] = useState(localStorage.getItem('vior_motion') || 'expressive')
  useEffect(() => {
    document.documentElement.setAttribute('data-vior-style', style); localStorage.setItem('vior_style', style)
  }, [style])
  useEffect(() => {
    document.documentElement.setAttribute('data-vior-density', density); localStorage.setItem('vior_density', density)
  }, [density])
  useEffect(() => {
    document.documentElement.setAttribute('data-vior-motion', motion); localStorage.setItem('vior_motion', motion)
  }, [motion])
  const Seg = ({ value, onChange, opts }) => (
    <div className="seg" style={{ gridTemplateColumns: `repeat(${opts.length},1fr)` }}>
      {opts.map(o => (
        <button key={o} className={`seg-btn ${value === o ? 'active' : ''}`} onClick={() => onChange(o)}>
          <div className="seg-row"><span style={{ textTransform: 'capitalize' }}>{o}</span></div>
        </button>
      ))}
    </div>
  )
  return (
    <div className="scroll settings-wrap">
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 20 }}>
        <button onClick={onClose} className="btn btn-ghost btn-sm" style={{ width: 38, padding: 0, minHeight: 38 }}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="M15 18l-6-6 6-6"/></svg>
        </button>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 17, fontWeight: 600 }}>Appearance</div>
          <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 1 }}>Theme, accent, density &amp; motion</div>
        </div>
      </div>

      <div className="label" style={{ marginBottom: 12 }}>Preview</div>
      <div className="inset" style={{ padding: 14, display: 'flex', alignItems: 'center', gap: 12 }}>
        <Glyph size={26} />
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 6 }}>
          <span style={{ height: 7, width: '72%', borderRadius: 'var(--r-pill)', background: 'var(--text-3)' }} />
          <span style={{ height: 7, width: '46%', borderRadius: 'var(--r-pill)', background: 'var(--surface-3)' }} />
        </div>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, padding: '8px 13px', borderRadius: 'var(--r)', background: 'var(--accent)', color: 'var(--on-accent)', fontSize: 12.5, fontWeight: 600 }}>
          <span className="dot dot-pulse" style={{ background: 'var(--on-accent)' }} /> Connect
        </span>
      </div>

      <div className="label" style={{ marginTop: 20, marginBottom: 12 }}>Style</div>
      <Seg value={style} onChange={setStyle} opts={['precise', 'instrument', 'soft']} />

      <div className="label" style={{ marginTop: 18, marginBottom: 12 }}>Accent</div>
      <div style={{ display: 'flex', gap: 10 }}>
        {ACCENTS.map(a => (
          <button key={a.hex} title={a.name}
            onClick={() => { applyAccent(a.hex); setAccent(a.hex) }}
            style={{
              flex: 1, height: 48, borderRadius: 'var(--r-sm)', cursor: 'pointer',
              background: a.hex,
              border: accent === a.hex ? '2px solid var(--text-1)' : '1px solid var(--border)',
              display: 'grid', placeItems: 'center', color: a.on,
            }}>
            {accent === a.hex && Icons.check(18)}
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 18, marginBottom: 12 }}>Density</div>
      <Seg value={density} onChange={setDensity} opts={['compact', 'regular', 'comfy']} />

      <div className="label" style={{ marginTop: 18, marginBottom: 12 }}>Motion</div>
      <Seg value={motion} onChange={setMotion} opts={['expressive', 'subtle', 'off']} />

      <button className="btn btn-primary btn-block" style={{ marginTop: 26 }} onClick={onClose}>Done</button>
    </div>
  )
}

const accentName = (hex) => (ACCENTS.find(a => a.hex.toLowerCase() === (hex || '').toLowerCase())?.name) || 'Custom'

// ── Settings ──
function SettingsScreen({ config, onChange, accent, setAccent }) {
  const presets = [
    { id: 'performance', label: 'Performance', sub: 'Lower latency', q: 60, f: 60 },
    { id: 'balanced', label: 'Balanced', sub: 'Recommended', q: 80, f: 30 },
    { id: 'quality', label: 'Quality', sub: 'Sharper image', q: 92, f: 30 },
  ]
  const resolutions = [
    { id: 'auto', label: 'Auto', sub: 'Match the display' },
    { id: '2560', label: '2560 × 1600', sub: 'Retina' },
    { id: '1920', label: '1920 × 1200', sub: '' },
    { id: '1280', label: '1280 × 800', sub: 'Bandwidth saver' },
  ]
  const cur = presets.find(p => p.q === config.quality && p.f === config.frameRate)?.id || 'balanced'
  const [res, setRes] = useState('auto')
  const [usb, setUsb] = useState(true)
  const [adb, setAdb] = useState(true)
  const [menuBar, setMenuBar] = useState(true)
  useEffect(() => { GetMenuBarVisible?.().then(setMenuBar).catch(() => {}) }, [])
  const toggleMenuBar = (v) => { setMenuBar(v); SetMenuBarVisible?.(v) }
  const [appearance, setAppearance] = useState(false)
  const style = localStorage.getItem('vior_style') || 'precise'
  const density = localStorage.getItem('vior_density') || 'regular'
  const motion = localStorage.getItem('vior_motion') || 'expressive'

  if (appearance) return <AppearancePanel accent={accent} setAccent={setAccent} onClose={() => setAppearance(false)} />

  return (
    <div className="scroll settings-wrap">
      <div className="label" style={{ marginBottom: 12 }}>Stream quality</div>
      <div className="seg" style={{ gridTemplateColumns: 'repeat(3, 1fr)' }}>
        {presets.map(p => (
          <button key={p.id} className={`seg-btn ${cur === p.id ? 'active' : ''}`}
            onClick={() => onChange({ ...config, quality: p.q, frameRate: p.f })}>
            <div className="seg-row"><span>{p.label}</span></div>
            <div className="seg-sub" style={{ marginLeft: 0 }}>{p.sub}</div>
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Resolution</div>
      <div className="card" style={{ overflow: 'hidden' }}>
        {resolutions.map((r, i) => (
          <button key={r.id} onClick={() => setRes(r.id)} style={{
            display: 'flex', alignItems: 'center', gap: 12, width: '100%', textAlign: 'left',
            padding: '13px 15px', background: res === r.id ? 'var(--accent-weak)' : 'transparent',
            border: 'none', borderTop: i ? '1px solid var(--border)' : 'none', cursor: 'pointer',
          }}>
            <span style={{ flex: 1, minWidth: 0 }}>
              <span style={{ display: 'block', fontSize: 14, fontWeight: 600, color: 'var(--text-1)' }}>{r.label}</span>
              {r.sub && <span className="mono" style={{ display: 'block', fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>{r.sub}</span>}
            </span>
            {res === r.id
              ? <span style={{ color: 'var(--accent)' }}>{Icons.check(18)}</span>
              : <span style={{ width: 18, height: 18, borderRadius: '50%', border: '1.5px solid var(--border-strong)', flex: 'none' }} />}
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Frame rate</div>
      <div className="seg" style={{ gridTemplateColumns: '1fr 1fr' }}>
        {['30', '60'].map(v => (
          <button key={v} className={`seg-btn ${String(config.frameRate) === v ? 'active' : ''}`}
            onClick={() => onChange({ ...config, frameRate: parseInt(v) })}>
            <div className="seg-row"><span>{v} fps</span></div>
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Connectivity</div>
      <div className="card">
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Allow USB connections</div>
            <div className="settings-row-sub">Connect over cable when Wi-Fi drops</div>
          </div>
          <button className={`toggle ${usb ? 'toggle-on' : 'toggle-off'}`} onClick={() => setUsb(!usb)}>
            <span className="toggle-knob" style={{ transform: `translateX(${usb ? 17 : 0}px)` }} />
          </button>
        </div>
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Show in macOS menu bar</div>
            <div className="settings-row-sub">Quick status + Start/Stop/Quit in the top-right tray</div>
          </div>
          <button className={`toggle ${menuBar ? 'toggle-on' : 'toggle-off'}`} onClick={() => toggleMenuBar(!menuBar)}>
            <span className="toggle-knob" style={{ transform: `translateX(${menuBar ? 17 : 0}px)` }} />
          </button>
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Displays</div>
      <div className="card">
        <div className="settings-row">
          <span style={{ color: 'var(--text-3)' }}>{Icons.monitor2(18)}</span>
          <div className="settings-row-body">
            <div className="settings-row-title">Built-in Retina Display</div>
            <div className="settings-row-sub">2560 × 1600</div>
          </div>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--accent)', padding: '4px 9px', borderRadius: 'var(--r-pill)', background: 'var(--accent-weak)' }}>Primary</span>
        </div>
        <div className="settings-row">
          <span style={{ color: 'var(--text-3)' }}>{Icons.monitor2(18)}</span>
          <div className="settings-row-body">
            <div className="settings-row-title">DELL U2720Q</div>
            <div className="settings-row-sub">3840 × 2160</div>
          </div>
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>USB / ADB</div>
      <div className="card">
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">{adb ? '1 device attached' : 'No bridge installed'}</div>
            <div className="settings-row-sub">{adb ? 'Pixel 8 Pro · usb-2' : 'platform-tools not found'}</div>
          </div>
          {adb
            ? <button className="btn btn-ghost btn-sm" onClick={() => setAdb(true)}>{Icons.refresh(17)} Restart</button>
            : <button className="btn btn-primary btn-sm" onClick={() => setAdb(true)}>{Icons.download(17)} Install</button>}
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>About</div>
      <div className="card">
        <div className="settings-row">
          <Glyph size={28} />
          <div className="settings-row-body">
            <div className="settings-row-title">Vior</div>
            <div className="settings-row-sub">Phase 2 · open source</div>
          </div>
          <a href="https://github.com/subhashraveendran/Vior" target="_blank" rel="noreferrer" className="btn btn-ghost btn-sm" style={{ textDecoration: 'none' }}>Check for updates</a>
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Appearance</div>
      <button onClick={() => setAppearance(true)} className="card" style={{ width: '100%', textAlign: 'left', cursor: 'pointer' }}>
        <div className="settings-row">
          <span style={{ width: 34, height: 34, flex: 'none', borderRadius: 'var(--r-sm)', background: 'var(--accent)', display: 'grid', placeItems: 'center', color: 'var(--on-accent)' }}>{Icons.display(17)}</span>
          <div className="settings-row-body">
            <div className="settings-row-title">Theme &amp; motion</div>
            <div className="settings-row-sub" style={{ textTransform: 'capitalize' }}>{style} · {accentName(accent)} · {density} · {motion}</div>
          </div>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" style={{ color: 'var(--text-3)' }}><path d="M9 5l7 7-7 7"/></svg>
        </div>
      </button>
    </div>
  )
}

// ── Permissions modal (macOS first-run) ──
function PermissionsModal({ onDone }) {
  const [granted, setGranted] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const verify = async () => {
    setVerifying(true)
    try { await CheckPermissions(); setGranted(true) } catch { /* still denied */ }
    setVerifying(false)
  }
  return (
    <div className="modal-backdrop">
      <div className="card modal">
        <span className="modal-icon" style={{ color: granted ? 'var(--ok)' : 'var(--accent)' }}>
          {granted ? Icons.check(28) : Icons.shield(28)}
        </span>
        <div className="modal-title">{granted ? 'Permission granted' : 'Screen Recording access'}</div>
        <div className="modal-body">
          {granted ? 'Vior can now mirror your display. You’re all set.'
            : <>macOS requires permission for Vior to capture your screen. Enable it under <span style={{ color: 'var(--text-1)', fontWeight: 600 }}>Privacy &amp; Security → Screen Recording</span>.</>}
        </div>
        <div style={{ display: 'flex', gap: 10, marginTop: 22 }}>
          {granted
            ? <button className="btn btn-primary btn-block" onClick={onDone}>{Icons.arrowR(19)} Continue</button>
            : <>
                <button className="btn btn-ghost btn-block">{Icons.settings(19)} Open Settings</button>
                <button className="btn btn-primary btn-block" onClick={verify}>{verifying ? <span className="spin" style={{ width: 17, height: 17, border: '2.4px solid var(--surface-3)', borderTopColor: 'var(--on-accent)', borderRadius: '50%', display: 'inline-block' }} /> : 'Verify'}</button>
              </>}
        </div>
      </div>
    </div>
  )
}

// ── Main App ──
export default function App() {
  const [serverStatus, setServerStatus] = useState(null)
  const [client, setClient] = useState(null)
  const [config, setConfig] = useState({ port: 0, quality: 80, frameRate: 30 })
  const [version, setVersion] = useState('')
  const [nav, setNav] = useState('server')
  const [mode, setMode] = useState('extend')
  const [errorState, setErrorState] = useState(false)
  const [showUpdate, setShowUpdate] = useState(false)
  const [showPerms, setShowPerms] = useState(false)
  const [toasts, setToasts] = useState([])
  const [accent, setAccent] = useState(localStorage.getItem('vior_accent') || '#ff8a4c')

  useEffect(() => { applyAccent(accent) }, [])
  const idRef = useRef(100)

  const toast = useCallback((tone, title, msg) => {
    const id = ++idRef.current
    setToasts(ts => [...ts, { id, tone, title, msg }])
    setTimeout(() => setToasts(ts => ts.filter(t => t.id !== id)), 3500)
  }, [])

  // bootstrap
  useEffect(() => {
    (async () => {
      try { setVersion(await GetVersion()) } catch {}
      try { const c = await GetConfig(); setConfig({ port: c.port, quality: c.quality, frameRate: c.frameRate }) } catch {}
      try {
        const s = await GetServerStatus()
        if (s.running) {
          setServerStatus(s)
          const cs = await GetConnectedClients()
          if (cs?.length > 0) setClient(cs[0])
        }
      } catch {}
    })()
  }, [])

  // events
  useEffect(() => {
    const off1 = EventsOn('client:connected', (info) => {
      setClient(info); setErrorState(false)
      toast('success', 'Device connected', info.name)
    })
    const off2 = EventsOn('client:disconnected', () => {
      setClient(null)
      toast('info', 'Device disconnected', null)
    })
    return () => { off1 && off1(); off2 && off2() }
  }, [toast])

  // poll status
  useEffect(() => {
    if (!serverStatus?.running) return
    const id = setInterval(async () => {
      try { const s = await GetServerStatus(); setServerStatus(s); if (!s.running) { setClient(null) } } catch {}
    }, 3000)
    return () => clearInterval(id)
  }, [serverStatus?.running])

  const start = async () => {
    try { await StartServer(); const s = await GetServerStatus(); setServerStatus(s) }
    catch (e) { toast('error', 'Failed to start', String(e)) }
  }
  const stop = async () => {
    try { await StopServer() } catch {}
    setServerStatus(null); setClient(null); setErrorState(false)
  }
  const sendFile = async () => {
    try { await PickAndSendFile(); toast('success', 'File sent', null) }
    catch (e) { toast('error', 'Send failed', String(e)) }
  }
  const copyUrl = () => {
    if (!serverStatus?.url) return
    navigator.clipboard?.writeText(serverStatus.url).then(() => toast('success', 'Copied', serverStatus.url))
  }
  const updateConfig = async (c) => {
    setConfig(c)
    try { await UpdateConfig({ ...c, host: '0.0.0.0', transferDir: '.' }) } catch {}
  }

  const running = !!serverStatus?.running
  const connected = !!client
  const sidebarState = connected ? ['dot-ok', 'Connected'] : running ? ['dot-ok', 'Running'] : ['dot-idle', 'Stopped']

  let body
  if (nav === 'settings') body = <SettingsScreen config={config} onChange={updateConfig} accent={accent} setAccent={setAccent} />
  else if (!running) body = <IdleScreen onStart={start} showUpdate={showUpdate} onUpdate={() => setShowUpdate(false)} onDismiss={() => setShowUpdate(false)} />
  else if (!connected) body = <WaitingScreen status={serverStatus} onStop={stop} onCopy={copyUrl} />
  else body = <ConnectedScreen status={serverStatus} client={client} mode={mode} setMode={setMode} onDisconnect={stop} onSendFile={sendFile} errorState={errorState} onRetry={() => setErrorState(false)} onStop={stop} />

  return (
    <div className="dwin">
      <div className="titlebar">
        <div style={{ width: 60 }} />
        <div className="titlebar-center"><Glyph size={15} /><span>Vior</span></div>
        <div style={{ flex: 'none' }} className="titlebar-state">
          <span className={`dot ${sidebarState[0]} ${running ? 'dot-pulse' : ''}`} />
          {sidebarState[1]}
        </div>
      </div>
      <div className="dbody">
        <div className="sidebar">
          {[
            { id: 'server', label: 'Server', icon: Icons.display },
            { id: 'files', label: 'Files', icon: Icons.files },
            { id: 'settings', label: 'Settings', icon: Icons.settings },
          ].map(n => (
            <button key={n.id} className={`nav-item ${nav === n.id ? 'active' : ''}`} onClick={() => setNav(n.id)}>
              {n.icon(18)}
              <span style={{ flex: 1 }}>{n.label}</span>
              {n.id === 'server' && running && !connected && <span className="badge" />}
            </button>
          ))}
          <div className="sidebar-foot">
            <div className="sidebar-foot-label">Server</div>
            <div className="sidebar-foot-state"><span className={`dot ${sidebarState[0]} ${running ? 'dot-pulse' : ''}`} />{sidebarState[1]}</div>
          </div>
        </div>
        <div className="main">{body}</div>
      </div>
      {showPerms && <PermissionsModal onDone={() => setShowPerms(false)} />}
      <ToastHost toasts={toasts} onClose={(id) => setToasts(ts => ts.filter(t => t.id !== id))} />
    </div>
  )
}
