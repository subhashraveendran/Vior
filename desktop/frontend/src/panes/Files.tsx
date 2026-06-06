// Files pane — drop zone + receive history. Subscribes to the
// 'file:received' Wails event to append incoming files. Kept as its
// own pane so the Connected screen stays focused on display + mode.
import { useState, useEffect } from 'react'
import type { DragEvent } from 'react'
import { EventsOn } from '../lib/api'
import { Icons } from '../lib/icons'
import type { ClientInfo } from '../types'

interface FileItem {
  id: string
  name: string
  size: number
  kind: 'in' | 'out'
  done: boolean
}

interface ReceivedFile {
  id: string
  name: string
  size: number
}

interface Props {
  onSendFile: () => void
  client: ClientInfo | null
}

export default function FilesPane({ onSendFile, client }: Props) {
  const [over, setOver] = useState<boolean>(false)
  const [files, setFiles] = useState<FileItem[]>([])
  useEffect(() => {
    const off = EventsOn('file:received', (f: ReceivedFile) => {
      setFiles(fs => [{ id: f.id, name: f.name, size: f.size, kind: 'in', done: true }, ...fs])
    })
    return () => { if (off) off() }
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
      <div className="label" style={{ marginBottom: 12 }}>Recent</div>
      {files.length === 0
        ? <div style={{ padding: '24px 0', textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>No transfers yet.</div>
        : <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {files.map(f => (
              <div key={f.id} className="card file-row">
                <span className="file-icon">{Icons.file(16)}</span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="file-name">{f.name}</div>
                  <div className="file-meta">{f.size} bytes · {f.kind === 'in' ? 'received' : 'sent'}</div>
                </div>
                <span style={{ color: 'var(--ok)' }}>{Icons.check(17)}</span>
              </div>
            ))}
          </div>}
    </div>
  )
}
