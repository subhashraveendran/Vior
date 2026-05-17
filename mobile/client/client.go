// Package client provides a WebSocket client for connecting to the Vior server.
// It reuses the internal/protocol package for message types.
package client

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/subhashraveendran/vior/internal/protocol"
)

// State represents the client connection state.
type State int

const (
	Disconnected State = iota
	Connecting
	Connected
	Streaming
)

// ViorClient connects to a Vior server via WebSocket.
type ViorClient struct {
	host    string
	port    int
	conn    *websocket.Conn
	state   State
	stateMu sync.RWMutex

	// Screen dimensions to report.
	ScreenWidth  int
	ScreenHeight int
	DPR          float64
	DeviceName   string

	// Callbacks.
	OnReady      func(streamURL string, resolution string)
	OnStatus     func(fps, clients, uptime int)
	OnError      func(code, message string)
	OnDisconnect func()

	// Stream info from server.
	StreamURL  string
	Resolution string
	SessionID  string

	mu   sync.Mutex
	stop chan struct{}
}

// New creates a new ViorClient.
func New(host string, port int) *ViorClient {
	return &ViorClient{
		host: host,
		port: port,
		stop: make(chan struct{}),
	}
}

// Connect establishes a WebSocket connection and performs the handshake.
func (c *ViorClient) Connect() error {
	c.setState(Connecting)

	url := fmt.Sprintf("ws://%s:%d/ws", c.host, c.port)
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		c.setState(Disconnected)
		return fmt.Errorf("dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.stop = make(chan struct{})
	c.mu.Unlock()

	c.setState(Connected)

	// Send hello with screen dimensions.
	hello := protocol.HelloMessage{
		Width:  c.ScreenWidth,
		Height: c.ScreenHeight,
		DPR:    c.DPR,
		Name:   c.DeviceName,
	}
	helloBytes, err := protocol.Encode(protocol.MsgHello, &hello)
	if err != nil {
		c.Disconnect()
		return fmt.Errorf("encode hello: %w", err)
	}
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, helloBytes); err != nil {
		c.Disconnect()
		return fmt.Errorf("send hello: %w", err)
	}

	// Start read loop.
	go c.readLoop()
	return nil
}

// Disconnect closes the connection.
func (c *ViorClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.stop:
	default:
		close(c.stop)
	}

	if c.conn != nil {
		// Try to send bye.
		bye, _ := protocol.Encode(protocol.MsgBye, nil)
		c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		c.conn.WriteMessage(websocket.TextMessage, bye)
		c.conn.Close()
		c.conn = nil
	}

	c.setState(Disconnected)
	if c.OnDisconnect != nil {
		c.OnDisconnect()
	}
}

// SendInput sends a touch/mouse/key input event to the server.
func (c *ViorClient) SendInput(event, action string, x, y float64) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := protocol.InputMessage{
		Event:  event,
		Action: action,
		X:      x,
		Y:      y,
	}
	b, err := protocol.Encode(protocol.MsgInput, &msg)
	if err != nil {
		return err
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, b)
}

// SendScroll sends a scroll event.
func (c *ViorClient) SendScroll(dx, dy float64) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := protocol.InputMessage{
		Event:  "scroll",
		Action: "scroll",
		DX:     dx,
		DY:     dy,
	}
	b, err := protocol.Encode(protocol.MsgInput, &msg)
	if err != nil {
		return err
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, b)
}

// State returns the current connection state.
func (c *ViorClient) GetState() State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// FullStreamURL returns the absolute stream URL.
func (c *ViorClient) FullStreamURL() string {
	return fmt.Sprintf("http://%s:%d%s", c.host, c.port, c.StreamURL)
}

func (c *ViorClient) setState(s State) {
	c.stateMu.Lock()
	c.state = s
	c.stateMu.Unlock()
}

func (c *ViorClient) readLoop() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
		c.setState(Disconnected)
		if c.OnDisconnect != nil {
			c.OnDisconnect()
		}
	}()

	for {
		select {
		case <-c.stop:
			return
		default:
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			return
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			return
		}

		env, err := protocol.Decode(msg)
		if err != nil {
			continue
		}

		switch env.Type {
		case protocol.MsgReady:
			ready, err := protocol.DecodeData[protocol.ReadyMessage](env)
			if err != nil {
				continue
			}
			c.StreamURL = ready.StreamURL
			c.Resolution = ready.Resolution
			c.SessionID = ready.SessionID
			c.setState(Streaming)
			if c.OnReady != nil {
				c.OnReady(ready.StreamURL, ready.Resolution)
			}

		case protocol.MsgStatus:
			status, err := protocol.DecodeData[protocol.StatusMessage](env)
			if err != nil {
				continue
			}
			if c.OnStatus != nil {
				c.OnStatus(status.FPS, status.Clients, status.Uptime)
			}

		case protocol.MsgError:
			errMsg, err := protocol.DecodeData[protocol.ErrorMessage](env)
			if err != nil {
				continue
			}
			if c.OnError != nil {
				c.OnError(errMsg.Code, errMsg.Message)
			}
		}
	}
}
