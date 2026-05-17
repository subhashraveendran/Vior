package ui

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// SettingsScreen shows quality presets, touch options, about.
type SettingsScreen struct {
	th          *material.Theme
	backBtn     widget.Clickable
	qualityBtns [4]widget.Clickable
	activeQ     int // 0=Low, 1=Med, 2=High, 3=Ultra
	OnBack      func()
}

func NewSettingsScreen(th *material.Theme) *SettingsScreen {
	return &SettingsScreen{th: th, activeQ: 2}
}

func (s *SettingsScreen) Layout(gtx layout.Context) layout.Dimensions {
	if s.backBtn.Clicked(gtx) && s.OnBack != nil {
		s.OnBack()
	}
	for i := range s.qualityBtns {
		if s.qualityBtns[i].Clicked(gtx) {
			s.activeQ = i
		}
	}

	FillBg(gtx)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(52), Left: unit.Dp(20), Right: unit.Dp(20), Bottom: unit.Dp(14)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						// Back.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Clickable(gtx, &s.backBtn, func(gtx layout.Context) layout.Dimensions {
								sz := gtx.Dp(unit.Dp(36))
								RRect(gtx, ColSurface, sz, sz, 18)
								return layout.Inset{Top: unit.Dp(7), Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									l := material.Body1(s.th, "‹")
									l.Color = ColText
									l.TextSize = unit.Sp(20)
									return l.Layout(gtx)
								})
							})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							l := material.H6(s.th, "Settings")
							l.Color = ColHead
							l.Font.Weight = font.Bold
							l.TextSize = unit.Sp(20)
							return l.Layout(gtx)
						}),
					)
				})
		}),
		layout.Rigid(Divider),

		// Content.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// Quality section.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return sectionLabel(gtx, s.th, "QUALITY")
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.qualityGrid(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

						// Control section.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return sectionLabel(gtx, s.th, "CONTROL")
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return settingRow(gtx, s.th, "Touch sensitivity", "Standard")
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return settingRow(gtx, s.th, "Auto-reconnect", "On")
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

						// About section.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return sectionLabel(gtx, s.th, "ABOUT")
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return aboutCard(gtx, s.th)
						}),
					)
				})
		}),
	)
}

func (s *SettingsScreen) qualityGrid(gtx layout.Context) layout.Dimensions {
	type preset struct {
		name, sub string
	}
	presets := []preset{
		{"Low", "480p · 30 fps"},
		{"Medium", "720p · 30 fps"},
		{"High", "1080p · 60 fps"},
		{"Ultra", "Native · 60 fps"},
	}

	halfW := (gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8))) / 2

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Row 1.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = halfW
					gtx.Constraints.Min.X = halfW
					return qualityCard(gtx, s.th, &s.qualityBtns[0], presets[0].name, presets[0].sub, s.activeQ == 0)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = halfW
					gtx.Constraints.Min.X = halfW
					return qualityCard(gtx, s.th, &s.qualityBtns[1], presets[1].name, presets[1].sub, s.activeQ == 1)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		// Row 2.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = halfW
					gtx.Constraints.Min.X = halfW
					return qualityCard(gtx, s.th, &s.qualityBtns[2], presets[2].name, presets[2].sub, s.activeQ == 2)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = halfW
					gtx.Constraints.Min.X = halfW
					return qualityCard(gtx, s.th, &s.qualityBtns[3], presets[3].name, presets[3].sub, s.activeQ == 3)
				}),
			)
		}),
	)
}

func qualityCard(gtx layout.Context, th *material.Theme, btn *widget.Clickable, name, sub string, active bool) layout.Dimensions {
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		h := gtx.Dp(unit.Dp(60))

		bg := ColSurf2
		if active {
			bg = ColAlpha(ColIndigo, 25)
		}
		RRect(gtx, bg, w, h, 10)

		// Border.
		if active {
			paint.FillShape(gtx.Ops, ColAlpha(ColIndigo, 100),
				clip.Stroke{Path: clip.RRect{
					Rect: image.Rectangle{Max: image.Pt(w, h)},
					NE: 10, NW: 10, SE: 10, SW: 10,
				}.Path(gtx.Ops), Width: 1.5}.Op())
		}

		return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						c := ColHead
						if active {
							c = ColIndigo2
						}
						l := material.Body1(th, name)
						l.Color = c
						l.Font.Weight = font.SemiBold
						l.TextSize = unit.Sp(13)
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(th, sub)
						l.Color = ColDim
						l.TextSize = unit.Sp(10.5)
						return l.Layout(gtx)
					}),
				)
			})
	})
}

func sectionLabel(gtx layout.Context, th *material.Theme, text string) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(4), Bottom: unit.Dp(10)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(th, text)
			l.Color = ColDim
			l.TextSize = unit.Sp(10.5)
			l.Font.Weight = font.SemiBold
			return l.Layout(gtx)
		})
}

func settingRow(gtx layout.Context, th *material.Theme, label, detail string) layout.Dimensions {
	w := gtx.Constraints.Max.X
	h := gtx.Dp(unit.Dp(48))
	RRect(gtx, ColSurface, w, h, 10)

	return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(14), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(th, label)
					l.Color = ColText
					l.TextSize = unit.Sp(13.5)
					return l.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(th, detail)
					l.Color = ColDim
					l.TextSize = unit.Sp(12.5)
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(th, "›")
					l.Color = ColDim
					l.TextSize = unit.Sp(18)
					return l.Layout(gtx)
				}),
			)
		})
}

func aboutCard(gtx layout.Context, th *material.Theme) layout.Dimensions {
	w := gtx.Constraints.Max.X
	h := gtx.Dp(unit.Dp(60))
	RRect(gtx, ColSurface, w, h, 10)

	return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				// Brand mark.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Dp(unit.Dp(32))
					RRect(gtx, ColIndigo, sz, sz, 8)
					return layout.Dimensions{Size: image.Pt(sz, sz)}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Body1(th, "Vior")
							l.Color = ColHead
							l.Font.Weight = font.SemiBold
							l.TextSize = unit.Sp(13.5)
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(th, "v0.1.0-dev")
							l.Color = ColDim
							l.TextSize = unit.Sp(11)
							return l.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(th, "›")
					l.Color = ColDim
					l.TextSize = unit.Sp(18)
					return l.Layout(gtx)
				}),
			)
		})
}

