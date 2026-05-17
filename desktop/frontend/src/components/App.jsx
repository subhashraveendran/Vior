import React, { useState, useEffect, useCallback } from 'react'
import { AnimatePresence } from 'framer-motion'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  StartServer, StopServer, GetServerStatus,
  GetConnectedClients, GetConfig, UpdateConfig,
  GetVersion
} from '../../wailsjs/go/main/App'

import IdleView from './IdleView'
import WaitingView from './WaitingView'
import ConnectedView from './ConnectedView'
import Settings from './Settings'
import Toast from './Toast'

const VIEWS = { IDLE: 'idle', WAITING: 'waiting', CONNECTED: 'connected' }

export default function App() {
  const [view, setView] = useState(VIEWS.IDLE)
  const [serverStatus, setServerStatus] = useState(null)
  const [clientInfo, setClientInfo] = useState(null)
  const [config, setConfig] = useState({ port: 8080, quality: 80, frameRate: 30 })
  const [version, setVersion] = useState('')
  const [showSettings, setShowSettings] = useState(false)
  const [toast, setToast] = useState(null)

  // Toast helper.
  const notify = useCallback((msg, type = 'info') => {
    setToast({ msg, type, id: Date.now() })
  }, [])

  // Init: load config, version, check if already running.
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

  // Wails events.
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

  // Poll server status while running.
  useEffect(() => {
    if (view === VIEWS.IDLE) return
    const id = setInterval(async () => {
      try {
        const s = await GetServerStatus()
        setServerStatus(s)
        if (!s.running) {
          setView(VIEWS.IDLE)
        }
      } catch {}
    }, 3000)
    return () => clearInterval(id)
  }, [view])

  // Handlers.
  const handleStart = async () => {
    try {
      await StartServer()
      const s = await GetServerStatus()
      setServerStatus(s)
      setView(VIEWS.WAITING)
      notify('Server started', 'success')
    } catch (e) {
      notify('Failed to start: ' + e, 'error')
    }
  }

  const handleStop = async () => {
    try {
      await StopServer()
    } catch {}
    setServerStatus(null)
    setClientInfo(null)
    setView(VIEWS.IDLE)
    notify('Server stopped')
  }

  const handleConfigChange = async (newConfig) => {
    setConfig(newConfig)
    try {
      await UpdateConfig({
        port: newConfig.port,
        quality: newConfig.quality,
        frameRate: newConfig.frameRate,
        host: '0.0.0.0',
        transferDir: '.'
      })
    } catch {}
  }

  return (
    <div className="container">
      <header className="header">
        <h1>Vior</h1>
        <span className="version">{version}</span>
        <button
          className="icon-btn"
          onClick={() => setShowSettings(!showSettings)}
          title="Settings"
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
            <path d="M19.14 12.94c.04-.31.06-.63.06-.94 0-.31-.02-.63-.06-.94l2.03-1.58a.49.49 0 00.12-.61l-1.92-3.32a.49.49 0 00-.59-.22l-2.39.96a7.04 7.04 0 00-1.62-.94l-.36-2.54a.48.48 0 00-.48-.41h-3.84a.48.48 0 00-.48.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96a.49.49 0 00-.59.22L2.74 8.87a.48.48 0 00.12.61l2.03 1.58c-.04.31-.06.63-.06.94 0 .31.02.63.06.94l-2.03 1.58a.49.49 0 00-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.36 1.03.7 1.62.94l.36 2.54c.05.24.26.41.48.41h3.84c.24 0 .44-.17.48-.41l.36-2.54c.59-.24 1.13-.57 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.03-1.58zM12 15.6A3.6 3.6 0 1112 8.4a3.6 3.6 0 010 7.2z"/>
          </svg>
        </button>
      </header>

      <div className="view-container">
        <AnimatePresence mode="wait">
          {view === VIEWS.IDLE && (
            <IdleView key="idle" onStart={handleStart} />
          )}
          {view === VIEWS.WAITING && (
            <WaitingView
              key="waiting"
              status={serverStatus}
              onStop={handleStop}
              onNotify={notify}
            />
          )}
          {view === VIEWS.CONNECTED && (
            <ConnectedView
              key="connected"
              status={serverStatus}
              client={clientInfo}
              onDisconnect={handleStop}
              onNotify={notify}
            />
          )}
        </AnimatePresence>
      </div>

      <AnimatePresence>
        {showSettings && (
          <Settings
            config={config}
            onChange={handleConfigChange}
            onClose={() => setShowSettings(false)}
          />
        )}
      </AnimatePresence>

      <AnimatePresence>
        {toast && (
          <Toast
            key={toast.id}
            message={toast.msg}
            type={toast.type}
            onDone={() => setToast(null)}
          />
        )}
      </AnimatePresence>
    </div>
  )
}
