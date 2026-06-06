(function () {
  'use strict';
  var $ = function (id) { return document.getElementById(id); };

  // ── State ──
  var ws = null;
  var displayW = 0, displayH = 0;
  var selectedServer = null, selectedMode = 'extend';
  var serverName = '', serverPlatform = '', serverRes = '—';
  var streamImg = $('stream-img');
  var framePolling = false, frameBaseUrl = '', blobUrl = null;
  var overlayTimer = null, reconnectAttempts = 0, maxReconnect = 5;
  var connected = false, currentTab = 'display';
  var streamVisible = false, fps = 0, frameCount = 0, fpsTimer = null;

  // ── Toasts ──
  var toastId = 0;
  function toast(tone, title, msg) {
    var id = ++toastId;
    var host = $('toast-host');
    var el = document.createElement('div');
    el.className = 'toast';
    el.dataset.id = id;
    var dotCls = tone === 'success' ? 'dot-ok' : tone === 'warning' ? 'dot-warn' : tone === 'error' ? 'dot-err' : 'dot-idle';
    el.innerHTML =
      '<span class="dot ' + dotCls + '" style="margin-top: 5px;"></span>' +
      '<div style="flex:1;min-width:0;">' +
        '<div class="toast-title">' + esc(title) + '</div>' +
        (msg ? '<div class="toast-msg">' + esc(msg) + '</div>' : '') +
      '</div>';
    host.appendChild(el);
    setTimeout(function () { if (el.parentNode) el.parentNode.removeChild(el); }, 3500);
  }
  function esc(s) { var d = document.createElement('div'); d.textContent = String(s == null ? '' : s); return d.innerHTML; }

  // ── Connection chip ──
  function setConnState(state) {
    var dot = $('conn-dot'), label = $('conn-label');
    dot.className = 'dot';
    if (state === 'online') { dot.classList.add('dot-ok', 'dot-pulse'); label.textContent = 'Connected'; }
    else if (state === 'connecting') { dot.classList.add('dot-warn', 'dot-pulse'); label.textContent = 'Connecting'; }
    else if (state === 'reconnecting') { dot.classList.add('dot-warn', 'dot-pulse'); label.textContent = 'Reconnecting'; }
    else if (state === 'error') { dot.classList.add('dot-err'); label.textContent = 'Disconnected'; }
    else { dot.classList.add('dot-idle'); label.textContent = 'Not connected'; }
  }

  // ── Tab switch ──
  function switchTab(name) {
    currentTab = name;
    // Always release camera when navigating away — leaving the scanner
    // hot in the background is what causes the next acquire to fail with
    // NotReadableError. `stopQRScan` is defined later and is idempotent.
    if (typeof stopQRScan === 'function') stopQRScan();
    var items = document.querySelectorAll('.tab-item');
    for (var i = 0; i < items.length; i++) items[i].classList.toggle('active', items[i].dataset.tab === name);
    var panes = document.querySelectorAll('.pane');
    for (var j = 0; j < panes.length; j++) panes[j].classList.toggle('active', panes[j].id === 'pane-' + name);
  }
  document.querySelectorAll('.tab-item').forEach(function (btn) {
    var fired = false;
    btn.addEventListener('touchend', function (e) { e.preventDefault(); fired = true; switchTab(btn.dataset.tab); setTimeout(function () { fired = false; }, 400); }, { passive: false });
    btn.addEventListener('click', function () { if (!fired) switchTab(btn.dataset.tab); });
  });

  // ── Mode select ──
  document.querySelectorAll('#disc-dock .seg-btn').forEach(function (btn) {
    btn.addEventListener('click', function () {
      selectedMode = btn.dataset.mode;
      document.querySelectorAll('#disc-dock .seg-btn').forEach(function (b) { b.classList.toggle('active', b === btn); });
    });
  });

  // ── Discovery ──
  var foundServers = {}, discoveryTimeout = null, scanning = false;
  function startDiscovery() {
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
    $('connect-btn').disabled = true;
    $('connect-label').textContent = 'Select a server';

    var last = localStorage.getItem('vior_last');
    if (last) { var p = last.split(':'); probeServer(p[0], parseInt(p[1])); }

    getLocalIP(function (ip) {
      if (!ip) { setTimeout(showEmpty, 2500); return; }
      var base = ip.split('.').slice(0, 3).join('.');
      // Parallel /24 sweep — fire all 254 probes at once; AbortController
      // inside probeServer enforces a 1.5s per-probe timeout. Total wall
      // time ≈ 1.5s instead of ~5s for sequential 13×300ms batching.
      var probes = [];
      for (var i = 1; i < 255; i++) probes.push(probeServer(base + '.' + i, 8080));
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

  function probeServer(host, port) {
    var key = host + ':' + port;
    if (foundServers[key]) return Promise.resolve();
    var ctrl = new AbortController();
    setTimeout(function () { ctrl.abort(); }, 1500);
    return fetch('http://' + host + ':' + port + '/info', { signal: ctrl.signal })
      .then(function (r) { return r.json(); })
      .then(function (info) {
        if (foundServers[key]) return;
        foundServers[key] = info;
        clearTimeout(discoveryTimeout);
        $('disc-status').textContent = Object.keys(foundServers).length + ' server' + (Object.keys(foundServers).length > 1 ? 's' : '') + ' found';
        renderServerList();
        if (!selectedServer) {
          selectServer(host, port, info.name || host, info.platform || '');
          var last = localStorage.getItem('vior_last');
          var auto = localStorage.getItem('vior_autoconnect') !== '0';
          if (auto && last === host + ':' + port && !connected) {
            setTimeout(function () { if (!connected) doConnect(); }, 400);
          }
        }
      })
      .catch(function () {});
  }

  function renderServerList() {
    var list = $('disc-list');
    list.innerHTML = '';
    Object.keys(foundServers).forEach(function (key) {
      var info = foundServers[key];
      var parts = key.split(':');
      var host = parts[0], port = parseInt(parts[1]);
      var row = document.createElement('button');
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

  function selectServer(host, port, name, platform) {
    selectedServer = { host: host, port: port };
    serverName = name || host; serverPlatform = platform || '';
    renderServerList();
    $('connect-btn').disabled = false;
    $('connect-label').textContent = 'Connect';
  }

  function showView(name) {
    $('disc-view').classList.toggle('hidden', name !== 'disc');
    $('empty-view').classList.toggle('hidden', name !== 'empty');
    $('connected-view').classList.toggle('hidden', name !== 'connected');
  }
  function showEmpty() { showView('empty'); $('disc-status').textContent = 'No servers found'; }

  function getLocalIP(cb) {
    try {
      var pc = new RTCPeerConnection({ iceServers: [] }), done = false;
      pc.createDataChannel('');
      pc.createOffer().then(function (o) { return pc.setLocalDescription(o); });
      pc.onicecandidate = function (e) {
        if (done || !e || !e.candidate) return;
        var m = e.candidate.candidate.match(/(\d+\.\d+\.\d+\.\d+)/);
        if (m && m[1] !== '0.0.0.0') { done = true; pc.close(); cb(m[1]); }
      };
      setTimeout(function () { if (!done) { done = true; pc.close(); cb(null); } }, 3000);
    } catch (e) { cb(null); }
  }

  $('disc-refresh').addEventListener('click', startDiscovery);
  $('rescan-btn').addEventListener('click', startDiscovery);

  // ── QR scanner (native BarcodeDetector with jsQR fallback) ──
  var qrStream = null, qrRunning = false, qrDetector = null, qrCanvas = null, qrCtx = null;
  function parseScanResult(raw) {
    var url = raw, code = '';
    var m = raw.match(/^vior:\/\/([\d.]+)(?::(\d+))?(?:\?pair=([A-F0-9]+))?/i);
    if (m) { url = 'http://' + m[1] + ':' + (m[2] || '8080'); code = m[3] || ''; }
    else {
      var u = raw.match(/^https?:\/\/([\d.]+)(?::(\d+))?/i);
      if (u) { url = 'http://' + u[1] + ':' + (u[2] || '8080'); }
      var pm = raw.match(/[?&]pair=([A-F0-9]+)/i); if (pm) code = pm[1];
    }
    var hp = url.replace(/^https?:\/\//, '').split(':');
    return { host: hp[0], port: parseInt(hp[1] || '8080'), code: code };
  }
  function onScanHit(raw) {
    var p = parseScanResult(raw);
    // Visible "decoded" feedback before the modal disappears so the user
    // knows the scan worked — otherwise the modal just vanishes and they
    // wonder if they missed it.
    var dot = $('qr-scanning-dot'), txt = $('qr-hint-text');
    if (dot) dot.style.background = '#3ecf6f';
    if (txt) txt.textContent = 'Decoded ' + (p.host || 'server') + ' — connecting…';
    if (p.code) $('manual-pair').value = p.code.toUpperCase();
    toast('success', 'QR scanned', 'Connecting to ' + p.host + (p.port !== 8080 ? ':' + p.port : '') + '…');
    setTimeout(function () {
      stopQRScan();
      selectServer(p.host, p.port, p.host, '');
      doConnect();
    }, 250);
  }
  function stopQRScan() {
    // Idempotent — safe to call from anywhere (cancel button, error path,
    // tab switch, visibilitychange, pagehide, successful detect). Without
    // this guard, double-invocation could fight with an in-flight start.
    if (!qrRunning && !qrStream) { $('qr-modal').classList.add('hidden'); return; }
    qrRunning = false;
    if (qrStream) {
      try { qrStream.getTracks().forEach(function (t) { try { t.stop(); } catch (_) {} }); } catch (_) {}
      qrStream = null;
    }
    var v = $('qr-video');
    if (v) { try { v.pause(); } catch (_) {} v.srcObject = null; }
    $('qr-modal').classList.add('hidden');
  }
  // Layered camera acquisition: try the strictest constraint first so we
  // get the rear camera on multi-camera devices, then fall back. Honor
  // tablets often refuse `facingMode: { exact: 'environment' }` with
  // OverconstrainedError; the fallback paths handle that.
  async function acquireCamera() {
    // Cap sensor output at 720×720 — QR codes only need a few hundred px to
    // decode, but modern phone cameras default to 1080p/4K which burns the
    // main thread on every getImageData. Constraint is ideal, not exact.
    var sz = { width: { ideal: 720, max: 1280 }, height: { ideal: 720, max: 1280 } };
    var tries = [
      { video: Object.assign({ facingMode: { exact: 'environment' } }, sz) },
      { video: Object.assign({ facingMode: 'environment' }, sz) },
      { video: Object.assign({}, sz) },
      { video: true }
    ];
    var lastErr;
    for (var i = 0; i < tries.length; i++) {
      try { return await navigator.mediaDevices.getUserMedia(tries[i]); }
      catch (e) {
        lastErr = e;
        // Only iterate on constraint-shape errors — other errors (permission,
        // hardware busy, missing device) will not be solved by relaxing.
        if (e && e.name !== 'OverconstrainedError' && e.name !== 'TypeError') break;
      }
    }
    throw lastErr;
  }
  async function startQRScan() {
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
      toast('error', 'Camera unsupported', 'getUserMedia unavailable on this WebView.'); return;
    }
    // jsQR is loaded with `defer` — if the user taps Scan QR before the
    // script finishes parsing, jsQR will be undefined. Wait for load.
    if (!window.jsQR && !('BarcodeDetector' in window)) {
      await new Promise(function (r) {
        if (document.readyState === 'complete') r();
        else window.addEventListener('load', r, { once: true });
      });
    }
    try {
      try {
        qrStream = await acquireCamera();
      } catch (e1) {
        // NotReadableError typically means the camera device is held by a
        // previous (un-stopped) MediaStreamTrack in this WebView. Tear down
        // any stale stream and retry once after a short delay.
        if (e1 && e1.name === 'NotReadableError') {
          stopQRScan();
          await new Promise(function (r) { setTimeout(r, 250); });
          qrStream = await acquireCamera();
        } else { throw e1; }
      }
      var video = $('qr-video');
      video.srcObject = qrStream;
      try { await video.play(); }
      catch (pe) {
        // Some WebViews resolve play() into a rejected promise (autoplay
        // policy, lost focus). Surface it instead of leaving a black modal.
        toast('error', 'Video failed to start', String(pe && pe.message || pe));
        stopQRScan();
        return;
      }
      $('qr-modal').classList.remove('hidden');
      qrRunning = true;
      if ('BarcodeDetector' in window) {
        try { qrDetector = new BarcodeDetector({ formats: ['qr_code'] }); tickQRNative(); return; }
        catch (be) { console.warn('BarcodeDetector advertised but instantiation failed; falling back to jsQR:', be); }
      }
      if (typeof jsQR === 'function') { tickQRJsQR(); return; }
      toast('error', 'Scanner load failed', 'jsQR not loaded; reinstall app.');
      stopQRScan();
    } catch (e) {
      var name = (e && e.name) || '';
      if (name === 'NotAllowedError' || name === 'PermissionDeniedError') {
        toast('error', 'Camera blocked', 'Allow camera in System Settings → Apps → Vior.');
      } else if (name === 'NotReadableError') {
        toast('error', 'Camera busy', 'Camera in use by another app — close it and retry.');
      } else if (name === 'NotFoundError') {
        toast('error', 'No camera', 'No camera device found.');
      } else {
        toast('error', 'Camera failed', String(e && e.message || e));
      }
      stopQRScan();
    }
  }
  async function tickQRNative() {
    if (!qrRunning) return;
    try {
      var codes = await qrDetector.detect($('qr-video'));
      if (codes && codes.length) { onScanHit(codes[0].rawValue || ''); return; }
    } catch (_) {}
    requestAnimationFrame(tickQRNative);
  }
  function tickQRJsQR() {
    if (!qrRunning) return;
    var v = $('qr-video');
    if (!qrCanvas) { qrCanvas = document.createElement('canvas'); qrCtx = qrCanvas.getContext('2d', { willReadFrequently: true }); }
    if (v.readyState === v.HAVE_ENOUGH_DATA && v.videoWidth) {
      var w = v.videoWidth, h = v.videoHeight;
      // Downscale for perf: detect on 480-wide frame max.
      var scale = w > 480 ? 480 / w : 1;
      var dw = Math.round(w * scale), dh = Math.round(h * scale);
      // Only resize when dims actually change — every canvas.width assignment
      // reallocates the backing store, which is wasteful at 60fps.
      if (qrCanvas.width !== dw || qrCanvas.height !== dh) { qrCanvas.width = dw; qrCanvas.height = dh; }
      qrCtx.drawImage(v, 0, 0, dw, dh);
      var img = qrCtx.getImageData(0, 0, dw, dh);
      var code = jsQR(img.data, dw, dh, { inversionAttempts: 'attemptBoth' });
      if (code && code.data) { onScanHit(code.data); return; }
    }
    requestAnimationFrame(tickQRJsQR);
  }
  $('scan-qr-btn').addEventListener('click', startQRScan);
  $('qr-cancel').addEventListener('click', stopQRScan);
  // Release the camera whenever the page is backgrounded or torn down —
  // Android WebView does not automatically stop MediaStreamTracks when
  // the app goes to the background, and a lingering track will refuse
  // the next acquire with NotReadableError.
  document.addEventListener('visibilitychange', function () { if (document.hidden) stopQRScan(); });
  window.addEventListener('pagehide', stopQRScan);
  $('manual-ip').addEventListener('input', function (e) { $('manual-go').disabled = !e.target.value.trim(); });
  $('manual-go').addEventListener('click', function () {
    var v = $('manual-ip').value.trim();
    if (!v) return;
    var parts = v.split(':'), host = parts[0], port = parts.length > 1 ? parseInt(parts[1]) : 8080;
    selectServer(host, port, host, '');
    doConnect();
  });

  // ── Connect / disconnect ──
  $('connect-btn').addEventListener('click', function () {
    if (!selectedServer) return;
    // If we have no pair code on hand AND we've never successfully
    // paired with this server before, prompt for the code instead of
    // firing a guaranteed-to-fail Connect.
    var key = selectedServer.host + ':' + selectedServer.port;
    var known = localStorage.getItem('vior_known_' + key) === '1';
    var pair = ($('manual-pair') && $('manual-pair').value || '').trim();
    if (!known && !pair) { promptPair(); return; }
    reconnectAttempts = 0; doConnect();
  });

  function promptPair() {
    var m = $('pair-prompt');
    if (m) { m.classList.remove('hidden'); var inp = $('pair-prompt-input'); if (inp) { inp.value = ''; setTimeout(function () { try { inp.focus(); } catch (_) {} }, 60); } }
  }
  function closePair() { var m = $('pair-prompt'); if (m) m.classList.add('hidden'); }
  document.addEventListener('click', function (e) {
    if (e.target && e.target.id === 'pair-prompt') closePair();
  });
  if ($('pair-prompt-cancel')) $('pair-prompt-cancel').addEventListener('click', closePair);
  if ($('pair-prompt-go')) $('pair-prompt-go').addEventListener('click', function () {
    var v = ($('pair-prompt-input').value || '').toUpperCase().trim();
    if (!v) return;
    $('manual-pair').value = v;
    closePair();
    reconnectAttempts = 0; doConnect();
  });
  if ($('pair-prompt-input')) $('pair-prompt-input').addEventListener('keydown', function (e) {
    if (e.key === 'Enter') { e.preventDefault(); $('pair-prompt-go').click(); }
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

  var connectTimeoutId = null;
  function doConnect() {
    setConnState('connecting');
    $('connecting-overlay').classList.remove('hidden');
    $('conn-title').textContent = 'Connecting';
    $('conn-sub').innerHTML = 'Establishing ' + selectedMode + ' session with<br><b>' + esc(serverName) + '</b>';
    $('conn-bar').classList.remove('hidden');
    $('conn-spin-ring').style.display = '';
    $('conn-spin-core').classList.remove('failed');

    var host = selectedServer.host, port = selectedServer.port;
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
      var dpr = window.devicePixelRatio || 1;
      var pair = ($('manual-pair') && $('manual-pair').value || '').toUpperCase().trim();
      // Stable per-install device ID — once the server trusts us, we never
      // need to re-enter the pair code from this app install again.
      var deviceID = localStorage.getItem('vior_device_id');
      if (!deviceID) {
        deviceID = 'mob-' + ((window.crypto && crypto.randomUUID) ? crypto.randomUUID() : (Math.random().toString(36).slice(2) + Date.now().toString(36)));
        try { localStorage.setItem('vior_device_id', deviceID); } catch (_) {}
      }
      ws.send(JSON.stringify({ type: 'hello', data: {
        width: Math.round(screen.width * dpr), height: Math.round(screen.height * dpr),
        dpr: dpr, name: 'Vior Mobile', mode: selectedMode, pairCode: pair, deviceId: deviceID
      }}));
    };

    ws.onmessage = function (e) {
      var msg = JSON.parse(e.data);
      if (msg.type === 'ready') {
        if (connectTimeoutId) { clearTimeout(connectTimeoutId); connectTimeoutId = null; }
        var res = msg.data.resolution.split('x');
        displayW = parseInt(res[0]); displayH = parseInt(res[1]);
        serverRes = msg.data.resolution.replace('x', ' × ');
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
        $('stat-mode').textContent = selectedMode === 'mirror' ? 'Mirror' : 'Extend';
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
        toast('error', 'Connection failed', (msg.data && msg.data.message) || 'Check both devices on same Wi-Fi. Try manual IP.');
        setConnState('offline');
      } else if (msg.type && msg.type.indexOf('file-') === 0) {
        try { handleFileMessage(msg); } catch (e) { console.error('file msg', e); }
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

  function doDisconnect() {
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

  // ── Stream fullscreen ──
  $('view-stream-btn').addEventListener('click', openStream);
  $('stream-back').addEventListener('click', hideStream);
  $('stream-disconnect').addEventListener('click', function () { hideStream(); doDisconnect(); });
  // Floating settings FAB inside the stream — opens the Settings sheet
  // on top of the stream without exiting fullscreen.
  var streamFab = $('stream-settings-fab');
  if (streamFab) {
    streamFab.addEventListener('click', function () {
      $('settings-sheet').classList.remove('hidden');
    });
  }

  function openStream() {
    streamVisible = true;
    $('stream-fs').classList.add('active');
    $('stream-name').textContent = serverName;
    $('stream-mode-text').textContent = selectedMode === 'mirror' ? 'Mirroring' : 'Extended display';
    $('stream-stat-res').textContent = serverRes;
    $('stream-loading').classList.remove('hidden');
    $('stream-loading-sub').textContent = 'negotiating MJPEG · ' + serverRes;
    startFramePolling();
    startOverlayAutoHide();
  }
  function hideStream() {
    streamVisible = false;
    stopFramePolling();
    cleanupBlob();
    streamImg.src = '';
    $('stream-fs').classList.remove('active');
    if (fpsTimer) { clearInterval(fpsTimer); fpsTimer = null; }
  }

  function startFramePolling() {
    framePolling = true;
    frameCount = 0;
    if (fpsTimer) clearInterval(fpsTimer);
    fpsTimer = setInterval(function () {
      fps = frameCount; frameCount = 0;
      $('stream-stat-fps').textContent = fps + ' fps';
      $('stream-stat-status').textContent = fps > 0 ? 'live' : '—';
    }, 1000);
    pollFrame();
  }
  function stopFramePolling() { framePolling = false; }
  function cleanupBlob() { if (blobUrl) { URL.revokeObjectURL(blobUrl); blobUrl = null; } }
  function pollFrame() {
    if (!framePolling) return;
    fetch(frameBaseUrl + '/snapshot?t=' + Date.now())
      .then(function (r) { if (!r.ok) throw 0; return r.blob(); })
      .then(function (b) {
        if (!framePolling) return;
        cleanupBlob();
        blobUrl = URL.createObjectURL(b);
        streamImg.src = blobUrl;
        frameCount++;
        var ld = $('stream-loading');
        if (!ld.classList.contains('hidden')) ld.classList.add('hidden');
        reconnectAttempts = 0;
        requestAnimationFrame(pollFrame);
      })
      .catch(function () { if (framePolling) setTimeout(pollFrame, 150); });
  }

  // ── Stream overlay auto-hide ──
  function startOverlayAutoHide() {
    showOverlay();
    streamImg.removeEventListener('click', toggleOverlay);
    streamImg.addEventListener('click', toggleOverlay);
  }
  function showOverlay() {
    $('stream-fs').classList.remove('dimmed');
    $('stream-top').style.transform = ''; $('stream-top').style.opacity = '';
    $('stream-bot').style.transform = ''; $('stream-bot').style.opacity = '';
    clearTimeout(overlayTimer);
    overlayTimer = setTimeout(function () {
      $('stream-top').style.transform = 'translateY(-110%)'; $('stream-top').style.opacity = '0';
      $('stream-bot').style.transform = 'translateY(110%)'; $('stream-bot').style.opacity = '0';
    }, 2800);
  }
  function toggleOverlay() {
    if ($('stream-top').style.opacity === '0') showOverlay();
    else {
      clearTimeout(overlayTimer);
      $('stream-top').style.transform = 'translateY(-110%)'; $('stream-top').style.opacity = '0';
      $('stream-bot').style.transform = 'translateY(110%)'; $('stream-bot').style.opacity = '0';
    }
  }

  // ── Touch input on stream ──
  function mapT(t) { var r = streamImg.getBoundingClientRect(); return { x: Math.round((t.clientX - r.left) / r.width * displayW), y: Math.round((t.clientY - r.top) / r.height * displayH) }; }
  function sendInput(action, x, y) { if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: 'input', data: { event: 'touch', action: action, x: x, y: y } })); }
  streamImg.addEventListener('touchstart', function (e) { e.preventDefault(); var p = mapT(e.changedTouches[0]); sendInput('down', p.x, p.y); }, { passive: false });
  streamImg.addEventListener('touchmove', function (e) { e.preventDefault(); var p = mapT(e.changedTouches[0]); sendInput('move', p.x, p.y); }, { passive: false });
  streamImg.addEventListener('touchend', function (e) { e.preventDefault(); var p = mapT(e.changedTouches[0]); sendInput('up', p.x, p.y); }, { passive: false });

  // ── Remote trackpad ──
  var trackpad = $('trackpad'), trackpadHint = $('trackpad-hint');
  var tpLastX = 0, tpLastY = 0, tpFingers = 0, tpMoved = false, tpStartT = 0;
  function wsSend(obj) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(obj)); }
  function flash(msg) {
    var p = $('flash-pill');
    p.textContent = msg + ' sent';
    p.classList.remove('hidden');
    clearTimeout(flash._t);
    flash._t = setTimeout(function () { p.classList.add('hidden'); }, 900);
  }
  trackpad.addEventListener('touchstart', function (e) {
    e.preventDefault();
    tpFingers = e.touches.length; tpMoved = false; tpStartT = Date.now();
    var t = e.touches[0]; tpLastX = t.clientX; tpLastY = t.clientY;
    trackpadHint.style.display = 'none';
  }, { passive: false });
  trackpad.addEventListener('touchmove', function (e) {
    e.preventDefault();
    var t = e.touches[0];
    var dx = t.clientX - tpLastX, dy = t.clientY - tpLastY;
    tpLastX = t.clientX; tpLastY = t.clientY;
    if (Math.abs(dx) + Math.abs(dy) > 2) tpMoved = true;
    if (e.touches.length >= 2) wsSend({ type: 'input', data: { event: 'scroll', dx: Math.round(dx / 4), dy: Math.round(dy / 4) } });
    else wsSend({ type: 'input', data: { event: 'mouse', action: 'move', dx: dx * 2, dy: dy * 2 } });
  }, { passive: false });
  trackpad.addEventListener('touchend', function (e) {
    e.preventDefault();
    var dur = Date.now() - tpStartT;
    if (!tpMoved && dur < 300) {
      var action = tpFingers >= 2 ? 'rightclick' : 'click';
      wsSend({ type: 'input', data: { event: 'mouse', action: action } });
      flash(action === 'rightclick' ? 'Right click' : 'Click');
    }
    tpFingers = 0;
    trackpadHint.style.display = '';
  }, { passive: false });

  // Scroll strip
  var ss = $('scroll-strip'), ssDown = false, ssY = 0;
  ss.addEventListener('touchstart', function (e) { e.preventDefault(); ssDown = true; ssY = e.touches[0].clientY; ss.classList.add('active'); }, { passive: false });
  ss.addEventListener('touchmove', function (e) {
    e.preventDefault();
    if (!ssDown) return;
    var y = e.touches[0].clientY, dy = y - ssY;
    if (Math.abs(dy) > 24) { ssY = y; wsSend({ type: 'input', data: { event: 'scroll', dx: 0, dy: dy > 0 ? 3 : -3 } }); flash('Scroll'); }
  }, { passive: false });
  ss.addEventListener('touchend', function (e) { e.preventDefault(); ssDown = false; ss.classList.remove('active'); }, { passive: false });

  $('click-btn').addEventListener('click', function () { wsSend({ type: 'input', data: { event: 'mouse', action: 'click' } }); flash('Click'); });
  $('rclick-btn').addEventListener('click', function () { wsSend({ type: 'input', data: { event: 'mouse', action: 'rightclick' } }); flash('Right click'); });

  // Remote view toggle
  $('remote-view-trackpad').addEventListener('click', function () {
    $('remote-view-trackpad').classList.add('active'); $('remote-view-keys').classList.remove('active');
    $('remote-trackpad-body').classList.remove('hidden'); $('remote-keys-body').classList.add('hidden');
  });
  $('remote-view-keys').addEventListener('click', function () {
    $('remote-view-trackpad').classList.remove('active'); $('remote-view-keys').classList.add('active');
    $('remote-trackpad-body').classList.add('hidden'); $('remote-keys-body').classList.remove('hidden');
  });

  // Shortcuts grid
  var SHORTCUTS = [
    ['Cmd+c', 'Copy', '⌘C', 'copy'], ['Cmd+v', 'Paste', '⌘V', 'paste'],
    ['Cmd+x', 'Cut', '⌘X', 'cut'], ['Cmd+z', 'Undo', '⌘Z', 'undo'],
    ['Cmd+Shift+z', 'Redo', '⇧⌘Z', 'redo'], ['Cmd+a', 'Select All', '⌘A', 'layers'],
    ['Cmd+s', 'Save', '⌘S', 'save'], ['Cmd+f', 'Find', '⌘F', 'search'],
    ['Cmd+Tab', 'App ⇆', '⌘⇥', 'swap'], ['Cmd+`', 'Window ⇆', '⌘`', 'window'],
    ['Cmd+Space', 'Spotlight', '⌘Sp', 'spotlight'], ['Cmd+q', 'Quit', '⌘Q', 'quit'],
  ];
  var ICONS = {
    copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
    paste: '<rect x="5" y="5" width="14" height="16" rx="2"/><rect x="9" y="2.5" width="6" height="3.5" rx="1.2"/>',
    cut: '<circle cx="6.5" cy="17" r="2.5"/><circle cx="6.5" cy="7" r="2.5"/><path d="M8.7 8.5L20 17M8.7 15.5L20 7"/>',
    undo: '<path d="M9 7L4.5 11.5 9 16"/><path d="M4.5 11.5H14a5 5 0 0 1 0 10h-1.5"/>',
    redo: '<path d="M15 7l4.5 4.5L15 16"/><path d="M19.5 11.5H10a5 5 0 0 0 0 10h1.5"/>',
    layers: '<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/>',
    save: '<path d="M5 4h11l3 3v13H5z"/><path d="M8 4v5h7V4M9 13h6v7H9z"/>',
    search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="M15.5 15.5L21 21"/>',
    swap: '<path d="M4 8h13M14 5l3 3-3 3"/><path d="M20 16H7M10 13l-3 3 3 3"/>',
    window: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18"/>',
    spotlight: '<circle cx="11" cy="11" r="6.5"/><path d="M15.6 15.6L21 21"/><circle cx="11" cy="11" r="2.3"/>',
    quit: '<path d="M14 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 12H3M6 8.5L2.5 12 6 15.5"/>',
  };
  var grid = $('shortcut-grid');
  SHORTCUTS.forEach(function (s) {
    var b = document.createElement('button');
    b.className = 'keycap';
    b.dataset.key = s[0];
    b.innerHTML =
      '<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">' + (ICONS[s[3]] || '') + '</svg>' +
      '<span class="keycap-label keycap-label-icon">' + s[1] + '</span>' +
      '<span class="keycap-sub">' + s[2] + '</span>';
    grid.appendChild(b);
  });

  // F-keys
  var fkeyGrid = $('fkey-grid');
  ['F1', 'F2', 'F3', 'F4', 'F5', 'F6', 'F11', 'F12'].forEach(function (f) {
    var b = document.createElement('button');
    b.className = 'keycap'; b.dataset.key = f;
    b.innerHTML = '<span class="keycap-label">' + f + '</span>';
    fkeyGrid.appendChild(b);
  });

  // Wire all keycap clicks
  document.querySelectorAll('.keycap').forEach(function (k) {
    var fired = false;
    function send() {
      var key = k.dataset.key; if (!key) return;
      wsSend({ type: 'input', data: { event: 'key', key: key } });
      var label = k.querySelector('.keycap-label'); flash(label ? label.textContent : key);
    }
    k.addEventListener('touchend', function (e) { e.preventDefault(); fired = true; send(); setTimeout(function () { fired = false; }, 400); }, { passive: false });
    k.addEventListener('click', function () { if (!fired) send(); });
  });

  // Soft keyboard
  var kbInput = $('kb-input');
  $('kb-btn').addEventListener('click', function () {
    kbInput.value = ''; kbInput.focus();
    toast('info', 'Keyboard ready', 'Type to forward keys.');
  });
  kbInput.addEventListener('input', function (e) {
    var data = e.data;
    if (data) { for (var i = 0; i < data.length; i++) wsSend({ type: 'input', data: { event: 'key', key: data[i] } }); }
    kbInput.value = '';
  });
  kbInput.addEventListener('keydown', function (e) {
    var map = { 'Backspace': 'BackSpace', 'Enter': 'Return', 'Tab': 'Tab', 'ArrowUp': 'Up', 'ArrowDown': 'Down', 'ArrowLeft': 'Left', 'ArrowRight': 'Right', 'Escape': 'Escape' };
    var k = map[e.key];
    if (k) { e.preventDefault(); wsSend({ type: 'input', data: { event: 'key', key: k } }); }
  });

  // ── File transfer ──
  var CHUNK_SIZE = 48 * 1024;
  var fileTransfers = {};

  $('send-file-btn').addEventListener('click', function () { $('file-input').click(); });
  $('send-photo-btn').addEventListener('click', function () { $('photo-input').click(); });
  $('file-input').addEventListener('change', function (e) { if (e.target.files[0]) sendFile(e.target.files[0]); e.target.value = ''; });
  $('photo-input').addEventListener('change', function (e) { if (e.target.files[0]) sendFile(e.target.files[0]); e.target.value = ''; });

  function genID() { var a = new Uint8Array(8); crypto.getRandomValues(a); return Array.from(a, function (b) { return ('0' + b.toString(16)).slice(-2); }).join(''); }
  function fmtSize(b) { if (b < 1024) return b + ' B'; if (b < 1048576) return (b / 1024).toFixed(1) + ' KB'; return (b / 1048576).toFixed(1) + ' MB'; }

  function sendFile(file) {
    var id = genID();
    var reader = new FileReader();
    reader.onload = function () {
      var data = new Uint8Array(reader.result);
      var t = { id: id, name: file.name, size: file.size, mimeType: file.type || 'application/octet-stream', preview: '', transferred: 0, complete: false, data: data, direction: 'out', status: 'sending' };
      fileTransfers[id] = t;
      if (file.type && file.type.indexOf('image/') === 0) {
        var pr = new FileReader();
        pr.onload = function () { t.preview = pr.result; sendOffer(t); };
        pr.readAsDataURL(file);
      } else { sendOffer(t); }
    };
    reader.readAsArrayBuffer(file);
  }
  function sendOffer(t) {
    if (!ws || ws.readyState !== 1) return;
    ws.send(JSON.stringify({ type: 'file-offer', data: { id: t.id, name: t.name, size: t.size, mimeType: t.mimeType, preview: t.preview } }));
    renderTransfers();
    toast('info', 'Offering', t.name);
  }
  function sendChunks(t) {
    var offset = 0;
    function next() {
      if (offset >= t.data.length) {
        t.complete = true; t.status = 'done';
        ws.send(JSON.stringify({ type: 'file-complete', data: { id: t.id, hash: '' } }));
        renderTransfers();
        toast('success', 'Sent', t.name);
        return;
      }
      var end = Math.min(offset + CHUNK_SIZE, t.data.length);
      var chunk = t.data.slice(offset, end);
      var s = ''; for (var i = 0; i < chunk.length; i++) s += String.fromCharCode(chunk[i]);
      ws.send(JSON.stringify({ type: 'file-chunk', data: { id: t.id, offset: offset, data: btoa(s) } }));
      offset = end; t.transferred = offset;
      t.progress = Math.round(offset / t.data.length * 100);
      renderTransfers();
      setTimeout(next, 5);
    }
    next();
  }
  function handleFileMessage(msg) {
    var d = msg.data;
    if (msg.type === 'file-offer') {
      fileTransfers[d.id] = { id: d.id, name: d.name, size: d.size, mimeType: d.mimeType, preview: d.preview || '', transferred: 0, complete: false, chunks: [], direction: 'in', pending: true, status: 'incoming' };
      renderIncoming();
      toast('info', 'Incoming', d.name);
      switchTab('files');
    } else if (msg.type === 'file-accept') {
      var t = fileTransfers[d.id]; if (t && t.direction === 'out') sendChunks(t);
    } else if (msg.type === 'file-reject') {
      var t2 = fileTransfers[d.id]; if (t2) { delete fileTransfers[d.id]; renderTransfers(); toast('warning', 'Declined', t2.name); }
    } else if (msg.type === 'file-chunk') {
      var t3 = fileTransfers[d.id]; if (t3 && t3.direction === 'in') {
        t3.chunks.push(d.data); t3.transferred += atob(d.data).length;
        t3.progress = Math.round(t3.transferred / t3.size * 100); t3.status = 'receiving';
        renderTransfers();
      }
    } else if (msg.type === 'file-complete') {
      var t4 = fileTransfers[d.id];
      if (t4 && t4.direction === 'in') {
        t4.complete = true; t4.status = 'received';
        var parts = []; for (var c = 0; c < t4.chunks.length; c++) { var raw = atob(t4.chunks[c]); var arr = new Uint8Array(raw.length); for (var j = 0; j < raw.length; j++) arr[j] = raw.charCodeAt(j); parts.push(arr); }
        t4.blobUrl = URL.createObjectURL(new Blob(parts, { type: t4.mimeType }));
        t4.chunks = [];
        renderTransfers(); renderIncoming();
        toast('success', 'Received', t4.name);
      }
    }
  }
  window._acceptFile = function (id) {
    var t = fileTransfers[id]; if (!t) return;
    t.pending = false; t.status = 'receiving';
    ws.send(JSON.stringify({ type: 'file-accept', data: { id: id } }));
    renderIncoming(); renderTransfers();
  };
  window._rejectFile = function (id) {
    ws.send(JSON.stringify({ type: 'file-reject', data: { id: id, reason: 'rejected' } }));
    delete fileTransfers[id]; renderIncoming(); renderTransfers();
  };
  window._saveFile = function (id) {
    var t = fileTransfers[id]; if (!t || !t.blobUrl) return;
    var a = document.createElement('a'); a.href = t.blobUrl; a.download = t.name; document.body.appendChild(a); a.click(); document.body.removeChild(a);
    toast('success', 'Saved', t.name);
  };
  function statusMeta(t) {
    if (t.status === 'failed') return { color: 'var(--err)', text: 'Failed' };
    if (t.status === 'done') return { color: 'var(--ok)', text: 'Sent' };
    if (t.status === 'received') return { color: 'var(--ok)', text: 'Received' };
    if (t.status === 'receiving') return { color: 'var(--warn)', text: 'Receiving · ' + (t.progress || 0) + '%' };
    return { color: 'var(--accent)', text: 'Sending · ' + (t.progress || 0) + '%' };
  }
  function fileIconSvg() { return '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3h7l5 5v13a0 0 0 0 1 0 0H6a0 0 0 0 1 0 0z"/><path d="M13 3v5h5"/></svg>'; }
  function photoIconSvg() { return '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="8.5" cy="10" r="1.6"/><path d="M5 17l4.5-4 3 2.6L16 12l3 3.2"/></svg>'; }

  function renderIncoming() {
    var wrap = $('incoming-wrap'), list = $('incoming-list');
    var html = ''; var has = false;
    Object.keys(fileTransfers).forEach(function (id) {
      var t = fileTransfers[id];
      if (t.direction !== 'in' || !t.pending) return;
      has = true;
      var icon = t.mimeType && t.mimeType.indexOf('image/') === 0 ? photoIconSvg() : fileIconSvg();
      html +=
        '<div class="incoming-card">' +
          '<div class="incoming-head">' +
            '<span class="incoming-icon">' + icon + '</span>' +
            '<div style="flex:1;min-width:0;">' +
              '<div class="incoming-name">' + esc(t.name) + '</div>' +
              '<div class="incoming-meta">' + fmtSize(t.size) + ' · from ' + esc(serverName) + '</div>' +
            '</div>' +
          '</div>' +
          '<div class="incoming-buttons">' +
            '<button class="btn btn-ghost btn-block" onclick="window._rejectFile(\'' + id + '\')">Decline</button>' +
            '<button class="btn btn-primary btn-block" onclick="window._acceptFile(\'' + id + '\')">' +
              '<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M7 11l5 5 5-5"/><path d="M5 20h14"/></svg>' +
              'Accept' +
            '</button>' +
          '</div>' +
        '</div>';
    });
    list.innerHTML = html;
    wrap.classList.toggle('hidden', !has);
  }
  function renderTransfers() {
    var list = $('transfer-list'), empty = $('transfer-empty');
    var html = ''; var count = 0;
    Object.keys(fileTransfers).forEach(function (id) {
      var t = fileTransfers[id]; if (t.pending) return; count++;
      var m = statusMeta(t);
      var active = t.status === 'sending' || t.status === 'receiving';
      var icon = t.mimeType && t.mimeType.indexOf('image/') === 0 ? photoIconSvg() : fileIconSvg();
      html +=
        '<div class="card transfer-row">' +
          '<div class="transfer-head">' +
            '<span class="transfer-icon" style="color:' + m.color + ';">' + icon + '</span>' +
            '<div style="flex:1;min-width:0;">' +
              '<div class="transfer-name">' + esc(t.name) + '</div>' +
              '<div class="transfer-meta">' +
                '<span class="transfer-status" style="color:' + m.color + ';">' + m.text + '</span>' +
                '<span class="transfer-size">· ' + fmtSize(t.size) + '</span>' +
              '</div>' +
            '</div>' +
            (t.status === 'received'
              ? '<button class="btn btn-primary btn-sm" onclick="window._saveFile(\'' + id + '\')"><svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M7 11l5 5 5-5"/><path d="M5 20h14"/></svg>Save</button>'
              : (t.status === 'done' ? '<span style="color: var(--ok);"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4.5 12.5l4.5 4.5L19.5 6.5"/></svg></span>' : '')) +
          '</div>' +
          (active ? '<div class="bar-inner"><i style="width:' + (t.progress || 0) + '%;"></i></div>' : '') +
        '</div>';
    });
    list.innerHTML = html;
    empty.classList.toggle('hidden', count > 0);
  }

  // ── Settings sheet ──
  $('settings-btn').addEventListener('click', function () { $('settings-sheet').classList.remove('hidden'); });
  $('settings-close').addEventListener('click', function () { $('settings-sheet').classList.add('hidden'); });
  $('settings-sheet').addEventListener('click', function (e) { if (e.target === $('settings-sheet')) $('settings-sheet').classList.add('hidden'); });

  // accent picker
  var ACCENT_PRESETS = [
    { color: '#ff8a4c', on: '#1a0e06', weak: 'rgba(255,138,76,0.14)', line: 'rgba(255,138,76,0.40)' },
    { color: '#4cc2ff', on: '#06121a', weak: 'rgba(76,194,255,0.14)', line: 'rgba(76,194,255,0.40)' },
    { color: '#46d39a', on: '#06140e', weak: 'rgba(70,211,154,0.14)', line: 'rgba(70,211,154,0.40)' },
    { color: '#e8e8ea', on: '#0b0d10', weak: 'rgba(232,232,234,0.14)', line: 'rgba(232,232,234,0.40)' },
  ];
  function applyAccent(hex) {
    var p = ACCENT_PRESETS.find(function (x) { return x.color === hex; }) || ACCENT_PRESETS[0];
    var r = document.documentElement.style;
    r.setProperty('--accent', p.color);
    r.setProperty('--accent-2', p.color);
    r.setProperty('--on-accent', p.on);
    r.setProperty('--accent-weak', p.weak);
    r.setProperty('--accent-line', p.line);
    localStorage.setItem('vior_accent', p.color);
    document.querySelectorAll('.accent-swatch').forEach(function (b) {
      b.classList.toggle('active', b.dataset.accent === p.color);
      b.style.setProperty('--swatch-color', b.dataset.accent);
    });
  }
  document.querySelectorAll('.accent-swatch').forEach(function (b) {
    b.style.setProperty('--swatch-color', b.dataset.accent);
    b.addEventListener('click', function () { applyAccent(b.dataset.accent); updateAppearanceSummary(); });
  });
  applyAccent(localStorage.getItem('vior_accent') || '#ff8a4c');

  // ── Appearance subscreen ──
  function applyStyle(v) { document.documentElement.setAttribute('data-vior-style', v); localStorage.setItem('vior_style', v); setSegActive('seg-style', 'style', v); }
  function applyDensity(v) { document.documentElement.setAttribute('data-vior-density', v); localStorage.setItem('vior_density', v); setSegActive('seg-density', 'density', v); }
  function applyMotion(v) { document.documentElement.setAttribute('data-vior-motion', v); localStorage.setItem('vior_motion', v); setSegActive('seg-motion', 'motion', v); }
  function setSegActive(segId, attr, v) {
    var seg = $(segId); if (!seg) return;
    seg.querySelectorAll('.seg-btn').forEach(function (b) { b.classList.toggle('active', b.dataset[attr] === v); });
  }
  function updateAppearanceSummary() {
    var style = localStorage.getItem('vior_style') || 'precise';
    var density = localStorage.getItem('vior_density') || 'regular';
    var motion = localStorage.getItem('vior_motion') || 'expressive';
    var hex = (localStorage.getItem('vior_accent') || '#ff8a4c').toLowerCase();
    var name = { '#ff8a4c': 'orange', '#4cc2ff': 'blue', '#46d39a': 'green', '#e8e8ea': 'white' }[hex] || 'custom';
    var sum = $('appearance-summary'); if (sum) sum.textContent = style + ' · ' + name + ' · ' + density + ' · ' + motion;
  }
  document.querySelectorAll('#seg-style .seg-btn').forEach(function (b) { b.addEventListener('click', function () { applyStyle(b.dataset.style); updateAppearanceSummary(); }); });
  document.querySelectorAll('#seg-density .seg-btn').forEach(function (b) { b.addEventListener('click', function () { applyDensity(b.dataset.density); updateAppearanceSummary(); }); });
  document.querySelectorAll('#seg-motion .seg-btn').forEach(function (b) { b.addEventListener('click', function () { applyMotion(b.dataset.motion); updateAppearanceSummary(); }); });
  applyStyle(localStorage.getItem('vior_style') || 'precise');
  applyDensity(localStorage.getItem('vior_density') || 'regular');
  applyMotion(localStorage.getItem('vior_motion') || 'expressive');
  updateAppearanceSummary();

  $('open-appearance').addEventListener('click', function () { $('settings-main').classList.add('hidden'); $('appearance-view').classList.remove('hidden'); });
  $('appearance-back').addEventListener('click', function () { $('appearance-view').classList.add('hidden'); $('settings-main').classList.remove('hidden'); });
  $('appearance-done').addEventListener('click', function () { $('appearance-view').classList.add('hidden'); $('settings-main').classList.remove('hidden'); });

  // Generic toggles bound to localStorage keys.
  document.querySelectorAll('.vior-toggle').forEach(function (t) {
    var key = t.dataset.key;
    var def = key === 'vior_usb_only' ? '0' : '1';
    var on = (localStorage.getItem(key) || def) !== '0';
    t.classList.toggle('off', !on);
    t.addEventListener('click', function () {
      on = !on;
      t.classList.toggle('off', !on);
      localStorage.setItem(key, on ? '1' : '0');
      if (key === 'vior_wifi' || key === 'vior_usb_only') {
        // Restart discovery to apply.
        if (on || key === 'vior_usb_only') startDiscovery();
        toast('info', 'Updated', t.previousElementSibling.querySelector('div').textContent + ' ' + (on ? 'enabled' : 'disabled'));
      }
    });
  });

  // Open Android Wi-Fi settings via intent URI.
  $('open-wifi-settings').addEventListener('click', function () {
    try { window.location.href = 'intent:#Intent;action=android.settings.WIFI_SETTINGS;end'; }
    catch (e) { toast('error', 'Cannot open Wi-Fi settings', String(e.message || e)); }
  });

  // Paste URL from clipboard.
  $('paste-url-btn').addEventListener('click', async function () {
    try {
      var text = await navigator.clipboard.readText();
      if (!text) { toast('warning', 'Clipboard empty', null); return; }
      var m = text.match(/(?:https?:\/\/)?([\d.]+)(?::(\d+))?(?:[?&]pair=([A-F0-9]+))?/i);
      if (!m) { toast('error', 'No URL in clipboard', text.slice(0, 40)); return; }
      var host = m[1], port = parseInt(m[2] || '8080'), code = m[3] || '';
      if (code) $('manual-pair').value = code.toUpperCase();
      $('settings-sheet').classList.add('hidden');
      selectServer(host, port, host, '');
      doConnect();
    } catch (e) {
      toast('error', 'Paste failed', 'Clipboard access denied.');
    }
  });

  // ── USB callbacks (Java bridge) ──
  window.onUsbFrame = function (b64) {
    if (!streamVisible) { openStream(); }
    streamImg.src = 'data:image/jpeg;base64,' + b64;
    var ld = $('stream-loading'); if (!ld.classList.contains('hidden')) ld.classList.add('hidden');
    frameCount++;
  };
  window.onUsbConnected = function () {
    connected = true;
    setConnState('online');
    $('stream-mode-text').textContent = 'USB · Live';
  };
  window.onUsbDisconnected = function () { if (streamVisible) hideStream(); };
  window.onUsbReady = function (w, h) {
    displayW = w; displayH = h;
    serverRes = w + ' × ' + h;
    var sr = $('stat-res'); if (sr) sr.textContent = serverRes;
  };

  // ── Boot ──
  setConnState('offline');
  startDiscovery();
})();
