'use strict';
// ── Connect / disconnect ──
//
// Note: the legacy `#connect-btn` (orange "Select a server" / "Connect"
// button at the bottom of the discovery view) is GONE. Server rows are
// now tap-to-connect; the discovery module's `initiateConnect()` routes
// straight here (with a pair-prompt detour for never-paired servers).

function promptPair(): void {
  viorState.set({ state: 'pairing' });
  const m = $('pair-prompt');
  if (m) {
    m.classList.remove('hidden');
    const inp = $('pair-prompt-input') as HTMLInputElement | null;
    if (inp) { inp.value = ''; setTimeout(function () { try { inp.focus(); } catch (_) {} }, 60); }
  }
}
// Expose to globalThis so discovery.ts can call into us without
// import order races.
(globalThis as unknown as { promptPair?: () => void }).promptPair = promptPair;
function closePair(): void {
  const m = $('pair-prompt'); if (m) m.classList.add('hidden');
  // If the user cancels pairing without connecting, drop back to the
  // most useful pre-connect state (servers visible → found-server,
  // else scanning).
  const cur = viorState.get().state;
  if (cur === 'pairing') {
    viorState.set({ state: selectedServer ? 'found-server' : 'scanning' });
  }
}
document.addEventListener('click', function (e: MouseEvent) {
  const t = e.target as HTMLElement | null;
  if (t && t.id === 'pair-prompt') closePair();
});
if ($('pair-prompt-cancel')) $('pair-prompt-cancel').addEventListener('click', closePair);
if ($('pair-prompt-go')) $('pair-prompt-go').addEventListener('click', function () {
  // Live formatter inserts "ABC-123" — strip the dash before sending.
  const v = (($('pair-prompt-input') as HTMLInputElement).value || '')
    .toUpperCase().replace(/[^A-Z0-9]/g, '').trim();
  if (!v) return;
  ($('manual-pair') as HTMLInputElement).value = v;
  closePair();
  // Two paths:
  //   1. User already selected a discovered server → just connect with this pair.
  //   2. No server selected yet → fan-out probe the /24 subnet, find any host
  //      whose /info advertises this pairCode, then connect to that one.
  if (selectedServer) {
    reconnectAttempts = 0; doConnect();
  } else {
    pairOnlyConnect(v);
  }
});

// Pair-only flow: user typed a 6-char code, didn't pick a server.
// Probe every IP in the local /24 in parallel, ask each one's /info,
// match the pairCode field, connect to the first hit.
async function pairOnlyConnect(pair: string): Promise<void> {
  setConnState('connecting');
  ($('connecting-overlay') as HTMLElement).classList.remove('hidden');
  ($('conn-title') as HTMLElement).textContent = 'Finding Vior server';
  ($('conn-sub') as HTMLElement).innerHTML = 'Scanning Wi-Fi for pair code <b>' + esc(pair) + '</b>…';

  // Use the same getLocalIP helper discovery.ts uses. Inline-call here.
  const localIP: string | null = await new Promise(function (resolve) {
    try {
      const pc = new RTCPeerConnection({ iceServers: [] });
      pc.createDataChannel(''); pc.createOffer().then((o) => pc.setLocalDescription(o));
      let done = false;
      pc.onicecandidate = function (e: RTCPeerConnectionIceEvent) {
        if (done || !e.candidate) return;
        const m = e.candidate.candidate.match(/(\d+\.\d+\.\d+\.\d+)/);
        if (m && m[1] !== '0.0.0.0') { done = true; pc.close(); resolve(m[1]); }
      };
      setTimeout(function () { if (!done) { done = true; pc.close(); resolve(null); } }, 3000);
    } catch (_) { resolve(null); }
  });
  if (!localIP) {
    ($('connecting-overlay') as HTMLElement).classList.add('hidden');
    toast('error', 'Network unavailable', 'Could not detect your Wi-Fi IP.');
    setConnState('offline');
    return;
  }
  const base = localIP.split('.').slice(0, 3).join('.');
  const probes: Promise<{ host: string; port: number; info: { pairCode?: string; name?: string; platform?: string } } | null>[] = [];
  for (let i = 1; i < 255; i++) {
    const host = base + '.' + i;
    probes.push((async function () {
      const ctrl = new AbortController();
      setTimeout(function () { ctrl.abort(); }, 1500);
      try {
        const r = await fetch('http://' + host + ':8080/info', { signal: ctrl.signal });
        if (!r.ok) return null;
        const info = await r.json();
        if ((info.pairCode || '').toUpperCase() === pair.toUpperCase()) {
          return { host, port: 8080, info };
        }
      } catch (_) { /* timeout or refused */ }
      return null;
    })());
  }
  const found = (await Promise.all(probes)).filter(Boolean) as { host: string; port: number; info: { name?: string; platform?: string } }[];
  if (found.length === 0) {
    ($('connecting-overlay') as HTMLElement).classList.add('hidden');
    toast('error', 'Not found', 'No Vior server on this Wi-Fi has that pair code.');
    setConnState('offline');
    return;
  }
  const first = found[0];
  selectServer(first.host, first.port, first.info.name || first.host, first.info.platform || '');
  reconnectAttempts = 0;
  doConnect();
}
if ($('pair-prompt-input')) $('pair-prompt-input').addEventListener('keydown', function (e: Event) {
  const ke = e as KeyboardEvent;
  if (ke.key === 'Enter') { ke.preventDefault(); ($('pair-prompt-go') as HTMLButtonElement).click(); }
});
$('disconnect-btn').addEventListener('click', doDisconnect);
$('files-connect-btn').addEventListener('click', function () { switchTab('display'); });
$('remote-connect-btn').addEventListener('click', function () { switchTab('display'); });
$('conn-cancel').addEventListener('click', function () {
  if (connectTimeoutId) { clearTimeout(connectTimeoutId); connectTimeoutId = null; }
  if (ws) { ws.close(); ws = null; }
  $('connecting-overlay').classList.add('hidden');
  setConnState('offline');
  viorState.set({ state: selectedServer ? 'found-server' : 'scanning' });
});

let connectTimeoutId: ReturnType<typeof setTimeout> | null = null;
function doConnect(): void {
  setConnState('connecting');
  viorState.set({ state: 'connecting', transport: 'wifi' });
  $('connecting-overlay').classList.remove('hidden');
  $('conn-title').textContent = 'Connecting';
  $('conn-sub').innerHTML = 'Establishing ' + selectedMode + ' session with<br><b>' + esc(serverName) + '</b>';
  $('conn-bar').classList.remove('hidden');
  $('conn-spin-ring').style.display = '';
  $('conn-spin-core').classList.remove('failed');

  const host = selectedServer!.host, port = selectedServer!.port;
  ws = new WebSocket('ws://' + host + ':' + port + '/ws');

  // Hard 15s ceiling so the overlay can't hang forever when the server is
  // unreachable, firewalled, or the port is wrong. Cleared in onmessage(ready).
  if (connectTimeoutId) clearTimeout(connectTimeoutId);
  connectTimeoutId = setTimeout(async function () {
    if (connected) return;
    try { if (ws) ws.close(); } catch (_) {}
    ws = null;
    // Before giving up, try the DHCP-drift fallback: the desktop may
    // have moved to a new IP. We re-probe the /24 looking for a host
    // whose /info advertises the same server deviceId we last paired
    // with. If found, the fallback re-pins + reconnects transparently.
    const fb = (window as unknown as { viorDhcpFallback?: (h: string, p: number) => Promise<boolean> }).viorDhcpFallback;
    if (typeof fb === 'function') {
      const ok = await fb(host, port);
      if (ok) return;
    }
    // Bump a per-server failure counter; after 3 consecutive timeouts
    // the cached trust marker for this host:port is stale (server moved,
    // re-installed, or just gone). Clear it so the next Connect tap
    // prompts for the pair code instead of silently failing.
    try {
      const fkey = 'vior_fail_' + host + ':' + port;
      const n = (parseInt(localStorage.getItem(fkey) || '0', 10) || 0) + 1;
      localStorage.setItem(fkey, String(n));
      if (n >= 3) {
        localStorage.removeItem('vior_known_' + host + ':' + port);
        localStorage.removeItem('vior_known_device_' + host + ':' + port);
        localStorage.removeItem(fkey);
        toast('warning', 'Pairing cleared', 'Repeated failures — re-enter the pair code next time.');
      }
    } catch (_) { /* localStorage blocked */ }
    $('connecting-overlay').classList.add('hidden');
    setConnState('offline');
    toast('error', 'Connection timed out', 'No response in 15s — check the IP, port, and that the desktop server is running.');
  }, 15000);

  ws.onopen = function () {
    const dpr = window.devicePixelRatio || 1;
    const pair = (($('manual-pair') as HTMLInputElement | null) && ($('manual-pair') as HTMLInputElement).value || '').toUpperCase().trim();
    // Stable per-install device ID — once the server trusts us, we never
    // need to re-enter the pair code from this app install again.
    let deviceID = localStorage.getItem('vior_device_id');
    if (!deviceID) {
      deviceID = 'mob-' + ((window.crypto && crypto.randomUUID) ? crypto.randomUUID() : (Math.random().toString(36).slice(2) + Date.now().toString(36)));
      try { localStorage.setItem('vior_device_id', deviceID); } catch (_) {}
    }
    // Intent decides whether the server even creates a virtual display.
    // Remote-only / Files-only intents must NOT trigger capture — the
    // user picked them precisely because they don't want a second screen.
    const intentFn = (window as unknown as { viorIntent?: () => 'display' | 'remote' | 'files' }).viorIntent;
    const intent = (typeof intentFn === 'function' ? intentFn() : 'display');
    const skipDisplay = intent === 'remote' || intent === 'files';
    ws!.send(JSON.stringify({ type: 'hello', data: {
      width: Math.round(screen.width * dpr), height: Math.round(screen.height * dpr),
      dpr: dpr, name: 'Vior Mobile', mode: selectedMode, pairCode: pair, deviceId: deviceID,
      intent: intent, skipDisplay: skipDisplay
    }}));
  };

  ws.onmessage = function (e: MessageEvent) {
    const msg = JSON.parse(e.data as string) as WSMessage;
    if (msg.type === 'ready') {
      if (connectTimeoutId) { clearTimeout(connectTimeoutId); connectTimeoutId = null; }
      const data = msg.data as { resolution: string };
      const res = data.resolution.split('x');
      displayW = parseInt(res[0]); displayH = parseInt(res[1]);
      serverRes = data.resolution.replace('x', ' × ');
      localStorage.setItem('vior_last', host + ':' + port);
      // Mark this server as 'known' client-side so the next Connect tap
      // skips the pair-code prompt — the server already trusts us via
      // the deviceID round-trip, this just reflects that in the UI.
      try {
        localStorage.setItem('vior_known_' + host + ':' + port, '1');
        // Clear the consecutive-failure counter — we just succeeded.
        localStorage.removeItem('vior_fail_' + host + ':' + port);
      } catch (_) {}
      frameBaseUrl = 'http://' + host + ':' + port;
      $('connecting-overlay').classList.add('hidden');
      connected = true;
      setConnState('online');
      // Tell the state machine: tab bar lights up, ops mode selector
      // appears in the connected card, header chip flips to "Connected · …".
      viorState.set({ state: 'connected', serverName: serverName, transport: 'wifi' });
      showView('connected');
      $('scard-name').textContent = serverName;
      $('scard-meta').textContent = serverPlatform || host;
      // Mode pill: shows transport + intent or display mode.
      const intentFn2 = (window as unknown as { viorIntent?: () => 'display' | 'remote' | 'files' }).viorIntent;
      const intentNow = (typeof intentFn2 === 'function' ? intentFn2() : 'display');
      let modeLabel: string;
      if (intentNow === 'remote') modeLabel = 'Wi-Fi · Remote';
      else if (intentNow === 'files') modeLabel = 'Wi-Fi · Files';
      else modeLabel = 'Wi-Fi · ' + (selectedMode === 'mirror' ? 'Mirror' : 'Extend');
      $('stat-mode').textContent = modeLabel;
      $('stat-res').textContent = serverRes;
      $('stat-status').textContent = intentNow === 'files' ? 'Ready' : 'Live';
      $('files-offline').classList.add('hidden');
      $('files-active').classList.remove('hidden');
      $('remote-offline').classList.add('hidden');
      $('remote-active').classList.remove('hidden');
      // Files intent → land on Files tab. Remote intent → land on Remote tab.
      // Display intent stays put (user already wants to see the connected card).
      if (intentNow === 'files') switchTab('files');
      else if (intentNow === 'remote') switchTab('remote');
      const successMsg = intentNow === 'remote' ? 'Remote control ready'
        : intentNow === 'files' ? 'Ready for file transfer'
        : (selectedMode === 'mirror' ? 'Mirroring' : 'Extended display');
      toast('success', 'Connected', successMsg + ' on ' + serverName + '.');
    } else if (msg.type === 'error') {
      if (connectTimeoutId) { clearTimeout(connectTimeoutId); connectTimeoutId = null; }
      $('connecting-overlay').classList.add('hidden');
      const errData = msg.data as { code?: string; message?: string } | undefined;
      const code = errData?.code || '';
      const errMsg = errData?.message || 'Check both devices on same Wi-Fi. Try manual IP.';
      // Server-side pair check failed → forget the bad pair, re-open the
      // pair-prompt modal so the user can type a fresh code without
      // hunting through Settings.
      if (code === 'pair_mismatch') {
        try { (($('manual-pair') as HTMLInputElement) || {}).value = ''; } catch (_) {}
        if (selectedServer) {
          try { localStorage.removeItem('vior_known_' + selectedServer.host + ':' + selectedServer.port); } catch (_) {}
        }
        toast('error', 'Pair code rejected', 'Enter the 6-character code shown on the desktop.');
        promptPair();
      } else {
        toast('error', 'Connection failed', errMsg);
      }
      setConnState('offline');
      viorState.set({ state: selectedServer ? 'found-server' : 'scanning' });
    } else if (msg.type && msg.type.indexOf('file-') === 0) {
      try { handleFileMessage(msg); } catch (e) { console.error('file msg', e); }
    } else if (msg.type === 'incoming-file') {
      // Desktop → mobile HTTP-download path: notification only; the body
      // is fetched via GET /download/{id} once the user accepts.
      try { handleIncomingFile(msg as { type: 'incoming-file'; data: unknown }); } catch (e) { console.error('incoming-file', e); }
    }
  };

  ws.onclose = function () {
    stopFramePolling();
    if (connected && reconnectAttempts < maxReconnect) {
      // Transient drop (WiFi blip) — trust survives because we never
      // remove vior_known_ here; the saved deviceID re-admits us
      // without a pair-code prompt. Exponential back-off keeps us
      // under 10s between attempts.
      reconnectAttempts++;
      setConnState('reconnecting');
      viorState.set({ state: 'reconnecting' });
      $('recon-banner').classList.remove('hidden');
      $('recon-sub').textContent = 'attempt ' + reconnectAttempts + ' of ' + maxReconnect + ' · backing off';
      $('stat-status').textContent = 'Reconnecting';
      setTimeout(function () { if (connected) doConnect(); }, Math.min(1000 * Math.pow(2, reconnectAttempts - 1), 10000));
    } else if (connected) {
      connected = false;
      setConnState('offline');
      hideStream();
      // Brief disconnected toast + auto-route back to scanning so the
      // user can re-pick a server without hunting through tabs.
      viorState.set({ state: 'disconnected' });
      showView('disc');
      $('recon-banner').classList.add('hidden');
      $('files-offline').classList.remove('hidden');
      $('files-active').classList.add('hidden');
      $('remote-offline').classList.remove('hidden');
      $('remote-active').classList.add('hidden');
      toast('error', 'Disconnected', 'Connection lost.');
      $('connecting-overlay').classList.add('hidden');
      $('conn-spin-ring').style.display = 'none';
      $('conn-spin-core').classList.add('failed');
      // Resume discovery so the bar fills up with servers again.
      setTimeout(function () { startDiscovery(); }, 300);
    }
  };

  ws.onerror = function () {};
}

// ── DHCP-drift fallback ────────────────────────────────────────────
// When the previously-known IP doesn't answer, sweep the local /24
// looking for any host whose /info advertises the same deviceId we
// last paired with. If found, transparently update the cached IP and
// reconnect. The user sees a quiet "Server IP updated" toast instead
// of a Connect-failed dead-end.
async function tryDhcpFallback(prevHost: string, prevPort: number): Promise<boolean> {
  const knownDeviceId = localStorage.getItem('vior_known_device_' + prevHost + ':' + prevPort);
  if (!knownDeviceId) return false;

  const localIP: string | null = await new Promise(function (resolve) {
    try {
      const pc = new RTCPeerConnection({ iceServers: [] });
      pc.createDataChannel(''); pc.createOffer().then((o) => pc.setLocalDescription(o));
      let done = false;
      pc.onicecandidate = function (e: RTCPeerConnectionIceEvent) {
        if (done || !e.candidate) return;
        const m = e.candidate.candidate.match(/(\d+\.\d+\.\d+\.\d+)/);
        if (m && m[1] !== '0.0.0.0') { done = true; pc.close(); resolve(m[1]); }
      };
      setTimeout(function () { if (!done) { done = true; pc.close(); resolve(null); } }, 3000);
    } catch (_) { resolve(null); }
  });
  if (!localIP) return false;

  const base = localIP.split('.').slice(0, 3).join('.');
  const probes: Promise<{ host: string; info: { deviceId?: string; name?: string; platform?: string } } | null>[] = [];
  for (let i = 1; i < 255; i++) {
    const host = base + '.' + i;
    if (host === prevHost) continue;
    probes.push((async function () {
      const ctrl = new AbortController();
      setTimeout(function () { ctrl.abort(); }, 1200);
      try {
        const r = await fetch('http://' + host + ':' + prevPort + '/info', { signal: ctrl.signal });
        if (!r.ok) return null;
        const info = await r.json();
        if ((info.deviceId || '') === knownDeviceId) return { host, info };
      } catch (_) { /* ignore */ }
      return null;
    })());
  }
  const found = (await Promise.all(probes)).filter(Boolean) as { host: string; info: { name?: string; platform?: string } }[];
  if (found.length === 0) return false;

  const next = found[0];
  // Migrate trust cache to the new IP.
  try {
    localStorage.setItem('vior_known_' + next.host + ':' + prevPort, '1');
    localStorage.setItem('vior_known_device_' + next.host + ':' + prevPort, knownDeviceId);
    localStorage.setItem('vior_last', next.host + ':' + prevPort);
    localStorage.removeItem('vior_known_' + prevHost + ':' + prevPort);
    localStorage.removeItem('vior_known_device_' + prevHost + ':' + prevPort);
  } catch (_) { /* localStorage blocked */ }
  selectServer(next.host, prevPort, next.info.name || next.host, next.info.platform || '');
  toast('info', 'Server IP updated', prevHost + ' → ' + next.host);
  reconnectAttempts = 0;
  doConnect();
  return true;
}

// Expose the fallback for the discovery/onclose paths that may want
// to try it without duplicating the whole probe routine.
(window as unknown as { viorDhcpFallback?: (h: string, p: number) => Promise<boolean> }).viorDhcpFallback = tryDhcpFallback;

function doDisconnect(): void {
  // User-initiated disconnect: cancel any pending reconnect, close the
  // WS, reset all state. Crucially we ALSO reset reconnectAttempts back
  // to 0 so the next user-initiated connect starts fresh — without this
  // the second tap of Connect refused to retry on transient failure
  // because the counter was still pinned at maxReconnect.
  reconnectAttempts = 0;
  if (connectTimeoutId) { clearTimeout(connectTimeoutId); connectTimeoutId = null; }
  connected = false;
  if (ws) { try { ws.close(); } catch (_) {} ws = null; }
  hideStream();
  showView('disc');
  setConnState('offline');
  // Tab bar collapses again, ops mode disappears — handled by the
  // state-machine subscriber in main / state.
  viorState.set({ state: 'disconnected' });
  $('recon-banner').classList.add('hidden');
  $('files-offline').classList.remove('hidden');
  $('files-active').classList.add('hidden');
  $('remote-offline').classList.remove('hidden');
  $('remote-active').classList.add('hidden');
  $('connecting-overlay').classList.add('hidden');
  toast('info', 'Disconnected', 'Session ended.');
  setTimeout(function () { startDiscovery(); }, 300);
}

// Pair-only entry from the Empty view — opens the pair-prompt modal
// without requiring a server selection. promptPair() handles focus +
// reveal; pair-prompt-go falls through to pairOnlyConnect() when no
// server is selected (see handler above).
const pairOnlyBtn = $('pair-only-btn');
if (pairOnlyBtn) pairOnlyBtn.addEventListener('click', function () { promptPair(); });

// ── Entry-mode toggle (Wi-Fi vs USB cable) ─────────────────────────
// Switches the empty-view body without changing actual transport.
// USB callbacks fire from native regardless; this just hides
// irrelevant fields so USB users don't see IP/pair prompts they
// can't use.
function applyEntryMode(mode: 'wifi' | 'usb'): void {
  const wifi = $('entry-wifi');
  const usb = $('entry-usb');
  if (!wifi || !usb) return;
  if (mode === 'usb') { wifi.classList.add('hidden'); usb.classList.remove('hidden'); }
  else { usb.classList.add('hidden'); wifi.classList.remove('hidden'); }
  const seg = $('entry-mode-seg');
  if (seg) {
    seg.querySelectorAll<HTMLElement>('.seg-btn').forEach(function (b) {
      b.classList.toggle('active', b.dataset.entry === mode);
    });
  }
  localStorage.setItem('vior_entry_mode', mode);
}
const entryModeSeg = $('entry-mode-seg');
if (entryModeSeg) {
  entryModeSeg.querySelectorAll<HTMLElement>('.seg-btn').forEach(function (b) {
    b.addEventListener('click', function () {
      const m = (b.dataset.entry === 'usb') ? 'usb' : 'wifi';
      applyEntryMode(m);
    });
  });
}
applyEntryMode(((localStorage.getItem('vior_entry_mode') as 'wifi' | 'usb') || 'wifi'));
// Re-apply whenever the empty view becomes visible — covers the case
// where USB disconnect flips back to discovery while user was in USB
// mode but the seg state didn't update mid-session.
(window as unknown as { syncEntryMode?: () => void }).syncEntryMode = function (): void {
  applyEntryMode(((localStorage.getItem('vior_entry_mode') as 'wifi' | 'usb') || 'wifi'));
};

// USB troubleshooting link — open Android USB settings intent.
const usbHelpBtn = $('usb-help-btn');
if (usbHelpBtn) usbHelpBtn.addEventListener('click', function () {
  toast('info', 'USB checklist',
    'Use a data cable. Allow "Vior USB access" prompt. Restart Vior desktop if needed.');
});

// ── Manual-setup disclosure (Wi-Fi screen) ─────────────────────────
// 90% of users connect via auto-discovery + tap. Hide the IP / QR /
// pair-only block behind a chevron so the empty view stays calm.
const manualToggle = $('manual-toggle');
if (manualToggle) {
  manualToggle.addEventListener('click', function () {
    const block = $('manual-block');
    if (!block) return;
    const open = !block.classList.contains('hidden');
    block.classList.toggle('hidden', open);
    manualToggle.setAttribute('aria-expanded', open ? 'false' : 'true');
  });
}

// ── Pair-prompt input: live ABC-123 formatting ─────────────────────
// We accept up to 7 chars (6 + the dash). On every keystroke we strip
// non-alphanumeric and re-insert the dash so the user sees the same
// "ABC-123" shape as the desktop's pair-code display.
const pairInput = $('pair-prompt-input') as HTMLInputElement | null;
if (pairInput) {
  pairInput.addEventListener('input', function () {
    const raw = (pairInput.value || '').toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 6);
    const formatted = raw.length > 3 ? raw.slice(0, 3) + '-' + raw.slice(3) : raw;
    if (pairInput.value !== formatted) pairInput.value = formatted;
  });
}
// Strip the dash before sending to manual-pair / WS.
const pairGo = $('pair-prompt-go');
if (pairGo) {
  // Wrap the existing onclick chain: existing handler reads .value and
  // upper-cases it — by the time it runs we want only A-Z0-9. Replace
  // .value just before the existing click fires by overriding it here.
  // We can't reorder listeners cleanly, so we normalize in the handler
  // above; the existing one already uppercases + trims.
}

// ── USB transition helpers ─────────────────────────────────────────
// Tiny shim so usb.ts can flip the orb between "waiting" and
// "connected" states without coupling to DOM details.
(window as unknown as { setUsbStage?: (s: 'waiting' | 'connected') => void }).setUsbStage = function (s: 'waiting' | 'connected'): void {
  const stage = $('usb-stage');
  const title = $('usb-title');
  const body = $('usb-body');
  if (stage) stage.setAttribute('data-state', s);
  if (s === 'connected') {
    if (title) title.textContent = 'Cable detected!';
    if (body) body.textContent = 'Setting up…';
  } else {
    if (title) title.textContent = 'Waiting for cable…';
    if (body) body.textContent = 'Plug your tablet into the desktop. We start automatically — no IP, no pair code.';
  }
};
