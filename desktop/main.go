// Vior desktop app entry point using Wails.
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/subhashraveendran/vior/internal/config"
	"github.com/subhashraveendran/vior/internal/virtual"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// Wails does NOT translate SIGINT/SIGTERM into OnBeforeClose (wails
	// issue #2421), so a terminal Ctrl-C, an IDE "stop", or a
	// crash-supervisor SIGTERM would otherwise bypass stopEverything()
	// and leak the macOS virtual display (phantom monitor + rearranged
	// windows) plus the USB claim. Catch the signals ourselves and run
	// teardown, bounded by a timeout so a wedged libusb close can't hang
	// exit forever. stopEverything is mutex-guarded, so this coexists
	// safely with OnBeforeClose if both fire.
	sigCtx, stopSig := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSig()
	go func() {
		<-sigCtx.Done()
		log.Println("desktop: signal received, shutting down")
		done := make(chan struct{})
		go func() { app.stopEverything(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Println("desktop: shutdown timed out, forcing exit")
		}
		os.Exit(0)
	}()

	// Last-ditch: a panic on the main path must still tear down the
	// virtual display (the one leak the OS won't promptly reclaim)
	// before the process dies.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("desktop: fatal panic, cleaning up virtual display: %v", r)
			virtual.Destroy()
			panic(r)
		}
	}()

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
		log.Printf("desktop: wails run error: %v", err)
		virtual.Destroy() // don't leak the virtual display on a startup failure
	}
}
