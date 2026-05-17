import React, { useMemo, useState } from 'react'
import { motion } from 'framer-motion'
import FileTransfer from './FileTransfer'

const pageVariants = {
  initial: { opacity: 0, y: 20 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.3, ease: [0.25, 0.1, 0.25, 1] } },
  exit: { opacity: 0, y: -10, transition: { duration: 0.2 } }
}

function formatUptime(seconds) {
  if (!seconds) return '0s'
  if (seconds < 60) return seconds + 's'
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return m + 'm ' + s + 's'
}

export default function ConnectedView({ status, client, onDisconnect, onNotify }) {
  const [tab, setTab] = useState('display')

  const streamUrl = useMemo(() => {
    const base = status?.url || 'http://localhost:' + (status?.port || 8080)
    return base + '/stream'
  }, [status])

  return (
    <motion.div className="view connected-view" variants={pageVariants} initial="initial" animate="animate" exit="exit">
      <div className="connected-layout">
        {/* Device info bar */}
        <motion.div
          className="device-info"
          initial={{ opacity: 0, x: -20 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ delay: 0.1 }}
        >
          <div className="device-badge">
            <motion.span
              className="conn-dot"
              animate={{ scale: [1, 1.3, 1] }}
              transition={{ repeat: Infinity, duration: 2, ease: 'easeInOut' }}
            />
            <span>{client?.name || 'Device'}</span>
          </div>
          <span className="device-res">
            {client?.width || 0} × {client?.height || 0}
          </span>
        </motion.div>

        {/* Tab bar */}
        <div className="tab-bar">
          <button
            className={`tab-btn ${tab === 'display' ? 'active' : ''}`}
            onClick={() => setTab('display')}
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
              <path d="M21 3H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h7v2H8v2h8v-2h-2v-2h7c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm0 14H3V5h18v12z"/>
            </svg>
            Display
          </button>
          <button
            className={`tab-btn ${tab === 'files' ? 'active' : ''}`}
            onClick={() => setTab('files')}
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
              <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm-1 4H8c-1.1 0-1.99.9-1.99 2L6 21c0 1.1.89 2 1.99 2H19c1.1 0 2-.9 2-2V11l-6-6zM8 21V7h6v5h5v9H8z"/>
            </svg>
            Files
          </button>
        </div>

        {/* Tab content */}
        <div className="tab-content">
          {tab === 'display' && (
            <div className="display-tab">
              <div className="preview-container">
                <img
                  src={streamUrl}
                  alt="Stream Preview"
                  onError={(e) => { e.target.style.opacity = '0.3' }}
                />
              </div>
              <div className="stats">
                <span className="stat">{status?.url || ''}</span>
                <span className="stat">{formatUptime(status?.uptime)}</span>
              </div>
            </div>
          )}

          {tab === 'files' && (
            <div className="files-tab">
              <FileTransfer onNotify={onNotify} />
            </div>
          )}
        </div>

        {/* Disconnect */}
        <button className="danger-btn" onClick={onDisconnect}>
          Disconnect
        </button>
      </div>
    </motion.div>
  )
}
