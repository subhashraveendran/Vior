// Core UI primitives: $ helper, shared state vars, toast, connection
// chip, tab switching, and mode-segment binding.
//
// During the TS migration this file still publishes its symbols on
// `globalThis` so the not-yet-converted screen modules keep working.
// The `declare global` block lives in ./types.ts.

// Non-null cast: every $ call site assumes the ID exists in index.html.
// If it doesn't, the runtime error surfaces the typo immediately.
var $ = <T extends HTMLElement = HTMLElement>(id: string): T =>
  document.getElementById(id) as T;

// ── State ──
var ws: WebSocket | null = null;
var displayW = 0, displayH = 0;
var selectedServer: ServerInfo | null = null;
var selectedMode: Mode = 'extend';
var serverName = '', serverPlatform = '', serverRes = '—';
var streamImg = $<HTMLImageElement>('stream-img');
var framePolling = false, frameBaseUrl = '';
var blobUrl: string | null = null;
var overlayTimer: ReturnType<typeof setTimeout> | null = null;
var reconnectAttempts = 0;
var maxReconnect = 5;
var connected = false;
var currentTab: TabName = 'display';
var streamVisible = false, fps = 0, frameCount = 0;
var fpsTimer: ReturnType<typeof setInterval> | null = null;

// Publish to globalThis so legacy .js modules can mutate / read the
// same values. Once every screen is converted to ESM imports we can
// drop this block in one go.

// ── Toasts ──
var toastId = 0;

function toast(tone: ToastTone, title: string, msg?: string | null): void {
  const id = ++toastId;
  toastId = toastId;
  const host = $('toast-host');
  if (!host) return;
  const el = document.createElement('div');
  // Tone modifier (toast-ok / toast-warn / toast-err / toast-idle) drives
  // the per-severity accent border in toast.css so success/error/warn read
  // distinctly without relying on the small colour dot alone.
  const toneCls =
    tone === 'success' ? 'toast-ok'
    : tone === 'warning' ? 'toast-warn'
    : tone === 'error' ? 'toast-err'
    : 'toast-idle';
  el.className = 'toast ' + toneCls;
  el.dataset.id = String(id);
  const dotCls =
    tone === 'success' ? 'dot-ok'
    : tone === 'warning' ? 'dot-warn'
    : tone === 'error' ? 'dot-err'
    : 'dot-idle';
  el.innerHTML =
    '<span class="dot ' + dotCls + '" style="margin-top: 5px;"></span>' +
    '<div style="flex:1;min-width:0;">' +
      '<div class="toast-title">' + esc(title) + '</div>' +
      (msg ? '<div class="toast-msg">' + esc(msg) + '</div>' : '') +
    '</div>';
  host.appendChild(el);
  setTimeout(function () { if (el.parentNode) el.parentNode.removeChild(el); }, 3500);
}

function esc(s: unknown): string {
  const d = document.createElement('div');
  d.textContent = String(s == null ? '' : s);
  return d.innerHTML;
}


// ── Connection chip ──
function setConnState(state: ConnectionState): void {
  const dot = $('conn-dot');
  const label = $('conn-label');
  if (!dot || !label) return;
  dot.className = 'dot';
  if (state === 'online') { dot.classList.add('dot-ok', 'dot-pulse'); label.textContent = 'Connected'; }
  else if (state === 'connecting') { dot.classList.add('dot-warn', 'dot-pulse'); label.textContent = 'Connecting'; }
  else if (state === 'reconnecting') { dot.classList.add('dot-warn', 'dot-pulse'); label.textContent = 'Reconnecting'; }
  else if (state === 'error') { dot.classList.add('dot-err'); label.textContent = 'Disconnected'; }
  else { dot.classList.add('dot-idle'); label.textContent = 'Not connected'; }
}

// ── Tab switch ──
function switchTab(name: TabName): void {
  currentTab = name;
  currentTab = currentTab;
  // Always release camera when navigating away — leaving the scanner
  // hot in the background is what causes the next acquire to fail with
  // NotReadableError. `stopQRScan` is defined later and is idempotent.
  if (typeof stopQRScan === 'function') stopQRScan();
  const items = document.querySelectorAll<HTMLElement>('.tab-item');
  for (let i = 0; i < items.length; i++) {
    items[i].classList.toggle('active', items[i].dataset.tab === name);
  }
  const panes = document.querySelectorAll<HTMLElement>('.pane');
  for (let j = 0; j < panes.length; j++) {
    panes[j].classList.toggle('active', panes[j].id === 'pane-' + name);
  }
}

document.querySelectorAll<HTMLElement>('.tab-item').forEach(function (btn) {
  let fired = false;
  btn.addEventListener('touchend', function (e) {
    e.preventDefault();
    fired = true;
    switchTab(btn.dataset.tab as TabName);
    setTimeout(function () { fired = false; }, 400);
  }, { passive: false });
  btn.addEventListener('click', function () {
    if (!fired) switchTab(btn.dataset.tab as TabName);
  });
});

// ── Mode select ──
// Post-connect ops bar lives inside the connected card now (#seg-mode),
// not in the discovery dock (#disc-dock — removed). Restore the persisted
// preference on boot so the connected card lights up the right segment
// even before the user touches it.
{
  const stored = localStorage.getItem('vior_last_mode');
  if (stored === 'mirror' || stored === 'extend') selectedMode = stored;
}
function reflectModeInUI(mode: Mode): void {
  document.querySelectorAll<HTMLElement>('#seg-mode .seg-btn').forEach(function (b) {
    b.classList.toggle('active', b.dataset.mode === mode);
  });
}
reflectModeInUI(selectedMode);
document.querySelectorAll<HTMLElement>('#seg-mode .seg-btn').forEach(function (btn) {
  btn.addEventListener('click', function () {
    const next = (btn.dataset.mode as Mode);
    if (!next || next === selectedMode) return;
    selectedMode = next;
    try { localStorage.setItem('vior_last_mode', next); } catch (_) {}
    reflectModeInUI(next);
    // Live mode switch — only meaningful once connected. Re-send the
    // hello-style dims with the new mode so the desktop reconfigures
    // its virtual display without a full reconnect.
    if (connected && ws && ws.readyState === 1) {
      const dpr = window.devicePixelRatio || 1;
      try {
        ws.send(JSON.stringify({ type: 'resize', data: {
          width: Math.round(screen.width * dpr),
          height: Math.round(screen.height * dpr),
          dpr: dpr,
          mode: next,
        }}));
      } catch (_) {}
      const sm = $('stat-mode');
      if (sm) sm.textContent = 'Wi-Fi · ' + (next === 'mirror' ? 'Mirror' : 'Extend');
      toast('info', 'Mode changed', next === 'mirror' ? 'Mirroring screen.' : 'Extended display.');
    }
  });
});

