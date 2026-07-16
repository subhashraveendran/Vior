// useFocusTrap — a small accessibility hook for modal dialogs.
//
// When `active` is true it:
//   • moves focus into the dialog on open (first focusable element, or
//     the container itself),
//   • traps Tab / Shift+Tab so focus cycles within the dialog instead of
//     escaping to the page behind the backdrop,
//   • calls `onClose` when Escape is pressed,
//   • restores focus to the element that was focused before the dialog
//     opened when it unmounts / deactivates.
//
// Attach the returned ref to the dialog container element.
import { useEffect, useRef } from 'react'

const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'textarea:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

export function useFocusTrap<T extends HTMLElement = HTMLDivElement>(
  active: boolean,
  onClose?: () => void,
): React.RefObject<T | null> {
  const ref = useRef<T | null>(null)

  useEffect(() => {
    if (!active) return
    const node = ref.current
    if (!node) return

    // Remember what had focus so we can restore it on close.
    const prevFocused = document.activeElement as HTMLElement | null

    const focusables = (): HTMLElement[] =>
      Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        el => el.offsetParent !== null || el === document.activeElement,
      )

    // Move focus in — first focusable child, else the container itself.
    const first = focusables()[0]
    if (first) {
      first.focus()
    } else {
      node.setAttribute('tabindex', '-1')
      node.focus()
    }

    const onKeyDown = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose?.()
        return
      }
      if (e.key !== 'Tab') return
      const items = focusables()
      if (items.length === 0) {
        e.preventDefault()
        return
      }
      const firstEl = items[0]!
      const lastEl = items[items.length - 1]!
      const activeEl = document.activeElement
      if (e.shiftKey) {
        if (activeEl === firstEl || !node.contains(activeEl)) {
          e.preventDefault()
          lastEl.focus()
        }
      } else {
        if (activeEl === lastEl || !node.contains(activeEl)) {
          e.preventDefault()
          firstEl.focus()
        }
      }
    }

    document.addEventListener('keydown', onKeyDown, true)
    return () => {
      document.removeEventListener('keydown', onKeyDown, true)
      // Restore focus to the previously-focused element on close.
      if (prevFocused && typeof prevFocused.focus === 'function') {
        prevFocused.focus()
      }
    }
  }, [active, onClose])

  return ref
}
