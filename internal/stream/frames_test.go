package stream

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/subhashraveendran/vior/internal/protocol"
	"github.com/subhashraveendran/vior/internal/securechan"
)

// wsFrameTestServer is wsTestServer's sibling for tests that also need the
// server value itself (to install a frame channel and watch the secure
// frame pump register as a distributor client).
func wsFrameTestServer(t *testing.T) (*MJPEGServer, *websocket.Conn, *stubHandler, func()) {
	t.Helper()

	prev := GetSecurityMode()
	SetSecurityMode(SecurePreferred)

	h := newStubHandler()
	srv := &MJPEGServer{
		clients: map[chan []byte]struct{}{},
		handler: h,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(srv.handleWebSocket))
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		ts.Close()
		SetSecurityMode(prev)
		t.Fatalf("dial: %v", err)
	}

	return srv, conn, h, func() {
		conn.Close()
		ts.Close()
		srv.SetFrameCh(nil)
		SetSecurityMode(prev)
	}
}

// secureFrameSession drives a full handshake + sealed hello and installs a
// live frame channel, waiting until the secure frame pump has registered.
func secureFrameSession(t *testing.T) (*MJPEGServer, *websocket.Conn, *stubHandler, chan []byte, *securechan.Channel, func()) {
	t.Helper()
	srv, conn, h, cleanup := wsFrameTestServer(t)

	ch, _ := clientHandshake(t, conn, ChannelSecret())
	sendSealed(t, conn, ch, protocol.MsgHello, &protocol.HelloMessage{
		Width: 1170, Height: 2532, DPR: 3, Name: "frame-phone",
		PairCode: PairCode(), Intent: "files", SkipDisplay: true,
	})
	select {
	case <-h.connected:
	case <-time.After(5 * time.Second):
		cleanup()
		t.Fatal("sealed hello never admitted")
	}

	frameCh := make(chan []byte, 4)
	srv.SetFrameCh(frameCh)

	// The pump registers as a distributor client right before ReadLoop;
	// wait for it so pushed frames cannot be fanned out into nothing.
	deadline := time.Now().Add(5 * time.Second)
	for srv.ClientCount() == 0 {
		if time.Now().After(deadline) {
			cleanup()
			t.Fatal("secure frame pump never registered as a client")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return srv, conn, h, frameCh, ch, cleanup
}

// readSealedFrame reads binary messages until it opens one; returns the
// sealed plaintext.
func readSealedFrame(t *testing.T, c *websocket.Conn, ch *securechan.Channel) []byte {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	frameType, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read sealed frame: %v", err)
	}
	if frameType != websocket.BinaryMessage {
		t.Fatalf("secure channel carried frame type %d, want binary", frameType)
	}
	plain, err := ch.Open(msg)
	if err != nil {
		t.Fatalf("open sealed frame: %v", err)
	}
	return plain
}

// The point of issue #87: a secure session's screen frames arrive as sealed
// binary WS messages — 0x01-prefixed plaintext the client can Open — instead
// of plain-HTTP MJPEG.
func TestSecureSessionReceivesSealedFramesOverWS(t *testing.T) {
	_, conn, _, frameCh, ch, cleanup := secureFrameSession(t)
	defer cleanup()

	jpeg := bytes.Repeat([]byte{0xAB}, 200*1024)
	frameCh <- jpeg

	plain := readSealedFrame(t, conn, ch)
	if len(plain) == 0 || plain[0] != protocol.FramePrefixJPEG {
		t.Fatalf("frame plaintext starts with %#x, want FramePrefixJPEG (%#x)",
			plain[:1], protocol.FramePrefixJPEG)
	}
	if !bytes.Equal(plain[1:], jpeg) {
		t.Fatal("frame payload corrupted between distributor and client")
	}
}

// Input must keep flowing while frames stream — the pump takes the session
// write lock per frame, never a standing claim, so a sealed input message
// interleaved with video still reaches the handler.
func TestInputStillFlowsDuringFrameStreaming(t *testing.T) {
	_, conn, h, frameCh, ch, cleanup := secureFrameSession(t)
	defer cleanup()

	frameCh <- bytes.Repeat([]byte{0x01}, 100*1024)
	_ = readSealedFrame(t, conn, ch)

	sendSealed(t, conn, ch, protocol.MsgInput, &protocol.InputMessage{
		Event: "key", Action: "down", Key: "a",
	})

	select {
	case msg := <-h.inputs:
		if msg.Key != "a" {
			t.Fatalf("input key = %q, want %q", msg.Key, "a")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("input never reached the handler while frames were streaming")
	}

	// And frames continue after the input.
	next := bytes.Repeat([]byte{0x02}, 50*1024)
	frameCh <- next
	plain := readSealedFrame(t, conn, ch)
	if plain[0] != protocol.FramePrefixJPEG || !bytes.Equal(plain[1:], next) {
		t.Fatal("frame after input did not arrive intact")
	}
}

// An oversized frame is skipped — not sent, and fatal to nothing. Later
// frames and input still work on the same session.
func TestOversizeFrameSkippedWithoutKillingSession(t *testing.T) {
	_, conn, h, frameCh, ch, cleanup := secureFrameSession(t)
	defer cleanup()

	// Sealed size would exceed the 1 MiB secure limit → pump must skip it.
	frameCh <- make([]byte, 1024*1024)
	// Give the pump a beat to consume and reject it before the next frame
	// lands, so the two cannot be coalesced by latest-frame-wins.
	time.Sleep(200 * time.Millisecond)

	small := bytes.Repeat([]byte{0x5A}, 64*1024)
	frameCh <- small

	plain := readSealedFrame(t, conn, ch)
	if plain[0] != protocol.FramePrefixJPEG || !bytes.Equal(plain[1:], small) {
		t.Fatal("frame after the oversized skip was not the small frame")
	}

	// Session is still fully alive: input round-trips.
	sendSealed(t, conn, ch, protocol.MsgInput, &protocol.InputMessage{
		Event: "key", Action: "down", Key: "z",
	})
	select {
	case <-h.inputs:
	case <-time.After(5 * time.Second):
		t.Fatal("session died after an oversized frame was skipped")
	}
}

// While a secure session is active, the plain-HTTP frame endpoints are
// loopback-only: even the paired client presenting its valid frame token is
// refused, because its frames travel the sealed channel instead.
func TestFrameEndpointsLoopbackOnlyDuringSecureSession(t *testing.T) {
	const clientIP = "192.168.1.50"
	const token = "tok_abc123"
	s := &MJPEGServer{
		clients:       map[chan []byte]struct{}{},
		frameClientIP: clientIP,
		frameToken:    token,
		currentFrame:  []byte{0xFF, 0xD8, 0xFF},
	}

	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"/stream":   s.handleStream,
		"/snapshot": s.handleSnapshot,
	}
	for path, handler := range handlers {
		req := httptest.NewRequest(http.MethodGet, path+"?t="+token, nil)
		req.RemoteAddr = clientIP + ":44444"
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s during secure session: status %d, want 403", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "encrypted") {
			t.Errorf("%s 403 body %q does not explain the encrypted-channel policy", path, rec.Body.String())
		}
	}

	// The desktop's own loopback preview keeps working, token or not.
	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	s.handleSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback /snapshot during secure session: status %d, want 200", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), s.currentFrame) {
		t.Fatal("loopback snapshot body is not the current frame")
	}
}

// Cleartext sessions keep the historical behaviour: the paired IP fetches
// frames over plain HTTP (that is all a legacy client can do), everyone
// else is refused.
func TestCleartextSessionKeepsPlainHTTPFrames(t *testing.T) {
	const clientIP = "192.168.1.50"
	s := &MJPEGServer{
		clients:       map[chan []byte]struct{}{},
		frameClientIP: clientIP,
		currentFrame:  []byte{0xFF, 0xD8, 0xFF},
	}

	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	req.RemoteAddr = clientIP + ":44444"
	rec := httptest.NewRecorder()
	s.handleSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleartext paired client /snapshot: status %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	req.RemoteAddr = "10.0.0.99:44444"
	rec = httptest.NewRecorder()
	s.handleSnapshot(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unpaired IP /snapshot: status %d, want 403", rec.Code)
	}
}

// StreamPathFor decides what the ready message advertises: cleartext
// sessions get the MJPEG path, secure sessions get nothing (their frames
// arrive over the sealed channel, and /stream would 403 them anyway).
func TestStreamPathForOmitsURLOnSecureSessions(t *testing.T) {
	srv, conn, h, cleanup := wsFrameTestServer(t)
	defer cleanup()

	ch, _ := clientHandshake(t, conn, ChannelSecret())
	sendSealed(t, conn, ch, protocol.MsgHello, &protocol.HelloMessage{
		Width: 100, Height: 100, DPR: 1, Name: "s",
		PairCode: PairCode(), Intent: "files", SkipDisplay: true,
	})
	select {
	case <-h.connected:
	case <-time.After(5 * time.Second):
		t.Fatal("hello not admitted")
	}

	srv.wsConnMu.Lock()
	sess := srv.wsConn
	srv.wsConnMu.Unlock()
	if sess == nil {
		t.Fatal("no active session")
	}
	if got := StreamPathFor(sess); got != "" {
		t.Fatalf("StreamPathFor(secure) = %q, want empty", got)
	}
}

func TestStreamPathForCleartextSession(t *testing.T) {
	// A session that never enabled the record layer advertises the
	// classic MJPEG path.
	s := &protocol.Session{}
	if got := StreamPathFor(s); got == "" {
		t.Fatal("StreamPathFor(cleartext) returned empty, want the MJPEG path")
	}
}
