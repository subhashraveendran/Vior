//go:build !darwin

package capture

import (
	"fmt"
	"image"

	"github.com/kbinani/screenshot"
)

// captureImagePlatform is the fallback for non-macOS platforms.
//
// The `bounds` from CaptureFrame carries only the pixel SIZE at origin
// (0,0) — correct on macOS, where CGDisplayCreateImage returns the
// target display's own image. But screenshot.CaptureRect interprets
// bounds.Min as ABSOLUTE virtual-desktop coordinates: a secondary
// monitor's origin is non-zero (Windows BitBlt x/y, X11 Xinerama
// offset). With origin forced to (0,0), every display index captured
// the primary display's top-left quadrant — i.e. extend/mirror of any
// non-primary display showed the wrong screen. Use the display's real
// absolute bounds instead so displayIndex is honored.
func captureImagePlatform(displayIndex int, bounds image.Rectangle) (*image.RGBA, error) {
	rect := screenshot.GetDisplayBounds(displayIndex)
	return screenshot.CaptureRect(rect)
}

// getPixelSize returns the display bounds dimensions (already pixel-level on non-Retina platforms).
func getPixelSize(displayIndex int) (int, int) {
	b := screenshot.GetDisplayBounds(displayIndex)
	return b.Dx(), b.Dy()
}

// MirrorDisplay is a stub for non-macOS platforms.
func MirrorDisplay(sourceDisplayIndex, targetDisplayIndex int) error {
	return fmt.Errorf("mirroring not supported on this platform")
}

// UnmirrorDisplay is a stub for non-macOS platforms.
func UnmirrorDisplay(displayIndex int) error {
	return fmt.Errorf("unmirroring not supported on this platform")
}

// IsMirrored is a stub for non-macOS platforms.
func IsMirrored(displayIndex int) (bool, error) {
	return false, nil
}

// CheckScreenRecordingPermission is a no-op on non-macOS.
func CheckScreenRecordingPermission() error {
	return nil
}

// RequestScreenRecordingPermission is a no-op on non-macOS (no such
// gate exists); always reports granted.
func RequestScreenRecordingPermission() bool {
	return true
}

// FindDisplayIndexByID is a stub on non-macOS. Returns -1 (not found).
func FindDisplayIndexByID(displayID uint32) int {
	return -1
}

// getDisplayName returns a human-readable product name for the display, or "".
func getDisplayName(displayIndex int) string {
	return ""
}
