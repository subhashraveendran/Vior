// macOS first-run Screen Recording prompt. Calls CheckPermissions to
// verify; success flips to a green check-state, then onDone returns
// to the regular UI.
import React, { useState } from 'react'
import { CheckPermissions } from '../../wailsjs/go/main/App'
import { Icons } from '../lib/icons'

export default function PermissionsModal({ onDone }) {
  const [granted, setGranted] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const verify = async () => {
    setVerifying(true)
    try { await CheckPermissions(); setGranted(true) } catch { /* still denied */ }
    setVerifying(false)
  }
  return (
    <div className="modal-backdrop">
      <div className="card modal">
        <span className="modal-icon" style={{ color: granted ? 'var(--ok)' : 'var(--accent)' }}>
          {granted ? Icons.check(28) : Icons.shield(28)}
        </span>
        <div className="modal-title">{granted ? 'Permission granted' : 'Screen Recording access'}</div>
        <div className="modal-body">
          {granted ? 'Vior can now mirror your display. You’re all set.'
            : <>macOS requires permission for Vior to capture your screen. Enable it under <span style={{ color: 'var(--text-1)', fontWeight: 600 }}>Privacy &amp; Security → Screen Recording</span>.</>}
        </div>
        <div style={{ display: 'flex', gap: 10, marginTop: 22 }}>
          {granted
            ? <button className="btn btn-primary btn-block" onClick={onDone}>{Icons.arrowR(19)} Continue</button>
            : <>
                <button className="btn btn-ghost btn-block">{Icons.settings(19)} Open Settings</button>
                <button className="btn btn-primary btn-block" onClick={verify}>{verifying ? <span className="spin" style={{ width: 17, height: 17, border: '2.4px solid var(--surface-3)', borderTopColor: 'var(--on-accent)', borderRadius: '50%', display: 'inline-block' }} /> : 'Verify'}</button>
              </>}
        </div>
      </div>
    </div>
  )
}
