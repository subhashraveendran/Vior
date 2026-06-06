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
  // Policy when both transports are alive: USB wins for video (low
  // latency, no Wi-Fi dependency). Close any active WS so we don't
  // have two competing video sources writing to the same <img>, two
  // file-transfer paths, and two sets of input forwarding.
  if (transportMode === 'wifi' && ws) {
    console.log('usb: cable arrived during Wi-Fi session — closing WS so USB owns the transport');
    try { ws.close(); } catch (_) { /* ignore */ }
    ws = null;
  }
  transportMode = 'usb';
  connected = true;
  // Remember USB preference so a later disconnect returns the user
  // to the USB entry surface (not Wi-Fi).
  try { localStorage.setItem('vior_entry_mode', 'usb'); } catch (_) {}

  // Flip the orb to "Cable detected!" before the connected card swaps in.
  const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'connected') => void }).setUsbStage;
  if (typeof setStage === 'function') setStage('connected');
  // Populate the same fields a Wi-Fi connect would, so the Connected
  // card + stream overlay render correctly.
  serverName = 'Desktop via USB';
  serverPlatform = 'USB cable';
  selectedMode = 'extend';

  setConnState('online');
  // USB cable is its own auth boundary — no scan/pair/connecting
  // states. Jump straight to connected.
  if (typeof viorState !== 'undefined') {
    viorState.set({ state: 'connected', serverName: serverName, transport: 'usb' });
  }

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
  // be torn down by a stale USB-disconnect from a previous run. Log the
  // skip path explicitly — silent early-returns are murder to debug
  // when the user reports "Wi-Fi died when I unplugged the cable".
  if (transportMode !== 'usb') {
    console.log('usb: onUsbDisconnected ignored (transport=' + transportMode + ')');
    return;
  }
  console.log('usb: cable disconnected, tearing down USB transport');
  transportMode = 'wifi';
  connected = false;

  if (streamVisible) hideStream();
  setConnState('offline');
  // Cable yanked → back to scanning so the user has a useful pre-connect
  // surface (matches the spec: USB disconnect returns to last Wi-Fi
  // state or scanning).
  if (typeof viorState !== 'undefined') viorState.set({ state: 'disconnected' });

  // Reset orb back to its breathing "waiting" state.
  const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'connected') => void }).setUsbStage;
  if (typeof setStage === 'function') setStage('waiting');

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
  // Resume Wi-Fi discovery so the user has something to tap on.
  setTimeout(function () { try { startDiscovery(); } catch (_) {} }, 300);
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
