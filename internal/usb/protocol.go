// Package usb implements Android Open Accessory (AOA) protocol for direct
// USB communication between desktop and phone without ADB or developer mode.
//
// Protocol: binary frames over USB bulk endpoints.
// Desktop = USB host, Phone = USB accessory device.
package usb

import (
	"encoding/binary"
	"math"
)

// Frame types sent over USB.
const (
	FrameVideo    byte = 0x01 // JPEG frame: [type][4-byte len][jpeg data]
	FrameTouch    byte = 0x02 // Touch event: [type][action 1B][x 4B][y 4B]
	FrameHello    byte = 0x03 // Hello: [type][magic 4B][ver 1B][w 4B][h 4B][dpr 4B]
	FrameReady    byte = 0x04 // Ready: [type][width 4B][height 4B]
	FrameHelloAck byte = 0x05 // HelloAck: [type][magic 4B][ver 1B]
	FrameBye      byte = 0x06 // Disconnect: [type]
	FramePing     byte = 0x07 // Liveness probe: [type]
	FramePong     byte = 0x08 // Liveness reply: [type]
)

// MaxFrameSize bounds any single video / touch / control frame.
// Prevents a malformed length header from triggering a multi-GB
// allocation on either peer. Sane upper bound for a 4K JPEG ~ 1 MB.
const MaxFrameSize = 8 * 1024 * 1024

// ProtocolVersion is sent inside Hello so peers can negotiate.
// Bump when the wire format changes incompatibly.
const ProtocolVersion = 1

// HelloMagic is the 4-byte tag prepended to Hello / HelloAck payloads
// so each side can verify the peer is actually a Vior client/server
// before treating subsequent bytes as touch / video frames. An AOA
// cable can otherwise hand us any random accessory's stream.
var HelloMagic = [4]byte{'V', 'I', 'O', 'R'}

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
// can't trigger an OOM via make([]byte, len). Also guards against
// signed overflow in the length field.
func DecodeFrameHeader(header []byte) (frameType byte, length uint32) {
	if len(header) < 5 {
		return 0, 0
	}
	l := int64(binary.BigEndian.Uint32(header[1:5]))
	if l < 0 || l > int64(MaxFrameSize) {
		l = 0
	}
	return header[0], uint32(l)
}

// EncodePing / EncodePong build single-byte liveness frames.
func EncodePing() []byte { return []byte{FramePing} }
func EncodePong() []byte { return []byte{FramePong} }

// EncodeTouchEvent creates a touch frame.
// Format: [0x02][action 1B][x float32 4B][y float32 4B]
// The float32 fields are IEEE 754 bit patterns (math.Float32bits), not
// value-cast integers — the older value-cast lost the fractional part
// of sub-pixel coords and produced platform-defined garbage for any
// negative value.
func EncodeTouchEvent(action byte, x, y float32) []byte {
	buf := make([]byte, 10)
	buf[0] = FrameTouch
	buf[1] = action
	binary.BigEndian.PutUint32(buf[2:6], math.Float32bits(x))
	binary.BigEndian.PutUint32(buf[6:10], math.Float32bits(y))
	return buf
}

// DecodeTouchEvent parses touch data.
func DecodeTouchEvent(data []byte) (action byte, x, y float32) {
	if len(data) < 9 {
		return 0, 0, 0
	}
	return data[0],
		math.Float32frombits(binary.BigEndian.Uint32(data[1:5])),
		math.Float32frombits(binary.BigEndian.Uint32(data[5:9]))
}

// EncodeHello creates a hello frame with magic + version + screen
// dimensions. Total wire size = 18 bytes (1 type + 4 magic + 1 ver +
// 4 w + 4 h + 4 dpr*100). Magic + version let the peer verify it's
// talking to another Vior process before processing any payload.
func EncodeHello(width, height int, dpr float32) []byte {
	buf := make([]byte, 18)
	buf[0] = FrameHello
	copy(buf[1:5], HelloMagic[:])
	buf[5] = byte(ProtocolVersion)
	binary.BigEndian.PutUint32(buf[6:10], uint32(width))
	binary.BigEndian.PutUint32(buf[10:14], uint32(height))
	binary.BigEndian.PutUint32(buf[14:18], uint32(dpr*100))
	return buf
}

// DecodeHello parses hello data (the bytes AFTER the 0x03 type byte).
// Returns ok=false if magic doesn't match or the buffer is short, so
// the caller can log + disconnect without touching the dims.
func DecodeHello(data []byte) (width, height int, dpr float32, version byte, ok bool) {
	if len(data) < 17 {
		return 0, 0, 0, 0, false
	}
	if data[0] != HelloMagic[0] || data[1] != HelloMagic[1] ||
		data[2] != HelloMagic[2] || data[3] != HelloMagic[3] {
		return 0, 0, 0, 0, false
	}
	return int(binary.BigEndian.Uint32(data[5:9])),
		int(binary.BigEndian.Uint32(data[9:13])),
		float32(binary.BigEndian.Uint32(data[13:17])) / 100,
		data[4],
		true
}

// EncodeHelloAck builds the desktop → phone ack so the phone can flip
// `transportMode='usb'` only after the desktop has verified our hello.
// Wire size = 6 bytes: [0x05][magic 4B][ver 1B].
func EncodeHelloAck() []byte {
	buf := make([]byte, 6)
	buf[0] = FrameHelloAck
	copy(buf[1:5], HelloMagic[:])
	buf[5] = byte(ProtocolVersion)
	return buf
}

// DecodeHelloAck validates the ack payload (bytes after 0x05). Same
// shape as Hello but with no dims. Returns ok=false on magic mismatch.
func DecodeHelloAck(data []byte) (version byte, ok bool) {
	if len(data) < 5 {
		return 0, false
	}
	if data[0] != HelloMagic[0] || data[1] != HelloMagic[1] ||
		data[2] != HelloMagic[2] || data[3] != HelloMagic[3] {
		return 0, false
	}
	return data[4], true
}

// EncodeReady creates ready frame.
func EncodeReady(width, height int) []byte {
	buf := make([]byte, 9)
	buf[0] = FrameReady
	binary.BigEndian.PutUint32(buf[1:5], uint32(width))
	binary.BigEndian.PutUint32(buf[5:9], uint32(height))
	return buf
}
