// Package session contains shared client-connection logic used by both
// the desktop app (Wails) and the CLI server. It owns the capture-mode
// decision (extend vs mirror) and virtual-display lifecycle.
package session

import (
	"errors"
	"fmt"
	"image"
	"log"
	"time"

	"github.com/subhashraveendran/vior/internal/capture"
	"github.com/subhashraveendran/vior/internal/config"
	"github.com/subhashraveendran/vior/internal/protocol"
	"github.com/subhashraveendran/vior/internal/virtual"
)

// Setup describes the active capture session after a client connects.
type Setup struct {
	Mode          string          // "extend" or "mirror"
	DisplayIndex  int             // index in capture.ListDisplays()
	DisplayBounds image.Rectangle // absolute screen coords of captured display
	Width         int             // capture resolution width
	Height        int             // capture resolution height
}

// Configure tears down any prior virtual display and prepares capture for
// a newly-connected client based on the client's hello message.
//
// Mirror mode: captures the main display directly, no virtual display created.
// Extend mode: creates a new virtual display matching the client's resolution.
// SkipDisplay (or Intent "remote"/"files"): no virtual display, no capture —
//
//	returns a Setup with Mode="none" and DisplayBounds pointing at the main
//	display so the touch mapper (Remote intent) still has a valid target.
//
// Caller is responsible for stopping the previous capture session before calling.
func Configure(hello *protocol.HelloMessage) (*Setup, error) {
	// Resolve skip from explicit flag OR derive from intent. Keeps
	// older clients (no intent field) working as before.
	skip := hello.SkipDisplay || hello.Intent == "remote" || hello.Intent == "files"

	mode := hello.Mode
	if mode == "" {
		mode = "extend"
	}

	// No-display path: no permission check required for capture, no
	// virtual display, no capture session. Return a Setup whose bounds
	// match the main display so input injection still has a target.
	if skip {
		// Tear down any previous virtual display before returning.
		virtual.Destroy()

		displays, err := capture.ListDisplays()
		if err != nil {
			return nil, fmt.Errorf("list displays: %w", err)
		}
		mainIdx := 0
		for i, d := range displays {
			if d.IsMain {
				mainIdx = i
				break
			}
		}
		return &Setup{
			Mode:          "none",
			DisplayIndex:  mainIdx,
			DisplayBounds: displays[mainIdx].Bounds,
			Width:         displays[mainIdx].Width,
			Height:        displays[mainIdx].Height,
		}, nil
	}

	if err := capture.CheckScreenRecordingPermission(); err != nil {
		return nil, fmt.Errorf("permission denied: %w", err)
	}

	// Always start from a clean slate.
	virtual.Destroy()

	var captureIdx, resW, resH int

	if mode == "mirror" {
		displays, err := capture.ListDisplays()
		if err != nil {
			return nil, fmt.Errorf("list displays: %w", err)
		}
		captureIdx = 0
		for i, d := range displays {
			if d.IsMain {
				captureIdx = i
				break
			}
		}
		resW = displays[captureIdx].Width
		resH = displays[captureIdx].Height
	} else {
		info := virtual.Info{
			Width:       uint32(hello.Width),
			Height:      uint32(hello.Height),
			RefreshRate: config.DefaultRefreshRate,
		}
		displayID, err := virtual.CreateVirtualDisplay(info)
		if err != nil {
			// Headless / Linux without xf86-video-dummy / Windows
			// without a virtual display driver: extend mode is
			// unavailable. Return a clean user-friendly error rather
			// than crashing the WS session — the desktop frontend
			// surfaces this to the user as "extend mode unavailable,
			// use mirror".
			if errors.Is(err, virtual.ErrUnsupported) {
				return nil, fmt.Errorf("extend mode unavailable on this host (no virtual display backend); choose mirror mode instead")
			}
			return nil, fmt.Errorf("create virtual display: %w", err)
		}
		// Wait for the OS to register the new display.
		time.Sleep(500 * time.Millisecond)

		vdIdx := capture.FindDisplayIndexByID(displayID)
		if vdIdx < 0 {
			displays, err := capture.ListDisplays()
			if err != nil {
				return nil, fmt.Errorf("list displays: %w", err)
			}
			vdIdx = len(displays) - 1
			log.Printf("session: display ID %d not found, falling back to last index %d", displayID, vdIdx)
		}
		if err := capture.UnmirrorDisplay(vdIdx); err != nil {
			log.Printf("session: extend warning: %v", err)
		}
		captureIdx = vdIdx
		resW = hello.Width
		resH = hello.Height
	}

	displays, err := capture.ListDisplays()
	if err != nil {
		return nil, fmt.Errorf("list displays: %w", err)
	}
	if captureIdx >= len(displays) {
		return nil, fmt.Errorf("display index %d out of range", captureIdx)
	}

	return &Setup{
		Mode:          mode,
		DisplayIndex:  captureIdx,
		DisplayBounds: displays[captureIdx].Bounds,
		Width:         resW,
		Height:        resH,
	}, nil
}
