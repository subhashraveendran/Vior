//go:build !darwin

package capture

import (
	"fmt"
	"image"

	"github.com/kbinani/screenshot"
)

// captureImagePlatform is the fallback for non-macOS platforms.
func captureImagePlatform(displayIndex int, bounds image.Rectangle) (*image.RGBA, error) {
	return screenshot.CaptureRect(bounds)
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

// FindDisplayIndexByID is a stub on non-macOS. Returns -1 (not found).
func FindDisplayIndexByID(displayID uint32) int {
	return -1
}
