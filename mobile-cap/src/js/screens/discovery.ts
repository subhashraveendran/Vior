'use strict';
// ── Discovery ──
let foundServers: Record<string, ServerInfo> = {};
let discoveryTimeout: ReturnType<typeof setTimeout> | null = null;
let scanning = false;
function startDiscovery(): void {
  foundServers = {}; selectedServer = null; scanning = true;
  if (localStorage.getItem('vior_wifi') === '0' || localStorage.getItem('vior_usb_only') === '1') {
    $('disc-status').textContent = 'Wi-Fi discovery off';
    $('disc-list').innerHTML = '<div class="empty"><span class="empty-icon"><svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M1 1l22 22M16.72 11.06A10.94 10.94 0 0119 12.55M5 12.55a10.94 10.94 0 015.17-2.39M10.71 5.05A16 16 0 0122.56 9M1.42 9a15.91 15.91 0 014.7-2.88M8.53 16.11a6 6 0 016.95 0"/><circle cx="12" cy="20" r="1" fill="currentColor" stroke="none"/></svg></span><div class="empty-title">Wi-Fi discovery is off</div><div class="empty-body">Enable in Settings → Connectivity, or use the manual fields below.</div></div>';
    showView('empty');
    return;
  }
  $('disc-status').textContent = 'Scanning network…';
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
  ($('connect-btn') as HTMLButtonElement).disabled = true;
  $('connect-label').textContent = 'Select a server';

  const last = localStorage.getItem('vior_last');
  if (last) { const p = last.split(':'); probeServer(p[0], parseInt(p[1])); }

  getLocalIP(function (ip: string | null) {
    if (!ip) { setTimeout(showEmpty, 2500); return; }
    const base = ip.split('.').slice(0, 3).join('.');
    // Parallel /24 sweep — fire all 254 probes at once; AbortController
    // inside probeServer enforces a 1.5s per-probe timeout. Total wall
    // time ≈ 1.5s instead of ~5s for sequential 13×300ms batching.
    const probes: Promise<void>[] = [];
    for (let i = 1; i < 255; i++) probes.push(probeServer(base + '.' + i, 8080));
    Promise.allSettled(probes).then(function () {
      scanning = false;
      if (!selectedServer && Object.keys(foundServers).length === 0) showEmpty();
    });
  });

  discoveryTimeout = setTimeout(function () {
    scanning = false;
    if (!selectedServer && Object.keys(foundServers).length === 0) showEmpty();
    else if (selectedServer) $('disc-status').textContent = Object.keys(foundServers).length + ' servers found';
  }, 4000);
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
      if (discoveryTimeout) clearTimeout(discoveryTimeout);
      $('disc-status').textContent = Object.keys(foundServers).length + ' server' + (Object.keys(foundServers).length > 1 ? 's' : '') + ' found';
      renderServerList();
      if (!selectedServer) {
        selectServer(host, port, info.name || host, info.platform || '');
        const last = localStorage.getItem('vior_last');
        const auto = localStorage.getItem('vior_autoconnect') !== '0';
        if (auto && last === host + ':' + port && !connected) {
          setTimeout(function () { if (!connected) doConnect(); }, 400);
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
    row.addEventListener('click', function () { selectServer(host, port, info.name || host, info.platform || ''); });
    list.appendChild(row);
  });
}

function selectServer(host: string, port: number, name: string, platform: string): void {
  selectedServer = { host: host, port: port };
  serverName = name || host; serverPlatform = platform || '';
  renderServerList();
  ($('connect-btn') as HTMLButtonElement).disabled = false;
  $('connect-label').textContent = 'Connect';
}

function showView(name: string): void {
  $('disc-view').classList.toggle('hidden', name !== 'disc');
  $('empty-view').classList.toggle('hidden', name !== 'empty');
  $('connected-view').classList.toggle('hidden', name !== 'connected');
}
function showEmpty(): void { showView('empty'); $('disc-status').textContent = 'No servers found'; }

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
$('rescan-btn').addEventListener('click', startDiscovery);
