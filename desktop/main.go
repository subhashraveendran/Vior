// Vior desktop app entry point using Wails.
package main

import (
	"embed"

	"github.com/subhashraveendran/vior/internal/config"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Vior — Extend your view",
		Width:     960,
		Height:    640,
		MinWidth:  720,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		// HideWindowOnClose: closing the window only hides it. The Go
		// server (HTTP+WS+USB) keeps running so already-paired phones can
		// reconnect at any time without the user re-opening Vior. Real
		// exit happens via Cmd+Q or the menu-bar Quit item.
		HideWindowOnClose: true,
		Bind: []any{
			app,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "Vior",
				Message: "Extend your view. Stream, control, transfer.\n\n" + config.Version,
			},
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
