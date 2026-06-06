// Top-level router. Owns the cross-screen state (server status,
// client, config, toasts, accent) and decides which screen to render.
// Every screen lives in ../screens/ or ../panes/; this file is
// intentionally minimal — wiring + state, no UI markup beyond the
// chrome (titlebar + sidebar + toast host).
import React, { useState, useEffect, useCallback, useRef } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  StartServer, StopServer, GetServerStatus,
  GetConnectedClients, GetConfig, UpdateConfig,
  GetVersion, PickAndSendFile,
  HasAccessibility,
} from '../../wailsjs/go/main/App'

import { Icons }       from '../lib/icons'
import Glyph           from '../lib/Glyph'
import ToastHost       from '../lib/Toast'
import { applyAccent } from '../lib/accent'

import IdleScreen        from '../screens/Idle'
import WaitingScreen     from '../screens/Waiting'
import ConnectedScreen   from '../screens/Connected'
import SettingsScreen    from '../screens/Settings'
import PermissionsModal  from '../screens/Permissions'

export default function App() {
  const [serverStatus, setServerStatus] = useState(null)
  const [client, setClient] = useState(null)
  const [config, setConfig] = useState({ port: 0, quality: 80, frameRate: 30 })
  const [, setVersion] = useState('')
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
    const off3 = EventsOn('permission:accessibility-missing', () => {
      // Trigger the OS deep-link dialog and surface a toast pointing the
      // user at the right Settings pane. Without Accessibility the
      // mobile Remote tab silently does nothing.
      HasAccessibility(true).catch(() => {})
      toast('error', 'Remote needs Accessibility', 'System Settings → Privacy & Security → Accessibility → enable Vior.')
    })
    return () => { off1 && off1(); off2 && off2(); off3 && off3() }
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
