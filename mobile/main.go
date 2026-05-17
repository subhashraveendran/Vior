// Vior Mobile — companion app for Vior second display server.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/subhashraveendran/vior/internal/discovery"
	"github.com/subhashraveendran/vior/mobile/client"
	"github.com/subhashraveendran/vior/mobile/ui"
)

// Design tokens.
var (
	colBg      = color.NRGBA{R: 12, G: 13, B: 18, A: 255}
	colSurface = color.NRGBA{R: 20, G: 22, B: 28, A: 255}
	colSurf2   = color.NRGBA{R: 10, G: 11, B: 16, A: 255}
	colBorder  = color.NRGBA{R: 30, G: 32, B: 41, A: 255}
	colText    = color.NRGBA{R: 201, G: 205, B: 211, A: 255}
	colDim     = color.NRGBA{R: 92, G: 97, B: 112, A: 255}
	colHead    = color.NRGBA{R: 240, G: 241, B: 243, A: 255}
	colIndigo  = color.NRGBA{R: 99, G: 102, B: 241, A: 255}
	colWhite   = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
)

func main() {
	go run()
	app.Main()
}

func run() {
	w := new(app.Window)
	w.Option(app.Title("Vior"), app.Size(unit.Dp(400), unit.Dp(800)))

	th := material.NewTheme()
	th.Palette.Bg = colBg
	th.Palette.Fg = colText
	th.Palette.ContrastBg = colIndigo
	th.Palette.ContrastFg = colWhite

	var (
		screen     = "discover"
		viorClient *client.ViorClient
		streamScr  *ui.StreamScreen

		ipEditor   widget.Editor
		connectBtn widget.Clickable
		connectErr string
		servers    []discovery.Beacon
		serverBtns [8]widget.Clickable
		serversMu  sync.Mutex
		scanning   bool
		list       widget.List
	)

	ipEditor.SingleLine = true
	list.Axis = layout.Vertical

	// LAN scan loop.
	scanning = true
	go func() {
		for scanning {
			beacons, _ := discovery.Listen(3 * time.Second)
			serversMu.Lock()
			servers = beacons
			serversMu.Unlock()
			w.Invalidate()
		}
	}()

	doConnect := func(host string, port int) {
		connectErr = ""
		viorClient = client.New(host, port)
		viorClient.ScreenWidth = 1179
		viorClient.ScreenHeight = 2556
		viorClient.DPR = 3
		viorClient.DeviceName = "Vior Mobile"
		viorClient.OnReady = func(streamURL, resolution string) {
			parts := strings.Split(resolution, "x")
			dw, _ := strconv.Atoi(parts[0])
			dh, _ := strconv.Atoi(parts[1])
			streamScr = ui.NewStreamScreen(th, viorClient)
			streamScr.OnBack = func() {
				screen = "discover"
				viorClient.Disconnect()
			}
			streamScr.StartStream(fmt.Sprintf("http://%s:%d%s", host, port, streamURL), dw, dh)
			screen = "stream"
			w.Invalidate()
		}
		viorClient.OnDisconnect = func() { screen = "discover"; w.Invalidate() }
		viorClient.OnError = func(_, msg string) { connectErr = msg; w.Invalidate() }
		if err := viorClient.Connect(); err != nil {
			connectErr = err.Error()
		}
	}

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			scanning = false
			if viorClient != nil {
				viorClient.Disconnect()
			}
			if streamScr != nil {
				streamScr.StopStream()
			}
			if e.Err != nil {
				log.Fatal(e.Err)
			}
			os.Exit(0)

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			bg(gtx, colBg)

			switch screen {
			case "discover":
				serversMu.Lock()
				srvSnap := make([]discovery.Beacon, len(servers))
				copy(srvSnap, servers)
				serversMu.Unlock()

				// Check server button clicks.
				for i := range srvSnap {
					if i < len(serverBtns) && serverBtns[i].Clicked(gtx) {
						doConnect(srvSnap[i].Name, srvSnap[i].Port)
					}
				}
				if connectBtn.Clicked(gtx) && ipEditor.Text() != "" {
					doConnect(ipEditor.Text(), 8080)
				}

				discoverLayout(gtx, th, &list, &ipEditor, &connectBtn, &connectErr, srvSnap, &serverBtns)

			case "stream":
				if streamScr != nil {
					streamScr.Layout(gtx)
				}
			}
			e.Frame(gtx.Ops)
		}
	}
}

// ── Drawing helpers ─────────────────────────────────────────────────

func bg(gtx layout.Context, c color.NRGBA) {
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: gtx.Constraints.Max}.Op())
}

func rrect(gtx layout.Context, c color.NRGBA, w, h, r int) {
	paint.FillShape(gtx.Ops, c, clip.RRect{
		Rect: image.Rectangle{Max: image.Pt(w, h)},
		NE: r, NW: r, SE: r, SW: r,
	}.Op(gtx.Ops))
}

func divider(gtx layout.Context) layout.Dimensions {
	paint.FillShape(gtx.Ops, colBorder, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
}

// ── Discover screen layout ──────────────────────────────────────────

func discoverLayout(gtx layout.Context, th *material.Theme, list *widget.List, ipEditor *widget.Editor, connectBtn *widget.Clickable, connectErr *string, servers []discovery.Beacon, serverBtns *[8]widget.Clickable) {
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Fixed header.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return headerBar(gtx, th)
		}),
		// Separator.
		layout.Rigid(divider),
		// Scanning strip.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return scanStrip(gtx, th)
		}),
		// Scrollable server list.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, list).Layout(gtx, max(len(servers), 1), func(gtx layout.Context, i int) layout.Dimensions {
				if len(servers) == 0 {
					return emptyState(gtx, th)
				}
				if i >= len(serverBtns) {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return serverCard(gtx, th, &serverBtns[i], servers[i])
				})
			})
		}),
		// Separator.
		layout.Rigid(divider),
		// Manual IP (pinned bottom).
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return manualIP(gtx, th, ipEditor, connectBtn, connectErr)
		}),
	)
}

func headerBar(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(48), Left: unit.Dp(20), Right: unit.Dp(20), Bottom: unit.Dp(16)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				// Brand mark.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(32))
					rrect(gtx, colIndigo, s, s, 8)
					// "V" inside.
					return layout.Dimensions{Size: image.Pt(s, s)}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.H6(th, "Connect")
					l.Color = colHead
					l.Font.Weight = font.Bold
					l.TextSize = unit.Sp(20)
					return l.Layout(gtx)
				}),
			)
		})
}

func scanStrip(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20), Top: unit.Dp(14), Bottom: unit.Dp(14)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				// Pulsing dot.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(8))
					paint.FillShape(gtx.Ops, colIndigo, clip.Ellipse{Max: image.Pt(s, s)}.Op(gtx.Ops))
					return layout.Dimensions{Size: image.Pt(s, s)}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(th, "Scanning your network...")
					l.Color = colDim
					l.TextSize = unit.Sp(12.5)
					return l.Layout(gtx)
				}),
			)
		})
}

func serverCard(gtx layout.Context, th *material.Theme, btn *widget.Clickable, srv discovery.Beacon) layout.Dimensions {
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		// Card bg.
		rrect(gtx, colSurface, w, gtx.Dp(unit.Dp(76)), 12)
		// Top border highlight.
		paint.FillShape(gtx.Ops, colBorder, clip.RRect{
			Rect: image.Rectangle{Max: image.Pt(w, 1)},
			NE: 12, NW: 12,
		}.Op(gtx.Ops))

		return layout.Inset{Top: unit.Dp(16), Bottom: unit.Dp(16), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					// Icon.
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						s := gtx.Dp(unit.Dp(42))
						rrect(gtx, colSurf2, s, s, 10)
						// Monitor icon area — center an "M" for now.
						return layout.Dimensions{Size: image.Pt(s, s)}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					// Info.
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.Body1(th, srv.Name)
								l.Color = colHead
								l.Font.Weight = font.SemiBold
								l.TextSize = unit.Sp(14)
								l.MaxLines = 1
								return l.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.Caption(th, fmt.Sprintf("%s · port %d", srv.Platform, srv.Port))
								l.Color = colDim
								l.TextSize = unit.Sp(11)
								return l.Layout(gtx)
							}),
						)
					}),
					// Chevron.
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Body1(th, "›")
						l.Color = colDim
						l.TextSize = unit.Sp(22)
						return l.Layout(gtx)
					}),
				)
			})
	})
}

func emptyState(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(60), Left: unit.Dp(40), Right: unit.Dp(40)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				// Pulse circle.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(64))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 99, G: 102, B: 241, A: 30}, clip.Ellipse{Max: image.Pt(s, s)}.Op(gtx.Ops))
					inner := gtx.Dp(unit.Dp(36))
					off := (s - inner) / 2
					paint.FillShape(gtx.Ops, color.NRGBA{R: 99, G: 102, B: 241, A: 60},
						clip.Ellipse{Min: image.Pt(off, off), Max: image.Pt(off+inner, off+inner)}.Op(gtx.Ops))
					return layout.Dimensions{Size: image.Pt(s, s)}
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.H6(th, "No servers found")
					l.Color = colHead
					l.Font.Weight = font.SemiBold
					l.Alignment = text.Middle
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(th, "Make sure Vior is running on your\ncomputer and same Wi-Fi network.")
					l.Color = colDim
					l.Alignment = text.Middle
					return l.Layout(gtx)
				}),
			)
		})
}

func manualIP(gtx layout.Context, th *material.Theme, ipEditor *widget.Editor, connectBtn *widget.Clickable, connectErr *string) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18), Top: unit.Dp(16), Bottom: unit.Dp(32)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Label.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(4), Bottom: unit.Dp(8)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(th, "CONNECT MANUALLY")
							l.Color = colDim
							l.TextSize = unit.Sp(10.5)
							l.Font.Weight = font.SemiBold
							return l.Layout(gtx)
						})
				}),
				// Input card.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					w := gtx.Constraints.Max.X
					h := gtx.Dp(unit.Dp(50))
					rrect(gtx, colSurface, w, h, 10)
					// Top highlight.
					paint.FillShape(gtx.Ops, colBorder, clip.RRect{
						Rect: image.Rectangle{Max: image.Pt(w, 1)},
						NE: 10, NW: 10,
					}.Op(gtx.Ops))

					return layout.UniformInset(unit.Dp(5)).Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx,
										func(gtx layout.Context) layout.Dimensions {
											ed := material.Editor(th, ipEditor, "192.168.x.x")
											ed.Color = colText
											ed.HintColor = colDim
											ed.TextSize = unit.Sp(15)
											return ed.Layout(gtx)
										})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(th, connectBtn, "Connect")
									btn.Background = colIndigo
									btn.Color = colWhite
									btn.CornerRadius = unit.Dp(8)
									btn.TextSize = unit.Sp(13)
									btn.Font.Weight = font.SemiBold
									btn.Inset = layout.Inset{
										Top: unit.Dp(9), Bottom: unit.Dp(9),
										Left: unit.Dp(18), Right: unit.Dp(18),
									}
									return btn.Layout(gtx)
								}),
							)
						})
				}),
				// Error.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if *connectErr == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(4)}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							l := material.Caption(th, *connectErr)
							l.Color = color.NRGBA{R: 239, G: 68, B: 68, A: 255}
							return l.Layout(gtx)
						})
				}),
			)
		})
}

