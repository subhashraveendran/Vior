// Core UI primitives: $ helper, shared state vars, toast, connection
// chip, tab switching, and mode-segment binding.
//
// During the TS migration this file still publishes its symbols on
// `globalThis` so the not-yet-converted screen modules keep working.
// The `declare global` block lives in ./types.ts.
import type { ConnectionState, Mode, ServerInfo, TabName, ToastTone } from './types';

const $: (id: string) => HTMLElement | null = function (id) {
  return document.getElementById(id);
};
globalThis.$ = $;

// ── State ──
let ws: WebSocket | null = null;
let displayW = 0, displayH = 0;
let selectedServer: ServerInfo | null = null;
let selectedMode: Mode = 'extend';
let serverName = '', serverPlatform = '', serverRes = '—';
const streamImg = $('stream-img') as HTMLImageElement | null;
let framePolling = false, frameBaseUrl = '';
let blobUrl: string | null = null;
let overlayTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
const maxReconnect = 5;
let connected = false;
let currentTab: TabName = 'display';
let streamVisible = false, fps = 0, frameCount = 0;
let fpsTimer: ReturnType<typeof setInterval> | null = null;

// Publish to globalThis so legacy .js modules can mutate / read the
// same values. Once every screen is converted to ESM imports we can
// drop this block in one go.
globalThis.ws = ws;
globalThis.displayW = displayW;
globalThis.displayH = displayH;
globalThis.selectedServer = selectedServer;
globalThis.selectedMode = selectedMode;
globalThis.serverName = serverName;
globalThis.serverPlatform = serverPlatform;
globalThis.serverRes = serverRes;
globalThis.streamImg = streamImg;
globalThis.framePolling = framePolling;
globalThis.frameBaseUrl = frameBaseUrl;
globalThis.blobUrl = blobUrl;
globalThis.overlayTimer = overlayTimer;
globalThis.reconnectAttempts = reconnectAttempts;
globalThis.maxReconnect = maxReconnect;
globalThis.connected = connected;
globalThis.currentTab = currentTab;
globalThis.streamVisible = streamVisible;
globalThis.fps = fps;
globalThis.frameCount = frameCount;
globalThis.fpsTimer = fpsTimer;

// ── Toasts ──
let toastId = 0;
globalThis.toastId = toastId;

function toast(tone: ToastTone, title: string, msg?: string): void {
  const id = ++toastId;
  globalThis.toastId = toastId;
  const host = $('toast-host');
  if (!host) return;
  const el = document.createElement('div');
  el.className = 'toast';
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

globalThis.toast = toast;
globalThis.esc = esc;

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
globalThis.setConnState = setConnState;

// ── Tab switch ──
function switchTab(name: TabName): void {
  currentTab = name;
  globalThis.currentTab = currentTab;
  // Always release camera when navigating away — leaving the scanner
  // hot in the background is what causes the next acquire to fail with
  // NotReadableError. `stopQRScan` is defined later and is idempotent.
  if (typeof globalThis.stopQRScan === 'function') globalThis.stopQRScan();
  const items = document.querySelectorAll<HTMLElement>('.tab-item');
  for (let i = 0; i < items.length; i++) {
    items[i].classList.toggle('active', items[i].dataset.tab === name);
  }
  const panes = document.querySelectorAll<HTMLElement>('.pane');
  for (let j = 0; j < panes.length; j++) {
    panes[j].classList.toggle('active', panes[j].id === 'pane-' + name);
  }
}
globalThis.switchTab = switchTab;

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
document.querySelectorAll<HTMLElement>('#disc-dock .seg-btn').forEach(function (btn) {
  btn.addEventListener('click', function () {
    selectedMode = (btn.dataset.mode as Mode);
    globalThis.selectedMode = selectedMode;
    document.querySelectorAll<HTMLElement>('#disc-dock .seg-btn').forEach(function (b) {
      b.classList.toggle('active', b === btn);
    });
  });
});

export {};
