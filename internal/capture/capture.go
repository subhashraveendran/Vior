// Package capture handles screen capture and display enumeration.
package capture

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
)

// Display represents a connected display.
type Display struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	IsMain   bool   `json:"isMain"`
	Mirrored bool   `json:"mirrored"`
	Bounds   image.Rectangle
}

// ListDisplays returns all connected displays with actual pixel resolutions.
func ListDisplays() ([]Display, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("no active displays found")
	}

	displays := make([]Display, n)
	for i := 0; i < n; i++ {
		bounds := screenshot.GetDisplayBounds(i)
		w, h := displayPixelSize(i)
		if w == 0 || h == 0 {
			w, h = bounds.Dx(), bounds.Dy()
		}
		mirrored, _ := IsMirrored(i)
		displays[i] = Display{
			Index:    i,
			Name:     fmt.Sprintf("Display %d", i),
			Width:    w,
			Height:   h,
			IsMain:   i == 0,
			Mirrored: mirrored,
			Bounds:   bounds,
		}
	}
	return displays, nil
}

// CaptureFrame captures a single frame from the specified display and returns JPEG bytes.
func CaptureFrame(displayIndex int, quality int) ([]byte, error) {
	n := screenshot.NumActiveDisplays()
	if displayIndex < 0 || displayIndex >= n {
		return nil, fmt.Errorf("display index %d out of range (0-%d)", displayIndex, n-1)
	}

	// On macOS HiDPI (Retina + CGVirtualDisplay) screenshot.GetDisplayBounds
	// returns LOGICAL points, but our capture call needs the PIXEL rect of
	// the display. Without this override, a 1290×2796 virtual display
	// captured against a 645×1398 logical rect produces a top-left quadrant
	// crop that the mobile client then stretches over the full viewport.
	pw, ph := displayPixelSize(displayIndex)
	if pw == 0 || ph == 0 {
		b := screenshot.GetDisplayBounds(displayIndex)
		pw, ph = b.Dx(), b.Dy()
	}
	bounds := image.Rect(0, 0, pw, ph)
	img, err := captureImage(displayIndex, bounds)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, fmt.Errorf("jpeg encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

// Session manages continuous screen capture.
type Session struct {
	displayIndex int
	quality      int
	fps          int
	running      bool
	mu           sync.RWMutex
	stopCh       chan struct{}

	// FrameCh delivers captured JPEG frames to consumers.
	// Closed when the session is stopped.
	FrameCh chan []byte
}

// NewSession creates a new capture session.
func NewSession(displayIndex, quality, fps int) *Session {
	return &Session{
		displayIndex: displayIndex,
		quality:      quality,
		fps:          fps,
		FrameCh:      make(chan []byte, 4),
		stopCh:       make(chan struct{}),
	}
}

// Start begins continuous capture. Frames sent to FrameCh.
func (s *Session) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("session already running")
	}
	s.running = true
	s.mu.Unlock()

	interval := time.Second / time.Duration(s.fps)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(s.FrameCh)

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				frame, err := CaptureFrame(s.displayIndex, s.quality)
				if err != nil {
					log.Printf("capture error: %v", err)
					continue
				}
				select {
				case s.FrameCh <- frame:
				case <-s.stopCh:
					return
				default:
				}
			}
		}
	}()

	return nil
}

// Stop ends the capture session.
func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false

		// Drain FrameCh to unblock any blocked sends.
		select {
		case <-s.FrameCh:
		default:
		}
		// Explicit GC hint after high-volume allocations.
		runtime.GC()
	}
}

// IsRunning reports if capture is active.
func (s *Session) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// captureImage captures the display region into an *image.RGBA.
// Uses platform-specific implementation for proper memory management.
func captureImage(displayIndex int, bounds image.Rectangle) (*image.RGBA, error) {
	return captureImagePlatform(displayIndex, bounds)
}

// displayPixelSize returns the actual pixel dimensions of a display.
// On macOS with Retina, this differs from CGDisplayBounds (logical points).
func displayPixelSize(displayIndex int) (int, int) {
	return getPixelSize(displayIndex)
}
