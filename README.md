<div align="center">

# Vior

**Phone → Mac/Linux/Windows. Extend display, file transfer, trackpad, keyboard, shortcuts. One app.**

[![Build](https://github.com/subhashraveendran/Vior/actions/workflows/build.yml/badge.svg)](https://github.com/subhashraveendran/Vior/actions/workflows/build.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev)

[Phase 2 docs](docs/phase-2.md) · [Downloads](https://github.com/subhashraveendran/Vior/releases) · [Contributing](CONTRIBUTING.md)

</div>

---

## What it does

Turn your phone or tablet into:

- **Second display** — extend or mirror your desktop. Touch on phone moves the cursor.
- **WiFi mouse + keyboard** — trackpad with 2-finger scroll, soft keyboard, 20+ shortcut keys (Copy/Paste/Cmd+Tab/Spotlight/F-keys).
- **File drop** — send files or photos between phone and desktop with progress + image previews.

Cross-platform server (macOS / Linux / Windows). Auto-discovery on LAN. Auto-connect on launch.

```
 ┌────────────────────────┐         WiFi          ┌────────────────────┐
 │ Desktop (Wails / CLI)  │ ◄─── WebSocket ─────► │ Phone / Tablet     │
 │                        │                       │  (Capacitor APK)   │
 │  Virtual Display       │ ◄─── MJPEG /snapshot  │   Display tab      │
 │  Mouse / Keyboard      │ ◄─── input events     │   Files tab        │
 │  ~/Downloads/Vior      │ ◄─── file chunks ────►│   Remote tab       │
 └────────────────────────┘                       └────────────────────┘
```

## Quick start

### 1. Server (Mac / Linux / Windows)

```bash
# Desktop app (Wails — recommended)
# Download from Releases, or:
git clone https://github.com/subhashraveendran/Vior.git
cd Vior/desktop && wails build

# Or CLI
go install github.com/subhashraveendran/vior/cmd/vior@latest
vior start
```

### 2. Mobile (Android)

- Download `vior-mobile-Android.apk` from [Releases](https://github.com/subhashraveendran/Vior/releases).
- Install. Open. Vior auto-discovers your computer on the same Wi-Fi and connects.

iOS / browser fallback: open the URL the server prints in any browser.

## Mobile app — 3 tabs

| Tab | What it does |
|-----|--------------|
| **Display** | Auto-discover servers, pick Extend or Mirror mode, view live stream fullscreen with touch input. Auto-reconnect with exponential backoff. |
| **Files** | Send any file or photo via native picker. Incoming offers show Accept / Reject. Progress bars, image thumbnails, Save button for received files. |
| **Remote** | Trackpad (1-finger move + tap-click, 2-finger scroll + tap-rightclick). Soft keyboard for printable + special keys (BackSpace, Enter, arrows). 20+ shortcut buttons: Copy / Paste / Cut / Undo / Cmd+Tab / Spotlight / F-keys. |

Auto-connect: app remembers last server and re-establishes the session automatically when you launch.

## Features

| Feature | Status |
|---|---|
| Extend mode (virtual display matching phone resolution) | ✓ |
| Mirror mode (capture main display, scaled to fit phone) | ✓ |
| Touch input → mouse | ✓ |
| Virtual trackpad with 2-finger scroll + tap shortcuts | ✓ |
| Soft keyboard + 20 shortcut buttons (Cmd/Ctrl chord support) | ✓ |
| Bidirectional file transfer with chunked progress | ✓ |
| LAN auto-discovery (HTTP subnet scan + UDP broadcast) | ✓ |
| Auto-connect to last-known server | ✓ |
| QR code for browser fallback | ✓ |
| USB Accessory Mode (no developer options needed) | ✓ (untested on shipping device) |
| Android 15 edge-to-edge handled (WebView insets) | ✓ |
| Production CI: macOS / Linux / Windows CLI + Wails + Android APK | ✓ |
| H.264 video / audio forwarding / clipboard sync | Phase 3 |

## Server CLI

```bash
vior start                         # auto-config; waits for phone
vior start --port 8080             # explicit port
vior displays                      # list connected displays
vior display mirror 1              # mirror display 1 to main
vior display extend 1              # extend display 1
vior usb setup                     # forward ADB ports for USB connection
vior virtual create 1920 1080      # create virtual display manually
vior virtual destroy
vior stop
```

## Architecture

```
cmd/vior/             CLI entry (Cobra)
desktop/              Wails desktop app (Go + React + Framer Motion)
  app.go              Frontend bridge, server lifecycle, file transfer
  frontend/           React UI (idle / waiting / connected views)
internal/
  capture/            Screen capture (CG / X11 / kbinani-screenshot)
  config/             Ports, defaults, free-port helper
  discovery/          UDP broadcast + LAN IP detection
  filetransfer/       Chunked transfer manager (48 KB / 5 ms throttle / SHA-256)
  input/              Mouse + keyboard injection (CGEvent / XTest / SendInput)
    touch.go          Touch + Mouse + Scroll handlers
  network/            QR code generator
  protocol/           WebSocket message types + session
  session/            Configure() — shared extend/mirror setup
  stream/             MJPEG + WebSocket + CORS server, /info, /snapshot
  usb/                Android Open Accessory protocol (gousb)
  virtual/            Virtual display creation (CGVirtualDisplay / xrandr / IDD)
mobile-cap/
  src/index.html      Entire mobile app (single file, ~1.2k LOC)
  android/            Custom Java + manifest overlays (CI regenerates rest)
docs/
  phase-2.md          Phase 2 architecture, protocol, gaps, roadmap
.github/workflows/
  build.yml           Unified CI: CLI + Wails + APK across 3 OSes
```

## Protocol

WebSocket signaling + MJPEG over HTTP for video + binary frames over AOA for USB.

```jsonc
// Phone → Server
{"type":"hello",       "data":{"width":1080,"height":2400,"dpr":3,"name":"Phone","mode":"extend"}}
{"type":"input",       "data":{"event":"touch", "action":"down", "x":540, "y":1200}}
{"type":"input",       "data":{"event":"mouse", "action":"move", "dx":10, "dy":-5}}
{"type":"input",       "data":{"event":"mouse", "action":"click"}}
{"type":"input",       "data":{"event":"scroll","dx":0,"dy":-3}}
{"type":"input",       "data":{"event":"key",   "key":"Cmd+c"}}
{"type":"resize",      "data":{"width":2400,"height":1080,"dpr":3}}
{"type":"file-offer",  "data":{"id":"abc","name":"photo.jpg","size":12345,"mimeType":"image/jpeg","preview":"data:..."}}
{"type":"file-chunk",  "data":{"id":"abc","offset":0,"data":"<base64>"}}
{"type":"file-complete","data":{"id":"abc","hash":"<sha256>"}}

// Server → Phone
{"type":"ready",       "data":{"streamUrl":"/stream","resolution":"1080x2400","sessionId":"s1"}}
{"type":"file-accept", "data":{"id":"abc"}}
{"type":"error",       "data":{"code":"perm_denied","message":"Screen recording permission required"}}
```

## Platform support

| OS | Virtual display | Capture | Input |
|---|---|---|---|
| **macOS 13+** | CGVirtualDisplay | CGDisplayCreateImage | CGEvent (Unicode + modifier chords) |
| **Linux** | xrandr + dummy driver | kbinani/screenshot | XTest |
| **Windows 10+** | IDD driver (manual install) | kbinani/screenshot | SendInput |

## CI

Single workflow (`build.yml`) builds and uploads:

- `vior-cli-{macOS,Linux,Windows}` — Go binaries
- `vior-desktop-{macOS,Linux,Windows}` — Wails apps
- `vior-mobile-Android` — Capacitor APK

`fail-fast: false`, all targets run in parallel. Mobile artifact is what you sideload to your phone.

## Tech

| Layer | Stack |
|---|---|
| Server | Go 1.25, cgo, gousb |
| Desktop | Wails v2, React 19, Vite, Framer Motion |
| Mobile | Capacitor 7 + plain HTML/CSS/JS (single file) |
| Streaming | MJPEG over HTTP (Phase 2). H.264 planned for Phase 3. |
| Signaling | WebSocket (gorilla/websocket) |
| Discovery | UDP broadcast (port 37680) + HTTP `/info` subnet scan |
| QR | go-qrcode |

## Roadmap

Phase 3 candidates: H.264 (VideoToolbox / NVENC / VA-API), audio forwarding (Opus), clipboard sync, multi-client, pen pressure, Linux/Windows keyboard parity with the new macOS Unicode path. Full list in [`docs/phase-2.md`](docs/phase-2.md).

## License

MIT. See [LICENSE](LICENSE).
