// Package mjpeg decodes an MJPEG HTTP stream into individual image frames.
package mjpeg

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"sync"
)

// Decoder reads an MJPEG stream and delivers decoded frames.
type Decoder struct {
	url     string
	resp    *http.Response
	frame   image.Image
	frameMu sync.RWMutex
	stop    chan struct{}
	running bool
	mu      sync.Mutex

	// OnFrame is called each time a new frame is decoded.
	OnFrame func(image.Image)
}

// NewDecoder creates a decoder for the given MJPEG stream URL.
func NewDecoder(url string) *Decoder {
	return &Decoder{
		url:  url,
		stop: make(chan struct{}),
	}
}

// Start begins fetching and decoding the MJPEG stream in a goroutine.
func (d *Decoder) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return fmt.Errorf("already running")
	}

	resp, err := http.Get(d.url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	d.resp = resp
	d.running = true

	go d.readLoop()
	return nil
}

// Stop stops the decoder.
func (d *Decoder) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running {
		return
	}
	d.running = false
	close(d.stop)
	if d.resp != nil {
		d.resp.Body.Close()
	}
}

// Frame returns the latest decoded frame.
func (d *Decoder) Frame() image.Image {
	d.frameMu.RLock()
	defer d.frameMu.RUnlock()
	return d.frame
}

func (d *Decoder) readLoop() {
	reader := bufio.NewReaderSize(d.resp.Body, 512*1024)

	for {
		select {
		case <-d.stop:
			return
		default:
		}

		// Find JPEG start marker (FFD8).
		jpegData, err := readJPEGFrame(reader)
		if err != nil {
			return
		}

		img, err := jpeg.Decode(bytes.NewReader(jpegData))
		if err != nil {
			continue
		}

		d.frameMu.Lock()
		d.frame = img
		d.frameMu.Unlock()

		if d.OnFrame != nil {
			d.OnFrame(img)
		}
	}
}

// readJPEGFrame extracts a single JPEG frame from an MJPEG multipart stream.
// It reads until it finds a JPEG SOI marker (0xFFD8) and then reads until EOI (0xFFD9).
func readJPEGFrame(r *bufio.Reader) ([]byte, error) {
	// Skip until we find Content-Type or JPEG SOI marker.
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 0xFF {
			next, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if next == 0xD8 {
				// Found SOI — start of JPEG.
				return readUntilEOI(r)
			}
			// Not SOI, continue scanning.
		}
	}
}

// readUntilEOI reads JPEG data until the EOI marker (0xFFD9).
func readUntilEOI(r *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	// Write SOI marker.
	buf.Write([]byte{0xFF, 0xD8})

	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				return buf.Bytes(), nil
			}
			return nil, err
		}
		buf.WriteByte(b)

		if b == 0xFF {
			next, err := r.ReadByte()
			if err != nil {
				return buf.Bytes(), nil
			}
			buf.WriteByte(next)
			if next == 0xD9 {
				// Found EOI — complete frame.
				return buf.Bytes(), nil
			}
		}
	}
}
