package input

import (
	"fmt"
	"image"
)

// TouchMapper translates touch events from a web client into mouse events,
// mapping coordinates from the client's virtual display space to the desktop's
// absolute coordinate space.
type TouchMapper struct {
	controller    Controller
	displayBounds image.Rectangle
}

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
		x, y, err := t.controller.CurrentMousePos()
		if err != nil {
			return err
		}
		// If the cursor was parked on an invisible virtual display (created
		// for the Stream tab's extend-mode capture), the user would never
		// see the Remote-tab trackpad move it because the new absolute
		// target is outside any visible screen. Warp back to the main
		// display before applying the delta so the cursor is visible
		// again immediately.
		mx, my, mw, mh := t.controller.MainDisplayBounds()
		if mw > 0 && mh > 0 {
			if x < mx || y < my || x >= mx+mw || y >= my+mh {
				x = mx + mw/2
				y = my + mh/2
			}
		}
		return t.controller.MoveMouse(x+int(dx), y+int(dy))
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

// SetDisplayBounds updates the display bounds (e.g. after display reconfiguration).
func (t *TouchMapper) SetDisplayBounds(bounds image.Rectangle) {
	t.displayBounds = bounds
}
