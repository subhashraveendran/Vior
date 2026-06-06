// Theme + accent + density + motion panel. Pushed off the Settings
// screen so it can take over the whole pane and feel like a
// modal-without-modal. Returns to Settings via onClose.
import React, { useState, useEffect } from 'react'
import { Icons } from '../lib/icons'
import Glyph from '../lib/Glyph'
import { ACCENTS, applyAccent } from '../lib/accent'

export default function AppearancePanel({ accent, setAccent, onClose }) {
  const [style, setStyle] = useState(localStorage.getItem('vior_style') || 'precise')
  const [density, setDensity] = useState(localStorage.getItem('vior_density') || 'regular')
  const [motion, setMotion] = useState(localStorage.getItem('vior_motion') || 'expressive')
  useEffect(() => {
    document.documentElement.setAttribute('data-vior-style', style); localStorage.setItem('vior_style', style)
  }, [style])
  useEffect(() => {
    document.documentElement.setAttribute('data-vior-density', density); localStorage.setItem('vior_density', density)
  }, [density])
  useEffect(() => {
    document.documentElement.setAttribute('data-vior-motion', motion); localStorage.setItem('vior_motion', motion)
  }, [motion])
  const Seg = ({ value, onChange, opts }) => (
    <div className="seg" style={{ gridTemplateColumns: `repeat(${opts.length},1fr)` }}>
      {opts.map(o => (
        <button key={o} className={`seg-btn ${value === o ? 'active' : ''}`} onClick={() => onChange(o)}>
          <div className="seg-row"><span style={{ textTransform: 'capitalize' }}>{o}</span></div>
        </button>
      ))}
    </div>
  )
  return (
    <div className="scroll settings-wrap">
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 20 }}>
        <button onClick={onClose} className="btn btn-ghost btn-sm" style={{ width: 38, padding: 0, minHeight: 38 }}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="M15 18l-6-6 6-6"/></svg>
        </button>
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 17, fontWeight: 600 }}>Appearance</div>
          <div style={{ fontSize: 12.5, color: 'var(--text-3)', marginTop: 1 }}>Theme, accent, density &amp; motion</div>
        </div>
      </div>

      <div className="label" style={{ marginBottom: 12 }}>Preview</div>
      <div className="inset" style={{ padding: 14, display: 'flex', alignItems: 'center', gap: 12 }}>
        <Glyph size={26} />
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 6 }}>
          <span style={{ height: 7, width: '72%', borderRadius: 'var(--r-pill)', background: 'var(--text-3)' }} />
          <span style={{ height: 7, width: '46%', borderRadius: 'var(--r-pill)', background: 'var(--surface-3)' }} />
        </div>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, padding: '8px 13px', borderRadius: 'var(--r)', background: 'var(--accent)', color: 'var(--on-accent)', fontSize: 12.5, fontWeight: 600 }}>
          <span className="dot dot-pulse" style={{ background: 'var(--on-accent)' }} /> Connect
        </span>
      </div>

      <div className="label" style={{ marginTop: 20, marginBottom: 12 }}>Style</div>
      <Seg value={style} onChange={setStyle} opts={['precise', 'instrument', 'soft']} />

      <div className="label" style={{ marginTop: 18, marginBottom: 12 }}>Accent</div>
      <div style={{ display: 'flex', gap: 10 }}>
        {ACCENTS.map(a => (
          <button key={a.hex} title={a.name}
            onClick={() => { applyAccent(a.hex); setAccent(a.hex) }}
            style={{
              flex: 1, height: 48, borderRadius: 'var(--r-sm)', cursor: 'pointer',
              background: a.hex,
              border: accent === a.hex ? '2px solid var(--text-1)' : '1px solid var(--border)',
              display: 'grid', placeItems: 'center', color: a.on,
            }}>
            {accent === a.hex && Icons.check(18)}
          </button>
        ))}
      </div>

      <div className="label" style={{ marginTop: 18, marginBottom: 12 }}>Density</div>
      <Seg value={density} onChange={setDensity} opts={['compact', 'regular', 'comfy']} />

      <div className="label" style={{ marginTop: 18, marginBottom: 12 }}>Motion</div>
      <Seg value={motion} onChange={setMotion} opts={['expressive', 'subtle', 'off']} />

      <button className="btn btn-primary btn-block" style={{ marginTop: 26 }} onClick={onClose}>Done</button>
    </div>
  )
}
