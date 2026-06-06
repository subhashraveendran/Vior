package protocol

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// noopHandler satisfies MessageHandler without doing anything. Used
// when the test only cares about session lifecycle, not message
// dispatch.
type noopHandler struct{}

func (noopHandler) OnHello(*Session, *HelloMessage) error                 { return nil }
func (noopHandler) OnInput(*Session, *InputMessage) error                 { return nil }
func (noopHandler) OnResize(*Session, *ResizeMessage) error               { return nil }
func (noopHandler) OnBye(*Session) error                                  { return nil }
func (noopHandler) OnFileOffer(*Session, *FileOfferMessage) error         { return nil }
func (noopHandler) OnFileAccept(*Session, *FileAcceptMessage) error       { return nil }
func (noopHandler) OnFileReject(*Session, *FileRejectMessage) error       { return nil }
func (noopHandler) OnFileChunk(*Session, *FileChunkMessage) error         { return nil }
func (noopHandler) OnFileComplete(*Session, *FileCompleteMessage) error   { return nil }
func (noopHandler) OnDownloadAccept(*Session, *DownloadAcceptMessage) error { return nil }
func (noopHandler) OnDownloadReject(*Session, *DownloadRejectMessage) error { return nil }
func (noopHandler) OnDownloadComplete(*Session, *DownloadCompleteMessage) error {
	return nil
}

// wsTestServer spins up a tiny WS endpoint that wraps the connection
// in a Session and runs ReadLoop. Returns the client-side connection
// and a teardown func. Hides the upgrader boilerplate so each test
// reads as just the behaviour under examination.
func wsTestServer(t *testing.T, h MessageHandler, onSession func(*Session)) (*websocket.Conn, func()) {
	t.Helper()
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		s := NewSession(c)
		if onSession != nil {
			onSession(s)
		}
		_ = s.ReadLoop(h)
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	return c, func() {
		_ = c.Close()
		srv.Close()
	}
}

// TestPingPongRefreshesDeadline sends an app-level MsgPing and asserts
// the server replies with MsgPong promptly. Indirectly proves the
// ReadLoop refreshed its deadline (the connection survives long
// enough to receive the reply).
func TestPingPongRefreshesDeadline(t *testing.T) {
	c, teardown := wsTestServer(t, noopHandler{}, nil)
	defer teardown()

	pingPayload, _ := Encode(MsgPing, nil)
	if err := c.WriteMessage(websocket.TextMessage, pingPayload); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	env, err := Decode(msg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgPong {
		t.Fatalf("got %s, want %s", env.Type, MsgPong)
	}
}

// TestPongUpdatesHealth verifies that an app-level ping moves
// lastPongAt forward, so the health logger / desktop status indicator
// sees a fresh value within tens of milliseconds.
func TestPongUpdatesHealth(t *testing.T) {
	var sess *Session
	var mu sync.Mutex
	c, teardown := wsTestServer(t, noopHandler{}, func(s *Session) {
		mu.Lock()
		sess = s
		mu.Unlock()
	})
	defer teardown()

	// Wait for the server to set the session pointer.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := sess != nil
		mu.Unlock()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if sess == nil {
		t.Fatal("session not captured")
	}

	time.Sleep(50 * time.Millisecond)
	before := sess.Snapshot().LastPongAgo

	pingPayload, _ := Encode(MsgPing, nil)
	if err := c.WriteMessage(websocket.TextMessage, pingPayload); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	// Drain the pong so it doesn't pile up in the client buffer.
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	// Give the server a beat to update its counters.
	time.Sleep(20 * time.Millisecond)

	after := sess.Snapshot().LastPongAgo
	if after >= before {
		t.Fatalf("lastPongAgo did not decrease: before=%s after=%s", before, after)
	}
}

// TestFireDisconnectOnReadFailure forces a read error by closing the
// client conn under the server and verifies FireDisconnect fires
// exactly once even if the caller also invokes it from a higher layer
// (the real stream package wraps the OnClientDisconnect call in a
// FireDisconnect call too — so it's racing the read-loop's own
// teardown). The sync.Once guarantee is what stops the macOS virtual
// display getting torn down twice.
func TestFireDisconnectOnReadFailure(t *testing.T) {
	var sess *Session
	var mu sync.Mutex
	done := make(chan struct{})

	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		s := NewSession(c)
		mu.Lock()
		sess = s
		mu.Unlock()
		_ = s.ReadLoop(noopHandler{})
		close(done)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Wait for session.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := sess != nil
		mu.Unlock()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if sess == nil {
		t.Fatal("session not captured")
	}

	// Hard-close the underlying TCP without a polite close frame —
	// simulates a WiFi blip / mobile screen-lock killing the socket.
	if nc, ok := c.UnderlyingConn().(*net.TCPConn); ok {
		_ = nc.SetLinger(0)
	}
	_ = c.Close()
	<-done

	var n int
	for i := 0; i < 5; i++ {
		sess.FireDisconnect(func() { n++ })
	}
	if n != 1 {
		t.Fatalf("FireDisconnect ran %d times, want 1", n)
	}
}

// TestOccupiedSecondConnectCloses asserts that the gorilla writer can
// send an error envelope + close cleanly. The stream package uses this
// path when a second client tries to grab a server that already has
// one. The replaced tab must see {"type":"error","code":"occupied"}
// and a clean close — not a reconnect loop.
func TestOccupiedSecondConnectCloses(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		s := NewSession(c)
		_ = s.Send(MsgError, &ErrorMessage{Code: "occupied", Message: "Another device is already connected"})
		_ = s.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read error frame: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != MsgError {
		t.Fatalf("got %s, want %s", env.Type, MsgError)
	}
	var em ErrorMessage
	if err := json.Unmarshal(env.Data, &em); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if em.Code != "occupied" {
		t.Fatalf("got code %q, want occupied", em.Code)
	}
	// Server should then close. Next read returns an error.
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("expected close after occupied error, got another message")
	}
}
