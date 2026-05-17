package ui

import (
	"image"
	"sync"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/subhashraveendran/vior/mobile/client"
	"github.com/subhashraveendran/vior/mobile/mjpeg"
)

// StreamScreen renders the MJPEG stream fullscreen and captures touch events.
type StreamScreen struct {
	th     *material.Theme
	client *client.ViorClient

	decoder  *mjpeg.Decoder
	frame    image.Image
	frameMu  sync.RWMutex
	frameOp  paint.ImageOp
	hasFrame bool

	// Display dimensions from server.
	displayWidth  int
	displayHeight int

	// Back button.
	backBtn widget.Clickable
	OnBack  func()

	// Status.
	statusText string
	statusMu   sync.RWMutex

	// Invalidator to trigger redraws on new frames.
	invalidate chan struct{}
}

// NewStreamScreen creates a stream viewer.
func NewStreamScreen(th *material.Theme, c *client.ViorClient) *StreamScreen {
	return &StreamScreen{
		th:         th,
		client:     c,
		invalidate: make(chan struct{}, 1),
	}
}

// StartStream begins fetching the MJPEG stream.
func (s *StreamScreen) StartStream(streamURL string, displayWidth, displayHeight int) {
	s.displayWidth = displayWidth
	s.displayHeight = displayHeight

	s.decoder = mjpeg.NewDecoder(streamURL)
	s.decoder.OnFrame = func(img image.Image) {
		s.frameMu.Lock()
		s.frame = img
		s.hasFrame = true
		s.frameMu.Unlock()

		// Signal UI to redraw.
		select {
		case s.invalidate <- struct{}{}:
		default:
		}
	}
	s.decoder.Start()

	s.setStatus("Streaming")
}

// StopStream stops the MJPEG stream.
func (s *StreamScreen) StopStream() {
	if s.decoder != nil {
		s.decoder.Stop()
		s.decoder = nil
	}
}

// Layout draws the stream screen.
func (s *StreamScreen) Layout(gtx layout.Context) layout.Dimensions {
	// Check back button.
	if s.backBtn.Clicked(gtx) {
		s.StopStream()
		if s.OnBack != nil {
			s.OnBack()
		}
	}

	// Fill black background.
	paint.FillShape(gtx.Ops, ColorBlack, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Handle touch events.
	s.handlePointerEvents(gtx)

	// Draw frame.
	s.drawFrame(gtx)

	// Draw back button overlay.
	s.drawOverlay(gtx)

	// If we have frames, request continuous redraws.
	if s.hasFrame {
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (s *StreamScreen) drawFrame(gtx layout.Context) {
	s.frameMu.RLock()
	frame := s.frame
	s.frameMu.RUnlock()

	if frame == nil {
		// Show "Connecting to stream..." text.
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return Caption(s.th, "Connecting to stream...").Layout(gtx)
		})
		return
	}

	// Calculate aspect-fit dimensions.
	imgBounds := frame.Bounds()
	imgW := float32(imgBounds.Dx())
	imgH := float32(imgBounds.Dy())
	viewW := float32(gtx.Constraints.Max.X)
	viewH := float32(gtx.Constraints.Max.Y)

	scaleX := viewW / imgW
	scaleY := viewH / imgH
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}

	drawW := imgW * scale
	drawH := imgH * scale
	offsetX := (viewW - drawW) / 2
	offsetY := (viewH - drawH) / 2

	// Create image op from frame.
	s.frameOp = paint.NewImageOp(frame)

	stack := op.Offset(image.Pt(int(offsetX), int(offsetY))).Push(gtx.Ops)
	clip.Rect{Max: image.Pt(int(drawW), int(drawH))}.Push(gtx.Ops).Pop()

	s.frameOp.Filter = paint.FilterLinear
	s.frameOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	stack.Pop()
}

func (s *StreamScreen) handlePointerEvents(gtx layout.Context) {
	// Register for pointer events on the full screen area.
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	area.Pop()

	// Process events.
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: s,
			Kinds:  pointer.Press | pointer.Release | pointer.Drag,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}

		// Map screen coordinates to virtual display coordinates.
		x, y := s.mapCoords(gtx, pe.Position.X, pe.Position.Y)

		switch pe.Kind {
		case pointer.Press:
			s.client.SendInput("touch", "down", x, y)
		case pointer.Drag:
			s.client.SendInput("touch", "move", x, y)
		case pointer.Release:
			s.client.SendInput("touch", "up", x, y)
		}
	}
}

func (s *StreamScreen) mapCoords(gtx layout.Context, screenX, screenY float32) (float64, float64) {
	if s.displayWidth == 0 || s.displayHeight == 0 {
		return float64(screenX), float64(screenY)
	}

	s.frameMu.RLock()
	frame := s.frame
	s.frameMu.RUnlock()
	if frame == nil {
		return float64(screenX), float64(screenY)
	}

	// Calculate the same aspect-fit transform used in drawFrame.
	imgBounds := frame.Bounds()
	imgW := float32(imgBounds.Dx())
	imgH := float32(imgBounds.Dy())
	viewW := float32(gtx.Constraints.Max.X)
	viewH := float32(gtx.Constraints.Max.Y)

	scaleX := viewW / imgW
	scaleY := viewH / imgH
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}

	drawW := imgW * scale
	drawH := imgH * scale
	offsetX := (viewW - drawW) / 2
	offsetY := (viewH - drawH) / 2

	// Map from screen coords to image coords, then to display coords.
	relX := (screenX - offsetX) / drawW
	relY := (screenY - offsetY) / drawH

	// Clamp to [0, 1].
	if relX < 0 {
		relX = 0
	}
	if relX > 1 {
		relX = 1
	}
	if relY < 0 {
		relY = 0
	}
	if relY > 1 {
		relY = 1
	}

	return float64(relX) * float64(s.displayWidth), float64(relY) * float64(s.displayHeight)
}

func (s *StreamScreen) drawOverlay(gtx layout.Context) {
	// Small back button in top-left.
	layout.Inset{Top: unit.Dp(40), Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(s.th, &s.backBtn, "←")
		btn.Background = ColorBg
		btn.Color = ColorTextDim
		btn.CornerRadius = unit.Dp(20)
		btn.TextSize = unit.Sp(18)
		gtx.Constraints.Max.X = gtx.Dp(unit.Dp(40))
		gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(40))
		return btn.Layout(gtx)
	})
}

func (s *StreamScreen) setStatus(text string) {
	s.statusMu.Lock()
	s.statusText = text
	s.statusMu.Unlock()
}
