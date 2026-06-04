# Vior — master plan

Phase 2 status + remaining work + Phase 3 roadmap. Single source of truth.

## Shipped (Phase 2)

### Server (Go)
- Auto virtual display creation matching client resolution
- Extend mode (virtual display) + Mirror mode (capture main display)
- HelloMessage with `mode` + `pairCode` fields
- 6-char hex pair code generated at boot, exposed via `/info` + Wails `ServerStatus.PairCode`
- Touch / mouse / scroll / keyboard input injection (CGEvent/XTest/SendInput)
- macOS keyboard: Unicode + named keys + modifier chords (`Cmd+c` etc.)
- macOS cursor visibility safeguard (CGDisplayShowCursor on every MoveMouse)
- File transfer end-to-end (chunked base64 over WS, SHA-256, image previews)
- LAN discovery: UDP broadcast + HTTP `/info` subnet scan
- `internal/session/setup.go` — shared Configure() for both Wails + CLI
- CI: 7 parallel jobs (CLI / Desktop / Mobile across macOS / Linux / Windows + Android)
- libusb auto-install per platform (apt for Linux, GitHub release for Windows)

### Desktop (Wails + React)
- Sidebar nav: Server / Files / Settings
- Idle screen with brand-glyph radar + Start Server CTA
- Waiting screen with monaco URL + Copy + QR code + pair code
- Connected screen with stat grid + Display/Files/Mirror tabs + Disconnect
- Settings 1:1 with design: Stream quality / Resolution radio / Frame rate /
  Connectivity / Displays card / USB-ADB card / About / Appearance
- Appearance subscreen: Style / Accent / Density / Motion + Preview + Done
- CSS data-attr remaps for Precise/Instrument/Soft + Compact/Regular/Comfy +
  Expressive/Subtle/Off
- App icon: rendered 1024 PNG → Wails generates `iconfile.icns` in app bundle
- macOS Dock + Cmd-Tab + Finder show real Vior icon
- Pair code shown below URL in mono accent
- Permissions modal (Screen Recording prompt for macOS)
- Update banner + Error overlay

### Mobile (Capacitor + HTML/CSS/JS, single file)
- Geist + Geist Mono fonts, orange default accent, cool-dark #0b0d10 base
- Bottom tab bar (Display / Files / Remote) outside app-shell (escapes stacking)
- Android 15 edge-to-edge fix via WebView insets in MainActivity
- Mixed-content fix: snapshots via `fetch()` + blob URLs
- Discovery: HTTP subnet scan in 20-host batches + UDP-less radar
- Auto-connect to last-known server (localStorage `vior_last`)
- Manual IP + Pair code (optional) fields
- **QR scanner** — inline `BarcodeDetector` API + `getUserMedia` (no plugin,
  no Google Play Services required → works on Honor / Huawei tablets)
- Connecting overlay with spinner + Cancel
- Stream fullscreen with auto-hide overlays + reconnect with backoff
- Files tab: send/receive with progress bars + image previews + Save
- Remote tab: trackpad (1F/2F gestures) + click bar + soft keyboard + 20
  shortcut keycaps + Esc/Enter/Tab/arrows + F1-F12
- Settings full-page (not sheet): Stream quality / Connectivity / About /
  Appearance → matches desktop 1:1 with subscreen
- Adaptive app icon (vector glyph in 108×108) on launcher + monochrome alias
- Auto runtime CAMERA permission request + WebChromeClient resource grant
- Connection chip in header with ringed status dot

### Cleanup
- Deleted `mobile/` (Gio), `tmp/`, `docs/detailed_sprint_plan.md`
- Removed `gioui.org`, framer-motion (desktop)
- Single unified `build.yml` workflow
- README + `docs/phase-2.md` + `docs/master-plan.md`

---

## Known issues / gaps

| Area | Symptom | Mitigation | Fix priority |
|---|---|---|---|
| Mirror mode | macOS may re-arrange physical displays when virtual display destroyed | Document; user should re-arrange in System Settings | Medium |
| Mouse pointer | Can park on virtual display where macOS hides it | `CGDisplayShowCursor` after each move (in place) | Done partial |
| QR scanner | BarcodeDetector requires Chromium 88+ WebView | Toast fallback "Scanner unsupported" | Low |
| Pair code | Generated + transmitted but not yet enforced server-side | Optional Phase 3 hardening | Medium |
| Linux/Windows keyboard | Only US layout, no Unicode like macOS | Port `postUnicode` strategy | Medium |
| H.264 | Still MJPEG → 5-10× bandwidth vs Spacedesk | VideoToolbox / NVENC / VA-API encode pipeline | High |
| Audio | None | ScreenCaptureKit / WASAPI / PulseAudio + Opus | High |
| Clipboard sync | None | `MsgClipboard` + Capacitor Clipboard plugin | Medium |
| Multi-client | Server limits to 1 client | Broadcast frames, virtual display per client | Medium |
| Tests | Zero | Protocol round-trip, file-transfer chunking | Medium |
| iOS app | Android-only | Capacitor iOS target + cert wrangling | Low |

---

## Phase 3 roadmap

### Performance
1. H.264 over WebRTC or RTSP
   - macOS: VideoToolbox encoder
   - Linux: VA-API
   - Windows: NVENC / AMF / Media Foundation
   - Decoder: native `<video>` in mobile WebView, hardware accelerated
2. Adaptive bitrate based on RTT measurement
3. Frame-dropping on congestion

### Features
4. **Audio** — capture system audio, Opus over WebSocket
5. **Clipboard sync** — both directions, image + text MIME types
6. **Multi-client** — N phones, one Mac, per-client virtual display
7. **Stylus / pen pressure** — pressure + tiltX/Y on touch events
8. **iOS client** — Capacitor iOS target
9. **End-to-end encryption** — Noise protocol over WebSocket
10. **Pair-code enforcement** — server rejects hello with wrong code

### Polish
11. **Linux/Windows keyboard Unicode** — port macOS `postUnicode` path
12. **Reduced motion** — full audit; `prefers-reduced-motion` honored
13. **Light theme** — toggle in Settings + token swap
14. **Per-display capture** — pick which physical display to stream
15. **Display arrangement memory** — restore macOS layout after mirror stops

### Infrastructure
16. **Tests** — protocol round-trip, file-transfer chunking, session.Configure
17. **Auto-update** — Sparkle for macOS, MSI for Windows
18. **Code signing** — Apple Developer ID, Microsoft EV cert
19. **Notarization** — macOS notarization for Gatekeeper
20. **Crash reporter** — sentry-go on server side

---

## Test plan

### Manual
- [ ] Mobile discovery finds server (same Wi-Fi)
- [ ] Manual IP entry connects
- [ ] QR scan parses + connects
- [ ] Pair code field accepts 6-char hex
- [ ] Auto-connect on launch to last server
- [ ] Display tab → mode picker → Connect → stream
- [ ] Stream touch maps to Mac cursor
- [ ] Tab swap (Display / Files / Remote) preserves WS session
- [ ] Files: send photo, receive file from desktop
- [ ] Remote: trackpad move + click + scroll + 2-finger right-click
- [ ] Remote: shortcut Cmd+c triggers Mac copy
- [ ] Soft keyboard sends keys to Mac
- [ ] Settings: accent picker live-remaps all UI
- [ ] Appearance: Style/Density/Motion toggle visual change
- [ ] Reconnect on Wi-Fi drop, amber pill, exponential backoff

### Automated (Phase 3)
- Protocol round-trip integration
- file-transfer chunking + SHA-256 verification
- session.Configure with mocked capture/virtual
- CI smoke test: start server, curl /info, assert pairCode present

---

## Build / install workflow (always)

After every mobile change:
```bash
git push origin main
# wait for CI mobile job
gh run download <run-id> -n vior-mobile-Android -D /tmp/vior-apk
adb -s <serial> uninstall com.vior.mobile
adb -s <serial> install /tmp/vior-apk/app-debug.apk
adb -s <serial> shell am force-stop com.vior.mobile
adb -s <serial> shell am start -n com.vior.mobile/.MainActivity
```

After every desktop change:
```bash
cd desktop && wails build -clean -o vior-desktop
pkill -f vior-frontend
rm -rf /Applications/Vior.app
cp -R build/bin/vior-app.app /Applications/Vior.app
killall Dock                  # refresh launchpad icon
open /Applications/Vior.app
```
