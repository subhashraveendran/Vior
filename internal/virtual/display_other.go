//go:build !darwin

package virtual

// Create is a stub for non-macOS platforms.
func Create(width, height uint32, refreshRate float64) (uint32, error) {
	return 0, ErrUnsupported
}

// CreateHiDPI is a stub for non-macOS platforms.
func CreateHiDPI(logicalWidth, logicalHeight uint32, refreshRate float64) (uint32, error) {
	return 0, ErrUnsupported
}

// Destroy is a stub for non-macOS platforms.
func Destroy() {}
