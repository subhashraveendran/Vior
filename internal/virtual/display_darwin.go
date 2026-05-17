//go:build darwin

package virtual

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework Foundation

#include "display_darwin.h"
*/
import "C"
import (
	"fmt"
	"sync"
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

// Destroy tears down the virtual display created by Create.
func Destroy() {
	mu.Lock()
	defer mu.Unlock()
	C.vior_vd_destroy()
}
