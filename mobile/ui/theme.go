// Package ui provides Gio-based UI screens for Vior mobile client.
package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget/material"
)

// Design tokens.
var (
	ColBg      = color.NRGBA{R: 12, G: 13, B: 18, A: 255}
	ColSurface = color.NRGBA{R: 20, G: 22, B: 28, A: 255}
	ColSurf2   = color.NRGBA{R: 24, G: 26, B: 34, A: 255}
	ColBorder  = color.NRGBA{R: 30, G: 32, B: 41, A: 255}
	ColBorderH = color.NRGBA{R: 38, G: 40, B: 51, A: 255}
	ColText    = color.NRGBA{R: 201, G: 205, B: 211, A: 255}
	ColDim     = color.NRGBA{R: 92, G: 97, B: 112, A: 255}
	ColHead    = color.NRGBA{R: 240, G: 241, B: 243, A: 255}
	ColIndigo  = color.NRGBA{R: 99, G: 102, B: 241, A: 255}
	ColIndigo2 = color.NRGBA{R: 129, G: 140, B: 248, A: 255}
	ColGreen   = color.NRGBA{R: 52, G: 211, B: 153, A: 255}
	ColRed     = color.NRGBA{R: 239, G: 68, B: 68, A: 255}
	ColWhite   = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	ColBlack   = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
)

// NewTheme creates dark Vior theme.
func NewTheme() *material.Theme {
	th := material.NewTheme()
	th.Palette.Bg = ColBg
	th.Palette.Fg = ColText
	th.Palette.ContrastBg = ColIndigo
	th.Palette.ContrastFg = ColWhite
	th.Shaper = &text.Shaper{}
	return th
}

// ── Drawing helpers ─────────────────────────────────────────────────

func FillBg(gtx layout.Context) {
	paint.FillShape(gtx.Ops, ColBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
}

func RRect(gtx layout.Context, c color.NRGBA, w, h, r int) {
	paint.FillShape(gtx.Ops, c, clip.RRect{
		Rect: image.Rectangle{Max: image.Pt(w, h)},
		NE: r, NW: r, SE: r, SW: r,
	}.Op(gtx.Ops))
}

func Divider(gtx layout.Context) layout.Dimensions {
	paint.FillShape(gtx.Ops, ColBorder, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
}

func Circle(gtx layout.Context, c color.NRGBA, sz int) layout.Dimensions {
	paint.FillShape(gtx.Ops, c, clip.Ellipse{Max: image.Pt(sz, sz)}.Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(sz, sz)}
}

// ColAlpha returns color with modified alpha.
func ColAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
