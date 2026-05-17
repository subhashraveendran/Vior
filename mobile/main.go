// Vior Mobile — companion app for Vior second display server.
// Built with Gio (gioui.org) for iOS and Android.
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/subhashraveendran/vior/mobile/client"
	"github.com/subhashraveendran/vior/mobile/ui"
)

// Screen represents the current app screen.
type Screen int

const (
	ScreenDiscover Screen = iota
	ScreenStream
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
	currentScreen := ScreenDiscover

	// Client.
	var viorClient *client.ViorClient

	// Screens.
	discoverScreen := ui.NewDiscoverScreen(th)
	discoverScreen.StartScan()

	var streamScreen *ui.StreamScreen

	// Handle connect from discover screen.
	discoverScreen.OnConnect = func(host string, port int) {
		log.Printf("Connecting to %s:%d", host, port)

		viorClient = client.New(host, port)

		// Get screen dimensions from window.
		// On mobile, these come from the device screen.
		// For now use reasonable defaults; Gio provides actual size via layout.
		viorClient.ScreenWidth = 1179
		viorClient.ScreenHeight = 2556
		viorClient.DPR = 3
		viorClient.DeviceName = "Vior Mobile"

		viorClient.OnReady = func(streamURL, resolution string) {
			log.Printf("Ready: %s %s", streamURL, resolution)

			// Parse resolution.
			parts := strings.Split(resolution, "x")
			dw, _ := strconv.Atoi(parts[0])
			dh, _ := strconv.Atoi(parts[1])

			streamScreen = ui.NewStreamScreen(th, viorClient)
			streamScreen.OnBack = func() {
				currentScreen = ScreenDiscover
				if viorClient != nil {
					viorClient.Disconnect()
				}
				discoverScreen.StartScan()
			}

			fullURL := fmt.Sprintf("http://%s:%d%s", host, port, streamURL)
			streamScreen.StartStream(fullURL, dw, dh)
			currentScreen = ScreenStream
			w.Invalidate()
		}

		viorClient.OnDisconnect = func() {
			log.Println("Disconnected")
			currentScreen = ScreenDiscover
			w.Invalidate()
		}

		viorClient.OnError = func(code, msg string) {
			log.Printf("Error: [%s] %s", code, msg)
		}

		if err := viorClient.Connect(); err != nil {
			log.Printf("Connect failed: %v", err)
		}
	}

	// Event loop.
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			if viorClient != nil {
				viorClient.Disconnect()
			}
			discoverScreen.StopScan()
			if streamScreen != nil {
				streamScreen.StopStream()
			}
			if e.Err != nil {
				log.Fatal(e.Err)
			}
			os.Exit(0)

		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			switch currentScreen {
			case ScreenDiscover:
				discoverScreen.Layout(gtx)
			case ScreenStream:
				if streamScreen != nil {
					streamScreen.Layout(gtx)
				}
			}

			e.Frame(gtx.Ops)
		}
	}
}
