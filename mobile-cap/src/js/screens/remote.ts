'use strict';
// ── Remote trackpad ──
const trackpad = $('trackpad') as HTMLElement, trackpadHint = $('trackpad-hint') as HTMLElement;
let tpLastX = 0, tpLastY = 0, tpFingers = 0, tpMoved = false, tpStartT = 0;

// Transport gate. The AOA wire protocol today carries video and touch
// only — no mouse / scroll / key frames — so every Remote-tab input
// here would silently no-op when the user is connected over USB. Until
// we extend internal/usb/protocol.go with FrameMouse/FrameScroll/FrameKey
// (tracked TODO), we surface the limitation as a banner the first time
// the user opens the Remote tab on a USB session, plus a no-op + flash
// on every interaction so the dead clicks aren't mysterious.
function viorTransport(): 'wifi' | 'usb' {
  const fn = (window as unknown as { viorTransport?: () => 'wifi' | 'usb' }).viorTransport;
  return typeof fn === 'function' ? fn() : 'wifi';
}
let usbRemoteHintShown = false;
function maybeWarnUsbRemote(): boolean {
  if (viorTransport() !== 'usb') return false;
  if (!usbRemoteHintShown) {
    usbRemoteHintShown = true;
    toast('warning', 'Remote needs Wi-Fi', 'Cable transport carries display only — use Wi-Fi for trackpad and keys.');
  }
  return true;
}
function wsSend(obj: unknown): void {
  if (maybeWarnUsbRemote()) return;
  if (ws && ws.readyState === 1) ws.send(JSON.stringify(obj));
}
interface FlashFn { (msg: string): void; _t?: ReturnType<typeof setTimeout>; }
const flash: FlashFn = function (msg: string): void {
  const p = $('flash-pill') as HTMLElement;
  p.textContent = msg + ' sent';
  p.classList.remove('hidden');
  clearTimeout(flash._t);
  flash._t = setTimeout(function (): void { p.classList.add('hidden'); }, 900);
};
trackpad.addEventListener('touchstart', function (e: TouchEvent): void {
  e.preventDefault();
  tpFingers = e.touches.length; tpMoved = false; tpStartT = Date.now();
  const t = e.touches[0]; tpLastX = t.clientX; tpLastY = t.clientY;
  trackpadHint.style.display = 'none';
}, { passive: false });
trackpad.addEventListener('touchmove', function (e: TouchEvent): void {
  e.preventDefault();
  if (maybeWarnUsbRemote()) return;
  const t = e.touches[0];
  const dx = t.clientX - tpLastX, dy = t.clientY - tpLastY;
  tpLastX = t.clientX; tpLastY = t.clientY;
  if (Math.abs(dx) + Math.abs(dy) > 2) tpMoved = true;
  if (e.touches.length >= 2) wsSend({ type: 'input', data: { event: 'scroll', dx: Math.round(dx / 4), dy: Math.round(dy / 4) } });
  else wsSend({ type: 'input', data: { event: 'mouse', action: 'move', dx: dx * 2, dy: dy * 2 } });
}, { passive: false });
trackpad.addEventListener('touchend', function (e: TouchEvent): void {
  e.preventDefault();
  if (maybeWarnUsbRemote()) return;
  const dur = Date.now() - tpStartT;
  if (!tpMoved && dur < 300) {
    const action = tpFingers >= 2 ? 'rightclick' : 'click';
    wsSend({ type: 'input', data: { event: 'mouse', action: action } });
    flash(action === 'rightclick' ? 'Right click' : 'Click');
  }
  tpFingers = 0;
  trackpadHint.style.display = '';
}, { passive: false });

// Scroll strip
const ss = $('scroll-strip') as HTMLElement;
let ssDown = false, ssY = 0;
ss.addEventListener('touchstart', function (e: TouchEvent): void { e.preventDefault(); ssDown = true; ssY = e.touches[0].clientY; ss.classList.add('active'); }, { passive: false });
ss.addEventListener('touchmove', function (e: TouchEvent): void {
  e.preventDefault();
  if (!ssDown) return;
  const y = e.touches[0].clientY, dy = y - ssY;
  if (Math.abs(dy) > 24) { ssY = y; wsSend({ type: 'input', data: { event: 'scroll', dx: 0, dy: dy > 0 ? 3 : -3 } }); flash('Scroll'); }
}, { passive: false });
ss.addEventListener('touchend', function (e: TouchEvent): void { e.preventDefault(); ssDown = false; ss.classList.remove('active'); }, { passive: false });

($('click-btn') as HTMLElement).addEventListener('click', function (): void { if (maybeWarnUsbRemote()) return; wsSend({ type: 'input', data: { event: 'mouse', action: 'click' } }); flash('Click'); });
($('rclick-btn') as HTMLElement).addEventListener('click', function (): void { if (maybeWarnUsbRemote()) return; wsSend({ type: 'input', data: { event: 'mouse', action: 'rightclick' } }); flash('Right click'); });

// Remote view toggle
($('remote-view-trackpad') as HTMLElement).addEventListener('click', function (): void {
  ($('remote-view-trackpad') as HTMLElement).classList.add('active'); ($('remote-view-keys') as HTMLElement).classList.remove('active');
  ($('remote-trackpad-body') as HTMLElement).classList.remove('hidden'); ($('remote-keys-body') as HTMLElement).classList.add('hidden');
});
($('remote-view-keys') as HTMLElement).addEventListener('click', function (): void {
  ($('remote-view-trackpad') as HTMLElement).classList.remove('active'); ($('remote-view-keys') as HTMLElement).classList.add('active');
  ($('remote-trackpad-body') as HTMLElement).classList.add('hidden'); ($('remote-keys-body') as HTMLElement).classList.remove('hidden');
});

// Shortcuts grid
type Shortcut = [string, string, string, string];
const SHORTCUTS: Shortcut[] = [
  ['Cmd+c', 'Copy', '⌘C', 'copy'], ['Cmd+v', 'Paste', '⌘V', 'paste'],
  ['Cmd+x', 'Cut', '⌘X', 'cut'], ['Cmd+z', 'Undo', '⌘Z', 'undo'],
  ['Cmd+Shift+z', 'Redo', '⇧⌘Z', 'redo'], ['Cmd+a', 'Select All', '⌘A', 'layers'],
  ['Cmd+s', 'Save', '⌘S', 'save'], ['Cmd+f', 'Find', '⌘F', 'search'],
  ['Cmd+Tab', 'App ⇆', '⌘⇥', 'swap'], ['Cmd+`', 'Window ⇆', '⌘`', 'window'],
  ['Cmd+Space', 'Spotlight', '⌘Sp', 'spotlight'], ['Cmd+q', 'Quit', '⌘Q', 'quit'],
];
const ICONS: Record<string, string> = {
  copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
  paste: '<rect x="5" y="5" width="14" height="16" rx="2"/><rect x="9" y="2.5" width="6" height="3.5" rx="1.2"/>',
  cut: '<circle cx="6.5" cy="17" r="2.5"/><circle cx="6.5" cy="7" r="2.5"/><path d="M8.7 8.5L20 17M8.7 15.5L20 7"/>',
  undo: '<path d="M9 7L4.5 11.5 9 16"/><path d="M4.5 11.5H14a5 5 0 0 1 0 10h-1.5"/>',
  redo: '<path d="M15 7l4.5 4.5L15 16"/><path d="M19.5 11.5H10a5 5 0 0 0 0 10h1.5"/>',
  layers: '<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/>',
  save: '<path d="M5 4h11l3 3v13H5z"/><path d="M8 4v5h7V4M9 13h6v7H9z"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="M15.5 15.5L21 21"/>',
  swap: '<path d="M4 8h13M14 5l3 3-3 3"/><path d="M20 16H7M10 13l-3 3 3 3"/>',
  window: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18"/>',
  spotlight: '<circle cx="11" cy="11" r="6.5"/><path d="M15.6 15.6L21 21"/><circle cx="11" cy="11" r="2.3"/>',
  quit: '<path d="M14 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 12H3M6 8.5L2.5 12 6 15.5"/>',
};
const grid = $('shortcut-grid') as HTMLElement;
SHORTCUTS.forEach(function (s: Shortcut): void {
  const b = document.createElement('button');
  b.className = 'keycap';
  b.dataset.key = s[0];
  b.setAttribute('aria-label', s[1] + ' key');
  b.innerHTML =
    '<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">' + (ICONS[s[3]] || '') + '</svg>' +
    '<span class="keycap-label keycap-label-icon">' + s[1] + '</span>' +
    '<span class="keycap-sub">' + s[2] + '</span>';
  grid.appendChild(b);
});

// F-keys
const fkeyGrid = $('fkey-grid') as HTMLElement;
['F1', 'F2', 'F3', 'F4', 'F5', 'F6', 'F11', 'F12'].forEach(function (f: string): void {
  const b = document.createElement('button');
  b.className = 'keycap'; b.dataset.key = f;
  b.setAttribute('aria-label', f + ' key');
  b.innerHTML = '<span class="keycap-label">' + f + '</span>';
  fkeyGrid.appendChild(b);
});

// Wire all keycap clicks
document.querySelectorAll<HTMLElement>('.keycap').forEach(function (k: HTMLElement): void {
  let fired = false;
  function send(): void {
    const key = k.dataset.key; if (!key) return;
    wsSend({ type: 'input', data: { event: 'key', key: key } });
    const label = k.querySelector('.keycap-label'); flash(label ? (label.textContent || key) : key);
  }
  k.addEventListener('touchend', function (e: TouchEvent): void { e.preventDefault(); fired = true; send(); setTimeout(function (): void { fired = false; }, 400); }, { passive: false });
  k.addEventListener('click', function (): void { if (!fired) send(); });
});

// Soft keyboard
const kbInput = $('kb-input') as HTMLInputElement;
($('kb-btn') as HTMLElement).addEventListener('click', function (): void {
  kbInput.value = ''; kbInput.focus();
  toast('info', 'Keyboard ready', 'Type to forward keys.');
});
kbInput.addEventListener('input', function (e: Event): void {
  const data = (e as InputEvent).data;
  if (data) { for (let i = 0; i < data.length; i++) wsSend({ type: 'input', data: { event: 'key', key: data[i] } }); }
  kbInput.value = '';
});
kbInput.addEventListener('keydown', function (e: KeyboardEvent): void {
  const map: Record<string, string> = { 'Backspace': 'BackSpace', 'Enter': 'Return', 'Tab': 'Tab', 'ArrowUp': 'Up', 'ArrowDown': 'Down', 'ArrowLeft': 'Left', 'ArrowRight': 'Right', 'Escape': 'Escape' };
  const k = map[e.key];
  if (k) { e.preventDefault(); wsSend({ type: 'input', data: { event: 'key', key: k } }); }
});
