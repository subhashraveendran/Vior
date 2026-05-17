import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { PrimaryButton } from '../design/Primitives'
import { Icons } from '../design/Icons'
import { T } from '../design/tokens'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'

const fade = {
  initial: { opacity: 0, y: 20 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.35 } },
  exit: { opacity: 0, y: -10, transition: { duration: 0.2 } }
}

function PulseScan() {
  return (
    <div className="pulse-scan">
      {[0, 0.7, 1.4].map((d, i) => (
        <span key={i} className="pulse-ring" style={{ animationDelay: `${d}s` }}/>
      ))}
      <div className="pulse-core">
        <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 12.55a11 11 0 0 1 14.08 0M1.42 9a16 16 0 0 1 21.16 0M8.53 16.11a6 6 0 0 1 6.95 0"/>
          <circle cx="12" cy="20" r="1" fill="#fff"/>
        </svg>
      </div>
    </div>
  )
}

function BrowserFallback({ expanded, onToggle, status, onNotify }) {
  const url = status?.url || ''

  const handleCopy = async () => {
    try {
      await ClipboardSetText(url)
      onNotify?.('URL copied to clipboard', 'success')
    } catch {
      try { await navigator.clipboard.writeText(url) } catch {}
    }
  }

  return (
    <div className="browser-fallback">
      <div className="browser-header" onClick={onToggle}>
        {Icons.qr(16, { color: T.textDim })}
        <span>Use a web browser instead</span>
        <span style={{ color: T.textDim, transition: 'transform 0.2s', transform: expanded ? 'rotate(180deg)' : 'none' }}>
          {Icons.chevD(14)}
        </span>
      </div>
      <AnimatePresence>
        {expanded && (
          <motion.div
            className="browser-body"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            style={{ overflow: 'hidden' }}
          >
            {status?.qrCodeDataUrl && (
              <div className="qr-box">
                <img src={status.qrCodeDataUrl} alt="QR" style={{ width: '100%', height: '100%', imageRendering: 'pixelated' }}/>
              </div>
            )}
            <div>
              <div style={{ fontSize: 12, color: T.textDim, marginBottom: 4 }}>Open this on any device:</div>
              <div className="mono" style={{ fontSize: 14, color: T.indigo2, fontWeight: 500, cursor: 'pointer' }} onClick={handleCopy}>
                {url || 'loading…'}
              </div>
              <div style={{ fontSize: 11, color: T.textDim, marginTop: 10 }}>
                Works on iOS, Android, and any modern browser. Same Wi-Fi required.
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

export default function WaitingView({ status, onStop, onNotify }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <motion.div variants={fade} initial="initial" animate="animate" exit="exit" style={{ position: 'absolute', inset: 0 }}>
      <div className="waiting-center">
        <PulseScan/>
        <div style={{ textAlign: 'center' }}>
          <h1 className="waiting-title">Ready to connect</h1>
          <p className="waiting-sub">
            Open <strong>Vior</strong> on your phone and pick<br/>
            <span className="waiting-hostname">this computer</span> from the list.
          </p>
        </div>
        <BrowserFallback expanded={expanded} onToggle={() => setExpanded(!expanded)} status={status} onNotify={onNotify}/>
      </div>

      <button className="stop-btn" onClick={onStop}>
        <span className="stop-dot"/>
        Stop server
      </button>
    </motion.div>
  )
}
