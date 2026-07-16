// Boot — last script in document order, runs after every other module's
// top-level code. Wrapped in try/catch so a single missing global (e.g.
// a screen module failing to load on a flaky CDN) surfaces in logcat
// instead of leaving the user with a stuck splash screen.

// ── Android hardware back button ──────────────────────────────────
// Capacitor's App plugin emits a `backButton` event on Android. We
// consume it so the physical/gesture back does something sensible
// instead of blindly killing the app:
//   1. If any modal / sheet / overlay is open → close it.
//   2. Else if the user is deep in the connect cascade (step B/C/D) →
//      step back one level (D→C, C→B, B→A rescan).
//   3. Else (at root) → exit the app.
// The plugin is accessed purely through the runtime global so we add no
// dependency and no import; if it's absent (browser/dev) this is a no-op.
(function wireHardwareBack(): void {
  interface CapBackApp {
    addListener: (
      ev: 'backButton',
      cb: (info: { canGoBack?: boolean }) => void
    ) => void;
    exitApp: () => void;
  }
  const CapApp = (window as unknown as {
    Capacitor?: { Plugins?: { App?: CapBackApp } };
  }).Capacitor?.Plugins?.App;
  if (!CapApp || typeof CapApp.addListener !== 'function') return;

  const notHidden = (id: string): HTMLElement | null => {
    const el = document.getElementById(id);
    return el && !el.classList.contains('hidden') ? el : null;
  };
  const clickIfPresent = (id: string): boolean => {
    const el = document.getElementById(id) as HTMLButtonElement | null;
    if (el) { el.click(); return true; }
    return false;
  };

  CapApp.addListener('backButton', function () {
    // 1 ── Dismiss any open surface, most-transient first.
    if (notHidden('qr-modal')) {
      const stop = (window as unknown as { stopQRScan?: () => void }).stopQRScan;
      if (typeof stop === 'function') { stop(); } else { clickIfPresent('qr-cancel'); }
      return;
    }
    if (notHidden('pair-prompt')) { clickIfPresent('pair-prompt-cancel'); return; }
    if (notHidden('settings-sheet')) { clickIfPresent('settings-close'); return; }
    if (notHidden('connecting-overlay')) { clickIfPresent('conn-cancel'); return; }

    // 2 ── Step back within the connect cascade if we're in it (B/C/D).
    // Only meaningful while the empty-view cascade surface is showing.
    const setStep = (window as unknown as {
      setCascadeStep?: (s: 'a' | 'b' | 'c' | 'd') => void;
    }).setCascadeStep;
    const emptyShown = !!notHidden('empty-view');
    if (emptyShown && typeof setStep === 'function') {
      if (notHidden('cascade-d')) { setStep('c'); return; }
      if (notHidden('cascade-c')) { setStep('b'); return; }
      if (notHidden('cascade-b')) {
        // Back from the first manual step → restart the auto scan (A).
        setStep('a');
        try {
          const rescan = (window as unknown as { startDiscovery?: () => void }).startDiscovery;
          if (typeof rescan === 'function') rescan();
        } catch (_) { /* best-effort */ }
        return;
      }
    }

    // 3 ── At root → exit.
    CapApp.exitApp();
  });
})();

try {
  globalThis.setConnState('offline');
  globalThis.startDiscovery();
} catch (e: unknown) {
  // eslint-disable-next-line no-console
  console.error('[vior] boot failed:', e);
  // Best-effort fallback: at least show the empty/no-server view so the
  // user has the manual-IP field to fall back on.
  try {
    const v = document.getElementById('empty-view');
    if (v) v.classList.remove('hidden');
    const d = document.getElementById('disc-view');
    if (d) d.classList.add('hidden');
  } catch (_inner: unknown) {
    // swallow — boot fallback is already best-effort
  }
}

