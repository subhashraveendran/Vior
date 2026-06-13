// Top-level router. Owns cross-screen state (server status, client,
// config, toasts, accent) and decides which screen to render based on
// the app's lifecycle state machine:
//
//   init → ready (idle) → waiting (server up, no client)
//                            ↓
//                        connected (full ops surface)
//                            ↓ disconnect
//                        waiting
//
// Sidebar items are gated by state — pre-connect the user only sees
// Home + Settings (Files is meaningless without a paired device). Once
// connected, Files appears. This keeps the "dumb user" surface tiny
// before they've done anything.
import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import {
  EventsOn,
  StartServer, StopServer, GetServerStatus,
  GetConnectedClients, GetConfig, UpdateConfig,
  GetVersion, PickAndSendFile,
  AcceptIncomingFile, RejectIncomingFile,
  HasAccessibility,
  MirrorDisplay, ExtendDisplay,
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

// AppState mirrors the lifecycle state machine. Derived from
// serverStatus + client so there's no second source of truth.
type AppState = 'ready' | 'waiting' | 'connected'

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
  // accessibilityOk drives the Permissions card on the Connected screen.
  // null = unknown / not yet polled; true/false = current OS state.
  const [accessibilityOk, setAccessibilityOk] = useState<boolean | null>(null)
  // disconnectBanner shows the transient "Device disconnected" notice
  // for ~3 seconds after a client drops, then auto-clears back to the
  // bare Waiting screen.
  const [disconnectBanner, setDisconnectBanner] = useState<string | null>(null)
  const [starting, setStarting] = useState<boolean>(false)
  const [startError, setStartError] = useState<string | null>(null)

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
      setClient(info); setErrorState(false); setDisconnectBanner(null)
      toast('success', 'Device connected', info.name)
      // Once a device connects, surface the current accessibility state
      // so the Connected screen's Permissions card has a value to render.
      HasAccessibility(false).then(setAccessibilityOk).catch(() => setAccessibilityOk(null))
    })
    const off2 = EventsOn('client:disconnected', () => {
      const name = client?.name || 'Device'
      setClient(null)
      setDisconnectBanner(name)
      window.setTimeout(() => setDisconnectBanner(null), 3000)
      toast('info', 'Device disconnected', null)
    })
    const off3 = EventsOn('permission:accessibility-missing', () => {
      // Trigger the OS deep-link dialog and surface a toast pointing the
      // user at the right Settings pane. Without Accessibility the
      // mobile Remote tab silently does nothing.
      HasAccessibility(true).catch(() => {})
      setAccessibilityOk(false)
      toast('error', 'Remote needs Accessibility', 'System Settings → Privacy & Security → Accessibility → enable Vior.')
    })
    const off8 = EventsOn('permission:screen-recording-missing', () => {
      setShowPerms(true)
      toast('error', 'Screen Recording needed', 'macOS must grant permission or the phone stream will be black.')
    })
    // Incoming file from mobile (untrusted device path). Trusted devices
    // never raise this event — desktop/app.go skips emit when trusted.
    const off4 = EventsOn('file:offer', (o: { id: string; name: string; size: number; mimeType: string; preview?: string }) => {
      setIncomingOffer(o)
    })
    const off5 = EventsOn('file:auto-accepted', (id: string) => {
      toast('info', 'File accepted', `Saving to ~/Downloads/Vior · ${id.slice(0, 6)}…`)
    })
    const off6 = EventsOn('client:resized', (dims: { width: number; height: number }) => {
      if (client && dims) {
        setClient({ ...client, width: dims.width, height: dims.height })
      }
    })
    const off7 = EventsOn('server:ip-changed', () => {
      // Trigger a fresh poll so the Waiting screen refreshes URLs/QR.
      GetServerStatus().then(setServerStatus).catch(() => {})
    })
    return () => { off1 && off1(); off2 && off2(); off3 && off3(); off4 && off4(); off5 && off5(); off6 && off6(); off7 && off7(); off8 && off8() }
  }, [toast, client])

  // poll status
  useEffect(() => {
    if (!serverStatus?.running) return
    let failCount = 0
    const id = setInterval(async () => {
      try {
        const s = await GetServerStatus()
        setServerStatus(s)
        failCount = 0
        if (!s.running) { setClient(null) }
      } catch {
        failCount++
        if (failCount >= 3) {
          setServerStatus(prev => prev ? { ...prev, running: false } : null)
          setClient(null)
          toast('error', 'Server stopped', 'The server stopped responding. Check if Vior is still running.')
        }
      }
    }, 3000)
    return () => clearInterval(id)
  }, [serverStatus?.running])

  // Poll accessibility state while a client is connected so the card
  // disappears the moment the user flips the toggle in System Settings.
  useEffect(() => {
    if (!client) { setAccessibilityOk(null); return }
    const id = setInterval(() => {
      HasAccessibility(false).then(setAccessibilityOk).catch(() => {})
    }, 2500)
    return () => clearInterval(id)
  }, [client])

  const start = async () => {
    setStarting(true); setStartError(null)
    try {
      await StartServer(); const s = await GetServerStatus(); setServerStatus(s)
      setStarting(false)
    } catch (e) {
      setStarting(false)
      const msg = String(e)
      setStartError(msg)
      toast('error', 'Failed to start', msg)
    }
  }
  const stop = async () => {
    setStarting(false); setStartError(null)
    try { await StopServer() } catch {}
    setServerStatus(null); setClient(null); setErrorState(false); setDisconnectBanner(null); setNav('server')
  }
  const sendFile = async () => {
    try { await PickAndSendFile(); toast('success', 'File sent', null) }
    catch (e) { toast('error', 'Send failed', String(e)) }
  }
  const copyUrl = () => {
    if (!serverStatus?.url) return
    navigator.clipboard?.writeText(serverStatus.url).then(() => toast('success', 'Copied URL', serverStatus.url))
  }
  const copyPair = () => {
    if (!serverStatus?.pairCode) return
    navigator.clipboard?.writeText(serverStatus.pairCode).then(() => toast('success', 'Copied pair code', formatPairCode(serverStatus.pairCode)))
  }
  const updateConfig = async (c: AppConfig) => {
    setConfig(c)
    try { await UpdateConfig({ ...c, host: '0.0.0.0', transferDir: '.' } as AppConfig) } catch {}
  }

  const onModeExtend = async () => {
    try { await ExtendDisplay?.(1) } catch {}
  }
  const onModeMirror = async () => {
    try { await MirrorDisplay?.(0, 1) } catch {}
  }

  const running = !!serverStatus?.running
  const connected = !!client
  const state: AppState = connected ? 'connected' : running ? 'waiting' : 'ready'
  const sidebarState: [string, string] = connected ? ['dot-ok', 'Connected'] : running ? ['dot-ok', 'Waiting'] : ['dot-idle', 'Ready']

  // Sidebar gating: pre-connect (ready / waiting) the user sees only
  // Home + Settings. Files is hidden because it has no target — a
  // dead nav item before any device joins is the kind of clutter that
  // makes the dumb-user say "what am I supposed to click?".
  const navItems = useMemo(() => {
    const items: Array<{ id: Nav; label: string; icon: (n?: number) => React.JSX.Element }> = [
      { id: 'server', label: 'Home', icon: Icons.display },
    ]
    if (state === 'connected') {
      items.push({ id: 'files', label: 'Files', icon: Icons.files })
    }
    items.push({ id: 'settings', label: 'Settings', icon: Icons.settings })
    return items
  }, [state])

  // If a state transition hides the current nav item, route back to Home
  // so the user never lands on a blank pane.
  useEffect(() => {
    if (!navItems.some(n => n.id === nav)) setNav('server')
  }, [navItems, nav])

  let body: React.ReactNode
  if (nav === 'settings') {
    body = <SettingsScreen config={config} onChange={updateConfig} accent={accent} setAccent={setAccent} />
  } else if (state === 'ready') {
    body = <IdleScreen onStart={start} showUpdate={showUpdate} onUpdate={() => setShowUpdate(false)} onDismiss={() => setShowUpdate(false)} starting={starting} startError={startError} />
  } else if (state === 'waiting') {
    body = (
      <WaitingScreen
        status={serverStatus}
        onStop={stop}
        onCopy={copyUrl}
        onCopyPair={copyPair}
        disconnectBanner={disconnectBanner}
      />
    )
  } else {
    body = (
      <ConnectedScreen
        status={serverStatus}
        client={client}
        mode={mode}
        setMode={setMode}
        onModeExtend={onModeExtend}
        onModeMirror={onModeMirror}
        onDisconnect={stop}
        onSendFile={sendFile}
        errorState={errorState}
        onRetry={() => setErrorState(false)}
        onStop={stop}
        accessibilityOk={accessibilityOk}
        onFixAccessibility={() => HasAccessibility(true).catch(() => {})}
        showFilesTab={nav === 'files'}
      />
    )
  }

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
          {navItems.map(n => (
            <button key={n.id} className={`nav-item ${nav === n.id ? 'active' : ''}`} onClick={() => setNav(n.id)}>
              {n.icon(18)}
              <span style={{ flex: 1 }}>{n.label}</span>
              {n.id === 'server' && state === 'waiting' && <span className="badge" />}
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

// formatPairCode splits the 6-char hex pair code into ABC-123 for the
// toast confirmation. Display-only — the server still stores the raw
// 6-char form and the mobile parses both transparently.
function formatPairCode(code: string | undefined): string {
  if (!code) return ''
  if (code.length <= 3) return code
  return `${code.slice(0, 3)}-${code.slice(3)}`
}
