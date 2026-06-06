// Package input handles remote mouse and keyboard control.
package input

// MouseButton represents a mouse button.
type MouseButton int

const (
	ButtonLeft MouseButton = iota
	ButtonRight
	ButtonMiddle
)

// Controller defines the interface for input control implementations.
type Controller interface {
	MoveMouse(x, y int) error
	MouseDown(x, y int, button MouseButton) error
	MouseUp(x, y int, button MouseButton) error
	Click(button MouseButton) error
	TypeKey(key string) error
	Scroll(dx, dy int) error
	// CurrentMousePos returns the current absolute mouse cursor position.
	CurrentMousePos() (int, int, error)
	// MainDisplayBounds returns the host's primary (visible) display rectangle
	// in absolute pixel coordinates. The Remote trackpad uses this to detect
	// when the cursor has wandered onto an invisible virtual display and
	// needs to be warped back so the user can see it move.
	MainDisplayBounds() (x, y, w, h int)
}

// DefaultController returns the platform-specific input controller.
var DefaultController Controller = newController()
