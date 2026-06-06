package stream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/subhashraveendran/vior/internal/protocol"
)

// captureHandler records the input messages it receives so a test can
// verify the upgrade → hello → input dispatch chain is intact end-to-end.
type captureHandler struct {
	mu     sync.Mutex
	inputs []protocol.InputMessage
	hello  chan *protocol.HelloMessage
	calls  int32
}

func (h *captureHandler) OnClientConnect(s *protocol.Session, hello *protocol.HelloMessage) error {
	select {
	case h.hello <- hello:
	default:
	}
	return nil
}
func (h *captureHandler) OnClientInput(s *protocol.Session, msg *protocol.InputMessage) error {
	atomic.AddInt32(&h.calls, 1)
	h.mu.Lock()
	h.inputs = append(h.inputs, *msg)
	h.mu.Unlock()
	return nil
}
func (h *captureHandler) OnClientResize(*protocol.Session, *protocol.ResizeMessage) error {
	return nil
}
func (h *captureHandler) OnClientDisconnect(*protocol.Session)                                {}
func (h *captureHandler) OnClientFileOffer(*protocol.Session, *protocol.FileOfferMessage) error {
	return nil
}
func (h *captureHandler) OnClientFileAccept(*protocol.Session, *protocol.FileAcceptMessage) error {
	return nil
}
func (h *captureHandler) OnClientFileReject(*protocol.Session, *protocol.FileRejectMessage) error {
	return nil
}
func (h *captureHandler) OnClientFileChunk(*protocol.Session, *protocol.FileChunkMessage) error {
	return nil
}
func (h *captureHandler) OnClientFileComplete(*protocol.Session, *protocol.FileCompleteMessage) error {
	return nil
}
func (h *captureHandler) OnClientDownloadAccept(*protocol.Session, *protocol.DownloadAcceptMessage) error {
	return nil
}
func (h *captureHandler) OnClientDownloadReject(*protocol.Session, *protocol.DownloadRejectMessage) error {
	return nil
}
func (h *captureHandler) OnClientDownloadComplete(*protocol.Session, *protocol.DownloadCompleteMessage) error {
	return nil
}
func (h *captureHandler) ServeDownload(w http.ResponseWriter, _ *http.Request, _ string) {
	http.NotFound(w, nil)
}

// TestRemoteTabInputReachesHandler simulates the mobile Remote-tab path:
// open WS, send hello with valid pair code, then a stream of mouse/key
// messages — asserts every one shows up at OnClientInput.
func TestRemoteTabInputReachesHandler(t *testing.T) {
	h := &captureHandler{hello: make(chan *protocol.HelloMessage, 1)}
	srv := NewMJPEGServer("127.0.0.1", 0, nil, h)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.handleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// Hello — use the active pair code so the trust check passes.
	helloJSON := `{"type":"hello","data":{"width":390,"height":844,"dpr":3,"name":"test","mode":"extend","pairCode":"` + pairCode + `","deviceId":"itest"}}`
	if err := c.WriteMessage(websocket.TextMessage, []byte(helloJSON)); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	// Wait for OnClientConnect.
	select {
	case <-h.hello:
	case <-time.After(2 * time.Second):
		t.Fatalf("OnClientConnect not called")
	}

	// Send the exact shapes that mobile-cap/src/js/screens/remote.ts produces.
	payloads := []string{
		`{"type":"input","data":{"event":"mouse","action":"move","dx":4.2,"dy":-1.5}}`,
		`{"type":"input","data":{"event":"mouse","action":"click"}}`,
		`{"type":"input","data":{"event":"mouse","action":"rightclick"}}`,
		`{"type":"input","data":{"event":"scroll","dx":0,"dy":3}}`,
		`{"type":"input","data":{"event":"key","key":"Cmd+c"}}`,
	}
	for _, p := range payloads {
		if err := c.WriteMessage(websocket.TextMessage, []byte(p)); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	// Allow the read loop to dispatch.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&h.calls) >= int32(len(payloads)) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.inputs) != len(payloads) {
		t.Fatalf("got %d inputs want %d: %#v", len(h.inputs), len(payloads), h.inputs)
	}
	if h.inputs[0].Event != "mouse" || h.inputs[0].Action != "move" || h.inputs[0].DX != 4.2 {
		t.Fatalf("mouse move dropped fields: %#v", h.inputs[0])
	}
	if h.inputs[1].Action != "click" {
		t.Fatalf("click misparsed: %#v", h.inputs[1])
	}
	if h.inputs[3].DY != 3 {
		t.Fatalf("scroll dy lost: %#v", h.inputs[3])
	}
	if h.inputs[4].Key != "Cmd+c" {
		t.Fatalf("key chord lost: %#v", h.inputs[4])
	}
}
