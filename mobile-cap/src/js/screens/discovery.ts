'use strict';
// ── Discovery ──
//
// Drives the pre-connect "scanning → found-server" sub-machine. No
// Mirror/Extend, no Connect button. Tapping a row goes straight to
// connect (or pair-prompt if the server hasn't been trusted yet).
let foundServers: Record<string, ServerInfo> = {};
let discoveryTimeout: ReturnType<typeof setTimeout> | null = null;
let autoConnectTimer: ReturnType<typeof setTimeout> | null = null;
let scanning = false;

// Toggle the refresh button into a busy state during a sweep so the tap
// registers visibly — otherwise a re-scan looks like nothing happened.
function setRefreshBusy(busy: boolean): void {
  const btn = document.getElementById('disc-refresh') as HTMLButtonElement | null;
  if (!btn) return;
  btn.disabled = busy;
  btn.setAttribute('aria-busy', busy ? 'true' : 'false');
  btn.classList.toggle('scanning', busy);
  // The label sits as a bare text node after the SVG icon; rewrite just
  // that node so we don't clobber the icon.
  for (let i = 0; i < btn.childNodes.length; i++) {
    const n = btn.childNodes[i];
    if (n.nodeType === Node.TEXT_NODE && (n.textContent || '').trim()) {
      n.textContent = busy ? ' Scanning… ' : ' Refresh ';
      break;
    }
  }
}

// Central end-of-sweep settle: clears the busy affordance. Idempotent.
function finishScan(): void {
  scanning = false;
  setRefreshBusy(false);
}

function startDiscovery(): void {
  foundServers = {}; selectedServer = null; scanning = true;
  setRefreshBusy(true);
  if (localStorage.getItem('vior_wifi') === '0' || localStorage.getItem('vior_usb_only') === '1') {
    $('disc-status').textContent = 'Wi-Fi discovery off';
    $('disc-list').innerHTML = '<div class="empty"><span class="empty-icon"><svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M1 1l22 22M16.72 11.06A10.94 10.94 0 0119 12.55M5 12.55a10.94 10.94 0 015.17-2.39M10.71 5.05A16 16 0 0122.56 9M1.42 9a15.91 15.91 0 014.7-2.88M8.53 16.11a6 6 0 016.95 0"/><circle cx="12" cy="20" r="1" fill="currentColor" stroke="none"/></svg></span><div class="empty-title">Wi-Fi discovery is off</div><div class="empty-body">Enable in Settings → Connectivity, or switch to USB cable mode using the transport toggle above.</div></div>';
    showView('empty');
    // If USB-only is on, auto-switch the entry surface to USB.
    if (localStorage.getItem('vior_usb_only') === '1') {
      applyEntryMode('usb');
    }
    finishScan();
    return;
  }
  viorState.set({ state: 'scanning' });
  $('disc-status').textContent = 'Looking for Vior on your Wi-Fi…';
  $('disc-list').innerHTML =
    '<div class="radar-search">' +
      '<div class="radar">' +
        '<span class="radar-ring"></span><span class="radar-ring"></span><span class="radar-ring"></span>' +
        '<div class="radar-core">' +
          '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4 8V6a2 2 0 0 1 2-2h2M16 4h2a2 2 0 0 1 2 2v2M20 16v2a2 2 0 0 1-2 2h-2M8 20H6a2 2 0 0 1-2-2v-2"/><path d="M4 12h16"/></svg>' +
        '</div>' +
      '</div>' +
      '<div class="empty-title">Searching</div>' +
      '<div class="empty-body">Looking for Vior servers on your network.</div>' +
    '</div>';
  showView('disc');

  const last = localStorage.getItem('vior_last');
  if (last) { const p = last.split(':'); probeServer(p[0], parseInt(p[1] || '8080')); }

  getLocalIP(function (ip: string | null) {
    if (!ip) {
      // No Wi-Fi IP → we're off-network (mobile data, airplane, or no
      // Wi-Fi). Don't silently show the empty "no servers" view — that
      // reads as "your desktop isn't running" when the real cause is the
      // phone's network. Surface it explicitly with a Retry affordance.
      if (discoveryTimeout) { clearTimeout(discoveryTimeout); discoveryTimeout = null; }
      showNoNetwork();
      finishScan();
      return;
    }
    const base = ip.split('.').slice(0, 3).join('.');
    // Parallel /24 sweep — fire all probes at once across common ports.
    // The UDP beacon carries the real port, but this HTTP fallback helps
    // when UDP is filtered. Tries 8080 and 8081 to catch auto-selected ports.
    const commonPorts = [8080, 8081]
    const probes: Promise<void>[] = [];
    for (let pi = 0; pi < commonPorts.length; pi++) {
      for (let i = 1; i < 255; i++) probes.push(probeServer(base + '.' + i, commonPorts[pi]));
    }
    Promise.allSettled(probes).then(function () {
      finishScan();
      if (!selectedServer && Object.keys(foundServers).length === 0) showEmpty();
    });
  });

  discoveryTimeout = setTimeout(function () {
    finishScan();
    if (!selectedServer && Object.keys(foundServers).length === 0) showEmpty();
    else if (selectedServer) $('disc-status').textContent = Object.keys(foundServers).length + ' server' + (Object.keys(foundServers).length > 1 ? 's' : '') + ' found · tap to connect';
  }, 4000);
}

// No-Wi-Fi / off-network state. Renders an inline card in the discovery
// list with a clear cause + a Retry button, and fires a matching toast.
function showNoNetwork(): void {
  showView('disc');
  $('disc-status').textContent = 'Not on a Wi-Fi network';
  $('disc-list').innerHTML =
    '<div class="empty"><span class="empty-icon"><svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M1 1l22 22M16.72 11.06A10.94 10.94 0 0119 12.55M5 12.55a10.94 10.94 0 015.17-2.39M10.71 5.05A16 16 0 0122.56 9M1.42 9a15.91 15.91 0 014.7-2.88M8.53 16.11a6 6 0 016.95 0"/><circle cx="12" cy="20" r="1" fill="currentColor" stroke="none"/></svg></span>' +
    '<div class="empty-title">Not on a Wi-Fi network</div>' +
    '<div class="empty-body">Connect to the same Wi-Fi network as your desktop, then Retry.</div>' +
    '<button class="btn btn-primary" id="no-network-retry" style="margin-top: 14px;">Retry</button>' +
    '</div>';
  const retry = document.getElementById('no-network-retry');
  if (retry) retry.addEventListener('click', function () { startDiscovery(); });
  toast('warning', 'Not on a Wi-Fi network', 'Connect to the same network as your desktop, then Retry.');
}

function probeServer(host: string, port: number): Promise<void> {
  const key = host + ':' + port;
  if (foundServers[key]) return Promise.resolve();
  const ctrl = new AbortController();
  setTimeout(function () { ctrl.abort(); }, 1500);
  return fetch('http://' + host + ':' + port + '/info', { signal: ctrl.signal })
    .then(function (r) { return r.json() as Promise<ServerInfo>; })
    .then(function (info: ServerInfo) {
      if (foundServers[key]) return;
      foundServers[key] = info;
      // Cache the server's stable deviceId per IP so we can later
      // detect that the same desktop has moved to a different IP
      // (DHCP renew, Wi-Fi/Ethernet switch). See connect.ts
      // tryDhcpFallback() for the consumer.
      const sid = (info as unknown as { deviceId?: string }).deviceId;
      if (sid) {
        try { localStorage.setItem('vior_known_device_' + host + ':' + port, sid); } catch (_) { /* blocked */ }
      }
      if (discoveryTimeout) clearTimeout(discoveryTimeout);
      const n = Object.keys(foundServers).length;
      $('disc-status').textContent = n + ' server' + (n > 1 ? 's' : '') + ' found · tap to connect';
      viorState.set({ state: 'found-server' });
      renderServerList();
      if (!selectedServer) {
        // Auto-connect path: silent select + immediate doConnect for the
        // previously-used server. We *don't* visibly highlight rows for
        // a manual "select" any more — taps go straight to connect (see
        // server-row click handler below).
        const last = localStorage.getItem('vior_last');
        const auto = localStorage.getItem('vior_autoconnect') !== '0';
        if (auto && last === host + ':' + port && !connected) {
          selectServer(host, port, info.name || host, info.platform || '');
          // 400 ms window lets the list render before we auto-plunge.
          // Clear any previous pending auto-connect so a fast manual
          // tap on another row doesn't race the timer.
          if (autoConnectTimer) clearTimeout(autoConnectTimer);
          autoConnectTimer = setTimeout(function () {
            autoConnectTimer = null;
            if (!connected) initiateConnect();
          }, 400);
        }
      }
    })
    .catch(function () {});
}

function renderServerList(): void {
  const list = $('disc-list');
  list.innerHTML = '';
  Object.keys(foundServers).forEach(function (key) {
    const info = foundServers[key];
    const parts = key.split(':');
    const host = parts[0], port = parseInt(parts[1]);
    const row = document.createElement('button');
    row.className = 'server-row';
    row.dataset.key = key;
    if (selectedServer && selectedServer.host === host && selectedServer.port === port) row.classList.add('selected');
    row.innerHTML =
      '<span class="server-icon"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="2.5" y="4.5" width="19" height="12.5" rx="1.6"/><path d="M2.5 13.5h19"/><path d="M9 21h6"/></svg></span>' +
      '<span class="server-body">' +
        '<span class="server-name">' + esc(info.name || host) + '</span>' +
        '<span class="server-meta">' + esc(host) + '</span>' +
      '</span>' +
      '<span class="server-right">' +
        '<span>' + esc(info.platform || 'Server') + '</span>' +
        (selectedServer && selectedServer.host === host && selectedServer.port === port
          ? '<span style="color: var(--accent);"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4.5 12.5l4.5 4.5L19.5 6.5"/></svg></span>'
          : '<span style="display: inline-flex; align-items: center; gap: 4px; font-size: 11px;"><span class="dot dot-ok" style="width: 6px; height: 6px;"></span></span>') +
      '</span>';
    // Tap-to-connect: per the new state machine there is no separate
    // "Connect" button. Select + connect in one tap; the connect path
    // routes through the pair-prompt modal if the server isn't known.
    row.addEventListener('click', function () {
      if (autoConnectTimer) { clearTimeout(autoConnectTimer); autoConnectTimer = null; }
      selectServer(host, port, info.name || host, info.platform || '');
      initiateConnect();
    });
    list.appendChild(row);
  });
}

function selectServer(host: string, port: number, name: string, platform: string): void {
  selectedServer = { host: host, port: port, name: name || host };
  serverName = name || host; serverPlatform = platform || '';
  renderServerList();
}

// Single entry point for "user wants to connect to selectedServer".
// Decides whether to go through the pair-prompt modal first or
// straight to doConnect(). Used by row taps + auto-connect.
function initiateConnect(): void {
  if (!selectedServer) return;
  const key = selectedServer.host + ':' + selectedServer.port;
  const known = localStorage.getItem('vior_known_' + key) === '1';
  if (!known) {
    viorState.set({ state: 'pairing' });
    // promptPair is defined in connect.ts (loaded later in the same
    // document). Late-bind to allow files to load in any order.
    const pp = (globalThis as unknown as { promptPair?: () => void }).promptPair;
    if (typeof pp === 'function') pp();
    return;
  }
  reconnectAttempts = 0;
  doConnect();
}

function showView(name: string): void {
  $('disc-view').classList.toggle('hidden', name !== 'disc');
  $('empty-view').classList.toggle('hidden', name !== 'empty');
  $('connected-view').classList.toggle('hidden', name !== 'connected');
}
function showEmpty(): void {
  showView('empty'); $('disc-status').textContent = 'No servers found';
  // Drive the cascade: a fresh "no servers" event moves the user to
  // step B (Scan QR). If they previously escaped to C/D we honour the
  // persisted choice instead (handled inside setCascadeStep + the
  // resume-on-boot block in connect.ts).
  try {
    const persisted = localStorage.getItem('vior_last_entry_step');
    const next = (persisted === 'c' || persisted === 'd') ? persisted : 'b';
    const fn = (window as unknown as { setCascadeStep?: (s: 'a' | 'b' | 'c' | 'd') => void }).setCascadeStep;
    if (typeof fn === 'function') fn(next as 'b' | 'c' | 'd');
  } catch (_) { /* localStorage blocked — leave on step A */ }
}

function getLocalIP(cb: (ip: string | null) => void): void {
  try {
    const pc = new RTCPeerConnection({ iceServers: [] });
    let done = false;
    pc.createDataChannel('');
    pc.createOffer().then(function (o) { return pc.setLocalDescription(o); });
    pc.onicecandidate = function (e: RTCPeerConnectionIceEvent) {
      if (done || !e || !e.candidate) return;
      const m = e.candidate.candidate.match(/(\d+\.\d+\.\d+\.\d+)/);
      if (m && m[1] !== '0.0.0.0') { done = true; pc.close(); cb(m[1]); }
    };
    setTimeout(function () { if (!done) { done = true; pc.close(); cb(null); } }, 3000);
  } catch (e) { cb(null); }
}

$('disc-refresh').addEventListener('click', startDiscovery);
const rescanBtn = document.getElementById('rescan-btn');
if (rescanBtn) rescanBtn.addEventListener('click', startDiscovery);

// "Connect manually" disclosure on the discovery view → jump straight
// to cascade step C (pair-code entry) which is the most common manual
// path. Users who want IP can drop further to step D from there.
const discManualLink = $('disc-manual-link');
if (discManualLink) {
  discManualLink.addEventListener('click', function () {
    showView('empty');
    const fn = (window as unknown as { setCascadeStep?: (s: 'a' | 'b' | 'c' | 'd') => void }).setCascadeStep;
    if (typeof fn === 'function') fn('c');
  });
}

// Expose for cross-module use (connect.ts calls into us after a
// successful pair-prompt submit to re-trigger the connect).
(globalThis as unknown as { initiateConnect?: () => void }).initiateConnect = initiateConnect;
