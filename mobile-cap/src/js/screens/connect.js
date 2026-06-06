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

