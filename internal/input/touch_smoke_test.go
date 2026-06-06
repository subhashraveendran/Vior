//go:build darwin

package input

import (
	"image"
	"os"
	"testing"
)

// Optional smoke test against the real Darwin CGEvent injector. Skips
// unless RUN_INPUT_SMOKE=1 is set so CI doesn't randomly jiggle the
// cursor on dev machines. Run with:
//
//	RUN_INPUT_SMOKE=1 go test ./internal/input -run TestDarwinMouseSmoke -v
func TestDarwinMouseSmoke(t *testing.T) {
	if os.Getenv("RUN_INPUT_SMOKE") != "1" {
		t.Skip("set RUN_INPUT_SMOKE=1 to exercise real CGEvent injection")
	}
	if !HasAccessibility(false) {
		t.Skip("accessibility not granted")
	}
	tm := NewTouchMapper(DefaultController, image.Rect(0, 0, 1920, 1080))
	x, y, err := DefaultController.CurrentMousePos()
	if err != nil {
		t.Fatalf("CurrentMousePos: %v", err)
	}
	t.Logf("start pos: %d,%d", x, y)
	for i := 0; i < 5; i++ {
		if err := tm.HandleMouse("move", 5, 0); err != nil {
			t.Fatalf("move: %v", err)
		}
	}
	nx, ny, _ := DefaultController.CurrentMousePos()
	t.Logf("end pos: %d,%d (delta %d,%d)", nx, ny, nx-x, ny-y)
	if nx == x && ny == y {
		t.Fatalf("cursor did not move (delta 0,0) — CGEventPost was dropped")
	}
}
