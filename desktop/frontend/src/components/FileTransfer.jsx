import React, { useState, useEffect } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { AcceptIncomingFile, RejectIncomingFile, GetActiveTransfers, PickAndSendFile } from '../../wailsjs/go/main/App'
import { PrimaryButton, Card } from '../design/Primitives'
import { Icons } from '../design/Icons'
import { T } from '../design/tokens'

function formatSize(b) {
  if (b < 1024) return b + ' B'
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB'
  return (b / 1073741824).toFixed(1) + ' GB'
}

export default function FileTransfer({ onNotify }) {
  const [transfers, setTransfers] = useState([])
  const [offers, setOffers] = useState([])

  useEffect(() => {
    EventsOn('file:received', (d) => {
      onNotify?.(`${d.name} received`, 'success')
      refresh()
    })
    EventsOn('file:offer', (d) => setOffers(p => [...p, d]))
    refresh()
  }, [])

  const refresh = async () => {
    try { const t = await GetActiveTransfers(); if (t) setTransfers(t) } catch {}
  }

  const handleSend = async () => {
    try {
      await PickAndSendFile()
      onNotify?.('Sending file…', 'info')
      refresh()
    } catch (e) { if (e) onNotify?.('Send failed: ' + e, 'error') }
  }

  const handleAccept = async (id) => {
    try { await AcceptIncomingFile(id); setOffers(p => p.filter(o => o.id !== id)); refresh() } catch {}
  }
  const handleReject = async (id) => {
    try { await RejectIncomingFile(id); setOffers(p => p.filter(o => o.id !== id)) } catch {}
  }

  const active = transfers.filter(t => !t.complete)
  const complete = transfers.filter(t => t.complete)
  const incomingCount = offers.length
  const transferCount = active.length + complete.length

  return (
    <>
      {/* Drop zone */}
      <div className="drop-zone">
        <div className="drop-icon">{Icons.upload(20)}</div>
        <div style={{ flex: 1 }}>
          <div style={{ color: T.heading, fontSize: 14, fontWeight: 600 }}>Drop files to send</div>
          <div style={{ color: T.textDim, fontSize: 12, marginTop: 2 }}>
            Or use the button. Anything up to 4 GB, sent peer-to-peer.
          </div>
        </div>
        <PrimaryButton icon={Icons.upload(14)} onClick={handleSend}>Send file</PrimaryButton>
      </div>

      {/* Lists */}
      <div className="files-grid">
        {/* Incoming */}
        <Card pad={0} style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <div className="list-header">
            <span className="list-title">Incoming</span>
            <span className="list-count">{incomingCount}</span>
          </div>
          <div style={{ flex: 1, overflow: 'auto' }}>
            {offers.length === 0 && (
              <div style={{ padding: 24, textAlign: 'center', color: T.textDim, fontSize: 12 }}>
                No incoming files
              </div>
            )}
            <AnimatePresence>
              {offers.map(o => (
                <motion.div key={o.id} className="file-row" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                  {o.preview ? (
                    <img src={o.preview} className="file-thumb-img" alt="" style={{ borderRadius: 7, width: 40, height: 40, objectFit: 'cover' }}/>
                  ) : (
                    <div className="file-thumb-generic">{Icons.file(18)}</div>
                  )}
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="file-name">{o.name}</div>
                    <div className="file-meta">from phone · {formatSize(o.size)}</div>
                  </div>
                  <button onClick={() => handleReject(o.id)} style={{
                    appearance: 'none', border: 0, cursor: 'pointer',
                    width: 30, height: 30, borderRadius: 8,
                    background: 'transparent', color: T.textDim,
                    boxShadow: `inset 0 0 0 1px ${T.border}`,
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                  }}>{Icons.x(14)}</button>
                  <button onClick={() => handleAccept(o.id)} style={{
                    appearance: 'none', border: 0, cursor: 'pointer',
                    height: 30, padding: '0 14px', borderRadius: 8,
                    background: 'linear-gradient(180deg, #7376f4, #5a5ee3)', color: '#fff',
                    fontWeight: 600, fontSize: 12,
                    boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.18), 0 4px 10px rgba(99,102,241,0.25)',
                  }}>Accept</button>
                </motion.div>
              ))}
            </AnimatePresence>
          </div>
        </Card>

        {/* Transfers */}
        <Card pad={0} style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <div className="list-header">
            <span className="list-title">Transfers</span>
            <span className="list-count">{transferCount}</span>
          </div>
          <div style={{ flex: 1, overflow: 'auto' }}>
            {active.map(t => (
              <div key={t.id} style={{ padding: '12px 14px', borderBottom: `1px solid ${T.border}` }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 8 }}>
                  <div className="file-thumb-generic">{Icons.file(18)}</div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="file-name">{t.name}</div>
                    <div className="file-meta">
                      {formatSize(t.transferred)} <span style={{ color: T.text }}>/</span> {formatSize(t.size)} · sending
                    </div>
                  </div>
                  <div className="mono" style={{ color: T.indigo2, fontSize: 13, fontWeight: 600 }}>
                    {t.size > 0 ? Math.round(t.transferred / t.size * 100) : 0}%
                  </div>
                </div>
                <div className="progress-track">
                  <div className="progress-fill" style={{ width: t.size > 0 ? `${(t.transferred / t.size) * 100}%` : '0%' }}/>
                </div>
              </div>
            ))}
            {complete.map(t => (
              <div key={t.id} style={{ padding: '12px 14px', display: 'flex', alignItems: 'center', gap: 12, borderBottom: `1px solid ${T.border}` }}>
                <div className="complete-dot">{Icons.check(12)}</div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ color: T.text, fontSize: 12.5, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{t.name}</div>
                </div>
                <div className="mono" style={{ color: T.textDim, fontSize: 11 }}>{formatSize(t.size)}</div>
              </div>
            ))}
            {transferCount === 0 && (
              <div style={{ padding: 24, textAlign: 'center', color: T.textDim, fontSize: 12 }}>
                No transfers yet
              </div>
            )}
          </div>
        </Card>
      </div>
    </>
  )
}
