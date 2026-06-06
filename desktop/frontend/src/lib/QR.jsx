// Deterministic decorative QR. NOT a real QR — purely a stylised grid
// derived from a seed string (FNV-1a + xorshift), with three finder
// squares stamped at the corners. Looks like a QR at a glance and
// changes per server URL so each session feels unique.
import React from 'react'

const qrCache = {}
function qrMatrix(seed = 'vior', n = 25) {
  if (qrCache[seed + n]) return qrCache[seed + n]
  let h = 2166136261
  for (let i = 0; i < seed.length; i++) { h ^= seed.charCodeAt(i); h = Math.imul(h, 16777619) }
  const rnd = () => { h ^= h << 13; h ^= h >>> 17; h ^= h << 5; return ((h >>> 0) % 1000) / 1000 }
  const m = Array.from({ length: n }, () => Array(n).fill(false))
  const finder = (r, c) => {
    for (let i = -1; i <= 7; i++) for (let j = -1; j <= 7; j++) {
      const rr = r + i, cc = c + j; if (rr < 0 || cc < 0 || rr >= n || cc >= n) continue
      const edge = i === 0 || i === 6 || j === 0 || j === 6
      const core = i >= 2 && i <= 4 && j >= 2 && j <= 4
      m[rr][cc] = (i >= 0 && i <= 6 && j >= 0 && j <= 6) ? (edge || core) : false
    }
  }
  for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) m[r][c] = rnd() > 0.5
  finder(0, 0); finder(0, n - 7); finder(n - 7, 0)
  qrCache[seed + n] = m
  return m
}

export default function QR({ size = 196, seed = 'vior' }) {
  const n = 25, m = qrMatrix(seed, n)
  const inner = size - 28, cell = inner / n
  return (
    <div className="qr-box" style={{ width: size, height: size }}>
      <div style={{ display: 'grid', gridTemplateColumns: `repeat(${n}, ${cell}px)`, gridTemplateRows: `repeat(${n}, ${cell}px)` }}>
        {m.flatMap((row, r) => row.map((on, c) => (
          <div key={r + '-' + c} style={{ background: on ? '#0b0d10' : 'transparent' }} />
        )))}
      </div>
    </div>
  )
}
