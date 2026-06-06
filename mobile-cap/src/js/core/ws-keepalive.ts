// ── WebSocket keepalive + reconnect helpers ─────────────────────────
//
// Why this exists
// ───────────────
// Android Doze and App Standby aggressively suspend timers and TCP
// sockets when the app is backgrounded or the screen locks. The
// WebSocket can appear connected for tens of seconds after the
// underlying TCP is dead — every onmessage callback never fires, every
// send() returns "ok" but never lands. The user comes back to the app,
// sees a green dot, taps a button, nothing happens, decides "the
// connection is broken" and disconnects.
//
// We can't ask the browser to emit a WebSocket-spec ping frame (the
// API doesn't expose it). So we drive an app-level MsgPing every
// PING_INTERVAL_MS, and if the server doesn't reply with MsgPong
// within PONG_TIMEOUT_MS, we force-close the socket — the consumer's
// existing onclose path then kicks off reconnect with backoff.
//
// On document.visibilitychange to 'visible' after >5s hidden we
// IMMEDIATELY ping with a tighter 3s window — the screen-lock case is
// where the user is most likely to notice a stale connection, so we
// pay a small extra packet to discover it before they tap anything.
//
// The cached-resume helpers (saveResume / loadResume) store enough
// metadata in localStorage that a cold app launch can skip pair
// re-entry and head straight back to the last server.

'use strict';

// Pulse cadence. 15s is short enough that Doze rarely manages to
// suspend the JS event loop between two pings, but long enough that
// we're not flooding the cable with pongs.
const PING_INTERVAL_MS = 15_000;

// Standard pong window. 20s is comfortably under the server's 40s
// read deadline and gives a slow-but-not-dead network (5G handover,
// captive-portal pause) a chance to recover.
const PONG_TIMEOUT_MS = 20_000;

// Tighter pong window used right after visibilitychange→visible.
// If the screen has been locked the WS is most likely dead; we want
// to discover that in the time it takes the user to read the screen.
const VISIBILITY_PONG_TIMEOUT_MS = 3_000;

// "Hidden for long enough that we should treat coming back as
// suspicious." Anything under 5s is almost certainly the user just
// scrolling the notification shade.
const VISIBILITY_REVIVE_THRESHOLD_MS = 5_000;

interface KeepaliveCallbacks {
  // Force the WS into reconnect mode. Caller owns the actual close
  // (we only ever call .close() on the socket we were given).
  onLost: (reason: string) => void;
}

interface Keepalive {
  attach(ws: WebSocket): void;
  // Call when an app-level MsgPong arrives — refreshes the deadline.
  notePong(): void;
  // Call from the onmessage handler so any inbound traffic counts as
  // liveness (the user actively typing is proof the link works).
  noteInbound(): void;
  // Stop all timers + handlers. Idempotent.
  stop(): void;
  // Returns ms since the last pong (or null if no ping yet).
  msSinceLastPong(): number | null;
}

function createKeepalive(cb: KeepaliveCallbacks): Keepalive {
  let socket: WebSocket | null = null;
  let pingTimer: ReturnType<typeof setInterval> | null = null;
  let pongDeadlineTimer: ReturnType<typeof setTimeout> | null = null;
  let lastPongAt = 0;
  let hiddenSince = 0;
  let stopped = false;

  function clearPongDeadline(): void {
    if (pongDeadlineTimer) {
      clearTimeout(pongDeadlineTimer);
      pongDeadlineTimer = null;
    }
  }

  function armPongDeadline(ms: number, why: string): void {
    clearPongDeadline();
    pongDeadlineTimer = setTimeout(function () {
      pongDeadlineTimer = null;
      console.warn('[ws] pong timeout (' + why + ') — forcing close');
      try {
        if (socket && socket.readyState === WebSocket.OPEN) socket.close();
      } catch (_) { /* ignore */ }
      cb.onLost('pong-timeout-' + why);
    }, ms);
  }

  function sendPing(why: string, deadlineMs: number): void {
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    try {
      socket.send(JSON.stringify({ type: 'ping' }));
      armPongDeadline(deadlineMs, why);
    } catch (e) {
      console.warn('[ws] ping send failed:', e);
      cb.onLost('ping-send-failed');
    }
  }

  function onVisibilityChange(): void {
    if (stopped) return;
    if (document.visibilityState === 'hidden') {
      hiddenSince = Date.now();
      return;
    }
    // visible
    const hiddenFor = hiddenSince ? Date.now() - hiddenSince : 0;
    hiddenSince = 0;
    if (hiddenFor < VISIBILITY_REVIVE_THRESHOLD_MS) return;
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    console.log('[ws] back from background after ' + hiddenFor + 'ms — fast ping');
    sendPing('visibility', VISIBILITY_PONG_TIMEOUT_MS);
  }

  function onNetworkChange(): void {
    if (stopped) return;
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    console.log('[ws] network change detected — forcing reconnect');
    try { socket.close(); } catch (_) { /* ignore */ }
    cb.onLost('network-change');
  }

  document.addEventListener('visibilitychange', onVisibilityChange);
  // navigator.connection is non-standard but present on Chromium-based
  // Android WebViews. Fall back silently when absent.
  type ConnLike = { addEventListener?: (t: string, cb: () => void) => void;
                    removeEventListener?: (t: string, cb: () => void) => void };
  const conn = (navigator as unknown as { connection?: ConnLike }).connection;
  if (conn && typeof conn.addEventListener === 'function') {
    conn.addEventListener('change', onNetworkChange);
  }
  // Generic browser online/offline is a useful belt-and-braces signal
  // (it fires for plane mode toggles, Wi-Fi reassociation on iOS).
  window.addEventListener('online', onNetworkChange);
  window.addEventListener('offline', function () { onNetworkChange(); });

  return {
    attach(ws: WebSocket): void {
      socket = ws;
      lastPongAt = Date.now();
      if (pingTimer) clearInterval(pingTimer);
      pingTimer = setInterval(function () {
        sendPing('interval', PONG_TIMEOUT_MS);
      }, PING_INTERVAL_MS);
    },
    notePong(): void {
      lastPongAt = Date.now();
      clearPongDeadline();
    },
    noteInbound(): void {
      // In-band traffic proves the link works in both directions
      // (server received our previous send, we got its reply). Treat
      // as a pong for freshness accounting; don't clear the explicit
      // ping deadline though — we still want to learn that a ping we
      // sent went unanswered, because that's the only signal the
      // socket is half-open (we can send but not receive).
      lastPongAt = Date.now();
    },
    stop(): void {
      stopped = true;
      if (pingTimer) { clearInterval(pingTimer); pingTimer = null; }
      clearPongDeadline();
      document.removeEventListener('visibilitychange', onVisibilityChange);
      if (conn && typeof conn.removeEventListener === 'function') {
        conn.removeEventListener('change', onNetworkChange);
      }
      window.removeEventListener('online', onNetworkChange);
      socket = null;
    },
    msSinceLastPong(): number | null {
      if (!lastPongAt) return null;
      return Date.now() - lastPongAt;
    },
  };
}

// ── Resume metadata ───────────────────────────────────────────────
//
// What we cache (per host:port):
//   • deviceId — stable per-install mobile ID (re-used across resume)
//   • server   — the host:port we last connected to
//   • known    — boolean (this server has trusted us before)
//   • pair     — optional, only if we were pair-promoted but haven't
//     been added to trust yet (covers the first-connect window)
// The "vior_resume" object is read at boot to skip the pair-entry
// path entirely when we have everything we need.

interface ResumeRecord {
  host: string;
  port: number;
  deviceId: string;
  serverDeviceId?: string;
  pair?: string;
  ts: number;
}

const RESUME_KEY = 'vior_resume';

function saveResume(r: ResumeRecord): void {
  try {
    localStorage.setItem(RESUME_KEY, JSON.stringify(r));
  } catch (_) { /* localStorage blocked (private mode) */ }
}

function loadResume(): ResumeRecord | null {
  try {
    const raw = localStorage.getItem(RESUME_KEY);
    if (!raw) return null;
    const r = JSON.parse(raw) as ResumeRecord;
    if (!r || typeof r.host !== 'string' || typeof r.port !== 'number') return null;
    return r;
  } catch (_) { return null; }
}

function clearResume(): void {
  try { localStorage.removeItem(RESUME_KEY); } catch (_) { /* ignore */ }
}

// ── Status indicator helper ───────────────────────────────────────
//
// Single source of truth for the header-chip dot tone. Called from
// connect.ts whenever the keepalive ticks or a state transition lands.
//   green pulse  → pong < 5s ago
//   yellow       → pong 5-20s ago
//   red          → pong > 20s ago (reconnecting)
function applyHealthTone(msSincePong: number | null): void {
  const dot = document.getElementById('conn-dot');
  if (!dot) return;
  dot.classList.remove('dot-ok', 'dot-warn', 'dot-err', 'dot-pulse');
  if (msSincePong === null) {
    dot.classList.add('dot-idle');
    return;
  }
  if (msSincePong < 5_000) {
    dot.classList.add('dot-ok', 'dot-pulse');
  } else if (msSincePong < 20_000) {
    dot.classList.add('dot-warn', 'dot-pulse');
  } else {
    dot.classList.add('dot-err');
  }
}

// Publish for connect.ts and any other module to use.
(globalThis as unknown as {
  viorKeepalive: {
    create: typeof createKeepalive;
    saveResume: typeof saveResume;
    loadResume: typeof loadResume;
    clearResume: typeof clearResume;
    applyHealthTone: typeof applyHealthTone;
  };
}).viorKeepalive = {
  create: createKeepalive,
  saveResume,
  loadResume,
  clearResume,
  applyHealthTone,
};
