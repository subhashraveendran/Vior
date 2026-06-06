// Vior brand glyph built from three positioned spans. Scales by `size`
// via a unit ratio so it stays crisp at 15 px (titlebar) and 44 px
// (idle radar core) without needing a separate SVG.
import React from 'react'

export default function Glyph({ size = 24 }) {
  const u = size / 24
  return (
    <span className="glyph" style={{ width: size, height: size }}>
      <span className="g-screen" style={{ left: 0, top: 2 * u, width: 18 * u, height: 13 * u, borderWidth: Math.max(1.5, 2 * u), borderRadius: 3.5 * u }} />
      <span className="g-stand" style={{ left: 6 * u, top: 16 * u, width: 6 * u, height: Math.max(1.5, 2 * u), borderRadius: 1 * u }} />
      <span className="g-phone" style={{ right: 0, bottom: 0, width: 9.5 * u, height: 14.5 * u, borderRadius: 3 * u }} />
    </span>
  )
}
