// MIGRATED → see core.ts (this .js stays as the runtime source until Vite bundling lands)
'use strict';
var $ = function (id) { return document.getElementById(id); };

// ── State ──
var ws = null;
var displayW = 0, displayH = 0;
var selectedServer = null, selectedMode = 'extend';
var serverName = '', serverPlatform = '', serverRes = '—';
var streamImg = $('stream-img');
var framePolling = false, frameBaseUrl = '', blobUrl = null;
var overlayTimer = null, reconnectAttempts = 0, maxReconnect = 5;
var connected = false, currentTab = 'display';
var streamVisible = false, fps = 0, frameCount = 0, fpsTimer = null;

// ── Toasts ──
var toastId = 0;
function toast(tone, title, msg) {
  var id = ++toastId;
  var host = $('toast-host');
  var el = document.createElement('div');
  el.className = 'toast';
  el.dataset.id = id;
  var dotCls = tone === 'success' ? 'dot-ok' : tone === 'warning' ? 'dot-warn' : tone === 'error' ? 'dot-err' : 'dot-idle';
  el.innerHTML =
    '<span class="dot ' + dotCls + '" style="margin-top: 5px;"></span>' +
    '<div style="flex:1;min-width:0;">' +
      '<div class="toast-title">' + esc(title) + '</div>' +
      (msg ? '<div class="toast-msg">' + esc(msg) + '</div>' : '') +
    '</div>';
  host.appendChild(el);
  setTimeout(function () { if (el.parentNode) el.parentNode.removeChild(el); }, 3500);
}
function esc(s) { var d = document.createElement('div'); d.textContent = String(s == null ? '' : s); return d.innerHTML; }

// ── Connection chip ──
function setConnState(state) {
  var dot = $('conn-dot'), label = $('conn-label');
  dot.className = 'dot';
  if (state === 'online') { dot.classList.add('dot-ok', 'dot-pulse'); label.textContent = 'Connected'; }
  else if (state === 'connecting') { dot.classList.add('dot-warn', 'dot-pulse'); label.textContent = 'Connecting'; }
  else if (state === 'reconnecting') { dot.classList.add('dot-warn', 'dot-pulse'); label.textContent = 'Reconnecting'; }
  else if (state === 'error') { dot.classList.add('dot-err'); label.textContent = 'Disconnected'; }
  else { dot.classList.add('dot-idle'); label.textContent = 'Not connected'; }
}

// ── Tab switch ──
function switchTab(name) {
  currentTab = name;
  // Always release camera when navigating away — leaving the scanner
  // hot in the background is what causes the next acquire to fail with
  // NotReadableError. `stopQRScan` is defined later and is idempotent.
  if (typeof stopQRScan === 'function') stopQRScan();
  var items = document.querySelectorAll('.tab-item');
  for (var i = 0; i < items.length; i++) items[i].classList.toggle('active', items[i].dataset.tab === name);
  var panes = document.querySelectorAll('.pane');
  for (var j = 0; j < panes.length; j++) panes[j].classList.toggle('active', panes[j].id === 'pane-' + name);
}
document.querySelectorAll('.tab-item').forEach(function (btn) {
  var fired = false;
  btn.addEventListener('touchend', function (e) { e.preventDefault(); fired = true; switchTab(btn.dataset.tab); setTimeout(function () { fired = false; }, 400); }, { passive: false });
  btn.addEventListener('click', function () { if (!fired) switchTab(btn.dataset.tab); });
});

// ── Mode select ──
document.querySelectorAll('#disc-dock .seg-btn').forEach(function (btn) {
  btn.addEventListener('click', function () {
    selectedMode = btn.dataset.mode;
    document.querySelectorAll('#disc-dock .seg-btn').forEach(function (b) { b.classList.toggle('active', b === btn); });
  });
});

