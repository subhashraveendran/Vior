// Package usb implements Android Open Accessory (AOA) protocol for direct
// USB communication between desktop and phone without ADB or developer mode.
//
// Protocol: binary frames over USB bulk endpoints.
// Desktop = USB host, Phone = USB accessory device.
package usb

import "encoding/binary"

// Frame types sent over USB.
const (
	FrameVideo  byte = 0x01 // JPEG frame: [type][4-byte len][jpeg data]
	FrameTouch  byte = 0x02 // Touch event: [type][action 1B][x 4B][y 4B]
	FrameHello  byte = 0x03 // Hello: [type][width 4B][height 4B][dpr 4B]
	FrameReady  byte = 0x04 // Ready: [type][width 4B][height 4B]
	FrameStatus byte = 0x05 // Status: [type][fps 4B][uptime 4B]
	FrameBye    byte = 0x06 // Disconnect: [type]
	FramePing   byte = 0x07 // Liveness probe: [type]
	FramePong   byte = 0x08 // Liveness reply: [type]
)

// MaxFrameSize bounds any single video / touch / control frame.
// Prevents a malformed length header from triggering a multi-GB
// allocation on either peer. Sane upper bound for a 4K JPEG ~ 1 MB.
const MaxFrameSize = 8 * 1024 * 1024

// ProtocolVersion is sent inside Hello so peers can negotiate.
// Bump when the wire format changes incompatibly.
const ProtocolVersion = 1

// Touch actions.
const (
	TouchDown byte = 0x01
	TouchMove byte = 0x02
	TouchUp   byte = 0x03
)

// AOA protocol constants.
const (
	// Android accessory vendor/product IDs after switching to accessory mode.
	AOAVendorID  = 0x18D1
	AOAProductID = 0x2D01 // accessory + ADB
	AOAProdNoADB = 0x2D00 // accessory only

	// AOA control requests.
	AOAGetProtocol   = 51
	AOASendString    = 52
	AOAStartAccessory = 53

	// AOA string indices.
	AOAStringManufacturer = 0
	AOAStringModel        = 1
	AOAStringDescription  = 2
	AOAStringVersion      = 3
	AOAStringURI          = 4
	AOAStringSerial       = 5
)

// AOA identification strings.
var AOAStrings = [6]string{
	"Vior",                                       // manufacturer
	"Vior Desktop",                               // model
	"Second display streaming over USB",          // description
	"1.0",                                        // version
	"https://github.com/subhashraveendran/Vior",  // URI
	"0001",                                       // serial
}

// EncodeVideoFrame creates a video frame packet.
// Format: [0x01][4-byte big-endian length][JPEG data]
func EncodeVideoFrame(jpeg []byte) []byte {
	buf := make([]byte, 5+len(jpeg))
	buf[0] = FrameVideo
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(jpeg)))
	copy(buf[5:], jpeg)
	return buf
}

// DecodeFrameHeader reads frame type and length from first 5 bytes.
// Length is clamped to MaxFrameSize so a corrupt or malicious header
// can't trigger an OOM via make([]byte, len).
func DecodeFrameHeader(header []byte) (frameType byte, length uint32) {
	if len(header) < 5 {
		return 0, 0
	}
	l := binary.BigEndian.Uint32(header[1:5])
	if l > MaxFrameSize {
		l = 0
	}
	return header[0], l
}

// EncodePing / EncodePong build single-byte liveness frames.
func EncodePing() []byte { return []byte{FramePing} }
func EncodePong() []byte { return []byte{FramePong} }

// EncodeTouchEvent creates a touch frame.
// Format: [0x02][action 1B][x float32 4B][y float32 4B]
func EncodeTouchEvent(action byte, x, y float32) []byte {
	buf := make([]byte, 10)
	buf[0] = FrameTouch
	buf[1] = action
	binary.BigEndian.PutUint32(buf[2:6], uint32(x))
	binary.BigEndian.PutUint32(buf[6:10], uint32(y))
	return buf
}

// DecodeTouchEvent parses touch data.
func DecodeTouchEvent(data []byte) (action byte, x, y float32) {
	if len(data) < 9 {
		return 0, 0, 0
	}
	return data[0], float32(binary.BigEndian.Uint32(data[1:5])), float32(binary.BigEndian.Uint32(data[5:9]))
}

// EncodeHello creates hello frame with screen dimensions.
func EncodeHello(width, height int, dpr float32) []byte {
	buf := make([]byte, 13)
	buf[0] = FrameHello
	binary.BigEndian.PutUint32(buf[1:5], uint32(width))
	binary.BigEndian.PutUint32(buf[5:9], uint32(height))
	binary.BigEndian.PutUint32(buf[9:13], uint32(dpr*100))
	return buf
}

// DecodeHello parses hello data.
func DecodeHello(data []byte) (width, height int, dpr float32) {
	if len(data) < 12 {
		return 0, 0, 0
	}
	return int(binary.BigEndian.Uint32(data[0:4])),
		int(binary.BigEndian.Uint32(data[4:8])),
		float32(binary.BigEndian.Uint32(data[8:12])) / 100
}

// EncodeReady creates ready frame.
func EncodeReady(width, height int) []byte {
	buf := make([]byte, 9)
	buf[0] = FrameReady
	binary.BigEndian.PutUint32(buf[1:5], uint32(width))
	binary.BigEndian.PutUint32(buf[5:9], uint32(height))
	return buf
}
