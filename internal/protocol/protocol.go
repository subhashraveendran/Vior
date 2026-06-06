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

	// Bidirectional HTTP-download path (desktop → mobile).
	// The desktop registers the file in fileMgr, pushes MsgIncomingFile
	// to the connected client, then waits for either MsgDownloadAccept
	// (which triggers an HTTP GET /download/{id} fetch) or
	// MsgDownloadReject. The mobile reports MsgDownloadComplete after
	// it finishes the GET so the desktop can clean up.
	MsgIncomingFile     MessageType = "incoming-file"
	MsgDownloadAccept   MessageType = "download-accept"
	MsgDownloadReject   MessageType = "download-reject"
	MsgDownloadComplete MessageType = "download-complete"

	// Application-level keepalive. Browsers can't issue WebSocket-spec
	// pings (the API doesn't expose ping frames), but Android Doze /
	// App-Standby will suspend timers AND silently kill TCP sockets in
	// the background — meaning the underlying gorilla ping frame never
	// reaches the client. App-level ping/pong lets the mobile drive the
	// liveness check itself: send MsgPing every 15s, expect MsgPong, if
	// none in 20s force-close + reconnect.
	MsgPing MessageType = "ping"
	MsgPong MessageType = "pong"
)

// Envelope is the outer JSON wrapper for all WebSocket messages.
type Envelope struct {
	Type MessageType     `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// HelloMessage is sent by the client after WebSocket connection.
// It reports the client device's screen dimensions.
type HelloMessage struct {
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	DPR      float64 `json:"dpr"`
	Name     string  `json:"name"`
	Mode     string  `json:"mode"` // "extend" or "mirror"
	PairCode string  `json:"pairCode,omitempty"`
	DeviceID string  `json:"deviceId,omitempty"`
	// Intent declares why the client connected: "display" (default —
	// virtual display + stream), "remote" (touchpad/keyboard only, no
	// virtual display, input maps to the main display), or "files"
	// (file transfer only, no capture, no input).
	Intent string `json:"intent,omitempty"`
	// SkipDisplay forces the server to skip virtual-display creation and
	// capture even when Mode says otherwise. Set by clients that only
	// need the WS channel (Remote-only / Files-only). Derived from
	// Intent server-side when this field is unset.
	SkipDisplay bool `json:"skipDisplay,omitempty"`
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

// IncomingFileMessage notifies the mobile client that the desktop has
// a file waiting for it at GET /download/{id}. The mobile responds
// with MsgDownloadAccept (and performs the HTTP fetch) or
// MsgDownloadReject. Trusted clients SHOULD auto-accept silently to
// match the upload-path UX.
type IncomingFileMessage struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime"`
	URL      string `json:"url,omitempty"`     // relative path "/download/{id}"
	Preview  string `json:"preview,omitempty"` // base64 thumbnail for images, empty otherwise
}

// DownloadAcceptMessage is sent by the mobile when it intends to GET
// /download/{id}. Lets the desktop log/track accepts.
type DownloadAcceptMessage struct {
	ID string `json:"id"`
}

// DownloadRejectMessage is sent by the mobile when the user declines
// or the platform can't write the file.
type DownloadRejectMessage struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

// DownloadCompleteMessage is sent by the mobile after the HTTP GET
// finishes so the desktop can mark the transfer done and free the
// pending entry in fileMgr.
type DownloadCompleteMessage struct {
	ID string `json:"id"`
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
