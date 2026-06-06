/* ──────────────────────────────────────────────────────────────────
   Vior web client — parity with mobile app:
     • Pair-code-only LAN scan (no host/port entry)
     • Intent picker (Display / Remote / Files)
     • Tab shell with intent-based gating
     • Trackpad + soft keyboard for Remote
     • Bidirectional file transfer with Accept/Decline modal
     • Settings sheet, toasts, reconnect banner, PWA hint

   Plain vanilla JS — embedded by Go //go:embed; no build step.
   ────────────────────────────────────────────────────────────────── */
(function () {
    'use strict';

    // ── Constants ──────────────────────────────────────────────────
    var CHUNK_SIZE = 48 * 1024;          // mirrors internal/filetransfer.ChunkSize
    var WS_RECONNECT_MAX = 30000;
    var WS_RECONNECT_MIN = 1000;
    var INTENT_KEY = 'vior_intent';
    var PAIR_KEY = 'vior_pair';
    var DEVICE_ID_KEY = 'vior_device_id';
    var HOST_KEY = 'vior_last_host';      // remembered "host:port" of last successful server

    // ── DOM cache ─────────────────────────────────────────────────
    function $(id) { return document.getElementById(id); }
    function $$(sel) { return Array.prototype.slice.call(document.querySelectorAll(sel)); }

    // ── Utilities ─────────────────────────────────────────────────
    function esc(s) {
        var d = document.createElement('div');
        d.textContent = String(s == null ? '' : s);
        return d.innerHTML;
    }
    function fmtSize(b) {
        if (b < 1024) return b + ' B';
        if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
        if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
        return (b / 1073741824).toFixed(2) + ' GB';
    }
    function genID() {
        var a = new Uint8Array(8);
        (crypto || window.msCrypto).getRandomValues(a);
        return Array.prototype.map.call(a, function (b) {
            return ('0' + b.toString(16)).slice(-2);
        }).join('');
    }
    function getDeviceName() {
        var ua = navigator.userAgent;
        if (/iPad/.test(ua)) return 'iPad Browser';
        if (/iPhone/.test(ua)) return 'iPhone Browser';
        if (/Android/.test(ua)) {
            var m = ua.match(/;\s*([^;)]+)\s*Build/);
            return (m ? m[1].trim() : 'Android') + ' Browser';
        }
        if (/Mac/.test(ua)) return 'Mac Browser';
        if (/Windows/.test(ua)) return 'Windows Browser';
        if (/Linux/.test(ua)) return 'Linux Browser';
        return 'Browser';
    }
    function loadDeviceID() {
        try {
            var id = localStorage.getItem(DEVICE_ID_KEY);
            if (id) return id;
            id = 'web-' + (crypto.randomUUID
                ? crypto.randomUUID()
                : (Math.random().toString(36).slice(2) + Date.now().toString(36)));
            localStorage.setItem(DEVICE_ID_KEY, id);
            return id;
        } catch (_) { return 'web-anon-' + Date.now(); }
    }
    function lsGet(k, fallback) {
        try { return localStorage.getItem(k) || fallback || ''; }
        catch (_) { return fallback || ''; }
    }
    function lsSet(k, v) { try { localStorage.setItem(k, v); } catch (_) {} }
    function lsDel(k) { try { localStorage.removeItem(k); } catch (_) {} }

    // ── Toast system ──────────────────────────────────────────────
    function toast(tone, title, msg, ttl) {
        var host = $('toast-host');
        if (!host) return;
        var dotCls = tone === 'success' ? 'dot-ok'
            : tone === 'warn' || tone === 'warning' ? 'dot-warn'
            : tone === 'error' ? 'dot-err'
            : 'dot-ok';
        var el = document.createElement('div');
        el.className = 'toast';
        el.innerHTML =
            '<span class="dot ' + dotCls + '"></span>' +
            '<div class="toast-body">' +
                '<div class="toast-title">' + esc(title) + '</div>' +
                (msg ? '<div class="toast-msg">' + esc(msg) + '</div>' : '') +
            '</div>';
        host.appendChild(el);
        var lifetime = ttl || 3400;
        setTimeout(function () {
            el.classList.add('toast-out');
            setTimeout(function () { if (el.parentNode) el.parentNode.removeChild(el); }, 220);
        }, lifetime);
    }

    // ── State ─────────────────────────────────────────────────────
    var state = {
        ws: null,
        host: '',                // resolved "host:port"
        scheme: 'http:',
        wsScheme: 'ws:',
        displayWidth: 0,
        displayHeight: 0,
        connected: false,
        reconnectDelay: WS_RECONNECT_MIN,
        reconnectTimer: null,
        currentTab: 'display',
        serverName: 'Vior',
        intent: 'display',
        deviceID: loadDeviceID(),
        pairCode: '',
        deferredPwaPrompt: null,
        scanAbortControllers: []
    };

    // ── Intent management ─────────────────────────────────────────
    function getIntent() {
        var v = lsGet(INTENT_KEY);
        return (v === 'remote' || v === 'files') ? v : 'display';
    }
    function setIntent(v) {
        if (v !== 'display' && v !== 'remote' && v !== 'files') v = 'display';
        lsSet(INTENT_KEY, v);
        state.intent = v;
        applyIntentToUI();
    }
    function applyIntentToUI() {
        // Tab visibility per intent. Display intent shows all tabs; Remote
        // hides Display (no stream); Files hides both Display + Remote.
        var tabDisplay = document.querySelector('.tab-item[data-tab="display"]');
        var tabRemote = document.querySelector('.tab-item[data-tab="remote"]');
        var tabFiles = document.querySelector('.tab-item[data-tab="files"]');

        if (tabDisplay) tabDisplay.classList.remove('hidden');
        if (tabRemote) tabRemote.classList.remove('hidden');
        if (tabFiles) tabFiles.classList.remove('hidden');

        if (state.intent === 'remote') {
            if (tabDisplay) tabDisplay.classList.add('hidden');
        } else if (state.intent === 'files') {
            if (tabDisplay) tabDisplay.classList.add('hidden');
            if (tabRemote) tabRemote.classList.add('hidden');
        }
        var streamEmpty = $('stream-empty');
        var streamImg = $('stream');
        if (state.intent === 'display') {
            if (streamEmpty) streamEmpty.classList.add('hidden');
            if (streamImg) streamImg.style.display = '';
        } else {
            if (streamEmpty) streamEmpty.classList.remove('hidden');
            if (streamImg) streamImg.style.display = 'none';
        }
        var sheetIntent = $('sheet-intent');
        if (sheetIntent) sheetIntent.textContent = state.intent;
    }

    // ── View routing ──────────────────────────────────────────────
    function showView(id) {
        ['landing-view', 'shell-view', 'disconnected-view'].forEach(function (v) {
            var el = $(v);
            if (!el) return;
            if (v === id) el.classList.remove('hidden');
            else el.classList.add('hidden');
        });
    }
    function switchTab(name) {
        // If the active tab is hidden by intent, fall back to first visible.
        var btn = document.querySelector('.tab-item[data-tab="' + name + '"]');
        if (btn && btn.classList.contains('hidden')) {
            var first = document.querySelector('.tab-item:not(.hidden)');
            if (first) name = first.dataset.tab;
        }
        state.currentTab = name;
        $$('.tab-item').forEach(function (el) {
            el.classList.toggle('active', el.dataset.tab === name);
        });
        $$('.pane').forEach(function (p) {
            p.classList.toggle('active', p.id === 'pane-' + name);
        });
    }

    // ── Intent picker wiring ──────────────────────────────────────
    function showIntentPicker() { $('intent-overlay').classList.remove('hidden'); }
    function hideIntentPicker() { $('intent-overlay').classList.add('hidden'); }
    $$('#intent-overlay .intent-tile').forEach(function (tile) {
        tile.addEventListener('click', function () {
            var v = tile.dataset.intent || 'display';
            setIntent(v);
            hideIntentPicker();
            toast('success', 'Mode set',
                v === 'display' ? 'Display — use this browser as a second screen.' :
                v === 'remote' ? 'Remote — control your Mac from here.' :
                'Files — send files only.');
            // If already connected, the user changed intent mid-session;
            // close & reconnect so the desktop tears down the virtual
            // display (or builds one) to match.
            if (state.connected) {
                disconnect();
                setTimeout(function () { connect(); }, 200);
            }
        });
    });
    $('intent-link').addEventListener('click', showIntentPicker);
    $('sheet-change-intent').addEventListener('click', function () {
        closeSheet();
        showIntentPicker();
    });

    // ── Pair-code input formatting ────────────────────────────────
    // Pair code is the machine's stable 4-digit "phone number". Strip
    // anything that isn't a decimal digit and clamp to 4 chars.
    var pairInput = $('pair-input');
    function formatPair(raw) {
        return (raw || '').replace(/[^0-9]/g, '').slice(0, 4);
    }
    function strippedPair() {
        return ((pairInput && pairInput.value) || '').replace(/[^0-9]/g, '');
    }
    pairInput.addEventListener('input', function () {
        var f = formatPair(pairInput.value);
        if (pairInput.value !== f) pairInput.value = f;
    });
    pairInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); $('connect-btn').click(); }
    });
    // Pre-fill from URL ?pair=1234 (server's QR uses this) or remembered.
    (function bootstrapPair() {
        var u = new URL(location.href);
        var fromUrl = formatPair(u.searchParams.get('pair') || '');
        if (fromUrl) {
            lsSet(PAIR_KEY, fromUrl);
            pairInput.value = fromUrl;
        } else {
            var saved = formatPair(lsGet(PAIR_KEY));
            if (saved) pairInput.value = saved;
        }
    })();

    // ── Advanced (host) disclosure ────────────────────────────────
    var advToggle = $('advanced-toggle');
    var advBlock = $('advanced-block');
    advToggle.addEventListener('click', function () {
        var open = !advBlock.classList.contains('hidden');
        advBlock.classList.toggle('hidden', open);
        advToggle.setAttribute('aria-expanded', open ? 'false' : 'true');
        advToggle.textContent = open ? 'Advanced' : 'Hide advanced';
    });

    // ── LAN scan via WebRTC ICE candidate trick ───────────────────
    // Mirrors mobile-cap/src/js/screens/connect.ts::pairOnlyConnect.
    function detectLocalIP(timeoutMs) {
        return new Promise(function (resolve) {
            var done = false;
            try {
                var pc = new RTCPeerConnection({ iceServers: [] });
                pc.createDataChannel('');
                pc.createOffer().then(function (o) { pc.setLocalDescription(o); });
                pc.onicecandidate = function (e) {
                    if (done || !e.candidate) return;
                    var m = e.candidate.candidate.match(/(\d+\.\d+\.\d+\.\d+)/);
                    if (m && m[1] !== '0.0.0.0' && !m[1].endsWith('.255')) {
                        done = true;
                        try { pc.close(); } catch (_) {}
                        resolve(m[1]);
                    }
                };
                setTimeout(function () {
                    if (done) return;
                    done = true;
                    try { pc.close(); } catch (_) {}
                    resolve(null);
                }, timeoutMs || 3000);
            } catch (_) { resolve(null); }
        });
    }

    // Probe one host:port for matching pair code.
    function probeHost(host, port, pair) {
        var ctrl = new AbortController();
        state.scanAbortControllers.push(ctrl);
        var to = setTimeout(function () { ctrl.abort(); }, 1500);
        return fetch('http://' + host + ':' + port + '/info', { signal: ctrl.signal })
            .then(function (r) { return r.ok ? r.json() : null; })
            .then(function (info) {
                clearTimeout(to);
                if (!info) return null;
                if ((info.pairCode || '') === pair) {
                    return { host: host, port: port, info: info };
                }
                return null;
            })
            .catch(function () { clearTimeout(to); return null; });
    }
    function abortScan() {
        state.scanAbortControllers.forEach(function (c) { try { c.abort(); } catch (_) {} });
        state.scanAbortControllers = [];
    }

    // Resolve the host: prefer explicit override, then try same-host
    // (we're already loaded from a Vior server!), then scan /24.
    function resolveHost(pair) {
        return new Promise(function (resolve, reject) {
            var manualHost = ($('manual-host').value || '').trim();
            if (manualHost) {
                // Already includes port?
                if (!/:\d+$/.test(manualHost)) manualHost += ':8080';
                return resolve(manualHost);
            }

            // Fast path: we were loaded from a Vior server; just check
            // it has the right pair code.
            if (location.host) {
                probeHost(location.hostname, parseInt(location.port || '8080', 10), pair)
                    .then(function (hit) {
                        if (hit) return resolve(hit.host + ':' + hit.port);
                        // Otherwise fall through to LAN scan.
                        scanLAN(pair).then(resolve, reject);
                    });
                return;
            }
            scanLAN(pair).then(resolve, reject);
        });
    }
    function scanLAN(pair) {
        setLandingStatus('Detecting your network…');
        return detectLocalIP(2500).then(function (ip) {
            if (!ip) throw new Error('Could not detect your Wi-Fi IP');
            setLandingStatus('Scanning Wi-Fi for pair code ' + pair + '…');
            var base = ip.split('.').slice(0, 3).join('.');
            var probes = [];
            for (var i = 1; i < 255; i++) {
                probes.push(probeHost(base + '.' + i, 8080, pair));
            }
            return Promise.all(probes).then(function (results) {
                var hit = results.filter(Boolean)[0];
                if (!hit) throw new Error('No Vior server on Wi-Fi matched that pair code');
                return hit.host + ':' + hit.port;
            });
        });
    }

    // ── Connect flow ──────────────────────────────────────────────
    var connectBtn = $('connect-btn');
    var connectBtnLabel = connectBtn.querySelector('.btn-label');
    var connectBtnSpin = connectBtn.querySelector('.btn-spin');

    function setConnecting(yes) {
        connectBtn.disabled = yes;
        connectBtnSpin.classList.toggle('hidden', !yes);
        connectBtnLabel.textContent = yes ? 'Connecting…' : 'Connect';
    }
    function setLandingStatus(text) {
        var box = $('landing-status'), label = $('landing-status-text');
        if (!text) { box.classList.add('hidden'); return; }
        box.classList.remove('hidden');
        label.textContent = text;
    }

    connectBtn.addEventListener('click', function () {
        var pair = strippedPair();
        if (pair.length !== 4) {
            toast('warn', 'Pair code', 'Enter the 4-digit code shown on the desktop.');
            return;
        }
        lsSet(PAIR_KEY, pair);
        state.pairCode = pair;
        setConnecting(true);
        setLandingStatus('Looking for Vior server…');

        resolveHost(pair).then(function (host) {
            abortScan();
            setLandingStatus('');
            state.host = host;
            lsSet(HOST_KEY, host);
            connect();
        }, function (err) {
            abortScan();
            setConnecting(false);
            setLandingStatus('');
            toast('error', 'Not found', (err && err.message) || 'Could not find Vior on Wi-Fi.');
        });
    });

    function connect() {
        // Tear down previous WS if any.
        if (state.ws) {
            try { state.ws.close(); } catch (_) {}
            state.ws = null;
        }
        if (state.reconnectTimer) {
            clearTimeout(state.reconnectTimer);
            state.reconnectTimer = null;
        }
        if (!state.host) {
            // Re-issued from disconnect view: use remembered host.
            state.host = lsGet(HOST_KEY);
            if (!state.host) {
                showView('landing-view');
                setConnecting(false);
                return;
            }
        }
        setConnecting(true);

        var wsUrl = state.wsScheme + '//' + state.host + '/ws';
        var ws;
        try { ws = new WebSocket(wsUrl); }
        catch (e) {
            setConnecting(false);
            toast('error', 'Connection failed', String(e));
            return;
        }
        state.ws = ws;

        ws.onopen = function () {
            state.reconnectDelay = WS_RECONNECT_MIN;
            var dpr = window.devicePixelRatio || 1;
            // Browser displays are typically already-oriented; use innerWidth/Height
            // multiplied by DPR for the "display" size the desktop should allocate.
            var w = Math.round(window.innerWidth * dpr);
            var h = Math.round(window.innerHeight * dpr);
            var intent = getIntent();
            var skipDisplay = intent !== 'display';
            // Friendly platform label for the desktop "Trusted Devices"
            // UI. Cheap UA sniff; never used for security.
            var ua = navigator.userAgent || '';
            var platform = 'Web';
            if (/iPad|iPhone|iPod/.test(ua)) platform = 'iOS Web';
            else if (/Android/.test(ua)) platform = 'Android Web';
            else if (/Mac/.test(ua)) platform = 'Mac Web';
            else if (/Windows/.test(ua)) platform = 'Windows Web';
            else if (/Linux/.test(ua)) platform = 'Linux Web';
            var hello = {
                type: 'hello',
                data: {
                    width: w, height: h, dpr: dpr,
                    name: getDeviceName(),
                    mode: 'extend',
                    pairCode: state.pairCode || lsGet(PAIR_KEY),
                    deviceId: state.deviceID,
                    intent: intent,
                    skipDisplay: skipDisplay,
                    platform: platform
                }
            };
            ws.send(JSON.stringify(hello));
        };

        ws.onmessage = function (evt) {
            var msg;
            try { msg = JSON.parse(evt.data); } catch (_) { return; }
            handleWSMessage(msg);
        };

        ws.onclose = function () {
            var streamImg = $('stream');
            if (streamImg) streamImg.src = '';
            if (state.connected) {
                state.connected = false;
                showView('disconnected-view');
                $('disconnect-sub').textContent = 'Attempting to reconnect…';
                scheduleReconnect();
            } else {
                // Open failed before ready.
                setConnecting(false);
                showView('landing-view');
            }
        };
        ws.onerror = function () { /* close fires after */ };
    }

    function handleWSMessage(msg) {
        if (msg.type === 'ready') {
            state.connected = true;
            setConnecting(false);
            var res = (msg.data.resolution || '0x0').split('x');
            state.displayWidth = parseInt(res[0], 10) || 0;
            state.displayHeight = parseInt(res[1], 10) || 0;
            // Server may return empty streamUrl when intent != display.
            var streamImg = $('stream');
            if (msg.data.streamUrl) {
                streamImg.src = state.scheme + '//' + state.host + msg.data.streamUrl;
            } else {
                streamImg.src = '';
            }
            $('sheet-res').textContent = msg.data.resolution || '—';
            $('sheet-server').textContent = state.host;
            $('conn-server').textContent = state.serverName || state.host;
            // Look up server name from /info (best-effort).
            fetch(state.scheme + '//' + state.host + '/info')
                .then(function (r) { return r.json(); })
                .then(function (info) {
                    if (info && info.name) {
                        state.serverName = info.name;
                        $('conn-server').textContent = info.name;
                        $('sheet-server').textContent = info.name + ' · ' + state.host;
                    }
                })
                .catch(function () {});
            showView('shell-view');
            // Land on the tab that matches the intent.
            if (state.intent === 'remote') switchTab('remote');
            else if (state.intent === 'files') switchTab('files');
            else switchTab('display');
            toast('success', 'Connected', state.intent === 'display' ? 'Stream is live.'
                : state.intent === 'remote' ? 'Remote control ready.'
                : 'Ready to transfer files.');
        } else if (msg.type === 'file-offer') {
            handleFileOffer(msg.data);
        } else if (msg.type === 'file-accept') {
            handleFileAccept(msg.data);
        } else if (msg.type === 'file-reject') {
            handleFileReject(msg.data);
        } else if (msg.type === 'file-chunk') {
            handleFileChunk(msg.data);
        } else if (msg.type === 'file-complete') {
            handleFileComplete(msg.data);
        } else if (msg.type === 'incoming-file') {
            handleIncomingFile(msg.data);
        } else if (msg.type === 'error') {
            var code = (msg.data && msg.data.code) || '';
            var text = (msg.data && msg.data.message) || 'Unknown error';
            if (code === 'occupied') {
                toast('error', 'Server busy', 'Another device is already connected.');
                // Don't reconnect-loop into a server that's occupied.
                state.connected = false;
                setConnecting(false);
                showView('landing-view');
            } else if (code === 'pair_mismatch') {
                toast('error', 'Pair code rejected', text);
                lsDel(PAIR_KEY);
                state.connected = false;
                setConnecting(false);
                showView('landing-view');
                pairInput.value = '';
                setTimeout(function () { try { pairInput.focus(); } catch (_) {} }, 60);
            } else {
                toast('error', 'Error', text);
            }
        }
    }

    function scheduleReconnect() {
        if (state.reconnectTimer) clearTimeout(state.reconnectTimer);
        state.reconnectDelay = Math.min(state.reconnectDelay * 2, WS_RECONNECT_MAX);
        var seconds = Math.ceil(state.reconnectDelay / 1000);
        $('disconnect-sub').textContent = 'Reconnecting in ' + seconds + 's…';
        state.reconnectTimer = setTimeout(function () {
            $('disconnect-sub').textContent = 'Reconnecting…';
            connect();
        }, state.reconnectDelay);
    }

    function disconnect() {
        state.connected = false;
        if (state.reconnectTimer) {
            clearTimeout(state.reconnectTimer);
            state.reconnectTimer = null;
        }
        if (state.ws) {
            try { state.ws.close(); } catch (_) {}
            state.ws = null;
        }
        var streamImg = $('stream');
        if (streamImg) streamImg.src = '';
        showView('landing-view');
        setConnecting(false);
    }

    $('reconnect-now-btn').addEventListener('click', function () {
        if (state.reconnectTimer) {
            clearTimeout(state.reconnectTimer);
            state.reconnectTimer = null;
        }
        state.reconnectDelay = WS_RECONNECT_MIN;
        connect();
    });
    $('back-landing-btn').addEventListener('click', disconnect);

    // ── Settings sheet ────────────────────────────────────────────
    function openSheet() { $('settings-sheet').classList.remove('hidden'); }
    function closeSheet() { $('settings-sheet').classList.add('hidden'); }
    $('settings-btn').addEventListener('click', openSheet);
    $('sheet-close').addEventListener('click', closeSheet);
    $('settings-sheet').addEventListener('click', function (e) {
        if (e.target === $('settings-sheet')) closeSheet();
    });
    $('sheet-fullscreen').addEventListener('click', function () {
        closeSheet();
        toggleFullscreen();
    });
    $('sheet-disconnect').addEventListener('click', function () {
        closeSheet();
        disconnect();
        toast('info', 'Disconnected', 'Session ended.');
    });
    $('sheet-forget').addEventListener('click', function () {
        lsDel(PAIR_KEY);
        lsDel(DEVICE_ID_KEY);
        lsDel(HOST_KEY);
        state.deviceID = loadDeviceID();
        state.pairCode = '';
        closeSheet();
        disconnect();
        if (pairInput) pairInput.value = '';
        toast('info', 'Forgotten', 'Pair code and device ID cleared.');
    });

    function toggleFullscreen() {
        if (document.fullscreenElement) {
            document.exitFullscreen().catch(function () {});
        } else {
            (document.documentElement.requestFullscreen ||
             document.documentElement.webkitRequestFullscreen).call(document.documentElement)
                .catch(function () {});
        }
    }
    $('fullscreen-btn').addEventListener('click', toggleFullscreen);

    // ── Tabs ──────────────────────────────────────────────────────
    $$('.tab-item').forEach(function (btn) {
        btn.addEventListener('click', function () {
            switchTab(btn.dataset.tab);
        });
    });

    // ── Display pane: stream interactions ─────────────────────────
    var streamImg = $('stream');
    function mapTouch(touch) {
        var rect = streamImg.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) return { x: 0, y: 0 };
        var x = (touch.clientX - rect.left) / rect.width * state.displayWidth;
        var y = (touch.clientY - rect.top) / rect.height * state.displayHeight;
        return { x: Math.round(x), y: Math.round(y) };
    }
    function wsSend(obj) {
        if (state.ws && state.ws.readyState === WebSocket.OPEN) {
            state.ws.send(JSON.stringify(obj));
        }
    }
    function sendStreamInput(event, action, x, y, extra) {
        var data = { event: event, action: action, x: x, y: y };
        if (extra) for (var k in extra) data[k] = extra[k];
        wsSend({ type: 'input', data: data });
    }
    streamImg.addEventListener('touchstart', function (e) {
        if (state.intent !== 'display' || !state.displayWidth) return;
        e.preventDefault();
        var p = mapTouch(e.changedTouches[0]);
        sendStreamInput('touch', 'down', p.x, p.y);
    }, { passive: false });
    streamImg.addEventListener('touchmove', function (e) {
        if (state.intent !== 'display' || !state.displayWidth) return;
        e.preventDefault();
        var p = mapTouch(e.changedTouches[0]);
        sendStreamInput('touch', 'move', p.x, p.y);
    }, { passive: false });
    streamImg.addEventListener('touchend', function (e) {
        if (state.intent !== 'display' || !state.displayWidth) return;
        e.preventDefault();
        var p = mapTouch(e.changedTouches[0]);
        sendStreamInput('touch', 'up', p.x, p.y);
    }, { passive: false });

    var streamMouseDown = false;
    streamImg.addEventListener('mousedown', function (e) {
        if (state.intent !== 'display' || !state.displayWidth) return;
        streamMouseDown = true;
        var p = mapTouch(e);
        sendStreamInput('touch', 'down', p.x, p.y);
    });
    streamImg.addEventListener('mousemove', function (e) {
        if (!streamMouseDown || state.intent !== 'display') return;
        var p = mapTouch(e);
        sendStreamInput('touch', 'move', p.x, p.y);
    });
    streamImg.addEventListener('mouseup', function (e) {
        if (!streamMouseDown) return;
        streamMouseDown = false;
        var p = mapTouch(e);
        sendStreamInput('touch', 'up', p.x, p.y);
    });
    streamImg.addEventListener('mouseleave', function (e) {
        if (!streamMouseDown) return;
        streamMouseDown = false;
        var p = mapTouch(e);
        sendStreamInput('touch', 'up', p.x, p.y);
    });
    streamImg.addEventListener('wheel', function (e) {
        if (state.intent !== 'display') return;
        e.preventDefault();
        sendStreamInput('scroll', 'scroll', 0, 0, {
            dx: Math.round(e.deltaX),
            dy: Math.round(e.deltaY)
        });
    }, { passive: false });

    // ── Remote pane: trackpad ─────────────────────────────────────
    // Mirrors mobile-cap/src/js/screens/remote.ts behavior:
    //   one finger → mouse move (relative dx/dy * 2)
    //   two fingers → scroll
    //   tap (< 300ms, no movement) → click
    //   tap with 2 fingers → right click
    var trackpad = $('trackpad');
    var tpLastX = 0, tpLastY = 0, tpFingers = 0, tpMoved = false, tpStartT = 0;
    function flashPill(label) {
        var p = $('flash-pill');
        p.textContent = label;
        p.classList.remove('hidden');
        clearTimeout(flashPill._t);
        flashPill._t = setTimeout(function () { p.classList.add('hidden'); }, 850);
    }
    trackpad.addEventListener('touchstart', function (e) {
        e.preventDefault();
        tpFingers = e.touches.length;
        tpMoved = false;
        tpStartT = Date.now();
        var t = e.touches[0];
        tpLastX = t.clientX; tpLastY = t.clientY;
        $('trackpad-hint').style.display = 'none';
    }, { passive: false });
    trackpad.addEventListener('touchmove', function (e) {
        e.preventDefault();
        var t = e.touches[0];
        var dx = t.clientX - tpLastX, dy = t.clientY - tpLastY;
        tpLastX = t.clientX; tpLastY = t.clientY;
        if (Math.abs(dx) + Math.abs(dy) > 2) tpMoved = true;
        if (e.touches.length >= 2) {
            wsSend({ type: 'input', data: { event: 'scroll', dx: Math.round(dx / 4), dy: Math.round(dy / 4) } });
        } else {
            wsSend({ type: 'input', data: { event: 'mouse', action: 'move', dx: dx * 2, dy: dy * 2 } });
        }
    }, { passive: false });
    trackpad.addEventListener('touchend', function (e) {
        e.preventDefault();
        var dur = Date.now() - tpStartT;
        if (!tpMoved && dur < 300) {
            var action = tpFingers >= 2 ? 'rightclick' : 'click';
            wsSend({ type: 'input', data: { event: 'mouse', action: action } });
            flashPill(action === 'rightclick' ? 'Right click' : 'Click');
        }
        tpFingers = 0;
        $('trackpad-hint').style.display = '';
    }, { passive: false });

    // Desktop browser trackpad fallback (mouse + wheel).
    var tpMouseDown = false, tpMX = 0, tpMY = 0, tpMouseMoved = false, tpMouseStart = 0;
    trackpad.addEventListener('mousedown', function (e) {
        tpMouseDown = true; tpMouseMoved = false;
        tpMX = e.clientX; tpMY = e.clientY;
        tpMouseStart = Date.now();
        $('trackpad-hint').style.display = 'none';
    });
    trackpad.addEventListener('mousemove', function (e) {
        if (!tpMouseDown) return;
        var dx = e.clientX - tpMX, dy = e.clientY - tpMY;
        tpMX = e.clientX; tpMY = e.clientY;
        if (Math.abs(dx) + Math.abs(dy) > 2) tpMouseMoved = true;
        wsSend({ type: 'input', data: { event: 'mouse', action: 'move', dx: dx * 2, dy: dy * 2 } });
    });
    trackpad.addEventListener('mouseup', function (e) {
        if (!tpMouseDown) return;
        tpMouseDown = false;
        var dur = Date.now() - tpMouseStart;
        if (!tpMouseMoved && dur < 300) {
            var action = e.button === 2 ? 'rightclick' : 'click';
            wsSend({ type: 'input', data: { event: 'mouse', action: action } });
            flashPill(action === 'rightclick' ? 'Right click' : 'Click');
        }
        $('trackpad-hint').style.display = '';
    });
    trackpad.addEventListener('contextmenu', function (e) { e.preventDefault(); });
    trackpad.addEventListener('wheel', function (e) {
        e.preventDefault();
        wsSend({ type: 'input', data: { event: 'scroll', dx: Math.round(e.deltaX / 8), dy: Math.round(e.deltaY / 8) } });
    }, { passive: false });

    // Click / right-click buttons.
    $$('.kbd-btn[data-mb]').forEach(function (b) {
        b.addEventListener('click', function () {
            var action = b.dataset.mb === 'right' ? 'rightclick' : 'click';
            wsSend({ type: 'input', data: { event: 'mouse', action: action } });
            flashPill(action === 'rightclick' ? 'Right click' : 'Click');
        });
    });

    // ── Remote pane: shortcuts + F-keys ───────────────────────────
    var SHORTCUTS = [
        ['Cmd+c', 'Copy', '⌘C'], ['Cmd+v', 'Paste', '⌘V'],
        ['Cmd+x', 'Cut', '⌘X'], ['Cmd+z', 'Undo', '⌘Z'],
        ['Cmd+Shift+z', 'Redo', '⇧⌘Z'], ['Cmd+a', 'Sel All', '⌘A'],
        ['Cmd+s', 'Save', '⌘S'], ['Cmd+f', 'Find', '⌘F'],
        ['Cmd+Tab', 'App ⇆', '⌘⇥'], ['Cmd+Space', 'Spotlight', '⌘Sp'],
        ['Escape', 'Esc', 'esc'], ['Return', 'Enter', '↵']
    ];
    var keyGrid = $('key-grid');
    SHORTCUTS.forEach(function (s) {
        var b = document.createElement('button');
        b.className = 'keycap';
        b.dataset.key = s[0];
        b.innerHTML = '<span class="keycap-label">' + esc(s[1]) + '</span>' +
                      '<span class="keycap-sub">' + esc(s[2]) + '</span>';
        b.addEventListener('click', function () {
            wsSend({ type: 'input', data: { event: 'key', key: s[0] } });
            flashPill(s[1]);
        });
        keyGrid.appendChild(b);
    });

    // Soft keyboard: opens hidden <input>, forwards each character.
    var kbInput = $('kb-input');
    $('kb-btn').addEventListener('click', function () {
        kbInput.value = '';
        kbInput.focus();
        toast('info', 'Keyboard ready', 'Type to forward keys to the Mac.');
    });
    kbInput.addEventListener('input', function (e) {
        var data = e.data;
        if (data) {
            for (var i = 0; i < data.length; i++) {
                wsSend({ type: 'input', data: { event: 'key', key: data[i] } });
            }
        }
        kbInput.value = '';
    });
    kbInput.addEventListener('keydown', function (e) {
        var map = {
            'Backspace': 'BackSpace', 'Enter': 'Return', 'Tab': 'Tab',
            'ArrowUp': 'Up', 'ArrowDown': 'Down', 'ArrowLeft': 'Left', 'ArrowRight': 'Right',
            'Escape': 'Escape'
        };
        var k = map[e.key];
        if (k) {
            e.preventDefault();
            wsSend({ type: 'input', data: { event: 'key', key: k } });
        }
    });

    // ── Files pane ───────────────────────────────────────────────
    // fileTransfers: id -> { state: incoming|sending|receiving|done|err,
    //   direction:in|out, ... }
    var fileTransfers = {};
    var incomingHttp = {}; // id -> {url, ...} (HTTP-download path)

    function renderTransfers() {
        var list = $('transfer-list'), empty = $('transfer-empty');
        var html = ''; var count = 0;
        Object.keys(fileTransfers).forEach(function (id) {
            var t = fileTransfers[id];
            if (t.pending) return; // pending offers go in incoming list
            count++;
            var statusCls = t.state === 'done' || t.state === 'received' ? 'status-ok'
                : t.state === 'err' ? 'status-err' : 'status-active';
            var statusTxt = t.state === 'done' ? 'Sent'
                : t.state === 'received' ? 'Received'
                : t.state === 'sending' ? 'Sending · ' + (t.progress || 0) + '%'
                : t.state === 'receiving' ? 'Receiving · ' + (t.progress || 0) + '%'
                : t.state === 'err' ? 'Failed' : '';
            var active = t.state === 'sending' || t.state === 'receiving';
            var saveBtn = t.state === 'received' && t.blobUrl
                ? '<button class="btn btn-ghost btn-sm" data-act="save" data-id="' + id + '">Save</button>'
                : '';
            html +=
                '<div class="transfer-row">' +
                    '<div class="transfer-head">' +
                        renderThumb(t) +
                        '<div class="file-meta">' +
                            '<div class="file-name">' + esc(t.name) + '</div>' +
                            '<div class="file-sub">' +
                                '<span class="status-text ' + statusCls + '">' + statusTxt + '</span>' +
                                ' · ' + esc(fmtSize(t.size)) +
                            '</div>' +
                        '</div>' +
                        saveBtn +
                    '</div>' +
                    (active ? '<div class="progress-bar"><i style="width:' + (t.progress || 0) + '%;"></i></div>' : '') +
                '</div>';
        });
        list.innerHTML = html;
        empty.classList.toggle('hidden', count > 0);
        // Wire save buttons.
        $$('button[data-act="save"]').forEach(function (b) {
            b.addEventListener('click', function () {
                var t = fileTransfers[b.dataset.id];
                if (t && t.blobUrl) saveBlob(t);
            });
        });
    }
    function renderThumb(t) {
        if (t.preview) {
            var src = t.preview.indexOf('data:') === 0
                ? t.preview
                : ('data:' + (t.mimeType || 'image/jpeg') + ';base64,' + t.preview);
            return '<img class="preview-thumb" src="' + esc(src) + '" alt="">';
        }
        var ext = (t.name.split('.').pop() || 'FILE').slice(0, 4).toUpperCase();
        return '<span class="preview-badge">' + esc(ext) + '</span>';
    }
    function renderIncoming() {
        var wrap = $('incoming-wrap'), list = $('incoming-list');
        var html = ''; var has = false;
        Object.keys(fileTransfers).forEach(function (id) {
            var t = fileTransfers[id];
            if (t.direction !== 'in' || !t.pending) return;
            has = true;
            html +=
                '<div class="incoming-card">' +
                    '<div class="incoming-head">' +
                        renderThumb(t) +
                        '<div class="file-meta">' +
                            '<div class="file-name">' + esc(t.name) + '</div>' +
                            '<div class="file-sub">' + esc(fmtSize(t.size)) + ' · from ' + esc(state.serverName) + '</div>' +
                        '</div>' +
                    '</div>' +
                    '<div class="incoming-buttons">' +
                        '<button class="btn btn-ghost" data-act="decline" data-id="' + id + '">Decline</button>' +
                        '<button class="btn btn-primary" data-act="accept" data-id="' + id + '">Accept</button>' +
                    '</div>' +
                '</div>';
        });
        list.innerHTML = html;
        wrap.classList.toggle('hidden', !has);
        // Wire accept/decline.
        $$('button[data-act="accept"]').forEach(function (b) {
            b.addEventListener('click', function () { acceptIncoming(b.dataset.id); });
        });
        $$('button[data-act="decline"]').forEach(function (b) {
            b.addEventListener('click', function () { declineIncoming(b.dataset.id); });
        });
    }

    function saveBlob(t) {
        var a = document.createElement('a');
        a.href = t.blobUrl;
        a.download = t.name;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        toast('success', 'Saved', t.name);
    }

    // ── Sending files (browser → desktop) ─────────────────────────
    $('send-file-btn').addEventListener('click', function () {
        $('file-input').click();
    });
    $('file-input').addEventListener('change', function (e) {
        var files = e.target.files;
        if (!files) return;
        for (var i = 0; i < files.length; i++) sendFile(files[i]);
        e.target.value = '';
    });

    function sendFile(file) {
        if (!state.connected) {
            toast('warn', 'Not connected', 'Connect to a Vior server before sending files.');
            return;
        }
        var id = genID();
        var reader = new FileReader();
        reader.onload = function () {
            var data = new Uint8Array(reader.result);
            var t = {
                id: id,
                name: file.name,
                size: file.size,
                mimeType: file.type || 'application/octet-stream',
                preview: '',
                transferred: 0,
                direction: 'out',
                state: 'sending',
                progress: 0,
                data: data
            };
            fileTransfers[id] = t;
            switchTab('files');
            if (file.type && file.type.indexOf('image/') === 0) {
                var pr = new FileReader();
                pr.onload = function () {
                    t.preview = pr.result;
                    sendOffer(t);
                };
                pr.readAsDataURL(file);
            } else {
                sendOffer(t);
            }
        };
        reader.readAsArrayBuffer(file);
    }
    function sendOffer(t) {
        wsSend({
            type: 'file-offer',
            data: { id: t.id, name: t.name, size: t.size, mimeType: t.mimeType, preview: t.preview }
        });
        renderTransfers();
        toast('info', 'Offering', t.name);
    }
    function sendChunks(t) {
        var offset = 0;
        function next() {
            if (!t.data) return;
            if (offset >= t.data.length) {
                t.state = 'done';
                t.progress = 100;
                wsSend({ type: 'file-complete', data: { id: t.id, hash: '' } });
                renderTransfers();
                toast('success', 'Sent', t.name);
                return;
            }
            var end = Math.min(offset + CHUNK_SIZE, t.data.length);
            var chunk = t.data.subarray(offset, end);
            // base64 encode in small steps to avoid call-stack issues.
            var s = '';
            for (var i = 0; i < chunk.length; i++) s += String.fromCharCode(chunk[i]);
            wsSend({ type: 'file-chunk', data: { id: t.id, offset: offset, data: btoa(s) } });
            offset = end;
            t.transferred = offset;
            t.progress = Math.round(offset / t.data.length * 100);
            renderTransfers();
            setTimeout(next, 5);
        }
        next();
    }

    // ── Receiving (WS-chunked, desktop → browser legacy path) ────
    function handleFileOffer(d) {
        var t = {
            id: d.id, name: d.name || 'file', size: d.size || 0,
            mimeType: d.mimeType || 'application/octet-stream', preview: d.preview || '',
            transferred: 0, direction: 'in', pending: true, state: 'incoming',
            chunks: []
        };
        fileTransfers[d.id] = t;
        renderIncoming();
        switchTab('files');
        toast('info', 'Incoming file', t.name);
    }
    function handleFileAccept(d) {
        var t = fileTransfers[d.id];
        if (t && t.direction === 'out') sendChunks(t);
    }
    function handleFileReject(d) {
        var t = fileTransfers[d.id];
        if (t) {
            toast('warn', 'Declined', t.name);
            delete fileTransfers[d.id];
            renderTransfers();
        }
    }
    function handleFileChunk(d) {
        var t = fileTransfers[d.id];
        if (!t || t.direction !== 'in') return;
        var bin = atob(d.data);
        var arr = new Uint8Array(bin.length);
        for (var i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
        t.chunks.push(arr);
        t.transferred += arr.length;
        t.progress = t.size > 0 ? Math.round(t.transferred / t.size * 100) : 0;
        t.state = 'receiving';
        renderTransfers();
    }
    function handleFileComplete(d) {
        var t = fileTransfers[d.id];
        if (!t || t.direction !== 'in') return;
        t.blobUrl = URL.createObjectURL(new Blob(t.chunks, { type: t.mimeType }));
        t.chunks = [];
        t.state = 'received';
        t.progress = 100;
        renderTransfers();
        renderIncoming();
        toast('success', 'Received', t.name);
    }

    // ── Receiving (HTTP-download, modern desktop → browser path) ─
    function handleIncomingFile(d) {
        if (!d || !d.id || !d.url) return;
        var t = {
            id: d.id, name: d.name || 'file', size: d.size || 0,
            mimeType: d.mime || 'application/octet-stream', preview: d.preview || '',
            transferred: 0, direction: 'in', pending: true, state: 'incoming'
        };
        fileTransfers[d.id] = t;
        incomingHttp[d.id] = { url: d.url };
        renderIncoming();
        switchTab('files');
        toast('info', 'Incoming file', t.name);
    }
    function acceptIncoming(id) {
        var t = fileTransfers[id];
        if (!t) return;
        t.pending = false;
        if (incomingHttp[id]) {
            // HTTP-download path.
            fetchDownload(id);
        } else {
            // WS-chunked path.
            t.state = 'receiving';
            wsSend({ type: 'file-accept', data: { id: id } });
        }
        renderIncoming(); renderTransfers();
    }
    function declineIncoming(id) {
        var t = fileTransfers[id];
        if (!t) return;
        if (incomingHttp[id]) {
            wsSend({ type: 'download-reject', data: { id: id, reason: 'rejected' } });
            delete incomingHttp[id];
        } else {
            wsSend({ type: 'file-reject', data: { id: id, reason: 'rejected' } });
        }
        delete fileTransfers[id];
        renderIncoming(); renderTransfers();
        toast('info', 'Declined', t.name);
    }
    function fetchDownload(id) {
        var t = fileTransfers[id];
        var http = incomingHttp[id];
        if (!t || !http) return;
        t.state = 'receiving';
        wsSend({ type: 'download-accept', data: { id: id } });
        var url = state.scheme + '//' + state.host + http.url;
        fetch(url).then(function (resp) {
            if (!resp.ok) throw new Error('HTTP ' + resp.status);
            var total = t.size || parseInt(resp.headers.get('Content-Length') || '0', 10);
            if (resp.body && resp.body.getReader) {
                var reader = resp.body.getReader();
                var parts = [];
                var got = 0;
                function pump() {
                    return reader.read().then(function (r) {
                        if (r.done) {
                            t.blobUrl = URL.createObjectURL(new Blob(parts, { type: t.mimeType }));
                            t.state = 'received';
                            t.progress = 100;
                            renderTransfers();
                            wsSend({ type: 'download-complete', data: { id: id } });
                            toast('success', 'Received', t.name);
                            delete incomingHttp[id];
                            // Auto-save when image (preserve mobile UX where it lands in Downloads).
                            return;
                        }
                        if (r.value) {
                            parts.push(r.value);
                            got += r.value.byteLength;
                            t.transferred = got;
                            t.progress = total > 0 ? Math.round(got / total * 100) : 0;
                            renderTransfers();
                        }
                        return pump();
                    });
                }
                return pump();
            }
            // Fallback: full blob.
            return resp.blob().then(function (blob) {
                t.blobUrl = URL.createObjectURL(blob);
                t.state = 'received';
                t.progress = 100;
                renderTransfers();
                wsSend({ type: 'download-complete', data: { id: id } });
                toast('success', 'Received', t.name);
                delete incomingHttp[id];
            });
        }).catch(function (e) {
            t.state = 'err';
            renderTransfers();
            toast('error', 'Download failed', String(e));
            wsSend({ type: 'download-reject', data: { id: id, reason: String(e) } });
            delete incomingHttp[id];
        });
    }

    // ── Resize handling ───────────────────────────────────────────
    var lastW = 0, lastH = 0, resizeTimer = null;
    function reportResize() {
        if (state.intent !== 'display' || !state.connected) return;
        var dpr = window.devicePixelRatio || 1;
        var w = Math.round(window.innerWidth * dpr);
        var h = Math.round(window.innerHeight * dpr);
        if (w === lastW && h === lastH) return;
        lastW = w; lastH = h;
        wsSend({ type: 'resize', data: { width: w, height: h, dpr: dpr } });
    }
    window.addEventListener('resize', function () {
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(reportResize, 350);
    });
    window.addEventListener('orientationchange', function () {
        setTimeout(reportResize, 350);
    });

    // ── PWA install ───────────────────────────────────────────────
    window.addEventListener('beforeinstallprompt', function (e) {
        e.preventDefault();
        state.deferredPwaPrompt = e;
        // Don't nag on landing — show after a successful connect.
        if (state.connected && !lsGet('vior_pwa_dismissed')) {
            $('pwa-hint').classList.remove('hidden');
        }
    });
    $('pwa-install-btn').addEventListener('click', function () {
        if (!state.deferredPwaPrompt) return;
        state.deferredPwaPrompt.prompt();
        state.deferredPwaPrompt.userChoice.finally(function () {
            state.deferredPwaPrompt = null;
            $('pwa-hint').classList.add('hidden');
        });
    });
    $('pwa-hint-dismiss').addEventListener('click', function () {
        $('pwa-hint').classList.add('hidden');
        lsSet('vior_pwa_dismissed', '1');
    });
    // iOS standalone hint — iOS doesn't fire beforeinstallprompt.
    function checkIosInstallHint() {
        if (window.navigator.standalone) return; // already installed
        var ua = navigator.userAgent;
        if (!/iPhone|iPad|iPod/.test(ua) || /CriOS|FxiOS/.test(ua)) return;
        if (lsGet('vior_pwa_dismissed')) return;
        if (!state.connected) return;
        // No prompt event — silently skip rather than nagging; the manifest
        // is enough for Safari users who tap Share → Add to Home Screen.
    }

    // ── Boot ──────────────────────────────────────────────────────
    function boot() {
        state.intent = getIntent();
        applyIntentToUI();
        // Compute schemes off current page (works for plain http://lan-ip).
        state.scheme = location.protocol === 'https:' ? 'https:' : 'http:';
        state.wsScheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
        // First-run gate: no intent ever picked → show picker on landing.
        var stored = lsGet(INTENT_KEY);
        showView('landing-view');
        if (!stored) {
            showIntentPicker();
        }
        // If served from a Vior host (typical) AND we have a saved pair
        // AND we have a saved host equal to this one AND a saved deviceID
        // (= server already trusts us), auto-connect.
        var savedPair = lsGet(PAIR_KEY);
        var savedHost = lsGet(HOST_KEY);
        var sameHost = location.host && savedHost &&
            (location.hostname + ':' + (location.port || '8080')) === savedHost;
        if (stored && savedPair && (savedHost && sameHost)) {
            state.pairCode = savedPair;
            state.host = savedHost;
            setTimeout(function () { connect(); }, 50);
        } else if (stored && savedPair && location.host) {
            // We loaded from a Vior server but never saved it as last-host.
            // Try direct connect against this host — fastest path.
            state.pairCode = savedPair;
            state.host = location.hostname + ':' + (location.port || '8080');
            setTimeout(function () { connect(); }, 50);
        }
    }

    // Prevent context menu globally on touch UI (right-click on trackpad
    // should fire OUR rightclick, not browser's menu).
    document.addEventListener('contextmenu', function (e) {
        var t = e.target;
        if (t && (t.id === 'trackpad' || (t.closest && t.closest('#trackpad')))) {
            e.preventDefault();
        }
    });

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', boot);
    } else {
        boot();
    }
})();
