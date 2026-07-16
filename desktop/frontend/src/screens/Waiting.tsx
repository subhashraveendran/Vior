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
import React, { useState } from 'react'
import { Icons } from '../lib/icons'
import ConfirmModal from '../lib/ConfirmModal'
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

// pairDigits splits the code into individual characters so each digit
// can render in its own chip — the value the user reads off and MATCHES
// against the code shown on their phone.
function pairDigits(code: string | undefined): string[] {
  if (!code) return []
  return code.split('')
}

export default function WaitingScreen({ status, onStop, onCopy, onCopyPair, disconnectBanner }: WaitingScreenProps): React.JSX.Element {
  const s: ServerStatus | null = status
  const url = s?.url || ''
  const pairDisplay = formatPair(s?.pairCode)
  const digits = pairDigits(s?.pairCode)

  const [confirmStop, setConfirmStop] = useState(false)
  // Tracks which copy button last succeeded so we can flash "Copied!" for
  // ~1.5s in addition to the toast. Cleared on a timer.
  const [copied, setCopied] = useState<'url' | 'pair' | null>(null)
  const flashCopied = (which: 'url' | 'pair'): void => {
    setCopied(which)
    window.setTimeout(() => setCopied(c => (c === which ? null : c)), 1500)
  }

  return (
    <div className="waiting-v2">
      {disconnectBanner && (
        <div className="waiting-banner">
          <span className="dot dot-warn dot-pulse" role="img" aria-label="Status: Waiting to reconnect" />
          <span>{disconnectBanner} disconnected — waiting for it to reconnect.</span>
        </div>
      )}

      <div className="waiting-hero">
        <div className="waiting-eyebrow">
          <span className="dot dot-ok dot-pulse" role="img" aria-label="Status: Server ready, waiting for a device" />
          <span>Server ready · waiting for a device</span>
        </div>

        {/* QR is the hero — large, high-contrast, the primary path. */}
        <div className="waiting-qr-wrap">
          {s?.qrCodeDataUrl ? (
            <img
              src={s.qrCodeDataUrl}
              alt={url ? `QR code to connect to ${url} — scan with the Vior mobile app` : 'QR code — scan with the Vior mobile app to connect'}
              style={{ width: 236, height: 236, borderRadius: 12, display: 'block' }}
            />
          ) : (
            <div style={{ width: 236, height: 236, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)', background: 'var(--surface-1)', borderRadius: 12 }}>QR loading…</div>
          )}
        </div>

        <div className="waiting-headline">Scan to connect</div>
        <div className="waiting-sub">
          Open Vior on your phone and scan this QR with the in-app scanner.
          Both devices must be on the same Wi-Fi network.
        </div>

        {/* Pair code — the value the user MATCHES against their phone.
            Spaced mono digits, visually distinct from the QR fallback. */}
        {pairDisplay && (
          <div className="waiting-pair">
            <div className="waiting-pair-label">or enter code</div>
            <div className="waiting-pair-digits" aria-label={`Pair code: ${digits.join(' ')}`}>
              {digits.map((d, i) => (
                <span className="waiting-pair-digit" key={i}>{d}</span>
              ))}
            </div>
            <button className="btn btn-quiet btn-sm waiting-pair-copy" onClick={() => { onCopyPair(); flashCopied('pair') }}>
              {copied === 'pair' ? <>{Icons.check(14)} Copied!</> : <>{Icons.copy(14)} Copy code</>}
            </button>
          </div>
        )}

        {/* Secondary — raw connection URL + other interfaces, tucked below. */}
        {(url || (s?.urls && s.urls.length > 1)) && (
          <div className="waiting-secondary">
            {url && (
              <div className="waiting-url-row">
                <span className="waiting-cred-label">Address</span>
                <span className="mono waiting-url-val">{url}</span>
                <button className="btn btn-quiet btn-sm" onClick={() => { onCopy(); flashCopied('url') }} disabled={!url} aria-label="Copy connection URL">
                  {copied === 'url' ? <>{Icons.check(14)} Copied!</> : Icons.copy(14)}
                </button>
              </div>
            )}

            {s?.urls && s.urls.length > 1 && (
              <details className="waiting-other-ifs">
                <summary>Other network addresses ({s.urls.length})</summary>
                <ul>
                  {s.urls.map(u => (
                    <li key={u}>
                      <span className="mono">{u}</span>
                      <button className="btn btn-quiet btn-sm" onClick={() => navigator.clipboard?.writeText(u)} aria-label={`Copy ${u}`}>{Icons.copy(13)}</button>
                    </li>
                  ))}
                </ul>
              </details>
            )}
          </div>
        )}
      </div>

      <div className="waiting-foot">
        <button
          className="btn btn-ghost"
          onClick={() => setConfirmStop(true)}
        >
          {Icons.power(19)} Stop Server
        </button>
      </div>

      {confirmStop && (
        <ConfirmModal
          title="Stop the server?"
          body="Your phone won't be able to connect until you start the server again."
          confirmLabel="Stop server"
          cancelLabel="Keep running"
          danger
          onConfirm={() => { setConfirmStop(false); onStop() }}
          onCancel={() => setConfirmStop(false)}
        />
      )}
    </div>
  )
}
