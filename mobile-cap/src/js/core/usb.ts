// ── USB callbacks (Java bridge) ──
//
// The Android host injects these as plain window-level functions and
// invokes them from native code. They share state with the rest of the
// app via the globals declared in ../types.ts.
import type {} from '../types';

window.onUsbFrame = function (b64: string): void {
  if (!globalThis.streamVisible) { globalThis.openStream(); }
  if (globalThis.streamImg) {
    globalThis.streamImg.src = 'data:image/jpeg;base64,' + b64;
  }
  const ld = globalThis.$('stream-loading');
  if (ld && !ld.classList.contains('hidden')) ld.classList.add('hidden');
  globalThis.frameCount++;
};

window.onUsbConnected = function (): void {
  globalThis.connected = true;
  globalThis.setConnState('online');
  const t = globalThis.$('stream-mode-text');
  if (t) t.textContent = 'USB · Live';
};

window.onUsbDisconnected = function (): void {
  if (globalThis.streamVisible) globalThis.hideStream();
};

window.onUsbReady = function (w: number, h: number): void {
  globalThis.displayW = w;
  globalThis.displayH = h;
  globalThis.serverRes = w + ' × ' + h;
  const sr = globalThis.$('stat-res');
  if (sr) sr.textContent = globalThis.serverRes;
};

export {};
