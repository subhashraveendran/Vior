package session

import "github.com/subhashraveendran/vior/internal/protocol"

// ResizeHello builds the HelloMessage that Configure should re-run for a
// resize: the new dimensions from msg, and everything else — intent,
// SkipDisplay, name, device identity — carried over from the hello that
// opened the session. That keeps a Remote-only session display-less across
// a rotation and a mirror session mirrored, instead of the old behaviour of
// re-running setup as a fresh extend-mode hello. msg.Mode, when set,
// switches the display mode in place.
func ResizeHello(prev *protocol.HelloMessage, msg *protocol.ResizeMessage) *protocol.HelloMessage {
	h := &protocol.HelloMessage{}
	if prev != nil {
		*h = *prev
	}
	h.Width, h.Height, h.DPR = msg.Width, msg.Height, msg.DPR
	if msg.Mode != "" {
		h.Mode = msg.Mode
	}
	if h.Mode == "" {
		h.Mode = "extend"
	}
	return h
}
