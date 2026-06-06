//go:build darwin

package virtual

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework CoreGraphics -framework Foundation

#include "display_darwin.h"
*/
import "C"
import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/subhashraveendran/vior/internal/capture"
)

var mu sync.Mutex

// Create spawns a virtual display with the given pixel dimensions and refresh rate.
// Returns the CGDirectDisplayID that kbinani/screenshot can capture from.
func Create(width, height uint32, refreshRate float64) (uint32, error) {
	mu.Lock()
	defer mu.Unlock()

	var displayID C.uint
	rc := C.vior_vd_create(C.uint(width), C.uint(height), C.double(refreshRate), &displayID)
	if rc != 0 {
		return 0, fmt.Errorf("virtual display creation failed (code %d)", int(rc))
	}
	return uint32(displayID), nil
}

// CreateHiDPI creates a virtual HiDPI display from logical point dimensions.
// Pixels will be 2x the logical dimensions (Retina scale).
func CreateHiDPI(logicalWidth, logicalHeight uint32, refreshRate float64) (uint32, error) {
	mu.Lock()
	defer mu.Unlock()

	var displayID C.uint
	rc := C.vior_vd_create_hidpi(C.uint(logicalWidth), C.uint(logicalHeight), C.double(refreshRate), &displayID)
	if rc != 0 {
		return 0, fmt.Errorf("virtual HiDPI display creation failed (code %d)", int(rc))
	}
	return uint32(displayID), nil
}

// Destroy tears down the virtual display created by Create. It blocks
// up to 2s waiting for capture.ListDisplays() to stop returning the
// torn-down display ID. If the ghost is still present after one
// teardown pass, it retries once before giving up — the CGVirtualDisplay
// private API has no formal -invalidate, so this poll+retry is the
// best we can do without entering kernel-mode hacks.
func Destroy() {
	mu.Lock()
	defer mu.Unlock()
	destroyLocked()
}

func destroyLocked() {
	gone := uint32(C.vior_vd_destroy())
	if gone == 0 {
		return
	}
	if waitForRemoval(gone, 2*time.Second) {
		return
	}
	// Ghost is still in the display list — usually a sign that the
	// runloop didn't get a chance to drain. Try one more teardown pass
	// after a short wait. (We can't re-call destroy on a nil-ed
	// _viorDisplay; instead we ping the runloop indirectly by listing.)
	log.Printf("virtual: display %d ghost-lingered after destroy; retrying", gone)
	time.Sleep(100 * time.Millisecond)
	_ = C.vior_vd_destroy() // idempotent, returns 0 when nil
	if !waitForRemoval(gone, 1*time.Second) {
		log.Printf("virtual: display %d still in display list after retry — user may see a ghost until next reconnect", gone)
	}
}

// waitForRemoval polls capture.ListDisplays until id disappears or the
// deadline elapses. Returns true if the display went away.
func waitForRemoval(id uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !displayPresent(id) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !displayPresent(id)
}

func displayPresent(id uint32) bool {
	return capture.FindDisplayIndexByID(id) >= 0
}
