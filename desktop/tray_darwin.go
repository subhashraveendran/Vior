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
	"os"
	"path/filepath"
	"strings"
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
	if !readMenuBarPref() {
		return // user disabled the menu bar in Settings
	}
	trayOnce.Do(func() {
		C.viorTrayInstall()
		go func() {
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for range t.C {
				s := app.GetServerStatus()
				clients := app.GetConnectedClients()
				// Status entry: "Vior — Connected: <name>" / "Vior —
				// Waiting · ABC-123" / "Vior — Ready". Mirrors the
				// in-app state pill so the user never has to open the
				// window to know the state.
				var text string
				var running C.int
				switch {
				case s.Running && len(clients) > 0:
					text = fmt.Sprintf("Vior — Connected: %s", clients[0].Name)
					running = 1
				case s.Running:
					text = fmt.Sprintf("Vior — Waiting · %s", formatPairCode(s.PairCode))
					running = 1
				default:
					text = "Vior — Ready"
				}
				cText := C.CString(text)
				C.viorTraySetStatus(cText, running)
				C.free(unsafe.Pointer(cText))
			}
		}()
	})
}

// formatPairCode splits the 6-char hex pair code into "ABC-123" purely
// for display. Kept here so both the tray and any other Go-side surface
// share one formatter; the mobile side parses both forms transparently.
func formatPairCode(code string) string {
	if len(code) <= 3 {
		return code
	}
	return code[:3] + "-" + code[3:]
}

// setMenuBarVisible toggles the NSStatusItem at runtime.
func setMenuBarVisible(visible bool) {
	if visible {
		if trayCtx != nil && trayApp != nil {
			startTray(trayCtx, trayApp)
		}
		return
	}
	C.viorTrayUninstall()
	// Reset the sync.Once so a later show can reinstall cleanly.
	trayOnce = sync.Once{}
}

// readMenuBarPref returns true unless the file ~/.vior/menubar.flag
// contains the literal "off".
func readMenuBarPref() bool {
	p, err := menuBarPrefPath()
	if err != nil {
		return true
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(b)) != "off"
}

func writeMenuBarPref(visible bool) {
	p, err := menuBarPrefPath()
	if err != nil {
		return
	}
	val := []byte("on")
	if !visible {
		val = []byte("off")
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, val, 0o600)
}

func menuBarPrefPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vior", "menubar.flag"), nil
}
