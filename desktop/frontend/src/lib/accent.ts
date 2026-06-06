// Accent presets + the applyAccent side-effect. Keeping the global
// CSS-variable write here (rather than scattered in components) means
// every theme write goes through a single, tested place.

import type { AccentHex, AccentPreset } from '../types'

export const ACCENTS: AccentPreset[] = [
  { hex: '#ff8a4c', on: '#1a0e06', weak: 'rgba(255,138,76,0.14)', line: 'rgba(255,138,76,0.40)', name: 'Orange' },
  { hex: '#4cc2ff', on: '#06121a', weak: 'rgba(76,194,255,0.14)', line: 'rgba(76,194,255,0.40)', name: 'Blue' },
  { hex: '#46d39a', on: '#06140e', weak: 'rgba(70,211,154,0.14)', line: 'rgba(70,211,154,0.40)', name: 'Green' },
  { hex: '#e8e8ea', on: '#0b0d10', weak: 'rgba(232,232,234,0.14)', line: 'rgba(232,232,234,0.40)', name: 'White' },
]

export function applyAccent(hex: AccentHex): void {
  const p = ACCENTS.find(a => a.hex === hex) || ACCENTS[0]
  const r = document.documentElement.style
  r.setProperty('--accent', p.hex)
  r.setProperty('--accent-2', p.hex)
  r.setProperty('--on-accent', p.on)
  r.setProperty('--accent-weak', p.weak)
  r.setProperty('--accent-line', p.line)
  localStorage.setItem('vior_accent', p.hex)
}

export const accentName = (hex: AccentHex | null | undefined): string =>
  (ACCENTS.find(a => a.hex.toLowerCase() === (hex || '').toLowerCase())?.name) || 'Custom'
