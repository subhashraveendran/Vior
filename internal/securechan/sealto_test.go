package securechan

import (
	"bytes"
	"fmt"
	"testing"
)

// TestSealToRoundTrip proves SealTo produces frames indistinguishable from
// Seal's: the peer opens them, the counter advances, and interleaving the
// two seal entry points on one channel stays coherent.
func TestSealToRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, KeySize)
	sender, err := NewChannel(key, true)
	if err != nil {
		t.Fatalf("NewChannel sender: %v", err)
	}
	receiver, err := NewChannel(key, false)
	if err != nil {
		t.Fatalf("NewChannel receiver: %v", err)
	}

	buf := make([]byte, 0, 1024)
	msgs := [][]byte{
		[]byte("first via SealTo"),
		[]byte("second via Seal"),
		[]byte("third via SealTo again"),
	}
	for i, msg := range msgs {
		var sealed []byte
		if i == 1 {
			sealed, err = sender.Seal(msg)
		} else {
			sealed, err = sender.SealTo(buf, msg)
		}
		if err != nil {
			t.Fatalf("seal %d: %v", i, err)
		}
		plain, err := receiver.Open(sealed)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if !bytes.Equal(plain, msg) {
			t.Fatalf("round trip %d: got %q, want %q", i, plain, msg)
		}
	}
}

// TestSealToZeroAlloc is the reason SealTo exists: with a big-enough dst the
// 30fps video path must not allocate per frame.
func TestSealToZeroAlloc(t *testing.T) {
	key := bytes.Repeat([]byte{0x17}, KeySize)
	ch, err := NewChannel(key, true)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	plain := make([]byte, 64*1024)
	buf := make([]byte, 0, len(plain)+Overhead)

	allocs := testing.AllocsPerRun(50, func() {
		out, err := ch.SealTo(buf, plain)
		if err != nil {
			t.Fatalf("SealTo: %v", err)
		}
		_ = out
	})
	if allocs != 0 {
		t.Fatalf("SealTo allocated %.1f times per call with a sufficient buffer, want 0", allocs)
	}
}

// TestSealToGrowsShortBuffer: an undersized dst must still work (fresh
// allocation, same output), so pool warm-up needs no special casing.
func TestSealToGrowsShortBuffer(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, KeySize)
	sender, _ := NewChannel(key, true)
	receiver, _ := NewChannel(key, false)

	msg := bytes.Repeat([]byte{0xAB}, 4096)
	sealed, err := sender.SealTo(make([]byte, 0, 8), msg)
	if err != nil {
		t.Fatalf("SealTo: %v", err)
	}
	plain, err := receiver.Open(sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(plain, msg) {
		t.Fatal("short-buffer SealTo corrupted the payload")
	}
}

// BenchmarkSealFrame measures the seal cost at representative JPEG frame
// sizes (issue #87 testing plan). "reuse" is the production video path
// (pooled buffer, zero alloc); "alloc" is what Seal did before SealTo.
func BenchmarkSealFrame(b *testing.B) {
	key := bytes.Repeat([]byte{0x42}, KeySize)
	for _, size := range []int{100 * 1024, 300 * 1024, 600 * 1024} {
		plain := make([]byte, size)
		b.Run(fmt.Sprintf("%dKB/reuse", size/1024), func(b *testing.B) {
			ch, err := NewChannel(key, true)
			if err != nil {
				b.Fatalf("NewChannel: %v", err)
			}
			buf := make([]byte, 0, size+Overhead)
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ch.SealTo(buf, plain); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("%dKB/alloc", size/1024), func(b *testing.B) {
			ch, err := NewChannel(key, true)
			if err != nil {
				b.Fatalf("NewChannel: %v", err)
			}
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ch.Seal(plain); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
