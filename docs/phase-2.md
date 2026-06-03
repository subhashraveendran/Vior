# Vior — Phase 2

Phase 2 takes Vior from a working extend-display prototype to a multi-feature
mobile companion: tabbed mobile app, file transfer, virtual trackpad, software
keyboard, mirror mode, USB Accessory transport, and a unified CI pipeline.

## What shipped

### Server (Go)

- **Mirror mode** in addition to Extend. `HelloMessage.Mode = "mirror" | "extend"`.
  Mirror captures the main display directly (no virtual display); Extend creates
  one matching the client's resolution.
- **Shared session setup** (`internal/session/setup.go`). One `Configure()` call
  handles permission check, virtual-display lifecycle, display lookup, bounds.
  Replaces ~80 duplicate lines in `desktop/app.go` and `cmd/vior/cli/start.go`.
- **Remote mouse** (`internal/input/touch.go::HandleMouse`). Relative `move`
  with `dx/dy`, plus `click` / `rightclick` / `middleclick`. Backed by
  `Controller.CurrentMousePos()` implemented on macOS (CGEventGetLocation),
  Linux (XQueryPointer), Windows (GetCursorPos).
- **Real keyboard injection on macOS**. `TypeKey` now uses
  `CGEventKeyboardSetUnicodeString` for printable characters and a virtual
  keycode table for named keys (BackSpace, Return, Tab, arrows, F1-F4, etc.).
  Previous implementation always sent keycode 0.
- **Bidirectional file transfer** wired end-to-end in mobile + desktop +
  protocol. Chunked base64 (48KB) over WebSocket with SHA-256 integrity,
  image previews, auto-receive directory at `~/Downloads/Vior/`.

### Mobile (Capacitor + HTML/CSS/JS)

- **Bottom tab navigation**: Display · Files · Remote. Persistent across
  connection state. Inactive tabs show "Connect first" placeholders.
- **Display tab**: discovery (UDP-less HTTP subnet scan, 20-host batches),
  mode picker (Extend/Mirror), connected-state info card with mode,
  resolution, status stats, View Stream + Disconnect actions.
- **Stream fullscreen overlay**: device name + mode badge, auto-hiding
  controls (3.5s), reconnect pill with exponential backoff (5 attempts).
- **Files tab**: native file/photo pickers, send-offer / accept-reject UI for
  incoming files, per-transfer progress bars, image thumbnails, Save link
  for received blobs.
- **Remote tab (trackpad)**: 1-finger move + tap-click, 2-finger scroll +
  tap-rightclick, click / right-click buttons, soft-keyboard popup that
  forwards keys including BackSpace/Enter/Tab/arrows via hidden input.
- **Mixed-content fix**: snapshots fetched via `fetch()` → blob URL
  (Capacitor patches `fetch` to bypass `https://localhost` →
  `http://server` restriction). Old `<img src="http://...">` was silently
  blocked by Chromium.
- **Android 15 edge-to-edge fix**: `MainActivity.onCreate` installs a
  `ViewCompat.setOnApplyWindowInsetsListener` so the WebView is margined
  above the system navigation bar; otherwise the bottom tab bar was
  visually present but uninteractive (system UI ate touches).
- **Design tokens** aligned with desktop: same palette, engraved double-shadow
  cards, dot-grid background, indigo glow on active state, radar animation
  on discovery, toast notifications.

### CI / Build

- One workflow (`.github/workflows/build.yml`) builds three CLI binaries
  (`vior-cli-{macOS,Linux,Windows}`), three Wails desktop apps
  (`vior-desktop-{macOS,Linux,Windows}`), and one Android APK
  (`vior-mobile-Android`), `fail-fast: false`, all in parallel.
- Linux libusb via apt (`libusb-1.0-0-dev`); Windows libusb via GitHub
  release + auto-generated `.pc` file + `CGO_CFLAGS/LDFLAGS` exports.
- Wails Linux pinned to `ubuntu-22.04` (Wails v2 hardcodes
  `webkit2gtk-4.0`; 24.04 only ships 4.1).
- Capacitor APK regenerated each run from `mobile-cap/src/` + overlay
  of custom Java + manifest + xml.

### Cleanup

- Deleted `mobile/` (deprecated Gio app — abandoned due to EGL_BAD_SURFACE
  on this device's GPU stack).
- Deleted `tmp/` and `docs/detailed_sprint_plan.md`.
- Removed `gioui.org` from `go.mod`.
- Extended `.gitignore` to cover `desktop/build/`, `desktop/frontend/dist/`,
  Capacitor generated dirs, and stray binaries.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│ Phone (Capacitor WebView)         Desktop (Wails + React)    │
│  ┌──────┬──────┬──────┐            ┌──────────────────────┐  │
│  │ Disp │ Files│Remote│  ◄────┐   │  Idle ▸ Waiting       │  │
│  └──────┴──────┴──────┘       │   │       ▸ Connected     │  │
│         ▲ WS                  │   └──────────┬───────────┘  │
│         │   /info             │              │ Wails IPC     │
│         │   /snapshot         │              │               │
│         │   /ws ◄─────────────┴──────────────┤               │
└─────────┼─────────────────────────────────────┴──────────────┘
          │                                     │
          │              ┌──────────────────────┴───────────┐
          │              │  Server (Go)                     │
          │              │  internal/                       │
          │              │   ├─ stream  (MJPEG + WS + CORS) │
          │              │   ├─ session (Configure)         │
          │              │   ├─ capture (CG/X11/IDD)        │
          │              │   ├─ virtual (CGVirtualDisplay,  │
          │              │   │            xrandr, IDD)      │
          │              │   ├─ input   (CGEvent/XTest/     │
          │              │   │            SendInput)        │
          │              │   ├─ filetransfer                │
          │              │   ├─ usb (gousb / AOA)           │
          │              │   └─ discovery (UDP broadcast)   │
          │              └──────────────────────────────────┘
          ▼
       Snapshot JPEGs (Wi-Fi)
       Binary frame protocol (USB)
```

## Protocol additions (Phase 2)

| Message            | Direction      | Notes                                |
|--------------------|----------------|--------------------------------------|
| `hello.mode`       | client → server| New field: `"extend"` (default) or `"mirror"` |
| `input.event=mouse`| client → server| `action: move` w/ `dx`/`dy` (relative); `click`, `rightclick`, `middleclick` |
| `input.event=key`  | client → server| Now actually injects on macOS for printables + named keys |
| `file-offer`       | both           | Bidirectional now (was server→client only on desktop side) |
| `file-accept` / `file-reject` / `file-chunk` / `file-complete` | both | Same shape as Phase 1, wired both directions |

## Known gaps (deferred to Phase 3)

| Area               | Status                                                   |
|--------------------|----------------------------------------------------------|
| H.264 video        | Still MJPEG. ~5–10× more bandwidth than Spacedesk.       |
| Audio forwarding   | Not implemented. Spacedesk forwards system audio.        |
| Clipboard sync     | Not implemented. Could use Capacitor Clipboard plugin.   |
| Multi-device       | Server allows one client at a time.                      |
| Stylus / pressure  | Touch only; no pen pressure metadata.                    |
| Linux/Windows TypeKey | macOS Unicode fix only — Linux uses XStringToKeysym, Windows uses naive `uint16(rune)`. Should match macOS path. |
| USB AOA end-to-end | Code in place but not exercised on a real device yet.    |
| Reconnect on server-side | Mobile retries; server doesn't preserve session.    |

## Phase 3 candidates (priority order)

1. **H.264** via VideoToolbox (macOS), NVENC/AMF (Windows), VA-API (Linux).
2. **Audio** — capture system audio (ScreenCaptureKit on macOS, WASAPI on
   Windows, PulseAudio on Linux) and stream over Opus.
3. **Clipboard sync** — `MsgClipboard` both directions, Capacitor
   `Clipboard` plugin on mobile.
4. **Multi-client** — server keeps a list, broadcasts to each, virtual
   display per client.
5. **Linux/Windows keyboard parity** — port the macOS Unicode strategy.
6. **Stylus / pen pressure** — extra `pressure`, `tiltX`, `tiltY` fields on
   touch events; map to OS tablet APIs.
7. **Tests** — protocol round-trip, file-transfer chunking, session.Configure
   with mocked capture/virtual.

## File map (key paths)

```
cmd/vior/                  CLI entry (cobra)
desktop/                   Wails desktop app
  app.go                   Frontend bridge, server lifecycle, file transfer
  main.go                  Wails options, window, embed assets
  frontend/                React + Vite UI
internal/
  capture/                 Screen capture per OS
  config/                  Config + ports
  discovery/               UDP broadcast + LAN IP detection
  filetransfer/            Chunked transfer manager
  input/                   Mouse + keyboard injection per OS
    touch.go               TouchMapper + HandleMouse (Remote)
  network/                 QR code helper
  protocol/                WebSocket message types + session
  session/                 Configure() — shared setup logic
  stream/                  MJPEG + WS server + CORS + /info + /snapshot
    webclient/             Embedded fallback web client
  usb/                     Android Open Accessory protocol (gousb)
  virtual/                 Virtual display creation per OS
mobile-cap/
  src/index.html           Entire mobile app (single file)
  android/app/src/main/    Custom Java + manifest (overlays CI-regenerated)
docs/                      This file + screen flow plan
.github/workflows/build.yml  Unified CI
```
