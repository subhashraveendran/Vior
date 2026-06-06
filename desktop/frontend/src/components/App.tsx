// Top-level router. Owns the cross-screen state (server status,
// client, config, toasts, accent) and decides which screen to render.
// Every screen lives in ../screens/ or ../panes/; this file is
// intentionally minimal — wiring + state, no UI markup beyond the
// chrome (titlebar + sidebar + toast host).
import React, { useState, useEffect, useCallback, useRef } from 'react'
import {
  EventsOn,
  StartServer, StopServer, GetServerStatus,
  GetConnectedClients, GetConfig, UpdateConfig,
  GetVersion, PickAndSendFile,
  AcceptIncomingFile, RejectIncomingFile,
  HasAccessibility,
} from '../lib/api'

import { Icons }       from '../lib/icons'
import Glyph           from '../lib/Glyph'
import ToastHost       from '../lib/Toast'
import { applyAccent } from '../lib/accent'

import IdleScreen        from '../screens/Idle'
import WaitingScreen     from '../screens/Waiting'
import ConnectedScreen   from '../screens/Connected'
import SettingsScreen    from '../screens/Settings'
import PermissionsModal  from '../screens/Permissions'

import type {
  AccentHex,
  AppConfig,
  ClientInfo,
  Mode,
  Nav,
  ServerStatus,
  Toast,
  ToastTone,
} from '../types'

export default function App() {
  const [serverStatus, setServerStatus] = useState<ServerStatus | null>(null)
  const [client, setClient] = useState<ClientInfo | null>(null)
  const [config, setConfig] = useState<AppConfig>({ port: 0, quality: 80, frameRate: 30, host: '0.0.0.0', transferDir: '.' } as AppConfig)
  const [nav, setNav] = useState<Nav>('server')
  const [mode, setMode] = useState<Mode>('extend')
  const [errorState, setErrorState] = useState<boolean>(false)
  const [showUpdate, setShowUpdate] = useState<boolean>(false)
  const [showPerms, setShowPerms] = useState<boolean>(false)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [accent, setAccent] = useState<AccentHex>(localStorage.getItem('vior_accent') || '#ff8a4c')
  const [incomingOffer, setIncomingOffer] = useState<{ id: string; name: string; size: number; mimeType: string; preview?: string } | null>(null)

  // eslint-disable-next-line react-hooks/exhaustive-deps -- accent applies
  // once at boot; subsequent changes flow through Appearance → applyAccent
  // directly, so re-running this effect on every accent change would double-write.
  useEffect(() => { applyAccent(accent) }, [])
  const idRef = useRef<number>(100)

  const toast = useCallback((tone: ToastTone, title: string, msg: string | null) => {
    const id = ++idRef.current
    setToasts(ts => [...ts, { id, tone, title, msg }])
    setTimeout(() => setToasts(ts => ts.filter(t => t.id !== id)), 3500)
  }, [])

  // bootstrap
  useEffect(() => {
    (async () => {
      // GetVersion currently unused in the UI but called so the bound
      // method is exercised on launch (catches a broken Wails binding
      // before the user clicks anything).
      try { await GetVersion() } catch {}
      try { const c = await GetConfig(); setConfig({ ...c, port: c.port, quality: c.quality, frameRate: c.frameRate }) } catch {}
      try {
        const s = await GetServerStatus()
        if (s.running) {
          setServerStatus(s)
          const cs = await GetConnectedClients()
          if (cs?.length > 0) setClient(cs[0]!)
        }
      } catch {}
    })()
  }, [])

  // events
  useEffect(() => {
    const off1 = EventsOn('client:connected', (info: ClientInfo) => {
      setClient(info); setErrorState(false)
      toast('success', 'Device connected', info.name)
    })
    const off2 = EventsOn('client:disconnected', () => {
      setClient(null)
      toast('info', 'Device disconnected', null)
    })
    const off3 = EventsOn('permission:accessibility-missing', () => {
      // Trigger the OS deep-link dialog and surface a toast pointing the
      // user at the right Settings pane. Without Accessibility the
      // mobile Remote tab silently does nothing.
      HasAccessibility(true).catch(() => {})
      toast('error', 'Remote needs Accessibility', 'System Settings → Privacy & Security → Accessibility → enable Vior.')
    })
    // Incoming file from mobile (untrusted device path). Trusted devices
    // never raise this event — desktop/app.go skips emit when trusted.
    const off4 = EventsOn('file:offer', (o: { id: string; name: string; size: number; mimeType: string; preview?: string }) => {
      setIncomingOffer(o)
    })
    const off5 = EventsOn('file:auto-accepted', (id: string) => {
      toast('info', 'File accepted', `Saving to ~/Downloads/Vior · ${id.slice(0, 6)}…`)
    })
    return () => { off1 && off1(); off2 && off2(); off3 && off3(); off4 && off4(); off5 && off5() }
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
  const updateConfig = async (c: AppConfig) => {
    setConfig(c)
    try { await UpdateConfig({ ...c, host: '0.0.0.0', transferDir: '.' } as AppConfig) } catch {}
  }

  const running = !!serverStatus?.running
  const connected = !!client
  const sidebarState: [string, string] = connected ? ['dot-ok', 'Connected'] : running ? ['dot-ok', 'Running'] : ['dot-idle', 'Stopped']

  let body
  if (nav === 'settings') body = <SettingsScreen config={config} onChange={updateConfig} accent={accent} setAccent={setAccent} />
  else if (!running)     body = <IdleScreen onStart={start} showUpdate={showUpdate} onUpdate={() => setShowUpdate(false)} onDismiss={() => setShowUpdate(false)} />
  else if (!connected)   body = <WaitingScreen status={serverStatus} onStop={stop} onCopy={copyUrl} />
  else                   body = <ConnectedScreen status={serverStatus} client={client} mode={mode} setMode={setMode} onDisconnect={stop} onSendFile={sendFile} errorState={errorState} onRetry={() => setErrorState(false)} onStop={stop} />

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
            { id: 'server' as const, label: 'Server', icon: Icons.display },
            { id: 'files' as const, label: 'Files', icon: Icons.files },
            { id: 'settings' as const, label: 'Settings', icon: Icons.settings },
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
      {incomingOffer && (
        <div className="error-backdrop">
          <div className="card error-modal">
            {/* Preview: real thumbnail for images, big extension badge otherwise. */}
            {incomingOffer.preview ? (
              <img
                src={incomingOffer.preview.startsWith('data:') ? incomingOffer.preview : `data:${incomingOffer.mimeType || 'image/jpeg'};base64,${incomingOffer.preview}`}
                alt=""
                style={{
                  width: 96, height: 96, objectFit: 'cover',
                  borderRadius: 12, marginBottom: 14, alignSelf: 'center',
                  border: '1px solid var(--border)',
                }}
              />
            ) : (
              <div style={{
                width: 96, height: 96, borderRadius: 12, marginBottom: 14,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                background: 'var(--accent-weak)', color: 'var(--accent)',
                font: '700 22px/1 var(--font-mono)', letterSpacing: '0.04em',
                border: '1px solid var(--accent-line)', alignSelf: 'center',
                textTransform: 'uppercase',
              }}>
                {(incomingOffer.name.split('.').pop() || 'FILE').slice(0, 4)}
              </div>
            )}
            <div className="modal-title">Incoming file</div>
            <div className="modal-body">
              <b>{incomingOffer.name}</b>
              <br />
              {fmtSize(incomingOffer.size)} · {incomingOffer.mimeType || 'unknown type'}
              <br />
              <span style={{ color: 'var(--text-3)' }}>from {client?.name || 'connected device'}</span>
            </div>
            <div style={{ display: 'flex', gap: 10, marginTop: 22 }}>
              <button
                className="btn btn-ghost btn-block"
                onClick={async () => {
                  const id = incomingOffer.id
                  setIncomingOffer(null)
                  try { await RejectIncomingFile(id) } catch {}
                  toast('info', 'Declined', incomingOffer.name)
                }}
              >
                {Icons.close(19)} Decline
              </button>
              <button
                className="btn btn-primary btn-block"
                onClick={async () => {
                  const id = incomingOffer.id
                  const name = incomingOffer.name
                  setIncomingOffer(null)
                  try { await AcceptIncomingFile(id); toast('success', 'Accepting', name) }
                  catch (e) { toast('error', 'Accept failed', String(e)) }
                }}
              >
                {Icons.check(19)} Accept
              </button>
            </div>
          </div>
        </div>
      )}
      <ToastHost toasts={toasts} onClose={(id: number) => setToasts(ts => ts.filter(t => t.id !== id))} />
    </div>
  )
}

function fmtSize(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}
