import React, { useState, useEffect, useCallback } from 'react'
import { AnimatePresence } from 'framer-motion'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  StartServer, StopServer, GetServerStatus,
  GetConnectedClients, GetConfig, UpdateConfig,
  GetVersion
} from '../../wailsjs/go/main/App'

import { BrandMark } from '../design/Primitives'
import { Toast } from '../design/Primitives'
import { Icons } from '../design/Icons'
import { T } from '../design/tokens'

import IdleView from './IdleView'
import WaitingView from './WaitingView'
import ConnectedView from './ConnectedView'
import Settings from './Settings'

const VIEWS = { IDLE: 'idle', WAITING: 'waiting', CONNECTED: 'connected' }

export default function App() {
  const [view, setView] = useState(VIEWS.IDLE)
  const [serverStatus, setServerStatus] = useState(null)
  const [clientInfo, setClientInfo] = useState(null)
  const [config, setConfig] = useState({ port: 0, quality: 80, frameRate: 30 })
  const [version, setVersion] = useState('')
  const [showSettings, setShowSettings] = useState(false)
  const [toast, setToast] = useState(null)

  const notify = useCallback((msg, type = 'info') => {
    setToast({ msg, type, id: Date.now() })
  }, [])

  useEffect(() => {
    (async () => {
      try { setVersion(await GetVersion()) } catch {}
      try {
        const c = await GetConfig()
        setConfig({ port: c.port, quality: c.quality, frameRate: c.frameRate })
      } catch {}
      try {
        const s = await GetServerStatus()
        if (s.running) {
          setServerStatus(s)
          const clients = await GetConnectedClients()
          if (clients?.length > 0) {
            setClientInfo(clients[0])
            setView(VIEWS.CONNECTED)
          } else {
            setView(VIEWS.WAITING)
          }
        }
      } catch {}
    })()
  }, [])

  useEffect(() => {
    EventsOn('client:connected', (info) => {
      setClientInfo(info)
      setView(VIEWS.CONNECTED)
      notify(`${info.name} connected`, 'success')
    })
    EventsOn('client:disconnected', () => {
      setClientInfo(null)
      setView(VIEWS.WAITING)
      notify('Device disconnected')
    })
  }, [notify])

  useEffect(() => {
    if (view === VIEWS.IDLE) return
    const id = setInterval(async () => {
      try {
        const s = await GetServerStatus()
        setServerStatus(s)
        if (!s.running) setView(VIEWS.IDLE)
      } catch {}
    }, 3000)
    return () => clearInterval(id)
  }, [view])

  const handleStart = async () => {
    try {
      await StartServer()
      const s = await GetServerStatus()
      setServerStatus(s)
      setView(VIEWS.WAITING)
    } catch (e) {
      notify('Failed to start: ' + e, 'error')
    }
  }

  const handleStop = async () => {
    try { await StopServer() } catch {}
    setServerStatus(null)
    setClientInfo(null)
    setView(VIEWS.IDLE)
  }

  const handleConfigChange = async (newConfig) => {
    setConfig(newConfig)
    try {
      await UpdateConfig({
        port: newConfig.port || 0,
        quality: newConfig.quality,
        frameRate: newConfig.frameRate,
        host: '0.0.0.0', transferDir: '.'
      })
    } catch {}
  }

  return (
    <div className="app-shell">
      {/* macOS titlebar */}
      <div className="titlebar">
        <div style={{ width: 60 }}/>
        <div className="titlebar-center">Vior</div>
        <div style={{ width: 60 }}/>
      </div>

      {/* Content */}
      <div className="content">
        {/* Top bar (brand + gear) — shown on idle + waiting */}
        {view !== VIEWS.CONNECTED && (
          <div className="top-bar">
            <div className="top-bar-left">
              <BrandMark size={26}/>
              <div className="top-bar-brand">
                <span className="top-bar-name">Vior</span>
                <span className="top-bar-version">{version}</span>
              </div>
            </div>
            <button className="icon-btn" onClick={() => setShowSettings(true)}>
              {Icons.gear(18)}
            </button>
          </div>
        )}

        {/* Dot grid background on idle + waiting */}
        {view !== VIEWS.CONNECTED && (
          <div className="dot-grid" style={{ position: 'absolute', inset: 0, opacity: 0.7 }}/>
        )}

        <AnimatePresence mode="wait">
          {view === VIEWS.IDLE && (
            <IdleView key="idle" onStart={handleStart}/>
          )}
          {view === VIEWS.WAITING && (
            <WaitingView key="waiting" status={serverStatus} onStop={handleStop} onNotify={notify}/>
          )}
          {view === VIEWS.CONNECTED && (
            <ConnectedView
              key="connected"
              status={serverStatus}
              client={clientInfo}
              onDisconnect={handleStop}
              onNotify={notify}
              onSettings={() => setShowSettings(true)}
            />
          )}
        </AnimatePresence>

        {/* Footer tagline on idle */}
        {view === VIEWS.IDLE && (
          <div className="idle-footer">
            <span className="idle-tagline">Peer to peer · LAN only · zero telemetry</span>
          </div>
        )}
      </div>

      {/* Settings */}
      <AnimatePresence>
        {showSettings && (
          <Settings
            config={config}
            onChange={handleConfigChange}
            onClose={() => setShowSettings(false)}
          />
        )}
      </AnimatePresence>

      {/* Toast */}
      <AnimatePresence>
        {toast && (
          <Toast key={toast.id} kind={toast.type} onDone={() => setToast(null)}>
            {toast.msg}
          </Toast>
        )}
      </AnimatePresence>
    </div>
  )
}
