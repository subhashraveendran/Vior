// Settings — stream quality, resolution, fps, connectivity (USB +
// menu bar toggle), displays (stub), USB/ADB (stub), about, and the
// gateway into Appearance.
import React, { useState, useEffect } from 'react'
import { GetMenuBarVisible, SetMenuBarVisible } from '../../wailsjs/go/main/App'
import { Icons } from '../lib/icons'
import Glyph from '../lib/Glyph'
import { accentName } from '../lib/accent'
import AppearancePanel from './Appearance'

export default function SettingsScreen({ config, onChange, accent, setAccent }) {
  const presets = [
    { id: 'performance', label: 'Performance', sub: 'Lower latency', q: 60, f: 60 },
    { id: 'balanced', label: 'Balanced', sub: 'Recommended', q: 80, f: 30 },
    { id: 'quality', label: 'Quality', sub: 'Sharper image', q: 92, f: 30 },
  ]
  const resolutions = [
    { id: 'auto', label: 'Auto', sub: 'Match the display' },
    { id: '2560', label: '2560 × 1600', sub: 'Retina' },
    { id: '1920', label: '1920 × 1200', sub: '' },
    { id: '1280', label: '1280 × 800', sub: 'Bandwidth saver' },
  ]
  const cur = presets.find(p => p.q === config.quality && p.f === config.frameRate)?.id || 'balanced'
  const [res, setRes] = useState('auto')
  const [usb, setUsb] = useState(true)
  const [adb, setAdb] = useState(true)
  const [menuBar, setMenuBar] = useState(true)
  useEffect(() => { GetMenuBarVisible?.().then(setMenuBar).catch(() => {}) }, [])
  const toggleMenuBar = (v) => { setMenuBar(v); SetMenuBarVisible?.(v) }
  const [appearance, setAppearance] = useState(false)
  const style = localStorage.getItem('vior_style') || 'precise'
  const density = localStorage.getItem('vior_density') || 'regular'
  const motion = localStorage.getItem('vior_motion') || 'expressive'

  if (appearance) return <AppearancePanel accent={accent} setAccent={setAccent} onClose={() => setAppearance(false)} />

  return (
    <div className="scroll settings-wrap">
      <div className="label" style={{ marginBottom: 12 }}>Stream quality</div>
      <div className="seg" style={{ gridTemplateColumns: 'repeat(3, 1fr)' }}>
        {presets.map(p => (
          <button key={p.id} className={`seg-btn ${cur === p.id ? 'active' : ''}`}
            onClick={() => onChange({ ...config, quality: p.q, frameRate: p.f })}>
            <div className="seg-row"><span>{p.label}</span></div>
            <div className="seg-sub" style={{ marginLeft: 0 }}>{p.sub}</div>
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Resolution</div>
      <div className="card" style={{ overflow: 'hidden' }}>
        {resolutions.map((r, i) => (
          <button key={r.id} onClick={() => setRes(r.id)} style={{
            display: 'flex', alignItems: 'center', gap: 12, width: '100%', textAlign: 'left',
            padding: '13px 15px', background: res === r.id ? 'var(--accent-weak)' : 'transparent',
            border: 'none', borderTop: i ? '1px solid var(--border)' : 'none', cursor: 'pointer',
          }}>
            <span style={{ flex: 1, minWidth: 0 }}>
              <span style={{ display: 'block', fontSize: 14, fontWeight: 600, color: 'var(--text-1)' }}>{r.label}</span>
              {r.sub && <span className="mono" style={{ display: 'block', fontSize: 11.5, color: 'var(--text-3)', marginTop: 2 }}>{r.sub}</span>}
            </span>
            {res === r.id
              ? <span style={{ color: 'var(--accent)' }}>{Icons.check(18)}</span>
              : <span style={{ width: 18, height: 18, borderRadius: '50%', border: '1.5px solid var(--border-strong)', flex: 'none' }} />}
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Frame rate</div>
      <div className="seg" style={{ gridTemplateColumns: '1fr 1fr' }}>
        {['30', '60'].map(v => (
          <button key={v} className={`seg-btn ${String(config.frameRate) === v ? 'active' : ''}`}
            onClick={() => onChange({ ...config, frameRate: parseInt(v) })}>
            <div className="seg-row"><span>{v} fps</span></div>
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Connectivity</div>
      <div className="card">
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">Allow USB connections</div>
            <div className="settings-row-sub">Connect over cable when Wi-Fi drops</div>
          </div>
          <button className={`toggle ${usb ? 'toggle-on' : 'toggle-off'}`} onClick={() => setUsb(!usb)}>
            <span className="toggle-knob" style={{ transform: `translateX(${usb ? 17 : 0}px)` }} />
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

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>Displays</div>
      <div className="card">
        <div className="settings-row">
          <span style={{ color: 'var(--text-3)' }}>{Icons.monitor2(18)}</span>
          <div className="settings-row-body">
            <div className="settings-row-title">Built-in Retina Display</div>
            <div className="settings-row-sub">2560 × 1600</div>
          </div>
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--accent)', padding: '4px 9px', borderRadius: 'var(--r-pill)', background: 'var(--accent-weak)' }}>Primary</span>
        </div>
        <div className="settings-row">
          <span style={{ color: 'var(--text-3)' }}>{Icons.monitor2(18)}</span>
          <div className="settings-row-body">
            <div className="settings-row-title">DELL U2720Q</div>
            <div className="settings-row-sub">3840 × 2160</div>
          </div>
        </div>
      </div>

      <div className="label" style={{ marginTop: 24, marginBottom: 12 }}>USB / ADB</div>
      <div className="card">
        <div className="settings-row">
          <div className="settings-row-body">
            <div className="settings-row-title">{adb ? '1 device attached' : 'No bridge installed'}</div>
            <div className="settings-row-sub">{adb ? 'Pixel 8 Pro · usb-2' : 'platform-tools not found'}</div>
          </div>
          {adb
            ? <button className="btn btn-ghost btn-sm" onClick={() => setAdb(true)}>{Icons.refresh(17)} Restart</button>
            : <button className="btn btn-primary btn-sm" onClick={() => setAdb(true)}>{Icons.download(17)} Install</button>}
        </div>
      </div>

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
    </div>
  )
}
