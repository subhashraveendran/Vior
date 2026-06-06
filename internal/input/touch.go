package input

import (
	"fmt"
	"image"
	"sync"
	"time"
)

// TouchMapper translates touch events from a web client into mouse events,
// mapping coordinates from the client's virtual display space to the desktop's
// absolute coordinate space.
type TouchMapper struct {
	controller    Controller
	displayBounds image.Rectangle

	// Coalesced mouse position. The Remote-tab trackpad sends ~60
	// mouse-move events / second; querying the OS cursor location on
	// each one (CGEventCreate + CGEventGetLocation) is a noticeable
	// cgo overhead on busy systems and shows up as drag jitter. We
	// now refresh from the OS only on the first event of a drag or
	// after cachedPosIdle of silence, applying deltas to our own
	// cached position in between.
	mouseMu      sync.Mutex
	cachedX      int
	cachedY      int
	cachedAt     time.Time
	hasCachedPos bool
}

// cachedPosIdle is how long the cached cursor position is considered
// fresh. After this much silence we re-query the OS so we pick up
// movements the user made with their physical mouse/trackpad.
const cachedPosIdle = 50 * time.Millisecond

// NewTouchMapper creates a mapper for translating touch coordinates.
// displayBounds is the virtual display's position and size in desktop pixel space.
func NewTouchMapper(ctrl Controller, displayBounds image.Rectangle) *TouchMapper {
	return &TouchMapper{
		controller:    ctrl,
		displayBounds: displayBounds,
	}
}

// HandleTouch processes a touch event from the web client.
// x and y are pixel coordinates relative to the virtual display (0,0 = top-left of display).
func (t *TouchMapper) HandleTouch(action string, x, y float64) error {
	absX := t.displayBounds.Min.X + int(x)
	absY := t.displayBounds.Min.Y + int(y)

	switch action {
	case "down":
		return t.controller.MouseDown(absX, absY, ButtonLeft)
	case "move":
		return t.controller.MoveMouse(absX, absY)
	case "up":
		return t.controller.MouseUp(absX, absY, ButtonLeft)
	default:
		return fmt.Errorf("unknown touch action: %s", action)
	}
}

// HandleScroll processes a scroll event from the web client.
func (t *TouchMapper) HandleScroll(dx, dy float64) error {
	return t.controller.Scroll(int(dx), int(dy))
}

// HandleMouse processes a relative-mouse event from the Remote tab.
// Supports: action "move" (dx/dy relative), "click", "rightclick", "middleclick".
func (t *TouchMapper) HandleMouse(action string, dx, dy float64) error {
	switch action {
	case "move":
		t.mouseMu.Lock()
		defer t.mouseMu.Unlock()
		now := time.Now()
		// Re-query the OS cursor when we don't have a cached position
		// yet, or when the cache is stale enough that the user might
		// have moved the physical pointer in between.
		if !t.hasCachedPos || now.Sub(t.cachedAt) > cachedPosIdle {
			x, y, err := t.controller.CurrentMousePos()
			if err != nil {
				return err
			}
			t.cachedX, t.cachedY = x, y
			t.hasCachedPos = true
		}
		// If the cursor was parked on an invisible virtual display (created
		// for the Stream tab's extend-mode capture), the user would never
		// see the Remote-tab trackpad move it because the new absolute
		// target is outside any visible screen. Warp back to the main
		// display before applying the delta so the cursor is visible
		// again immediately.
		mx, my, mw, mh := t.controller.MainDisplayBounds()
		if mw > 0 && mh > 0 {
			if t.cachedX < mx || t.cachedY < my || t.cachedX >= mx+mw || t.cachedY >= my+mh {
				t.cachedX = mx + mw/2
				t.cachedY = my + mh/2
			}
		}
		newX := t.cachedX + int(dx)
		newY := t.cachedY + int(dy)
		// Update the cache to the synthetic target — subsequent moves
		// stack onto it without another OS round-trip.
		t.cachedX, t.cachedY = newX, newY
		t.cachedAt = now
		return t.controller.MoveMouse(newX, newY)
	case "click":
		return t.controller.Click(ButtonLeft)
	case "rightclick":
		return t.controller.Click(ButtonRight)
	case "middleclick":
		return t.controller.Click(ButtonMiddle)
	default:
		return fmt.Errorf("unknown mouse action: %s", action)
	}
}

// InvalidateMousePosCache forces the next HandleMouse("move") call to
// re-read the cursor position from the OS. Callers should invoke this
// when they know the host moved the cursor outside of our control —
// e.g. after a display reconfiguration.
func (t *TouchMapper) InvalidateMousePosCache() {
	t.mouseMu.Lock()
	t.hasCachedPos = false
	t.mouseMu.Unlock()
}

// SetDisplayBounds updates the display bounds (e.g. after display reconfiguration).
func (t *TouchMapper) SetDisplayBounds(bounds image.Rectangle) {
	t.displayBounds = bounds
}
