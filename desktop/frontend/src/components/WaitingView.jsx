import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'

const pageVariants = {
  initial: { opacity: 0, y: 20 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.35, ease: [0.25, 0.1, 0.25, 1] } },
  exit: { opacity: 0, y: -10, transition: { duration: 0.2 } }
}

export default function WaitingView({ status, onStop, onNotify }) {
  const [showWeb, setShowWeb] = useState(false)
  const url = status?.url || ''

  const handleCopy = async () => {
    try {
      await ClipboardSetText(url)
      onNotify('URL copied', 'success')
    } catch {
      try {
        await navigator.clipboard.writeText(url)
        onNotify('URL copied', 'success')
      } catch {}
    }
  }

  return (
    <motion.div className="view" variants={pageVariants} initial="initial" animate="animate" exit="exit">
      <div className="wait-layout">
        {/* Main content */}
        <div className="wait-hero">
          <motion.div
            className="wait-anim"
            animate={{ scale: [1, 1.05, 1], opacity: [0.6, 1, 0.6] }}
            transition={{ repeat: Infinity, duration: 3, ease: 'easeInOut' }}
          >
            <svg viewBox="0 0 80 80" width="80" height="80">
              <circle cx="40" cy="40" r="36" fill="none" stroke="#4f46e5" strokeWidth="2" opacity="0.2" />
              <circle cx="40" cy="40" r="24" fill="none" stroke="#4f46e5" strokeWidth="2" opacity="0.4" />
              <circle cx="40" cy="40" r="12" fill="none" stroke="#4f46e5" strokeWidth="2" opacity="0.6" />
              <circle cx="40" cy="40" r="4" fill="#6366f1" />
            </svg>
          </motion.div>

          <motion.h2
            className="wait-title"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.15 }}
          >
            Ready to connect
          </motion.h2>

          <motion.p
            className="wait-sub"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.25 }}
          >
            Open <strong>Vior</strong> on your phone or tablet to connect automatically
          </motion.p>
        </div>

        {/* Web browser option */}
        <motion.div
          className="wait-options"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.4 }}
        >
          <button
            className={`web-toggle ${showWeb ? 'open' : ''}`}
            onClick={() => setShowWeb(!showWeb)}
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
              <path d="M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zm6.93 6h-2.95a15.65 15.65 0 00-1.38-3.56A8.03 8.03 0 0118.92 8zM12 4.04c.83 1.2 1.48 2.53 1.91 3.96h-3.82c.43-1.43 1.08-2.76 1.91-3.96zM4.26 14C4.1 13.36 4 12.69 4 12s.1-1.36.26-2h3.38c-.08.66-.14 1.32-.14 2 0 .68.06 1.34.14 2H4.26zm.82 2h2.95c.32 1.25.78 2.45 1.38 3.56A7.987 7.987 0 015.08 16zm2.95-8H5.08a7.987 7.987 0 014.33-3.56A15.65 15.65 0 008.03 8zM12 19.96c-.83-1.2-1.48-2.53-1.91-3.96h3.82c-.43 1.43-1.08 2.76-1.91 3.96zM14.34 14H9.66c-.09-.66-.16-1.32-.16-2 0-.68.07-1.35.16-2h4.68c.09.65.16 1.32.16 2 0 .68-.07 1.34-.16 2zm.25 5.56c.6-1.11 1.06-2.31 1.38-3.56h2.95a8.03 8.03 0 01-4.33 3.56zM16.36 14c.08-.66.14-1.32.14-2 0-.68-.06-1.34-.14-2h3.38c.16.64.26 1.31.26 2s-.1 1.36-.26 2h-3.38z"/>
            </svg>
            <span>Use web browser instead</span>
            <svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" className={`chevron ${showWeb ? 'up' : ''}`}>
              <path d="M7.41 8.59L12 13.17l4.59-4.58L18 10l-6 6-6-6z"/>
            </svg>
          </button>

          <AnimatePresence>
            {showWeb && (
              <motion.div
                className="web-panel"
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: 'auto', opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.25 }}
              >
                <div className="web-panel-inner">
                  <p className="web-note">Stream only — no touch control in browser</p>

                  {status?.qrCodeDataUrl && (
                    <div className="qr-wrap">
                      <img src={status.qrCodeDataUrl} alt="QR Code" className="qr-img" />
                    </div>
                  )}

                  <div className="url-row">
                    <span className="url-text">{url}</span>
                    <button className="copy-btn" onClick={handleCopy} title="Copy">
                      <svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
                        <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
                      </svg>
                    </button>
                  </div>
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </motion.div>

        {/* Stop button */}
        <motion.button
          className="stop-link"
          onClick={onStop}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.5 }}
        >
          Stop server
        </motion.button>
      </div>
    </motion.div>
  )
}
