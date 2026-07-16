//go:build windows

package input

import (
	"encoding/binary"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	sendInput        = user32.NewProc("SendInput")
	setCursor        = user32.NewProc("SetCursorPos")
	getCursorPos     = user32.NewProc("GetCursorPos")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	smCXScreen = 0
	smCYScreen = 1
)

type winPoint struct{ X, Y int32 }

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
	keyeventfUnicode      = 0x0004

	inputSize = 40 // sizeof(INPUT) on 64-bit Windows

	// Modifier virtual-key codes.
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt
	vkLWin    = 0x5B
)

// namedVK maps the symbolic key names the mobile client sends to Win32
// virtual-key codes. Names mirror the darwin key table so the two
// platforms accept the same wire vocabulary.
var namedVK = map[string]uint16{
	"BackSpace": 0x08, "Backspace": 0x08,
	"Tab":    0x09,
	"Return": 0x0D, "Enter": 0x0D,
	"Escape": 0x1B, "Esc": 0x1B,
	"Space": 0x20, "space": 0x20,
	"Delete": 0x2E, "Del": 0x2E,
	"Home": 0x24, "End": 0x23,
	"PageUp": 0x21, "PageDown": 0x22,
	"Left": 0x25, "Up": 0x26, "Right": 0x27, "Down": 0x28,
	"F1": 0x70, "F2": 0x71, "F3": 0x72, "F4": 0x73,
	"F5": 0x74, "F6": 0x75, "F7": 0x76, "F8": 0x77,
	"F9": 0x78, "F10": 0x79, "F11": 0x7A, "F12": 0x7B,
}

// runeToVK maps a single printable rune to its virtual-key code for use
// in a modifier chord (where Unicode injection wouldn't combine with the
// held modifier). A-Z/a-z and 0-9 map to their ASCII-uppercase VK, which
// on Windows equals the VK constant.
func runeToVK(r rune) uint16 {
	switch {
	case r >= 'a' && r <= 'z':
		return uint16(r - 'a' + 'A')
	case r >= 'A' && r <= 'Z':
		return uint16(r)
	case r >= '0' && r <= '9':
		return uint16(r)
	default:
		return uint16(r)
	}
}

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
// KEYBDINPUT layout: wVk (offset 8), wScan (offset 10), dwFlags (offset 12).
func makeKeyInput(vk, scan uint16, flags uintptr) []byte {
	buf := make([]byte, inputSize)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(inputKeyboard))
	binary.LittleEndian.PutUint16(buf[8:10], vk)
	binary.LittleEndian.PutUint16(buf[10:12], scan)
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

// TypeKey injects a key or a modifier chord (e.g. "a", "Return",
// "Cmd+c", "Cmd+Shift+z"). The old implementation only handled single
// ASCII chars — every named key and every chord was a silent no-op, and
// lowercase letters/symbols hit the wrong virtual-key code — so the
// Windows Remote keyboard was effectively dead.
func (c *winController) TypeKey(key string) error {
	if key == "" {
		return nil
	}
	// Split modifier chord. A lone "+" (the plus key itself) has no
	// modifiers; guard the empty-final case by only splitting when there
	// is a non-empty tail after a "+".
	finalKey := key
	var mods []uint16
	if strings.Contains(key, "+") && key != "+" {
		parts := strings.Split(key, "+")
		finalKey = parts[len(parts)-1]
		for _, m := range parts[:len(parts)-1] {
			switch strings.ToLower(m) {
			case "cmd", "super", "win", "meta", "command":
				// Map Cmd→Ctrl on Windows so macOS-style shortcuts
				// (Cmd+C/V/Z) hit the Windows Ctrl-based equivalents.
				mods = append(mods, vkControl)
			case "ctrl", "control":
				mods = append(mods, vkControl)
			case "alt", "option", "opt":
				mods = append(mods, vkMenu)
			case "shift":
				mods = append(mods, vkShift)
			case "hyper":
				mods = append(mods, vkLWin)
			}
		}
	}

	// Press modifiers.
	for _, m := range mods {
		callSendInput(makeKeyInput(m, 0, 0))
	}

	switch {
	case namedVK[finalKey] != 0:
		vk := namedVK[finalKey]
		callSendInput(makeKeyInput(vk, 0, 0))
		callSendInput(makeKeyInput(vk, 0, keyeventfKeyUp))
	case len([]rune(finalKey)) == 1 && len(mods) > 0:
		// Char inside a chord → send its VK so it combines with the held
		// modifier (Unicode injection ignores modifiers).
		vk := runeToVK([]rune(finalKey)[0])
		callSendInput(makeKeyInput(vk, 0, 0))
		callSendInput(makeKeyInput(vk, 0, keyeventfKeyUp))
	case len([]rune(finalKey)) == 1:
		// Plain printable char → Unicode injection, layout-independent
		// (correct for lowercase, symbols, and non-ASCII).
		sc := uint16([]rune(finalKey)[0])
		callSendInput(makeKeyInput(0, sc, keyeventfUnicode))
		callSendInput(makeKeyInput(0, sc, keyeventfUnicode|keyeventfKeyUp))
	}

	// Release modifiers in reverse order.
	for i := len(mods) - 1; i >= 0; i-- {
		callSendInput(makeKeyInput(mods[i], 0, keyeventfKeyUp))
	}
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

func (c *winController) CurrentMousePos() (int, int, error) {
	var p winPoint
	getCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y), nil
}

func (c *winController) MainDisplayBounds() (int, int, int, int) {
	w, _, _ := getSystemMetrics.Call(uintptr(smCXScreen))
	h, _, _ := getSystemMetrics.Call(uintptr(smCYScreen))
	return 0, 0, int(int32(w)), int(int32(h))
}

var _ Controller = (*winController)(nil)
