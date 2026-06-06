package input

import (
	"image"
	"sync"
	"testing"
	"time"
)

type fakeCtrl struct {
	mu       sync.Mutex
	moves    [][2]int
	clicks   []MouseButton
	keys     []string
	scrolls  [][2]int
	curX     int
	curY     int
	posErr   error
	mainX    int
	mainY    int
	mainW    int
	mainH    int
	posCalls int
}

func (f *fakeCtrl) MoveMouse(x, y int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.moves = append(f.moves, [2]int{x, y})
	f.curX, f.curY = x, y
	return nil
}
func (f *fakeCtrl) MouseDown(x, y int, b MouseButton) error { return nil }
func (f *fakeCtrl) MouseUp(x, y int, b MouseButton) error   { return nil }
func (f *fakeCtrl) Click(b MouseButton) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clicks = append(f.clicks, b)
	return nil
}
func (f *fakeCtrl) TypeKey(k string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, k)
	return nil
}
func (f *fakeCtrl) Scroll(dx, dy int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scrolls = append(f.scrolls, [2]int{dx, dy})
	return nil
}
func (f *fakeCtrl) CurrentMousePos() (int, int, error) {
	f.mu.Lock()
	f.posCalls++
	f.mu.Unlock()
	if f.posErr != nil {
		return 0, 0, f.posErr
	}
	return f.curX, f.curY, nil
}
func (f *fakeCtrl) MainDisplayBounds() (int, int, int, int) {
	return f.mainX, f.mainY, f.mainW, f.mainH
}

// HandleMouse("move", dx, dy) must read the current cursor location and
// post a new absolute position offset by dx/dy. A bug would either skip
// the move entirely or accumulate the wrong axis.
func TestHandleMouseMoveAccumulates(t *testing.T) {
	// Cursor already on the main display — Remote moves should just apply
	// the delta, no warp.
	c := &fakeCtrl{curX: 500, curY: 400, mainW: 1920, mainH: 1080}
	tm := NewTouchMapper(c, image.Rect(0, 0, 1920, 1080))

	if err := tm.HandleMouse("move", 10, -5); err != nil {
		t.Fatal(err)
	}
	if err := tm.HandleMouse("move", 3.7, 2.2); err != nil {
		t.Fatal(err)
	}

	// With the sub-linear acceleration in HandleMouse:
	//   Move 1: mag=hypot(10,-5)=11.18, accel=11.18/8=1.397
	//     dx=int(10*1.397)=13, dy=int(-5*1.397)=-6 → (513, 394).
	//   Move 2: mag=hypot(3.7,2.2)=4.30, accel<1 → clamped to 1
	//     dx=int(3.7)=3, dy=int(2.2)=2 → (516, 396).
	want := [][2]int{{513, 394}, {516, 396}}
	if len(c.moves) != 2 {
		t.Fatalf("got %d moves want 2: %v", len(c.moves), c.moves)
	}
	for i, m := range c.moves {
		if m != want[i] {
			t.Fatalf("move %d got %v want %v", i, m, want[i])
		}
	}
}

// When the cursor lives on the captured (virtual) display the Remote
// tab is mirroring, the next "move" must apply the delta in place — NOT
// warp the cursor back to the main display. Warping in that case yanks
// the host's main-screen cursor on every touch (the "remote moves too"
// bug) and breaks the user's ability to actually drive the virtual
// display from the Remote tab.
func TestHandleMouseMoveStaysOnCapturedDisplay(t *testing.T) {
	c := &fakeCtrl{
		curX: 2500, curY: 300, // on the virtual display, off the main
		mainW: 1920, mainH: 1080,
	}
	tm := NewTouchMapper(c, image.Rect(1920, 0, 4000, 1300))

	if err := tm.HandleMouse("move", 10, 0); err != nil {
		t.Fatal(err)
	}

	if len(c.moves) != 1 {
		t.Fatalf("expected one move, got %v", c.moves)
	}
	// Cursor was inside the captured display → no warp; just dx*accel.
	// mag=10, accel=10/8=1.25, dx=int(10*1.25)=12 → (2512, 300).
	if c.moves[0] != [2]int{2512, 300} {
		t.Fatalf("expected in-place delta with accel, got %v", c.moves[0])
	}
}

// Cursor parked on a stale display (outside both main and the current
// captured display) is the only case where the warp-back-to-main rescue
// should trigger.
func TestHandleMouseMoveWarpsBackFromStaleDisplay(t *testing.T) {
	c := &fakeCtrl{
		curX: 5000, curY: 5000, // off main AND off captured
		mainW: 1920, mainH: 1080,
	}
	tm := NewTouchMapper(c, image.Rect(1920, 0, 4000, 1300))

	if err := tm.HandleMouse("move", 10, 0); err != nil {
		t.Fatal(err)
	}

	if len(c.moves) != 1 {
		t.Fatalf("expected one move, got %v", c.moves)
	}
	// Warp to main centre (960, 540) then apply dx=10*accel(1.25)=12.
	if c.moves[0] != [2]int{972, 540} {
		t.Fatalf("expected warp-to-main + accel delta, got %v", c.moves[0])
	}
}

// Small touch deltas (under the 8-pixel threshold) must pass through
// at gain 1 so fine-grained selection / button-targeting still works.
// Without the clamp, sub-1 accel values would shrink small moves to
// zero pixels and the user could never hit a precise target.
func TestHandleMouseSmallDeltaNoBoost(t *testing.T) {
	c := &fakeCtrl{curX: 500, curY: 400, mainW: 1920, mainH: 1080}
	tm := NewTouchMapper(c, image.Rect(0, 0, 1920, 1080))
	if err := tm.HandleMouse("move", 4, 0); err != nil {
		t.Fatal(err)
	}
	if c.moves[0] != [2]int{504, 400} {
		t.Fatalf("small delta got %v want {504,400} (no boost)", c.moves[0])
	}
}

// Large fling-sized deltas should get the sqrt-shaped boost so the
// user can cross a 4K virtual display in one swipe.
func TestHandleMouseLargeDeltaIsBoosted(t *testing.T) {
	c := &fakeCtrl{curX: 500, curY: 400, mainW: 4000, mainH: 1500}
	tm := NewTouchMapper(c, image.Rect(0, 0, 4000, 1500))
	if err := tm.HandleMouse("move", 64, 0); err != nil {
		t.Fatal(err)
	}
	// mag=64, accel=64/8=8, so dx becomes 64*8=512.
	if c.moves[0] != [2]int{1012, 400} {
		t.Fatalf("large fling got %v want {1012,400} (boosted 8×)", c.moves[0])
	}
}

func TestHandleMouseClick(t *testing.T) {
	c := &fakeCtrl{}
	tm := NewTouchMapper(c, image.Rect(0, 0, 1920, 1080))
	_ = tm.HandleMouse("click", 0, 0)
	_ = tm.HandleMouse("rightclick", 0, 0)
	if len(c.clicks) != 2 || c.clicks[0] != ButtonLeft || c.clicks[1] != ButtonRight {
		t.Fatalf("click dispatch wrong: %v", c.clicks)
	}
}

// TestHandleMouseCoalescesOSQueries verifies that a tight burst of
// mouse-move events does NOT round-trip through CurrentMousePos every
// time — the OS query happens once, then the cache is used until idle.
// Regression test for drag jitter on busy systems where the per-event
// cgo call into CGEventCreate became a bottleneck.
func TestHandleMouseCoalescesOSQueries(t *testing.T) {
	c := &fakeCtrl{curX: 500, curY: 400, mainW: 1920, mainH: 1080}
	tm := NewTouchMapper(c, image.Rect(0, 0, 1920, 1080))

	// Simulate a 60fps drag of 30 events with no idle gap between them.
	for i := 0; i < 30; i++ {
		if err := tm.HandleMouse("move", 1, 0); err != nil {
			t.Fatalf("move %d: %v", i, err)
		}
	}
	if c.posCalls != 1 {
		t.Fatalf("CurrentMousePos called %d times, want 1 (coalesced)", c.posCalls)
	}
	// Cache must have followed every delta, so the final move ends at
	// (500 + 30, 400).
	if got := c.moves[len(c.moves)-1]; got != [2]int{530, 400} {
		t.Fatalf("final move %v want {530,400}", got)
	}

	// After a longer idle the cache should expire and we re-query.
	time.Sleep(cachedPosIdle + 10*time.Millisecond)
	_ = tm.HandleMouse("move", 1, 0)
	if c.posCalls != 2 {
		t.Fatalf("CurrentMousePos called %d times after idle, want 2", c.posCalls)
	}
}

func TestInvalidateMousePosCache(t *testing.T) {
	c := &fakeCtrl{curX: 10, curY: 10, mainW: 1920, mainH: 1080}
	tm := NewTouchMapper(c, image.Rect(0, 0, 1920, 1080))
	_ = tm.HandleMouse("move", 0, 0)
	if c.posCalls != 1 {
		t.Fatalf("setup: posCalls=%d want 1", c.posCalls)
	}
	tm.InvalidateMousePosCache()
	_ = tm.HandleMouse("move", 0, 0)
	if c.posCalls != 2 {
		t.Fatalf("after invalidate: posCalls=%d want 2", c.posCalls)
	}
}

func TestHandleScroll(t *testing.T) {
	c := &fakeCtrl{}
	tm := NewTouchMapper(c, image.Rect(0, 0, 1920, 1080))
	_ = tm.HandleScroll(0, 3)
	_ = tm.HandleScroll(-2, 0)
	if len(c.scrolls) != 2 || c.scrolls[0] != [2]int{0, 3} || c.scrolls[1] != [2]int{-2, 0} {
		t.Fatalf("scroll dispatch wrong: %v", c.scrolls)
	}
}
