//go:build windows

package input

import (
	"syscall"
	"unsafe"
)

var (
	user32    = syscall.NewLazyDLL("user32.dll")
	sendInput = user32.NewProc("SendInput")
	setCursor = user32.NewProc("SetCursorPos")
)

const (
	INPUT_MOUSE            = 0
	INPUT_KEYBOARD         = 1
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_HWHEEL     = 0x1000
	KEYEVENTF_KEYUP        = 0x0002
)

type mouseInput struct {
	dx, dy, mouseData, dwFlags, time, dwExtraInfo uintptr
}

type keyInput struct {
	wVk, wScan, dwFlags, time, dwExtraInfo uintptr
}

type winInput struct {
	inputType uint32
	mi        mouseInput
	_         [8]byte // padding
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
	var downFlags uintptr
	switch button {
	case ButtonRight:
		downFlags = MOUSEEVENTF_RIGHTDOWN
	case ButtonMiddle:
		downFlags = MOUSEEVENTF_MIDDLEDOWN
	default:
		downFlags = MOUSEEVENTF_LEFTDOWN
	}
	inp := winInput{inputType: INPUT_MOUSE}
	inp.mi.dwFlags = downFlags
	sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))
	return nil
}

func (c *winController) MouseUp(x, y int, button MouseButton) error {
	setCursor.Call(uintptr(x), uintptr(y))
	var upFlags uintptr
	switch button {
	case ButtonRight:
		upFlags = MOUSEEVENTF_RIGHTUP
	case ButtonMiddle:
		upFlags = MOUSEEVENTF_MIDDLEUP
	default:
		upFlags = MOUSEEVENTF_LEFTUP
	}
	inp := winInput{inputType: INPUT_MOUSE}
	inp.mi.dwFlags = upFlags
	sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))
	return nil
}

func (c *winController) Click(button MouseButton) error {
	var downFlags, upFlags uintptr
	switch button {
	case ButtonRight:
		downFlags, upFlags = MOUSEEVENTF_RIGHTDOWN, MOUSEEVENTF_RIGHTUP
	case ButtonMiddle:
		downFlags, upFlags = MOUSEEVENTF_MIDDLEDOWN, MOUSEEVENTF_MIDDLEUP
	default:
		downFlags, upFlags = MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP
	}

	inp := winInput{inputType: INPUT_MOUSE}
	inp.mi.dwFlags = downFlags
	sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))

	inp.mi.dwFlags = upFlags
	sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))

	return nil
}

func (c *winController) TypeKey(key string) error {
	// Simplified: send virtual key code 0 for now.
	// Full keyboard support needs scan code mapping.
	vk := uintptr(0)
	if len(key) == 1 {
		vk = uintptr(syscall.StringToUTF16(key)[0])
	}

	inp := winInput{inputType: INPUT_KEYBOARD}
	inp.mi.dwFlags = 0
	inp.mi.wVk = vk
	sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))

	inp.mi.dwFlags = KEYEVENTF_KEYUP
	sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))

	return nil
}

func (c *winController) Scroll(dx, dy int) error {
	if dy != 0 {
		inp := winInput{inputType: INPUT_MOUSE}
		inp.mi.dwFlags = MOUSEEVENTF_WHEEL
		inp.mi.mouseData = uintptr(int32(dy) * 120)
		sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))
	}
	if dx != 0 {
		inp := winInput{inputType: INPUT_MOUSE}
		inp.mi.dwFlags = MOUSEEVENTF_HWHEEL
		inp.mi.mouseData = uintptr(int32(dx) * 120)
		sendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))
	}
	return nil
}

var _ Controller = (*winController)(nil)
