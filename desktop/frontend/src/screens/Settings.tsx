// Settings — the only screen always reachable regardless of state.
// Intentionally small surface:
//   • Stream quality preset (perf / balanced / quality)
//   • Auto-launch at login (persisted in localStorage; harness picks
//     it up at next launch via LaunchAtLogin login item — actual OS
//     wiring is a follow-up, the toggle is here so the UI is stable.)
//   • macOS menu-bar visibility (existing, persists to ~/.vior/menubar.flag)
//   • USB connections allowed (auto-accept paired devices over USB)
//   • Local discovery on/off (lets the user disable mDNS broadcast on
//     untrusted networks)
//   • Theme / Appearance (existing)
//
// Removed: resolution presets (cosmetic only, not respected by the
// capture pipeline — virtual displays match the phone's panel size),
// frame-rate toggle (now folded into the quality preset), and the
// fake Displays / USB-ADB placeholder rows.
import React, { useCallback, useEffect, useState } from 'react'
import {
  GetMenuBarVisible, SetMenuBarVisible,
  ListTrustedDevices, ForgetTrustedDevice, ClearAllTrustedDevices,
  GetServerStatus, SetPairCode,
  EventsOn,
  SetAutoDiscovery, GetAutoDiscovery,
  SetUSBAutoAccept, GetUSBAutoAccept,
} from '../lib/api'
import { Icons } from '../lib/icons'
import Glyph from '../lib/Glyph'
import { accentName } from '../lib/accent'
import AppearancePanel from './Appearance'
import type { SettingsScreenProps, AppConfig } from '../types'
import type { main } from '../../wailsjs/go/models'

// formatRelative renders an ISO timestamp as "3h ago" / "2 days ago".
// Falls back to the date for anything older than a week. Pure: zero
// dependencies, deterministic given (now - then). Used in the Trusted
// Devices card.
function formatRelative(iso: string): string {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diff = Date.now() - then
  if (diff < 60_000) return 'just now'
  const m = Math.floor(diff / 60_000)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 7) return `${d}d ago`
  return new Date(iso).toLocaleDateString()
}

interface Preset {
  id: string
  label: string
  sub: string
  q: number
  f: number
}

export default function SettingsScreen({ config, onChange, accent, setAccent }: SettingsScreenProps): React.JSX.Element {
  const presets: Preset[] = [
    { id: 'performance', label: 'Performance', sub: 'Lower latency · 60 fps · q60', q: 60, f: 60 },
    { id: 'balanced',    label: 'Balanced',    sub: 'Recommended · 30 fps · q80', q: 80, f: 30 },
    { id: 'quality',     label: 'Quality',     sub: 'Sharper image · 30 fps · q92', q: 92, f: 30 },
  ]
  const cur: string = presets.find(p => p.q === config.quality && p.f === config.frameRate)?.id || 'balanced'

  // Settings persisted in localStorage. The backend doesn't wire these
  // yet — toggles are marked "Coming soon" until the backend lands.
  const [menuBar, setMenuBar] = useState<boolean>(true)
  useEffect(() => { GetMenuBarVisible?.().then(setMenuBar).catch(() => {}) }, [])
  const toggleMenuBar = (v: boolean): void => { setMenuBar(v); SetMenuBarVisible?.(v) }

  // Wired discovery toggle — calls SetAutoDiscovery on the Go backend.
  const [autoDiscovery, setAutoDiscovery] = useState<boolean>(true)
  useEffect(() => { GetAutoDiscovery?.().then(setAutoDiscovery).catch(() => {}) }, [])
  const toggleDiscovery = (v: boolean): void => {
    setAutoDiscovery(v)
    SetAutoDiscovery?.(v)
  }

  // USB auto-accept toggle — calls SetUSBAutoAccept on the Go backend.
  const [usbAutoAccept, setUsbAutoAccept] = useState<boolean>(false)
  useEffect(() => { GetUSBAutoAccept?.().then(setUsbAutoAccept).catch(() => {}) }, [])
  const toggleUsbAutoAccept = (v: boolean): void => {
    setUsbAutoAccept(v)
    SetUSBAutoAccept?.(v)
  }

  // Trusted Devices list. Polled on mount + whenever a new client
  // connects (a pair-code success may add a fresh row). Empty array
  // means "no trusted devices yet" — distinct from the not-yet-loaded
  // state (null) so we can avoid an empty-state flash on first paint.
  const [trusted, setTrusted] = useState<main.TrustedDevice[] | null>(null)
  const refreshTrusted = useCallback((): void => {
    ListTrustedDevices?.().then(setTrusted).catch(() => setTrusted([]))
  }, [])
  useEffect(() => {
    refreshTrusted()
    const off = EventsOn?.('client:connected', () => refreshTrusted())
    return (): void => { if (typeof off === 'function') off() }
  }, [refreshTrusted])
  const onForget = (deviceID: string, name: string): void => {
    if (!window.confirm(`Forget "${name || deviceID}"? They'll need to re-enter the pair code next time.`)) return
    ForgetTrustedDevice?.(deviceID).then(refreshTrusted).catch(() => refreshTrusted())
  }
  const onClearAll = (): void => {
    if (!window.confirm('Forget every trusted device? All paired phones/tablets will need the pair code again on next connect.')) return
    ClearAllTrustedDevices?.().then(refreshTrusted).catch(() => refreshTrusted())
  }

  // Pair-code editor. Reads current code from the server on mount + after
  // every save; user types a 4-8 digit override and clicks Save, or
  // clicks Reset to fall back to the hardware-derived default by deleting
  // the override file (SetPairCode("") on the server side).
  const [pairCode, setPairCode] = useState<string>('')
  const [pairInput, setPairInput] = useState<string>('')
  const [pairSaving, setPairSaving] = useState<boolean>(false)
  const [pairMsg, setPairMsg] = useState<string>('')
  const refreshPair = useCallback((): void => {
    GetServerStatus?.().then(s => {
      const code = (s as { pairCode?: string }).pairCode || ''
      setPairCode(code)
      setPairInput(code)
    }).catch(() => {})
  }, [])
  useEffect(() => { refreshPair() }, [refreshPair])
  const onSavePair = async (): Promise<void> => {
    const v = pairInput.replace(/\D/g, '').slice(0, 8)
    if (v.length < 4) { setPairMsg('Pair code must be 4–8 digits.'); return }
    setPairSaving(true); setPairMsg('')
    try {
      await SetPairCode?.(v)
      setPairMsg('Saved. New code is active immediately.')
      refreshPair()
    } catch (e) {
      setPairMsg('Save failed: ' + String(e))
    } finally { setPairSaving(false) }
  }
  const onResetPair = async (): Promise<void> => {
    if (!window.confirm('Reset to the hardware-derived default? Devices that remember the current code will need the new one.')) return
    setPairSaving(true); setPairMsg('')
    try {
      await SetPairCode?.('')
      setPairMsg('Reset. Default pair code restored.')
      refreshPair()
    } catch (e) {
      setPairMsg('Reset failed: ' + String(e))
    } finally { setPairSaving(false) }
  }

  const [appearance, setAppearance] = useState<boolean>(false)
  const style: string = localStorage.getItem('vior_style') || 'precise'
  const density: string = localStorage.getItem('vior_density') || 'regular'
  const motion: string = localStorage.getItem('vior_motion') || 'expressive'

  if (appearance) return <AppearancePanel accent={accent} setAccent={setAccent} onClose={() => setAppearance(false)} />

  return (
    <div className="scroll settings-wrap">
      <div className="label" style={{ marginBottom: 12 }}>Stream quality</div>
      <div className="seg" style={{ gridTemplateColumns: 'repeat(3, 1fr)' }}>
        {presets.map(p => (
          <button key={p.id} className={`seg-btn ${cur === p.id ? 'active' : ''}`}
            onClick={() => onChange({ ...config, quality: p.q, frameRate: p.f } as AppConfig)}>
            <div className="seg-row"><span>{p.label}</span></div>
            <div className="seg-sub" style={{ marginLeft: 0 }}>{p.sub}</div>
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>General</div>
      <div className="card">
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Launch Vior at login</div>
            <div className="settings-row-sub">Server starts in the background, ready for the phone</div>
          </div>
          <span className="badge badge-coming">Coming soon</span>
        </div>
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Show in macOS menu bar</div>
            <div className="settings-row-sub">Quick status + Start/Stop/Quit in the top-right tray</div>
          </div>
          <button className={`toggle ${menuBar ? 'toggle-on' : 'toggle-off'}`} onClick={() => toggleMenuBar(!menuBar)}>
            <span className="toggle-knob" style={{ transform: `translateX(${menuBar ? 17 : 0}px)` }} />
          </button>
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Connectivity</div>
      <div className="card">
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Local discovery</div>
            <div className="settings-row-sub">Broadcast on the LAN so the phone finds Vior automatically</div>
          </div>
          <button className={`toggle ${autoDiscovery ? 'toggle-on' : 'toggle-off'}`} onClick={() => toggleDiscovery(!autoDiscovery)}>
            <span className="toggle-knob" style={{ transform: `translateX(${autoDiscovery ? 17 : 0}px)` }} />
          </button>
        </div>
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Auto-accept paired USB devices</div>
            <div className="settings-row-sub">Skip the connect prompt for phones you've paired before</div>
          </div>
          <button className={`toggle ${usbAutoAccept ? 'toggle-on' : 'toggle-off'}`} onClick={() => toggleUsbAutoAccept(!usbAutoAccept)}>
            <span className="toggle-knob" style={{ transform: `translateX(${usbAutoAccept ? 17 : 0}px)` }} />
          </button>
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Pair code</div>
      <div className="card">
        <div className="settings-row" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 8 }}>
          <div className="settings-row-body">
            <div className="settings-row-title">Your pair code</div>
            <div className="settings-row-sub">
              4–8 digit code phones type to connect. Defaults to a stable value derived from this Mac's hardware ID — survives reinstall + file delete. Override here to set your own memorable code.
            </div>
          </div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <input
              type="text"
              inputMode="numeric"
              pattern="[0-9]*"
              maxLength={8}
              value={pairInput}
              onChange={e => { setPairInput(e.target.value.replace(/\D/g, '').slice(0, 8)); setPairMsg('') }}
              placeholder={pairCode || '0000'}
              style={{
                fontFamily: 'var(--font-mono)', fontSize: 22, letterSpacing: '0.25em',
                padding: '10px 14px', borderRadius: 10, border: '1px solid var(--border)',
                background: 'var(--surface-1)', color: 'var(--text-1)', width: 160, textAlign: 'center',
              }}
            />
            <button
              className="btn btn-primary btn-sm"
              disabled={pairSaving || pairInput.length < 4 || pairInput === pairCode}
              onClick={onSavePair}>
              {pairSaving ? 'Saving…' : 'Save'}
            </button>
            <button
              className="btn btn-ghost btn-sm"
              disabled={pairSaving}
              onClick={onResetPair}
              title="Restore the hardware-derived default">
              Reset
            </button>
          </div>
          {pairMsg && (
            <div style={{ fontSize: 12, color: pairMsg.startsWith('Saved') || pairMsg.startsWith('Reset') ? 'var(--accent)' : '#e05a5a' }}>
              {pairMsg}
            </div>
          )}
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Trusted Devices</div>
      <div className="card">
        {trusted && trusted.length === 0 && (
          <div className="settings-row" style={{ color: 'var(--text-3)', fontSize: 13 }}>
            No trusted devices yet. Pair from your phone to add one.
          </div>
        )}
        {trusted && trusted.map(t => (
          <div className="settings-row" key={t.deviceId}>
            <div className="settings-row-body">
              <div className="settings-row-title">
                {t.name || 'Unknown device'}
                {t.platform && (
                  <span style={{
                    marginLeft: 8, padding: '2px 7px', borderRadius: 8,
                    background: 'var(--accent-weak)', color: 'var(--accent)',
                    fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5,
                  }}>{t.platform}</span>
                )}
              </div>
              <div className="settings-row-sub">Last seen {formatRelative(t.lastSeen)}</div>
            </div>
            <button className="btn btn-ghost btn-sm" onClick={() => onForget(t.deviceId, t.name)}>Forget</button>
          </div>
        ))}
        {trusted && trusted.length > 0 && (
          <div className="settings-row" style={{ justifyContent: 'flex-end' }}>
            <button
              onClick={onClearAll}
              style={{ background: 'none', border: 'none', color: '#e05a5a', cursor: 'pointer', fontSize: 12, fontWeight: 600 }}>
              Clear all trusted devices
            </button>
          </div>
        )}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Appearance</div>
      <button onClick={() => setAppearance(true)} className="card" style={{ width: '100%', textAlign: 'left', cursor: 'pointer' }}>
        <div className="settings-row">
          <span style={{ width: 34, height: 34, flex: 'none', borderRadius: 'var(--r-sm)', background: 'var(--accent)', display: 'grid', placeItems: 'center', color: 'var(--on-accent)' }}>{Icons.display(17)}</span>
          <div className="settings-row-body">
            <div className="settings-row-title">Theme &amp; motion</div>
            <div className="settings-row-sub" style={{ textTransform: 'capitalize' }}>{style} · {accentName(accent)} · {density} · {motion}</div>
          </div>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" style={{ color: 'var(--text-3)' }}><path d="M9 5l7 7-7 7"/></svg>
        </div>
      </button>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>About</div>
      <div className="card">
        <div className="settings-row">
          <Glyph size={28} />
          <div className="settings-row-body">
            <div className="settings-row-title">Vior</div>
            <div className="settings-row-sub">Phase 2 · open source</div>
          </div>
          <a href="https://github.com/subhashraveendran/Vior" target="_blank" rel="noreferrer" className="btn btn-ghost btn-sm" style={{ textDecoration: 'none' }}>Check for updates</a>
        </div>
      </div>
    </div>
  )
}
