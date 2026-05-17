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
}

// DefaultController returns the platform-specific input controller.
var DefaultController Controller = newController()
