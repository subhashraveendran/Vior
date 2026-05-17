// Package ui provides Gio-based UI screens for the Vior mobile client.
package ui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Vior dark theme colors.
var (
	ColorBg        = color.NRGBA{R: 15, G: 17, B: 23, A: 255}     // #0f1117
	ColorSurface   = color.NRGBA{R: 26, G: 29, B: 37, A: 255}     // #1a1d25
	ColorBorder    = color.NRGBA{R: 45, G: 48, B: 57, A: 255}     // #2d3039
	ColorText      = color.NRGBA{R: 209, G: 213, B: 219, A: 255}  // #d1d5db
	ColorTextDim   = color.NRGBA{R: 107, G: 114, B: 128, A: 255}  // #6b7280
	ColorHeading   = color.NRGBA{R: 243, G: 244, B: 246, A: 255}  // #f3f4f6
	ColorPrimary   = color.NRGBA{R: 79, G: 70, B: 229, A: 255}    // #4f46e5
	ColorPrimaryLt = color.NRGBA{R: 165, G: 180, B: 252, A: 255}  // #a5b4fc
	ColorGreen     = color.NRGBA{R: 52, G: 211, B: 153, A: 255}   // #34d399
	ColorRed       = color.NRGBA{R: 239, G: 68, B: 68, A: 255}    // #ef4444
	ColorBlack     = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	ColorWhite     = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
)

// NewTheme creates the Vior material theme.
func NewTheme() *material.Theme {
	th := material.NewTheme()
	th.Palette.Bg = ColorBg
	th.Palette.Fg = ColorText
	th.Palette.ContrastBg = ColorPrimary
	th.Palette.ContrastFg = ColorWhite
	th.Shaper = &text.Shaper{}
	return th
}

// H1 returns a heading label.
func H1(th *material.Theme, txt string) material.LabelStyle {
	l := material.H5(th, txt)
	l.Color = ColorHeading
	l.Font.Weight = font.Bold
	return l
}

// Body returns a body text label.
func Body(th *material.Theme, txt string) material.LabelStyle {
	l := material.Body1(th, txt)
	l.Color = ColorText
	return l
}

// Caption returns a caption text label.
func Caption(th *material.Theme, txt string) material.LabelStyle {
	l := material.Caption(th, txt)
	l.Color = ColorTextDim
	return l
}

// PrimaryButton returns a styled primary button.
func PrimaryButton(th *material.Theme, btn *material.ButtonStyle) {
	btn.Background = ColorPrimary
	btn.Color = ColorWhite
	btn.CornerRadius = unit.Dp(10)
	btn.TextSize = unit.Sp(16)
	btn.Font.Weight = font.SemiBold
	btn.Inset = layout.UniformInset(unit.Dp(14))
}
