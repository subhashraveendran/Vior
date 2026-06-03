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

import "unsafe"

type darwinController struct{}

func newController() Controller {
	return &darwinController{}
}

func (c *darwinController) MoveMouse(x, y int) error {
	C.postMouseMove(C.double(x), C.double(y))
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

// macOS virtual keycodes for named special keys. Source: Carbon/HIToolbox/Events.h.
var darwinKeyCodes = map[string]int{
	"BackSpace": 51, "Return": 36, "Tab": 48, "Escape": 53, "Space": 49,
	"Up": 126, "Down": 125, "Left": 123, "Right": 124,
	"Home": 115, "End": 119, "PageUp": 116, "PageDown": 121,
	"Delete": 117, "F1": 122, "F2": 120, "F3": 99, "F4": 118,
}

func (c *darwinController) TypeKey(key string) error {
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
