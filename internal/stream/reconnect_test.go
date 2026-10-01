package stream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/subhashraveendran/vior/internal/protocol"
)

// multiClientServer is wsTestServer without the single pre-dialled client,
// for tests that need several connections to one server.
func multiClientServer(t *testing.T) (dial func() *websocket.Conn, h *stubHandler, cleanup func()) {
	t.Helper()
	prev := GetSecurityMode()
	SetSecurityMode(SecurePreferred)

	h = newStubHandler()
	srv := &MJPEGServer{
		clients:  map[chan []byte]struct{}{},
		handler:  h,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
	}
	ts := httptest.NewServer(http.HandlerFunc(srv.handleWebSocket))
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	var conns []*websocket.Conn
	dial = func() *websocket.Conn {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		conns = append(conns, c)
		return c
	}
	cleanup = func() {
		for _, c := range conns {
			c.Close()
		}
		ts.Close()
		SetSecurityMode(prev)
	}
	return dial, h, cleanup
}

func helloFor(device string) *protocol.HelloMessage {
	return &protocol.HelloMessage{Name: "t", Width: 800, Height: 600, DPR: 1, PairCode: PairCode(), DeviceID: device}
}

func waitConnected(t *testing.T, h *stubHandler) {
	t.Helper()
	select {
	case <-h.connected:
	case <-time.After(5 * time.Second):
		t.Fatalf("OnClientConnect not called")
	}
}

func expectError(t *testing.T, c *websocket.Conn, code string) {
	t.Helper()
	env := readPlain(t, c)
	if env.Type != protocol.MsgError {
		t.Fatalf("type = %s, want error", env.Type)
	}
	em, err := protocol.DecodeData[protocol.ErrorMessage](env)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if em.Code != code {
		t.Fatalf("code = %q, want %q", em.Code, code)
	}
}

// TestSameDeviceReconnectEvictsStaleSession: after a silent link drop the
// server still holds the old session until its read deadline expires. A
// hello from the same deviceId must replace it — the phone's immediate
// retry used to be answered with "occupied" and gave up — while a hello
// from a different device is still refused.
func TestSameDeviceReconnectEvictsStaleSession(t *testing.T) {
	dial, h, cleanup := multiClientServer(t)
	defer cleanup()

	a := dial()
	sendPlain(t, a, protocol.MsgHello, helloFor("mob-same"))
	waitConnected(t, h)

	b := dial()
	sendPlain(t, b, protocol.MsgHello, helloFor("mob-same"))
	waitConnected(t, h)

	// The stale session's disconnect ran, and its socket was closed by
	// the server.
	select {
	case <-h.disconnected:
	case <-time.After(5 * time.Second):
		t.Fatalf("OnClientDisconnect not fired for the evicted session")
	}
	a.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := a.ReadMessage(); err == nil {
		t.Fatalf("stale session still readable after same-device reconnect")
	}

	c := dial()
	sendPlain(t, c, protocol.MsgHello, helloFor("mob-other"))
	expectError(t, c, "occupied")
}

// TestRejectedPeerDoesNotFireDisconnect: a device that is turned away never
// held the slot, so its teardown must not reach OnClientDisconnect — the
// desktop handler would stop capture and destroy the real client's display.
func TestRejectedPeerDoesNotFireDisconnect(t *testing.T) {
	dial, h, cleanup := multiClientServer(t)
	defer cleanup()

	a := dial()
	sendPlain(t, a, protocol.MsgHello, helloFor("mob-a"))
	waitConnected(t, h)

	c := dial()
	sendPlain(t, c, protocol.MsgHello, helloFor("mob-c"))
	expectError(t, c, "occupied")
	c.Close()

	select {
	case s := <-h.disconnected:
		t.Fatalf("OnClientDisconnect fired for a rejected peer (session %s)", s.ID)
	case <-time.After(300 * time.Millisecond):
	}

	// The real client's disconnect still fires when it goes away.
	a.Close()
	select {
	case <-h.disconnected:
	case <-time.After(5 * time.Second):
		t.Fatalf("OnClientDisconnect not fired for the real client")
	}
}

// TestWrongPairCodeDoesNotFireDisconnect: same guarantee for the other
// rejection path. Before the slot was claimed after admission, a failed
// pair attempt from a second phone ran the disconnect handler.
func TestWrongPairCodeDoesNotFireDisconnect(t *testing.T) {
	dial, h, cleanup := multiClientServer(t)
	defer cleanup()

	a := dial()
	sendPlain(t, a, protocol.MsgHello, helloFor("mob-a"))
	waitConnected(t, h)

	bad := helloFor("mob-bad")
	bad.PairCode = "000000"
	if bad.PairCode == PairCode() {
		bad.PairCode = "111111"
	}
	c := dial()
	sendPlain(t, c, protocol.MsgHello, bad)
	expectError(t, c, "pair_mismatch")

	select {
	case s := <-h.disconnected:
		t.Fatalf("OnClientDisconnect fired for a peer with a wrong pair code (session %s)", s.ID)
	case <-time.After(300 * time.Millisecond):
	}
}
