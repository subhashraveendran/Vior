// Boot — last script in document order, runs after every other module's
// top-level code. Wrapped in try/catch so a single missing global (e.g.
// a screen module failing to load on a flaky CDN) surfaces in logcat
// instead of leaving the user with a stuck splash screen.
import type {} from './types';

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

export {};
