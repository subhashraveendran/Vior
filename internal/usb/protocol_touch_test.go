package usb

import (
	"math"
	"testing"
)

// TestTouchEventRoundTripBitPattern pins the IEEE 754 framing fix. The
// old implementation value-cast float32→uint32 (truncating fraction,
// platform-defined for negatives). The new code uses bit reinterpret
// so the exact bit pattern survives a round trip. Without this,
// sub-pixel taps lost precision and negative values produced garbage.
func TestTouchEventRoundTripBitPattern(t *testing.T) {
	cases := []struct {
		name string
		x, y float32
	}{
		{"origin", 0, 0},
		{"integer", 320, 480},
		{"sub-pixel x", 320.5, 480.25},
		{"sub-pixel y", 0.1, 0.2},
		{"large positive", 1e6, 1e6},
		{"negative", -1, -3.5},
		{"very small", 1e-30, -1e-30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := EncodeTouchEvent(0x01, tc.x, tc.y)
			if len(buf) != 10 {
				t.Fatalf("encoded length = %d want 10", len(buf))
			}
			// DecodeTouchEvent expects the bytes AFTER the type byte
			// (length 9: action + 4 + 4).
			gotAction, gotX, gotY := DecodeTouchEvent(buf[1:])
			if gotAction != 0x01 {
				t.Errorf("action: got %#x want 0x01", gotAction)
			}
			if math.Float32bits(gotX) != math.Float32bits(tc.x) {
				t.Errorf("x round-trip: got %v (%#x) want %v (%#x)",
					gotX, math.Float32bits(gotX), tc.x, math.Float32bits(tc.x))
			}
			if math.Float32bits(gotY) != math.Float32bits(tc.y) {
				t.Errorf("y round-trip: got %v (%#x) want %v (%#x)",
					gotY, math.Float32bits(gotY), tc.y, math.Float32bits(tc.y))
			}
		})
	}
}
