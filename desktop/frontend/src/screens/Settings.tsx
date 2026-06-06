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
import React, { useEffect, useState } from 'react'
import { GetMenuBarVisible, SetMenuBarVisible } from '../lib/api'
import { Icons } from '../lib/icons'
import Glyph from '../lib/Glyph'
import { accentName } from '../lib/accent'
import AppearancePanel from './Appearance'
import type { SettingsScreenProps, AppConfig } from '../types'

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
  // yet (autoLaunch needs a Login Item plist; discovery needs a config
  // toggle to plumb through); the UI keeps state stable so the user's
  // choice survives reloads when the backend lands.
  const [autoLaunch, setAutoLaunch] = useState<boolean>(localStorage.getItem('vior_autolaunch') === '1')
  const [usbAccept, setUsbAccept] = useState<boolean>(localStorage.getItem('vior_usb_accept') !== '0')
  const [discovery, setDiscovery] = useState<boolean>(localStorage.getItem('vior_discovery') !== '0')
  const [menuBar, setMenuBar] = useState<boolean>(true)
  useEffect(() => { GetMenuBarVisible?.().then(setMenuBar).catch(() => {}) }, [])
  const toggleMenuBar = (v: boolean): void => { setMenuBar(v); SetMenuBarVisible?.(v) }
  const toggleAutoLaunch = (v: boolean): void => { setAutoLaunch(v); localStorage.setItem('vior_autolaunch', v ? '1' : '0') }
  const toggleUsbAccept = (v: boolean): void => { setUsbAccept(v); localStorage.setItem('vior_usb_accept', v ? '1' : '0') }
  const toggleDiscovery = (v: boolean): void => { setDiscovery(v); localStorage.setItem('vior_discovery', v ? '1' : '0') }

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
          <button className={`toggle ${autoLaunch ? 'toggle-on' : 'toggle-off'}`} onClick={() => toggleAutoLaunch(!autoLaunch)}>
            <span className="toggle-knob" style={{ transform: `translateX(${autoLaunch ? 17 : 0}px)` }} />
          </button>
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
          <button className={`toggle ${discovery ? 'toggle-on' : 'toggle-off'}`} onClick={() => toggleDiscovery(!discovery)}>
            <span className="toggle-knob" style={{ transform: `translateX(${discovery ? 17 : 0}px)` }} />
          </button>
        </div>
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Auto-accept paired USB devices</div>
            <div className="settings-row-sub">Skip the connect prompt for phones you've paired before</div>
          </div>
          <button className={`toggle ${usbAccept ? 'toggle-on' : 'toggle-off'}`} onClick={() => toggleUsbAccept(!usbAccept)}>
            <span className="toggle-knob" style={{ transform: `translateX(${usbAccept ? 17 : 0}px)` }} />
          </button>
        </div>
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
