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
  });
});

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
    const m = text.match(/(?:https?:\/\/)?([\d.]+)(?::(\d+))?(?:[?&]pair=([A-F0-9]+))?/i);
    if (!m) { toast('error', 'No URL in clipboard', text.slice(0, 40)); return; }
    const host = m[1], port = parseInt(m[2] || '8080'), code = m[3] || '';
    if (code) ($('manual-pair') as HTMLInputElement).value = code.toUpperCase();
    ($('settings-sheet') as HTMLElement).classList.add('hidden');
    selectServer(host, port, host, '');
    doConnect();
  } catch (e) {
    toast('error', 'Paste failed', 'Clipboard access denied.');
  }
});
