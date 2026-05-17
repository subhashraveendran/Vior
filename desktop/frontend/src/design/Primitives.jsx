import React from 'react'
import { T } from './tokens'
import { Icons } from './Icons'

// Brand mark — indigo rounded square with link glyph.
export function BrandMark({ size = 28 }) {
  return (
    <div style={{
      width: size, height: size, borderRadius: 8,
      background: 'linear-gradient(135deg, #6366f1, #4f46e5)',
      display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
      boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.25), 0 4px 12px rgba(99,102,241,0.35)',
      flexShrink: 0,
    }}>
      <svg width={size*0.55} height={size*0.55} viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
        <path d="M7 8a4 4 0 1 0 0 8h10a4 4 0 1 0 0-8 4 4 0 0 0-3.2 1.6L10.2 14.4A4 4 0 0 1 7 16"/>
      </svg>
    </div>
  )
}

// Primary CTA button.
export function PrimaryButton({ children, large, glow, icon, onClick, disabled, style = {} }) {
  return (
    <button onClick={onClick} disabled={disabled} style={{
      appearance: 'none', border: 0, cursor: disabled ? 'default' : 'pointer',
      height: large ? 44 : 36,
      padding: large ? '0 24px' : '0 16px',
      borderRadius: 8,
      background: 'linear-gradient(180deg, #7376f4, #5a5ee3)',
      color: '#fff', fontFamily: 'inherit', fontSize: large ? 15 : 13, fontWeight: 600,
      letterSpacing: 0.1,
      opacity: disabled ? 0.5 : 1,
      boxShadow: glow
        ? 'inset 0 1px 0 rgba(255,255,255,0.25), inset 0 -1px 0 rgba(0,0,0,0.2), 0 0 0 1px rgba(99,102,241,0.6), 0 8px 28px -4px rgba(99,102,241,0.55), 0 0 40px -4px rgba(99,102,241,0.45)'
        : 'inset 0 1px 0 rgba(255,255,255,0.18), inset 0 -1px 0 rgba(0,0,0,0.18), 0 4px 12px rgba(99,102,241,0.25)',
      display: 'inline-flex', alignItems: 'center', gap: 8,
      ...style,
    }}>
      {icon}{children}
    </button>
  )
}

// Ghost / secondary button.
export function GhostButton({ children, icon, danger, onClick, style = {} }) {
  return (
    <button onClick={onClick} style={{
      appearance: 'none', cursor: 'pointer',
      height: 32, padding: '0 12px', borderRadius: 8,
      background: 'transparent',
      border: `1px solid ${T.border}`,
      color: danger ? T.error : T.text,
      fontFamily: 'inherit', fontSize: 12.5, fontWeight: 500,
      display: 'inline-flex', alignItems: 'center', gap: 6,
      ...style,
    }}>{icon}{children}</button>
  )
}

// Card with engraved double-edge.
export function Card({ children, style = {}, pad = 16 }) {
  return (
    <div style={{
      background: T.surface, borderRadius: 12, padding: pad,
      boxShadow: `inset 0 1px 0 ${T.borderHi}, inset 0 -1px 0 #0c0d12, 0 0 0 1px ${T.border}`,
      ...style,
    }}>{children}</div>
  )
}

// Connected green dot with glow.
export function LiveDot({ size = 8 }) {
  return (
    <span style={{
      display: 'inline-block', width: size, height: size, borderRadius: '50%',
      background: T.success,
      boxShadow: '0 0 0 3px rgba(52,211,153,0.18), 0 0 8px rgba(52,211,153,0.55)',
      animation: 'pulse-dot 2.2s ease-in-out infinite',
    }}/>
  )
}

// Segmented tab control.
export function Segment({ items, active, onChange }) {
  return (
    <div style={{
      display: 'inline-flex', padding: 3, borderRadius: 9,
      background: '#0a0b10',
      boxShadow: `inset 0 0 0 1px ${T.border}`,
      gap: 2,
    }}>
      {items.map(it => {
        const on = it.id === active
        return (
          <button key={it.id} onClick={() => onChange?.(it.id)} style={{
            appearance: 'none', border: 0, cursor: 'pointer',
            height: 28, padding: '0 14px', borderRadius: 7,
            background: on ? 'linear-gradient(180deg, #20232c, #181a22)' : 'transparent',
            boxShadow: on ? `inset 0 1px 0 ${T.borderHi}, 0 0 0 1px ${T.border}` : 'none',
            color: on ? T.heading : T.textDim,
            fontFamily: 'inherit', fontSize: 12.5, fontWeight: on ? 600 : 500,
            display: 'inline-flex', alignItems: 'center', gap: 6,
          }}>
            {it.icon}{it.label}
          </button>
        )
      })}
    </div>
  )
}

// Toggle switch.
export function Toggle({ on, onChange }) {
  return (
    <div onClick={onChange} style={{
      width: 32, height: 18, borderRadius: 999, cursor: 'pointer',
      background: on ? T.indigo : '#22242e',
      boxShadow: on ? '0 0 0 1px rgba(99,102,241,0.55), 0 0 14px -2px rgba(99,102,241,0.55)' : `inset 0 0 0 1px ${T.border}`,
      position: 'relative', transition: 'all 0.2s',
    }}>
      <span style={{
        position: 'absolute', top: 2, left: on ? 16 : 2,
        width: 14, height: 14, borderRadius: '50%',
        background: '#fff', boxShadow: '0 1px 3px rgba(0,0,0,0.4)',
        transition: 'left 0.18s',
      }}/>
    </div>
  )
}

// Stat card for the display tab.
export function Stat({ label, value, mono, icon }) {
  return (
    <div style={{
      background: T.surface, borderRadius: 10, padding: '10px 14px',
      boxShadow: `inset 0 1px 0 ${T.borderHi}, 0 0 0 1px ${T.border}`,
      display: 'flex', flexDirection: 'column', gap: 4, justifyContent: 'center', minWidth: 0,
    }}>
      <div style={{ fontSize: 10.5, color: T.textDim, textTransform: 'uppercase', letterSpacing: 1, display: 'flex', alignItems: 'center', gap: 6 }}>
        {label}{icon && <span style={{ color: T.textDim }}>{icon}</span>}
      </div>
      <div className={mono ? 'mono' : undefined} style={{
        fontSize: typeof value === 'string' ? 13 : 14, color: T.heading, fontWeight: 500,
        whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
      }}>{value}</div>
    </div>
  )
}

// Quality preset card.
export function QualityCard({ name, sub, active, onClick }) {
  return (
    <div onClick={onClick} style={{
      padding: '12px 12px', borderRadius: 10, cursor: 'pointer',
      background: active ? 'rgba(99,102,241,0.1)' : T.surface2,
      boxShadow: active
        ? `inset 0 0 0 1.5px ${T.indigo}, 0 0 24px -8px rgba(99,102,241,0.4)`
        : `inset 0 0 0 1px ${T.border}`,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, color: active ? T.indigo2 : T.heading, fontSize: 13, fontWeight: 600 }}>
        {name}
        {active && <span style={{ marginLeft: 'auto' }}>{Icons.check(14, { color: T.indigo2 })}</span>}
      </div>
      <div className="mono" style={{ color: T.textDim, fontSize: 10.5, marginTop: 4 }}>{sub}</div>
    </div>
  )
}

// Toast notification.
export function Toast({ kind = 'info', children, onDone }) {
  const cfg = {
    info: { c: T.indigo, glow: 'rgba(99,102,241,0.45)' },
    success: { c: T.success, glow: 'rgba(52,211,153,0.45)' },
    error: { c: T.error, glow: 'rgba(239,68,68,0.45)' },
  }[kind]

  React.useEffect(() => {
    if (onDone) {
      const t = setTimeout(onDone, 2500)
      return () => clearTimeout(t)
    }
  }, [onDone])

  return (
    <div style={{
      display: 'inline-flex', alignItems: 'center', gap: 10,
      padding: '10px 16px 10px 14px', borderRadius: 999,
      background: 'rgba(20,22,28,0.92)',
      backdropFilter: 'blur(20px) saturate(180%)', WebkitBackdropFilter: 'blur(20px) saturate(180%)',
      boxShadow: `inset 0 0 0 1px ${T.borderHi}, 0 12px 32px rgba(0,0,0,0.55)`,
      color: T.heading, fontSize: 13, fontWeight: 500,
      animation: 'toast-in 0.25s ease-out',
      position: 'fixed', bottom: 22, left: '50%', transform: 'translateX(-50%)', zIndex: 200,
    }}>
      <span style={{
        width: 8, height: 8, borderRadius: '50%',
        background: cfg.c, boxShadow: `0 0 0 3px ${cfg.glow}`,
        flexShrink: 0,
      }}/>
      {children}
    </div>
  )
}
