'use strict';
// ── Connect / disconnect ──
//
// Note: the legacy `#connect-btn` (orange "Select a server" / "Connect"
// button at the bottom of the discovery view) is GONE. Server rows are
// now tap-to-connect; the discovery module's `initiateConnect()` routes
// straight here (with a pair-prompt detour for never-paired servers).

function promptPair(opts?: { preserveValue?: boolean }): void {
  viorState.set({ state: 'pairing' });
  const m = $('pair-prompt');
  if (m) {
    m.classList.remove('hidden');
    const inp = $('pair-prompt-input') as HTMLInputElement | null;
    if (inp) {
      // On a rejected code we keep the typed value so the user can fix a
      // typo instead of re-entering the whole code. Otherwise start fresh.
      if (!(opts && opts.preserveValue)) inp.value = '';
      setTimeout(function () {
        try {
          inp.focus();
          // Select the preserved value so the first keystroke overwrites
          // it, but a single edit is still possible.
          if (opts && opts.preserveValue) inp.select();
        } catch (_) {}
      }, 60);
    }
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
  // Digits-only — the live formatter already strips non-numeric input.
  const v = (($('pair-prompt-input') as HTMLInputElement).value || '')
    .replace(/[^0-9]/g, '').trim();
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

// Pair-only flow: user typed a 4-digit code, didn't pick a server.
// Probe every IP in the local /24 in parallel, ask each one's /info,
// match the pairCode field, connect to the first hit.
async function pairOnlyConnect(pair: string): Promise<void> {
  setConnState('connecting');
  ($('connecting-overlay') as HTMLElement).classList.remove('hidden');
  ($('conn-title') as HTMLElement).textContent = 'Finding Vior server';
  ($('conn-sub') as HTMLElement).innerHTML = 'Scanning Wi-Fi for the desktop with this code…';
  // Show the code we're hunting for in the match chip so the user can
  // confirm it against the desktop while the sweep runs.
  showConnMatch(pair);

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
  const probes: Promise<{ host: string; port: number; info: { paired?: boolean; name?: string; platform?: string } } | null>[] = [];
  for (let i = 1; i < 255; i++) {
    const host = base + '.' + i;
    probes.push((async function () {
      const ctrl = new AbortController();
      setTimeout(function () { ctrl.abort(); }, 1500);
      try {
        // Send the typed code as ?probe= so the server can confirm it's
        // the right one WITHOUT ever publishing the code. The desktop
        // returns {"paired":true} only on a constant-time match (and
        // rate-limits mismatches). The old flow read info.pairCode,
        // which meant the code was readable by anyone on the LAN.
        const r = await fetch('http://' + host + ':8080/info?probe=' + encodeURIComponent(pair), { signal: ctrl.signal });
        if (!r.ok) return null;
        const info = await r.json();
        if (info.paired === true) {
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
// Live keepalive instance. Recreated on every doConnect so a stale
// timer from a previous session can't fire pings into a fresh socket.
// Stopped from onclose AND doDisconnect.
let keepalive: ViorKeepalive | null = null;
// Banner update timer — polls msSinceLastPong every 1s so the header
// chip dot tracks pong freshness without us having to plumb a
// callback through the keepalive module. Lives for the duration of a
// connected WS; cleared in cleanupKeepalive.
let healthTickId: ReturnType<typeof setInterval> | null = null;

function cleanupKeepalive(): void {
  if (keepalive) { try { keepalive.stop(); } catch (_) { /* ignore */ } keepalive = null; }
  if (healthTickId) { clearInterval(healthTickId); healthTickId = null; }
}

// "Replaced by another session" pathway — set when the server sends
// {code:"occupied"}. We use this flag from onclose to suppress the
// reconnect-loop the user would otherwise see (two tabs ping-pong
// fighting over one server).
let replacedByOtherSession = false;
// Timestamp (ms) of the last successful 'ready'. Used to decide whether
// a drop followed a durable session (→ refill the reconnect budget) or
// a flaky never-quite-connected loop (→ keep counting toward the cap).
let lastReadyAt = 0;
// True from the moment a connect is initiated until 'ready' arrives or
// we give up. Lets onclose distinguish a drop DURING the handshake
// (socket opened, hello sent, but 'ready' never came) from an idle
// close — the former must still trigger reconnect even though
// `connected` is still false. Without this a handshake-window drop hung
// the UI on the spinner for 15s with no retry.
let connecting = false;

// Populate the connecting-overlay's mutual-match chip. Shows the pair
// code we're about to present (spaced + mono) so the user can eyeball
// it against the digits the desktop displays — the client half of a
// "does this match?" gesture. Hidden when the code is unknown (trusted
// auto-reconnect / DHCP fallback), where there's nothing to match.
function showConnMatch(pair: string): void {
  const wrap = document.getElementById('conn-match');
  const code = document.getElementById('conn-match-code');
  if (!wrap || !code) return;
  const digits = (pair || '').replace(/[^0-9]/g, '');
  if (digits.length >= PAIR_CODE_MIN) {
    // Space every digit so it reads as discrete numerals, matching how
    // the desktop presents the code. Letter-spacing (CSS) adds the gaps;
    // the raw digits stay copy-friendly.
    code.textContent = digits;
    wrap.classList.remove('hidden');
  } else {
    code.textContent = '';
    wrap.classList.add('hidden');
  }
}

function doConnect(): void {
  connecting = true;
  setConnState('connecting');
  viorState.set({ state: 'connecting', transport: 'wifi' });
  $('connecting-overlay').classList.remove('hidden');
  $('conn-title').textContent = 'Connecting';
  // Lead with WHAT we're connecting to — the host name — so the user can
  // confirm the target at a glance. The pair code (if any) is shown
  // separately in the match chip below.
  $('conn-sub').innerHTML = 'Establishing ' + esc(selectedMode) + ' session with<br><b>' + esc(serverName) + '</b>';
  // The pair code being presented lives on #manual-pair during the
  // connect flow (written by the cascade/pair-prompt/QR paths). Surface
  // it so the user can visually match it against the desktop.
  const pairForMatch = (($('manual-pair') as HTMLInputElement | null) && ($('manual-pair') as HTMLInputElement).value || '')
    .replace(/[^0-9]/g, '').trim();
  showConnMatch(pairForMatch);
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
    const pair = (($('manual-pair') as HTMLInputElement | null) && ($('manual-pair') as HTMLInputElement).value || '').replace(/[^0-9]/g, '').trim();
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
    // Friendly platform label so the desktop "Trusted Devices" UI can
    // show an "iOS" / "Android" / "Mobile" pill. Cheap UA sniff — the
    // value is never used for security, only display.
    const ua = navigator.userAgent || '';
    let platform = 'Mobile';
    if (/iPad|iPhone|iPod/.test(ua)) platform = 'iOS';
    else if (/Android/.test(ua)) platform = 'Android';
    ws!.send(JSON.stringify({ type: 'hello', data: {
      width: Math.round(screen.width * dpr), height: Math.round(screen.height * dpr),
      dpr: dpr, name: 'Vior Mobile', mode: selectedMode, pairCode: pair, deviceId: deviceID,
      intent: intent, skipDisplay: skipDisplay, platform: platform
    }}));
  };

  ws.onmessage = function (e: MessageEvent) {
    // Any inbound traffic is a liveness proof — refresh freshness first
    // (before parsing) so even a frame we can't decode still counts as
    // the link being alive.
    if (keepalive) keepalive.noteInbound();
    // Guard the parse: a single non-JSON frame (binary frame, truncated
    // frame after a Doze-suspended socket resumes, a proxy error page)
    // would otherwise throw out of the handler and silently kill the
    // message pump for the rest of the session.
    let msg: WSMessage;
    try {
      msg = JSON.parse(e.data as string) as WSMessage;
    } catch (_) {
      return;
    }
    if (msg.type === 'pong') {
      if (keepalive) keepalive.notePong();
      return;
    }
    if (msg.type === 'ping') {
      // Server-driven app-level ping (today the server doesn't issue
      // these — it uses gorilla's spec ping — but reply anyway so a
      // future symmetric heartbeat works without a client update).
      try { ws!.send(JSON.stringify({ type: 'pong' })); } catch (_) {}
      return;
    }
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
        // Successful connect resets the cascade memory — next launch
        // starts at step A again rather than dropping the user into
        // pair-code entry they no longer need.
        localStorage.removeItem('vior_last_entry_step');
      } catch (_) {}
      frameBaseUrl = 'http://' + host + ':' + port;
      $('connecting-overlay').classList.add('hidden');
      connected = true;
      connecting = false;
      // A completed handshake refills the reconnect budget. Previously
      // the counter was only reset in scattered callers, so transient
      // blips accumulated across independent-but-recoverable drops and
      // eventually exhausted the budget, dumping the user to discovery.
      reconnectAttempts = 0;
      lastReadyAt = Date.now();
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

      // Resume MJPEG polling if the stream was visible before reconnect.
      // Without this a Wi-Fi blip freezes the stream on the last frame.
      if (streamVisible) { startFramePolling(); }
      const successMsg = intentNow === 'remote' ? 'Remote control ready'
        : intentNow === 'files' ? 'Ready for file transfer'
        : (selectedMode === 'mirror' ? 'Mirroring' : 'Extended display');
      toast('success', 'Connected', successMsg + ' on ' + serverName + '.');

      // Spin up the app-level keepalive: pings every 15s, force-close
      // on missed pong, fast-ping on visibilitychange. Without this,
      // a backgrounded mobile silently loses TCP after a few minutes
      // of Doze and the user sees a green dot over a dead socket.
      cleanupKeepalive();
      const kpAttempts = reconnectAttempts;
      keepalive = viorKeepalive.create({
        onLost: function (reason) {
          console.warn('[ws] keepalive lost (' + reason + ') — forcing reconnect');
          // The keepalive already called ws.close() — onclose will run
          // the reconnect path. Reset the attempt counter so a
          // genuinely-fine connection doesn't burn its budget on the
          // first transient blip. Preserve the count if we were
          // already mid-reconnect.
          if (kpAttempts === 0) reconnectAttempts = 0;
        },
      });
      keepalive.attach(ws!);
      // 1s health tick → header dot tone. Cheap; one DOM mutation per
      // second, no layout impact (class swap on a single element).
      if (healthTickId) clearInterval(healthTickId);
      healthTickId = setInterval(function () {
        if (!keepalive) return;
        viorKeepalive.applyHealthTone(keepalive.msSinceLastPong());
      }, 1000);

      // Save resume metadata so the next cold app launch can skip the
      // pair prompt and head straight back to this server. We capture
      // the server's deviceId (from /info) async so the resume record
      // can also drive the DHCP-drift fallback on next boot.
      (async function () {
        try {
          const r = await fetch('http://' + host + ':' + port + '/info', {
            signal: AbortSignal.timeout ? AbortSignal.timeout(2000) : undefined,
          });
          const info = r.ok ? await r.json() : {};
          const deviceId = localStorage.getItem('vior_device_id') || '';
          viorKeepalive.saveResume({
            host, port, deviceId,
            serverDeviceId: (info && info.deviceId) || undefined,
            ts: Date.now(),
          });
          if (info && info.deviceId) {
            try { localStorage.setItem('vior_known_device_' + host + ':' + port, info.deviceId); } catch (_) {}
          }
        } catch (_) { /* best-effort */ }
      })();
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
        toast('error', 'Pair code rejected', 'Check the code shown on the desktop and try again.');
        // Preserve the typed code so the user can correct a single typo
        // instead of re-entering the whole thing.
        promptPair({ preserveValue: true });
      } else if (code === 'occupied') {
        // Second-tab scenario: the desktop server already has a WS
        // client. Surface a clean "you were replaced" message and stop
        // the reconnect-loop dance — racing the other tab is pointless
        // and looks broken. The replacing tab keeps the session.
        replacedByOtherSession = true;
        reconnectAttempts = maxReconnect; // suppress onclose retry
        toast('warning', 'Replaced by another session', 'Another device is using this desktop.');
        // Don't drop the server selection — user might want to retry
        // after the other side disconnects.
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
    // Clear stale file transfers — chunks from the previous session
    // will never complete. Without this, progress bars freeze forever.
    clearFileTransfers();
    // Always stop the keepalive before we either reconnect or give up
    // — the timers belong to the dead socket. doConnect re-creates a
    // fresh instance on the next 'ready'.
    cleanupKeepalive();
    if (replacedByOtherSession) {
      // Don't reconnect — the user was actively replaced. Reset for
      // future manual Connect attempts but stay quiet right now.
      replacedByOtherSession = false;
      connected = false;
      setConnState('offline');
      hideStream();
      viorState.set({ state: 'disconnected' });
      showView('disc');
      $('recon-banner').classList.add('hidden');
      $('files-offline').classList.remove('hidden');
      $('files-active').classList.add('hidden');
      $('remote-offline').classList.remove('hidden');
      $('remote-active').classList.add('hidden');
      $('connecting-overlay').classList.add('hidden');
      setTimeout(function () { startDiscovery(); }, 300);
      return;
    }
    if ((connected || connecting) && reconnectAttempts < maxReconnect) {
      // Transient drop (WiFi blip) OR a drop mid-handshake — either way
      // retry. Trust survives because we never remove vior_known_ here;
      // the saved deviceID re-admits us without a pair-code prompt.
      connected = false;
      connecting = false;
      reconnectAttempts++;
      setConnState('reconnecting');
      viorState.set({ state: 'reconnecting' });
      $('recon-banner').classList.remove('hidden');
      $('recon-sub').textContent = 'attempt ' + reconnectAttempts + ' of ' + maxReconnect + ' · backing off';
      $('stat-status').textContent = 'Reconnecting';
      // Visible header-chip text — "Reconnecting (2/5)…" so the user
      // can see we're working, not stuck. Overrides the state-machine
      // label in core/state.ts for this transitional moment.
      const lbl = document.getElementById('conn-label');
      if (lbl) lbl.textContent = 'Reconnecting (' + reconnectAttempts + '/' + maxReconnect + ')…';
      // Exponential backoff (cap 30s) with ±25% jitter. Jitter
      // desynchronizes retry waves so multiple clients don't all hammer
      // a just-restarted desktop at identical 1s/2s/4s boundaries.
      const backoffBase = Math.min(1000 * Math.pow(2, reconnectAttempts - 1), 30000);
      const jittered = backoffBase * (0.75 + Math.random() * 0.5);
      setTimeout(function () { doConnect(); }, jittered);
    } else if (connected || connecting) {
      connecting = false;
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
  cleanupKeepalive();
  // Clear the resume hint — the user explicitly said "stop", so we
  // shouldn't silently reattach on next boot. They can still tap the
  // server from discovery (which is faster than re-pairing thanks to
  // the deviceID round-trip).
  viorKeepalive.clearResume();
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

// ── Entry-mode toggle (Wi-Fi vs USB cable) ─────────────────────────
// The toggle now lives in the persistent app-shell pill above the
// content area (`#transport-toggle`), not inside the empty view, so
// users can flip transport mid-flow without backing out of whatever
// cascade step they're on. localStorage key is preserved
// (`vior_entry_mode`) so existing installs don't lose their preference.
function applyEntryMode(mode: 'wifi' | 'usb'): void {
  const wifi = document.getElementById('entry-wifi');
  const usb = document.getElementById('entry-usb');
  if (!wifi || !usb) return;
  if (mode === 'usb') { wifi.classList.add('hidden'); usb.classList.remove('hidden'); }
  else { usb.classList.add('hidden'); wifi.classList.remove('hidden'); }
  // Reflect into the new persistent toggle pill.
  const wifiBtn = document.getElementById('transport-wifi');
  const usbBtn = document.getElementById('transport-usb');
  if (wifiBtn) {
    wifiBtn.classList.toggle('active', mode === 'wifi');
    wifiBtn.setAttribute('aria-selected', mode === 'wifi' ? 'true' : 'false');
  }
  if (usbBtn) {
    usbBtn.classList.toggle('active', mode === 'usb');
    usbBtn.setAttribute('aria-selected', mode === 'usb' ? 'true' : 'false');
  }
  try { localStorage.setItem('vior_entry_mode', mode); } catch (_) {}
}
document.querySelectorAll<HTMLElement>('#transport-toggle .transport-btn').forEach(function (b) {
  b.addEventListener('click', function () {
    const m = (b.dataset.transport === 'usb') ? 'usb' : 'wifi';
    applyEntryMode(m);
  });
});
applyEntryMode(((localStorage.getItem('vior_entry_mode') as 'wifi' | 'usb') || 'wifi'));
// Re-apply whenever the empty view becomes visible — covers the case
// where USB disconnect flips back to discovery while user was in USB
// mode but the seg state didn't update mid-session.
(window as unknown as { syncEntryMode?: () => void }).syncEntryMode = function (): void {
  applyEntryMode(((localStorage.getItem('vior_entry_mode') as 'wifi' | 'usb') || 'wifi'));
};

// USB troubleshooting link — open Android USB settings intent.
const usbHelpBtn = document.getElementById('usb-help-btn');
if (usbHelpBtn) usbHelpBtn.addEventListener('click', function () {
  toast('info', 'USB checklist',
    'Use a data cable. Allow "Vior USB access" prompt. Restart Vior desktop if needed.');
});

// "Try again" button for USB hello-ack timeout. Calls into the Java
// bridge to resend the hello frame + reset the 3s timer.
const usbRetryBtn = document.getElementById('usb-retry-btn');
if (usbRetryBtn) usbRetryBtn.addEventListener('click', function () {
  const bridge = (window as unknown as { Android?: { usbRetryHello?: () => void } }).Android;
  // Optimistic UI: drop straight back to verifying — the timeout will
  // re-fire if the desktop is still silent.
  const setStage = (window as unknown as { setUsbStage?: (s: 'waiting' | 'verifying' | 'connected' | 'failed') => void }).setUsbStage;
  if (typeof setStage === 'function') setStage('verifying');
  if (bridge && typeof bridge.usbRetryHello === 'function') {
    try { bridge.usbRetryHello(); } catch (e) { console.error('usbRetryHello bridge', e); }
  } else {
    // No Java bridge (browser preview) — just fake-toast.
    toast('info', 'Retrying', 'Re-sending USB handshake…');
  }
});

// ── Progressive disclosure cascade (Wi-Fi entry) ───────────────────
// Steps:
//   A — scanning (auto)
//   B — no servers, primary "Scan QR Code", tiny escape → C
//   C — pair-code entry, primary "Connect", tiny escape → D
//   D — IP entry, primary "Connect", tiny escape ← C
// Refresh on B/C/D restarts the scan (goes back to A).
// Last-used step persisted in `vior_last_entry_step` so retry-after-
// app-restart skips the user ahead, unless a Wi-Fi connect succeeded
// (the connect path clears it).
type CascadeStep = 'a' | 'b' | 'c' | 'd';
const CASCADE_KEY = 'vior_last_entry_step';

function setCascadeStep(step: CascadeStep, opts?: { persist?: boolean }): void {
  (['a', 'b', 'c', 'd'] as CascadeStep[]).forEach(function (s) {
    const el = document.getElementById('cascade-' + s);
    if (el) el.classList.toggle('hidden', s !== step);
  });
  if (opts && opts.persist === false) return;
  try { localStorage.setItem(CASCADE_KEY, step); } catch (_) {}
}

function clearCascadeMemory(): void {
  try { localStorage.removeItem(CASCADE_KEY); } catch (_) {}
}

// Normalise pair-code input: strip everything that isn't a decimal
// digit and truncate to PAIR_CODE_MAX chars. The desktop derives a
// 6-digit code and accepts 4-8 digit overrides (pairOverrideRe =
// ^[0-9]{4,8}$ in internal/stream/stream.go), so the mobile side must
// accept the same range. Truncating to 4 here — the old value — silently
// dropped the last two digits of the real 6-digit code and made pairing
// impossible.
const PAIR_CODE_MIN = 4;
const PAIR_CODE_MAX = 8;
function normalisePairCode(raw: string): string {
  return (raw || '').replace(/[^0-9]/g, '').slice(0, PAIR_CODE_MAX);
}
function formatPairCode(raw: string): string {
  return normalisePairCode(raw);
}

// Permissive IP/URL parser. Accepts:
//   192.168.1.4
//   192.168.1.4:8080
//   http://192.168.1.4:8080
//   http://192.168.1.4:8080/?pair=ABC123
//   vior://192.168.1.4:8080/?pair=ABC123
//   vior://192.168.1.4
// Returns { host, port, pair? } or null on garbage. Pasted whitespace
// is trimmed; port defaults to 8080.
interface ParsedAddr { host: string; port: number; pair?: string }
function parseAddrInput(raw: string): ParsedAddr | null {
  let s = (raw || '').trim();
  if (!s) return null;
  // Strip a scheme if present, treat vior:// like http://.
  s = s.replace(/^\s*(vior|http|https):\/\//i, '');
  // Pull out a ?pair=… or ?code=… query if present, then drop the query.
  let pair: string | undefined;
  const q = s.indexOf('?');
  if (q !== -1) {
    const query = s.slice(q + 1);
    s = s.slice(0, q);
    const m = query.match(/(?:pair|code)=([0-9A-Za-z-]+)/);
    if (m) pair = normalisePairCode(m[1]);
  }
  // Drop any path component.
  const slash = s.indexOf('/');
  if (slash !== -1) s = s.slice(0, slash);
  // Now s should be host or host:port.
  const m = s.match(/^([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})(?::([0-9]{1,5}))?$/);
  if (!m) return null;
  const host = m[1];
  const port = m[2] ? parseInt(m[2], 10) : 8080;
  if (port < 1 || port > 65535) return null;
  return { host: host, port: port, pair: pair };
}

// Helper: from cascade-c input, build a connect attempt that pair-only
// probes the local /24.
function cascadeSubmitPair(raw: string): void {
  const pair = normalisePairCode(raw);
  if (pair.length < PAIR_CODE_MIN) {
    toast('error', 'Pair code too short', `Enter at least ${PAIR_CODE_MIN} digits.`);
    return;
  }
  const mp = document.getElementById('manual-pair') as HTMLInputElement | null;
  if (mp) mp.value = pair;
  clearCascadeMemory();
  pairOnlyConnect(pair);
}

// Helper: from cascade-d input, parse + connect.
function cascadeSubmitAddr(raw: string): void {
  const parsed = parseAddrInput(raw);
  if (!parsed) {
    toast('error', 'Invalid address', 'Use 192.168.1.4 or 192.168.1.4:8080.');
    return;
  }
  if (parsed.pair) {
    const mp = document.getElementById('manual-pair') as HTMLInputElement | null;
    if (mp) mp.value = parsed.pair;
  }
  clearCascadeMemory();
  selectServer(parsed.host, parsed.port, parsed.host, '');
  reconnectAttempts = 0;
  // If we already have a pair code in hand, trust the pre-pair path
  // straight to doConnect; otherwise let initiateConnect decide (which
  // routes via the pair-prompt modal for unknown servers).
  if (parsed.pair) { doConnect(); } else { initiateConnect(); }
}

// ── Wire cascade controls ─────────────────────────────────────────
const cascadeBScan = document.getElementById('cascade-b-scan');
if (cascadeBScan) cascadeBScan.addEventListener('click', function () {
  // Defer to QR module if loaded; otherwise show toast.
  const startQR = (window as unknown as { startQRScan?: () => void }).startQRScan
    || (window as unknown as { openQR?: () => void }).openQR;
  if (typeof startQR === 'function') {
    try { startQR(); } catch (e) { console.error('startQR', e); }
  } else {
    // Fall back to the legacy scan-qr-btn which the QR module listens to.
    const legacy = document.getElementById('scan-qr-btn');
    if (legacy) legacy.click();
    else toast('warning', 'QR scanner unavailable', 'Use Pair code or IP instead.');
  }
});
// "Enter code" — co-equal primary choice → pair-code entry (step C).
const cascadeBCode = document.getElementById('cascade-b-code');
if (cascadeBCode) cascadeBCode.addEventListener('click', function () {
  setCascadeStep('c');
  // Focus the pair input so the keyboard opens straight away.
  setTimeout(function () {
    const inp = document.getElementById('cascade-c-input') as HTMLInputElement | null;
    if (inp) { try { inp.focus(); } catch (_) { /* ignore */ } }
  }, 60);
});
// Now that pair-code is a co-equal primary on step B, the tiny escape
// drops straight to the IP path (step D) — the "having trouble?" tier.
const cascadeBNext = document.getElementById('cascade-b-next');
if (cascadeBNext) cascadeBNext.addEventListener('click', function () { setCascadeStep('d'); });
const cascadeBRefresh = document.getElementById('cascade-b-refresh');
if (cascadeBRefresh) cascadeBRefresh.addEventListener('click', function () {
  clearCascadeMemory(); setCascadeStep('a', { persist: false });
  try { startDiscovery(); } catch (_) {}
});

const cascadeCInput = document.getElementById('cascade-c-input') as HTMLInputElement | null;
const cascadeCGo = document.getElementById('cascade-c-go') as HTMLButtonElement | null;
if (cascadeCInput) {
  cascadeCInput.addEventListener('input', function () {
    cascadeCInput.value = formatPairCode(cascadeCInput.value);
    const ok = normalisePairCode(cascadeCInput.value).length >= PAIR_CODE_MIN;
    if (cascadeCGo) cascadeCGo.disabled = !ok;
  });
  cascadeCInput.addEventListener('keydown', function (e: Event) {
    const ke = e as KeyboardEvent;
    if (ke.key === 'Enter') { ke.preventDefault(); if (cascadeCGo && !cascadeCGo.disabled) cascadeCGo.click(); }
  });
}
if (cascadeCGo) cascadeCGo.addEventListener('click', function () {
  if (!cascadeCInput) return;
  cascadeSubmitPair(cascadeCInput.value);
});
const cascadeCNext = document.getElementById('cascade-c-next');
if (cascadeCNext) cascadeCNext.addEventListener('click', function () { setCascadeStep('d'); });
// Back to the first-choice moment (QR / code) so a user who tapped
// "Enter code" can switch to scanning without a dead-end.
const cascadeCBack = document.getElementById('cascade-c-back');
if (cascadeCBack) cascadeCBack.addEventListener('click', function () { setCascadeStep('b'); });

const cascadeDInput = document.getElementById('cascade-d-input') as HTMLInputElement | null;
const cascadeDGo = document.getElementById('cascade-d-go') as HTMLButtonElement | null;
if (cascadeDInput) {
  cascadeDInput.addEventListener('input', function () {
    const parsed = parseAddrInput(cascadeDInput.value);
    if (cascadeDGo) cascadeDGo.disabled = !parsed;
  });
  cascadeDInput.addEventListener('paste', function () {
    // Defer to next tick so .value reflects the pasted content, then
    // re-run validation. Allows pasting a vior:// URL or a full http URL.
    setTimeout(function () {
      const parsed = parseAddrInput(cascadeDInput.value);
      if (cascadeDGo) cascadeDGo.disabled = !parsed;
    }, 0);
  });
  cascadeDInput.addEventListener('keydown', function (e: Event) {
    const ke = e as KeyboardEvent;
    if (ke.key === 'Enter') { ke.preventDefault(); if (cascadeDGo && !cascadeDGo.disabled) cascadeDGo.click(); }
  });
}
if (cascadeDGo) cascadeDGo.addEventListener('click', function () {
  if (!cascadeDInput) return;
  cascadeSubmitAddr(cascadeDInput.value);
});
const cascadeDBack = document.getElementById('cascade-d-back');
if (cascadeDBack) cascadeDBack.addEventListener('click', function () { setCascadeStep('c'); });
const cascadeDRefresh = document.getElementById('cascade-d-refresh');
if (cascadeDRefresh) cascadeDRefresh.addEventListener('click', function () {
  clearCascadeMemory(); setCascadeStep('a', { persist: false });
  try { startDiscovery(); } catch (_) {}
});

// "Open Wi-Fi settings" corner link on the cascade — same intent the
// Settings sheet uses. Reuses the existing helper if available so
// behaviour stays in lock-step.
const cascadeWifiLink = document.getElementById('cascade-wifi-link');
if (cascadeWifiLink) cascadeWifiLink.addEventListener('click', function () {
  const fn = (window as unknown as { openWifiSettings?: () => void }).openWifiSettings;
  if (typeof fn === 'function') {
    try { fn(); return; } catch (_) { /* fall through */ }
  }
  const legacy = document.getElementById('open-wifi-settings');
  if (legacy) { legacy.click(); return; }
  toast('info', 'Wi-Fi settings', 'Open Settings → Wi-Fi on your device.');
});

// Cascade resume on boot: if the user previously bailed past step A,
// drop them back at the same step on next launch. Cleared on
// successful connect / explicit refresh.
(function resumeCascade(): void {
  try {
    const s = localStorage.getItem(CASCADE_KEY) as CascadeStep | null;
    if (s === 'b' || s === 'c' || s === 'd') {
      // Defer so showEmpty() running later still flips empty-view in.
      setTimeout(function () { setCascadeStep(s, { persist: false }); }, 0);
    }
  } catch (_) { /* localStorage blocked */ }
})();

// Public hook so the discovery module can drive cascade transitions
// (A → B when "no servers found" fires).
(window as unknown as { setCascadeStep?: (s: CascadeStep) => void }).setCascadeStep = function (s) {
  setCascadeStep(s);
};
(window as unknown as { clearCascadeMemory?: () => void }).clearCascadeMemory = clearCascadeMemory;

// ── Pair-prompt input: live digit-only formatting ──────────────────
// Pair code is 4-8 numeric digits (desktop derives 6, accepts 4-8
// overrides). Strip everything else and clamp to the max.
const pairInput = $('pair-prompt-input') as HTMLInputElement | null;
if (pairInput) {
  pairInput.addEventListener('input', function () {
    const raw = normalisePairCode(pairInput.value);
    if (pairInput.value !== raw) pairInput.value = raw;
  });
}

// ── USB transition helpers ─────────────────────────────────────────
// Tiny shim so usb.ts can flip the orb through its 4-state lifecycle
// without coupling to DOM details:
//   waiting   — no cable plugged yet
//   verifying — cable up, waiting for the Vior magic+version ack
//   connected — peer verified, transport promoted to USB
//   failed    — handshake timed out (3s) → recovery surface w/ retry
(window as unknown as {
  setUsbStage?: (s: 'waiting' | 'verifying' | 'connected' | 'failed') => void;
}).setUsbStage = function (s: 'waiting' | 'verifying' | 'connected' | 'failed'): void {
  const stage = document.getElementById('usb-stage');
  const title = document.getElementById('usb-title');
  const body = document.getElementById('usb-body');
  const retry = document.getElementById('usb-retry-btn');
  const checklist = document.getElementById('usb-checklist');
  if (stage) stage.setAttribute('data-state', s);
  switch (s) {
    case 'verifying':
      if (title) title.textContent = 'Verifying cable…';
      if (body) body.textContent = 'Cable detected. Checking the desktop is running Vior…';
      if (retry) retry.classList.add('hidden');
      if (checklist) checklist.classList.add('hidden');
      break;
    case 'connected':
      if (title) title.textContent = 'Cable detected!';
      if (body) body.textContent = 'Setting up…';
      if (retry) retry.classList.add('hidden');
      if (checklist) checklist.classList.add('hidden');
      break;
    case 'failed':
      if (title) title.textContent = 'Vior desktop not responding';
      if (body) body.textContent = 'Cable connected, but the Vior desktop app didn\'t reply. Make sure it\'s running, then try again.';
      if (retry) retry.classList.remove('hidden');
      if (checklist) checklist.classList.remove('hidden');
      break;
    case 'waiting':
    default:
      if (title) title.textContent = 'Waiting for cable…';
      if (body) body.textContent = 'Plug your tablet into the desktop. We start automatically — no IP, no pair code.';
      if (retry) retry.classList.add('hidden');
      if (checklist) checklist.classList.remove('hidden');
      break;
  }
};
