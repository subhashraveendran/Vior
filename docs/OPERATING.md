# Vior — Operating Manual

End-to-end guide for installing, pairing, and using Vior (desktop + Android companion).

## 0. Build artifacts

| Component | Default install path |
|---|---|
| Desktop app (macOS) | `/Applications/vior-app.app` |
| Android APK         | `~/Downloads/vior-latest.apk` |

## 1. Launch desktop (macOS)

```bash
open /Applications/vior-app.app
```

First launch may be blocked by Gatekeeper ("unidentified developer"). Allow via:

- **System Settings → Privacy & Security → "Open Anyway"**, or
- `xattr -dr com.apple.quarantine /Applications/vior-app.app` then relaunch.

## 2. Install APK on Android

### Option A — `adb` (USB cable, fastest)

1. Enable *Developer options → USB debugging* on phone.
2. Plug phone into Mac.
3. ```bash
   adb devices                            # confirm phone listed
   adb install -r ~/Downloads/vior-latest.apk
   ```

If reinstall fails with `INSTALL_FAILED_UPDATE_INCOMPATIBLE`:

```bash
adb uninstall com.vior.mobile
adb install ~/Downloads/vior-latest.apk
```

### Option B — sideload (no adb)

1. Transfer APK to phone (AirDrop / email / Drive / `scp`).
2. Tap the APK on phone → allow *Install unknown apps* for the file manager → Install.

## 3. Pair phone ↔ Mac (Wi-Fi)

1. Launch `vior-app` on Mac. Main screen shows the **pair code** (default `9801`; override in Settings).
2. Ensure phone + Mac are on the same Wi-Fi subnet.
3. On phone: open Vior → Discovery screen auto-lists the Mac.
4. Tap the Mac entry → enter pair code → connected.

Available tabs once paired:

- **Stream** — mirror Mac display to phone
- **Remote** — trackpad + soft keyboard control of Mac
- **Files** — bi-directional file transfer

## 4. USB-AOA mode (cable, no Wi-Fi)

1. Plug phone → Mac via USB.
2. Mac side auto-detects via Android Open Accessory protocol. Orb transitions: scanning → verifying → connected.
3. Phone shows "Vior — USB connected".

**Limitation**: cable currently carries display only. Remote-tab input (trackpad / keys) still requires Wi-Fi. A one-shot toast on the phone surfaces this the first time you try to use Remote over USB.

Heartbeat: desktop sends `FramePing` every 5s; tears down on no `FramePong` for 10s. Stale "USB connected" states resolve on their own.

## 5. File transfers

| Direction | How |
|---|---|
| Mac → phone | Files tab → **Send** → pick file. Live progress bar fills as bytes flow. |
| Phone → Mac | Files tab → **+** → pick. Mac modal pops with thumbnail + Accept/Reject. |

- LAN throughput ceiling: ~48 MB/s (WS-chunked, 1ms inter-chunk yield).
- Hard size cap: `MaxDownloadSize` constant in `internal/filetransfer/filetransfer.go`. Oversize offers auto-reject before disk is touched.
- Filenames sanitized — traversal (`../`), control chars, Windows reserved names, leading dot/dash, and >255-byte names all neutralized.

## 6. Remote tab gestures

| Gesture | Action |
|---|---|
| One-finger drag | Move cursor — sub-linear acceleration (small=precise, fling=fast). |
| Tap | Left click |
| Two-finger tap | Right click |
| Two-finger drag | Scroll |
| Soft keyboard | Sends keys via `darwinKeyCodes` map |

Cursor warp-back guard: the cursor only snaps to the main display centre if it has wandered off **both** the host's main display and the captured virtual display — touching Remote on the active virtual display never yanks the cursor away.

## 7. Pair code override

Mac app → **Settings** → *Pair code*:

- **View** current code
- **Override** with any 4-digit value
- **Reset** back to the deterministic default

Persists across launches.

## 8. Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| Phone doesn't see Mac in Discovery | Different Wi-Fi subnets; or macOS firewall blocking. Verify listener: `lsof -iTCP -sTCP:LISTEN \| grep vior`. |
| USB orb stuck on "verifying" | Unplug, replug. Heartbeat (commit `7de9333`) auto-kills wedged sessions after 10s. |
| `adb install` fails with `UPDATE_INCOMPATIBLE` | Signature mismatch with previous build. `adb uninstall com.vior.mobile` first. |
| Desktop won't open ("damaged" / "unidentified") | Gatekeeper. `xattr -dr com.apple.quarantine /Applications/vior-app.app`. |
| File transfer "stuck at 0%" | Receiver hasn't accepted yet. Look for accept modal on the destination side. |
| Remote tab input silently no-ops on USB | Expected. Use Wi-Fi for trackpad + keys. AOA input-frame extension is a known follow-up. |

## 9. Rebuild from source

### Desktop (macOS)

```bash
cd "/Users/subhashraveendran/Documents/Source Codes/Vior/desktop"
wails build
rm -rf /Applications/vior-app.app
cp -R build/bin/vior-app.app /Applications/
```

### Android APK (CI route — preferred, no Android SDK needed locally)

```bash
gh run list --workflow Build --limit 1 --branch main           # grab latest run ID
gh run download <id> --name vior-mobile-Android --dir /tmp/apk
mv /tmp/apk/app-debug.apk ~/Downloads/vior-latest.apk
```

### Android APK (local — needs Android SDK + `npx cap`)

```bash
cd mobile-cap
npm run build
npx cap sync android
cd android && ./gradlew assembleDebug
cp app/build/outputs/apk/debug/app-debug.apk ~/Downloads/vior-latest.apk
```

## 10. Verification checklist

Before reporting a build as good:

- [ ] `go build ./...` — clean
- [ ] `go test ./internal/...` — green
- [ ] `cd desktop/frontend && tsc --noEmit` — clean
- [ ] `cd mobile-cap && npx tsc --noEmit -p tsconfig.json && npm run build` — clean
- [ ] Wi-Fi pair end-to-end: Discovery → pair code → Stream visible
- [ ] USB-AOA: orb reaches green, mirror visible
- [ ] File send Mac→phone: progress bar advances mid-stream (not 0→100 jump)
- [ ] Remote drag: small deltas are precise, long flings cover the screen
