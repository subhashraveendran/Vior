// Package capture handles screen capture and display enumeration.
package capture

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"log"
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
		name := fmt.Sprintf("Display %d", i)
		if dn := getDisplayName(i); dn != "" {
			name = dn
		}
		displays[i] = Display{
			Index:    i,
			Name:     name,
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
	captureDone  chan struct{} // closed when the capture goroutine exits

	// FrameCh delivers captured JPEG frames to consumers.
	// Closed when the session is stopped.
	FrameCh chan []byte
}

// NewSession creates a new capture session.
func NewSession(displayIndex, quality, fps int) *Session {
	if quality < 1 {
		quality = 80
	}
	if quality > 100 {
		quality = 100
	}
	if fps < 1 {
		fps = 30
	}
	return &Session{
		displayIndex: displayIndex,
		quality:      quality,
		fps:          fps,
		FrameCh:      make(chan []byte, 4),
		stopCh:       make(chan struct{}),
		captureDone:  make(chan struct{}),
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
		// Defers run LIFO: captureDone closes LAST (so a Stop() waiting
		// on it knows FrameCh is already closed), FrameCh closes before
		// that, and the recover runs first to keep a cgo-capture panic
		// from crashing the whole process.
		defer close(s.captureDone)
		defer close(s.FrameCh)
		defer ticker.Stop()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("capture: goroutine panic recovered: %v", r)
			}
		}()

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

// Stop ends the capture session and waits (bounded) for the capture
// goroutine to fully exit before returning, so a subsequent Start()
// can't run a second capture loop against the same display.
func (s *Session) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	close(s.stopCh)
	s.running = false
	done := s.captureDone
	// Release the lock BEFORE waiting. The goroutine never needs s.mu to
	// exit, but holding it here would block IsRunning and other callers
	// for a whole frame interval. The old code instead busy-looped
	// draining FrameCh under the lock — which spun forever once the
	// goroutine had already closed FrameCh (recv on a closed channel is
	// always ready, so the `default` branch was never taken).
	s.mu.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		// The capture goroutine is wedged in a cgo call; give up waiting
		// rather than hang shutdown. It will exit on its own and close
		// the channels then.
		log.Printf("capture: Stop timed out waiting for goroutine")
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
