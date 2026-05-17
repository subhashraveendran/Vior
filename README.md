<div align="center">

# Vior

**Extend your view. Stream, control, transfer.**

Turn your phone or tablet into a second monitor. No cables. No app install on phone. Just works.

[![Build](https://github.com/subhashraveendran/Vior/actions/workflows/build.yml/badge.svg)](https://github.com/subhashraveendran/Vior/actions/workflows/build.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev)

[Documentation](https://subhashraveendran.github.io/Vior) | [Download](https://github.com/subhashraveendran/Vior/releases) | [Contributing](CONTRIBUTING.md)

</div>

---

## What is Vior?

Vior creates a **virtual second monitor** on your computer and streams it to any device with a browser. Your phone becomes an extended display — drag windows onto it, use touch to control, and transfer files between devices.

```
 Computer                              Phone / Tablet
 ┌──────────────────────┐              ┌───────────────┐
 │  Vior Server         │   WiFi/USB   │  Browser      │
 │                      │              │               │
 │  Virtual Display ◄───┼──────────────┼─ Live Stream  │
 │  Screen Capture      │              │  Touch Input  │
 │  Input Injection     │              │  File Transfer│
 └──────────────────────┘              └───────────────┘
```

### How it works

1. Start Vior on your computer
2. Phone connects automatically (or scan QR code)
3. Phone reports its screen dimensions
4. Vior creates a matching virtual display
5. Stream begins — drag windows onto it like a real monitor

## Install

### CLI (Quick)

```bash
go install github.com/subhashraveendran/vior/cmd/vior@latest
```

### Desktop App

Download from [Releases](https://github.com/subhashraveendran/Vior/releases) or build from source:

```bash
git clone https://github.com/subhashraveendran/Vior.git
cd Vior
make desktop
```

### Build from Source

**Prerequisites:** Go 1.25+, [Bun](https://bun.sh), [Wails](https://wails.io)

```bash
# CLI only
make build

# Desktop app (macOS/Linux/Windows)
make desktop

# Mobile app (requires gogio)
cd mobile && go run .
```

## Usage

### Desktop App

Launch the app. Click **Start Server**. Scan the QR code on your phone. Done.

### CLI

```bash
# Start server — phone connects and display auto-configures
vior start

# Legacy mode — specify resolution manually
vior start --virtual-width 1179 --virtual-height 2556

# USB connection (Android)
vior usb setup
vior start

# List displays
vior displays

# Stop
vior stop
```

### Phone / Tablet

Open the URL shown by Vior in any browser. The stream starts automatically with touch forwarding.

For full-screen experience, tap the fullscreen button or add to home screen (PWA).

## Features

| Feature | Description |
|---------|------------|
| **Auto Resolution** | Phone reports its screen size. Virtual display matches automatically. |
| **Touch Control** | Touch on phone = mouse on computer. Full touch-to-mouse mapping. |
| **Orientation** | Rotate phone to landscape — display resizes automatically. |
| **File Transfer** | Drag files to send. Accept incoming files from phone. Preview images. |
| **USB Mode** | Connect via USB for lower latency. ADB auto-downloaded if needed. |
| **Auto Discovery** | UDP broadcast finds Vior servers on LAN automatically. |
| **QR Code** | Scan to connect — no typing URLs. |
| **Web Client** | Works in any browser. No app install on phone. |
| **Cross-Platform** | macOS, Linux, Windows server. Any device with a browser as client. |

## Architecture

```
vior/
├── cmd/vior/cli/          CLI (Cobra)
├── desktop/               Wails Desktop App (Go + React)
├── mobile/                Gio Mobile App (Go)
├── internal/
│   ├── capture/           Screen capture (CGDisplayCreateImage / kbinani)
│   ├── stream/            MJPEG HTTP server + WebSocket
│   ├── virtual/           Virtual display (CGVirtualDisplay / xrandr / Win32)
│   ├── input/             Mouse/keyboard injection (CGEvent / XTest / SendInput)
│   ├── protocol/          WebSocket message protocol
│   ├── discovery/         UDP LAN auto-discovery
│   ├── filetransfer/      Bidirectional file transfer
│   ├── adb/               ADB/USB helpers with auto-download
│   ├── network/           QR code generation
│   ├── config/            Configuration
│   └── transfer/          TCP file transfer (legacy)
```

### Protocol

Vior uses WebSocket for signaling and MJPEG over HTTP for streaming:

```
Phone → Server:  {"type":"hello",  "data":{"width":1179,"height":2556,"dpr":3,"name":"iPhone"}}
Server → Phone:  {"type":"ready",  "data":{"streamUrl":"/stream","resolution":"1179x2556"}}
Phone → Server:  {"type":"input",  "data":{"event":"touch","action":"down","x":540,"y":1200}}
Phone → Server:  {"type":"resize", "data":{"width":2556,"height":1179,"dpr":3}}
```

### Platform Support

| Platform | Virtual Display | Screen Capture | Input Control |
|----------|----------------|---------------|---------------|
| **macOS** | CGVirtualDisplay | CGDisplayCreateImage | CGEvent |
| **Linux** | xrandr + dummy driver | kbinani/screenshot | X11 XTest |
| **Windows** | Win32 + IDD driver | kbinani/screenshot | SendInput |

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.25 |
| CLI | Cobra |
| Desktop | Wails v2 + React + Framer Motion |
| Mobile | Gio |
| Streaming | MJPEG over HTTP |
| Signaling | WebSocket (gorilla/websocket) |
| Discovery | UDP broadcast |
| QR Codes | go-qrcode |
| Build | Make + Vite + Bun |

## Requirements

| Platform | Minimum |
|----------|---------|
| macOS | Ventura (13.0) |
| Linux | X11 + xrandr |
| Windows | 10+ build 1809 |

## License

[MIT](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions and guidelines.
