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

// ── Cable attached at the OS layer ────────────────────────────────
// IMPORTANT: arriving here means the AOA cable handshake completed —
// it does NOT mean the peer is Vior. We stay in "verifying" until a
// FrameHelloAck with the magic + version comes back from the desktop.
// Until then we don't:
//   • flip transportMode to 'usb'
//   • close any active Wi-Fi WS
//   • show the connected card
//   • forward any input
// This blocks the "cable handshakes at the OS layer but the desktop
// side isn't running Vior" failure mode the previous code had.
window.onUsbConnected = function (): void {
  // Surface the cable in the Wi-Fi transport pill as a "Cable detected"
  // nudge, no matter which transport the user is currently viewing.
  const wifiBadge = document.getElementById('transport-wifi-badge');
  if (wifiBadge) wifiBadge.classList.remove('hidden');

  // Show "Verifying cable…" on the orb regardless of which surface is
  // visible. If the user is on the Wi-Fi cascade they'll see the badge
  // and can swap to USB to watch the handshake.
  const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'verifying' | 'connected' | 'failed') => void }).setUsbStage;
  if (typeof setStage === 'function') setStage('verifying');
};

// ── Magic + version verified by the desktop ───────────────────────
// Only at this point do we promote the cable to the active transport
// (closing any Wi-Fi WS, flipping the view, etc.). Until this fires
// the cable is "wired but unverified".
window.onUsbHelloAck = function (): void {
  // Policy when both transports are alive: USB wins for video (low
  // latency, no Wi-Fi dependency). Close any active WS so we don't
  // have two competing video sources writing to the same <img>, two
  // file-transfer paths, and two sets of input forwarding.
  if (transportMode === 'wifi' && ws) {
    console.log('usb: cable verified during Wi-Fi session — closing WS so USB owns the transport');
    try { ws.close(); } catch (_) { /* ignore */ }
    ws = null;
  }
  transportMode = 'usb';
  connected = true;
  // Remember USB preference so a later disconnect returns the user
  // to the USB entry surface (not Wi-Fi).
  try { localStorage.setItem('vior_entry_mode', 'usb'); } catch (_) {}

  const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'verifying' | 'connected' | 'failed') => void }).setUsbStage;
  if (typeof setStage === 'function') setStage('connected');

  // Populate the same fields a Wi-Fi connect would, so the Connected
  // card + stream overlay render correctly.
  serverName = 'Desktop via USB';
  serverPlatform = 'USB cable';
  selectedMode = 'extend';

  setConnState('online');
  if (typeof viorState !== 'undefined') {
    viorState.set({ state: 'connected', serverName: serverName, transport: 'usb' });
  }

  const cardName = $('scard-name');
  const cardMeta = $('scard-meta');
  const statMode = $('stat-mode');
  const statStatus = $('stat-status');
  if (cardName) cardName.textContent = serverName;
  if (cardMeta) cardMeta.textContent = 'Wired connection · low latency';
  if (statMode) statMode.textContent = 'USB';
  if (statStatus) statStatus.textContent = 'Live';

  const showFn = (window as unknown as { showView?: (n: string) => void }).showView;
  if (typeof showFn === 'function') showFn('connected');
  $('files-offline')?.classList.add('hidden');
  $('files-active')?.classList.remove('hidden');
  $('remote-offline')?.classList.add('hidden');
  $('remote-active')?.classList.remove('hidden');

  const t = $('stream-mode-text');
  if (t) t.textContent = 'USB · live';

  toast('success', 'USB connected', 'Verified Vior desktop — streaming over cable.');
};

// ── No hello-ack within the 3s window ─────────────────────────────
// Cable came up but the desktop didn't speak Vior back. Most common
// cause: desktop app isn't running. Show the recovery surface with a
// "Try again" button (calls Android.usbRetryHello via the JS bridge).
window.onUsbHelloTimeout = function (): void {
  console.log('usb: hello-ack timeout — desktop probably not running Vior');
  // Don't tear down the cable — the user might launch Vior and retry.
  // Just flip the orb into a failed state with recovery copy.
  const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'verifying' | 'connected' | 'failed') => void }).setUsbStage;
  if (typeof setStage === 'function') setStage('failed');
};

// ── Cable yanked / desktop quit ───────────────────────────────────
window.onUsbDisconnected = function (): void {
  // Clear the "Cable detected" badge on the Wi-Fi transport pill no
  // matter what — even if USB never reached the verified state, the
  // cable just went away.
  const wifiBadge = document.getElementById('transport-wifi-badge');
  if (wifiBadge) wifiBadge.classList.add('hidden');

  // Only act if USB was the active transport. A Wi-Fi session shouldn't
  // be torn down by a stale USB-disconnect from a previous run. Log the
  // skip path explicitly — silent early-returns are murder to debug
  // when the user reports "Wi-Fi died when I unplugged the cable".
  if (transportMode !== 'usb') {
    console.log('usb: onUsbDisconnected ignored (transport=' + transportMode + ')');
    // But still reset the orb in case we were in verifying/failed.
    const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'verifying' | 'connected' | 'failed') => void }).setUsbStage;
    if (typeof setStage === 'function') setStage('waiting');
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
  const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'verifying' | 'connected' | 'failed') => void }).setUsbStage;
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
