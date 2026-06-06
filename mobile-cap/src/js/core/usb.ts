// ── USB callbacks (Java bridge) ──
//
// The Android host injects these as plain window-level functions and
// invokes them from native code. They share state with the rest of the
// app via the globals declared in ../types.ts.

window.onUsbFrame = function (b64: string): void {
  if (!streamVisible) { openStream(); }
  if (streamImg) {
    streamImg.src = 'data:image/jpeg;base64,' + b64;
  }
  const ld = $('stream-loading');
  if (ld && !ld.classList.contains('hidden')) ld.classList.add('hidden');
  frameCount++;
};

window.onUsbConnected = function (): void {
  connected = true;
  setConnState('online');
  const t = $('stream-mode-text');
  if (t) t.textContent = 'USB · Live';
};

window.onUsbDisconnected = function (): void {
  if (streamVisible) hideStream();
};

window.onUsbReady = function (w: number, h: number): void {
  displayW = w;
  displayH = h;
  serverRes = w + ' × ' + h;
  const sr = $('stat-res');
  if (sr) sr.textContent = serverRes;
};

