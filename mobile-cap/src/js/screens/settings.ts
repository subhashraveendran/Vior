'use strict';
// ── Settings sheet ──
($('settings-btn') as HTMLElement).addEventListener('click', function (): void { ($('settings-sheet') as HTMLElement).classList.remove('hidden'); });
($('settings-close') as HTMLElement).addEventListener('click', function (): void { ($('settings-sheet') as HTMLElement).classList.add('hidden'); });
($('settings-sheet') as HTMLElement).addEventListener('click', function (e: Event): void { if (e.target === $('settings-sheet')) ($('settings-sheet') as HTMLElement).classList.add('hidden'); });

// accent picker
interface AccentPreset { color: string; on: string; weak: string; line: string; }
const ACCENT_PRESETS: AccentPreset[] = [
  { color: '#ff8a4c', on: '#1a0e06', weak: 'rgba(255,138,76,0.14)', line: 'rgba(255,138,76,0.40)' },
  { color: '#4cc2ff', on: '#06121a', weak: 'rgba(76,194,255,0.14)', line: 'rgba(76,194,255,0.40)' },
  { color: '#46d39a', on: '#06140e', weak: 'rgba(70,211,154,0.14)', line: 'rgba(70,211,154,0.40)' },
  { color: '#e8e8ea', on: '#0b0d10', weak: 'rgba(232,232,234,0.14)', line: 'rgba(232,232,234,0.40)' },
];
function applyAccent(hex: string | null): void {
  const p = ACCENT_PRESETS.find(function (x: AccentPreset): boolean { return x.color === hex; }) || ACCENT_PRESETS[0];
  const r = document.documentElement.style;
  r.setProperty('--accent', p.color);
  r.setProperty('--accent-2', p.color);
  r.setProperty('--on-accent', p.on);
  r.setProperty('--accent-weak', p.weak);
  r.setProperty('--accent-line', p.line);
  localStorage.setItem('vior_accent', p.color);
  document.querySelectorAll<HTMLElement>('.accent-swatch').forEach(function (b: HTMLElement): void {
    b.classList.toggle('active', b.dataset.accent === p.color);
    b.style.setProperty('--swatch-color', b.dataset.accent || '');
  });
}
document.querySelectorAll<HTMLElement>('.accent-swatch').forEach(function (b: HTMLElement): void {
  b.style.setProperty('--swatch-color', b.dataset.accent || '');
  b.addEventListener('click', function (): void { applyAccent(b.dataset.accent || null); updateAppearanceSummary(); });
});
applyAccent(localStorage.getItem('vior_accent') || '#ff8a4c');

// ── Appearance subscreen ──
function applyStyle(v: string): void { document.documentElement.setAttribute('data-vior-style', v); localStorage.setItem('vior_style', v); setSegActive('seg-style', 'style', v); }
function applyDensity(v: string): void { document.documentElement.setAttribute('data-vior-density', v); localStorage.setItem('vior_density', v); setSegActive('seg-density', 'density', v); }
function applyMotion(v: string): void { document.documentElement.setAttribute('data-vior-motion', v); localStorage.setItem('vior_motion', v); setSegActive('seg-motion', 'motion', v); }
function setSegActive(segId: string, attr: string, v: string): void {
  const seg = $(segId); if (!seg) return;
  seg.querySelectorAll<HTMLElement>('.seg-btn').forEach(function (b: HTMLElement): void { b.classList.toggle('active', b.dataset[attr] === v); });
}
function updateAppearanceSummary(): void {
  const style = localStorage.getItem('vior_style') || 'precise';
  const density = localStorage.getItem('vior_density') || 'regular';
  const motion = localStorage.getItem('vior_motion') || 'expressive';
  const hex = (localStorage.getItem('vior_accent') || '#ff8a4c').toLowerCase();
  const name = ({ '#ff8a4c': 'orange', '#4cc2ff': 'blue', '#46d39a': 'green', '#e8e8ea': 'white' } as Record<string, string>)[hex] || 'custom';
  const sum = $('appearance-summary'); if (sum) sum.textContent = style + ' · ' + name + ' · ' + density + ' · ' + motion;
}
document.querySelectorAll<HTMLElement>('#seg-style .seg-btn').forEach(function (b: HTMLElement): void { b.addEventListener('click', function (): void { applyStyle(b.dataset.style || ''); updateAppearanceSummary(); }); });
document.querySelectorAll<HTMLElement>('#seg-density .seg-btn').forEach(function (b: HTMLElement): void { b.addEventListener('click', function (): void { applyDensity(b.dataset.density || ''); updateAppearanceSummary(); }); });
document.querySelectorAll<HTMLElement>('#seg-motion .seg-btn').forEach(function (b: HTMLElement): void { b.addEventListener('click', function (): void { applyMotion(b.dataset.motion || ''); updateAppearanceSummary(); }); });
applyStyle(localStorage.getItem('vior_style') || 'precise');
applyDensity(localStorage.getItem('vior_density') || 'regular');
applyMotion(localStorage.getItem('vior_motion') || 'expressive');
updateAppearanceSummary();

($('open-appearance') as HTMLElement).addEventListener('click', function (): void { ($('settings-main') as HTMLElement).classList.add('hidden'); ($('appearance-view') as HTMLElement).classList.remove('hidden'); });
($('appearance-back') as HTMLElement).addEventListener('click', function (): void { ($('appearance-view') as HTMLElement).classList.add('hidden'); ($('settings-main') as HTMLElement).classList.remove('hidden'); });
($('appearance-done') as HTMLElement).addEventListener('click', function (): void { ($('appearance-view') as HTMLElement).classList.add('hidden'); ($('settings-main') as HTMLElement).classList.remove('hidden'); });

// Generic toggles bound to localStorage keys.
document.querySelectorAll<HTMLElement>('.vior-toggle').forEach(function (t: HTMLElement): void {
  const key = t.dataset.key || '';
  const def = key === 'vior_usb_only' ? '0' : '1';
  let on = (localStorage.getItem(key) || def) !== '0';
  t.classList.toggle('off', !on);
  t.addEventListener('click', function (): void {
    on = !on;
    t.classList.toggle('off', !on);
    localStorage.setItem(key, on ? '1' : '0');
    if (key === 'vior_wifi' || key === 'vior_usb_only') {
      // Restart discovery to apply.
      if (on || key === 'vior_usb_only') startDiscovery();
      const prev = t.previousElementSibling;
      const label = prev ? prev.querySelector('div') : null;
      toast('info', 'Updated', (label ? label.textContent : '') + ' ' + (on ? 'enabled' : 'disabled'));
    }
    // Mirror the boot-autostart flag to native side so the
    // BootReceiver can read it from SharedPreferences.
    if (key === 'vior_boot_autostart') {
      const bridge = (window as unknown as { Android?: { setBootAutostart?: (v: boolean) => void } }).Android;
      if (bridge && typeof bridge.setBootAutostart === 'function') {
        try { bridge.setBootAutostart(on); } catch (_) {}
      }
      toast('info', 'Auto-launch on boot', on ? 'enabled' : 'disabled');
    }
  });
});

// ── Display orientation lock ───────────────────────────────────────
// Locks the Android Activity orientation. screen.width/height that the
// app sends in the hello message reflect the locked orientation, so the
// desktop creates the virtual display in the right shape.
function applyOrient(v: string): void {
  const valid = (v === 'landscape' || v === 'portrait') ? v : 'auto';
  localStorage.setItem('vior_orient', valid);
  const seg = $('seg-orient');
  if (seg) seg.querySelectorAll<HTMLElement>('.seg-btn').forEach(function (b) {
    b.classList.toggle('active', b.dataset.orient === valid);
  });
  const bridge = (window as unknown as { Android?: { setOrientation?: (v: string) => void } }).Android;
  if (bridge && typeof bridge.setOrientation === 'function') {
    try { bridge.setOrientation(valid); } catch (_) {}
  }
}
document.querySelectorAll<HTMLElement>('#seg-orient .seg-btn').forEach(function (b) {
  b.addEventListener('click', function () {
    const v = b.dataset.orient || 'auto';
    applyOrient(v);
    toast('info', 'Orientation', v === 'auto' ? 'Follows device rotation.' :
      ('Locked to ' + v + '.'));
  });
});
applyOrient(localStorage.getItem('vior_orient') || 'auto');

// Open Android Wi-Fi settings via intent URI.
($('open-wifi-settings') as HTMLElement).addEventListener('click', function (): void {
  try { window.location.href = 'intent:#Intent;action=android.settings.WIFI_SETTINGS;end'; }
  catch (e) { toast('error', 'Cannot open Wi-Fi settings', String((e as Error).message || e)); }
});

// Paste URL from clipboard.
($('paste-url-btn') as HTMLElement).addEventListener('click', async function (): Promise<void> {
  try {
    const text = await navigator.clipboard.readText();
    if (!text) { toast('warning', 'Clipboard empty', null); return; }
    const m = text.match(/(?:https?:\/\/)?([\d.]+)(?::(\d+))?(?:[?&]pair=([0-9A-Z]+))?/i);
    if (!m) { toast('error', 'No URL in clipboard', text.slice(0, 40)); return; }
    const host = m[1], port = parseInt(m[2] || '8080'), code = m[3] || '';
    const ok = window.confirm(`Connect to ${host}:${port}?${code ? ' Pair code: ' + code : ''}`);
    if (!ok) return;
    if (code) ($('manual-pair') as HTMLInputElement).value = code.replace(/[^0-9]/g, '');
    ($('settings-sheet') as HTMLElement).classList.add('hidden');
    selectServer(host, port, host, '');
    doConnect();
  } catch (e) {
    toast('error', 'Paste failed', 'Clipboard access denied.');
  }
});

// ── Saved connections list + clear ─────────────────────────────
function renderSavedConns(): void {
  const host = $('saved-conns-list');
  if (!host) return;
  // Collect every server we've successfully connected to (marked
  // 'vior_known_<host>:<port>' = '1' in connect.ts after the 'ready'
  // message). Show one row per host:port + a 'forget' affordance.
  const items: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k && k.indexOf('vior_known_') === 0) {
      items.push(k.substring('vior_known_'.length));
    }
  }
  if (items.length === 0) {
    host.innerHTML = '<div style="padding: 14px 15px; color: var(--text-3); font-size: 12.5px;" class="mono">No saved connections yet.</div>';
    return;
  }
  host.innerHTML = items.map(function (hp) {
    return '<div style="display:flex;align-items:center;gap:12px;padding:12px 15px;border-bottom:1px solid var(--border);">' +
             '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" style="color:var(--accent);"><rect x="2.5" y="4" width="19" height="13" rx="2"/></svg>' +
             '<div style="flex:1;font-family:var(--font-mono);font-size:13px;">' + esc(hp) + '</div>' +
             '<button data-forget="' + esc(hp) + '" style="background:none;border:none;color:#ff6464;cursor:pointer;font-size:12px;font-weight:600;">Forget</button>' +
           '</div>';
  }).join('');
  // Wire forget buttons
  host.querySelectorAll<HTMLButtonElement>('[data-forget]').forEach(function (b) {
    b.addEventListener('click', function () {
      const hp = b.getAttribute('data-forget') || '';
      localStorage.removeItem('vior_known_' + hp);
      if (localStorage.getItem('vior_last') === hp) localStorage.removeItem('vior_last');
      renderSavedConns();
      toast('info', 'Forgotten', hp);
    });
  });
}
// Refresh the list every time the sheet opens.
const settingsBtn = $('settings-btn');
if (settingsBtn) settingsBtn.addEventListener('click', function () { setTimeout(renderSavedConns, 30); });
renderSavedConns();

const clearBtn = $('clear-saved-conns');
if (clearBtn) clearBtn.addEventListener('click', function () {
  // Forget every saved server + pair + device id. Next connect re-prompts.
  const keys: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k && (k.indexOf('vior_known_') === 0 || k === 'vior_last' || k === 'vior_pair' || k === 'vior_device_id')) {
      keys.push(k);
    }
  }
  keys.forEach(function (k) { localStorage.removeItem(k); });
  (($('manual-pair') as HTMLInputElement) || {}).value = '';
  renderSavedConns();
  toast('success', 'Cleared', keys.length + ' saved key' + (keys.length === 1 ? '' : 's') + ' forgotten.');
});

// ── Advanced flag toggle reveals the Debug block ───────────────
function syncAdvancedBlock(): void {
  const block = $('advanced-block');
  if (!block) return;
  if (localStorage.getItem('vior_advanced') === '1') block.classList.remove('hidden');
  else block.classList.add('hidden');
}
syncAdvancedBlock();
const advTrack = $('advanced-track');
if (advTrack) advTrack.addEventListener('click', function () { setTimeout(syncAdvancedBlock, 20); });

// Debug: reset every preference key.
const dbgReset = $('dbg-reset');
if (dbgReset) dbgReset.addEventListener('click', function () {
  const n = localStorage.length;
  localStorage.clear();
  toast('warning', 'All preferences cleared', n + ' keys removed. Reloading…');
  setTimeout(function () { location.reload(); }, 800);
});
