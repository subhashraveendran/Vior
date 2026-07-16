package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// pongWait is the deadline for reading the next pong message.
	pongWait = 40 * time.Second

	// pingInterval is how often pings are sent. Must be less than pongWait.
	pingInterval = 30 * time.Second

	// helloTimeout is how long the server waits for a hello message after upgrade.
	helloTimeout = 10 * time.Second

	// maxMessageSize is the maximum WebSocket message size in bytes.
	// Large enough for file chunks (48KB data base64-encoded = ~65KB + JSON overhead).
	maxMessageSize = 128 * 1024
)

// Session represents a connected client with an active WebSocket.
type Session struct {
	ID        string
	Conn      *websocket.Conn
	Hello     *HelloMessage
	CreatedAt time.Time
	mu        sync.Mutex
	closed    bool
	// disconnectOnce guards SessionHandler.OnClientDisconnect so it
	// fires exactly once per session even when both the read-loop
	// defer and an explicit Bye both try to invoke it.
	disconnectOnce sync.Once
	stopPing       chan struct{} // closed by Close() to stop pingLoop

	// Health metrics — updated on every message + pong. Used by the
	// 60s "session healthy" log line and by anyone (UI, debug
	// endpoint) who wants to read the connection's pulse without
	// adding more callbacks. healthMu guards the lot; reads are cheap
	// (RLock would be overkill for a 4-field struct).
	healthMu     sync.Mutex
	lastPongAt   time.Time
	lastReadAt   time.Time
	bytesIn      uint64
	bytesOut     uint64
	messagesIn   uint64
}

// Health snapshots the current liveness counters for a session.
// Cheap to call from any goroutine — copied under a brief mutex.
type Health struct {
	LastPongAgo  time.Duration
	LastReadAgo  time.Duration
	BytesIn      uint64
	BytesOut     uint64
	MessagesIn   uint64
}

// Snapshot returns the current health counters relative to now.
// Safe to call concurrently with the read loop.
func (s *Session) Snapshot() Health {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	now := time.Now()
	h := Health{
		BytesIn:    s.bytesIn,
		BytesOut:   s.bytesOut,
		MessagesIn: s.messagesIn,
	}
	if !s.lastPongAt.IsZero() {
		h.LastPongAgo = now.Sub(s.lastPongAt)
	}
	if !s.lastReadAt.IsZero() {
		h.LastReadAgo = now.Sub(s.lastReadAt)
	}
	return h
}

// FireDisconnect runs fn exactly once for the lifetime of this session.
// The stream package wraps the SessionHandler.OnClientDisconnect call
// with this so a Bye-then-defer race can't tear the virtual display
// down twice.
func (s *Session) FireDisconnect(fn func()) {
	s.disconnectOnce.Do(fn)
}

// NewSession creates a session from an upgraded WebSocket connection.
func NewSession(conn *websocket.Conn) *Session {
	now := time.Now()
	// Bound reads from the very first frame. WaitForHello reads the
	// hello before ReadLoop runs, so without this the pre-auth hello
	// would be unbounded — an unauthenticated LAN peer could push an
	// oversized message before ever proving the pair code. ReadLoop
	// re-asserts the same limit defensively.
	conn.SetReadLimit(maxMessageSize)
	return &Session{
		ID:         generateID(),
		Conn:       conn,
		CreatedAt:  now,
		lastPongAt: now,
		lastReadAt: now,
		stopPing:   make(chan struct{}),
	}
}

// Send sends a typed message to the client. Thread-safe.
func (s *Session) Send(msgType MessageType, data any) error {
	b, err := Encode(msgType, data)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("session closed")
	}
	s.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	writeErr := s.Conn.WriteMessage(websocket.TextMessage, b)
	s.mu.Unlock()
	if writeErr == nil {
		s.healthMu.Lock()
		s.bytesOut += uint64(len(b))
		s.healthMu.Unlock()
	}
	return writeErr
}

// ReadLoop reads messages from the client and dispatches them to the handler.
// It blocks until the connection closes or an error occurs.
func (s *Session) ReadLoop(handler MessageHandler) error {
	s.Conn.SetReadLimit(maxMessageSize)
	s.Conn.SetReadDeadline(time.Now().Add(pongWait))
	s.Conn.SetPongHandler(func(string) error {
		// Spec-level pong (gorilla driver). Refresh the read deadline
		// and snapshot the time so the health logger / mobile UI can
		// see freshness even when the only traffic is keepalive.
		s.Conn.SetReadDeadline(time.Now().Add(pongWait))
		s.healthMu.Lock()
		s.lastPongAt = time.Now()
		s.healthMu.Unlock()
		return nil
	})

	// Start ping ticker + 60s health logger.
	go s.pingLoop()
	stopHealth := make(chan struct{})
	go s.healthLogger(stopHealth)
	defer close(stopHealth)

	for {
		_, msg, err := s.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("protocol: ws read error [%s]: %v", s.ID, err)
			}
			return err
		}

		// Bump byte/message counters under the same brief lock used by
		// Snapshot/healthLogger. Refreshing lastReadAt here (in addition
		// to the spec-pong refresh above) means any in-band message
		// counts as a liveness proof — useful when Doze suspends the
		// mobile's ping timer but the user is actively typing.
		s.healthMu.Lock()
		s.bytesIn += uint64(len(msg))
		s.messagesIn++
		s.lastReadAt = time.Now()
		s.healthMu.Unlock()

		env, err := Decode(msg)
		if err != nil {
			log.Printf("protocol: ws decode error [%s]: %v", s.ID, err)
			continue
		}

		// App-level ping/pong is handled inline — never reaches the
		// SessionHandler. Browsers can't issue spec ping frames, so
		// mobile clients fake it with an in-band MsgPing every 15s and
		// expect a MsgPong reply. Treating it as a normal message also
		// updates lastReadAt above, which doubles as the freshness
		// signal for the health logger.
		if env.Type == MsgPing {
			s.healthMu.Lock()
			s.lastPongAt = time.Now()
			s.healthMu.Unlock()
			if err := s.Send(MsgPong, nil); err != nil {
				log.Printf("protocol: ws pong send failed [%s]: %v", s.ID, err)
			}
			continue
		}
		if env.Type == MsgPong {
			// Symmetric path — if the desktop ever drives pings (it
			// doesn't today; gorilla's spec ping is plenty), this is
			// where we'd record the round-trip.
			s.healthMu.Lock()
			s.lastPongAt = time.Now()
			s.healthMu.Unlock()
			continue
		}

		if err := s.dispatch(env, handler); err != nil {
			log.Printf("protocol: ws dispatch error [%s] type=%s: %v", s.ID, env.Type, err)
			s.Send(MsgError, &ErrorMessage{Code: "handler_error", Message: err.Error()})
		}
	}
}

// healthLogger emits one structured log line every 60s for the life of
// the session. Quiet by design — the user complaint we're diagnosing is
// "the connection drops without warning", so one line per minute is
// enough to spot silent gaps in the timestamp series. Stops when the
// caller closes stop (typically the ReadLoop's defer).
func (s *Session) healthLogger(stop <-chan struct{}) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			h := s.Snapshot()
			log.Printf("protocol: session %s healthy: msgs=%d in=%dB out=%dB lastPong=%dms lastRead=%dms",
				s.ID, h.MessagesIn, h.BytesIn, h.BytesOut,
				h.LastPongAgo.Milliseconds(), h.LastReadAgo.Milliseconds())
		}
	}
}

// WaitForHello blocks until the client sends a hello message or the timeout expires.
func (s *Session) WaitForHello() (*HelloMessage, error) {
	s.Conn.SetReadDeadline(time.Now().Add(helloTimeout))
	_, msg, err := s.Conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("waiting for hello: %w", err)
	}
	env, err := Decode(msg)
	if err != nil {
		return nil, fmt.Errorf("decode hello: %w", err)
	}
	if env.Type != MsgHello {
		return nil, fmt.Errorf("expected hello, got %s", env.Type)
	}
	hello, err := DecodeData[HelloMessage](env)
	if err != nil {
		return nil, fmt.Errorf("decode hello data: %w", err)
	}
	s.Hello = hello
	return hello, nil
}

// Close cleanly shuts down the session.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	// Stop the ping loop so it doesn't try to write to a closed conn.
	select {
	case <-s.stopPing:
	default:
		close(s.stopPing)
	}
	return s.Conn.Close()
}

func (s *Session) dispatch(env *Envelope, handler MessageHandler) error {
	switch env.Type {
	case MsgInput:
		msg, err := DecodeData[InputMessage](env)
		if err != nil {
			return err
		}
		return handler.OnInput(s, msg)
	case MsgResize:
		msg, err := DecodeData[ResizeMessage](env)
		if err != nil {
			return err
		}
		return handler.OnResize(s, msg)
	case MsgBye:
		return handler.OnBye(s)
	case MsgFileOffer:
		msg, err := DecodeData[FileOfferMessage](env)
		if err != nil {
			return err
		}
		return handler.OnFileOffer(s, msg)
	case MsgFileAccept:
		msg, err := DecodeData[FileAcceptMessage](env)
		if err != nil {
			return err
		}
		return handler.OnFileAccept(s, msg)
	case MsgFileReject:
		msg, err := DecodeData[FileRejectMessage](env)
		if err != nil {
			return err
		}
		return handler.OnFileReject(s, msg)
	case MsgFileChunk:
		msg, err := DecodeData[FileChunkMessage](env)
		if err != nil {
			return err
		}
		return handler.OnFileChunk(s, msg)
	case MsgFileComplete:
		msg, err := DecodeData[FileCompleteMessage](env)
		if err != nil {
			return err
		}
		return handler.OnFileComplete(s, msg)
	case MsgDownloadAccept:
		msg, err := DecodeData[DownloadAcceptMessage](env)
		if err != nil {
			return err
		}
		return handler.OnDownloadAccept(s, msg)
	case MsgDownloadReject:
		msg, err := DecodeData[DownloadRejectMessage](env)
		if err != nil {
			return err
		}
		return handler.OnDownloadReject(s, msg)
	case MsgDownloadComplete:
		msg, err := DecodeData[DownloadCompleteMessage](env)
		if err != nil {
			return err
		}
		return handler.OnDownloadComplete(s, msg)
	default:
		return fmt.Errorf("unexpected message type: %s", env.Type)
	}
}

func (s *Session) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopPing:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				return
			}
			s.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := s.Conn.WriteMessage(websocket.PingMessage, nil)
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure — fall back to time-based uniqueness.
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(b)
}
