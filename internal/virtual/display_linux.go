//go:build linux

package virtual

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// cleanupPath is where the virtual display config is saved so Destroy()
// can tear it down without guessing.
var cleanupPath = filepath.Join(os.TempDir(), "vior-display.cleanup")

// Create creates a virtual display on Linux by configuring a dummy X11 output.
// Requires xf86-video-dummy or a similar virtual output driver to be configured.
// See SetupXorgDummyConfig() to generate the necessary xorg.conf.d snippet.
func Create(width, height uint32, refreshRate float64) (uint32, error) {
	// Find an unused/disconnected output that can be used as virtual.
	output, err := findAvailableOutput()
	if err != nil {
		// Headless box or no dummy driver — surface ErrUnsupported so
		// the session layer can fall back to mirror with a clean
		// message instead of crashing the WS handler.
		return 0, fmt.Errorf("%w: %v (install xf86-video-dummy and run 'vior virtual setup')", ErrUnsupported, err)
	}

	modeName := fmt.Sprintf("vior_%dx%d", width, height)

	// Create a new mode line and add it to the output.
	modeline := generateModeline(modeName, width, height, refreshRate)
	newmodeArgs := append([]string{"--newmode"}, modeline...)
	if err := xrandr(newmodeArgs...); err != nil {
		return 0, fmt.Errorf("failed to create mode: %w", err)
	}

	if err := xrandr("--addmode", output, modeName); err != nil {
		return 0, fmt.Errorf("failed to add mode to output %s: %w", output, err)
	}

	// Enable the output with the new mode.
	if err := xrandr("--output", output, "--mode", modeName); err != nil {
		return 0, fmt.Errorf("failed to enable output %s: %w", output, err)
	}

	// Return the output index as a uint32. X11 outputs are string names,
	// but we encode the output name hash as a numeric ID for the capture layer.
	id := outputHash(output)

	// Persist cleanup info so Destroy() knows what to tear down.
	cleanup := fmt.Sprintf("%s|%s", output, modeName)
	_ = os.WriteFile(cleanupPath, []byte(cleanup), 0o600)

	return id, nil
}

// CreateHiDPI creates a virtual HiDPI (2x) display on Linux.
func CreateHiDPI(logicalWidth, logicalHeight uint32, refreshRate float64) (uint32, error) {
	return Create(logicalWidth*2, logicalHeight*2, refreshRate)
}

// Destroy disables the virtual display output and removes the created mode.
func Destroy() {
	// If we have cleanup info, use it.
	data, err := os.ReadFile(cleanupPath)
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(data)), "|", 2)
		if len(parts) == 2 {
			_ = xrandr("--output", parts[0], "--off")
			_ = xrandr("--delmode", parts[0], parts[1])
			_ = xrandr("--rmmode", parts[1])
		}
		_ = os.Remove(cleanupPath)
	}
}

// findAvailableOutput finds a disconnected output that can be used as virtual.
// Prefers VIRTUAL* outputs (xf86-video-dummy) over physical disconnected ports
// like HDMI-1, so plugging a real monitor doesn't conflict.
func findAvailableOutput() (string, error) {
	out := xrandrOutput("--query")
	if out == "" {
		return "", fmt.Errorf("xrandr not available")
	}

	re := regexp.MustCompile(`^(\S+)\s+disconnected`)
	lines := strings.Split(out, "\n")
	var fallback string
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 {
			name := matches[1]
			if strings.HasPrefix(strings.ToUpper(name), "VIRTUAL") {
				return name, nil
			}
			// Hold the first physical disconnected output as fallback.
			if fallback == "" {
				fallback = name
			}
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no disconnected outputs found (install xf86-video-dummy)")
}

// outputHash converts an X11 output name to a stable uint32 ID.
func outputHash(name string) uint32 {
	var h uint32
	for _, c := range name {
		h = h*31 + uint32(c)
	}
	return h
}

// xrandr runs xrandr with the given arguments and returns stdout.
func xrandr(args ...string) error {
	cmd := exec.Command("xrandr", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xrandr %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// xrandrOutput runs xrandr and returns stdout as string.
func xrandrOutput(args ...string) string {
	cmd := exec.Command("xrandr", args...)
	out, _ := cmd.Output()
	return string(out)
}

// generateModeline computes an X11 modeline for the given resolution and refresh.
// Uses the CVT standard formula for a reduced-blanking mode.
func generateModeline(name string, width, height uint32, refresh float64) []string {
	h := width
	v := height

	// CVT reduced blanking (simplified).
	hTotal := float64(h) * 1.1
	vTotal := float64(v) * 1.05
	pixelClock := hTotal * vTotal * refresh / 1000000.0

	hSyncStart := float64(h)
	hSyncEnd := hSyncStart + hTotal*0.05
	vSyncStart := float64(v)
	vSyncEnd := vSyncStart + vTotal*0.01

	return []string{
		name,
		strconv.FormatFloat(pixelClock, 'f', 2, 64),
		strconv.FormatUint(uint64(h), 10),
		strconv.FormatUint(uint64(hSyncStart), 10),
		strconv.FormatUint(uint64(hSyncEnd), 10),
		strconv.FormatUint(uint64(hTotal), 10),
		strconv.FormatUint(uint64(v), 10),
		strconv.FormatUint(uint64(vSyncStart), 10),
		strconv.FormatUint(uint64(vSyncEnd), 10),
		strconv.FormatUint(uint64(vTotal), 10),
		"-HSync", "+VSync",
	}
}
