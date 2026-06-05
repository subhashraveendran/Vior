//go:build darwin

// macOS menu-bar (NSStatusItem) integration. The Objective-C
// implementation lives in tray_darwin.m so cgo only compiles the class
// once — without splitting, cgo's own re-imports cause duplicate
// OBJC_CLASS / OBJC_IVAR linker symbols on every Go file.

package main

/*
#cgo CFLAGS:  -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework AppKit
#include <stdlib.h>
#include "tray_darwin.h"
*/
import "C"

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	trayOnce sync.Once
	trayCtx  context.Context
	trayApp  *App
)

//export viorTrayMenuClicked
func viorTrayMenuClicked(tag C.int) {
	if trayApp == nil || trayCtx == nil {
		return
	}
	switch int(tag) {
	case 1: // Show
		wruntime.WindowShow(trayCtx)
		wruntime.WindowUnminimise(trayCtx)
	case 2: // Hide
		wruntime.WindowHide(trayCtx)
	case 3: // Start
		_ = trayApp.StartServer()
	case 4: // Stop
		_ = trayApp.StopServer()
	case 5: // Quit
		_ = trayApp.StopServer()
		wruntime.Quit(trayCtx)
	}
}

// startTray installs the menu-bar item and a background ticker that
// keeps the status entry in sync with the server.
func startTray(ctx context.Context, app *App) {
	trayCtx = ctx
	trayApp = app
	trayOnce.Do(func() {
		C.viorTrayInstall()
		go func() {
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for range t.C {
				s := app.GetServerStatus()
				var text string
				var running C.int
				if s.Running {
					text = fmt.Sprintf("Server: running on :%d", s.Port)
					running = 1
				} else {
					text = "Server: stopped"
				}
				cText := C.CString(text)
				C.viorTraySetStatus(cText, running)
				C.free(unsafe.Pointer(cText))
			}
		}()
	})
}
