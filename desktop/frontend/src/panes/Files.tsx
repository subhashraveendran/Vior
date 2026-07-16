// Files pane — drop zone + receive history. Subscribes to:
//   • 'file:progress'  — mid-stream byte counts (coalesced ~once per
//      256 KiB by the Go side) so the bar fills smoothly during a
//      mobile → desktop WS-chunked receive.
//   • 'file:received'  — final completion event, promotes the row to
//      a done state and snapshots the final size.
// Kept as its own pane so the Connected screen stays focused on
// display + mode.
import { useState, useEffect } from 'react'
import type { DragEvent } from 'react'
import { EventsOn } from '../lib/api'
import { Icons } from '../lib/icons'
import type { ClientInfo } from '../types'

interface FileItem {
  id: string
  name: string
  size: number
  // For in-flight receives, transferred bytes so far (clamped to size).
  transferred: number
  kind: 'in' | 'out'
  done: boolean
  // startedAt: epoch ms of the first progress event — used to derive
  // live transfer speed and ETA for the in-flight bar.
  startedAt: number
}

interface ReceivedFile {
  id: string
  name: string
  size: number
}

interface ProgressFile {
  id: string
  name: string
  size: number
  transferred: number
}

interface Props {
  onSendFile: () => void
  client: ClientInfo | null
}

// fmtBytes renders a byte count in the largest sensible unit.
function fmtBytes(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}

// fmtSpeed / fmtEta derive a human "4.2 MB/s · ~12s left" string from
// bytes transferred and elapsed wall-clock time. Returns '' until there's
// enough signal (some bytes + a little elapsed time) to avoid jitter.
function transferRate(transferred: number, size: number, startedAt: number): string {
  const elapsed = (Date.now() - startedAt) / 1000
  if (elapsed < 0.4 || transferred <= 0) return ''
  const bps = transferred / elapsed
  if (bps <= 0) return ''
  const speed = `${fmtBytes(bps)}/s`
  const remaining = Math.max(0, size - transferred)
  if (remaining <= 0 || size <= 0) return speed
  const etaSec = Math.round(remaining / bps)
  const eta = etaSec >= 60 ? `~${Math.floor(etaSec / 60)}m ${etaSec % 60}s left` : `~${etaSec}s left`
  return `${speed} · ${eta}`
}

export default function FilesPane({ onSendFile, client }: Props) {
  const [over, setOver] = useState<boolean>(false)
  const [files, setFiles] = useState<FileItem[]>([])
  // Tick once a second while any transfer is in-flight so the derived
  // speed/ETA re-computes even between coalesced progress events.
  const [, setTick] = useState(0)
  const hasInflight = files.some(f => !f.done)
  useEffect(() => {
    if (!hasInflight) return
    const id = setInterval(() => setTick(n => n + 1), 1000)
    return () => clearInterval(id)
  }, [hasInflight])
  useEffect(() => {
    const offProgress = EventsOn('file:progress', (p: ProgressFile) => {
      // Upsert by ID — first progress event for a transfer creates the
      // row, subsequent ones just bump `transferred` so React only
      // re-renders the bar, not the whole list.
      setFiles(fs => {
        const idx = fs.findIndex(f => f.id === p.id)
        if (idx === -1) {
          return [{
            id: p.id, name: p.name, size: p.size,
            transferred: p.transferred, kind: 'in', done: false,
            startedAt: Date.now(),
          }, ...fs]
        }
        const next = fs.slice()
        next[idx] = { ...next[idx], transferred: p.transferred, size: p.size || next[idx].size }
        return next
      })
    })
    const off = EventsOn('file:received', (f: ReceivedFile) => {
      setFiles(fs => {
        const idx = fs.findIndex(x => x.id === f.id)
        if (idx === -1) {
          return [{
            id: f.id, name: f.name, size: f.size,
            transferred: f.size, kind: 'in', done: true,
            startedAt: Date.now(),
          }, ...fs]
        }
        const next = fs.slice()
        next[idx] = { ...next[idx], size: f.size, transferred: f.size, done: true }
        return next
      })
    })
    return () => {
      if (off) off()
      if (offProgress) offProgress()
    }
  }, [])
  return (
    <div>
      <div className={`drop ${over ? 'over' : ''}`}
        onDragOver={(e: DragEvent<HTMLDivElement>) => { e.preventDefault(); setOver(true) }}
        onDragLeave={() => setOver(false)}
        onDrop={(e: DragEvent<HTMLDivElement>) => { e.preventDefault(); setOver(false); onSendFile() }}
      >
        <span className="drop-icon" aria-hidden="true">{Icons.download(22)}</span>
        <div className="drop-title">Drop files to send to {client?.name || 'device'}</div>
        <div className="drop-sub">or <b onClick={onSendFile} role="button" tabIndex={0} onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSendFile() } }} style={{ cursor: 'pointer', display: 'inline-flex', alignItems: 'center', minHeight: 24, padding: '0 4px' }}>browse</b> — up to 2 GB per file</div>
      </div>
      <button
        className="btn btn-primary"
        style={{ width: '100%', margin: '12px 0' }}
        onClick={onSendFile}
        disabled={!client}
      >
        {Icons.arrowR(18)} Send file to phone
      </button>
      <div className="label" style={{ marginBottom: 12 }}>Recent</div>
      {files.length === 0
        ? <div style={{ padding: '24px 0', textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>No transfers yet.</div>
        : <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {files.map(f => {
              const pct = f.size > 0 ? Math.min(100, Math.round((f.transferred / f.size) * 100)) : 0
              const rate = !f.done ? transferRate(f.transferred, f.size, f.startedAt) : ''
              return (
                <div key={f.id} className="card file-row">
                  <span className="file-icon" aria-hidden="true">{Icons.file(16)}</span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="file-name">{f.name}</div>
                    <div className="file-meta">
                      {f.done
                        ? `${fmtBytes(f.size)} · ${f.kind === 'in' ? 'received' : 'sent'}`
                        : `${fmtBytes(f.transferred)} / ${fmtBytes(f.size)} · ${pct}%${rate ? ` · ${rate}` : ''}`}
                    </div>
                    {!f.done && (
                      <div style={{
                        height: 4, marginTop: 6, borderRadius: 2,
                        background: 'var(--border)', overflow: 'hidden',
                      }}>
                        <div style={{
                          width: pct + '%', height: '100%',
                          background: 'var(--accent)', transition: 'width 120ms linear',
                        }} />
                      </div>
                    )}
                  </div>
                  {f.done && (
                    <span
                      style={{ color: 'var(--ok)', width: 24, height: 24, display: 'grid', placeItems: 'center', flex: 'none' }}
                      role="img"
                      aria-label={f.kind === 'in' ? 'Received' : 'Sent'}
                    >{Icons.check(17)}</span>
                  )}
                </div>
              )
            })}
          </div>}
    </div>
  )
}
