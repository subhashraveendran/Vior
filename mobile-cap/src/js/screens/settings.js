// MIGRATED → see settings.ts
'use strict';
// ── Settings sheet ──
$('settings-btn').addEventListener('click', function () { $('settings-sheet').classList.remove('hidden'); });
$('settings-close').addEventListener('click', function () { $('settings-sheet').classList.add('hidden'); });
$('settings-sheet').addEventListener('click', function (e) { if (e.target === $('settings-sheet')) $('settings-sheet').classList.add('hidden'); });

// accent picker
var ACCENT_PRESETS = [
  { color: '#ff8a4c', on: '#1a0e06', weak: 'rgba(255,138,76,0.14)', line: 'rgba(255,138,76,0.40)' },
  { color: '#4cc2ff', on: '#06121a', weak: 'rgba(76,194,255,0.14)', line: 'rgba(76,194,255,0.40)' },
  { color: '#46d39a', on: '#06140e', weak: 'rgba(70,211,154,0.14)', line: 'rgba(70,211,154,0.40)' },
  { color: '#e8e8ea', on: '#0b0d10', weak: 'rgba(232,232,234,0.14)', line: 'rgba(232,232,234,0.40)' },
];
function applyAccent(hex) {
  var p = ACCENT_PRESETS.find(function (x) { return x.color === hex; }) || ACCENT_PRESETS[0];
  var r = document.documentElement.style;
  r.setProperty('--accent', p.color);
  r.setProperty('--accent-2', p.color);
  r.setProperty('--on-accent', p.on);
  r.setProperty('--accent-weak', p.weak);
  r.setProperty('--accent-line', p.line);
  localStorage.setItem('vior_accent', p.color);
  document.querySelectorAll('.accent-swatch').forEach(function (b) {
    b.classList.toggle('active', b.dataset.accent === p.color);
    b.style.setProperty('--swatch-color', b.dataset.accent);
  });
}
document.querySelectorAll('.accent-swatch').forEach(function (b) {
  b.style.setProperty('--swatch-color', b.dataset.accent);
  b.addEventListener('click', function () { applyAccent(b.dataset.accent); updateAppearanceSummary(); });
});
applyAccent(localStorage.getItem('vior_accent') || '#ff8a4c');

// ── Appearance subscreen ──
function applyStyle(v) { document.documentElement.setAttribute('data-vior-style', v); localStorage.setItem('vior_style', v); setSegActive('seg-style', 'style', v); }
function applyDensity(v) { document.documentElement.setAttribute('data-vior-density', v); localStorage.setItem('vior_density', v); setSegActive('seg-density', 'density', v); }
function applyMotion(v) { document.documentElement.setAttribute('data-vior-motion', v); localStorage.setItem('vior_motion', v); setSegActive('seg-motion', 'motion', v); }
function setSegActive(segId, attr, v) {
  var seg = $(segId); if (!seg) return;
  seg.querySelectorAll('.seg-btn').forEach(function (b) { b.classList.toggle('active', b.dataset[attr] === v); });
}
function updateAppearanceSummary() {
  var style = localStorage.getItem('vior_style') || 'precise';
  var density = localStorage.getItem('vior_density') || 'regular';
  var motion = localStorage.getItem('vior_motion') || 'expressive';
  var hex = (localStorage.getItem('vior_accent') || '#ff8a4c').toLowerCase();
  var name = { '#ff8a4c': 'orange', '#4cc2ff': 'blue', '#46d39a': 'green', '#e8e8ea': 'white' }[hex] || 'custom';
  var sum = $('appearance-summary'); if (sum) sum.textContent = style + ' · ' + name + ' · ' + density + ' · ' + motion;
}
document.querySelectorAll('#seg-style .seg-btn').forEach(function (b) { b.addEventListener('click', function () { applyStyle(b.dataset.style); updateAppearanceSummary(); }); });
document.querySelectorAll('#seg-density .seg-btn').forEach(function (b) { b.addEventListener('click', function () { applyDensity(b.dataset.density); updateAppearanceSummary(); }); });
document.querySelectorAll('#seg-motion .seg-btn').forEach(function (b) { b.addEventListener('click', function () { applyMotion(b.dataset.motion); updateAppearanceSummary(); }); });
applyStyle(localStorage.getItem('vior_style') || 'precise');
applyDensity(localStorage.getItem('vior_density') || 'regular');
applyMotion(localStorage.getItem('vior_motion') || 'expressive');
updateAppearanceSummary();

$('open-appearance').addEventListener('click', function () { $('settings-main').classList.add('hidden'); $('appearance-view').classList.remove('hidden'); });
$('appearance-back').addEventListener('click', function () { $('appearance-view').classList.add('hidden'); $('settings-main').classList.remove('hidden'); });
$('appearance-done').addEventListener('click', function () { $('appearance-view').classList.add('hidden'); $('settings-main').classList.remove('hidden'); });

// Generic toggles bound to localStorage keys.
document.querySelectorAll('.vior-toggle').forEach(function (t) {
  var key = t.dataset.key;
  var def = key === 'vior_usb_only' ? '0' : '1';
  var on = (localStorage.getItem(key) || def) !== '0';
  t.classList.toggle('off', !on);
  t.addEventListener('click', function () {
    on = !on;
    t.classList.toggle('off', !on);
    localStorage.setItem(key, on ? '1' : '0');
    if (key === 'vior_wifi' || key === 'vior_usb_only') {
      // Restart discovery to apply.
      if (on || key === 'vior_usb_only') startDiscovery();
      toast('info', 'Updated', t.previousElementSibling.querySelector('div').textContent + ' ' + (on ? 'enabled' : 'disabled'));
    }
  });
});

// Open Android Wi-Fi settings via intent URI.
$('open-wifi-settings').addEventListener('click', function () {
  try { window.location.href = 'intent:#Intent;action=android.settings.WIFI_SETTINGS;end'; }
  catch (e) { toast('error', 'Cannot open Wi-Fi settings', String(e.message || e)); }
});

// Paste URL from clipboard.
$('paste-url-btn').addEventListener('click', async function () {
  try {
    var text = await navigator.clipboard.readText();
    if (!text) { toast('warning', 'Clipboard empty', null); return; }
    var m = text.match(/(?:https?:\/\/)?([\d.]+)(?::(\d+))?(?:[?&]pair=([A-F0-9]+))?/i);
    if (!m) { toast('error', 'No URL in clipboard', text.slice(0, 40)); return; }
    var host = m[1], port = parseInt(m[2] || '8080'), code = m[3] || '';
    if (code) $('manual-pair').value = code.toUpperCase();
    $('settings-sheet').classList.add('hidden');
    selectServer(host, port, host, '');
    doConnect();
  } catch (e) {
    toast('error', 'Paste failed', 'Clipboard access denied.');
  }
});

