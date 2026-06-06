// MIGRATED → see usb.ts (this .js stays as the runtime source until Vite bundling lands)
'use strict';
// ── USB callbacks (Java bridge) ──
window.onUsbFrame = function (b64) {
  if (!streamVisible) { openStream(); }
  streamImg.src = 'data:image/jpeg;base64,' + b64;
  var ld = $('stream-loading'); if (!ld.classList.contains('hidden')) ld.classList.add('hidden');
  frameCount++;
};
window.onUsbConnected = function () {
  connected = true;
  setConnState('online');
  $('stream-mode-text').textContent = 'USB · Live';
};
window.onUsbDisconnected = function () { if (streamVisible) hideStream(); };
window.onUsbReady = function (w, h) {
  displayW = w; displayH = h;
  serverRes = w + ' × ' + h;
  var sr = $('stat-res'); if (sr) sr.textContent = serverRes;
};

