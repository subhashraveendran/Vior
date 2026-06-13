'use strict';
// ── Stream fullscreen ──
$('view-stream-btn').addEventListener('click', openStream);
$('stream-back').addEventListener('click', hideStream);
$('stream-disconnect').addEventListener('click', function () { hideStream(); doDisconnect(); });
// Floating settings FAB inside the stream — opens the Settings sheet
// on top of the stream without exiting fullscreen.
const streamFab = $('stream-settings-fab');
if (streamFab) {
  streamFab.addEventListener('click', function () {
    $('settings-sheet').classList.remove('hidden');
  });
}

function openStream(): void {
  streamVisible = true;
  $('stream-fs').classList.add('active');
  $('stream-name').textContent = serverName;
  $('stream-mode-text').textContent = selectedMode === 'mirror' ? 'Mirroring' : 'Extended display';
  $('stream-stat-res').textContent = serverRes;
  $('stream-loading').classList.remove('hidden');
  $('stream-loading-sub').textContent = 'negotiating MJPEG · ' + serverRes;
  startFramePolling();
  startOverlayAutoHide();
}
function hideStream(): void {
  streamVisible = false;
  stopFramePolling();
  cleanupBlob();
  streamImg.src = '';
  $('stream-fs').classList.remove('active');
  if (fpsTimer) { clearInterval(fpsTimer); fpsTimer = null; }
}

function startFramePolling(): void {
  framePolling = true;
  frameCount = 0;
  if (fpsTimer) clearInterval(fpsTimer);
  fpsTimer = setInterval(function () {
    fps = frameCount; frameCount = 0;
    $('stream-stat-fps').textContent = fps + ' fps';
    $('stream-stat-status').textContent = fps > 0 ? 'live' : '—';
  }, 1000);
  pollFrame();
}
function stopFramePolling(): void {
  framePolling = false;
  if (fpsTimer) { clearInterval(fpsTimer); fpsTimer = null; }
}
function cleanupBlob(): void { if (blobUrl) { URL.revokeObjectURL(blobUrl); blobUrl = null; } }
function pollFrame(): void {
  if (!framePolling) return;
  fetch(frameBaseUrl + '/snapshot?t=' + Date.now())
    .then(function (r) { if (!r.ok) throw 0; return r.blob(); })
    .then(function (b: Blob) {
      if (!framePolling) return;
      cleanupBlob();
      blobUrl = URL.createObjectURL(b);
      streamImg.src = blobUrl;
      frameCount++;
      const ld = $('stream-loading');
      if (!ld.classList.contains('hidden')) ld.classList.add('hidden');
      reconnectAttempts = 0;
      requestAnimationFrame(pollFrame);
    })
    .catch(function () { if (framePolling) setTimeout(pollFrame, 150); });
}

// ── Stream overlay auto-hide ──
function startOverlayAutoHide(): void {
  showOverlay();
  streamImg.removeEventListener('click', toggleOverlay);
  streamImg.addEventListener('click', toggleOverlay);
}
function showOverlay(): void {
  $('stream-fs').classList.remove('dimmed');
  $('stream-top').style.transform = ''; $('stream-top').style.opacity = '';
  $('stream-bot').style.transform = ''; $('stream-bot').style.opacity = '';
  if (overlayTimer) clearTimeout(overlayTimer);
  overlayTimer = setTimeout(function () {
    $('stream-top').style.transform = 'translateY(-110%)'; $('stream-top').style.opacity = '0';
    $('stream-bot').style.transform = 'translateY(110%)'; $('stream-bot').style.opacity = '0';
  }, 2800);
}
function toggleOverlay(): void {
  if ($('stream-top').style.opacity === '0') showOverlay();
  else {
    if (overlayTimer) clearTimeout(overlayTimer);
    $('stream-top').style.transform = 'translateY(-110%)'; $('stream-top').style.opacity = '0';
    $('stream-bot').style.transform = 'translateY(110%)'; $('stream-bot').style.opacity = '0';
  }
}

// ── Touch input on stream ──
function mapT(t: Touch): { x: number; y: number } {
  const r = streamImg.getBoundingClientRect();
  return { x: Math.round((t.clientX - r.left) / r.width * displayW), y: Math.round((t.clientY - r.top) / r.height * displayH) };
}
function sendInput(action: 'down' | 'move' | 'up', x: number, y: number): void {
  if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: 'input', data: { event: 'touch', action: action, x: x, y: y } }));
}
streamImg.addEventListener('touchstart', function (e: TouchEvent) { e.preventDefault(); const p = mapT(e.changedTouches[0]); sendInput('down', p.x, p.y); }, { passive: false });
streamImg.addEventListener('touchmove', function (e: TouchEvent) { e.preventDefault(); const p = mapT(e.changedTouches[0]); sendInput('move', p.x, p.y); }, { passive: false });
streamImg.addEventListener('touchend', function (e: TouchEvent) { e.preventDefault(); const p = mapT(e.changedTouches[0]); sendInput('up', p.x, p.y); }, { passive: false });
