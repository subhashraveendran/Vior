package usb

import (
	"bytes"
	"testing"
)

// TestEncodeHelloByteLayout pins the wire shape of FrameHello so any
// future tweak that drifts away from the 18-byte layout that the Java
// side (MainActivity.sendHello) hand-builds gets caught at CI time.
// Layout: [0x03][V][I][O][R][1][w 4B][h 4B][dpr*100 4B].
func TestEncodeHelloByteLayout(t *testing.T) {
	got := EncodeHello(1920, 1080, 2.0)
	if len(got) != 18 {
		t.Fatalf("EncodeHello length=%d want 18", len(got))
	}
	if got[0] != FrameHello {
		t.Errorf("type byte=0x%02x want 0x%02x", got[0], FrameHello)
	}
	if !bytes.Equal(got[1:5], HelloMagic[:]) {
		t.Errorf("magic=%v want %v", got[1:5], HelloMagic)
	}
	if got[5] != byte(ProtocolVersion) {
		t.Errorf("version=%d want %d", got[5], ProtocolVersion)
	}
	// Width 1920 = 0x00000780.
	wantW := []byte{0x00, 0x00, 0x07, 0x80}
	if !bytes.Equal(got[6:10], wantW) {
		t.Errorf("width bytes=%v want %v", got[6:10], wantW)
	}
}

// TestDecodeHelloRoundTrip ensures the dims survive a full encode →
// strip-type-byte → decode cycle, which is how the desktop side
// actually consumes a phone hello (accessory.go::readInput slices
// buf[1:n] before calling DecodeHello).
func TestDecodeHelloRoundTrip(t *testing.T) {
	wire := EncodeHello(2560, 1600, 3.5)
	// readInput passes data = buf[1:n] — i.e. everything past the
	// 0x03 type byte. DecodeHello must be called on that slice.
	w, h, dpr, ver, ok := DecodeHello(wire[1:])
	if !ok {
		t.Fatal("DecodeHello returned ok=false on a valid hello")
	}
	if w != 2560 || h != 1600 {
		t.Errorf("dims=(%d,%d) want (2560,1600)", w, h)
	}
	if dpr < 3.49 || dpr > 3.51 {
		t.Errorf("dpr=%f want ~3.5", dpr)
	}
	if ver != byte(ProtocolVersion) {
		t.Errorf("ver=%d want %d", ver, ProtocolVersion)
	}
}

// TestDecodeHelloMagicMismatch — random AOA accessory plugged into the
// host should be rejected (ok=false) so we don't feed its payload to
// OnConnect / treat subsequent bytes as touch.
func TestDecodeHelloMagicMismatch(t *testing.T) {
	bad := make([]byte, 17)
	copy(bad, []byte{'X', 'X', 'X', 'X', 1}) // wrong magic
	_, _, _, _, ok := DecodeHello(bad)
	if ok {
		t.Error("DecodeHello accepted wrong magic")
	}
}

// TestDecodeHelloShortBuffer — truncated frames must drop cleanly.
func TestDecodeHelloShortBuffer(t *testing.T) {
	for _, n := range []int{0, 1, 4, 16} {
		buf := make([]byte, n)
		copy(buf, HelloMagic[:])
		_, _, _, _, ok := DecodeHello(buf)
		if ok {
			t.Errorf("len=%d: expected ok=false", n)
		}
	}
}

// TestEncodeHelloAckByteLayout pins the 6-byte ack layout.
// [0x05][V][I][O][R][1].
func TestEncodeHelloAckByteLayout(t *testing.T) {
	got := EncodeHelloAck()
	if len(got) != 6 {
		t.Fatalf("EncodeHelloAck length=%d want 6", len(got))
	}
	if got[0] != FrameHelloAck {
		t.Errorf("type=0x%02x want 0x%02x", got[0], FrameHelloAck)
	}
	if !bytes.Equal(got[1:5], HelloMagic[:]) {
		t.Errorf("magic=%v want %v", got[1:5], HelloMagic)
	}
	if got[5] != byte(ProtocolVersion) {
		t.Errorf("ver=%d want %d", got[5], ProtocolVersion)
	}
}

// TestDecodeHelloAckRoundTrip — the mobile-side decode reads from the
// full frame (incl. type byte at [0]), but DecodeHelloAck takes the
// data-only slice the same way DecodeHello does.
func TestDecodeHelloAckRoundTrip(t *testing.T) {
	wire := EncodeHelloAck()
	ver, ok := DecodeHelloAck(wire[1:])
	if !ok {
		t.Fatal("DecodeHelloAck returned ok=false on a valid ack")
	}
	if ver != byte(ProtocolVersion) {
		t.Errorf("ver=%d want %d", ver, ProtocolVersion)
	}
}

// TestDecodeHelloAckShortBuffer — protect against a truncated read
// (USB bulk transfers can split frames; a 5-byte buffer would still
// look "almost valid" if we didn't gate on len>=5 + magic+ver).
func TestDecodeHelloAckShortBuffer(t *testing.T) {
	for _, n := range []int{0, 1, 4} {
		_, ok := DecodeHelloAck(make([]byte, n))
		if ok {
			t.Errorf("len=%d: expected ok=false", n)
		}
	}
}

// TestDecodeFrameHeaderClampsLength — a malicious / corrupt header with
// length > MaxFrameSize must be clamped to 0 instead of triggering a
// multi-GB allocation in the caller's make([]byte, len).
func TestDecodeFrameHeaderClampsLength(t *testing.T) {
	bad := []byte{FrameVideo, 0xFF, 0xFF, 0xFF, 0xFF} // ~4 GB
	_, l := DecodeFrameHeader(bad)
	if l != 0 {
		t.Errorf("oversize header returned len=%d, want 0 (clamped)", l)
	}

	// Short header → both zero.
	if ty, l := DecodeFrameHeader([]byte{0x01}); ty != 0 || l != 0 {
		t.Errorf("short header got (%d,%d) want (0,0)", ty, l)
	}
}

// TestEncodeVideoFrameRoundTrip — header length + payload survive
// encode → decode. Catches an accidental endianness flip.
func TestEncodeVideoFrameRoundTrip(t *testing.T) {
	payload := []byte("hello jpeg")
	wire := EncodeVideoFrame(payload)
	if wire[0] != FrameVideo {
		t.Fatalf("type=0x%02x want 0x%02x", wire[0], FrameVideo)
	}
	ty, l := DecodeFrameHeader(wire[:5])
	if ty != FrameVideo {
		t.Errorf("decoded type=0x%02x", ty)
	}
	if int(l) != len(payload) {
		t.Errorf("len=%d want %d", l, len(payload))
	}
	if !bytes.Equal(wire[5:], payload) {
		t.Errorf("payload mismatch")
	}
}

// TestEncodeReadyByteLayout — desktop → phone "display is ready"
// frame, 9 bytes: [0x04][w 4B][h 4B].
func TestEncodeReadyByteLayout(t *testing.T) {
	got := EncodeReady(2560, 1440)
	if len(got) != 9 {
		t.Fatalf("EncodeReady len=%d want 9", len(got))
	}
	if got[0] != FrameReady {
		t.Errorf("type=0x%02x want 0x%02x", got[0], FrameReady)
	}
}

// TestEncodePingPongIsSingleByte — the heartbeat is intentionally
// a one-byte frame so the receive-loop short-circuit (len<5 drop)
// must NOT reject it. Documents that single-byte control frames are
// special-cased upstream.
func TestEncodePingPongIsSingleByte(t *testing.T) {
	if p := EncodePing(); len(p) != 1 || p[0] != FramePing {
		t.Errorf("EncodePing=%v want [0x%02x]", p, FramePing)
	}
	if p := EncodePong(); len(p) != 1 || p[0] != FramePong {
		t.Errorf("EncodePong=%v want [0x%02x]", p, FramePong)
	}
}
