//go:build windows

package input

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

var (
	user32    = syscall.NewLazyDLL("user32.dll")
	sendInput = user32.NewProc("SendInput")
	setCursor = user32.NewProc("SetCursorPos")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfLeftDown   = 0x0002
	mouseeventfLeftUp     = 0x0004
	mouseeventfRightDown  = 0x0008
	mouseeventfRightUp    = 0x0010
	mouseeventfMiddleDown = 0x0020
	mouseeventfMiddleUp   = 0x0040
	mouseeventfWheel      = 0x0800
	mouseeventfHWheel     = 0x1000
	keyeventfKeyUp        = 0x0002

	inputSize = 40 // sizeof(INPUT) on 64-bit Windows
)

// makeMouseInput creates a raw INPUT struct for mouse events.
func makeMouseInput(flags, mouseData uintptr) []byte {
	buf := make([]byte, inputSize)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(inputMouse))
	// MOUSEINPUT starts at offset 8 (after type + padding)
	// dx=0, dy=0 (offsets 8,16), mouseData (offset 24), dwFlags (offset 28)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(mouseData))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(flags))
	return buf
}

// makeKeyInput creates a raw INPUT struct for keyboard events.
func makeKeyInput(vk uint16, flags uintptr) []byte {
	buf := make([]byte, inputSize)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(inputKeyboard))
	// KEYBDINPUT starts at offset 8
	// wVk (offset 8, uint16), wScan (offset 10, uint16), dwFlags (offset 12, uint32)
	binary.LittleEndian.PutUint16(buf[8:10], vk)
	binary.LittleEndian.PutUint32(buf[12:16], uint32(flags))
	return buf
}

func callSendInput(buf []byte) {
	sendInput.Call(1, uintptr(unsafe.Pointer(&buf[0])), uintptr(inputSize))
}

type winController struct{}

func newController() Controller {
	return &winController{}
}

func (c *winController) MoveMouse(x, y int) error {
	setCursor.Call(uintptr(x), uintptr(y))
	return nil
}

func (c *winController) MouseDown(x, y int, button MouseButton) error {
	setCursor.Call(uintptr(x), uintptr(y))
	var flags uintptr
	switch button {
	case ButtonRight:
		flags = mouseeventfRightDown
	case ButtonMiddle:
		flags = mouseeventfMiddleDown
	default:
		flags = mouseeventfLeftDown
	}
	callSendInput(makeMouseInput(flags, 0))
	return nil
}

func (c *winController) MouseUp(x, y int, button MouseButton) error {
	setCursor.Call(uintptr(x), uintptr(y))
	var flags uintptr
	switch button {
	case ButtonRight:
		flags = mouseeventfRightUp
	case ButtonMiddle:
		flags = mouseeventfMiddleUp
	default:
		flags = mouseeventfLeftUp
	}
	callSendInput(makeMouseInput(flags, 0))
	return nil
}

func (c *winController) Click(button MouseButton) error {
	var downFlags, upFlags uintptr
	switch button {
	case ButtonRight:
		downFlags, upFlags = mouseeventfRightDown, mouseeventfRightUp
	case ButtonMiddle:
		downFlags, upFlags = mouseeventfMiddleDown, mouseeventfMiddleUp
	default:
		downFlags, upFlags = mouseeventfLeftDown, mouseeventfLeftUp
	}
	callSendInput(makeMouseInput(downFlags, 0))
	callSendInput(makeMouseInput(upFlags, 0))
	return nil
}

func (c *winController) TypeKey(key string) error {
	var vk uint16
	if len(key) == 1 {
		r := []rune(key)[0]
		vk = uint16(r)
	}
	callSendInput(makeKeyInput(vk, 0))
	callSendInput(makeKeyInput(vk, keyeventfKeyUp))
	return nil
}

func (c *winController) Scroll(dx, dy int) error {
	if dy != 0 {
		callSendInput(makeMouseInput(mouseeventfWheel, uintptr(int32(dy)*120)))
	}
	if dx != 0 {
		callSendInput(makeMouseInput(mouseeventfHWheel, uintptr(int32(dx)*120)))
	}
	return nil
}

var _ Controller = (*winController)(nil)
