'use strict';
// ── Connect / disconnect ──
$('connect-btn').addEventListener('click', function () {
  if (!selectedServer) return;
  // If we have no pair code on hand AND we've never successfully
  // paired with this server before, prompt for the code instead of
  // firing a guaranteed-to-fail Connect.
  const key = selectedServer.host + ':' + selectedServer.port;
  const known = localStorage.getItem('vior_known_' + key) === '1';
  const pair = (($('manual-pair') as HTMLInputElement | null) && ($('manual-pair') as HTMLInputElement).value || '').trim();
  if (!known && !pair) { promptPair(); return; }
  reconnectAttempts = 0; doConnect();
});

function promptPair(): void {
  const m = $('pair-prompt');
  if (m) {
    m.classList.remove('hidden');
    const inp = $('pair-prompt-input') as HTMLInputElement | null;
    if (inp) { inp.value = ''; setTimeout(function () { try { inp.focus(); } catch (_) {} }, 60); }
  }
}
function closePair(): void { const m = $('pair-prompt'); if (m) m.classList.add('hidden'); }
document.addEventListener('click', function (e: MouseEvent) {
  const t = e.target as HTMLElement | null;
  if (t && t.id === 'pair-prompt') closePair();
});
if ($('pair-prompt-cancel')) $('pair-prompt-cancel').addEventListener('click', closePair);
if ($('pair-prompt-go')) $('pair-prompt-go').addEventListener('click', function () {
  const v = (($('pair-prompt-input') as HTMLInputElement).value || '').toUpperCase().trim();
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
});

let connectTimeoutId: ReturnType<typeof setTimeout> | null = null;
function doConnect(): void {
  setConnState('connecting');
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
  connectTimeoutId = setTimeout(function () {
    if (connected) return;
    try { if (ws) ws.close(); } catch (_) {}
    ws = null;
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
    ws!.send(JSON.stringify({ type: 'hello', data: {
      width: Math.round(screen.width * dpr), height: Math.round(screen.height * dpr),
      dpr: dpr, name: 'Vior Mobile', mode: selectedMode, pairCode: pair, deviceId: deviceID
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
      try { localStorage.setItem('vior_known_' + host + ':' + port, '1'); } catch (_) {}
      frameBaseUrl = 'http://' + host + ':' + port;
      $('connecting-overlay').classList.add('hidden');
      connected = true;
      setConnState('online');
      showView('connected');
      $('scard-name').textContent = serverName;
      $('scard-meta').textContent = serverPlatform || host;
      // Mode pill shows transport + display mode: "Wi-Fi · Extend"
      $('stat-mode').textContent = 'Wi-Fi · ' + (selectedMode === 'mirror' ? 'Mirror' : 'Extend');
      $('stat-res').textContent = serverRes;
      $('stat-status').textContent = 'Live';
      $('files-offline').classList.add('hidden');
      $('files-active').classList.remove('hidden');
      $('remote-offline').classList.add('hidden');
      $('remote-active').classList.remove('hidden');
      toast('success', 'Connected', (selectedMode === 'mirror' ? 'Mirroring' : 'Extended display') + ' on ' + serverName + '.');
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
      reconnectAttempts++;
      setConnState('reconnecting');
      $('recon-banner').classList.remove('hidden');
      $('recon-sub').textContent = 'attempt ' + reconnectAttempts + ' of ' + maxReconnect + ' · backing off';
      $('stat-status').textContent = 'Reconnecting';
      setTimeout(function () { if (connected) doConnect(); }, Math.min(1000 * Math.pow(2, reconnectAttempts - 1), 10000));
    } else if (connected) {
      connected = false;
      setConnState('offline');
      hideStream();
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
    }
  };

  ws.onerror = function () {};
}

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
  $('recon-banner').classList.add('hidden');
  $('files-offline').classList.remove('hidden');
  $('files-active').classList.add('hidden');
  $('remote-offline').classList.remove('hidden');
  $('remote-active').classList.add('hidden');
  $('connecting-overlay').classList.add('hidden');
  toast('info', 'Disconnected', 'Session ended.');
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
