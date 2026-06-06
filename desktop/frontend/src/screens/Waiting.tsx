// Waiting screen — server is up, no client has joined yet.
//
// UX: collapse everything onto one centred column with the QR as the
// hero element (Deskreen-style — large, instantly scannable, with a
// copy-URL fallback right under it). Pair code is rendered ABC-123
// purely for visual chunking — the server stores the raw 6-char form
// and the mobile parses both with or without the dash.
//
// Pre-connect surface intentionally hides Files / Permissions / virtual-
// display info: there's no client to send a file to, no input to inject,
// no display to describe. Those panels only appear after pairing.
import React from 'react'
import { Icons } from '../lib/icons'
import QR from '../lib/QR'
import type { WaitingScreenProps as BaseWaitingScreenProps, ServerStatus } from '../types'

interface WaitingScreenProps extends BaseWaitingScreenProps {
  onCopyPair: () => void
  // Transient "Device disconnected" banner shown for ~3s after the
  // client drops. App.tsx clears it on a timer.
  disconnectBanner: string | null
}

// formatPair splits "ABC123" → "ABC-123" purely for readability.
function formatPair(code: string | undefined): string {
  if (!code) return ''
  if (code.length <= 3) return code
  return `${code.slice(0, 3)}-${code.slice(3)}`
}

export default function WaitingScreen({ status, onStop, onCopy, onCopyPair, disconnectBanner }: WaitingScreenProps): React.JSX.Element {
  const s: ServerStatus | null = status
  const url = s?.url || ''
  const seed = s?.url || 'vior'
  const pairDisplay = formatPair(s?.pairCode)
  return (
    <div className="waiting-v2">
      {disconnectBanner && (
        <div className="waiting-banner">
          <span className="dot dot-warn dot-pulse" />
          <span>{disconnectBanner} disconnected — waiting for it to reconnect.</span>
        </div>
      )}

      <div className="waiting-hero">
        <div className="waiting-eyebrow">
          <span className="dot dot-ok dot-pulse" />
          <span>Server ready · waiting for a device</span>
        </div>
        <div className="waiting-headline">Scan to connect</div>
        <div className="waiting-sub">
          Open Vior on your phone and either scan the QR or enter the pair code.
          Vior on your phone should also appear in its device list automatically.
        </div>

        <div className="waiting-qr-wrap">
          <QR size={280} seed={seed} />
        </div>

        <div className="waiting-actions">
          <button className="btn btn-ghost btn-sm" onClick={onCopy} disabled={!url}>
            {Icons.copy(15)} Copy URL
          </button>
          <button className="btn btn-ghost btn-sm" onClick={onCopyPair} disabled={!pairDisplay}>
            {Icons.copy(15)} Copy pair code
          </button>
        </div>

        {(url || pairDisplay) && (
          <div className="waiting-creds">
            {url && (
              <div className="waiting-cred-row">
                <span className="waiting-cred-label">Address</span>
                <span className="mono waiting-cred-val">{url}</span>
              </div>
            )}
            {pairDisplay && (
              <div className="waiting-cred-row">
                <span className="waiting-cred-label">Pair code</span>
                <span className="mono waiting-cred-val waiting-cred-pair">{pairDisplay}</span>
              </div>
            )}
          </div>
        )}

        {s?.urls && s.urls.length > 1 && (
          <details className="waiting-other-ifs">
            <summary>Other network addresses ({s.urls.length})</summary>
            <ul>
              {s.urls.map(u => (
                <li key={u}>
                  <span className="mono">{u}</span>
                  <button className="btn btn-quiet btn-sm" onClick={() => navigator.clipboard?.writeText(u)}>{Icons.copy(13)}</button>
                </li>
              ))}
            </ul>
          </details>
        )}
      </div>

      <div className="waiting-foot">
        <button className="btn btn-ghost" onClick={onStop}>
          {Icons.power(19)} Stop Server
        </button>
      </div>
    </div>
  )
}
