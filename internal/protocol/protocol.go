// Package protocol defines the WebSocket message types for Vior client-server communication.
package protocol

import "encoding/json"

// MessageType identifies the kind of WebSocket message.
type MessageType string

const (
	MsgHello        MessageType = "hello"
	MsgReady        MessageType = "ready"
	MsgInput        MessageType = "input"
	MsgStatus       MessageType = "status"
	MsgError        MessageType = "error"
	MsgBye          MessageType = "bye"
	MsgResize       MessageType = "resize"
	MsgFileOffer    MessageType = "file-offer"
	MsgFileAccept   MessageType = "file-accept"
	MsgFileReject   MessageType = "file-reject"
	MsgFileChunk    MessageType = "file-chunk"
	MsgFileComplete MessageType = "file-complete"
)

// Envelope is the outer JSON wrapper for all WebSocket messages.
type Envelope struct {
	Type MessageType     `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// HelloMessage is sent by the client after WebSocket connection.
// It reports the client device's screen dimensions.
type HelloMessage struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	DPR    float64 `json:"dpr"`
	Name   string  `json:"name"`
}

// ReadyMessage is sent by the server after virtual display creation succeeds.
type ReadyMessage struct {
	StreamURL  string `json:"streamUrl"`
	Resolution string `json:"resolution"`
	SessionID  string `json:"sessionId"`
}

// InputMessage is sent by the client for touch, mouse, keyboard, and scroll events.
type InputMessage struct {
	Event  string  `json:"event"`  // "touch", "mouse", "key", "scroll"
	Action string  `json:"action"` // "down", "move", "up", "click"
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Key    string  `json:"key,omitempty"`
	DX     float64 `json:"dx,omitempty"`
	DY     float64 `json:"dy,omitempty"`
}

// StatusMessage is sent by the server periodically with session stats.
type StatusMessage struct {
	FPS     int `json:"fps"`
	Clients int `json:"clients"`
	Uptime  int `json:"uptime"` // seconds
}

// ErrorMessage is sent by the server when an operation fails.
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ResizeMessage is sent by the client when screen orientation changes.
type ResizeMessage struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	DPR    float64 `json:"dpr"`
}

// ── File Transfer Messages ──────────────────────────────────────────

// FileOfferMessage is sent by either side to propose a file transfer.
type FileOfferMessage struct {
	ID       string `json:"id"`       // unique transfer ID
	Name     string `json:"name"`     // file name
	Size     int64  `json:"size"`     // total bytes
	MimeType string `json:"mimeType"` // e.g. "image/jpeg", "application/pdf"
	Preview  string `json:"preview"`  // base64 thumbnail for images/videos, empty otherwise
}

// FileAcceptMessage acknowledges a file offer.
type FileAcceptMessage struct {
	ID string `json:"id"`
}

// FileRejectMessage declines a file offer.
type FileRejectMessage struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

// FileChunkMessage carries a piece of file data (base64-encoded).
type FileChunkMessage struct {
	ID     string `json:"id"`
	Offset int64  `json:"offset"` // byte offset in file
	Data   string `json:"data"`   // base64-encoded chunk
}

// FileCompleteMessage signals the transfer is done.
type FileCompleteMessage struct {
	ID   string `json:"id"`
	Hash string `json:"hash,omitempty"` // SHA-256 of complete file for integrity
}

// Encode wraps a typed payload into an Envelope.
func Encode(msgType MessageType, data any) ([]byte, error) {
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(Envelope{Type: msgType, Data: raw})
}

// Decode parses an Envelope from raw JSON bytes.
func Decode(b []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// DecodeData unmarshals the Data field of an Envelope into a concrete type.
func DecodeData[T any](env *Envelope) (*T, error) {
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
