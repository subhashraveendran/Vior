package ui

import (
	"image"
	"image/color"
	"sync"

	"gioui.org/font"
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

// StreamScreen renders MJPEG fullscreen with overlay controls.
type StreamScreen struct {
	th     *material.Theme
	client *client.ViorClient

	decoder  *mjpeg.Decoder
	frame    image.Image
	frameMu  sync.RWMutex
	frameOp  paint.ImageOp
	hasFrame bool

	displayWidth  int
	displayHeight int

	backBtn widget.Clickable
	OnBack  func()

	statusText string
	statusMu   sync.RWMutex
	invalidate chan struct{}
}

func NewStreamScreen(th *material.Theme, c *client.ViorClient) *StreamScreen {
	return &StreamScreen{th: th, client: c, invalidate: make(chan struct{}, 1)}
}

func (s *StreamScreen) StartStream(streamURL string, dw, dh int) {
	s.displayWidth = dw
	s.displayHeight = dh
	s.decoder = mjpeg.NewDecoder(streamURL)
	s.decoder.OnFrame = func(img image.Image) {
		s.frameMu.Lock()
		s.frame = img
		s.hasFrame = true
		s.frameMu.Unlock()
		select {
		case s.invalidate <- struct{}{}:
		default:
		}
	}
	s.decoder.Start()
	s.setStatus("Connected")
}

func (s *StreamScreen) StopStream() {
	if s.decoder != nil {
		s.decoder.Stop()
		s.decoder = nil
	}
}

func (s *StreamScreen) Layout(gtx layout.Context) layout.Dimensions {
	if s.backBtn.Clicked(gtx) {
		s.StopStream()
		if s.OnBack != nil {
			s.OnBack()
		}
	}

	// Black bg.
	paint.FillShape(gtx.Ops, ColBlack, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Touch events.
	s.handlePointer(gtx)

	// Draw frame.
	s.drawFrame(gtx)

	// Overlay controls.
	s.drawOverlay(gtx)

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
		// Loading state.
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Body2(s.th, "Connecting to stream...")
			l.Color = ColDim
			return l.Layout(gtx)
		})
		return
	}

	bounds := frame.Bounds()
	imgW := float32(bounds.Dx())
	imgH := float32(bounds.Dy())
	viewW := float32(gtx.Constraints.Max.X)
	viewH := float32(gtx.Constraints.Max.Y)

	scale := viewW / imgW
	if s2 := viewH / imgH; s2 < scale {
		scale = s2
	}

	drawW := imgW * scale
	drawH := imgH * scale
	offsetX := (viewW - drawW) / 2
	offsetY := (viewH - drawH) / 2

	s.frameOp = paint.NewImageOp(frame)
	stack := op.Offset(image.Pt(int(offsetX), int(offsetY))).Push(gtx.Ops)
	clip.Rect{Max: image.Pt(int(drawW), int(drawH))}.Push(gtx.Ops).Pop()
	s.frameOp.Filter = paint.FilterLinear
	s.frameOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	stack.Pop()
}

func (s *StreamScreen) handlePointer(gtx layout.Context) {
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	area.Pop()

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

func (s *StreamScreen) mapCoords(gtx layout.Context, sx, sy float32) (float64, float64) {
	if s.displayWidth == 0 || s.displayHeight == 0 {
		return float64(sx), float64(sy)
	}
	s.frameMu.RLock()
	frame := s.frame
	s.frameMu.RUnlock()
	if frame == nil {
		return float64(sx), float64(sy)
	}

	bounds := frame.Bounds()
	imgW := float32(bounds.Dx())
	imgH := float32(bounds.Dy())
	viewW := float32(gtx.Constraints.Max.X)
	viewH := float32(gtx.Constraints.Max.Y)

	scale := viewW / imgW
	if s2 := viewH / imgH; s2 < scale {
		scale = s2
	}
	drawW := imgW * scale
	drawH := imgH * scale
	offX := (viewW - drawW) / 2
	offY := (viewH - drawH) / 2

	relX := clamp01((sx - offX) / drawW)
	relY := clamp01((sy - offY) / drawH)
	return float64(relX) * float64(s.displayWidth), float64(relY) * float64(s.displayHeight)
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (s *StreamScreen) drawOverlay(gtx layout.Context) {
	maxW := gtx.Constraints.Max.X
	maxH := gtx.Constraints.Max.Y

	// Back button (top-left).
	layout.Inset{Top: unit.Dp(52), Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, &s.backBtn, func(gtx layout.Context) layout.Dimensions {
			sz := gtx.Dp(unit.Dp(40))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 120},
				clip.Ellipse{Max: image.Pt(sz, sz)}.Op(gtx.Ops))
			// Arrow.
			return layout.Inset{Top: unit.Dp(10), Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body1(s.th, "‹")
				l.Color = ColWhite
				l.TextSize = unit.Sp(20)
				return l.Layout(gtx)
			})
		})
	})

	// Status pill (bottom-center).
	pillW := gtx.Dp(unit.Dp(160))
	pillH := gtx.Dp(unit.Dp(36))
	pillX := (maxW - pillW) / 2
	pillY := maxH - gtx.Dp(unit.Dp(60))

	stack := op.Offset(image.Pt(pillX, pillY)).Push(gtx.Ops)
	// Pill bg.
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 140},
		clip.RRect{
			Rect: image.Rectangle{Max: image.Pt(pillW, pillH)},
			NE: pillH / 2, NW: pillH / 2, SE: pillH / 2, SW: pillH / 2,
		}.Op(gtx.Ops))

	// Green dot + "Connected" + FPS.
	layout.Inset{Top: unit.Dp(8), Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return Circle(gtx, ColGreen, gtx.Dp(unit.Dp(7)))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(s.th, "Connected")
				l.Color = ColWhite
				l.Font.Weight = font.SemiBold
				l.TextSize = unit.Sp(12)
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(s.th, "30 fps")
				l.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 200}
				l.TextSize = unit.Sp(11)
				return l.Layout(gtx)
			}),
		)
	})
	stack.Pop()
}

func (s *StreamScreen) setStatus(t string) {
	s.statusMu.Lock()
	s.statusText = t
	s.statusMu.Unlock()
}
