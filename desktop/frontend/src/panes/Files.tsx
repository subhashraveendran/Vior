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

export default function FilesPane({ onSendFile, client }: Props) {
  const [over, setOver] = useState<boolean>(false)
  const [files, setFiles] = useState<FileItem[]>([])
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
        <span className="drop-icon">{Icons.download(22)}</span>
        <div className="drop-title">Drop files to send to {client?.name || 'device'}</div>
        <div className="drop-sub">or <b onClick={onSendFile}>browse</b> — up to 2 GB per file</div>
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
              return (
                <div key={f.id} className="card file-row">
                  <span className="file-icon">{Icons.file(16)}</span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="file-name">{f.name}</div>
                    <div className="file-meta">
                      {f.done
                        ? `${f.size} bytes · ${f.kind === 'in' ? 'received' : 'sent'}`
                        : `${f.transferred} / ${f.size} bytes · ${pct}%`}
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
                  {f.done && <span style={{ color: 'var(--ok)' }}>{Icons.check(17)}</span>}
                </div>
              )
            })}
          </div>}
    </div>
  )
}
