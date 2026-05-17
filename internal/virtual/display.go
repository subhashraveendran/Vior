// Package virtual provides cross-platform virtual display creation.
// On macOS (Apple Silicon + Intel), it uses the CGVirtualDisplay private API.
// On other platforms, it returns ErrUnsupported.
package virtual

import "errors"

// ErrUnsupported is returned when virtual display creation is not available
// on the current platform.
var ErrUnsupported = errors.New("virtual display creation not supported on this platform")

// Info describes a virtual display configuration to create.
type Info struct {
	Width       uint32
	Height      uint32
	RefreshRate float64
	HiDPI       bool
}

// DefaultForResolution returns config matching a target device resolution.
// For example, an iPhone 15 Pro (1179×2556) → virtual display at 393×852 logical.
func DefaultForResolution(width, height uint32) Info {
	return Info{
		Width:       width,
		Height:      height,
		RefreshRate: 60,
		HiDPI:       false,
	}
}

// CreateVirtualDisplay creates a virtual display suitable for streaming to a
// phone or tablet, matching the target device's dimensions.
//
// On macOS it uses the CGVirtualDisplay private API (stable since Ventura).
// On Linux it returns ErrUnsupported.
// On Windows it returns ErrUnsupported.
//
// Returns the native display ID that can be passed to capture.CaptureFrame.
func CreateVirtualDisplay(info Info) (uint32, error) {
	if info.HiDPI {
		return CreateHiDPI(info.Width, info.Height, info.RefreshRate)
	}
	return Create(info.Width, info.Height, info.RefreshRate)
}
