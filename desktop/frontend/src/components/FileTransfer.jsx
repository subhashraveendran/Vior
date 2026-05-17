import React, { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  SendFile, AcceptIncomingFile, RejectIncomingFile, GetActiveTransfers,
  PickAndSendFile
} from '../../wailsjs/go/main/App'

const FILE_ICONS = {
  image: '🖼',
  video: '🎬',
  audio: '🎵',
  pdf: '📄',
  default: '📁'
}

function getFileIcon(mimeType) {
  if (!mimeType) return FILE_ICONS.default
  if (mimeType.startsWith('image/')) return FILE_ICONS.image
  if (mimeType.startsWith('video/')) return FILE_ICONS.video
  if (mimeType.startsWith('audio/')) return FILE_ICONS.audio
  if (mimeType === 'application/pdf') return FILE_ICONS.pdf
  return FILE_ICONS.default
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
}

export default function FileTransfer({ onNotify }) {
  const [transfers, setTransfers] = useState([])
  const [offers, setOffers] = useState([]) // pending incoming offers
  const [dragOver, setDragOver] = useState(false)
  const dropRef = useRef(null)

  // Listen for file events.
  useEffect(() => {
    EventsOn('file:received', (data) => {
      onNotify?.(`Received: ${data.name}`, 'success')
      refreshTransfers()
    })

    EventsOn('file:offer', (data) => {
      setOffers(prev => [...prev, data])
    })

    refreshTransfers()
  }, [onNotify])

  const refreshTransfers = async () => {
    try {
      const t = await GetActiveTransfers()
      if (t) setTransfers(t)
    } catch {}
  }

  // Drag and drop from Wails (uses OnFileDrop).
  useEffect(() => {
    // Wails runtime file drop.
    try {
      const { OnFileDrop } = require('../../wailsjs/runtime/runtime')
      OnFileDrop(dropRef.current, async (x, y, paths) => {
        setDragOver(false)
        for (const path of paths) {
          try {
            await SendFile(path)
            onNotify?.('Sending: ' + path.split('/').pop(), 'info')
          } catch (e) {
            onNotify?.('Send failed: ' + e, 'error')
          }
        }
        refreshTransfers()
      }, true)
    } catch {}
  }, [onNotify])

  const handleAccept = async (id) => {
    try {
      await AcceptIncomingFile(id)
      setOffers(prev => prev.filter(o => o.id !== id))
      refreshTransfers()
    } catch (e) {
      onNotify?.('Accept failed: ' + e, 'error')
    }
  }

  const handlePickFile = async () => {
    try {
      await PickAndSendFile()
      onNotify?.('Sending file...', 'info')
      refreshTransfers()
    } catch (e) {
      if (e) onNotify?.('Send failed: ' + e, 'error')
    }
  }

  const handleReject = async (id) => {
    try {
      await RejectIncomingFile(id)
      setOffers(prev => prev.filter(o => o.id !== id))
    } catch {}
  }

  const hasContent = offers.length > 0 || transfers.length > 0

  return (
    <div
      ref={dropRef}
      className={`file-transfer ${dragOver ? 'drag-over' : ''}`}
      onDragOver={(e) => { e.preventDefault(); setDragOver(true) }}
      onDragLeave={() => setDragOver(false)}
      onDrop={() => setDragOver(false)}
    >
      {/* Drop zone indicator */}
      <AnimatePresence>
        {dragOver && (
          <motion.div
            className="drop-overlay"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
          >
            <div className="drop-label">Drop files to send</div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Pending offers from phone */}
      <AnimatePresence>
        {offers.map((offer) => (
          <motion.div
            key={offer.id}
            className="file-offer"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, height: 0 }}
          >
            <div className="file-row">
              {offer.preview ? (
                <img src={offer.preview} className="file-thumb" alt="" />
              ) : (
                <span className="file-icon">{getFileIcon(offer.mimeType)}</span>
              )}
              <div className="file-info">
                <span className="file-name">{offer.name}</span>
                <span className="file-size">{formatSize(offer.size)}</span>
              </div>
              <div className="file-actions">
                <button className="accept-btn" onClick={() => handleAccept(offer.id)}>Accept</button>
                <button className="reject-btn" onClick={() => handleReject(offer.id)}>✕</button>
              </div>
            </div>
          </motion.div>
        ))}
      </AnimatePresence>

      {/* Active transfers */}
      {transfers.filter(t => !t.complete).map((t) => (
        <div key={t.id} className="file-transfer-item">
          <div className="file-row">
            {t.preview ? (
              <img src={t.preview} className="file-thumb" alt="" />
            ) : (
              <span className="file-icon">{getFileIcon(t.mimeType)}</span>
            )}
            <div className="file-info">
              <span className="file-name">{t.name}</span>
              <span className="file-size">
                {formatSize(t.transferred)} / {formatSize(t.size)}
              </span>
            </div>
          </div>
          <div className="progress-bar">
            <div
              className="progress-fill"
              style={{ width: t.size > 0 ? `${(t.transferred / t.size) * 100}%` : '0%' }}
            />
          </div>
        </div>
      ))}

      {/* Share button + drop hint */}
      <div className="file-bottom">
        <button className="share-btn" onClick={handlePickFile} title="Send a file">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
            <path d="M18 16.08c-.76 0-1.44.3-1.96.77L8.91 12.7c.05-.23.09-.46.09-.7s-.04-.47-.09-.7l7.05-4.11c.54.5 1.25.81 2.04.81 1.66 0 3-1.34 3-3s-1.34-3-3-3-3 1.34-3 3c0 .24.04.47.09.7L8.04 9.81C7.5 9.31 6.79 9 6 9c-1.66 0-3 1.34-3 3s1.34 3 3 3c.79 0 1.5-.31 2.04-.81l7.12 4.16c-.05.21-.08.43-.08.65 0 1.61 1.31 2.92 2.92 2.92 1.61 0 2.92-1.31 2.92-2.92s-1.31-2.92-2.92-2.92z"/>
          </svg>
          <span>Send File</span>
        </button>
        {!hasContent && (
          <span className="drop-hint-text">or drop files here</span>
        )}
      </div>
    </div>
  )
}
