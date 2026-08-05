package usb

import (
	"bytes"
	"testing"
)

// captured records what splitFrames delivered.
type captured struct {
	frameType byte
	payload   []byte
}

// collect returns a dispatch func that records every frame and keeps going.
func collect(got *[]captured) func(byte, []byte) bool {
	return func(ft byte, payload []byte) bool {
		*got = append(*got, captured{ft, append([]byte(nil), payload...)})
		return true
	}
}

// The bug this guards: a USB bulk transfer is not a frame boundary. The reader
// used to take buf[0] as the type and buf[1:n] as the payload, so exactly one
// frame survived per transfer and everything batched behind it was discarded.
func TestSplitFramesDeliversEveryFrameInOneTransfer(t *testing.T) {
	var buf []byte
	buf = append(buf, EncodeTouchEvent(TouchDown, 10, 20)...)
	buf = append(buf, EncodeTouchEvent(TouchMove, 30, 40)...)
	buf = append(buf, EncodeTouchEvent(TouchUp, 50, 60)...)
	buf = append(buf, EncodePing()...)

	var got []captured
	consumed, stop, _, ok := splitFrames(buf, collect(&got))

	if !ok || stop {
		t.Fatalf("splitFrames ok=%v stop=%v, want ok=true stop=false", ok, stop)
	}
	if consumed != len(buf) {
		t.Fatalf("consumed %d of %d bytes", consumed, len(buf))
	}
	if len(got) != 4 {
		t.Fatalf("delivered %d frames, want 4 — the tail of the transfer was dropped", len(got))
	}

	for i, want := range []byte{FrameTouch, FrameTouch, FrameTouch, FramePing} {
		if got[i].frameType != want {
			t.Errorf("frame %d type = 0x%02x, want 0x%02x", i, got[i].frameType, want)
		}
	}

	// Payloads must be exactly one frame's worth, not the rest of the buffer.
	if n := len(got[0].payload); n != 9 {
		t.Errorf("touch payload = %d bytes, want 9 (decoder was seeing the following frames too)", n)
	}
	action, x, y := DecodeTouchEvent(got[2].payload)
	if action != TouchUp || x != 50 || y != 60 {
		t.Errorf("third touch = (%d, %v, %v), want (TouchUp, 50, 60)", action, x, y)
	}
}

// A frame split across two bulk transfers must be reassembled, not mangled.
func TestSplitFramesHoldsPartialFrameForNextRead(t *testing.T) {
	full := EncodeTouchEvent(TouchMove, 7, 9)
	head, tail := full[:4], full[4:]

	var got []captured
	consumed, _, _, ok := splitFrames(head, collect(&got))
	if !ok {
		t.Fatal("partial frame reported as unparsable")
	}
	if consumed != 0 {
		t.Fatalf("consumed %d bytes of an incomplete frame, want 0", consumed)
	}
	if len(got) != 0 {
		t.Fatalf("delivered %d frames from an incomplete buffer, want 0", len(got))
	}

	// Second transfer completes it.
	rest := append(append([]byte(nil), head...), tail...)
	consumed, _, _, ok = splitFrames(rest, collect(&got))
	if !ok || consumed != len(full) || len(got) != 1 {
		t.Fatalf("reassembly failed: ok=%v consumed=%d frames=%d", ok, consumed, len(got))
	}
	action, x, y := DecodeTouchEvent(got[0].payload)
	if action != TouchMove || x != 7 || y != 9 {
		t.Errorf("reassembled touch = (%d, %v, %v), want (TouchMove, 7, 9)", action, x, y)
	}
}

// Trailing bytes of a second frame must stay unconsumed so the caller can keep
// them for the next read.
func TestSplitFramesConsumesOnlyCompleteFrames(t *testing.T) {
	full := EncodeTouchEvent(TouchDown, 1, 2)
	buf := append(append([]byte(nil), full...), full[:5]...) // one whole + a fragment

	var got []captured
	consumed, _, _, ok := splitFrames(buf, collect(&got))
	if !ok {
		t.Fatal("unexpected parse failure")
	}
	if consumed != len(full) {
		t.Fatalf("consumed %d bytes, want %d — the fragment must be left for the next read", consumed, len(full))
	}
	if len(got) != 1 {
		t.Fatalf("delivered %d frames, want 1", len(got))
	}
}

// An unknown type is unrecoverable: there is no length, so no way to find the
// next boundary. It must be reported rather than skipped.
func TestSplitFramesRejectsUnknownType(t *testing.T) {
	buf := append(EncodePing(), 0xEE, 0x01, 0x02)

	var got []captured
	consumed, _, badType, ok := splitFrames(buf, collect(&got))
	if ok {
		t.Fatal("unknown frame type accepted — the stream would silently desync")
	}
	if badType != 0xEE {
		t.Errorf("badType = 0x%02x, want 0xEE", badType)
	}
	// Frames before the bad byte should still have been delivered.
	if consumed != 1 || len(got) != 1 {
		t.Errorf("consumed=%d frames=%d, want 1 and 1 (the valid ping preceding it)", consumed, len(got))
	}
}

// A dispatch that returns false stops the walk immediately — used for Bye and
// for protocol violations that tear the connection down.
func TestSplitFramesStopsWhenDispatchSaysSo(t *testing.T) {
	var buf []byte
	buf = append(buf, EncodePing()...)
	buf = append(buf, EncodeTouchEvent(TouchDown, 1, 1)...)
	buf = append(buf, EncodePing()...)

	seen := 0
	consumed, stop, _, ok := splitFrames(buf, func(ft byte, _ []byte) bool {
		seen++
		return ft != FrameTouch // stop on the touch frame
	})

	if !ok || !stop {
		t.Fatalf("ok=%v stop=%v, want ok=true stop=true", ok, stop)
	}
	if seen != 2 {
		t.Errorf("dispatched %d frames, want 2 — it kept going past the stop", seen)
	}
	if want := 1 + 10; consumed != want {
		t.Errorf("consumed = %d, want %d (through the stopping frame)", consumed, want)
	}
}

func TestSplitFramesOnEmptyBuffer(t *testing.T) {
	var got []captured
	consumed, stop, _, ok := splitFrames(nil, collect(&got))
	if !ok || stop || consumed != 0 || len(got) != 0 {
		t.Fatalf("empty buffer: consumed=%d stop=%v ok=%v frames=%d", consumed, stop, ok, len(got))
	}
}

// The declared sizes must match what the encoders actually produce, or the
// framing silently mis-splits every stream.
func TestInboundFrameSizeMatchesEncoders(t *testing.T) {
	cases := []struct {
		name    string
		encoded []byte
	}{
		{"hello", EncodeHello(1920, 1080, 2)},
		{"helloAck", EncodeHelloAck()},
		{"touch", EncodeTouchEvent(TouchDown, 1, 2)},
		{"ready", EncodeReady(800, 600)},
		{"ping", EncodePing()},
		{"pong", EncodePong()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, ok := InboundFrameSize(tc.encoded[0])
			if !ok {
				t.Fatalf("InboundFrameSize(0x%02x) reported unknown", tc.encoded[0])
			}
			if size != len(tc.encoded) {
				t.Errorf("declared size %d, encoder produced %d bytes", size, len(tc.encoded))
			}
		})
	}

	// Bye has no encoder helper but is a bare type byte.
	if size, ok := InboundFrameSize(FrameBye); !ok || size != 1 {
		t.Errorf("FrameBye size = %d (ok=%v), want 1", size, ok)
	}

	// Video is host→phone and its length is not derivable from the type,
	// so it must not be accepted as an inbound frame.
	if _, ok := InboundFrameSize(FrameVideo); ok {
		t.Error("FrameVideo accepted inbound — its length is not derivable from the type")
	}
}

// Round-trip a realistic batch through the same path readInput uses, including
// the caller's compaction step.
func TestFramingSurvivesArbitrarySplitPoints(t *testing.T) {
	var stream []byte
	stream = append(stream, EncodeHello(1170, 2532, 3)...)
	for i := range 5 {
		stream = append(stream, EncodeTouchEvent(TouchMove, float32(i), float32(i*2))...)
	}
	stream = append(stream, EncodePong()...)

	// Feed the stream one byte at a time — the most hostile chunking there is.
	var got []captured
	var pending []byte
	for i := range stream {
		pending = append(pending, stream[i])
		consumed, stop, _, ok := splitFrames(pending, collect(&got))
		if !ok || stop {
			t.Fatalf("byte %d: ok=%v stop=%v", i, ok, stop)
		}
		pending = append(pending[:0], pending[consumed:]...)
	}

	if len(pending) != 0 {
		t.Errorf("%d bytes left over, want 0", len(pending))
	}
	if len(got) != 7 {
		t.Fatalf("delivered %d frames, want 7", len(got))
	}
	if got[0].frameType != FrameHello || got[6].frameType != FramePong {
		t.Errorf("frame order wrong: first=0x%02x last=0x%02x", got[0].frameType, got[6].frameType)
	}
	w, h, dpr, ver, ok := DecodeHello(got[0].payload)
	if !ok || w != 1170 || h != 2532 || dpr != 3 || ver != ProtocolVersion {
		t.Errorf("hello decoded as %dx%d @%v v%d ok=%v", w, h, dpr, ver, ok)
	}
	if !bytes.Equal(got[3].payload, EncodeTouchEvent(TouchMove, 2, 4)[1:]) {
		t.Error("third touch payload did not survive byte-at-a-time framing")
	}
}
