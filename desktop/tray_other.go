//go:build !darwin

package main

import "context"

// startTray is a no-op on non-macOS platforms.
func startTray(_ context.Context, _ *App) {}
