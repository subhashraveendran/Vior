//go:build !darwin

package main

import "context"

// startTray is a no-op on non-macOS platforms.
func startTray(_ context.Context, _ *App) {}

// setMenuBarVisible / read+write prefs are macOS-only; stubs keep the
// frontend bindings compiling.
func setMenuBarVisible(_ bool) {}
func readMenuBarPref() bool    { return false }
func writeMenuBarPref(_ bool)  {}
