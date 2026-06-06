// Waiting screen — server running, no client yet. Shows the URL list,
// pair code (with Copy buttons), and a decorative QR.
import React from 'react'
import { Icons } from '../lib/icons'
import QR from '../lib/QR'

export default function WaitingScreen({ status, onStop, onCopy }) {
  const url = status?.url || ''
  const seed = status?.url || 'vior'
  return (
    <div className="waiting">
      <div className="waiting-left">
        <div className="waiting-tag">
          <span className="dot dot-ok dot-pulse" />
          <span className="waiting-tag-text">Server running · waiting for a device</span>
        </div>
        <div className="waiting-title">Connect your phone</div>
        <div className="waiting-body">Open Vior on your phone — it should appear automatically. Or use the address below.</div>

        <div className="card" style={{ marginTop: 24, padding: 18 }}>
          <div className="label" style={{ marginBottom: 12 }}>Connect manually</div>
          {(status?.urls && status.urls.length > 1) ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {status.urls.map(u => (
                <div key={u} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <span className="mono" style={{ fontSize: 15, fontWeight: 500 }}>{u}</span>
                  <button className="btn btn-ghost btn-sm" onClick={() => navigator.clipboard?.writeText(u)}>{Icons.copy(15)}Copy</button>
                </div>
              ))}
              <div style={{ fontSize: 11.5, color: 'var(--text-3)' }}>Multiple interfaces detected — try each on your phone.</div>
            </div>
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
              <span className="mono" style={{ fontSize: 17, fontWeight: 500 }}>{url || `:${status?.port}`}</span>
              <button className="btn btn-ghost btn-sm" onClick={onCopy}>{Icons.copy(15)}Copy</button>
            </div>
          )}
          {status?.pairCode && (
            <div style={{ marginTop: 14, paddingTop: 14, borderTop: '1px solid var(--border)' }}>
              <div className="label" style={{ marginBottom: 6 }}>Pair code</div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <span className="mono" style={{ fontSize: 22, fontWeight: 600, letterSpacing: '0.18em', color: 'var(--accent)' }}>{status.pairCode}</span>
                <button className="btn btn-ghost btn-sm" onClick={() => navigator.clipboard?.writeText(status.pairCode)} title="Copy pair code">{Icons.copy(14)}Copy</button>
              </div>
              <div style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 6 }}>Enter this on your phone to authorize the connection.</div>
            </div>
          )}
          <div style={{ height: 1, background: 'var(--border)', margin: '14px 0' }} />
          <div style={{ display: 'flex', gap: 18 }}>
            <a href={url} style={{ display: 'flex', alignItems: 'center', gap: 7, color: 'var(--text-2)', fontSize: 13, fontWeight: 600, textDecoration: 'none' }}>{Icons.link(16)} Open in browser</a>
          </div>
        </div>
        <div style={{ flex: 1 }} />
        <button className="btn btn-ghost" style={{ alignSelf: 'flex-start', marginTop: 18 }} onClick={onStop}>
          {Icons.power(19)} Stop Server
        </button>
      </div>
      <div className="waiting-right">
        <QR size={196} seed={seed} />
        <div style={{ textAlign: 'center' }}>
          <div style={{ fontSize: 14, fontWeight: 600 }}>Scan to connect</div>
          <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 4 }}>Point your phone camera here</div>
        </div>
      </div>
    </div>
  )
}
