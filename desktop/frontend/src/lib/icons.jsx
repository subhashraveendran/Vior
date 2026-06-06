// Icon registry. Each entry is a factory: `Icons.foo(size)` returns the
// JSX <svg> element. Keeps the call sites compact (`{Icons.power(20)}`)
// without forcing every component to import a dozen named symbols.
import React from 'react'

const I = (p) => (size = 18) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
    stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"
    dangerouslySetInnerHTML={{ __html: p }} />
)

export const Icons = {
  display: I('<rect x="2.5" y="4" width="19" height="13" rx="2"/><path d="M8 21h8M12 17v4"/>'),
  files:   I('<path d="M3 7.5a2 2 0 0 1 2-2h4l2 2.2h6a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>'),
  remote:  I('<path d="M5.5 3.2l13.6 7.2a.7.7 0 0 1-.1 1.27l-5.3 1.9-2.4 5.1a.7.7 0 0 1-1.3-.06z"/>'),
  settings:I('<circle cx="12" cy="12" r="3.2"/><path d="M12 3v2.2M12 18.8V21M21 12h-2.2M5.2 12H3M18.4 5.6l-1.6 1.6M7.2 16.8l-1.6 1.6M18.4 18.4l-1.6-1.6M7.2 7.2L5.6 5.6"/>'),
  power:   I('<path d="M12 3v8"/><path d="M6.5 7a8 8 0 1 0 11 0"/>'),
  link:    I('<path d="M9.5 14.5l5-5"/><path d="M8 11l-2 2a3.2 3.2 0 0 0 4.5 4.5l2-2"/><path d="M16 13l2-2A3.2 3.2 0 0 0 13.5 6.5l-2 2"/>'),
  copy:    I('<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>'),
  usb:     I('<circle cx="12" cy="20" r="1.4" fill="currentColor" stroke="none"/><path d="M12 18.6V4M12 4l-2.4 2.6M12 4l2.4 2.6"/><path d="M7 11l2 1.4v2M17 9l-2 1.4v4"/>'),
  refresh: I('<path d="M20 11a8 8 0 1 0-1.5 5.5"/><path d="M20 5v5h-5"/>'),
  close:   I('<path d="M6 6l12 12M18 6L6 18"/>'),
  check:   I('<path d="M4.5 12.5l4.5 4.5L19.5 6.5"/>'),
  arrowR:  I('<path d="M5 12h14M13 6l6 6-6 6"/>'),
  layers:  I('<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/>'),
  download:I('<path d="M12 4v11M7 11l5 5 5-5"/><path d="M5 20h14"/>'),
  monitor2:I('<rect x="2.5" y="4.5" width="19" height="12.5" rx="1.6"/><path d="M2.5 13.5h19"/><path d="M9 21h6"/>'),
  remote2: I('<rect x="2.5" y="4" width="19" height="13" rx="2"/><path d="M8 21h8M12 17v4"/>'),
  shield:  I('<path d="M12 3l7 2.5v5c0 4.5-3 8.2-7 9.5-4-1.3-7-5-7-9.5v-5z"/><path d="M9 12l2 2 4-4"/>'),
  alert:   I('<path d="M12 3.5L21.5 20H2.5z"/><path d="M12 10v4.5M12 17.5h.01"/>'),
  file:    I('<path d="M6 3h7l5 5v13H6z"/><path d="M13 3v5h5"/>'),
  photo:   I('<rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="8.5" cy="10" r="1.6"/><path d="M5 17l4.5-4 3 2.6L16 12l3 3.2"/>'),
}
