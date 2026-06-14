package input

import (
	"image"
	"math"
	"testing"
)

// recCtrl records every Mouse* call's coordinates so the bounds-clamp
// in HandleTouch can be asserted.
type recCtrl struct {
	last [2]int
	down [2]int
	up   [2]int
}

func (c *recCtrl) MoveMouse(x, y int) error              { c.last = [2]int{x, y}; return nil }
func (c *recCtrl) MouseDown(x, y int, b MouseButton) error {
	c.down = [2]int{x, y}
	c.last = [2]int{x, y}
	return nil
}
func (c *recCtrl) MouseUp(x, y int, b MouseButton) error {
	c.up = [2]int{x, y}
	c.last = [2]int{x, y}
	return nil
}
func (c *recCtrl) Click(b MouseButton) error             { return nil }
func (c *recCtrl) TypeKey(k string) error                { return nil }
func (c *recCtrl) Scroll(dx, dy int) error               { return nil }
func (c *recCtrl) CurrentMousePos() (int, int, error)    { return 0, 0, nil }
func (c *recCtrl) MainDisplayBounds() (int, int, int, int) { return 0, 0, 1920, 1080 }

// TestHandleTouchClampsToDisplayBounds — out-of-range coordinates from a
// malicious client (huge positive, negative) must clamp to the captured
// display rect rather than reach the OS as raw int(x) where x could be
// nonsense. The display is at origin (200, 100) sized 1280×800.
func TestHandleTouchClampsToDisplayBounds(t *testing.T) {
	bounds := image.Rect(200, 100, 200+1280, 100+800)
	m := NewTouchMapper(&recCtrl{}, bounds)

	cases := []struct {
		name     string
		x, y     float64
		wantX    int
		wantY    int
	}{
		{"in range middle", 640, 400, 200 + 640, 100 + 400},
		{"x negative clamps to 0", -50, 100, 200 + 0, 100 + 100},
		{"y negative clamps to 0", 100, -1e9, 200 + 100, 100 + 0},
		{"x past right edge clamps to width-1", 99999, 50, 200 + 1279, 100 + 50},
		{"y past bottom clamps to height-1", 50, 99999, 200 + 50, 100 + 799},
		{"both huge clamp to bottom-right corner", 1e10, 1e10, 200 + 1279, 100 + 799},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &recCtrl{}
			m = NewTouchMapper(c, bounds)
			if err := m.HandleTouch("down", tc.x, tc.y); err != nil {
				t.Fatalf("HandleTouch returned error: %v", err)
			}
			if c.down[0] != tc.wantX || c.down[1] != tc.wantY {
				t.Errorf("HandleTouch(down, %v, %v) → (%d,%d), want (%d,%d)",
					tc.x, tc.y, c.down[0], c.down[1], tc.wantX, tc.wantY)
			}
		})
	}
}

// TestHandleTouchRejectsNaN — every NaN comparison is false, so without
// an explicit guard the bounds clamp lets NaN through to int(NaN) = 0,
// teleporting the cursor on every malformed event.
func TestHandleTouchRejectsNaN(t *testing.T) {
	bounds := image.Rect(0, 0, 1000, 1000)
	cases := []struct {
		name string
		x, y float64
	}{
		{"x is NaN", math.NaN(), 50},
		{"y is NaN", 50, math.NaN()},
		{"x is +Inf", math.Inf(1), 50},
		{"y is -Inf", 50, math.Inf(-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &recCtrl{last: [2]int{-1, -1}}
			m := NewTouchMapper(c, bounds)
			err := m.HandleTouch("move", tc.x, tc.y)
			if err == nil {
				t.Errorf("HandleTouch(%v,%v) accepted bad coords", tc.x, tc.y)
			}
			if c.last[0] != -1 || c.last[1] != -1 {
				t.Errorf("HandleTouch with bad coords reached controller: %v", c.last)
			}
		})
	}
}

// TestHandleTouchEmptyBoundsRejected — a closed virtual display has zero
// width/height and cannot map a touch event to absolute coords. Reject
// rather than divide-by-zero or accept-into-a-point.
func TestHandleTouchEmptyBoundsRejected(t *testing.T) {
	c := &recCtrl{last: [2]int{-1, -1}}
	m := NewTouchMapper(c, image.Rect(0, 0, 0, 0))
	if err := m.HandleTouch("move", 0, 0); err == nil {
		t.Errorf("HandleTouch on empty bounds did not error")
	}
	if c.last[0] != -1 || c.last[1] != -1 {
		t.Errorf("HandleTouch on empty bounds reached controller: %v", c.last)
	}
}
