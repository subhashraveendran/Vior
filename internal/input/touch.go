package input

import (
	"fmt"
	"image"
	"math"
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
	// Reject NaN/Inf — all NaN comparisons are false, so without this
	// guard a malformed message would fall through to int(NaN) which
	// silently returns 0, teleporting the cursor on every bad event.
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return fmt.Errorf("invalid touch coords: x=%v y=%v", x, y)
	}
	// Clamp into the captured display rect so an attacker with a
	// valid pair-code session can't drive the cursor off-screen via
	// huge or negative values (e.g. x=1e10 → int(x) = INT_MAX-ish on
	// 64-bit, but undefined-ish on 32-bit OS APIs).
	w, h := t.displayBounds.Dx(), t.displayBounds.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("display bounds empty: %v", t.displayBounds)
	}
	if x < 0 {
		x = 0
	} else if x > float64(w-1) {
		x = float64(w - 1)
	}
	if y < 0 {
		y = 0
	} else if y > float64(h-1) {
		y = float64(h - 1)
	}
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
	// Clamp scroll deltas to a reasonable magnitude so a malicious
	// or buggy client can't flood the desktop with thousands of
	// scroll events in a single message. Real scroll gestures
	// produce single-digit deltas; 200 covers edge-case flings.
	const maxScrollDelta = 200
	if dx < -maxScrollDelta {
		dx = -maxScrollDelta
	} else if dx > maxScrollDelta {
		dx = maxScrollDelta
	}
	if dy < -maxScrollDelta {
		dy = -maxScrollDelta
	} else if dy > maxScrollDelta {
		dy = maxScrollDelta
	}
	return t.controller.Scroll(int(dx), int(dy))
}

// HandleMouse processes a relative-mouse event from the Remote tab.
// Supports: action "move" (dx/dy relative), "click", "rightclick", "middleclick".
func (t *TouchMapper) HandleMouse(action string, dx, dy float64) error {
	if math.IsNaN(dx) || math.IsNaN(dy) || math.IsInf(dx, 0) || math.IsInf(dy, 0) {
		return nil
	}
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
		// The cursor may legitimately live on either the host's main
		// display OR the captured display (which, in extend mode, is the
		// virtual display the phone is mirroring). Only warp the cursor
		// back to the main display if it has wandered off BOTH — that
		// covers the pathological case where a previous touch left it on
		// a stale virtual display rect that no longer exists, without
		// yanking it off the active virtual display every time the user
		// drags on the Remote trackpad. (Previous behavior warped on the
		// first move whenever the cursor sat outside the main display,
		// which made the host cursor pop to the centre of the main Mac
		// every time the user touched the Remote tab — the "remote moves
		// too" report.)
		mx, my, mw, mh := t.controller.MainDisplayBounds()
		if mw > 0 && mh > 0 {
			insideMain := t.cachedX >= mx && t.cachedY >= my &&
				t.cachedX < mx+mw && t.cachedY < my+mh
			insideCaptured := !t.displayBounds.Empty() &&
				t.cachedX >= t.displayBounds.Min.X && t.cachedY >= t.displayBounds.Min.Y &&
				t.cachedX < t.displayBounds.Max.X && t.cachedY < t.displayBounds.Max.Y
			if !insideMain && !insideCaptured {
				t.cachedX = mx + mw/2
				t.cachedY = my + mh/2
			}
		}
		// Mild pointer acceleration. On a tablet the user wants two
		// behaviours from one trackpad: pixel-precise fine work AND
		// large flings across a 4K virtual display. With a flat scale
		// the user either spends the whole transfer ratchet-dragging
		// or overshoots small targets. The classic remedy is a
		// nonlinear gain: small deltas pass through unchanged (gain=1),
		// long sweeps get a sub-linear boost from sqrt(magnitude).
		// The mobile side already pre-scales touch deltas by 2× to
		// keep the trackpad usable on a small slab of glass; this
		// adds a second stage that only kicks in past ~8 px of
		// movement per event (the "fling" regime).
		mag := math.Hypot(dx, dy)
		accel := mag / 8
		if accel < 1 {
			accel = 1
		}
		newX := t.cachedX + int(dx*accel)
		newY := t.cachedY + int(dy*accel)
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
