//go:build darwin

package input

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices

#include <stdlib.h>
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

static void postMouseMove(double x, double y) {
	CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved,
		CGPointMake(x, y), kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, move);
	CFRelease(move);
}

static void getMouseEventTypes(int button, CGEventType *downType, CGEventType *upType, CGMouseButton *cgButton) {
	switch (button) {
		case 1:  *downType = kCGEventRightMouseDown; *upType = kCGEventRightMouseUp;
		         *cgButton = kCGMouseButtonRight; break;
		case 2:  *downType = kCGEventOtherMouseDown; *upType = kCGEventOtherMouseUp;
		         *cgButton = kCGMouseButtonCenter; break;
		default: *downType = kCGEventLeftMouseDown; *upType = kCGEventLeftMouseUp;
		         *cgButton = kCGMouseButtonLeft; break;
	}
}

static void postMouseDown(double x, double y, int button) {
	CGEventType downType, upType;
	CGMouseButton cgButton;
	getMouseEventTypes(button, &downType, &upType, &cgButton);
	CGEventRef down = CGEventCreateMouseEvent(NULL, downType, CGPointMake(x, y), cgButton);
	CGEventPost(kCGHIDEventTap, down);
	CFRelease(down);
}

static void postMouseUp(double x, double y, int button) {
	CGEventType downType, upType;
	CGMouseButton cgButton;
	getMouseEventTypes(button, &downType, &upType, &cgButton);
	CGEventRef up = CGEventCreateMouseEvent(NULL, upType, CGPointMake(x, y), cgButton);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(up);
}

static void postMouseClick(int button) {
	CGEventType downType, upType;
	CGMouseButton cgButton;
	getMouseEventTypes(button, &downType, &upType, &cgButton);
	CGPoint loc = CGEventGetLocation(CGEventCreate(NULL));

	CGEventRef down = CGEventCreateMouseEvent(NULL, downType, loc, cgButton);
	CGEventPost(kCGHIDEventTap, down);
	CFRelease(down);

	CGEventRef up = CGEventCreateMouseEvent(NULL, upType, loc, cgButton);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(up);
}

static void getMousePos(int *outX, int *outY) {
	CGEventRef e = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(e);
	*outX = (int)p.x;
	*outY = (int)p.y;
	CFRelease(e);
}

// Force the system cursor visible on the main display. Phone-driven mouse
// moves can park the pointer on a virtual display where macOS hides it;
// calling CGDisplayShowCursor restores visibility on next host interaction.
static void showCursor(void) {
	CGAssociateMouseAndMouseCursorPosition(true);
	CGDisplayShowCursor(kCGDirectMainDisplay);
}

static void postScroll(double dx, double dy) {
	CGEventRef scroll = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitPixel, 1,
		(int32_t)dy, (int32_t)dx);
	CGEventPost(kCGHIDEventTap, scroll);
	CFRelease(scroll);
}

// Post a synthetic key event using a raw virtual keycode.
static void postKeyCode(int keycode) {
	CGEventRef down = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, true);
	CGEventPost(kCGHIDEventTap, down);
	CFRelease(down);
	CGEventRef up = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, false);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(up);
}

// Post a key with modifier flags (Cmd/Ctrl/Shift/Alt).
static void postKeyCodeMods(int keycode, int flags) {
	CGEventRef down = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, true);
	CGEventSetFlags(down, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, down);
	CFRelease(down);
	CGEventRef up = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, false);
	CGEventSetFlags(up, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(up);
}

// Post a Unicode character as a synthetic key event. Works for any printable
// character without needing a per-key virtual keycode mapping.
static void postUnicode(const char *utf8) {
	if (!utf8 || !*utf8) return;
	CFStringRef s = CFStringCreateWithCString(NULL, utf8, kCFStringEncodingUTF8);
	CFIndex len = CFStringGetLength(s);
	if (len > 32) len = 32;
	UniChar buf[32];
	CFStringGetCharacters(s, CFRangeMake(0, len), buf);
	CFRelease(s);

	CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
	CGEventKeyboardSetUnicodeString(down, len, buf);
	CGEventPost(kCGHIDEventTap, down);
	CFRelease(down);

	CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
	CGEventKeyboardSetUnicodeString(up, len, buf);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(up);
}
*/
import "C"

import (
	"strings"
	"unsafe"
)

type darwinController struct{}

func newController() Controller {
	return &darwinController{}
}

func (c *darwinController) MoveMouse(x, y int) error {
	C.postMouseMove(C.double(x), C.double(y))
	C.showCursor()
	return nil
}

func (c *darwinController) MouseDown(x, y int, button MouseButton) error {
	C.postMouseDown(C.double(x), C.double(y), C.int(button))
	return nil
}

func (c *darwinController) MouseUp(x, y int, button MouseButton) error {
	C.postMouseUp(C.double(x), C.double(y), C.int(button))
	return nil
}

func (c *darwinController) Click(button MouseButton) error {
	C.postMouseClick(C.int(button))
	return nil
}

// macOS virtual keycodes (Carbon/HIToolbox/Events.h).
var darwinKeyCodes = map[string]int{
	"BackSpace": 51, "Return": 36, "Enter": 36, "Tab": 48, "Escape": 53, "Esc": 53, "Space": 49,
	"Up": 126, "Down": 125, "Left": 123, "Right": 124,
	"Home": 115, "End": 119, "PageUp": 116, "PageDown": 121,
	"Delete": 117, "F1": 122, "F2": 120, "F3": 99, "F4": 118, "F5": 96, "F6": 97,
	"F7": 98, "F8": 100, "F9": 101, "F10": 109, "F11": 103, "F12": 111,
	// Letter keycodes for modifier chords (lowercase). US layout.
	"a": 0, "b": 11, "c": 8, "d": 2, "e": 14, "f": 3, "g": 5, "h": 4, "i": 34,
	"j": 38, "k": 40, "l": 37, "m": 46, "n": 45, "o": 31, "p": 35, "q": 12,
	"r": 15, "s": 1, "t": 17, "u": 32, "v": 9, "w": 13, "x": 7, "y": 16, "z": 6,
	"0": 29, "1": 18, "2": 19, "3": 20, "4": 21, "5": 23, "6": 22, "7": 26,
	"8": 28, "9": 25,
}

// macOS CGEventFlags for modifiers.
const (
	flagCmd   = 1 << 20
	flagShift = 1 << 17
	flagAlt   = 1 << 19
	flagCtrl  = 1 << 18
	flagFn    = 1 << 23
)

func (c *darwinController) TypeKey(key string) error {
	// Modifier chord: "Cmd+c", "Ctrl+Shift+t", "Cmd+Shift+4" etc.
	if strings.Contains(key, "+") {
		parts := strings.Split(key, "+")
		var flags int
		var last string
		for _, p := range parts {
			switch strings.ToLower(p) {
			case "cmd", "meta", "win", "super":
				flags |= flagCmd
			case "shift":
				flags |= flagShift
			case "alt", "opt", "option":
				flags |= flagAlt
			case "ctrl", "control":
				flags |= flagCtrl
			case "fn":
				flags |= flagFn
			default:
				last = p
			}
		}
		if last == "" {
			return nil
		}
		if code, ok := darwinKeyCodes[strings.ToLower(last)]; ok {
			C.postKeyCodeMods(C.int(code), C.int(flags))
			return nil
		}
	}
	if code, ok := darwinKeyCodes[key]; ok {
		C.postKeyCode(C.int(code))
		return nil
	}
	cs := C.CString(key)
	defer C.free(unsafe.Pointer(cs))
	C.postUnicode(cs)
	return nil
}

func (c *darwinController) Scroll(dx, dy int) error {
	C.postScroll(C.double(dx), C.double(dy))
	return nil
}

func (c *darwinController) CurrentMousePos() (int, int, error) {
	var x, y C.int
	C.getMousePos(&x, &y)
	return int(x), int(y), nil
}

var _ Controller = (*darwinController)(nil)
