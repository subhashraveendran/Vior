//go:build linux

package input

/*
#cgo LDFLAGS: -lX11 -lXtst

#include <X11/Xlib.h>
#include <X11/extensions/XTest.h>

static Display *openDisplay(void) {
	return XOpenDisplay(NULL);
}

static void closeDisplay(Display *dpy) {
	if (dpy) XCloseDisplay(dpy);
}

static void moveMouse(Display *dpy, int x, int y) {
	if (!dpy) return;
	XTestFakeMotionEvent(dpy, DefaultScreen(dpy), x, y, CurrentTime);
	XFlush(dpy);
}

static int mapButton(int button) {
	int btn[] = {1, 3, 2};
	return (button >= 0 && button <= 2) ? btn[button] : 1;
}

static void clickMouse(Display *dpy, int button) {
	if (!dpy) return;
	int b = mapButton(button);
	XTestFakeButtonEvent(dpy, b, True, CurrentTime);
	XTestFakeButtonEvent(dpy, b, False, CurrentTime);
	XFlush(dpy);
}

static void mouseDown(Display *dpy, int x, int y, int button) {
	if (!dpy) return;
	XTestFakeMotionEvent(dpy, DefaultScreen(dpy), x, y, CurrentTime);
	XTestFakeButtonEvent(dpy, mapButton(button), True, CurrentTime);
	XFlush(dpy);
}

static void mouseUp(Display *dpy, int x, int y, int button) {
	if (!dpy) return;
	XTestFakeMotionEvent(dpy, DefaultScreen(dpy), x, y, CurrentTime);
	XTestFakeButtonEvent(dpy, mapButton(button), False, CurrentTime);
	XFlush(dpy);
}

static void scrollMouse(Display *dpy, int dx, int dy) {
	if (!dpy) return;
	if (dy != 0) {
		int btn = (dy > 0) ? 5 : 4;
		for (int i = 0; i < abs(dy); i++) {
			XTestFakeButtonEvent(dpy, btn, True, CurrentTime);
			XTestFakeButtonEvent(dpy, btn, False, CurrentTime);
		}
	}
	if (dx != 0) {
		int btn = (dx > 0) ? 7 : 6;
		for (int i = 0; i < abs(dx); i++) {
			XTestFakeButtonEvent(dpy, btn, True, CurrentTime);
			XTestFakeButtonEvent(dpy, btn, False, CurrentTime);
		}
	}
	XFlush(dpy);
}

static void pressKey(Display *dpy, const char *key) {
	if (!dpy || !key) return;
	KeySym ks = XStringToKeysym(key);
	if (ks == NoSymbol) return;
	KeyCode kc = XKeysymToKeycode(dpy, ks);
	XTestFakeKeyEvent(dpy, kc, True, CurrentTime);
	XTestFakeKeyEvent(dpy, kc, False, CurrentTime);
	XFlush(dpy);
}
*/
import "C"

type linuxController struct {
	dpy *C.Display
}

func newController() Controller {
	return &linuxController{
		dpy: C.openDisplay(),
	}
}

func (c *linuxController) MoveMouse(x, y int) error {
	C.moveMouse(c.dpy, C.int(x), C.int(y))
	return nil
}

func (c *linuxController) MouseDown(x, y int, button MouseButton) error {
	C.mouseDown(c.dpy, C.int(x), C.int(y), C.int(button))
	return nil
}

func (c *linuxController) MouseUp(x, y int, button MouseButton) error {
	C.mouseUp(c.dpy, C.int(x), C.int(y), C.int(button))
	return nil
}

func (c *linuxController) Click(button MouseButton) error {
	C.clickMouse(c.dpy, C.int(button))
	return nil
}

func (c *linuxController) TypeKey(key string) error {
	C.pressKey(c.dpy, C.CString(key))
	return nil
}

func (c *linuxController) Scroll(dx, dy int) error {
	C.scrollMouse(c.dpy, C.int(dx), C.int(dy))
	return nil
}

var _ Controller = (*linuxController)(nil)
