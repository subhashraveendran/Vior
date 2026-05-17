// Vior Mobile — companion app for Vior second display server.
// Built with Gio (gioui.org) for iOS and Android.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"strconv"
	"strings"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/subhashraveendran/vior/mobile/client"
	"github.com/subhashraveendran/vior/mobile/ui"
)

func main() {
	go run()
	app.Main()
}

func run() {
	w := new(app.Window)
	w.Option(
		app.Title("Vior"),
		app.Size(unit.Dp(400), unit.Dp(800)),
	)

	th := ui.NewTheme()

	var (
		currentScreen  = "discover"
		viorClient     *client.ViorClient
		streamScreen   *ui.StreamScreen
		ipEditor       widget.Editor
		connectBtn     widget.Clickable
		connectErr     string
	)

	ipEditor.SingleLine = true

	var ops op.Ops

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			if viorClient != nil {
				viorClient.Disconnect()
			}
			if streamScreen != nil {
				streamScreen.StopStream()
			}
			if e.Err != nil {
				log.Fatal(e.Err)
			}
			os.Exit(0)

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			// Background.
			paint.FillShape(gtx.Ops, color.NRGBA{R: 12, G: 13, B: 18, A: 255},
				clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

			switch currentScreen {
			case "discover":
				layoutDiscover(gtx, th, &ipEditor, &connectBtn, &connectErr, func(host string, port int) {
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

						streamScreen = ui.NewStreamScreen(th, viorClient)
						streamScreen.OnBack = func() {
							currentScreen = "discover"
							if viorClient != nil {
								viorClient.Disconnect()
							}
						}

						fullURL := fmt.Sprintf("http://%s:%d%s", host, port, streamURL)
						streamScreen.StartStream(fullURL, dw, dh)
						currentScreen = "stream"
						w.Invalidate()
					}

					viorClient.OnDisconnect = func() {
						currentScreen = "discover"
						w.Invalidate()
					}

					viorClient.OnError = func(code, msg string) {
						connectErr = msg
						w.Invalidate()
					}

					if err := viorClient.Connect(); err != nil {
						connectErr = err.Error()
					}
				})

			case "stream":
				if streamScreen != nil {
					streamScreen.Layout(gtx)
				}
			}

			e.Frame(gtx.Ops)
		}
	}
}

func layoutDiscover(gtx layout.Context, th *material.Theme, ipEditor *widget.Editor, connectBtn *widget.Clickable, connectErr *string, onConnect func(string, int)) layout.Dimensions {
	// Handle connect click.
	if connectBtn.Clicked(gtx) {
		ip := ipEditor.Text()
		if ip != "" {
			onConnect(ip, 8080)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Spacer top.
		layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),

		// Title.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.H4(th, "Vior")
				l.Color = color.NRGBA{R: 129, G: 140, B: 248, A: 255}
				return l.Layout(gtx)
			})
		}),

		// Subtitle.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32), Bottom: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(th, "Enter your computer's IP address")
				l.Color = color.NRGBA{R: 92, G: 97, B: 112, A: 255}
				return l.Layout(gtx)
			})
		}),

		// IP input.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Background for editor.
				paint.FillShape(gtx.Ops, color.NRGBA{R: 20, G: 22, B: 28, A: 255},
					clip.RRect{Rect: image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(48)))},
						NE: 10, NW: 10, SE: 10, SW: 10}.Op(gtx.Ops))

				return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(th, ipEditor, "192.168.x.x")
					ed.Color = color.NRGBA{R: 201, G: 205, B: 211, A: 255}
					ed.HintColor = color.NRGBA{R: 62, G: 67, B: 80, A: 255}
					ed.TextSize = unit.Sp(16)
					return ed.Layout(gtx)
				})
			})
		}),

		// Connect button.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(th, connectBtn, "Connect")
				btn.Background = color.NRGBA{R: 79, G: 70, B: 229, A: 255}
				btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				btn.CornerRadius = unit.Dp(12)
				btn.TextSize = unit.Sp(16)
				return btn.Layout(gtx)
			})
		}),

		// Error text.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if *connectErr == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(32), Right: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(th, *connectErr)
				l.Color = color.NRGBA{R: 239, G: 68, B: 68, A: 255}
				return l.Layout(gtx)
			})
		}),

		// Spacer bottom.
		layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)
}
