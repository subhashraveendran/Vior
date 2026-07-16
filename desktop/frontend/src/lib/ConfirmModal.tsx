// ConfirmModal — a reusable styled confirmation dialog that replaces
// native window.confirm(). Matches the DisconnectModal styling (uses the
// .error-backdrop / .error-modal shell) and reuses the focus-trap hook
// so Tab stays inside, Escape cancels, and focus is restored on close.
//
// Behaviour mirrors window.confirm: rendering the modal presents the
// choice; clicking Confirm runs onConfirm, Cancel (or Escape/backdrop)
// runs onCancel. The caller controls mount/unmount.
import React from 'react'
import { Icons } from './icons'
import { useFocusTrap } from './useFocusTrap'

interface ConfirmModalProps {
  title: string
  body: React.ReactNode
  confirmLabel?: string
  cancelLabel?: string
  // danger renders the confirm button in the destructive style.
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
  // labelId is the id linking the dialog to its accessible name.
  labelId?: string
}

export default function ConfirmModal({
  title, body, confirmLabel = 'Confirm', cancelLabel = 'Cancel',
  danger = false, onConfirm, onCancel, labelId = 'confirm-modal-title',
}: ConfirmModalProps): React.JSX.Element {
  const ref = useFocusTrap<HTMLDivElement>(true, onCancel)
  return (
    <div className="error-backdrop" onClick={onCancel}>
      <div
        className="card error-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelId}
        ref={ref}
        onClick={e => e.stopPropagation()}
      >
        <span className="error-icon">{Icons.alert(26)}</span>
        <div className="modal-title" id={labelId}>{title}</div>
        <div className="modal-body">{body}</div>
        <div style={{ display: 'flex', gap: 10, marginTop: 22 }}>
          <button className="btn btn-ghost btn-block" onClick={onCancel}>
            {Icons.close(19)} {cancelLabel}
          </button>
          <button
            className={`btn btn-block ${danger ? 'btn-danger' : 'btn-primary'}`}
            onClick={onConfirm}
          >
            {Icons.check(19)} {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
