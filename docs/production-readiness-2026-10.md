# Vior — production readiness review (October 2026)

Full-application review: dependencies, security posture, desktop and mobile
UX, backend completeness, and how comparable products solve the same
problems. This supersedes the "Known issues / gaps" table in
`master-plan.md`, which had drifted from the code.

Method: `govulncheck` + `npm audit` on every manifest; a file-by-file read of
`desktop/frontend/src`, `desktop/app.go`, `mobile-cap/src`, the Android Java
layer, `internal/**` and `cmd/**`; and a diff of every plan/audit document
against the code. Items marked **(verify on device)** were traced from source
but not reproduced on hardware.

---

## 1. Where the product stands

| Area | State |
|---|---|
| Happy path (desktop Start → QR/pair → phone connected → stream + trackpad + files) | Works on macOS + Android over Wi-Fi |
| Dependencies | Clean. Go 1.26.8 + current `golang.org/x/*`; npm production and dev deps at 0 findings; Dependabot enabled |
| CI | Builds CLI ×3, Wails ×3, APK; vet, staticcheck, gofmt, govulncheck, npm audit, type-checks |
| Security | Pair-code admission + throttling solid. Encrypted channel implemented server-side, **not used by any shipped client** → all traffic cleartext today |
| Desktop UX | Happy path fine; several controls are dead or lie about state (§3) |
| Mobile UX | Connect loop, file transfer and reconnect fixed; stream trap, USB and touch mapping still open (§4) |
| CLI | Core `vior start` works; legacy modes and `virtual`/`display` subcommands are broken or ineffective (§5) |
| Linux / Windows | Capture + input real; virtual display unreliable (Linux) or **dangerous** (Windows) (§6) |
| Release engineering | Unsigned artifacts, no auto-update, no Play Store, no config persistence, no log file (§7) |

**Verdict:** a solid beta for macOS + Android on a trusted home network. Not
production-ready for a general audience until the P0 items below are closed.

---

## 2. Fixed in this pass

- Go toolchain 1.25.6 → 1.26.8; `golang.org/x/{crypto,net,sys,text,image}` to current. Closes all 18 reachable `govulncheck` findings (15 stdlib, 3 in `x/image`).
- `mobile-cap`: vite 5 → 8, `@types/node` 22; transitive dev-dep fixes. `desktop/frontend`: vite 8.3, React 19.3, plugin-react 6.1. All audits at 0, including dev.
- Dependabot alerts + security updates enabled on the repo; `.github/dependabot.yml` added (gomod, 3× npm, github-actions; weekly and grouped, except `/npm/cli` which is monthly).
- CI: `go-version-file: go.mod`, Node 22, staticcheck 2026.2.1, `govulncheck` gate, `npm audit --omit=dev --audit-level=high` gate, `npm ci`.
- **Mobile connect loop** (`connect.ts`): a rejected pair code no longer triggers 5 automatic retries with an empty code (which hit the server's 5/min limit and locked the user out); Cancel actually cancels; the 15 s timeout no longer restarts the overlay; the code that worked is cached per server so a relaunch can reconnect (the server requires the code on every connect — a remembered deviceId was never enough). `rate_limited` gets its own message; the `occupied` message no longer tells the rejected device it was "replaced".
- **Desktop**: clicking a quality preset no longer resets the bound port to 0 (which broke the URL, QR, USB port-forward and discovery beacon). Dead "auto-accept USB devices" toggle removed. Port-in-use on Start no longer leaves the server stuck as "running" (+ regression test).
- Web client: pair input accepted only 4 digits against a 6-digit code; its LAN scan compared against a field the server stopped publishing. Both fixed (`/info?probe=`).
- Pair-code copy aligned to 6 digits (README, Settings, Waiting, server comment). README security model rewritten to match the code. `vior version` prints the real toolchain.
- `internal/filetransfer/test_out_of_order.go` was compiled into the production binary (no `_test` suffix) and asserted the pre-fix corrupting behaviour; replaced by a real test of the rejection path.
- Housekeeping: stale `bun.lock`, scratch `test_port.go`, four merged agent worktrees.

**Second pass — connection layer and file transfer** (details and the target design in [`connection-architecture.md`](connection-architecture.md)):

- Phone→desktop file transfer never succeeded (empty hash was treated as corruption and the file deleted); desktop→phone 403'd every file outside `~/Downloads/Vior`; any photo over ~96 KB killed the WebSocket (full image in the offer); Android "Save" wrote nothing. All four fixed; Android now saves through the system DownloadManager **(verify on device)**.
- Reconnect after a silent Wi-Fi drop got "occupied" and gave up: the slot is now claimed after admission and a same-device hello evicts the stale session; rejected peers no longer run the disconnect handler (which tore down the real client's display). App-level pings now count as liveness.
- Mirror/extend switch did nothing (`ResizeMessage` had no `mode`); rotating a remote-only phone created a virtual display; the CLI streamed the whole desktop to remote-only clients and cast dimensions unchecked. One shared resize path now carries intent and mode on desktop and CLI; the desktop UI shows a session card instead of a display grid for remote/files sessions and sends device identity on every connect.
- Snapshot polling no longer spins forever on a session with no frames; reconnect banner clears; pair-code search probes 8081 too; keepalive listener leak fixed.

---

## 3. Desktop app — open items

**P0**
1. **Settings are not persisted.** `NewApp` always starts from `config.Default()`; nothing loads or saves a config file (`desktop/app.go`, `internal/config/config.go` has yaml tags but no loader). Quality/fps reset every launch while menu-bar and pair code persist. Add `~/.vior/config.json` load/save; validate on load.
2. **Permission "Fix"/"Open Settings" buttons are plain `<a href="x-apple.systempreferences:…">`** (`Connected.tsx`, `Permissions.tsx`). WKWebView does not navigate custom schemes; use Wails `runtime.BrowserOpenURL`. Same for the "Check for updates" link. **(verify on device)**
3. **Screen Recording is only checked when a phone connects** (`app.go` OnClientConnect). First-time users see a black phone screen before any prompt. Check at Start; say that macOS needs an app restart after granting.
4. **Encryption state is never shown.** `ServerStatus.Secure`/`SecureMode` are computed and ignored by the UI. Must be visible before/after the secure channel ships (lock icon + "Unencrypted" badge).
5. ~~Device identity on the trust card is just a self-chosen name.~~ Fixed in the second pass: the connect event now carries IP, platform, deviceId and intent on every path. The card itself should still render them (`Connected.tsx`).

**P1**
6. Quality changes only apply on next connect/resize; the Connected screen shows the *config* fps, not the streaming fps. Either restart capture live or label "applies on next connection".
7. "Connection lost" dialog, "Reconnecting" chip and the update banner are unreachable dead code (`App.tsx` never sets `errorState`/`showUpdate`); the banner hardcodes "Vior 2.1 … H.264". Delete or wire to a real version check (`GetVersion` is fetched and discarded).
8. Extend/Mirror buttons hardcode display indexes and swallow errors (`App.tsx` `onModeExtend/onModeMirror`).
9. File transfer: drop zone ignores drops; outgoing transfers have no UI (`download:*` events unhandled); "File sent" fires on offer and on picker cancel; incoming rows freeze at partial % on disconnect; history lost on tab switch (state lives in `FilesPane`, lift to `App`).
10. Saved appearance (style/density/motion) is applied only after opening Appearance; boot applies accent only.
11. Errors are raw Go strings ("listen tcp … bind: address already in use"). Map the common ones (port busy → "Port 8080 is in use; pick another in Settings"; no interface → "Connect to Wi-Fi"). `ResolvePort` remembers a port without re-checking it.
12. Waiting screen: no "can't find it?" help (firewall, AP isolation, VPN); promises discovery even when `status.discovery` is off; QR failure shows "QR loading…" forever; `localhost` fallback is encoded into the QR silently.
13. `GetServerStatus` runs `adb.Check()` (spawns `adb`) every 3 s poll; result never displayed.
14. Only disconnect action also stops the server and does not forget the device; add "Disconnect and forget".

**P2 (a11y + polish)**
15. Toggles lack `role="switch"`/`aria-checked`; segmented buttons lack `aria-pressed`; icon buttons lack names; no `:focus-visible`; `--text-3` (#626c79 on #0b0d10 ≈ 3.6:1) used for 11 px labels; toasts auto-dismiss in 3.5 s even for long fix instructions; `Seg` defined inside render loses focus on each click.
16. "Launch at login" permanent "Coming soon"; About says "Phase 2" with no version; `window.confirm` on pair reset; fake `QR.tsx` unused; refresh rate hardcoded 60 Hz; bound-but-unused `StartStream/CreateVirtualDisplay/TakeSnapshot/SetupUSB`.

---

## 4. Mobile app — open items

**P0**
1. **Full-screen stream can trap the user.** `touchstart` `preventDefault` on `#stream-img` suppresses the click that toggles the overlay; after 2.8 s the bars hide and cannot return. Hardware Back (`main.ts`) does not check `#stream-fs.active` and calls `exitApp()`. **(verify on device)**
2. **Touch mapping is wrong under letterboxing.** `mapT` scales against the `<img>` box while CSS uses `object-fit: contain`. Map against the rendered content rect.
3. **USB is display-only, and frames > 64 KB are probably dropped.** `sendInput` writes only to `ws` (null over USB); nothing calls `Android.sendTouch`. `UsbAccessoryPlugin.readLoop` does one 64 KiB read per frame; the desktop writes whole JPEGs. Needs a reassembly buffer on the phone (the desktop side already has one). **(verify on device)**
4. ~~"Save" on received files likely does nothing in the APK.~~ Fixed in the second pass (system `DownloadManager` via `Android.downloadFile`). Still open: a download the user cancels from the system notification sends no completion broadcast, so the row stays "receiving" — reconcile pending ids on resume. **(verify on device)**
5. **Secure channel not used.** `clients/secure/vior-secure.js` is never loaded by `index.html`; only its `.d.ts` exists in `src/js`. See §8.

**P1**
6. ~~Mirror/Extend switch while connected is a no-op.~~ Fixed in the second pass (`ResizeMessage.mode`).
7. Stream quality presets (`#seg-preset`) and "Verbose console" have no handler; most settings toggles default to ON visually (`def='1'`) while their real state is off; Wi-Fi/USB-only toggles write before the "disconnect first" guard.
8. Auto-launch on boot cannot work on Android 10+ (`BootReceiver` starts an activity from background).
9. ~~Reconnect banner never clears on `ready`; stream spinner polls `/snapshot` forever.~~ Fixed in the second pass.
10. ~~Pair-code-only search probes port 8080 only.~~ Fixed; still open: cancelling the search does not stop it.
11. Local-IP detection relies on WebRTC ICE; Chromium hides it behind mDNS → likely "Not on Wi-Fi" on a working network. Add a tiny native `ConnectivityManager` helper. **(verify on device)**
12. USB attach while app is open is missed (`singleTask` + no `onNewIntent`; scan runs once 1 s after `onCreate`).
13. Files: auto-accepts downloads up to 2 GB in memory from any known server; outgoing sends read whole file to RAM, ignore `bufferedAmount`, no cancel.
14. Remote tab over USB silently drops input (no mouse/scroll/key frames in the cable protocol); the "limitation banner" is a one-shot toast.
15. One `confirm()` left (`settings.ts` Paste URL); raw exception text in toasts (`qr.ts`, `files.ts`); pair prompt accepts a 1-digit code.

**P2**
16. `user-scalable=no`; `<span>` toggles with no role/tabindex/label; icon buttons without `aria-label`; tap targets of 26–30 px (`.sheet-close`, `.cascade-escape`, "Forget"); `--text-3` contrast; insets probably applied twice and `ime()` insets ignored under edge-to-edge **(verify on device)**.
17. Ship hygiene: `webDir: "src"` ships `.ts` sources + untracked compiled `.js` (stale JS if `cap sync` runs without `npm run build`); `manifest.json` referenced but missing; fonts from Google CDN (fails offline, contacts Google each launch); `versionCode 1`/`versionName 1.0` vs package.json 0.1.0; no release `signingConfigs`; `minifyEnabled false`; `usesCleartextTraffic` + `allowMixedContent` required until §8 lands.
18. Shortcut labels are Mac-only (⌘, Spotlight, Quit).
19. The embedded web client (`internal/stream/webclient`) has drifted into a separate product: own storage keys, no keepalive, no QR, no cascade. Decide: retire it (keep a "download the app" page) or bring it to parity.

---

## 5. Backend and CLI — open items

**P0**
1. **CLI legacy mode cannot stream to a phone.** `runLegacyMode` passes a nil handler → `/ws` never registered → `frameClientIP` empty → `authorizeFrameRequest` 403s every non-loopback request. `vior virtual setup/create` tell users to use exactly this mode. Desktop `StartStream` has the same bug.
2. **macOS `vior virtual create` / `vior display mirror|extend` do nothing that lasts** — the `CGVirtualDisplay` lives in a process-static and mirror uses `kCGConfigureForAppOnly`, so the effect dies when the CLI exits. `vior virtual destroy` runs in a fresh process with nothing to destroy. Either make these long-running (`--hold`) or remove them.

**P1**
3. CLI file transfer handlers are no-op stubs (`start.go` `OnClientFile*`): the phone waits forever after an offer. (The CLI now honours Remote-only/Files-only intents.)
4. `SetSecurityMode` has no caller: `SecureRequired` is unreachable in a shipped build. No per-connection message rate limit remains open from the July audit (the pre-auth slot claim was fixed in the second pass).
5. Logging: stdlib `log` to stderr only, no levels, no file. A packaged `.app` user cannot produce a log. Add `~/.vior/logs/` with rotation and a "Reveal logs" button.
6. CLI: `cliSessionHandler` has no mutex (shutdown races); `vior stop` fails on Windows (`os.Interrupt`); PID file in shared temp, 0644. (Resize now goes through `Configure`, so the unchecked `uint32` cast and ignored `Start()` error are gone.)
7. ADB: no timeouts on `exec.Command`; `adb devices` + `getprop` spawned on every desktop status poll; unpinned platform-tools `latest` download with no checksum; old install deleted before extraction succeeds.
8. `Config` is half-real: yaml tags, no loader/saver; `TransferDir` ignored (hardcoded `~/Downloads/Vior`); `--verbose` never read; `UpdateConfig` unvalidated (now narrowed).
9. UDP discovery beacon broadcasts every 2 s but nothing reads it (the WebView cannot); it also lacks `deviceId`/`secure`. Keep only if a native listener is planned; otherwise drop and simplify docs.
10. macOS capture uses `CGDisplayCreateImage` via `dlsym` — obsoleted in macOS 15. Plan a ScreenCaptureKit path before it is removed.

**P2**
11. No HTTP write timeout; package `init` creates `~/.vior/*` secrets and starts a goroutine even for `vior version`; handler errors echoed verbatim to the peer; no `recover` in `pumpSecureFrames`/file-progress goroutines; Linux cleanup file at a predictable `/tmp` path following symlinks; Xlib `Display*` shared across goroutines without `XInitThreads`; `Version` is a constant (no ldflags); `MJPEGServer`/`Accessory` cannot be restarted (stop channels never recreated — callers construct new instances today).
12. Tests: none for `adb`, `config`, `discovery`, `machineid`, `network`, `virtual`, `cmd/vior/cli`. Most valuable: nil-handler server serving `/stream` to a LAN peer (catches P0-1), `cliSessionHandler` with a fake controller (`Mode="none"`, negative resize, file-offer gets a reject), parser table tests for `machineid`/`adb`.

---

## 6. Platform truth table

| | macOS | Linux (X11) | Windows |
|---|---|---|---|
| Capture | Real (obsoleted API, see §5-10) | Real; Wayland unsupported | Real (GDI) |
| Input | Real, Unicode + chords | Partial: no chords/shift/symbols/Unicode (`XStringToKeysym` only); silent no-op if `XOpenDisplay` fails | Real; non-BMP truncated |
| Virtual display | Real, in-process | Needs `xf86-video-dummy` + xrandr; display ID unresolvable (`FindDisplayIndexByID` stub → "last display" guess) | **Not virtual: `Create` reconfigures the first non-primary adapter with `CDS_UPDATEREGISTRY` and `Destroy` restores to the registry mode it just wrote → can permanently change a real monitor's resolution. P0.** |
| Mirror | Real (OS-level lost on exit) | Stream-only | Stream-only |
| USB / AOA | Code present, untested on hardware | Same | Same + needs WinUSB driver |
| Discovery | HTTP sweep works; UDP beacon unread | Same | Same |
| Tray | Real | No-op | No-op |
| Accessibility check | Real | Always `true` | Always `true` |

---

## 7. Release engineering gaps

- No code signing / notarization (macOS Gatekeeper warning on every install; the Makefile strips quarantine locally). No Windows Authenticode. No Play Store listing; APK is a debug build.
- No auto-update, and no version check at all (the update banner is hardcoded fiction).
- `make release` is CLI-only and per-platform by hand; desktop artifacts only come from CI uploads, not GitHub Releases.
- No persisted config, no log file, no crash reporting.
- Version strings: `config.Version` constant, APK `versionCode 1`, npm CLI pinned to `v0.2.0` release that must exist for `npx vior` to work.

---

## 8. Security: the one decision that matters

Today the server offers `SecurePreferred`: it will run the X25519 → HKDF → XSalsa20-Poly1305 channel (frames included) if the client starts the handshake, and silently falls back to plaintext otherwise. No shipped client starts it, so 100 % of real sessions are cleartext, the UI says nothing, and the Android manifest keeps `usesCleartextTraffic`.

Order of work:
1. Load `clients/secure/vior-secure.js` (+ vendored tweetnacl) in the Capacitor app; initiate the handshake in `doConnect` when `/info.secureVersion` matches; decrypt frames on the sealed stream instead of polling `/snapshot`. Measure per-frame decrypt cost on a mid-range Android (the open question from `transport-security-reference-comparison.md`).
2. Show lock state on both ends (desktop `ServerStatus.Secure`; mobile header chip).
3. Flip the default to `SecureRequired`, expose it in Settings, delete `SecureOff`.
4. Drop `usesCleartextTraffic`, `allowMixedContent`, `cleartext: true`.
5. Then decide the web client's fate (it needs the same JS initiator; the fragment-carried `#k=` secret already exists for it).

---

## 9. How comparable products do it

| Product | Host OS | Client | Video | Transport security | Pairing | Virtual display | Extras | Model |
|---|---|---|---|---|---|---|---|---|
| **Vior** | macOS / Linux / Windows | Android app, any browser | MJPEG over HTTP (+ sealed WS path, unused) | App-layer E2E implemented server-side; **cleartext in practice** | 6-digit code + QR, LAN subnet scan | macOS `CGVirtualDisplay` (no driver); Linux dummy; Windows broken | Trackpad, keyboard, files, USB AOA | MIT, no account |
| Spacedesk | Windows only | Android / iOS / browser | H.264/HEVC hardware | None (LAN assumed) | IP / auto-discovery | Own IDD driver | Multi-client (3 displays), audio, USB tethering | Free with ads; paid Pro |
| Duet Display | macOS / Windows | iPad / iPhone / Android | H.264 hardware, 60 fps | TLS; account required | Account login | Own driver | Wired USB + wireless, audio, remote desktop | Paid / subscription |
| Luna Display | macOS / Windows | iPad / Mac | GPU-accelerated | Local only | Dongle presence | Hardware dongle (real display) | ~16–24 ms latency | $129 one-time |
| Apple Sidecar | macOS | iPad | H.264/HEVC, 60 fps | Encrypted peer-to-peer (AWDL) | Same Apple ID + device whitelist | Built-in | Apple Pencil, audio | Free, Apple-only |
| Deskreen | macOS / Linux / Windows (Electron) | Any browser | WebRTC (VP8/H.264) | **E2E in JS (node-forge) on an insecure origin** | QR / link | None (needs dummy plug) | Multi-client, app sharing | AGPL, free |
| Weylus | Linux-first (Rust) | Any browser | Hardware encode via ffmpeg | Optional access code; no E2E | Access code | xrandr/dummy | Stylus pressure/tilt, multitouch | AGPL, free |
| Sunshine + Moonlight | Windows / Linux / macOS | Native on everything | H.264 / HEVC / AV1 hardware, <10 ms | TLS, self-signed cert pinned at pairing | 4-digit PIN | Third-party IDD driver | Gamepad, audio, HDR | GPL, free |
| RustDesk | All | Native | VP8/VP9/AV1/H.264/H.265 | Curve25519 + secretbox on **one** stream (video, input, files) | Password / ID | n/a (remote desktop) | Files, clipboard, audio | AGPL |
| KDE Connect | Linux (+ others) | Android / iOS | n/a | TLS self-signed, pinned at pairing | Pair prompt both ends | n/a | Clipboard, files, input, notifications | GPL |

What this says about Vior's gaps, in order of user impact:

1. **Encryption is table stakes for anything that carries the screen.** Every project above with a video path encrypts it except Spacedesk (criticised for it) and Weylus (optional code). Deskreen proves the browser-on-insecure-origin case is solvable with a pure-JS library — exactly what `clients/secure/` is. Ship §8.
2. **Hardware H.264/HEVC is universal; MJPEG is 5–10× the bandwidth and burns phone CPU/battery.** This is the first thing a user coming from Spacedesk/Duet/Sidecar feels. VideoToolbox (macOS) → Media Foundation/NVENC (Windows) → VA-API (Linux), decoded by the WebView's native `<video>`/WebCodecs. Sunshine's pipeline is the reference for latency.
3. **Pairing UX is on par** (Sunshine PIN, Deskreen QR). Keep the 6-digit code + QR; add the lock indicator.
4. **Clipboard sync, audio, multi-client** are the next features users expect (KDE Connect, Spacedesk, Duet). Multi-client is also the strongest differentiator Vior could add over Duet/Sidecar on macOS.
5. **Distribution**: every commercial competitor is signed, notarized, auto-updating and in a store. Unsigned GitHub artifacts and a sideloaded debug APK cap adoption regardless of features.
6. **Linux/Windows parity**: Spacedesk/Sunshine ship a virtual-display driver; Vior's Windows path must at minimum refuse to touch real adapters until an IDD driver exists.

---

## 10. Suggested release gates

**v0.3 — Stabilise (≈2–3 weeks)**
Mobile P0 1–3, desktop P0 1–3 and P1 6–11, Windows virtual-display guard (§6), config persistence + log file (§5-5/8), CLI P0 1–2 (fix or remove), fix the settings toggles that lie (mobile P1 7). Convert "verify on device" items into a manual test run on at least one Android 13+ and one Android 15 device.

**v0.5 — Secure by default**
§8 in full. README "What is not protected" shrinks to USB-skips-pairing and no-per-device-credential.

**v1.0 — Product**
H.264 pipeline, clipboard sync, audio, multi-client; signing + notarization + auto-update; Play Store; release automation for desktop artifacts; macOS ScreenCaptureKit capture.

---

## 11. Documents this supersedes or corrects

- `master-plan.md` "Known issues / gaps": pair-code *is* enforced; tests are *not* zero (23+ files); mobile is 13 modules, not "single file"; discovery is an HTTP sweep, not 20-host batches; settings is a sheet, not full-page; shortcut count and F-key range overstated.
- `dead-code-audit.md`: `DecodeFrameHeader`, `EncodeTouchEvent`, `EncodeHello` are referenced only by tests.
- `codebase-audit-2026-07-16.md` P1-1 (pre-auth slot claim) and P1-2 (per-connection rate limit): still open.
- `secure-channel-implementation.md` §7.2 "no client implementation yet": still true.
- README (fixed in this pass): pair-code length, regeneration-on-restart, "no rate limit", trust-by-deviceId, UDP discovery, Go version.
