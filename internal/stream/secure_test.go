package stream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/subhashraveendran/vior/internal/handshake"
	"github.com/subhashraveendran/vior/internal/protocol"
	"github.com/subhashraveendran/vior/internal/securechan"
)

// stubHandler satisfies SessionHandler so handleWebSocket can run without a
// virtual display or capture pipeline behind it.
type stubHandler struct {
	connected chan *protocol.HelloMessage
}

func newStubHandler() *stubHandler {
	return &stubHandler{connected: make(chan *protocol.HelloMessage, 1)}
}

func (h *stubHandler) OnClientConnect(_ *protocol.Session, hello *protocol.HelloMessage) error {
	select {
	case h.connected <- hello:
	default:
	}
	return nil
}
func (h *stubHandler) OnClientInput(*protocol.Session, *protocol.InputMessage) error   { return nil }
func (h *stubHandler) OnClientResize(*protocol.Session, *protocol.ResizeMessage) error { return nil }
func (h *stubHandler) OnClientDisconnect(*protocol.Session)                            {}
func (h *stubHandler) OnClientFileOffer(*protocol.Session, *protocol.FileOfferMessage) error {
	return nil
}
func (h *stubHandler) OnClientFileAccept(*protocol.Session, *protocol.FileAcceptMessage) error {
	return nil
}
func (h *stubHandler) OnClientFileReject(*protocol.Session, *protocol.FileRejectMessage) error {
	return nil
}
func (h *stubHandler) OnClientFileChunk(*protocol.Session, *protocol.FileChunkMessage) error {
	return nil
}
func (h *stubHandler) OnClientFileComplete(*protocol.Session, *protocol.FileCompleteMessage) error {
	return nil
}
func (h *stubHandler) OnClientDownloadAccept(*protocol.Session, *protocol.DownloadAcceptMessage) error {
	return nil
}
func (h *stubHandler) OnClientDownloadReject(*protocol.Session, *protocol.DownloadRejectMessage) error {
	return nil
}
func (h *stubHandler) OnClientDownloadComplete(*protocol.Session, *protocol.DownloadCompleteMessage) error {
	return nil
}
func (h *stubHandler) ServeDownload(http.ResponseWriter, *http.Request, string) {}

// wsTestServer spins up the real handleWebSocket behind httptest and returns a
// dialled client connection.
func wsTestServer(t *testing.T, mode SecurityMode) (*websocket.Conn, *stubHandler, func()) {
	t.Helper()

	prev := GetSecurityMode()
	SetSecurityMode(mode)

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

	return conn, h, func() {
		conn.Close()
		ts.Close()
		SetSecurityMode(prev)
	}
}

// sendPlain writes an unsealed envelope.
func sendPlain(t *testing.T, c *websocket.Conn, msgType protocol.MessageType, data any) {
	t.Helper()
	b, err := protocol.Encode(msgType, data)
	if err != nil {
		t.Fatalf("encode %s: %v", msgType, err)
	}
	if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("write %s: %v", msgType, err)
	}
}

// readPlain reads an unsealed envelope.
func readPlain(t *testing.T, c *websocket.Conn) *protocol.Envelope {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	env, err := protocol.Decode(msg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env
}

// sendSealed writes an envelope through the record layer.
func sendSealed(t *testing.T, c *websocket.Conn, ch *securechan.Channel, msgType protocol.MessageType, data any) {
	t.Helper()
	b, err := protocol.Encode(msgType, data)
	if err != nil {
		t.Fatalf("encode %s: %v", msgType, err)
	}
	sealed, err := ch.Seal(b)
	if err != nil {
		t.Fatalf("seal %s: %v", msgType, err)
	}
	if err := c.WriteMessage(websocket.BinaryMessage, sealed); err != nil {
		t.Fatalf("write sealed %s: %v", msgType, err)
	}
}

// readSealed reads and opens one sealed envelope.
func readSealed(t *testing.T, c *websocket.Conn, ch *securechan.Channel) *protocol.Envelope {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	frameType, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read sealed: %v", err)
	}
	if frameType != websocket.BinaryMessage {
		t.Fatalf("expected a binary frame on a secure channel, got type %d: %s", frameType, msg)
	}
	plain, err := ch.Open(msg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	env, err := protocol.Decode(plain)
	if err != nil {
		t.Fatalf("decode sealed: %v", err)
	}
	return env
}

// clientHandshake drives the initiator side against a live server and returns
// the established channel plus the frame token.
func clientHandshake(t *testing.T, c *websocket.Conn, secret []byte) (*securechan.Channel, string) {
	t.Helper()

	initiator, err := handshake.NewInitiator(secret)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	init, err := initiator.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	sendPlain(t, c, protocol.MsgSecureInit, &protocol.SecureInitMessage{
		Version: init.Version, PubKey: init.PubKey, Nonce: init.Nonce,
	})

	env := readPlain(t, c)
	if env.Type != protocol.MsgSecureResp {
		t.Fatalf("expected %s, got %s", protocol.MsgSecureResp, env.Type)
	}
	respMsg, err := protocol.DecodeData[protocol.SecureRespMessage](env)
	if err != nil {
		t.Fatalf("decode resp: %v", err)
	}

	conf, err := initiator.Finish(&handshake.Response{
		PubKey: respMsg.PubKey, Nonce: respMsg.Nonce, MAC: respMsg.MAC,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	sendPlain(t, c, protocol.MsgSecureConfirm, &protocol.SecureConfirmMessage{MAC: conf.MAC})

	key, err := initiator.SessionKey()
	if err != nil {
		t.Fatalf("SessionKey: %v", err)
	}
	ch, err := securechan.NewChannel(key, true)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}

	ready := readSealed(t, c, ch)
	if ready.Type != protocol.MsgSecureReady {
		t.Fatalf("expected %s, got %s", protocol.MsgSecureReady, ready.Type)
	}
	readyMsg, err := protocol.DecodeData[protocol.SecureReadyMessage](ready)
	if err != nil {
		t.Fatalf("decode ready: %v", err)
	}
	return ch, readyMsg.FrameToken
}

// The whole point of the design: the pair code must be sealed, not sent in
// front of the handshake.
func TestSecureHandshakeThenSealedHello(t *testing.T) {
	conn, h, cleanup := wsTestServer(t, SecurePreferred)
	defer cleanup()

	ch, token := clientHandshake(t, conn, ChannelSecret())
	if token == "" {
		t.Fatal("server issued an empty frame token")
	}

	sendSealed(t, conn, ch, protocol.MsgHello, &protocol.HelloMessage{
		Width: 1170, Height: 2532, DPR: 3, Name: "test-phone",
		PairCode: PairCode(), Intent: "files", SkipDisplay: true,
	})

	select {
	case hello := <-h.connected:
		if hello.Name != "test-phone" {
			t.Fatalf("hello.Name = %q, want test-phone", hello.Name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never admitted the sealed hello")
	}
}

// A wrong channel secret must fail the handshake with a generic error — never
// a hint about how close the guess was.
func TestSecureHandshakeWrongSecretRejected(t *testing.T) {
	conn, _, cleanup := wsTestServer(t, SecurePreferred)
	defer cleanup()

	wrong := make([]byte, handshake.SecretSize)
	for i := range wrong {
		wrong[i] = 0xAB
	}

	initiator, err := handshake.NewInitiator(wrong)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	init, _ := initiator.Init()
	sendPlain(t, conn, protocol.MsgSecureInit, &protocol.SecureInitMessage{
		Version: init.Version, PubKey: init.PubKey, Nonce: init.Nonce,
	})

	// The server still answers — it cannot know the client is wrong until
	// the confirm — but the client's own verification must fail.
	env := readPlain(t, conn)
	if env.Type != protocol.MsgSecureResp {
		t.Fatalf("expected %s, got %s", protocol.MsgSecureResp, env.Type)
	}
	respMsg, _ := protocol.DecodeData[protocol.SecureRespMessage](env)
	if _, err := initiator.Finish(&handshake.Response{
		PubKey: respMsg.PubKey, Nonce: respMsg.Nonce, MAC: respMsg.MAC,
	}); err == nil {
		t.Fatal("client accepted a server MAC derived from a different secret")
	}

	// And a forged confirm must be refused by the server.
	sendPlain(t, conn, protocol.MsgSecureConfirm, &protocol.SecureConfirmMessage{
		MAC: make([]byte, handshake.MACSize),
	})
	errEnv := readPlain(t, conn)
	if errEnv.Type != protocol.MsgError {
		t.Fatalf("expected %s, got %s", protocol.MsgError, errEnv.Type)
	}
	errMsg, err := protocol.DecodeData[protocol.ErrorMessage](errEnv)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errMsg.Code != protocol.ErrCodeSecureFailed {
		t.Fatalf("error code = %q, want %q", errMsg.Code, protocol.ErrCodeSecureFailed)
	}
	assertNoFurtherError(t, conn)
}

// Under SecureRequired a legacy client must get an actionable code, not a
// bare close it cannot explain to the user.
func TestSecureRequiredRejectsCleartextHello(t *testing.T) {
	conn, _, cleanup := wsTestServer(t, SecureRequired)
	defer cleanup()

	sendPlain(t, conn, protocol.MsgHello, &protocol.HelloMessage{
		Width: 100, Height: 100, DPR: 1, Name: "legacy", PairCode: PairCode(),
	})

	env := readPlain(t, conn)
	if env.Type != protocol.MsgError {
		t.Fatalf("expected %s, got %s", protocol.MsgError, env.Type)
	}
	errMsg, err := protocol.DecodeData[protocol.ErrorMessage](env)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errMsg.Code != protocol.ErrCodeUpgradeRequired {
		t.Fatalf("error code = %q, want %q", errMsg.Code, protocol.ErrCodeUpgradeRequired)
	}

	// Exactly one error, then close. A second, vaguer error on top would
	// leave the client acting on the less useful of the two — showing
	// "connection setup failed" instead of "update your app".
	assertNoFurtherError(t, conn)
}

// assertNoFurtherError requires that the connection carries no additional
// error message before it closes.
func assertNoFurtherError(t *testing.T, c *websocket.Conn) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			return // closed without another message, as required
		}
		env, decodeErr := protocol.Decode(msg)
		if decodeErr != nil {
			continue
		}
		if env.Type == protocol.MsgError {
			second, _ := protocol.DecodeData[protocol.ErrorMessage](env)
			t.Fatalf("server sent a second error after an actionable one: code=%q", second.Code)
		}
	}
}

// Under SecurePreferred an old client still works — that is the whole point of
// the rollout mode.
func TestSecurePreferredAdmitsCleartextHello(t *testing.T) {
	conn, h, cleanup := wsTestServer(t, SecurePreferred)
	defer cleanup()

	sendPlain(t, conn, protocol.MsgHello, &protocol.HelloMessage{
		Width: 100, Height: 100, DPR: 1, Name: "legacy",
		PairCode: PairCode(), Intent: "files", SkipDisplay: true,
	})

	select {
	case hello := <-h.connected:
		if hello.Name != "legacy" {
			t.Fatalf("hello.Name = %q, want legacy", hello.Name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleartext client was not admitted under SecurePreferred")
	}
}

// Once the channel is secure, an unsealed frame is an injection attempt and
// must kill the session rather than being processed.
func TestPlaintextAfterHandshakeIsFatal(t *testing.T) {
	conn, _, cleanup := wsTestServer(t, SecurePreferred)
	defer cleanup()

	ch, _ := clientHandshake(t, conn, ChannelSecret())
	sendSealed(t, conn, ch, protocol.MsgHello, &protocol.HelloMessage{
		Width: 100, Height: 100, DPR: 1, Name: "p",
		PairCode: PairCode(), Intent: "files", SkipDisplay: true,
	})

	// Now inject cleartext.
	sendPlain(t, conn, protocol.MsgInput, &protocol.InputMessage{Event: "key", Action: "down", Key: "a"})

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return // connection dropped, as required
		}
	}
}

// A replayed sealed frame must be rejected by the record layer's counter
// check and take the session down with it.
func TestReplayedSealedFrameIsFatal(t *testing.T) {
	conn, _, cleanup := wsTestServer(t, SecurePreferred)
	defer cleanup()

	ch, _ := clientHandshake(t, conn, ChannelSecret())

	hello, err := protocol.Encode(protocol.MsgHello, &protocol.HelloMessage{
		Width: 100, Height: 100, DPR: 1, Name: "r",
		PairCode: PairCode(), Intent: "files", SkipDisplay: true,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	sealed, err := ch.Seal(hello)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, sealed); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Replay the identical frame.
	if err := conn.WriteMessage(websocket.BinaryMessage, sealed); err != nil {
		t.Fatalf("replay write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return // dropped, as required
		}
	}
}

// frameClientAuthorized is the gate on the raw-screen endpoints. Loopback
// short-circuits it, so this exercises it directly with synthetic remote
// addresses rather than through httptest.
func TestFrameClientAuthorized(t *testing.T) {
	const clientIP = "192.168.1.50"
	const token = "tok_abc123"

	req := func(ip, query string) *http.Request {
		return &http.Request{
			RemoteAddr: ip + ":54321",
			URL:        &url.URL{Path: "/stream", RawQuery: query},
		}
	}

	t.Run("secure session requires a matching token", func(t *testing.T) {
		s := &MJPEGServer{frameClientIP: clientIP, frameToken: token}
		if !s.frameClientAuthorized(req(clientIP, "t="+token)) {
			t.Error("correct token rejected")
		}
		if s.frameClientAuthorized(req(clientIP, "t=wrong")) {
			t.Error("wrong token accepted")
		}
		if s.frameClientAuthorized(req(clientIP, "")) {
			t.Error("missing token accepted — IP alone must not be enough for a secure session")
		}
	})

	t.Run("token does not override the IP check", func(t *testing.T) {
		s := &MJPEGServer{frameClientIP: clientIP, frameToken: token}
		if s.frameClientAuthorized(req("192.168.1.99", "t="+token)) {
			t.Error("a different IP was admitted with a valid token")
		}
	})

	t.Run("cleartext session falls back to IP only", func(t *testing.T) {
		s := &MJPEGServer{frameClientIP: clientIP}
		if !s.frameClientAuthorized(req(clientIP, "")) {
			t.Error("legacy client rejected when no token was issued")
		}
		if s.frameClientAuthorized(req("10.0.0.1", "")) {
			t.Error("unpaired IP admitted")
		}
	})

	t.Run("nobody paired means frames are closed", func(t *testing.T) {
		s := &MJPEGServer{}
		if s.frameClientAuthorized(req(clientIP, "t="+token)) {
			t.Error("frames served with no paired client")
		}
	})

	t.Run("loopback is always allowed for the desktop preview", func(t *testing.T) {
		s := &MJPEGServer{frameClientIP: clientIP, frameToken: token}
		if !s.frameClientAuthorized(req("127.0.0.1", "")) {
			t.Error("loopback preview blocked")
		}
	})
}

// The secret must round-trip through its stored encoding, and must never be
// accepted below the length that makes the handshake sound.
func TestChannelSecretEncoding(t *testing.T) {
	if got := len(ChannelSecret()); got < handshake.MinSecretSize {
		t.Fatalf("active secret is %d bytes, want >= %d", got, handshake.MinSecretSize)
	}

	param := ChannelSecretParam()
	decoded, ok := decodeSecret(param)
	if !ok {
		t.Fatal("ChannelSecretParam output failed to decode")
	}
	if string(decoded) != string(ChannelSecret()) {
		t.Fatal("secret did not survive the encode/decode round trip")
	}
	for _, bad := range []string{"", "short", "!!!not-base64!!!"} {
		if _, ok := decodeSecret(bad); ok {
			t.Errorf("decodeSecret(%q) accepted an unusable secret", bad)
		}
	}
}

// /info is reachable by any LAN peer without authentication. It advertises the
// security capability so a client knows whether to handshake — but publishing
// the secret there would hand the encrypted channel to exactly the attacker it
// defends against.
func TestInfoAdvertisesCapabilityButNeverTheSecret(t *testing.T) {
	srv := &MJPEGServer{clients: map[chan []byte]struct{}{}}
	ts := httptest.NewServer(http.HandlerFunc(srv.handleInfo))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET /info: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /info: %v", err)
	}

	// Capability must be advertised so a client can decide to handshake.
	for _, key := range []string{"secure", "secureMode", "secureRequired", "secureVersion"} {
		if _, ok := body[key]; !ok {
			t.Errorf("/info is missing %q", key)
		}
	}
	if got := body["secureVersion"]; got != float64(handshake.Version) {
		t.Errorf("secureVersion = %v, want %d", got, handshake.Version)
	}

	// And the secret must appear nowhere in the payload, in any encoding.
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	secret := ChannelSecretParam()
	if secret == "" {
		t.Fatal("no active secret to check against")
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("/info published the channel secret")
	}
	if strings.Contains(string(raw), string(ChannelSecret())) {
		t.Fatal("/info published the raw channel secret bytes")
	}
	// The pair code is also an admission secret and must stay unpublished.
	if strings.Contains(string(raw), PairCode()) {
		t.Fatal("/info published the pair code")
	}
}

func TestSecurityModeString(t *testing.T) {
	cases := map[SecurityMode]string{
		SecurePreferred: "preferred",
		SecureRequired:  "required",
		SecureOff:       "off",
	}
	for mode, want := range cases {
		if got := mode.String(); got != want {
			t.Errorf("SecurityMode(%d).String() = %q, want %q", mode, got, want)
		}
	}
}
