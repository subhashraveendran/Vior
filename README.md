<div align="center">

# Vior

**Phone or tablet → Mac / Linux / Windows.** Second screen, file drop, trackpad, keyboard. One app, your network, no cloud.

[![Build](https://github.com/subhashraveendran/Vior/actions/workflows/build.yml/badge.svg)](https://github.com/subhashraveendran/Vior/actions/workflows/build.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev)

[Website](https://subhashraveendran.github.io/Vior/) · [Releases](https://github.com/subhashraveendran/Vior/releases) · [Phase 2 docs](docs/phase-2.md) · [Contributing](CONTRIBUTING.md)

</div>

---

## What is Vior

Vior is an open-source companion app that turns any phone or tablet into a real second display, wireless trackpad, soft keyboard, and file-drop target for your desktop. The desktop server runs natively on macOS, Linux, and Windows. The mobile client is a Capacitor Android app; any browser also works as a fallback. Everything happens on your LAN over Wi-Fi (or a USB cable when there is no network).

## Why Vior exists

Spacedesk, Duet, and Splashtop solve adjacent problems, but each has at least one of: closed source, telemetry, mandatory accounts, paywalled features, Windows-only servers, or no real second-screen support on macOS. Vior is an attempt to ship the same product as a small, MIT-licensed Go binary with no cloud.

What you get:

- **macOS extended display** via the private `CGVirtualDisplay` API — no kernel extension, no signed driver, no Sidecar-style device whitelist.
- **One binary** per OS. No installer wizard on macOS or Linux. Windows needs an optional IDD driver only for virtual-display mode.
- **No account, no telemetry.** Pair once with a 6-digit code; the phone finds the desktop by scanning the local subnet (or scan the QR code).
- **Wi-Fi *and* USB.** AOA (Android Open Accessory) lets the phone connect with a plain USB cable, no ADB and no developer mode.
- **Cross-platform from day one.** macOS, Linux (X11), Windows server. Android client today, iOS via Safari PWA, native iOS planned.

Honest trade-offs you should know about before you switch:

- Video is **MJPEG over HTTP** today. It is universally compatible and works in any browser, but it uses more bandwidth than H.264. Hardware-encoded H.264 (VideoToolbox / NVENC / VA-API) is the Phase 3 target.
- **No audio forwarding** yet. Phase 3.
- **No native iOS app** yet — the browser fallback works but is not as polished.
- **Linux is X11 only.** Wayland is not supported because virtual-display creation there still requires distro-specific compositor plumbing.
- **Windows virtual display** needs a one-time IDD driver install. Capture and input work without it.
- Pair codes protect against random LAN neighbours, and the server implements an application-layer encrypted channel — but the shipped Android and browser clients do not use it yet, so today's traffic is still readable by anyone on the same Wi-Fi. See [Security model](#security-model).

## Quick start

### Desktop server

Download a prebuilt artifact for your OS from [Releases](https://github.com/subhashraveendran/Vior/releases):

- `vior-desktop-macOS` — Wails app bundle (recommended)
- `vior-desktop-Linux` — Wails app
- `vior-desktop-Windows` — Wails app
- `vior-cli-{macOS,Linux,Windows}` — headless Go binary

Or build from source:

```bash
git clone https://github.com/subhashraveendran/Vior.git
cd Vior

# Desktop GUI (Wails)
cd desktop && wails build

# Or CLI
go install github.com/subhashraveendran/vior/cmd/vior@latest
vior start
```

### Mobile client

- **Android:** download `vior-mobile-Android.apk` from [Releases](https://github.com/subhashraveendran/Vior/releases) and sideload it. Play Store listing is not published yet.
- **iOS / any browser:** open the URL the desktop prints in any browser. A native iOS app is on the roadmap.

### First run

1. Launch the desktop app and click **Start Server**. You will see one or more LAN URLs, a QR code, and a 6-digit pair code.
2. Open Vior on the phone. It finds the desktop by probing the local subnet. If that is blocked (guest Wi-Fi, AP isolation), scan the QR code or type the IP.
3. Enter the 6-digit pair code once. The phone remembers it and reconnects automatically next time; the desktop lists the device under Settings → Trusted devices.

## Features

- Second display — extend mode (virtual display matches phone resolution) or mirror mode (scales main display to fit phone).
- Touch on phone → mouse cursor on desktop.
- Virtual trackpad with two-finger scroll, tap-click, two-finger-tap right-click.
- Soft keyboard plus 40+ shortcut buttons (Copy/Paste/Cut/Undo, Cmd+Tab, Spotlight, F-keys, arrow keys, modifier chords).
- Bidirectional file transfer with progress bars, SHA-256 integrity check, image thumbnails, native file picker. Auto-accepts from trusted devices.
- LAN auto-discovery (HTTP subnet scan of `/info`; a UDP beacon on port 37680 is also broadcast for third-party clients).
- Auto-reconnect to the last known server.
- QR-code pairing for the browser fallback.
- USB Accessory Mode — works without ADB, without USB debugging, without developer options.
- Pair-code admission + persistent trust store.
- Browser-based web client (served by the desktop server, embedded into the binary).
- Android 15 edge-to-edge / safe-area aware UI.
- Optional macOS menu-bar (NSStatusItem) controller.
- CI builds CLI + Wails + APK across macOS, Linux, and Windows on every commit.

Screenshots live in [`docs/site/img/`](docs/site/img): `desktop-connected.png`, `mobile-display.png`, `mobile-files.png`, `mobile-remote.png`.

## Architecture

```
                      ┌─────────────────────────────────────────────┐
                      │            Desktop (Go + Wails)             │
                      │                                             │
  ┌──────────────┐    │  ┌──────────┐     ┌────────────────────┐    │
  │  System UI   │◄──►│  │  Wails   │◄───►│  HTTP + WS server  │    │
  │  (React)     │    │  │  bridge  │     │  /stream  /info    │    │
  └──────────────┘    │  └──────────┘     │  /ws      /web/    │    │
                      │       ▲           └─────────┬──────────┘    │
                      │       │                     │               │
                      │  ┌────┴───────┐   ┌─────────┴──────────┐    │
                      │  │  Virtual   │   │  Session +         │    │
                      │  │  Display   │◄──┤  Capture + Input   │    │
                      │  │  (CGVD/IDD)│   │  (CGEvent/XTest/  )│    │
                      │  └────────────┘   └─────────┬──────────┘    │
                      │                             │               │
                      │  ┌────────────────────────┐ │               │
                      │  │  USB Accessory (AOA)   │◄┘               │
                      │  └───────────┬────────────┘                 │
                      │              │                              │
                      │  ┌───────────┴────────────┐                 │
                      │  │  Discovery (UDP 37680) │                 │
                      │  └────────────────────────┘                 │
                      └─────────────────┬───────────────────────────┘
                                        │
                       ┌────────────────┼────────────────┐
                       │                │                │
                  Wi-Fi (WS +      USB cable (AOA     Wi-Fi (any
                  MJPEG + UDP)     binary frames)     browser)
                       │                │                │
              ┌────────▼─────────┐ ┌────▼──────────┐ ┌───▼─────────┐
              │ Android app      │ │ Android app   │ │  Browser    │
              │ (Capacitor       │ │ (Capacitor    │ │  webclient  │
              │  WebView)        │ │  + USB plugin)│ │  (embedded) │
              └──────────────────┘ └───────────────┘ └─────────────┘
```

Three transports share one envelope shape:

- **Wi-Fi:** UDP discovery beacon advertises the server; client connects to `/ws` (gorilla/websocket) for signaling, fetches `/stream` (MJPEG over multipart HTTP) for video, exchanges files as base-64 chunks over the same WS.
- **USB:** desktop uses `gousb` to switch the phone into AOA mode (vendor `0x18D1`, product `0x2D00`/`0x2D01`), then exchanges binary `[type][len][payload]` frames over bulk endpoints. Same hello/ready/touch semantics, no JSON.
- **Browser:** the embedded web client (`internal/stream/webclient/`) runs the same WS protocol and pair-code prompt as the native app.

## Why each module exists

| Path | Purpose |
|------|---------|
| `cmd/vior/` | Cobra-based CLI. `vior start`, `vior displays`, `vior display mirror/extend`, `vior usb setup/teardown/status`, `vior stop`, `vior version`. |
| `desktop/` | Wails v2 wrapper around the same Go server. `app.go` is the bridge the React frontend calls into (`StartServer`, `GetServerStatus`, `SendFile`, permissions, menu-bar toggle, etc.). |
| `mobile-cap/` | Capacitor 7 Android app. TypeScript UI in `src/`, custom `MainActivity.java` adds USB-accessory + WebView insets + camera permission handling. |
| `internal/protocol` | The WebSocket envelope (`type` + `data`) and all typed message structs (`Hello`, `Ready`, `Input`, `Resize`, `FileOffer/Chunk/Complete`, etc.). Single source of truth for the wire format. |
| `internal/virtual` | Virtual-display creation. macOS uses the private `CGVirtualDisplay` Objective-C API (reverse-engineered headers, stable since macOS 13) so we do not need a kext or signed driver. Linux uses `xrandr` + dummy driver. Windows uses an IDD driver. |
| `internal/capture` | Display enumeration and frame capture. macOS: `CGDisplayCreateImage` via cgo. Linux/Windows: `kbinani/screenshot`. Encodes to JPEG and pushes to a frame channel. |
| `internal/input` | Mouse + keyboard injection. macOS uses `CGEvent` (with Unicode + modifier-chord support and an Accessibility permission check). Linux uses `XTest`. Windows uses `SendInput`. |
| `internal/usb` | Android Open Accessory protocol implementation. Switches the device into accessory mode, opens bulk endpoints, runs a binary frame loop. No ADB, no USB debugging. |
| `internal/discovery` | UDP broadcast beacon on port 37680 every 2s carrying `{magic:"VIOR", name, port, platform}` so the mobile app can find the desktop zero-config. |
| `internal/trust` | Persistent trusted-device list at `~/.vior/trusted.json` (name, platform, last seen — shown in Settings). Admission always requires the pair code; the phone caches it so the user types it once. |
| `internal/stream` | HTTP + WebSocket server, MJPEG endpoint, embedded web client, CORS, the pair-code handshake. The single place that touches the network. |
| `internal/session` | `Configure(hello)` — decides display index + bounds for extend vs mirror mode and is shared between the Wails app, the CLI, and the USB path so they cannot drift. |
| `internal/filetransfer` | Chunked file transfer manager (48 KB chunks, ~5 ms throttle, SHA-256 integrity). Transport-agnostic — it asks the caller for a `Send` function. |
| `internal/adb` | Optional ADB autodownload + `adb reverse` setup for the legacy USB tethering mode (predates AOA). |
| `internal/config` | Defaults, port helpers (`FreePort`), version constant. |
| `internal/network` | QR code generation for the browser fallback URL. |
| `docs/site/` | Marketing site, deployed to GitHub Pages via `.github/workflows/pages.yml`. |

## Security model

Vior is designed for trusted LANs (home, office, hotspot). What is actually protected today:

- **Pair-code admission.** On every connection the phone sends `{deviceId, pairCode}` in its hello. The server admits the session only if `pairCode` matches the 6-digit code shown on the desktop (constant-time compare). A previously seen `deviceId` on its own is *not* enough — `~/.vior/trusted.json` (mode `0600`) is informational (name, platform, last seen) and the phone caches the code so you only type it once.
- **The pair code is stable per install.** It is derived from a per-install secret in `~/.vior/pair-secret` plus the machine ID, so it survives restarts. You can override it from Settings → Pair code (4–8 digits, stored in `~/.vior/pair.txt`); changing it is how you revoke every phone at once.
- **Brute-force throttling.** 5 wrong codes per minute per IP plus a global ceiling, after which the server answers `rate_limited` and closes the socket. The same limit applies to `/info?probe=` pairing probes.
- **File integrity.** Transfers SHA-256 the full payload, drop out-of-order chunks, and delete the file on a hash mismatch.
- **No cloud.** All traffic stays on your LAN. There is no telemetry, no analytics, no remote endpoint of any kind. Run `grep -ri "http" internal/ | grep -v test` to verify.

What is **not** protected yet:

- **Traffic on the LAN is still in the clear for the shipped clients.** The server implements an application-layer encrypted channel (X25519 key exchange → HKDF → XSalsa20-Poly1305 over the WebSocket, with screen frames delivered on the same sealed stream — see [`docs/secure-channel-implementation.md`](docs/secure-channel-implementation.md)), and a JavaScript initiator exists in [`clients/secure/`](clients/secure/). Neither the Android app nor the embedded web client has been switched over to it yet, so today every session negotiates down to plaintext: a peer on the same Wi-Fi can read frames, keystrokes and files. Treat Vior like AirDrop or Spacedesk: fine at home, not fine on a coffee-shop network. Wiring the clients to the secure channel is the top item in [`docs/production-readiness-2026-10.md`](docs/production-readiness-2026-10.md).
- **USB (AOA) sessions skip pairing entirely.** Physical access to the cable is the trust decision.
- **No per-device credential.** You cannot revoke one phone without changing the pair code for all of them.

## Permissions

Each platform asks for a small, predictable set of permissions:

- **macOS:**
  - *Screen Recording* (System Settings → Privacy & Security → Screen Recording) — required for `CGDisplayCreateImage` to return real pixels.
  - *Accessibility* (System Settings → Privacy & Security → Accessibility) — required for `CGEvent` to inject mouse and keyboard events. Without it the Remote tab looks healthy on the wire but every event is silently dropped; the desktop UI surfaces a permission card on the first failed input.
- **Linux:** no special grants; X11 display is needed (Wayland is not supported).
- **Windows:** no special grants for capture/input. The IDD virtual-display driver is opt-in and ships separately.
- **Android:**
  - `INTERNET`, `ACCESS_NETWORK_STATE`, `ACCESS_WIFI_STATE` — for the WS + MJPEG transport and discovery.
  - `CAMERA` — only for the QR scanner.
  - `RECEIVE_BOOT_COMPLETED` — optional, only if you opt into Settings → Auto-start on boot.
  - USB accessory intent — declared in the manifest, no runtime prompt; Android asks the user to allow the accessory the first time it is plugged in.

## Configuration

The Wails desktop and the CLI share one `config.Config`. Defaults live in [`internal/config/config.go`](internal/config/config.go):

| Setting | Default | Notes |
|---------|---------|-------|
| `port` | `0` (auto-select free port) | CLI: `--port` / `-p`. The Wails UI also picks free. |
| `host` | `0.0.0.0` | Bind address. |
| `frame_rate` | `30` | CLI: `--fps` / `-f`. |
| `quality` | `80` | JPEG quality 1–100. CLI: `--quality` / `-q`. |
| `discovery_port` | `37680` | UDP broadcast port. |
| `auto_discovery` | `true` | CLI: `--no-discovery` disables broadcasting. |
| `transfer_dir` | `~/Downloads/Vior` | Where received files are saved. |

CLI commands:

```bash
vior start                              # WS mode — waits for client, auto-creates virtual display on connect
vior start --port 8080 --fps 30 -q 80   # explicit
vior start --virtual-width 1920 \
           --virtual-height 1080        # legacy mode — create virtual display upfront
vior start --no-discovery               # disable UDP broadcast
vior start --no-websocket               # legacy MJPEG-only, no client signaling

vior displays                           # list connected displays
vior display mirror --source 1 --target 0
vior display extend 1
vior usb setup                          # adb reverse port forward (legacy USB path)
vior usb teardown
vior usb status
vior stop                               # interrupt the running 'vior start'
vior version
```

Persistent state files under `~/.vior/`:

- `trusted.json` — paired devices (mode `0600`).
- `menubar.flag` — macOS menu-bar toggle preference.

The mobile app stores its own preferences in Android `SharedPreferences` (`vior_prefs`): last server URL, orientation lock, boot-autostart, USB-vs-Wi-Fi preference.

## Building from source

Toolchain:

- **Go 1.26** (see `go.mod`; the default `GOTOOLCHAIN=auto` fetches the exact patch release automatically).
- **Wails v2.12.0** for the desktop GUI: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`.
- **Node + npm** for the desktop and mobile frontends (`desktop/frontend/`, `mobile-cap/`).
- **cgo** is required on macOS (CGVirtualDisplay, CGEvent, CGDisplayCreateImage) and Linux (XTest).
- **libusb-1.0** for the `gousb` AOA path (`brew install libusb` on macOS, `apt install libusb-1.0-0-dev` on Debian/Ubuntu).
- **Android SDK + JDK 21 + Gradle** for the mobile APK. CI uses `android-actions/setup-android@v3` and `./gradlew assembleDebug`. Capacitor regenerates the Gradle project from `mobile-cap/capacitor.config.json` on `cap sync`.

Common targets:

```bash
make build              # CLI binary into ./tmp/vior
make run                # build + run 'vior start'
make desktop            # build the Wails desktop app
make desktop-dev        # Wails hot-reload dev mode
make test               # go test ./...
make lint               # go vet ./...
make install            # go install ./cmd/vior

# Mobile (from mobile-cap/)
npm install
npm run build           # tsc emit
npx cap sync android
cd android && ./gradlew assembleDebug
```

CI ([`.github/workflows/build.yml`](.github/workflows/build.yml)) runs all three (CLI × 3 OSes, Wails × 3 OSes, Android APK) in parallel with `fail-fast: false` on every push and PR to `main`.

## Contributing

- File a bug or feature request via the [GitHub issue templates](https://github.com/subhashraveendran/Vior/issues/new/choose).
- Fork, branch off `main`, run `go fmt ./...` and `go vet ./...` before pushing.
- The branch convention used here is `<type>/<short-slug>` (`feat/`, `fix/`, `refactor/`, `chore/`, `docs/`, `ts/` for TypeScript migrations).
- TypeScript is gradually being adopted across `desktop/frontend/` and `mobile-cap/src/` — keep new code in TS where the surrounding files already are.
- See [CONTRIBUTING.md](CONTRIBUTING.md) for the full setup walkthrough.

## License

[MIT](LICENSE) © 2026 Subhash Raveendran.
