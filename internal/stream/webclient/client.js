(function () {
    'use strict';

    var ws = null;
    var displayWidth = 0;
    var displayHeight = 0;
    var reconnectDelay = 1000;
    var maxReconnectDelay = 30000;
    var statusTimeout = null;
    var reconnectTimer = null;

    var connectingEl = document.getElementById('connecting');
    var streamViewEl = document.getElementById('stream-view');
    var disconnectedEl = document.getElementById('disconnected');
    var errorViewEl = document.getElementById('error-view');
    var streamImg = document.getElementById('stream');
    var fullscreenBtn = document.getElementById('fullscreen-btn');
    var statusBar = document.getElementById('status-bar');
    var connStatus = document.getElementById('conn-status');
    var connText = document.getElementById('conn-text');
    var errorMsg = document.getElementById('error-msg');
    var retryBtn = document.getElementById('retry-btn');

    function showView(view) {
        connectingEl.classList.add('hidden');
        streamViewEl.classList.add('hidden');
        disconnectedEl.classList.add('hidden');
        errorViewEl.classList.add('hidden');
        view.classList.remove('hidden');
    }

    function getDeviceName() {
        var ua = navigator.userAgent;
        if (/iPad/.test(ua)) return 'iPad';
        if (/iPhone/.test(ua)) return 'iPhone';
        if (/Android/.test(ua)) {
            var m = ua.match(/;\s*([^;)]+)\s*Build/);
            return m ? m[1].trim() : 'Android Device';
        }
        return 'Browser';
    }

    function connect() {
        showView(connectingEl);
        if (reconnectTimer) {
            clearTimeout(reconnectTimer);
            reconnectTimer = null;
        }

        var protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        var wsUrl = protocol + '//' + location.host + '/ws';

        try {
            ws = new WebSocket(wsUrl);
        } catch (e) {
            showError('Could not connect to server');
            return;
        }

        ws.onopen = function () {
            reconnectDelay = 1000;
            var dpr = window.devicePixelRatio || 1;
            var sw = Math.round(screen.width * dpr);
            var sh = Math.round(screen.height * dpr);
            var isLandscape = window.innerWidth > window.innerHeight;
            var w = isLandscape ? Math.max(sw, sh) : Math.min(sw, sh);
            var h = isLandscape ? Math.min(sw, sh) : Math.max(sw, sh);
            lastWidth = w;
            lastHeight = h;
            var hello = {
                type: 'hello',
                data: { width: w, height: h, dpr: dpr, name: getDeviceName() }
            };
            ws.send(JSON.stringify(hello));
        };

        ws.onmessage = function (evt) {
            var msg;
            try { msg = JSON.parse(evt.data); } catch (e) { return; }

            if (msg.type === 'ready') {
                displayWidth = parseInt(msg.data.resolution.split('x')[0], 10);
                displayHeight = parseInt(msg.data.resolution.split('x')[1], 10);
                streamImg.src = msg.data.streamUrl;
                showView(streamViewEl);
                flashStatus('Connected', true);
            } else if (msg.type === 'file-offer') {
                handleFileOffer(msg.data);
            } else if (msg.type === 'file-chunk') {
                handleFileChunk(msg.data);
            } else if (msg.type === 'file-complete') {
                handleFileComplete(msg.data);
            } else if (msg.type === 'error') {
                var errText = msg.data.message || 'Unknown error';
                if (msg.data.code === 'occupied') {
                    showError('Another device is already connected to this server.');
                } else if (msg.data.code === 'setup_failed') {
                    showError(errText);
                } else {
                    showError(errText);
                }
            }
        };

        ws.onclose = function () {
            streamImg.src = '';
            showView(disconnectedEl);
            scheduleReconnect();
        };

        ws.onerror = function () {
            // onclose will fire after
        };
    }

    function showError(msg) {
        errorMsg.textContent = msg;
        showView(errorViewEl);
    }

    function scheduleReconnect() {
        reconnectTimer = setTimeout(function () {
            reconnectDelay = Math.min(reconnectDelay * 2, maxReconnectDelay);
            connect();
        }, reconnectDelay);
    }

    function flashStatus(text, connected) {
        connText.textContent = text;
        connStatus.className = connected ? 'connected' : 'disconnected';
        statusBar.classList.add('visible');
        clearTimeout(statusTimeout);
        statusTimeout = setTimeout(function () {
            statusBar.classList.remove('visible');
        }, 3000);
    }

    // --- Retry button ---
    retryBtn.addEventListener('click', function () {
        reconnectDelay = 1000;
        connect();
    });

    // --- Touch handling ---

    function mapTouch(touch) {
        var rect = streamImg.getBoundingClientRect();
        var x = (touch.clientX - rect.left) / rect.width * displayWidth;
        var y = (touch.clientY - rect.top) / rect.height * displayHeight;
        return { x: Math.round(x), y: Math.round(y) };
    }

    function sendInput(event, action, x, y, extra) {
        if (!ws || ws.readyState !== WebSocket.OPEN) return;
        var msg = {
            type: 'input',
            data: { event: event, action: action, x: x, y: y }
        };
        if (extra) {
            for (var k in extra) msg.data[k] = extra[k];
        }
        ws.send(JSON.stringify(msg));
    }

    streamImg.addEventListener('touchstart', function (e) {
        e.preventDefault();
        var t = e.changedTouches[0];
        var p = mapTouch(t);
        sendInput('touch', 'down', p.x, p.y);
    }, { passive: false });

    streamImg.addEventListener('touchmove', function (e) {
        e.preventDefault();
        var t = e.changedTouches[0];
        var p = mapTouch(t);
        sendInput('touch', 'move', p.x, p.y);
    }, { passive: false });

    streamImg.addEventListener('touchend', function (e) {
        e.preventDefault();
        var t = e.changedTouches[0];
        var p = mapTouch(t);
        sendInput('touch', 'up', p.x, p.y);
    }, { passive: false });

    // Mouse fallback for desktop browser testing
    var mouseDown = false;

    streamImg.addEventListener('mousedown', function (e) {
        e.preventDefault();
        mouseDown = true;
        var p = mapTouch(e);
        sendInput('touch', 'down', p.x, p.y);
    });

    streamImg.addEventListener('mousemove', function (e) {
        if (!mouseDown) return;
        e.preventDefault();
        var p = mapTouch(e);
        sendInput('touch', 'move', p.x, p.y);
    });

    streamImg.addEventListener('mouseup', function (e) {
        if (!mouseDown) return;
        mouseDown = false;
        e.preventDefault();
        var p = mapTouch(e);
        sendInput('touch', 'up', p.x, p.y);
    });

    // Scroll
    streamImg.addEventListener('wheel', function (e) {
        e.preventDefault();
        sendInput('scroll', 'scroll', 0, 0, {
            dx: Math.round(e.deltaX),
            dy: Math.round(e.deltaY)
        });
    }, { passive: false });

    // --- Orientation change ---

    var lastWidth = 0;
    var lastHeight = 0;

    function checkOrientation() {
        var w = Math.round(screen.width * (window.devicePixelRatio || 1));
        var h = Math.round(screen.height * (window.devicePixelRatio || 1));

        // On mobile, screen.width/height don't change — use window.innerWidth/Height.
        var viewW = Math.round(window.innerWidth * (window.devicePixelRatio || 1));
        var viewH = Math.round(window.innerHeight * (window.devicePixelRatio || 1));

        // Detect if orientation flipped (landscape vs portrait).
        var isLandscape = window.innerWidth > window.innerHeight;
        var newW = isLandscape ? Math.max(w, h) : Math.min(w, h);
        var newH = isLandscape ? Math.min(w, h) : Math.max(w, h);

        if (newW !== lastWidth || newH !== lastHeight) {
            lastWidth = newW;
            lastHeight = newH;
            if (ws && ws.readyState === WebSocket.OPEN && displayWidth > 0) {
                var resize = {
                    type: 'resize',
                    data: {
                        width: newW,
                        height: newH,
                        dpr: window.devicePixelRatio || 1
                    }
                };
                ws.send(JSON.stringify(resize));
            }
        }
    }

    window.addEventListener('orientationchange', function () {
        setTimeout(checkOrientation, 300);
    });
    window.addEventListener('resize', function () {
        // Debounce resize events.
        clearTimeout(window._viorResizeTimer);
        window._viorResizeTimer = setTimeout(checkOrientation, 500);
    });

    // --- Fullscreen ---

    fullscreenBtn.addEventListener('click', function () {
        if (document.fullscreenElement) {
            document.exitFullscreen();
        } else {
            document.documentElement.requestFullscreen().catch(function () {});
        }
    });

    // --- File Transfer (receiving from desktop) ---

    var fileBuffers = {}; // id -> { chunks: [], name, mimeType, size, preview }

    function handleFileOffer(data) {
        // Auto-accept all file offers from desktop.
        fileBuffers[data.id] = {
            chunks: [],
            name: data.name,
            mimeType: data.mimeType,
            size: data.size,
            preview: data.preview,
            received: 0
        };
        // Send accept.
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'file-accept', data: { id: data.id } }));
        }
        flashStatus('Receiving: ' + data.name, true);
    }

    function handleFileChunk(data) {
        var buf = fileBuffers[data.id];
        if (!buf) return;
        // Decode base64 chunk.
        var binary = atob(data.data);
        var bytes = new Uint8Array(binary.length);
        for (var i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        buf.chunks.push(bytes);
        buf.received += bytes.length;
    }

    function handleFileComplete(data) {
        var buf = fileBuffers[data.id];
        if (!buf) return;

        // Combine chunks into blob.
        var blob = new Blob(buf.chunks, { type: buf.mimeType || 'application/octet-stream' });

        // Trigger download.
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url;
        a.download = buf.name;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);

        flashStatus('Received: ' + buf.name, true);
        delete fileBuffers[data.id];
    }

    // --- Start ---
    connect();
})();
