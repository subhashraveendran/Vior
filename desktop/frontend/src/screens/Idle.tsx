// Idle screen — what the user sees before they hit Start Server.
import React from 'react'
import { Icons } from '../lib/icons'
import Glyph from '../lib/Glyph'

interface IdleScreenProps {
  onStart: () => void
  showUpdate?: boolean
  onUpdate: () => void
  onDismiss: () => void
}

export default function IdleScreen({ onStart, showUpdate, onUpdate, onDismiss }: IdleScreenProps) {
  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      {showUpdate && (
        <div className="update-banner">
          <span style={{ color: 'var(--accent)' }}>{Icons.download(19)}</span>
          <div style={{ flex: 1 }}>
            <span className="update-banner-title">Vior 2.1 is available.</span>
            <span className="update-banner-body">H.264 streaming and faster discovery.</span>
          </div>
          <button className="btn btn-primary btn-sm" onClick={onUpdate}>Update</button>
          <button onClick={onDismiss} style={{ background: 'none', border: 'none', color: 'var(--text-3)', cursor: 'pointer', padding: 4 }}>{Icons.close(15)}</button>
        </div>
      )}
      <div className="idle">
        <div className="radar-wrap">
          <div className="radar-circle" style={{ width: 210, height: 210, opacity: 0.7 }} />
          <div className="radar-circle" style={{ width: 158, height: 158, opacity: 0.54 }} />
          <div className="radar-circle" style={{ width: 110, height: 110, opacity: 0.38 }} />
          <span className="radar-ping" />
          <span className="radar-ping" style={{ animationDelay: '1.4s' }} />
          <div className="radar-core"><Glyph size={44} /></div>
        </div>
        <div className="state-pill">
          <span className="dot dot-idle" />
          <span className="label">Not broadcasting</span>
        </div>
        <div className="idle-title">Ready when you are</div>
        <div className="idle-sub">Start the server and Vior on your phone discovers this Mac automatically — over your local network, no setup.</div>
        <button className="btn btn-primary idle-cta-btn" onClick={onStart}>
          {Icons.power(20)} Start Server
        </button>
        <p className="idle-sub" style={{ textAlign: 'center', marginTop: 12, color: 'var(--text-3)' }}>
          Once started, your phone discovers this Mac over Wi-Fi — open the Vior mobile app and scan the QR.
        </p>
      </div>
    </div>
  )
}
