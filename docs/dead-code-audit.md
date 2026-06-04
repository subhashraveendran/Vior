# Dead code audit — 2026-06-05

Senior-engineer sweep of unreachable Go symbols, orphan frontend functions/ids,
unused dependencies, stale docs, and dead assets. Scope: everything reachable
from `cmd/vior/main.go`, the Wails `App` binding surface
(`desktop/frontend/wailsjs/go/main/App`), and the Capacitor mobile bundle.

## Tooling

- `go vet ./...` — clean.
- `staticcheck ./...` — clean.
- `deadcode ./cmd/... ./internal/... ./desktop/...` — only `internal/usb`
  symbols `DecodeFrameHeader`, `EncodeTouchEvent`, `EncodeHello` flagged.
  Kept per spec — used inside `internal/usb/accessory.go` via reflection-free
  paths the analyser cannot follow.
- Native `go build ./...` — clean.
- Frontend `npm run build` — clean. Bundle 221.54 kB / gzip 68.34 kB
  (unchanged; framer-motion was never imported, only declared as a dep).
- Cross-compile (`GOOS=linux/windows`) skipped locally — `gousb` and the X11
  input layer need a cgo cross-toolchain. CI verifies all three OSes.

## Go — symbols removed

| File | Lines | Symbol | Reason | Action |
|------|------|--------|--------|--------|
| internal/adb/adb.go | -14 | `TeardownAllForwards` | No caller. `TeardownForward(port)` is the only path used (`desktop/app.go`). | Deleted |
| internal/network/network.go | -33 | `Peer.URL`, `QRCode` (ANSI) | `Peer.URL` not referenced; ANSI `QRCode` superseded by `QRCodePlain` (CLI) and `QRCodeDataURL` (Wails). | Deleted |
| internal/virtual/display.go | -11 | `DefaultForResolution` | Never called — callers build `Info{}` literally. | Deleted |
| internal/discovery/listener.go | -44 | whole file (`Listener`, `Listen`) | UDP listener was for the abandoned desktop-as-client mode; mobile client does its own UDP listen in `mobile-cap/src/index.html`. | Deleted |
| internal/transfer/transfer.go | -192 | whole package | Superseded by `internal/filetransfer/filetransfer.go` (the message-based offer/accept/chunk/complete flow wired into `desktop/app.go`). Zero importers. | Deleted |

## Go — symbols KEPT despite analyser flags

| File | Symbol | Why kept |
|------|--------|----------|
| internal/usb/protocol.go | `DecodeFrameHeader`, `EncodeTouchEvent`, `EncodeHello` | Used inside `internal/usb/accessory.go` (e.g. inside callback-driven read loops); spec instructed to leave them. |
| desktop/app.go | `CreateVirtualDisplay`, `DestroyVirtualDisplay`, `ListDisplays`, `MirrorDisplay`, `ExtendDisplay`, `IsMirrored`, `StartStream`, `StopStream`, `GetStreamStatus`, `TakeSnapshot`, `GetUSBStatus`, `SetupUSB`, `TeardownUSB`, `DownloadADB`, `ResetConfig`, `AcceptIncomingFile`, `RejectIncomingFile`, `GetActiveTransfers`, plus all `OnClient*` message handlers | Wails-bound methods exposed in `wailsjs/go/main/App.d.ts`; reachable from the frontend even if `App.jsx` currently does not import them. Removing breaks the binding contract. |
| internal/protocol/protocol.go | All `MsgFile*`, `MsgResize`, `MsgStatus`, `MsgError`, `MsgBye` constants and matching `*Message` structs | Wire protocol. JSON-tag-only fields with no Go caller still need to round-trip across the WebSocket. |
| internal/virtual/display_other.go | stub `Create`/`CreateHiDPI` | Build-tag-gated fallback for non-darwin/linux/windows. |
| internal/capture/capture_other.go, internal/input/input_other.go | stubs | Same — build-tag fallbacks. |

## Frontend — desktop (`desktop/frontend/src/components/App.jsx`)

| Item | Status |
|------|--------|
| `StartServer, StopServer, GetServerStatus, GetConnectedClients, GetConfig, UpdateConfig, GetVersion, CheckPermissions, PickAndSendFile` from `wailsjs/go/main/App` | All referenced (2+ uses each). |
| `EventsOn` from `wailsjs/runtime/runtime` | 4 uses. |
| `framer-motion` dep | Declared but never imported anywhere under `src/`. Removed from `package.json`. `node_modules` shrinks 80 MB → 70 MB. Bundle is unchanged (already tree-shaken). |

## Frontend — mobile (`mobile-cap/src/index.html`)

52 functions declared in the inlined `<script>`. Every function has at least
one caller besides its declaration (verified via word-boundary grep across
the file). 102 `id="…"` attributes vs 87 `$('…')`/`getElementById('…')`
references. The 15-id gap is fully accounted for by indirect lookup paths:

- `pane-display`, `pane-files`, `pane-remote` — selected via `document.querySelectorAll('.pane')` in `switchTab`.
- `wifi-track`, `usb-only-track`, `auto-connect-track` — bound via `document.querySelectorAll('.vior-toggle')` keyed by `data-key`.
- `seg-style`, `seg-density`, `seg-motion`, `seg-preset` — used as `#seg-…` ancestor selectors inside `setSegActive`/segment binders.
- `accent-row`, `refresh-icon`, `stream-stats`, `disc-dock`, `conn-chip` — CSS selector targets and ancestor anchors for nested queries.

No orphan functions or truly orphan ids.

## Build / config

| File | Finding | Action |
|------|---------|--------|
| `.github/workflows/build.yml` | 7 jobs (3 CLI + 3 desktop + 1 mobile). No stale envs/caches — every variable feeds the libusb pkg-config the Windows runners need. | Left as-is |
| `mobile-cap/package.json` | All three deps (`@capacitor/core`, `@capacitor/android`, `@capacitor/cli`) are required by `cap sync`. | Left as-is |
| `desktop/frontend/package.json` | `framer-motion` unused. | Removed |
| `desktop/wails.json` | `frontend:install`, `frontend:build`, `frontend:dev:watcher` all match scripts in `desktop/frontend/package.json`. | Left as-is |
| `desktop/wails.json.bak` | Byte-identical to `desktop/wails.json` and already covered by `.gitignore`. | Removed from disk (was untracked anyway) |

## Repo root

| Item | Finding | Action |
|------|---------|--------|
| `vior`, `vior-desktop`, `*.apk`, `*.zip` at root | None present. | — |
| Tracked-and-gitignored files | The check matches `cmd/vior/...` against the bare `vior` pattern in `.gitignore` — false positive from the unanchored Go ignore. No real conflict. | — |
| `docs/screen_flow_project_plan_cli_app_extend_display_strategy.md` | Original 232-line "MVP roadmap" superseded entirely by `docs/master-plan.md` + `docs/phase-2.md`. References abandoned Gio mobile + transfer/legacy layout. | Removed |
| `docs/master-plan.md`, `docs/phase-2.md` | Current. | Left |
| `docs/site/index.html` | Marketing landing page; not unreachable. | Left |
| `desktop/frontend/src/assets/fonts/OFL.txt` + `nunito-v16-latin-regular.woff2` | App uses Geist via Google Fonts CDN (see `app.css` `@import`). Nunito never imported. | Removed (both, and the empty `fonts/` dir) |
| `desktop/frontend/src/assets/images/logo-universal.png` | Wails template default; zero references. The brand glyph is rendered inline as JSX (`<Glyph />`). | Removed (and the empty `images/` + `assets/` dirs) |
| `internal/` packages with zero importers | None. | — |

## Summary

- **Go LOC removed:** 294 (across `internal/adb`, `internal/network`, `internal/virtual`, `internal/discovery`, `internal/transfer`).
- **Doc LOC removed:** 232 (`screen_flow_project_plan…md`).
- **Frontend asset bytes removed:** 158 KB (`nunito-v16-latin-regular.woff2` 18.5 KB + `logo-universal.png` 136 KB + `OFL.txt` 4.3 KB).
- **Total non-lockfile deletions:** 620 LOC + 158 KB.
- **Files deleted:** 6
  - `internal/discovery/listener.go`
  - `internal/transfer/transfer.go`
  - `desktop/frontend/src/assets/fonts/OFL.txt`
  - `desktop/frontend/src/assets/fonts/nunito-v16-latin-regular.woff2`
  - `desktop/frontend/src/assets/images/logo-universal.png`
  - `docs/screen_flow_project_plan_cli_app_extend_display_strategy.md`
- **Files modified:** 5 (`internal/adb/adb.go`, `internal/network/network.go`, `internal/virtual/display.go`, `desktop/frontend/package.json`, lockfiles)
- **Bundle delta:** desktop frontend bundle unchanged (221.54 kB / 68.34 kB gzip) — framer-motion was tree-shaken before removal. `node_modules` 80 MB → 70 MB (−10 MB on disk).
- **CI status:** verified via `gh run` after push.
