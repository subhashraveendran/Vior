// Toast host — renders the queue from App.jsx. The host is dumb; App
// owns the toasts array and the addToast hook. Keeps state in one
// place so the same toast can fire from any screen.
import React from 'react'
import { Icons } from './icons'

export type ToastTone = 'success' | 'warning' | 'error' | 'info'
export type Toast = { id: number; tone: ToastTone; title: string; msg?: string | null }
type ToastHostProps = { toasts: Toast[]; onClose: (id: number) => void }

export default function ToastHost({ toasts, onClose }: ToastHostProps): React.JSX.Element {
  return (
    <div className="toast-host">
      {toasts.map(t => (
        <div key={t.id} className="toast">
          <span className={`dot ${t.tone === 'success' ? 'dot-ok' : t.tone === 'warning' ? 'dot-warn' : t.tone === 'error' ? 'dot-err' : 'dot-idle'}`} style={{ marginTop: 5 }} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="toast-title">{t.title}</div>
            {t.msg && <div className="toast-msg">{t.msg}</div>}
          </div>
          <button onClick={() => onClose(t.id)} style={{ background: 'none', border: 'none', color: 'var(--text-3)', cursor: 'pointer', padding: 2 }}>{Icons.close(15)}</button>
        </div>
      ))}
    </div>
  )
}
