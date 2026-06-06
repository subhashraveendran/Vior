'use strict';
// ── Intent picker ──
//
// First-run gate: shows three giant tiles ("Display", "Remote control",
// "File transfer") and writes the choice to localStorage. Subsequent
// launches skip the overlay and jump straight into the matching UI.
// The user can re-open it any time via Settings → Change intent.
//
// All other screens read the current intent via `getIntent()` and tune
// themselves (hide stream tab, suppress mode selector, set
// hello.skipDisplay, etc).

type ViorIntent = 'display' | 'remote' | 'files';
const VIOR_INTENT_KEY = 'vior_intent';

function getIntent(): ViorIntent {
  const raw = localStorage.getItem(VIOR_INTENT_KEY);
  if (raw === 'remote' || raw === 'files') return raw;
  return 'display';
}
function setIntent(v: ViorIntent): void {
  try { localStorage.setItem(VIOR_INTENT_KEY, v); } catch (_) {}
}

// Hide / show tab bar items + bottom-of-display content based on intent.
//   Display → all tabs visible, mode segment visible
//   Remote  → hide Stream view button, hide Files tab? No — keep Files.
//             Just hide mode-segment + view-stream-btn so the user can't
//             accidentally trigger virtual display.
//   Files   → hide Remote tab + Stream button + mode segment. Default tab
//             becomes Files after a successful connect.
function applyIntentToUI(intent: ViorIntent): void {
  const tabFiles = document.querySelector<HTMLElement>('.tab-item[data-tab="files"]');
  const tabRemote = document.querySelector<HTMLElement>('.tab-item[data-tab="remote"]');
  const tabDisplay = document.querySelector<HTMLElement>('.tab-item[data-tab="display"]');
  const segMode = document.getElementById('seg-mode');
  const viewStreamBtn = document.getElementById('view-stream-btn');
  const statModeLabel = document.getElementById('stat-mode');

  // Reset everything visible first.
  if (tabFiles) tabFiles.classList.remove('hidden');
  if (tabRemote) tabRemote.classList.remove('hidden');
  if (tabDisplay) tabDisplay.classList.remove('hidden');
  if (segMode) segMode.classList.remove('hidden');
  if (viewStreamBtn) viewStreamBtn.classList.remove('hidden');

  if (intent === 'remote') {
    // No display capture → no stream button, no mode selector. Tabs:
    // keep Display (= connection home) + Files + Remote.
    if (segMode) segMode.classList.add('hidden');
    if (viewStreamBtn) viewStreamBtn.classList.add('hidden');
    if (statModeLabel) statModeLabel.textContent = 'Remote';
  } else if (intent === 'files') {
    // No display, no remote. Tabs: Display (home) + Files only.
    if (tabRemote) tabRemote.classList.add('hidden');
    if (segMode) segMode.classList.add('hidden');
    if (viewStreamBtn) viewStreamBtn.classList.add('hidden');
    if (statModeLabel) statModeLabel.textContent = 'Files';
  }

  // Update Settings summary label.
  const sum = document.getElementById('intent-summary');
  if (sum) sum.textContent = intent;
}

function showIntentPicker(): void {
  const ov = document.getElementById('intent-overlay');
  if (!ov) return;
  ov.classList.remove('hidden');
}
function hideIntentPicker(): void {
  const ov = document.getElementById('intent-overlay');
  if (!ov) return;
  ov.classList.add('hidden');
}

// Wire tiles once.
document.querySelectorAll<HTMLElement>('#intent-overlay .intent-tile').forEach(function (b) {
  b.addEventListener('click', function () {
    const v = (b.dataset.intent as ViorIntent) || 'display';
    setIntent(v);
    applyIntentToUI(v);
    hideIntentPicker();
    toast('success', 'Got it', v === 'display' ? 'Display mode — pick a server below.' :
      v === 'remote' ? 'Remote-control mode — no screen mirroring.' :
      'File-transfer mode — connect to send files.');
  });
});

// Settings → Change intent.
const changeIntentBtn = document.getElementById('change-intent');
if (changeIntentBtn) {
  changeIntentBtn.addEventListener('click', function () {
    // Close the settings sheet first so the overlay is on top.
    const sheet = document.getElementById('settings-sheet');
    if (sheet) sheet.classList.add('hidden');
    showIntentPicker();
  });
}

// Boot: if no choice ever made, show overlay. Otherwise apply.
const stored = localStorage.getItem(VIOR_INTENT_KEY);
if (!stored) {
  showIntentPicker();
} else {
  applyIntentToUI(getIntent());
}

// Expose for cross-module use (connect.ts reads getIntent in hello build).
(window as unknown as {
  viorIntent?: () => ViorIntent;
  viorApplyIntent?: (v: ViorIntent) => void;
}).viorIntent = getIntent;
(window as unknown as {
  viorIntent?: () => ViorIntent;
  viorApplyIntent?: (v: ViorIntent) => void;
}).viorApplyIntent = applyIntentToUI;
