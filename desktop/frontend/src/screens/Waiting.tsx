// Waiting screen — server is up, no client has joined yet.
//
// UX: collapse everything onto one centred column with the QR as the
// hero element (Deskreen-style — large, instantly scannable, with a
// copy-URL fallback right under it). Pair code is a 4-digit number —
// the user's "phone number" for Vior — stable for the life of the
// hardware, so they can memorise it once.
//
// Pre-connect surface intentionally hides Files / Permissions / virtual-
// display info: there's no client to send a file to, no input to inject,
// no display to describe. Those panels only appear after pairing.
import React from 'react'
import { Icons } from '../lib/icons'
import type { WaitingScreenProps as BaseWaitingScreenProps, ServerStatus } from '../types'

interface WaitingScreenProps extends BaseWaitingScreenProps {
  onCopyPair: () => void
  // Transient "Device disconnected" banner shown for ~3s after the
  // client drops. App.tsx clears it on a timer.
  disconnectBanner: string | null
}

// formatPair renders the pair code. Now that the code is 4 numeric
// digits there's nothing to chunk — but the helper stays so any
// future digit-grouping (e.g. 6-digit override) lives in one place.
function formatPair(code: string | undefined): string {
  if (!code) return ''
  return code
}

export default function WaitingScreen({ status, onStop, onCopy, onCopyPair, disconnectBanner }: WaitingScreenProps): React.JSX.Element {
  const s: ServerStatus | null = status
  const url = s?.url || ''
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
          Open the Vior mobile app on your phone and scan this QR with the
          in-app scanner, or enter the pair code shown below. Both devices must
          be on the same Wi-Fi network.
        </div>

        <div className="waiting-qr-wrap">
          {s?.qrCodeDataUrl ? (
            <img src={s.qrCodeDataUrl} alt="QR code — scan with Vior mobile app" style={{ width: 280, height: 280, borderRadius: 14, border: '1px solid var(--border)' }} />
          ) : (
            <div className="waiting-qr-wrap" style={{ width: 280, height: 280, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)' }}>QR loading…</div>
          )}
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
        <button
          className="btn btn-ghost"
          onClick={() => {
            if (window.confirm('Stop the server? Your phone will be disconnected.')) {
              onStop()
            }
          }}
        >
          {Icons.power(19)} Stop Server
        </button>
      </div>
    </div>
  )
}
