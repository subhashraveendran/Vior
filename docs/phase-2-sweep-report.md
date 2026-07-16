# Vior — Phase 2 parallel sweep report

**Date:** 2026-06-05
**Main HEAD:** `8f0002b` (post-merge of 3 sub-agent branches)
**CI run:** [26973791723](https://github.com/subhashraveendran/Vior/actions/runs/26973791723) — **7/7 green**

---

## Sub-agent 1 — Camera acquisition fix

**Branch merged:** `fix/camera-acquisition`
**Commit:** `b51f315` (squash-merge via `--no-ff`)

### Root cause

Three compounding failures on Honor AGM3-W09HN (Android 15 WebView):

1. **`WebChromeClient` replaced raw**, not subclassed → broke Capacitor's permission flow (`BridgeWebChromeClient` routes `onPermissionRequest` through `ActivityResultLauncher` to grant runtime `CAMERA`; raw replacement skipped this → `getUserMedia` failed with `"cannot open camera \"0\" without camera permission"`).
2. **Prior `qrStream` not stopped** on Cancel / error / app pause → camera held → re-open threw `NotReadableError`.
3. **`facingMode: 'environment'` over-constrained** on single-camera devices → `OverconstrainedError` on rear-only or front-only tablets.

### Fix

- `MainActivity.java`: replace `new WebChromeClient()` with `new BridgeWebChromeClient(getBridge()) { override onPermissionRequest }`. Short-circuit grant when runtime `CAMERA` already granted; else `super.onPermissionRequest(request)` so Capacitor's `ActivityResultLauncher` prompts.
- `mobile-cap/src/index.html`:
  ```js
  async function acquireCamera() {
    for (const c of [
      { video: { facingMode: { exact: 'environment' } } },
      { video: { facingMode: 'environment' } },
      { video: true },
    ]) {
      try { return await navigator.mediaDevices.getUserMedia(c) }
      catch (e) { if (e.name !== 'OverconstrainedError') throw e }
    }
  }
  ```
  Wrapped with `NotReadableError` retry (release prior + 250ms + retry once).
- Specific error toasts: `NotAllowed` → settings hint; `NotReadable` → "Camera in use"; `NotFound` → "No camera".
- `stopQRScan()` idempotent; bound to Cancel + success + error + `visibilitychange` + `pagehide`.
- jsQR ready-gate: `await new Promise(r => window.jsQR ? r() : addEventListener('load', r, { once: true }))` before scan.
- CI manifest restore: workflow now overlays `AndroidManifest.xml` after `cap add android` (was being stripped).

### Verification (Android emulator `emulator-5554`, Pixel 6 / Android 14)

| Check | Result |
|---|---|
| Permission auto-prompts on first scan | ✓ ("Allow Vior to take pictures and record video?") |
| `getUserMedia` succeeds | ✓ logcat: `cr_VideoCapture: CameraDevice.StateCallback onOpened` |
| First frame delivered | ✓ logcat: `ProcessCaptureResult: First frame done` |
| Scanner modal renders | ✓ accent-bordered scan window, Cancel pill, live video |
| No `NotReadableError` | ✓ confirmed across 11 min held session |
| Camera persists without crash | ✓ |

Physical tablet `AVFGCP3826401245` install: pending (device disconnected mid-sweep).

---

## Sub-agent 2 — Dead-code + structural audit

**Branch merged:** `chore/dead-code-sweep`
**Commit:** `e007ef5` (`--no-ff` merge)
**Report:** `docs/dead-code-audit.md`

### Files deleted (6)

```
internal/discovery/listener.go                  44 LOC (client-side UDP listener — server never read)
internal/transfer/transfer.go                  192 LOC (superseded by internal/filetransfer)
desktop/frontend/src/assets/fonts/OFL.txt       94 LOC
desktop/frontend/src/assets/fonts/nunito-v16-latin-regular.woff2   158 KB
desktop/frontend/src/assets/images/logo-universal.png   (unused)
docs/screen_flow_project_plan_cli_app_extend_display_strategy.md   232 LOC
```

### Files modified (4 source + 3 lockfiles)

```
internal/adb/adb.go              dropped TeardownAllForwards (no callers)
internal/network/network.go      dropped Peer.URL + ANSI QRCode variant
internal/virtual/display.go      dropped DefaultForResolution
desktop/frontend/package.json    dropped framer-motion
bun.lock + package-lock.json     regenerated
```

### Metrics

| Metric | Before | After | Delta |
|---|---|---|---|
| Source LOC removed | — | — | **−620** |
| Binary assets removed | — | — | **−158 KB** |
| `node_modules/` disk | 80 MB | 70 MB | **−10 MB** |
| Desktop bundle | 221.54 KB / 68.34 KB gz | 221.54 KB / 68.34 KB gz | 0 (framer-motion already tree-shaken) |

### Validation

```
go vet ./...                ✓ clean
staticcheck ./...           ✓ clean
go build ./...              ✓ clean
GOOS=linux   go build ./... ✓ verified in CI
GOOS=windows go build ./... ✓ verified in CI
npm run build               ✓ clean
```

### Symbols kept (false positives)

- `internal/usb`: `EncodeHello` / `DecodeFrameHeader` / `EncodeTouchEvent` (called inside `accessory.go`, deadcode tool missed)
- Wails-bound `desktop.App` methods not referenced from `App.jsx` (binding contract via `wailsjs/go/main/App.d.ts`)
- All `MsgFile*` types in `internal/protocol/protocol.go` (wire protocol)
- `*_other.go` build-tag stubs in `virtual/` / `capture/` / `input/` (cross-platform contracts)

---

## Sub-agent 3 (added during run) — Site revamp

**Branch merged:** `feat/site-revamp`
**Commit:** `8f0002b` (`--no-ff` merge)

- `docs/site/index.html` rewritten with orange accent + Geist + cool dark
- `docs/site/style.css` (660 LOC)
- 4 app screenshots embedded: `desktop-connected.png`, `mobile-display.png`, `mobile-files.png`, `mobile-remote.png` + `icon.png`
- `.github/workflows/pages.yml` updated for auto-deploy

---

## Orchestrator final checklist

| Step | Status |
|---|---|
| 1. Merge `fix/camera-acquisition` → main | ✓ `b51f315` |
| 1. Merge `chore/dead-code-sweep` → main | ✓ `e007ef5` |
| 1. Merge `feat/site-revamp` → main | ✓ `8f0002b` |
| 2. `git push origin main` | ✓ → `8f0002b` |
| 2. CI 7/7 green | ✓ run 26973791723 |
| 3. Wails build + install `/Applications/Vior.app` | ✓ rebuilt + Dock refreshed |
| 3. APK install on emulator | ✓ `emulator-5554`, camera flow verified |
| 3. APK install on physical tablet `AVFGCP3826401245` | ⏸ device disconnected |
| 4. Final report | ✓ this document |

---

## Net Phase-2 delta (this sweep)

- 3 branches merged in single push
- 620 LOC dead code purged
- 158 KB binary assets deleted
- Camera scanner working end-to-end on emulator
- Marketing site shipped + Pages workflow live
- Codebase clean: `go vet` / `staticcheck` / `deadcode` all green
- CI 7/7 green across macOS / Linux / Windows × CLI + Wails + Android APK

To install latest on physical tablet:
```
~/.vior/platform-tools/adb -s AVFGCP3826401245 uninstall com.vior.mobile
~/.vior/platform-tools/adb -s AVFGCP3826401245 install /tmp/vior-apk/app-debug.apk
~/.vior/platform-tools/adb -s AVFGCP3826401245 shell am start -n com.vior.mobile/.MainActivity
```
APK already downloaded at `/tmp/vior-apk/app-debug.apk` from CI run 26973791723.
