import React, { useState } from 'react'
import { motion } from 'framer-motion'
import { PrimaryButton } from '../design/Primitives'
import { Icons } from '../design/Icons'

const pageFade = {
  initial: { opacity: 0, y: 20 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.5, ease: [0.25, 0.1, 0.25, 1] } },
  exit: { opacity: 0, y: -10, transition: { duration: 0.2 } }
}

function HeroIllustration() {
  return (
    <motion.div
      style={{ position: 'relative', width: 480, height: 200 }}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.6 }}
    >
      {/* Subtle ambient glow — single, slow */}
      <div style={{
        position: 'absolute', left: '50%', top: '48%', transform: 'translate(-50%,-50%)',
        width: 340, height: 340, borderRadius: '50%',
        background: 'radial-gradient(circle, rgba(99,102,241,0.18), transparent 65%)',
        filter: 'blur(20px)', pointerEvents: 'none',
      }}/>

      <svg viewBox="0 0 480 200" width="480" height="200" style={{ position: 'relative', display: 'block' }}>
        <defs>
          <linearGradient id="scr" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#1e2038"/><stop offset="1" stopColor="#12142a"/>
          </linearGradient>
          <linearGradient id="phn" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#1a1c2a"/><stop offset="1" stopColor="#0e1018"/>
          </linearGradient>
          <linearGradient id="bm" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0" stopColor="#6366f1" stopOpacity="0"/>
            <stop offset="0.5" stopColor="#818cf8"/>
            <stop offset="1" stopColor="#6366f1" stopOpacity="0"/>
          </linearGradient>
        </defs>

        {/* ── Monitor ────────────────────────── */}
        {/* Stand */}
        <rect x="120" y="172" width="36" height="5" rx="2" fill="#16181f"/>
        <rect x="100" y="175" width="76" height="4" rx="2" fill="#16181f"/>
        {/* Body */}
        <rect x="38" y="20" width="200" height="150" rx="10" fill="#0c0d14" stroke="#262833" strokeWidth="1"/>
        {/* Screen */}
        <rect x="46" y="28" width="184" height="128" rx="6" fill="url(#scr)"/>
        {/* Content — clean layout blocks */}
        <rect x="58" y="40" width="52" height="6" rx="2" fill="#6366f1" opacity="0.65"/>
        <rect x="58" y="52" width="100" height="3" rx="1.5" fill="#2e3148" opacity="0.8"/>
        <rect x="58" y="59" width="72" height="3" rx="1.5" fill="#2e3148" opacity="0.6"/>
        <rect x="58" y="72" width="72" height="48" rx="5" fill="#181a2e" stroke="#262833" strokeWidth="0.5"/>
        <rect x="136" y="72" width="72" height="48" rx="5" fill="#181a2e" stroke="#262833" strokeWidth="0.5"/>
        <rect x="58" y="126" width="150" height="22" rx="4" fill="#181a2e" stroke="#262833" strokeWidth="0.5"/>
        {/* Webcam dot */}
        <circle cx="138" cy="24" r="1.5" fill="#2a2d38"/>

        {/* ── Connection beam ────────────────── */}
        {/* Soft glow */}
        <line x1="248" y1="100" x2="322" y2="100" stroke="url(#bm)" strokeWidth="12" strokeLinecap="round" opacity="0.2" filter="url(#blur)"/>
        {/* Core */}
        <line x1="248" y1="100" x2="322" y2="100" stroke="url(#bm)" strokeWidth="1.5" strokeLinecap="round"/>
        {/* Dashes — single subtle animation */}
        <line x1="246" y1="100" x2="324" y2="100" stroke="#818cf8" strokeWidth="1.5" strokeLinecap="round"
          strokeDasharray="3 8" opacity="0.7" style={{ animation: 'beam-flow 1.2s linear infinite' }}/>
        {/* Dots */}
        <circle cx="246" cy="100" r="3" fill="#6366f1"/>
        <circle cx="324" cy="100" r="3" fill="#6366f1"/>

        {/* ── Phone ──────────────────────────── */}
        <rect x="332" y="28" width="72" height="144" rx="14" fill="#0c0d14" stroke="#262833" strokeWidth="1"/>
        {/* Notch */}
        <rect x="356" y="34" width="24" height="5" rx="2.5" fill="#000"/>
        {/* Screen */}
        <rect x="337" y="42" width="62" height="122" rx="9" fill="url(#phn)"/>
        {/* Content */}
        <rect x="344" y="54" width="30" height="4" rx="1.5" fill="#6366f1" opacity="0.65"/>
        <rect x="344" y="63" width="44" height="2.5" rx="1.25" fill="#2e3148" opacity="0.7"/>
        <rect x="344" y="69" width="34" height="2.5" rx="1.25" fill="#2e3148" opacity="0.5"/>
        <rect x="344" y="80" width="48" height="32" rx="4" fill="#181a2e" stroke="#262833" strokeWidth="0.5"/>
        <rect x="344" y="117" width="48" height="18" rx="4" fill="#181a2e" stroke="#262833" strokeWidth="0.5"/>
        <rect x="344" y="140" width="48" height="16" rx="4" fill="#181a2e" stroke="#262833" strokeWidth="0.5"/>
        {/* Home indicator */}
        <rect x="355" y="166" width="26" height="2" rx="1" fill="#fff" opacity="0.15"/>

        {/* Blur filter */}
        <defs>
          <filter id="blur"><feGaussianBlur stdDeviation="4"/></filter>
        </defs>
      </svg>
    </motion.div>
  )
}

export default function IdleView({ onStart }) {
  const [loading, setLoading] = useState(false)
  const handleClick = async () => {
    setLoading(true)
    try { await onStart() } catch {}
    setLoading(false)
  }

  return (
    <motion.div className="idle-center" variants={pageFade} initial="initial" animate="animate" exit="exit">
      <HeroIllustration/>
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.25, duration: 0.45 }}
      >
        <h1 className="idle-heading">Mirror your phone on this Mac</h1>
        <p className="idle-sub">
          Start a local server, scan the QR on your phone, and stream — no cable, no cloud.
        </p>
      </motion.div>
      <motion.div
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.45, duration: 0.4 }}
      >
        <PrimaryButton large glow icon={Icons.play(14)} onClick={handleClick} disabled={loading}>
          {loading ? 'Starting…' : 'Start Server'}
        </PrimaryButton>
      </motion.div>
    </motion.div>
  )
}
