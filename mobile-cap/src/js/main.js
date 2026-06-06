'use strict';
// Boot — last script in document order, runs after every other module's
// top-level code. Wrapped in try/catch so a single missing global (e.g.
// a screen module failing to load on a flaky CDN) surfaces in logcat
// instead of leaving the user with a stuck splash screen.
try {
  setConnState('offline');
  startDiscovery();
} catch (e) {
  // eslint-disable-next-line no-console
  console.error('[vior] boot failed:', e);
  // Best-effort fallback: at least show the empty/no-server view so the
  // user has the manual-IP field to fall back on.
  try {
    var v = document.getElementById('empty-view');
    if (v) v.classList.remove('hidden');
    var d = document.getElementById('disc-view');
    if (d) d.classList.add('hidden');
  } catch (_) {}
}
