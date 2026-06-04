// Package adb provides ADB (Android Debug Bridge) helpers for USB connectivity.
// It wraps the `adb` CLI tool to set up reverse port forwarding so that
// a USB-connected Android device can access the Vior server.
//
// If ADB is not found in PATH, it auto-downloads Google's official platform-tools
// to the app support directory (~/.vior/platform-tools/).
package adb

import (
	"fmt"
	"os/exec"
	"strings"
)

// Status reports ADB availability and device connection state.
type Status struct {
	Available  bool   `json:"available"`
	DeviceName string `json:"deviceName,omitempty"`
	Connected  bool   `json:"connected"`
	Forwarding bool   `json:"forwarding"`
	Bundled    bool   `json:"bundled"` // true if using Vior's bundled ADB
}

// adbPath returns the path to the adb binary.
// Checks bundled location first, then PATH.
func adbPath() string {
	// Check bundled ADB first.
	bundled := bundledADBPath()
	if bundled != "" {
		return bundled
	}
	// Fallback to system PATH.
	p, err := exec.LookPath("adb")
	if err != nil {
		return ""
	}
	return p
}

// Check verifies that adb is available and a device is connected.
func Check() Status {
	s := Status{}

	path := adbPath()
	if path == "" {
		return s
	}
	s.Available = true
	s.Bundled = isBundled(path)

	// Check for connected devices.
	out, err := exec.Command(path, "devices").Output()
	if err != nil {
		return s
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines[1:] { // skip header
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == "device" {
			s.Connected = true
			break
		}
	}

	if s.Connected {
		s.DeviceName = deviceModel(path)
		s.Forwarding = isForwarding(path)
	}

	return s
}

// SetupForward runs `adb reverse tcp:<remotePort> tcp:<localPort>`.
func SetupForward(localPort, remotePort int) error {
	path := adbPath()
	if path == "" {
		return fmt.Errorf("adb not available")
	}
	cmd := exec.Command(path, "reverse",
		fmt.Sprintf("tcp:%d", remotePort),
		fmt.Sprintf("tcp:%d", localPort))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb reverse failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// TeardownForward removes the reverse port forwarding.
func TeardownForward(remotePort int) error {
	path := adbPath()
	if path == "" {
		return fmt.Errorf("adb not available")
	}
	cmd := exec.Command(path, "reverse", "--remove", fmt.Sprintf("tcp:%d", remotePort))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb reverse remove failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// EnsureADB downloads ADB if not available. Returns nil if already available.
func EnsureADB() error {
	if adbPath() != "" {
		return nil // Already available.
	}
	return downloadPlatformTools()
}

// IsAvailable returns true if ADB is available (system or bundled).
func IsAvailable() bool {
	return adbPath() != ""
}

func deviceModel(path string) string {
	out, err := exec.Command(path, "shell", "getprop", "ro.product.model").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isForwarding(path string) bool {
	out, err := exec.Command(path, "reverse", "--list").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
