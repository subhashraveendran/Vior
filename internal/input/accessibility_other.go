//go:build !darwin

package input

// HasAccessibility is a no-op on non-macOS platforms. The Linux + Windows
// input controllers don't gate input injection behind an OS allow-list.
func HasAccessibility(_ bool) bool { return true }
