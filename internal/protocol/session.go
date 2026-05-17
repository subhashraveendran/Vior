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
}

// NewSession creates a session from an upgraded WebSocket connection.
func NewSession(conn *websocket.Conn) *Session {
	return &Session{
		ID:        generateID(),
		Conn:      conn,
		CreatedAt: time.Now(),
	}
}

// Send sends a typed message to the client. Thread-safe.
func (s *Session) Send(msgType MessageType, data any) error {
	b, err := Encode(msgType, data)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session closed")
	}
	s.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return s.Conn.WriteMessage(websocket.TextMessage, b)
}

// ReadLoop reads messages from the client and dispatches them to the handler.
// It blocks until the connection closes or an error occurs.
func (s *Session) ReadLoop(handler MessageHandler) error {
	s.Conn.SetReadLimit(maxMessageSize)
	s.Conn.SetReadDeadline(time.Now().Add(pongWait))
	s.Conn.SetPongHandler(func(string) error {
		s.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Start ping ticker.
	go s.pingLoop()

	for {
		_, msg, err := s.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error [%s]: %v", s.ID, err)
			}
			return err
		}

		env, err := Decode(msg)
		if err != nil {
			log.Printf("ws decode error [%s]: %v", s.ID, err)
			continue
		}

		if err := s.dispatch(env, handler); err != nil {
			log.Printf("ws dispatch error [%s] type=%s: %v", s.ID, env.Type, err)
			s.Send(MsgError, &ErrorMessage{Code: "handler_error", Message: err.Error()})
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
	default:
		return fmt.Errorf("unexpected message type: %s", env.Type)
	}
}

func (s *Session) pingLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for range ticker.C {
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

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
