package securechan

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// pair builds the two ends of a channel from one shared key.
func pair(t *testing.T) (initiator, responder *Channel) {
	t.Helper()
	var key [KeySize]byte
	for i := range key {
		key[i] = byte(i * 7)
	}
	i, err := NewChannel(key[:], true)
	if err != nil {
		t.Fatalf("NewChannel initiator: %v", err)
	}
	r, err := NewChannel(key[:], false)
	if err != nil {
		t.Fatalf("NewChannel responder: %v", err)
	}
	return i, r
}

func TestRoundTripBothDirections(t *testing.T) {
	i, r := pair(t)

	// initiator -> responder
	msg := []byte("frames, input, file chunks")
	frame, err := i.Seal(msg)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := r.Open(frame)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("i->r mismatch: got %q want %q", got, msg)
	}

	// responder -> initiator
	msg2 := []byte("ready + resize + bye")
	frame2, err := r.Seal(msg2)
	if err != nil {
		t.Fatalf("Seal r: %v", err)
	}
	got2, err := i.Open(frame2)
	if err != nil {
		t.Fatalf("Open i: %v", err)
	}
	if !bytes.Equal(got2, msg2) {
		t.Fatalf("r->i mismatch: got %q want %q", got2, msg2)
	}
}

func TestEmptyPlaintextRoundTrips(t *testing.T) {
	i, r := pair(t)
	frame, err := i.Seal(nil)
	if err != nil {
		t.Fatalf("Seal empty: %v", err)
	}
	got, err := r.Open(frame)
	if err != nil {
		t.Fatalf("Open empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes", len(got))
	}
}

func TestTamperedFrameRejected(t *testing.T) {
	i, r := pair(t)
	frame, err := i.Seal([]byte("authentic"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	// Flip a bit in the ciphertext body (past the 8-byte counter prefix).
	frame[len(frame)-1] ^= 0x01
	if _, err := r.Open(frame); err != ErrDecrypt {
		t.Fatalf("tamper: want ErrDecrypt, got %v", err)
	}
}

func TestReplayRejected(t *testing.T) {
	i, r := pair(t)
	frame, err := i.Seal([]byte("once"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := r.Open(frame); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, err := r.Open(frame); err != ErrReplay {
		t.Fatalf("replay: want ErrReplay, got %v", err)
	}
}

func TestReorderRejected(t *testing.T) {
	i, r := pair(t)
	f0, _ := i.Seal([]byte("m0"))
	f1, _ := i.Seal([]byte("m1"))
	// Deliver f1 first, then f0 (which now has a lower counter).
	if _, err := r.Open(f1); err != nil {
		t.Fatalf("Open f1: %v", err)
	}
	if _, err := r.Open(f0); err != ErrReplay {
		t.Fatalf("reorder: want ErrReplay for stale counter, got %v", err)
	}
}

func TestWrongKeyRejected(t *testing.T) {
	i, _ := pair(t)
	var other [KeySize]byte
	for j := range other {
		other[j] = 0xAB
	}
	rOther, err := NewChannel(other[:], false)
	if err != nil {
		t.Fatalf("NewChannel other: %v", err)
	}
	frame, _ := i.Seal([]byte("secret"))
	if _, err := rOther.Open(frame); err != ErrDecrypt {
		t.Fatalf("wrong key: want ErrDecrypt, got %v", err)
	}
}

// A peer must not be able to open its own sealed frame: send and receive keys
// are distinct per direction, so an attacker reflecting a frame back gets
// nothing.
func TestReflectionRejected(t *testing.T) {
	i, _ := pair(t)
	frame, _ := i.Seal([]byte("no reflection"))
	if _, err := i.Open(frame); err != ErrDecrypt {
		t.Fatalf("reflection: want ErrDecrypt, got %v", err)
	}
}

func TestManyMessagesInOrder(t *testing.T) {
	i, r := pair(t)
	for n := 0; n < 1000; n++ {
		msg := make([]byte, n%64)
		for k := range msg {
			msg[k] = byte(n + k)
		}
		frame, err := i.Seal(msg)
		if err != nil {
			t.Fatalf("Seal %d: %v", n, err)
		}
		got, err := r.Open(frame)
		if err != nil {
			t.Fatalf("Open %d: %v", n, err)
		}
		if !bytes.Equal(got, msg) {
			t.Fatalf("msg %d mismatch", n)
		}
	}
}

func TestShortInputs(t *testing.T) {
	if _, err := NewChannel(make([]byte, KeySize-1), true); err != ErrShortKey {
		t.Fatalf("short key: want ErrShortKey, got %v", err)
	}
	i, r := pair(t)
	_ = i
	if _, err := r.Open([]byte{0x00, 0x01}); err != ErrShortFrame {
		t.Fatalf("short frame: want ErrShortFrame, got %v", err)
	}
}

func TestNonceExhaustion(t *testing.T) {
	i, _ := pair(t)
	i.sendCounter = math.MaxUint64
	if _, err := i.Seal([]byte("last")); err != ErrNonceExhausted {
		t.Fatalf("exhaustion: want ErrNonceExhausted, got %v", err)
	}
}

// The frame's leading 8 bytes are the plaintext counter, and it advances by one
// per Seal. (This is what lets the receiver reconstruct the nonce.)
func TestCounterPrefixAdvances(t *testing.T) {
	i, _ := pair(t)
	for want := uint64(0); want < 5; want++ {
		frame, err := i.Seal([]byte("x"))
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		if got := binary.BigEndian.Uint64(frame[:counterPrefix]); got != want {
			t.Fatalf("counter prefix: got %d want %d", got, want)
		}
	}
}

// Guard the Overhead constant so callers sizing buffers stay correct if the
// framing changes.
func TestOverhead(t *testing.T) {
	i, _ := pair(t)
	frame, _ := i.Seal(nil)
	if len(frame) != Overhead {
		t.Fatalf("empty frame len = %d, want Overhead=%d", len(frame), Overhead)
	}
}
