//go:build darwin

package input

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices

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

static void postScroll(double dx, double dy) {
	CGEventRef scroll = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitPixel, 1,
		(int32_t)dy, (int32_t)dx);
	CGEventPost(kCGHIDEventTap, scroll);
	CFRelease(scroll);
}

static void postKeyPress(const char *key) {
	CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
	CGEventRef up   = CGEventCreateKeyboardEvent(NULL, 0, false);

	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);

	CFRelease(down);
	CFRelease(up);
}
*/
import "C"

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

func (c *darwinController) TypeKey(key string) error {
	C.postKeyPress(C.CString(key))
	return nil
}

func (c *darwinController) Scroll(dx, dy int) error {
	C.postScroll(C.double(dx), C.double(dy))
	return nil
}

var _ Controller = (*darwinController)(nil)
