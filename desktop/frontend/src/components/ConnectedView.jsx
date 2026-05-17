import React, { useState, useMemo } from 'react'
import { motion } from 'framer-motion'
import { LiveDot, Segment, Stat, GhostButton, PrimaryButton, Card } from '../design/Primitives'
import { Icons } from '../design/Icons'
import { T } from '../design/tokens'
import FileTransfer from './FileTransfer'

const fade = {
  initial: { opacity: 0 },
  animate: { opacity: 1, transition: { duration: 0.3 } },
  exit: { opacity: 0, transition: { duration: 0.15 } }
}

function formatUptime(s) {
  if (!s) return '00:00:00'
  const h = Math.floor(s / 3600).toString().padStart(2, '0')
  const m = Math.floor((s % 3600) / 60).toString().padStart(2, '0')
  const sec = (s % 60).toString().padStart(2, '0')
  return `${h}:${m}:${sec}`
}

export default function ConnectedView({ status, client, onDisconnect, onNotify, onSettings }) {
  const [tab, setTab] = useState('display')

  const streamUrl = useMemo(() => {
    const base = status?.url || 'http://localhost:8080'
    return base + '/stream'
  }, [status])

  return (
    <motion.div variants={fade} initial="initial" animate="animate" exit="exit"
      style={{ position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column' }}>

      {/* Device bar */}
      <div className="device-bar">
        <div className="device-info">
          {Icons.phone(16, { color: T.text })}
          <div>
            <div className="device-name">
              {client?.name || 'Device'} <LiveDot/>
            </div>
            <div className="device-res">
              {client?.width || 0} × {client?.height || 0}
            </div>
          </div>
        </div>
        <div className="device-bar-center">
          <Segment
            active={tab}
            onChange={setTab}
            items={[
              { id: 'display', label: 'Display', icon: Icons.monitor(14) },
              { id: 'files', label: 'Files', icon: Icons.file(14) },
            ]}
          />
        </div>
        <div className="device-bar-right">
          <button className="disconnect-link" onClick={onDisconnect}>Disconnect</button>
          <button className="icon-btn" onClick={onSettings}>{Icons.gear(16)}</button>
        </div>
      </div>

      {/* Tab content */}
      <div className="connected-content">
        {tab === 'display' && (
          <div className="display-layout">
            <div className="preview-area">
              <div className="preview-frame">
                <img src={streamUrl} alt="Stream" onError={e => e.target.style.opacity = '0.3'}/>
              </div>
            </div>
            <div className="stats-row">
              <Stat label="Stream URL" value={status?.url || ''} mono icon={Icons.refresh(12)}/>
              <Stat label="Uptime" value={formatUptime(status?.uptime)} mono/>
              <Stat label="FPS" value={
                <span>
                  <span style={{ fontSize: 20, fontWeight: 600, color: T.heading }}>30</span>
                  <span style={{ color: T.textDim, fontSize: 11, marginLeft: 4 }}>/ 30</span>
                </span>
              }/>
              <div style={{ display: 'flex', alignItems: 'center' }}>
                <GhostButton danger onClick={onDisconnect}>Disconnect</GhostButton>
              </div>
            </div>
          </div>
        )}

        {tab === 'files' && (
          <div className="files-layout">
            <FileTransfer onNotify={onNotify}/>
          </div>
        )}
      </div>
    </motion.div>
  )
}
