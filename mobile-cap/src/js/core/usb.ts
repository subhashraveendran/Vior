// ── USB callbacks (Java bridge) ──
//
// The Android host injects these as plain window-level functions and
// invokes them from native code. They share state with the rest of the
// app via the globals declared in ../globals.d.ts.

// `transportMode` is read by other modules to decide which path to
// route through. 'wifi' = WebSocket + MJPEG, 'usb' = AOA over the
// cable, frames pushed by Java.
let transportMode: 'wifi' | 'usb' = 'wifi';

// ── Inbound video frame from the cable ────────────────────────────
window.onUsbFrame = function (b64: string): void {
  if (!streamVisible) { openStream(); }
  if (streamImg) {
    streamImg.src = 'data:image/jpeg;base64,' + b64;
  }
  const ld = $('stream-loading');
  if (ld && !ld.classList.contains('hidden')) ld.classList.add('hidden');
  frameCount++;
};

// ── Cable attached + AOA handshake done ───────────────────────────
window.onUsbConnected = function (): void {
  transportMode = 'usb';
  connected = true;
  // Remember USB preference so a later disconnect returns the user
  // to the USB entry surface (not Wi-Fi).
  try { localStorage.setItem('vior_entry_mode', 'usb'); } catch (_) {}
  // Populate the same fields a Wi-Fi connect would, so the Connected
  // card + stream overlay render correctly.
  serverName = 'Desktop via USB';
  serverPlatform = 'USB cable';
  selectedMode = 'extend';

  setConnState('online');

  // Connected card: show "Desktop via USB" with a USB pill.
  const cardName = $('scard-name');
  const cardMeta = $('scard-meta');
  const statMode = $('stat-mode');
  const statStatus = $('stat-status');
  if (cardName) cardName.textContent = serverName;
  if (cardMeta) cardMeta.textContent = 'Wired connection · low latency';
  if (statMode) statMode.textContent = 'USB';
  if (statStatus) statStatus.textContent = 'Live';

  // Flip the view to the connected card + unlock Files / Remote tabs.
  const showFn = (window as unknown as { showView?: (n: string) => void }).showView;
  if (typeof showFn === 'function') showFn('connected');
  $('files-offline')?.classList.add('hidden');
  $('files-active')?.classList.remove('hidden');
  $('remote-offline')?.classList.add('hidden');
  $('remote-active')?.classList.remove('hidden');

  // Stream overlay text.
  const t = $('stream-mode-text');
  if (t) t.textContent = 'USB · live';

  toast('success', 'USB connected', 'Streaming over cable. No Wi-Fi needed.');
};

// ── Cable yanked / desktop quit ───────────────────────────────────
window.onUsbDisconnected = function (): void {
  // Only act if USB was the active transport. A Wi-Fi session shouldn't
  // be torn down by a stale USB-disconnect from a previous run.
  if (transportMode !== 'usb') return;
  transportMode = 'wifi';
  connected = false;

  if (streamVisible) hideStream();
  setConnState('offline');

  // Flip back to discovery view; reset card + tabs.
  const showFn = (window as unknown as { showView?: (n: string) => void }).showView;
  if (typeof showFn === 'function') showFn('disc');
  const sync = (window as unknown as { syncEntryMode?: () => void }).syncEntryMode;
  if (typeof sync === 'function') sync();
  $('files-offline')?.classList.remove('hidden');
  $('files-active')?.classList.add('hidden');
  $('remote-offline')?.classList.remove('hidden');
  $('remote-active')?.classList.add('hidden');

  toast('warning', 'USB disconnected', 'Cable unplugged — re-plug or use Wi-Fi.');
};

// ── Resolution handshake from the desktop ─────────────────────────
window.onUsbReady = function (w: number, h: number): void {
  displayW = w;
  displayH = h;
  serverRes = w + ' × ' + h;
  const sr = $('stat-res');
  if (sr) sr.textContent = serverRes;
  const streamRes = $('stream-stat-res');
  if (streamRes) streamRes.textContent = serverRes;
};

// Expose transportMode so other modules can branch (file transfer
// disables on USB, mode pill chooses label, etc).
(window as unknown as { viorTransport?: () => 'wifi' | 'usb' }).viorTransport = () => transportMode;
