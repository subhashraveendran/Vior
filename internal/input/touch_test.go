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

	want := [][2]int{{510, 395}, {513, 397}}
	if len(c.moves) != 2 {
		t.Fatalf("got %d moves want 2: %v", len(c.moves), c.moves)
	}
	for i, m := range c.moves {
		if m != want[i] {
			t.Fatalf("move %d got %v want %v", i, m, want[i])
		}
	}
}

// When the cursor has wandered onto a virtual display (Stream-tab touches
// can park it there), the next Remote-tab "move" must warp the cursor
// back to the centre of the host's primary display so the user can see
// it. Without the warp the cursor keeps sliding around an invisible
// region and the Remote tab looks dead.
func TestHandleMouseMoveWarpsBackFromVirtualDisplay(t *testing.T) {
	c := &fakeCtrl{
		curX: 2500, curY: 300, // off-screen, past mainW=1920
		mainW: 1920, mainH: 1080,
	}
	tm := NewTouchMapper(c, image.Rect(1920, 0, 4000, 1300))

	if err := tm.HandleMouse("move", 10, 0); err != nil {
		t.Fatal(err)
	}

	if len(c.moves) != 1 {
		t.Fatalf("expected one move, got %v", c.moves)
	}
	// Should have warped to centre (960, 540) then applied dx=10,dy=0.
	if c.moves[0] != [2]int{970, 540} {
		t.Fatalf("expected warp-to-main + delta, got %v", c.moves[0])
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
