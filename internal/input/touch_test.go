package input

import (
	"image"
	"sync"
	"testing"
)

type fakeCtrl struct {
	mu      sync.Mutex
	moves   [][2]int
	clicks  []MouseButton
	keys    []string
	scrolls [][2]int
	curX    int
	curY    int
	posErr  error
	mainX   int
	mainY   int
	mainW   int
	mainH   int
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
	// Cursor was inside the captured display → no warp; just dx applied.
	if c.moves[0] != [2]int{2510, 300} {
		t.Fatalf("expected in-place delta, got %v", c.moves[0])
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
	// Warp to main centre (960, 540) then apply dx=10, dy=0.
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

func TestHandleScroll(t *testing.T) {
	c := &fakeCtrl{}
	tm := NewTouchMapper(c, image.Rect(0, 0, 1920, 1080))
	_ = tm.HandleScroll(0, 3)
	_ = tm.HandleScroll(-2, 0)
	if len(c.scrolls) != 2 || c.scrolls[0] != [2]int{0, 3} || c.scrolls[1] != [2]int{-2, 0} {
		t.Fatalf("scroll dispatch wrong: %v", c.scrolls)
	}
}
