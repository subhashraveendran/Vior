'use strict';
// ── QR scanner (native BarcodeDetector with jsQR fallback) ──
let qrStream: MediaStream | null = null;
let qrRunning = false;
let qrDetector: BarcodeDetectorLike | null = null;
let qrCanvas: HTMLCanvasElement | null = null;
let qrCtx: CanvasRenderingContext2D | null = null;

interface ParsedScan { host: string; port: number; code: string; }

function parseScanResult(raw: string): ParsedScan {
  let url = raw, code = '';
  const m = raw.match(/^vior:\/\/([\d.]+)(?::(\d+))?(?:\?pair=([A-F0-9]+))?/i);
  if (m) { url = 'http://' + m[1] + ':' + (m[2] || '8080'); code = m[3] || ''; }
  else {
    const u = raw.match(/^https?:\/\/([\d.]+)(?::(\d+))?/i);
    if (u) { url = 'http://' + u[1] + ':' + (u[2] || '8080'); }
    const pm = raw.match(/[?&]pair=([0-9A-Z]+)/i); if (pm) code = pm[1];
  }
  const hp = url.replace(/^https?:\/\//, '').split(':');
  return { host: hp[0], port: parseInt(hp[1] || '8080'), code: code };
}
function onScanHit(raw: string): void {
  const p = parseScanResult(raw);
  // Visible "decoded" feedback before the modal disappears so the user
  // knows the scan worked — otherwise the modal just vanishes and they
  // wonder if they missed it.
  const dot = $('qr-scanning-dot'), txt = $('qr-hint-text');
  if (dot) dot.style.background = '#3ecf6f';
  if (txt) txt.textContent = 'Decoded ' + (p.host || 'server') + ' — connecting…';
  if (p.code) ($('manual-pair') as HTMLInputElement).value = p.code.replace(/[^0-9]/g, '');
  toast('success', 'QR scanned', 'Connecting to ' + p.host + (p.port !== 8080 ? ':' + p.port : '') + '…');
  setTimeout(function () {
    stopQRScan();
    selectServer(p.host, p.port, p.host, '');
    doConnect();
  }, 250);
}
function stopQRScan(): void {
  // Idempotent — safe to call from anywhere (cancel button, error path,
  // tab switch, visibilitychange, pagehide, successful detect). Without
  // this guard, double-invocation could fight with an in-flight start.
  if (!qrRunning && !qrStream) { $('qr-modal').classList.add('hidden'); return; }
  qrRunning = false;
  if (qrStream) {
    try { qrStream.getTracks().forEach(function (t) { try { t.stop(); } catch (_) {} }); } catch (_) {}
    qrStream = null;
  }
  const v = $('qr-video') as HTMLVideoElement | null;
  if (v) { try { v.pause(); } catch (_) {} v.srcObject = null; }
  $('qr-modal').classList.add('hidden');
}
// Layered camera acquisition: try the strictest constraint first so we
// get the rear camera on multi-camera devices, then fall back. Honor
// tablets often refuse `facingMode: { exact: 'environment' }` with
// OverconstrainedError; the fallback paths handle that.
async function acquireCamera(): Promise<MediaStream> {
  // Cap sensor output at 720×720 — QR codes only need a few hundred px to
  // decode, but modern phone cameras default to 1080p/4K which burns the
  // main thread on every getImageData. Constraint is ideal, not exact.
  const sz = { width: { ideal: 720, max: 1280 }, height: { ideal: 720, max: 1280 } };
  const tries: MediaStreamConstraints[] = [
    { video: Object.assign({ facingMode: { exact: 'environment' } }, sz) },
    { video: Object.assign({ facingMode: 'environment' }, sz) },
    { video: Object.assign({}, sz) },
    { video: true }
  ];
  let lastErr: unknown;
  for (let i = 0; i < tries.length; i++) {
    try { return await navigator.mediaDevices.getUserMedia(tries[i]); }
    catch (e) {
      lastErr = e;
      // Only iterate on constraint-shape errors — other errors (permission,
      // hardware busy, missing device) will not be solved by relaxing.
      const name = (e as { name?: string } | null)?.name;
      if (e && name !== 'OverconstrainedError' && name !== 'TypeError') break;
    }
  }
  throw lastErr;
}
async function startQRScan(): Promise<void> {
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    toast('error', 'Camera unsupported', 'getUserMedia unavailable on this WebView.'); return;
  }
  // jsQR is loaded with `defer` — if the user taps Scan QR before the
  // script finishes parsing, jsQR will be undefined. Wait for load.
  if (!window.jsQR && !('BarcodeDetector' in window)) {
    await new Promise<void>(function (r) {
      if (document.readyState === 'complete') r();
      else window.addEventListener('load', () => r(), { once: true });
    });
  }
  try {
    try {
      qrStream = await acquireCamera();
    } catch (e1) {
      // NotReadableError typically means the camera device is held by a
      // previous (un-stopped) MediaStreamTrack in this WebView. Tear down
      // any stale stream and retry once after a short delay.
      if (e1 && (e1 as { name?: string }).name === 'NotReadableError') {
        stopQRScan();
        await new Promise<void>(function (r) { setTimeout(r, 250); });
        qrStream = await acquireCamera();
      } else { throw e1; }
    }
    const video = $('qr-video') as HTMLVideoElement;
    video.srcObject = qrStream;
    try { await video.play(); }
    catch (pe) {
      // Some WebViews resolve play() into a rejected promise (autoplay
      // policy, lost focus). Surface it instead of leaving a black modal.
      toast('error', 'Video failed to start', String((pe as { message?: string } | null)?.message || pe));
      stopQRScan();
      return;
    }
    $('qr-modal').classList.remove('hidden');
    qrRunning = true;
    if ('BarcodeDetector' in window) {
      try {
        const Ctor = (window as unknown as { BarcodeDetector: new (opts: { formats: string[] }) => BarcodeDetectorLike }).BarcodeDetector;
        qrDetector = new Ctor({ formats: ['qr_code'] });
        tickQRNative();
        return;
      }
      catch (be) { console.warn('BarcodeDetector advertised but instantiation failed; falling back to jsQR:', be); }
    }
    if (typeof jsQR === 'function') { tickQRJsQR(); return; }
    toast('error', 'Scanner load failed', 'jsQR not loaded; reinstall app.');
    stopQRScan();
  } catch (e) {
    const name = ((e as { name?: string } | null)?.name) || '';
    if (name === 'NotAllowedError' || name === 'PermissionDeniedError') {
      toast('error', 'Camera blocked', 'Allow camera in System Settings → Apps → Vior.');
    } else if (name === 'NotReadableError') {
      toast('error', 'Camera busy', 'Camera in use by another app — close it and retry.');
    } else if (name === 'NotFoundError') {
      toast('error', 'No camera', 'No camera device found.');
    } else {
      toast('error', 'Camera failed', String((e as { message?: string } | null)?.message || e));
    }
    stopQRScan();
  }
}
async function tickQRNative(): Promise<void> {
  if (!qrRunning) return;
  try {
    const codes = await qrDetector!.detect($('qr-video') as HTMLVideoElement);
    if (codes && codes.length) { onScanHit(codes[0].rawValue || ''); return; }
  } catch (_) {}
  requestAnimationFrame(tickQRNative);
}
function tickQRJsQR(): void {
  if (!qrRunning) return;
  const v = $('qr-video') as HTMLVideoElement;
  if (!qrCanvas) { qrCanvas = document.createElement('canvas'); qrCtx = qrCanvas.getContext('2d', { willReadFrequently: true }); }
  if (v.readyState === v.HAVE_ENOUGH_DATA && v.videoWidth) {
    const w = v.videoWidth, h = v.videoHeight;
    // Downscale for perf: detect on 480-wide frame max.
    const scale = w > 480 ? 480 / w : 1;
    const dw = Math.round(w * scale), dh = Math.round(h * scale);
    // Only resize when dims actually change — every canvas.width assignment
    // reallocates the backing store, which is wasteful at 60fps.
    if (qrCanvas!.width !== dw || qrCanvas!.height !== dh) { qrCanvas!.width = dw; qrCanvas!.height = dh; }
    qrCtx!.drawImage(v, 0, 0, dw, dh);
    const img = qrCtx!.getImageData(0, 0, dw, dh);
    const code = jsQR ? jsQR(img.data, dw, dh, { inversionAttempts: 'attemptBoth' }) : null;
    if (code && code.data) { onScanHit(code.data); return; }
  }
  requestAnimationFrame(tickQRJsQR);
}
$('scan-qr-btn').addEventListener('click', startQRScan);
$('qr-cancel').addEventListener('click', stopQRScan);
// Release the camera whenever the page is backgrounded or torn down —
// Android WebView does not automatically stop MediaStreamTracks when
// the app goes to the background, and a lingering track will refuse
// the next acquire with NotReadableError.
document.addEventListener('visibilitychange', function () { if (document.hidden) stopQRScan(); });
window.addEventListener('pagehide', stopQRScan);
$('manual-ip').addEventListener('input', function (e: Event) { ($('manual-go') as HTMLButtonElement).disabled = !(e.target as HTMLInputElement).value.trim(); });
$('manual-go').addEventListener('click', function () {
  const v = ($('manual-ip') as HTMLInputElement).value.trim();
  if (!v) return;
  const parts = v.split(':'), host = parts[0], port = parts.length > 1 ? parseInt(parts[1]) : 8080;
  selectServer(host, port, host, '');
  doConnect();
});
