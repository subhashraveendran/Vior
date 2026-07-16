# Vior — Codebase Audit Report (2026-07-16)

Vior is a LAN screen-mirroring / remote-input tool: a Go desktop host (CLI + Wails v2 desktop app) streams MJPEG frames and accepts touch/keyboard input and file transfers from an Android Capacitor app or any browser, over HTTP/WebSocket, USB AOA, or ADB reverse-forwarding. Repo is clean on branch `fix/capture-web-remote-cli`, one commit (`c541091`) ahead of `main`; that unmerged commit contains a critical security fix (see P0). All Go tests pass (`go test ./internal/...`: 8 packages ok, 6 with no test files, 23 test files total).

## 1. Architecture Overview

```
                         ┌──────────────────────────────────────────────┐
                         │           DESKTOP HOST (Go)                  │
                         │                                              │
  ┌───────────────┐      │  cmd/vior (CLI, cobra)   desktop/ (Wails v2) │
  │  npm/cli      │──────│        │                      │              │
  │  vior.js shim │ runs │        └────────┬─────────────┘              │
  │ (downloads Go │      │                 ▼                            │
  │  binary from  │      │        internal/* shared core                │
  │  GH Releases) │      │  capture → session → stream (HTTP+WS server) │
  └───────────────┘      │  input ◄─ protocol ─► filetransfer/trust     │
                         │  virtual (CGVirtualDisplay)  discovery (UDP) │
                         │  usb (AOA/gousb)  adb (reverse forward)      │
                         └──────┬───────────┬───────────┬───────────────┘
                                │           │           │
              HTTP :8080        │  UDP :37680 beacon    │ USB (AOA bulk EP)
        /stream /info /ws       │  broadcast            │ or ADB reverse :8080
        /snapshot /download/    │           │           │
                                ▼           ▼           ▼
        ┌──────────────────────────────────────────────────────────┐
        │  CLIENTS                                                 │
        │  1. mobile-cap/ Capacitor Android app (WebView TS app    │
        │     + UsbAccessoryPlugin.java native AOA plugin)         │
        │  2. Any browser → embedded web client                    │
        │     (internal/stream/webclient/ served at "/")           │
        └──────────────────────────────────────────────────────────┘
```

### Tech stack

- **Go core** (`go.mod`): Go 1.25.6, module `github.com/subhashraveendran/vior`. Deps: `gorilla/websocket v1.5.3`, `google/gousb v1.1.3`, `kbinani/screenshot`, `skip2/go-qrcode`, `spf13/cobra v1.10.2`, `wailsapp/wails/v2 v2.12.0`, `golang.org/x/image v0.40.0`. CGO/Obj-C for macOS tray (`desktop/tray_darwin.m`) and virtual display (`internal/virtual/display_darwin.m`).
- **Desktop frontend** (`desktop/frontend/package.json`): React ^19.2.6, Vite ^8.0.13, TypeScript ^5.6.0; Wails bindings in `desktop/frontend/wailsjs/go/main/App.js`; config `desktop/wails.json`.
- **Mobile** (`mobile-cap/package.json`, `mobile-cap/capacitor.config.json`): Capacitor ^7.0.0, app id `com.vior.mobile`, vanilla-TS WebView app (`webDir: src`, TS emitted via `tsconfig.emit.json`), Android-only native Java in `mobile-cap/android/app/src/main/java/com/vior/mobile/`, bundled jsQR (`mobile-cap/src/jsqr.min.js`).
- **npm CLI** (`npm/cli/package.json`): package `vior` v0.2.0, Node >=18; `npm/cli/vior.js` downloads the pinned `v0.2.0` platform binary from GitHub Releases into `~/.vior/bin/` and `execSync`-forwards args.

### Entry points

| Path | Starts |
|---|---|
| `cmd/vior/main.go` → `cmd/vior/cli/root.go` | CLI: `start` (full server), `stop` (PID file in os.TempDir), `displays`, `display mirror/extend`, `usb setup/teardown/status` (ADB reverse), `virtual create/destroy/setup`, `version` |
| `desktop/main.go` | Wails v2 app; embeds `frontend/dist`; SIGINT/SIGTERM + panic teardown; bound Go API in `desktop/app.go`; macOS tray `desktop/tray_darwin.go/.m/.h` |
| `desktop/frontend/src/main.tsx` | React UI (screens in `src/screens/`, Files pane in `src/panes/Files.tsx`) |
| `mobile-cap/src/index.html` + `src/js/main.ts` | Capacitor WebView boot; screens in `src/js/screens/`, core in `src/js/core/` |
| `mobile-cap/android/.../MainActivity.java` | Android activity + `UsbAccessoryPlugin.java` (AOA), `BootReceiver.java` |
| `npm/cli/vior.js` | npm shim; `npm/cli/install.js` pre-downloads at postinstall |

### internal/ package map

| Package | Purpose | Key files |
|---|---|---|
| `internal/capture` | Screen capture + display enumeration, JPEG frames | `capture.go`, `capture_darwin.go` |
| `internal/config` | Config struct + constants (note: `Version = "v0.1.0-dev"` at `config.go:11`) | `config.go` |
| `internal/input` | Mouse/keyboard injection per OS; `TouchMapper` phone-touch → host input | `input.go`, `input_darwin.go`, `input_windows.go`, `input_linux.go`, `touch.go` |
| `internal/stream` | HTTP/WS server (`MJPEGServer`): `/stream`, `/snapshot`, `/info`, `/ws`, `/download/`, embedded web client at `/`; pair-code derivation, rate limiting, origin/host guards | `stream.go` (routes :453-466), `webclient.go`, `webclient/client.js` |
| `internal/network` | Terminal QR rendering of connect URL (half-block glyphs via `QRCodePlain`) | `network.go` |
| `internal/discovery` | UDP broadcast beacon (`"VIOR"` magic, protocol v1, port 37680, every 2 s) | `discovery.go` |
| `internal/protocol` | WS message schema (hello/input/resize/file-*/download-*), `Session` keepalive, 128 KB max message | `protocol.go`, `session.go`, `handler.go` |
| `internal/usb` | AOA: desktop = USB host via gousb; binary framing, 8 MiB max frame, heartbeat | `protocol.go`, `accessory.go` |
| `internal/adb` | Wraps `adb` for reverse forwarding; auto-downloads platform-tools to `~/.vior/platform-tools/` | `adb.go`, `download.go` |
| `internal/virtual` | Virtual display: macOS CGVirtualDisplay private API; Linux via X11 xf86-video-dummy (`vior virtual setup`); Windows configures a pre-installed IDD/dummy-plug display; ErrUnsupported only on other platforms / when drivers are absent | `display.go`, `display_darwin.go/.m/.h`, `display_linux.go`, `display_windows.go`, `display_other.go` |
| `internal/filetransfer` | Chunked base64 uploads over WS + registered HTTP downloads; SHA-256 verify, previews | `filetransfer.go` |
| `internal/trust` | Persisted trusted-device store (`~/.vior/`) | `trust.go` |
| `internal/machineid` | Stable host ID (IOPlatformUUID / /etc/machine-id / MachineGuid); feeds pair-code derivation | `machineid.go` |
| `internal/session` | Shared connect logic (mirror vs extend, virtual-display lifecycle, dimension clamping) | `setup.go` |

### Runtime protocols

| Protocol | Implementation |
|---|---|
| HTTP :8080 — `/stream` MJPEG, `/snapshot`, `/info`, `/download/{id}`, `/` web client | `internal/stream/stream.go:453-466`, `internal/stream/webclient/client.js` |
| WebSocket `/ws` — hello + pair code/trust, input, resize, file transfer | `internal/stream/stream.go`, `internal/protocol/`; mobile: `mobile-cap/src/js/screens/connect.ts:178` |
| UDP discovery beacon (port 37680) | `internal/discovery/discovery.go` |
| HTTP subnet sweep fallback (mobile probes `/info` across the /24, ports 8080/8081) | `mobile-cap/src/js/screens/discovery.ts` |
| USB AOA binary frames (no ADB) | `internal/usb/`, `mobile-cap/android/.../UsbAccessoryPlugin.java`, `mobile-cap/src/js/core/usb.ts` |
| ADB reverse port-forward | `internal/adb/adb.go`, `cmd/vior/cli/usb.go` |
| QR pairing | `internal/network/network.go`, `mobile-cap/src/js/screens/qr.ts` |
| Pair code (6-digit decimal, derived from machine ID) + trust store | `internal/stream/stream.go`, `internal/trust/trust.go`, `internal/machineid/machineid.go` |

### Build & CI

- **Makefile**: `build` (→ `./tmp/vior`), `run`/`start`, `dev` (Air, `.air.toml`), `test`, `lint` (`go vet`), `desktop` (`wails build`), `macos-install` (copies .app to /Applications and strips quarantine — a workaround for missing signing).
- **CI** (`.github/workflows/build.yml`): 3 jobs — `cli` (Go 1.25, 3 OSes, `vior-cli-*` artifacts), `desktop` (Wails v2.12.0 + Node 20, `wails build -clean`, 3 OSes), `mobile` (Java 21 + Android SDK; regenerates `android/` via `npx cap add android`, overlays custom Java/manifest/resources, `assembleDebug` APK). `pages.yml` deploys `docs/` to GitHub Pages.
- **Tests**: 23 Go test files, all under `internal/` (stream has 9). Zero tests in `internal/adb`, `internal/config`, `internal/discovery`, `internal/machineid`, `internal/network`, `internal/virtual`, all of `cmd/` and `desktop/`, and zero JS/TS tests in either frontend.

## 2. Security & Best-Practice Findings

### 2.1 Transport is entirely cleartext — the dominant open risk (#51)

There is no TLS anywhere: zero `crypto/tls` / `ListenAndServeTLS` usage in the Go tree; `internal/stream/stream.go` `Start()` serves plain HTTP (lines 489-509). The mobile client hardcodes `ws://` (`mobile-cap/src/js/screens/connect.ts:178`) and `http://` frame/probe URLs (`connect.ts:96`, `~292`; `discovery.ts`); the embedded web client (`internal/stream/webclient/client.js:1235`) can never negotiate `wss:` because the server has no TLS listener. Screen frames, injected input, file chunks, and the pair code itself all cross the LAN unencrypted. Four Android layers exist solely to permit this: `cleartext: true` and `allowMixedContent: true` in `mobile-cap/capacitor.config.json`, `android:usesCleartextTraffic="true"` at `mobile-cap/android/app/src/main/AndroidManifest.xml:20`, and a cleartext `base-config` at `mobile-cap/android/app/src/main/res/xml/network_security_config.xml:3`.

Recommended design (verified applicable to this codebase):

- Migrate all channels to `wss://` / TLS 1.2+ — "Always use WSS in production."
  https://websocket.org/guides/security/
  https://github.com/Checkmarx/Go-SCP/blob/master/src/communication-security/websockets.md
- Generate a per-device self-signed cert on first run (mkcert is explicitly not for end-user machines, and its README notes real-CA certificates are "dangerous or impossible" for hosts like localhost or 127.0.0.1 — public CAs do not issue for private LAN IPs). Natural home: a new `internal/tlscert` package persisting under `~/.vior/` alongside `internal/machineid` and `internal/trust`.
  https://github.com/FiloSottile/mkcert
- Distribute the cert's SPKI/SHA-256 fingerprint during pairing (QR connect URL, `/info`, hello) and pin it on clients with a backup pin. Caveat: the WebView's JS `WebSocket` cannot do custom pin validation, and Android NSC `<pin-set>`/`<trust-anchors>` match *hostnames* while Vior connects to raw LAN IP literals — so pinning requires a hostname strategy (e.g. mDNS name in the QR/discovery payload) as part of the WSS design.
  https://cheatsheetseries.owasp.org/cheatsheets/Pinning_Cheat_Sheet.html
  https://developer.android.com/privacy-and-security/security-config
- After WSS lands, remove all four cleartext layers — both Capacitor flags are documented dev conveniences ("not intended for use in production"; cleartext is off by default since API 28). Note the CI mobile job regenerates `android/` and overlays these files, so change the overlay sources.
  https://capacitorjs.com/docs/config

### 2.2 WS server hygiene — verified ALREADY DONE (confidence items)

The codebase is further along on WebSocket hygiene than generic guidance assumes:

- **Strict Origin checking**: `Upgrader.CheckOrigin = checkWSOrigin` (`internal/stream/stream.go:434-435`), restricting Origin hosts to loopback/link-local/RFC1918 (`:1057-1067`, `isPrivateHost` `:1087-1099`), plus a DNS-rebinding Host-header guard and Origin-allowlisted CORS (`corsHandler`, `:1112-1141`). Tested (`ws_origin_test.go`, `host_guard_test.go`).
  https://pkg.go.dev/github.com/gorilla/websocket
  https://github.com/Checkmarx/Go-SCP/blob/master/src/communication-security/websockets.md
- **Read limits**: `SetReadLimit(128*1024)` at `internal/protocol/session.go:132` (limit constant at `:26`), within the ~1 MB ceiling guidance; USB transport has its own 8 MiB cap (`internal/usb/protocol.go:28`). One residual: the hello read in `WaitForHello` (`session.go:231-250`) precedes the limit — move `SetReadLimit` into `NewSession` (`session.go:95`).
  https://websocket.org/guides/security/
- **Pair-code brute-force limiting**: 5 failed attempts/min/IP + global 60/min ceiling (`internal/stream/stream.go:226-320`), enforced at both the `/info?probe=` oracle (`:781`, with `subtle.ConstantTimeCompare` at `:778`) and the WS mismatch path (`:948`). Pair code required on every connect; deviceID alone no longer admits (`:920-967`).
  https://websocket.org/guides/security/
- **Message schema validation**: typed decode with unknown types rejected (`internal/protocol/session.go:269-336`); touch coords NaN/Inf-rejected and clamped, scroll deltas capped (`internal/input/touch.go:49-61,90`); filenames sanitized and receive paths containment-checked, symlinks rejected via `EvalSymlinks` + `IsRegular` (`internal/filetransfer/filetransfer.go:554-577,779-796`); download IDs reject path separators (`stream.go:752-757`) and the mobile side regex-validates file ids (`mobile-cap/src/js/screens/files.ts:102`).
  https://github.com/Checkmarx/Go-SCP/blob/master/src/communication-security/websockets.md
- **Embedded assets only**: `//go:embed all:frontend/dist` with `Assets`-only AssetServer (`desktop/main.go:21,67-69`); LAN web client likewise embedded (`internal/stream/webclient.go:10`); no `AssetsHandler`.
  https://raw.githubusercontent.com/wailsapp/wails/master/website/docs/guides/application-development.mdx
- **Wails v2 (stable), not v3 alpha**: `go.mod` and CI both pin v2.12.0 (`.github/workflows/build.yml:122`). Optional in-major bump to v2.13.0.
  https://github.com/wailsapp/wails
- **No `-devtools` builds anywhere**; Capacitor `androidScheme: "https"`, default `localhost` hostname, and `webContentsDebuggingEnabled` unset (off in release) are all correct by default.
  https://raw.githubusercontent.com/wailsapp/wails/master/website/docs/reference/cli.mdx
  https://capacitorjs.com/docs/config

### 2.3 Open WS/server gaps

- **Unauthenticated upgrade + pre-auth slot claim**: the WS upgrade (`internal/stream/stream.go:858-863`) carries no credential; the pair code is only checked inside the post-upgrade hello (`:932-967`), and the single client slot is claimed *before* authentication (`wsConn` set at `:869-880`). An unauthenticated LAN peer can occupy the only session slot for up to the 10 s hello timeout (`internal/protocol/session.go:22`), repeatedly — a trivial pairing DoS. Mint a short-lived token from the rate-limited `/info?probe=` confirmation and validate it before upgrade (or at minimum before claiming the slot). Note: browser/WebView JS cannot set custom WS headers, so the token must ride a query param, cookie, or `Sec-WebSocket-Protocol` value.
  https://websocket.org/guides/security/
  https://github.com/Checkmarx/Go-SCP/blob/master/src/communication-security/websockets.md
- **No per-connection message rate limiting**: `ReadLoop` (`internal/protocol/session.go:151-206`) dispatches unlimited msg/sec; a paired or hijacked session can flood input-injection and file-chunk handlers. Add a token bucket in `ReadLoop` or the `wsMessageAdapter` dispatch in `internal/stream/stream.go`.
  https://websocket.org/guides/security/
- **Timing-inconsistent pair-code compare**: WS hello uses plain `==` (`internal/stream/stream.go:934`) while `/info` probe correctly uses `subtle.ConstantTimeCompare` (`:778`) for the same secret. Marginal over a network, trivially fixed.
- **Residual input validation**: no whitelist of final key names in `TypeKey` handling (`internal/input/input_darwin.go:236+` and OS counterparts); hello string fields (name/deviceID/platform) have no length caps before being stored in the trust file and logs (`internal/protocol/protocol.go`, admission point in `stream.go`).
  https://github.com/Checkmarx/Go-SCP/blob/master/src/communication-security/websockets.md
- **`/info` pair-code leak — fixed but unmerged**: on `main`, `/info` still publishes the raw pair code to any LAN client, nullifying the entire pair-code admission scheme. Branch commit `c541091` replaces it with a rate-limited, constant-time `?probe=` confirm endpoint (`internal/stream/stream.go:770-786`, `info_test.go`) and adds host-guarded frame endpoints (`host_guard_test.go`), a distributor shutdown double-close fix (`distributor_test.go`), and Windows Unicode keyboard (`internal/input/input_windows.go:220-224`). This commit is not on `main` and is undocumented in `tasklist.md`.

### 2.4 Capacitor / Android app

- **`"allowNavigation": ["*"]`** in `mobile-cap/capacitor.config.json` lets *any* external URL load inside the privileged WebView — worse than the generic case the docs warn about ("not intended for use in production"). Remove or tightly scope; independent of the WSS work.
  https://capacitorjs.com/docs/config
- **No Content-Security-Policy** in either client: `mobile-cap/src/index.html` has no CSP meta tag and loads remote Google Fonts (lines 12-14, which any CSP must account for — or bundle them); `internal/stream/webclient/index.html` also has none. Until WSS lands the policy can only be scheme-scoped (`connect-src ws: http:` → `wss: https:` after).
  https://capacitorjs.com/docs/guides/security
- **Pair code persisted in plain localStorage**: mobile resume record (`ResumeRecord.pair`, `mobile-cap/src/js/core/ws-keepalive.ts` `saveResume` ~204-210; reset list acknowledges `vior_pair` at `mobile-cap/src/js/screens/settings.ts:201`) and the embedded web client (`PAIR_KEY='vior_pair'`, `internal/stream/webclient/client.js:20`). Move to Android Keystore (small native plugin beside `UsbAccessoryPlugin.java`) or memory-only; the plain-browser web client's best option is session-scoped storage.
  https://capacitorjs.com/docs/guides/security

### 2.5 Build & release hardening

- **No code signing or notarization anywhere**: no signtool/codesign/notarytool steps in `.github/workflows/build.yml`; no entitlements.plist in `desktop/build/`; the npm shim distributes unsigned binaries; and `Makefile:72-80` (`macos-install`) strips macOS quarantine (`xattr -dr com.apple.quarantine`) — the tell-tale workaround for unsigned builds. Windows: standard (non-EV) cert with `signtool sign /fd sha256 /tr <ts-server>`, cert in CI secrets, applied to both desktop and CLI artifacts. macOS: Developer ID .p12 in secrets, codesign with entitlements (Vior needs Screen Recording + Accessibility declared — cf. `internal/capture`, `internal/input/input_darwin.go`, `desktop/app.go` `CheckPermissions` ~:1077), notarize, staple; then retire the quarantine strip.
  https://raw.githubusercontent.com/wailsapp/wails/master/website/docs/guides/signing.mdx
- **Build flags**: CI uses `wails build -clean` (`.github/workflows/build.yml:168`) and `Makefile:61` is bare `wails build` — add `-trimpath` to both (strips filesystem paths from the binary) and an explicit `-webview2` strategy (e.g. `download`) on the Windows leg. `-devtools` already correctly absent.
  https://raw.githubusercontent.com/wailsapp/wails/master/website/docs/reference/cli.mdx
- **Bind-exposed methods are a security boundary**: `desktop/main.go` binds the entire `App` struct (53 public methods in `desktop/app.go`; the generated wailsjs `App.d.ts` lists only 49, i.e. bindings are stale). `SendFileToPhone(path string)` (`app.go:910`) and `SendFile(path string)` (`app.go:937`) accept raw absolute paths from frontend JS; `OfferDownload` (`internal/filetransfer/filetransfer.go:547-577`) checks regular-file/size/symlink but not *which* files may be offered — a compromised/XSS'd webview could exfiltrate any readable file (e.g. `~/.ssh` keys) to the connected phone. Gate behind the dialog-driven `PickAndSendFile` flow or a user-picked-path allowlist; add bounds validation to `StartStream`/`UpdateConfig`/`CreateVirtualDisplay` inputs. (Good counter-examples already exist: `SetPairCode` is regex-validated in `internal/stream/stream.go:137-153`.)
  https://raw.githubusercontent.com/wailsapp/wails/master/website/docs/guides/application-development.mdx

### 2.6 Repo-specific hygiene findings (verified against source)

- All 20 fix commits in `tasklist.md` exist in history, and 7 spot-checked fixes are confirmed live in code (pair-code admission `stream.go:920-967`; 8 MiB frame bound `MainActivity.java:129-141` + `usb/protocol.go:28`; file-id regex `files.ts:102`; symlink resolution `filetransfer.go:554-565`; touch clamp `touch.go:49-55`; WS origin `stream.go:435,1052-1120`; float32 wire encoding `usb/protocol.go:117-118` ↔ `MainActivity.java:349-350`).
- Stale comments misstate the security posture: `internal/stream/stream.go:44` says "4-digit numeric" (actual: `pairCodeDigits = 6`, `:60`); `:947` says "16M-combo hex" (actual: 6-digit decimal, 1M combos). `docs/master-plan.md:11,73,79` are wrong on pair-code format, enforcement ("not yet enforced" — false since `d1915f0`), and test count ("Zero" — false, 23 files); lines 126 and 143 also repeat the stale 6-char-hex format and the /info-exposes-pairCode behaviour.
- `docs/dead-code-audit.md:36` falsely claims `DecodeFrameHeader`/`EncodeTouchEvent`/`EncodeHello` are "used inside internal/usb/accessory.go" — `accessory.go:415,434,442` use `DecodeHello`/`DecodeTouchEvent`/`EncodeHelloAck`; the three symbols (`internal/usb/protocol.go:92,113,136`) are phone-side codec mirrors referenced only by tests.
- `docs/phase-2-sweep-report.md:118` cites nonexistent commit `ac11286`; the actual site-revamp merge is `8f0002b`.
- Editor junk `App.d.ts.tmp` / `App.js.tmp` is *tracked in git* (`desktop/frontend/wailsjs/go/main/`, committed in `c541091`, 303 lines) and will churn on every bindings regen.
- Version skew: `internal/config/config.go:11` says `Version = "v0.1.0-dev"` while the npm shim pins release `v0.2.0` (`npm/cli/vior.js:12`).
- On-disk (gitignored, untracked) binaries: `./vior` (12.8 MB), `./filetransfer.test` (7.0 MB), `tmp/vior` (10.6 MB) — `tmp/` was declared deleted in `docs/master-plan.md:59`.
- USB AOA has had 10+ security fixes but has never been exercised on real hardware (`docs/phase-2.md:131`, `docs/phase-2-sweep-report.md:55,138`).

## 3. Prioritized Action Items

### P0 — security/correctness now

| # | What | Why | Where |
|---|---|---|---|
| 1 | Merge `c541091` (branch `fix/capture-web-remote-cli`) to `main` | Until merged, `main`'s `/info` publishes the raw pair code to any LAN client, nullifying all pair-code admission hardening; branch is 1 commit ahead, tests green | `internal/stream/stream.go:770-786`, `internal/stream/info_test.go`, `host_guard_test.go`, `distributor_test.go`, `internal/input/input_windows.go:220-224` |
| 2 | Implement WSS/TLS (#51): per-device self-signed cert generated on first run, persisted under `~/.vior/`; serve via `http.Server.ServeTLS` (min TLS 1.2); switch all client URLs; distribute SPKI fingerprint via pairing (needs a hostname/mDNS strategy for Android NSC pinning) | Admission secret, screen frames, input, and files traverse the LAN in cleartext; the four Android cleartext flags exist only to permit this | Server: `internal/stream/stream.go` (Start, :489-509), new `internal/tlscert/`; clients: `mobile-cap/src/js/screens/connect.ts:178`, `mobile-cap/src/js/screens/discovery.ts`, `internal/stream/webclient/client.js`; URL builders: `cmd/vior/cli/start.go`, `desktop/app.go`, `internal/network/network.go`. (USB AOA and ADB-localhost transports unaffected.) Sequenced follow-through: remove `cleartext`/`allowMixedContent` from `mobile-cap/capacitor.config.json`, `usesCleartextTraffic` from `mobile-cap/android/app/src/main/AndroidManifest.xml:20`, cleartext base-config from `mobile-cap/android/app/src/main/res/xml/network_security_config.xml:3`, then add `<trust-anchors>`/`<pin-set>` there — editing the CI overlay sources (`.github/workflows/build.yml` mobile job regenerates `android/`) |
| 3 | Use `subtle.ConstantTimeCompare` in the WS hello pair-code check | Same secret compared constant-time at `/info` probe but with plain `==` at hello — inconsistent, timing side channel | `internal/stream/stream.go:934` (mirror `:778`) |

### P1 — soon

| # | What | Why | Where |
|---|---|---|---|
| 1 | Authenticate the WS upgrade (short-lived token minted by the rate-limited `/info?probe=` confirm) before `Upgrade`, or at minimum before claiming the single-client slot; token via query param/cookie/`Sec-WebSocket-Protocol` (WebView JS cannot set headers) | Unauthenticated LAN peer can occupy the only WS slot for the 10 s hello timeout, repeatedly — pairing DoS | `internal/stream/stream.go:858-880`, `internal/protocol/session.go:22`; senders: `mobile-cap/src/js/screens/connect.ts`, `internal/stream/webclient/client.js` |
| 2 | Add per-connection message rate limiting (token bucket, ~10 msg/sec on control channel) | Paired/hijacked client can flood input-injection and file-chunk handlers without limit | `internal/protocol/session.go:151-206` (ReadLoop) or `wsMessageAdapter` in `internal/stream/stream.go` |
| 3 | Gate Bind-exposed raw-path methods behind the dialog flow / user-picked allowlist; validate `StartStream`/`UpdateConfig`/`CreateVirtualDisplay` inputs | XSS'd webview could exfiltrate any readable file to the phone via `SendFileToPhone`/`SendFile` | `desktop/app.go:910,937`; `desktop/main.go` (Bind), `internal/filetransfer/filetransfer.go:547-577` |
| 4 | Remove `"allowNavigation": ["*"]` | Wildcard lets any external URL load inside the privileged WebView; independent of WSS | `mobile-cap/capacitor.config.json` |
| 5 | Add CSP meta tags to both clients (bundle or allowlist the Google Fonts) | No content restriction in either WebView/browser client today | `mobile-cap/src/index.html` (fonts at lines 12-14), `internal/stream/webclient/index.html` |
| 6 | Move persisted pair code out of plain localStorage (Keystore-backed native store or memory-only on Android; session-scoped on web) | Admission secret at rest in plaintext in two clients | `mobile-cap/src/js/core/ws-keepalive.ts` (~204-210), `mobile-cap/src/js/screens/connect.ts`, `mobile-cap/src/js/screens/settings.ts:201`, `internal/stream/webclient/client.js:20` |
| 7 | Sign + notarize release artifacts in CI; add `-trimpath` and explicit `-webview2` to builds; retire the quarantine strip | Unsigned binaries on all 3 OSes; Makefile bypasses Gatekeeper locally; paths embedded in shipped binaries | `.github/workflows/build.yml` (:168 and macOS/Windows desktop legs, cli job artifacts), `Makefile:61,72-80`, new `desktop/build/darwin/entitlements.plist` (Screen Recording + Accessibility) |
| 8 | Apply `SetReadLimit` in `NewSession` so the pre-ReadLoop hello read is bounded | First WS frame currently unbounded (one-line fix) | `internal/protocol/session.go:95` (limit currently set at `:132`) |
| 9 | Add tests for `internal/virtual` and `internal/machineid` | Most complex untested package (5 Go files) and the pair-code entropy source have zero coverage | `internal/virtual/`, `internal/machineid/machineid.go` |
| 10 | Fix stale security-posture comments/docs | Wrong docs made the `/info` leak look intentional | `internal/stream/stream.go:44,947`; `docs/master-plan.md:11,73,79,126,143` |
| 11 | Remove committed `.tmp` bindings junk | Tracked editor/build artifacts (303 lines) churn on every wails bindings regen | `desktop/frontend/wailsjs/go/main/App.d.ts.tmp`, `App.js.tmp` |
| 12 | Exercise USB AOA end-to-end on a physical device | The AOA path has 10+ security fixes (`c0cac1c`, `3ec33ac`, `cedcd75`, …) but zero real-hardware validation on record | `internal/usb/`, `mobile-cap/android/.../UsbAccessoryPlugin.java`; deferral recorded at `docs/phase-2.md:131`, `docs/phase-2-sweep-report.md:55,138` |
| 13 | H.264 video + audio forwarding pipeline | Both rated "High" since Phase 2; still MJPEG, no audio path | `internal/stream/stream.go` (MJPEGServer); plan: `docs/master-plan.md:75-76` |

### P2 — nice-to-have

| # | What | Why | Where |
|---|---|---|---|
| 1 | Key-name whitelist for `TypeKey` + length caps on hello fields (name/deviceID/platform) | Residual input-validation gaps on WS-sourced data | `internal/input/input_darwin.go:236+` (and `_windows`/`_linux`), `internal/protocol/protocol.go` |
| 2 | Delete or explicitly justify dead USB codec symbols; correct the dead-code audit's false claim | `DecodeFrameHeader`/`EncodeTouchEvent`/`EncodeHello` have zero non-test references; doc says otherwise | `internal/usb/protocol.go:92,113,136`; `docs/dead-code-audit.md:36` |
| 3 | Fix version skew: bump `Version` constant to match released v0.2.0 (or wire it to release tags) | `/info` and CLI report `v0.1.0-dev` while npm distributes v0.2.0 | `internal/config/config.go:11`, `npm/cli/vior.js:12` |
| 4 | Correct sweep-report commit hash `ac11286` → `8f0002b` | Nonexistent hash in the audit record | `docs/phase-2-sweep-report.md:118` |
| 5 | Add frontend/mobile test infra (test scripts + first tests) | Zero JS/TS tests anywhere; only Capacitor template stubs | `desktop/frontend/package.json`, `mobile-cap/package.json` |
| 6 | Clean local binary artifacts (~30 MB, gitignored but present; `tmp/` was declared deleted) | Repo hygiene | `./vior`, `./filetransfer.test`, `tmp/vior` |
| 7 | Optional: bump Wails v2.12.0 → v2.13.0 (stay on v2; do not migrate to v3 alpha) | In-major maintenance | `go.mod`, `.github/workflows/build.yml:122` |
| 8 | Remaining roadmap: clipboard sync, multi-client WS, Linux Unicode keyboard parity (Windows done in `c541091`), stylus pressure, iOS client, Noise E2E, auto-update/crash reporter | Planned in Phase 2/3, all zero-hit greps today | `internal/protocol/` (no `MsgClipboard`), `internal/stream/stream.go:868-877` (single client) & `:342` (maxClients=16), `internal/input/input_linux.go:90`, `mobile-cap/` (android-only); plans: `docs/master-plan.md:70-76,109` |
