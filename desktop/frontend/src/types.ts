// Shared types used across screens, panes and lib helpers.
// Wails-generated classes in ../wailsjs/go/models live behind
// re-exports here so screens import every contract from one place.
// Component prop interfaces are exported so each per-screen agent
// can import the canonical shape without re-deriving it.

import type { main } from '../wailsjs/go/models'

// ─── Wails domain re-exports ────────────────────────────────────────
// ServerStatus is augmented with a UI-side `frameRate` that the
// Connected screen reads. The Go side returns 0 today; Connected
// falls back to 30 via `?? 30`. Promote to a real Go field on the
// next protocol bump.
export type ServerStatus = main.ServerStatus & { frameRate?: number }
export type ClientInfo = main.ClientInfo
export type AppConfig = main.AppConfig
export type DisplayInfo = main.DisplayInfo
export type StreamConfig = main.StreamConfig
export type StreamStatus = main.StreamStatus
export type VirtualDisplayConfig = main.VirtualDisplayConfig

// ─── UI primitives ──────────────────────────────────────────────────
export type ToastTone = 'success' | 'info' | 'warning' | 'error'

export interface Toast {
  id: number
  tone: ToastTone
  title: string
  msg: string | null
}

export type Mode = 'extend' | 'mirror'
export type Nav = 'server' | 'files' | 'settings'
export type AccentHex = string

// ─── Accent palette entry ───────────────────────────────────────────
export interface AccentPreset {
  hex: AccentHex
  on: string
  weak: string
  line: string
  name: string
}

// ─── Component prop contracts ───────────────────────────────────────

export interface ToastHostProps {
  toasts: Toast[]
  onClose: (id: number) => void
}

export interface GlyphProps {
  size?: number
}

export interface QRProps {
  seed?: string
  size?: number
}

export interface IdleScreenProps {
  onStart: () => void
  showUpdate: boolean
  onUpdate: () => void
  onDismiss: () => void
}

export interface WaitingScreenProps {
  status: ServerStatus | null
  onStop: () => void
  onCopy: () => void
}

export interface ConnectedScreenProps {
  status: ServerStatus | null
  client: ClientInfo | null
  mode: Mode
  setMode: (m: Mode) => void
  onModeExtend: () => void
  onModeMirror: () => void
  onDisconnect: () => void
  onSendFile: () => void
  errorState: boolean
  onRetry: () => void
  onStop: () => void
  accessibilityOk: boolean | null
  onFixAccessibility: () => void
  showFilesTab: boolean
}

export interface SettingsScreenProps {
  config: AppConfig
  onChange: (c: AppConfig) => void
  accent: AccentHex
  setAccent: (hex: AccentHex) => void
}

export interface AppearancePanelProps {
  accent: AccentHex
  setAccent: (hex: AccentHex) => void
  onClose: () => void
}

export interface PermissionsModalProps {
  onDone: () => void
}

export interface FilesPaneProps {
  onSendFile: () => void
  client: ClientInfo | null
}

// ─── Inbound file event (from 'file:received' Wails event) ──────────
export interface ReceivedFile {
  id: string
  name: string
  size: number
}
