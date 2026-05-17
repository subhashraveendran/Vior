//go:build !darwin && !linux && !windows

package input

import "fmt"

type stubController struct{}

func newController() Controller {
	return &stubController{}
}

func (c *stubController) MoveMouse(x, y int) error {
	return fmt.Errorf("input control not supported on this platform")
}
func (c *stubController) MouseDown(x, y int, button MouseButton) error {
	return fmt.Errorf("input control not supported on this platform")
}
func (c *stubController) MouseUp(x, y int, button MouseButton) error {
	return fmt.Errorf("input control not supported on this platform")
}
func (c *stubController) Click(button MouseButton) error {
	return fmt.Errorf("input control not supported on this platform")
}
func (c *stubController) TypeKey(key string) error {
	return fmt.Errorf("input control not supported on this platform")
}
func (c *stubController) Scroll(dx, dy int) error {
	return fmt.Errorf("input control not supported on this platform")
}

var _ Controller = (*stubController)(nil)
