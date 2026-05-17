# Vior

**Extend your view. Stream, control, transfer.**

Vior turns your phone or tablet into a second monitor. One button. No cables. Any browser.

---

## How It Works

```
 macOS/Linux/Windows                    Phone / Tablet
 ┌──────────────────────┐              ┌───────────────┐
 │  Vior                 │   WiFi/LAN  │  Web Browser  │
 │                      │   MJPEG/HTTP │               │
 │  Virtual Display ◄────┼─────────────┼─ <img> tag    │
 │  Screen Capture       │             │               │
 │  Stream Server :8080  │             │  Open URL      │
 └──────────────────────┘              └───────────────┘
```

1. Vior creates a virtual display matching your phone's resolution
2. macOS/Linux/Windows sees it as a real second monitor
3. Vior captures that display and streams it over HTTP
4. You open the URL on your phone — it shows the extended screen
5. Drag any window onto it like a real monitor

---

## Quick Start

```bash
# CLI
vior virtual create --device iphone-pro
vior start
# Open the URL shown on your phone

# Desktop App
# Launch Vior → pick your device → press "Connect Phone"
```

---

## Install

```bash
go install github.com/subhashraveendran/vior/cmd/vior@latest
```

Or download the desktop app from releases.

---

## Features

### Virtual Display

Creates a real virtual monitor the OS treats as a physical display. Supports custom resolutions matching any phone or tablet.

| Platform | Technology |
|----------|-----------|
| macOS | CGVirtualDisplay (private API, stable since Ventura) |
| Linux | xrandr + xf86-video-dummy |
| Windows | Requires IDD driver or dummy HDMI plug |

Device presets: iPhone 14, iPhone 15 Pro, iPhone 14 Plus, iPad Air, iPad Pro, iPad Mini.

### Screen Streaming

Captures the virtual display at configurable FPS and quality. Streams as MJPEG over HTTP — works in any browser with zero setup on the client.

| Feature | Value |
|---------|-------|
| Protocol | MJPEG over HTTP (multipart/x-mixed-replace) |
| Format | JPEG, configurable quality (1–100) |
| Frame rate | Configurable, default 30 FPS |
| Clients | Up to 16 concurrent |
| Latency | ~50–100ms on LAN |
| Security | LAN-only, no auth |

### Display Modes

- **Extend** — separate screen. Drag windows onto the virtual display. Phone acts as second monitor.
- **Mirror** — same content. Phone shows exactly what your main screen shows.

### Memory Safe

Zero memory leaks. Every CoreFoundation and Objective-C object is traced and released. CGContext, CGImage, CGColorSpace, SCContentFilter, SCStreamConfiguration — all cleaned up per frame. Frame channel closed on stop. HTTP write deadlines prevent stalled-client leaks. Client limit prevents resource exhaustion.

---

## Architecture

```
vior (single Go module)
│
├── cmd/vior/cli/          CLI (Cobra)
│   ├── start              Start streaming
│   ├── stop               Stop running session (PID file IPC)
│   ├── displays           List connected displays
│   ├── virtual            Create/destroy virtual displays
│   ├── display            Mirror/extend display modes
│   └── version            Print version
│
├── desktop/               Wails Desktop App
│   ├── app.go             15+ bound Go methods
│   ├── main.go            Window + Wails runtime
│   └── frontend/          Vite + vanilla JS UI
│
├── internal/capture/      Screen capture
│   ├── capture.go         Session, JPEG encode, display list
│   ├── capture_darwin.go  macOS: CGDisplayCreateImage (dlsym, no permissions)
│   └── capture_other.go   Linux/Windows: kbinani/screenshot fallback
│
├── internal/stream/       MJPEG HTTP server
│   └── stream.go          /stream, /snapshot, / endpoints
│
├── internal/virtual/      Virtual display creation
│   ├── display_darwin.m   macOS: CGVirtualDisplay ObjC
│   ├── display_linux.go   Linux: xrandr
│   └── display_windows.go Windows: Win32 EnumDisplayDevices
│
├── internal/input/        Mouse/keyboard control
│   ├── input_darwin.go    macOS: CGEvent
│   ├── input_linux.go     Linux: X11 XTest
│   └── input_windows.go   Windows: SendInput
│
├── internal/transfer/     File transfer
│   └── transfer.go        TCP send/receive with progress
│
├── internal/network/      Discovery
│   └── network.go         QR code generation
│
└── internal/config/       Configuration
    └── config.go          Shared config with defaults
```

---

## Commands

```bash
vior start                          # Start streaming display 0 on port 8080
vior start --display 1 --port 9090  # Stream specific display on custom port
vior start --virtual-width 1179 --virtual-height 2556  # Create + stream in one command
vior stop                           # Stop the running session
vior displays                       # List all displays with pixel resolutions
vior display mirror --source 1 --target 0   # Mirror display 1 onto display 0
vior display extend 1               # Set display 1 to extended mode
vior virtual create --device iphone-pro     # Create virtual display matching iPhone 15 Pro
vior virtual create --width 1920 --height 1080  # Custom dimensions
vior virtual destroy                # Remove virtual display
vior virtual setup                  # Platform-specific setup guide
vior version                        # Print version
```

---

## Desktop App

Launch the app. Pick your device from the dropdown. Press the big button. That's it.

The app auto-creates the virtual display, sets extended mode, starts streaming, and shows the URL. Press again to stop and tear everything down.

Built with Wails v2. Go backend, vanilla JS frontend. ~9MB binary.

---

## Browser Client

Any browser works. Open the URL shown by Vior. The built-in viewer renders the MJPEG stream as a full-page image.

No install. No app. No permissions. Just a URL.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.25 |
| CLI | Cobra |
| Desktop GUI | Wails v2 + Vite |
| macOS capture | CoreGraphics (CGDisplayCreateImage via dlsym) |
| macOS virtual display | CGVirtualDisplay (private API) |
| Linux capture | kbinani/screenshot |
| Linux virtual display | xrandr + xf86-video-dummy |
| Windows capture | kbinani/screenshot |
| Windows virtual display | Win32 EnumDisplayDevices + IDD driver |
| Input control | CGEvent / X11 XTest / SendInput |
| QR codes | go-qrcode |
| Frontend build | Bun |

---

## Build

```bash
make build          # CLI binary → tmp/vior
make desktop        # Desktop app → desktop/build/bin/vior-app.app
make run            # Run CLI with Air hot-reload
make desktop-dev    # Run desktop with Wails dev mode
```

---

## Requirements

| Platform | Minimum Version |
|----------|----------------|
| macOS | Ventura (13.0) |
| Linux | X11 + xrandr |
| Windows | 10+ build 1809 |
