# Vior — connection, file transfer and pairing architecture

October 2026. The result of a focused review of how the three Vior transports
(Wi-Fi WebSocket + HTTP MJPEG, USB Android Open Accessory, the embedded
browser client) connect, stay connected, move files and authenticate — and
of how LocalSend, KDE Connect, PairDrop/Snapdrop, Syncthing, Magic Wormhole,
croc, Sunshine/Moonlight, RustDesk, Deskflow and scrcpy solve the same
problems. Part 1 is what is true in the code today (verified file-by-file).
Part 2 is the target design, concrete enough to implement in order.

Companion documents: `production-readiness-2026-10.md` (the overall backlog),
`transport-security-plan.md` and `secure-channel-implementation.md` (the
record layer this design assumes).

---

## Part 1 — Today

### 1.1 Three transports that drifted apart

| | Wi-Fi (mobile app) | USB AOA | Browser client |
|---|---|---|---|
| Framing | JSON `{type,data}` text frames (`internal/protocol/protocol.go`) | `[type][fixed payload]`; video `[0x01][len:4][jpeg]` (`internal/usb/protocol.go`) | Same JSON as mobile |
| hello / ready | Full hello: pair code, intent, deviceId | Width/height/DPR only — no pair code, no intent | Full hello |
| Video | HTTP `/snapshot` polling (`stream.ts`) | 0x01 frames — **dropped on the phone when a JPEG exceeds one `read()`** (no reassembly, `MainActivity.onData`) | `<img src=/stream>` MJPEG |
| Touch | WS | 0x02 exists on both ends, **no JS caller** — touch goes nowhere over USB | WS |
| Mouse / key / scroll / resize | WS | none (blocked in UI) | WS |
| Files | WS chunks up, HTTP `/download` down | none | WS chunks both ways |
| Keepalive | App ping 15 s / pong deadline 20 s; server spec ping 30 s / read deadline 40 s | Desktop ping 5 s / dead 10 s; phone has no liveness check | None app-level |
| Reconnect | 5 attempts, exp. backoff, ±25 % jitter | Desktop rescans every 2 s; phone never rescans (no `onNewIntent`) | Effectively one retry |
| Survives a drop | Nothing: new session id, display destroyed, transfers wiped | Nothing | Nothing |
| Single-client rule | One WS slot | No slot, no exclusion against WS | One WS slot |

The secure channel (X25519 → HKDF → XSalsa20-Poly1305, with frames on the
sealed stream) is implemented on the server and **used by no client**. Every
real session today is cleartext.

### 1.2 What was broken, and what this pass fixed

Verified in code and fixed, with tests where the logic is testable in Go:

| Symptom | Cause | Fix |
|---|---|---|
| **No phone→desktop file ever survived.** | Phone sent `hash:""`; `HandleComplete` treated any mismatch as corruption and deleted the file (`internal/filetransfer`). | Empty hash = "unverified", accepted and logged; the phone and browser now compute SHA-256 with WebCrypto where available. Tests: `download_test.go`. |
| **Desktop→phone failed for any file outside `~/Downloads/Vior`.** | `ServeDownload` required the path to be inside the receive dir. | Authorization is now "the exact file resolved at offer time" (a post-offer symlink swap still 403s). Tests cover both. |
| **Any photo over ~96 KB killed the WebSocket.** | The offer carried the whole image as a data URL; server pre-auth read limit is 128 KiB. | Canvas thumbnail ≤160 px / ≤32 KB, dropped if larger. |
| **"Save" on Android did nothing.** | `<a download>` on a `blob:` URL; the WebView has no `DownloadListener` and Capacitor does not handle blob downloads. | `Android.downloadFile()` hands the HTTP URL to the system `DownloadManager` (streams to `Downloads/Vior`, no JS heap, system notification); completion reported back via `viorNativeDownloadDone`. Browser keeps the fetch path. **Verify on a device.** |
| **Reconnect after a silent Wi-Fi drop → "occupied", gave up.** | Server kept the dead session up to 40 s; the slot was claimed before authentication. | Slot claimed after the pair check; a hello from the same `deviceId` evicts the stale session (its teardown runs first). Rejected peers no longer trigger `OnClientDisconnect`. Tests: `reconnect_test.go`. |
| App-level pings did not count as liveness. | Only spec pongs refreshed the read deadline. | `MsgPing` extends it too. |
| **Mirror ↔ extend switch did nothing.** | Mobile sent `resize{mode}`; `ResizeMessage` had no `mode`; the server re-ran setup as `extend`. | `ResizeMessage.Mode`; `session.ResizeHello` carries intent/mode across resizes (desktop + CLI). |
| Rotating a remote-only phone created a virtual display. | Resize hard-coded `extend` and dropped the intent. | Same fix; `Mode "none"` handled on resize. |
| **CLI streamed the whole desktop to remote-only clients.** | CLI ignored `Mode "none"`; its resize bypassed validation (`uint32(negative)`). | CLI shares one `applySetup` for connect and resize, via `session.Configure`. |
| Desktop showed a "Virtual display" grid for remote/files sessions. | No intent in the UI payload. | `ClientInfo.Intent`; the Connected screen shows a session card instead. Display-mode connect now also sends address/platform/deviceId. |
| "Starting stream…" forever on a session with no frames. | `/snapshot` polling retried every 150 ms with no bound. | Stops after ~6 s with a clear message. |
| Reconnect banner never cleared; pair-code search probed only port 8080; keepalive leaked an `offline` listener per session. | — | Fixed. |

Earlier in the same branch: the mobile connect loop (rejected code → five retries with an empty code → rate-limited), Cancel not cancelling, pair code not cached across launches.

### 1.3 Still open on the connection layer (ranked)

1. **USB video reassembly and USB input** (P0). The phone must accumulate `read()`s into frames (`[0x01][len][jpeg]`, reads can be ≤16 KiB); `stream.ts` must call `Android.sendTouch` when the transport is USB. ~80 lines of Java + 5 of TS; needs a cable to verify.
2. **USB ↔ Wi-Fi mutual destruction** (P0). `handleUSBConnect` stops the WS capture; the phone's USB hello-ack closes its Wi-Fi socket; the server's `OnClientDisconnect` then destroys the USB display just built. Needs one session owner (Part 2 §2.3).
3. `a.session`, `a.fileMgr`, `currentClientTrusted` shared across goroutines without locks (`desktop/app.go`).
4. USB `OnDisconnect` fires twice (heartbeat death → cleanup → read error → again); phone never rescans after detach.
5. The browser client is a separate copy of the protocol (own storage keys, no keepalive, one retry).
6. Nothing reads the UDP discovery beacon; mobile discovery fires 508 concurrent probes with no batching.
7. No session resumption: every drop destroys the virtual display and all transfers.

---

## Part 2 — Target design

### 2.1 Principles (what every reference project converged on)

- **Pairing is separate from the session.** Pair once → long-lived per-device identity. A reconnect authenticates with that identity and never re-sends the secret (KDE Connect, Moonlight, Syncthing, RustDesk, Windowcast).
- **One codec, many links.** KDE Connect's `LinkProvider`/`DeviceLink`, Syncthing's dialers with priorities, RustDesk's single encrypted stream carrying video + input + files. No reference project runs a second, differently-framed protocol per transport.
- **Discovery is a stack with fallbacks**, never multicast alone (LocalSend: multicast → HTTP sweep → manual; KDE Connect: UDP broadcast + NSD + unicast).
- **Liveness comes from heartbeats, never from `onclose`.** Android sockets stall silently on power-save and network switches.
- **Control over the message channel, bytes over HTTP** for files (LocalSend `prepare-upload` + `PUT`; KDE Connect's payload socket). Nobody who scaled file transfer kept base64-in-JSON.

### 2.2 One session, three links

```go
// internal/transport
type Link interface {
    Send(f Frame) error        // whole frame; blocking = backpressure
    Recv() (Frame, error)
    Close() error
    Kind() string              // "wifi" | "usb" | "browser"
    Priority() int             // usb > wifi > browser
    RemoteAddr() string        // "usb" for AOA
}

// Wire frame, identical on every link (12-byte header):
//   u8 type | u8 flags | u16 channel | u32 seq | u32 len | payload
```

- **WebSocket link:** one binary message per frame.
- **USB link:** the same bytes as a stream; the phone keeps an accumulator and emits a frame whenever `12+len` bytes are buffered; `len > 1 MiB` → drop the link. Keep the existing `0x03/0x05` magic + version exchange as the AOA identification step, then switch to framed mode.
- **Browser link:** the WebSocket link; the client page imports the same compiled TypeScript codec and keepalive modules as the app (served by `internal/stream/webclient`), deleting the separate `client.js`.
- **Message types** (one table): `HELLO, WELCOME, REJECT, PING, PONG, PAUSE, BYE, INPUT_TOUCH, INPUT_MOUSE, INPUT_KEY, INPUT_SCROLL, INPUT_TEXT, CLIPBOARD, VIDEO_CONFIG, VIDEO_FRAME, FILE_OFFER, FILE_ACCEPT, FILE_REJECT, FILE_PROGRESS, FILE_CANCEL, FILE_DONE, SETTINGS`. This is what gives USB keyboard, scroll and files.
- **Video rides the link** as `VIDEO_FRAME` (as secure sessions already do over the WebSocket). `/stream` and `/snapshot` remain loopback-only debug endpoints.
- **Send queue per link, by priority:** input > control > files > video; video keeps only the newest unsent frame (drop, never queue). Check `bufferedAmount` on WebSocket, queue depth on USB.
- **Server:** `protocol.Session` wraps a `Link` instead of `*websocket.Conn`. `usb.Accessory` produces a `Session` that goes through the same admission, `OnClientConnect` and read loop as the WebSocket path, deleting `handleUSBConnect`, `OnTouch` and the frame tee. The single slot becomes transport-agnostic: same `deviceId` on a better link replaces the current link **without ending the session** (Syncthing's rule); a different device gets `REJECT busy`.

### 2.3 Session state machine

Client: `Idle → Discovering → Connecting(link) → Handshaking → Active`
- `Active —(link error | heartbeat dead | app resume)→ Suspended`
- `Suspended → Connecting` with the resume token, full-jitter backoff `random(0, min(5 s, 250 ms·2ⁿ))`, no attempt cap while foregrounded; after 6 failures re-run discovery (the IP may have moved); retry immediately on a `ConnectivityManager` network-change callback or accessory attach.
- `Handshaking —REJECT(unpaired|auth)→ NeedsPairing` (stop retrying — shipped in `e94a189`); `—REJECT(version)→ Failed`.

Server, per session: `Active —(link lost)→ Detached(grace) —(HELLO with valid resume token)→ Active`; `Detached —(grace expires)→ TornDown`.
- While Detached: stop encoding, **release held keys and buttons immediately**, keep the virtual display.
- Grace: 60 s Wi-Fi/browser, 15 s USB after detach, 120 s if the client sent `PAUSE`.

Handshake (same on every link):
```
C→S HELLO   {proto, clientId, auth, resumeToken?, lastRxSeq?, caps[], transport}
S→C WELCOME {sessionId, resumeToken (rotated, single-use), resumed, epoch, hb:{interval:5000, deadAfter:15000}, maxFrame}
   | REJECT {code: unpaired|busy|version|auth}
```
Unknown/expired resume token → fresh session (`resumed:false`), never a pairing error. On resume the server replays control messages above `lastRxSeq`, sends a full frame, and re-sends the state snapshot (resolution, mode, transfers with committed offsets).

### 2.4 Keepalive numbers

- App-level `PING{t}`/`PONG{t}` on **every** link (AOA has no ping; browsers cannot send spec pings). Server pings every 5 s; **any** inbound frame is proof of life; dead after 15 s of silence (Deskflow's 3-missed rule). Client mirrors it.
- Keep gorilla spec pings on the Go WebSocket as a second signal (shipped: app pings now extend the read deadline too).
- On Capacitor `resume`: do not trust the socket — ping immediately, 2 s deadline, else `Suspended`. On `pause`: send `PAUSE`, let the grace period hold the session. Timers are throttled in background WebViews; reconnect loops must restart on `resume`.
- "Keep connected with the screen off" is a later feature and needs the socket in Java under a `connectedDevice` foreground service; `dataSync` is capped at 6 h/day since Android 15.

### 2.5 Discovery stack (run in parallel, first verified hit wins)

1. Cached last-known `{serverId, host, port, fp}` → `GET /info` with an 800 ms timeout; connect if `serverId` matches. Covers almost every launch.
2. mDNS/DNS-SD: server advertises `_vior._tcp` (TXT `id, v, name, fp, secure`); Android browses with `NsdManager` from a small Capacitor plugin (or the Capawesome Zeroconf plugin), holding a `MulticastLock`; stop on `pause`.
3. The existing UDP beacon, **received in Java** (`DatagramSocket` + `MulticastLock`) and forwarded as plugin events — cheap, works where mDNS is filtered. Add `deviceId` and `secure` to the beacon.
4. `/24` HTTP probe as the last automatic step: ≤32 in flight, 400 ms each, only after 1–3 find nothing within ~2 s.
5. QR (every interface address + id + fp + code) and manual IP. The only route through AP/client isolation; when nothing answers, say "this Wi-Fi probably blocks devices talking to each other (AP isolation) — use USB, a hotspot, or scan the QR".

### 2.6 File transfer v2

Control messages stay on the link; bytes move to HTTP on the same Go listener (LocalSend / KDE Connect pattern):

```
FILE_OFFER   {transferId, files:[{fileId, name, size, mtime, mime, thumb?}]}
FILE_ACCEPT  {transferId, files:[{fileId, token, offset}]}     // receiver may accept a subset; offset>0 = resume
FILE_REJECT  {transferId, fileId?, reason: declined|busy|too_large}
FILE_PROGRESS{transferId, fileId, committed}                   // bytes fsynced by the RECEIVER, every 4–8 MiB
FILE_CANCEL  {transferId, fileId?}                             // either direction
FILE_DONE    {transferId, fileId, sha256?}
```
- Phone/browser → desktop: `PUT /upload?transfer&file&token&offset=N`, body = `File.slice(offset)` (lazy, no RAM) or native `FileTransfer.uploadFile`; Go `io.Copy`s into `name.part`. `XMLHttpRequest.upload.onprogress` for upload progress.
- Desktop → phone/browser: `GET /download?transfer&file&token` with `Range` (shipped today as `/download/{id}`, now saved natively on Android). Make it resumable: drop the single-use flag in favour of a token TTL and allow `Range` retries.
- If bytes must stay on the link (USB), use binary frames `FILE_CHUNK{fileId, offset, bytes}` of 256 KiB–1 MiB, under the link's `maxFrame`, with the same `FILE_PROGRESS` ack window (2 windows in flight).
- Resume: receiver keeps `name.part` + sidecar `{transferId, size, mtime, committed, blockHashes[]}`; on reconnect the sender asks `FILE_ACCEPT` again and gets the offset. Expire `.part` after 24 h.
- Integrity: per-block SHA-256 (1 MiB) carried in the progress ack (also makes resume safe); whole-file digest optional. `subtle.digest` cannot stream; hash per block or use hash-wasm.
- Consent: **prompt by default**; auto-accept only from a device explicitly marked trusted, below a size cap (100 MB), never auto-open. Distinct `declined` vs `busy`.
- Progress = receiver-committed bytes, not bytes handed to the socket (LocalSend's "stuck at 100 %" bug).
- Saving on Android: system `DownloadManager` (shipped) or `@capacitor/file-transfer` `downloadFile` + Share sheet. Never build the file in a Blob; never `Filesystem.writeFile` a whole file as base64.

### 2.7 Remote-only and files-only sessions

Semantics (now implemented on desktop and CLI): `intent ∈ {display, remote, files}`; `remote`/`files` → `Mode "none"`: no virtual display, no capture, input mapped to the main display, `ready{streamUrl:""}`; resize keeps the intent; the desktop shows a session card, not a display grid; the phone stops polling frames after ~6 s and says so.

Remaining: `files` should refuse input events server-side (today it accepts them); the phone's Display tab should not offer "View stream" for these intents; changing intent mid-session should re-handshake (the browser client does, the app does not).

### 2.8 Pairing and authentication

Today the 6-digit code travels in plaintext in every hello. One passive capture of one handshake is permanent access; rate limiting does not help because nothing is guessed. Every reference project uses the short secret **once**, inside a protocol where a passive observer learns nothing, to pin a long-lived per-device key.

**Phase A — per-device keys, ship first (≈2–3 days):**
- Phone/browser generate an Ed25519 keypair on first launch (`nacl.sign.keyPair()`, randomness from `crypto.getRandomValues`, which works on insecure origins). Server uses `crypto/ed25519`.
- First pairing: `HELLO {code, devicePub, deviceName}` → desktop prompt "Allow *Pixel 8*?" → store `{deviceId=SHA-256(devicePub), pub, name, addedAt, lastSeen}`. The trusted-device list becomes the allow-list.
- Every reconnect: server nonce `Ns`; client sends `Nc, sig = Ed25519(sk, "vior-auth-v1" ‖ serverId ‖ Ns ‖ Nc ‖ protoVersion)`; server verifies against the pinned key; nonces single-use. **The code is never sent again.**
- Revocation = delete the record (Revoke button per device in Settings).
- The pair code is accepted only while a "Pair new device" window is open (2–5 min), rotates after a successful pairing, and failures count globally with backoff (OWASP: per-IP limits are evadable, global lockouts are a DoS; recovery is "open the pairing window again").
- Honest limit: still plaintext transport; a sniffer of the pairing session learns a code that now opens only that window and still needs the desktop prompt.

**Phase B — PAKE on the code → session key:**
- CPace (CFRG-selected, `draft-irtf-cfrg-cpace`), suite CPACE-RISTR255-SHA512. Go: `filippo.io/edwards25519` / `gtank/ristretto255` (the reference `filippo.io/cpace` is experimental). JS: `@noble/curves` + `@noble/hashes` (pure JS, audited); tweetnacl cannot do this (no Edwards point ops / hash-to-curve). SPAKE2 (RFC 9382) is the fallback; its only JS port is unaudited and not constant-time.
- `PRS = code`, `sid = Ns‖Nc` (fresh per attempt), `CI = "vior-pair-v1"‖serverId‖devicePub‖protoVersion` (channel binding + downgrade protection). **Mandatory key confirmation** (`HMAC-SHA512(ISK, "confirm-A"/"-B"‖transcript)`) before anything else — a wrong code fails visibly and costs exactly one guess (Magic Wormhole's property).
- HKDF-SHA256 → one `secretbox` key per direction (already vendored in JS; `x/crypto/nacl/secretbox` in Go), nonce = direction byte ‖ 64-bit counter, receiver rejects non-increasing counters. Exchange and pin `devicePub`/`serverPub` inside the channel. The QR becomes optional.

**Phase C — mutual auth with forward secrecy and verification UI:**
- SIGMA-style: ephemeral X25519 each side, each signs the transcript with its pinned Ed25519 key; HKDF over the shared secret; same framing. (Go-only alternative: `flynn/noise` XX/KK.)
- Downgrade protection: a client that has pinned `serverPub` refuses plaintext or an older version and shows an error; server default becomes "require encrypted clients" once Phase B ships; delete `SecureOff`.
- UI: lock state on every session; per-device fingerprint (first 8 base32 chars of SHA-256(pub), Syncthing-style); optional 4-emoji / 6-digit short authentication string at pairing; blocking SSH-style warning when a pinned key changes.
- Storage: device private key in Android Keystore (Capawesome Secure Preferences / `@aparajita/capacitor-secure-storage`), not `localStorage`. Alternatively serve the app from `androidScheme: https` (Capacitor default) to get a secure context — WebCrypto and a stable origin — at the cost of migrating stored data once.

Pitfalls to avoid (each has bitten a reference project): short/weak PAKE scalars (croc/`schollz/pake`), leaking the PIN in a handshake field (Moonlight-iOS GHSA-g298), static truncated fingerprints (KDE Connect 2025 advisory), no key confirmation, an unbound PAKE in front of plaintext, per-IP-only lockouts.

### 2.9 USB specifics

- Phone reassembly buffer; reads sized ≥16 KiB and a multiple of `wMaxPacketSize`; test host writes that are exact multiples of 512 bytes (may need a zero-length packet).
- Use `ACTION_USB_ACCESSORY_DETACHED` + closing the original fd to unblock reads; override `onNewIntent` for re-attach under `singleTask`; rescan on resume.
- Same codec as Wi-Fi once §2.2 lands; until then add touch/key/scroll/file frames to the fixed table.
- Secure channel over USB: physical access is the trust decision today; under Phase A, USB sessions should still present the device key so the trusted list and revocation apply uniformly.
- MJPEG over USB 2.0 bulk is fine on bandwidth (~38 MB/s vs 3–7.5 MB/s needed); the CPU/latency case for H.264 is the same as on Wi-Fi.

### 2.10 Order of work

1. USB reassembly + touch wiring; USB/Wi-Fi ownership (one `Link` slot, same-device replacement). Unblocks USB entirely.
2. Phase A auth + pairing window + per-device revoke. Removes the plaintext-code-on-every-connect exposure without waiting for PAKE.
3. Session resume (`resumeToken`, grace, held-key release). Removes the display flicker and transfer loss on every blip.
4. File transfer v2 (HTTP bytes, resume, consent policy). Shares one code path with the browser client.
5. Codec unification + browser client on the shared modules. Deletes `client.js` and the USB fixed table.
6. Discovery stack (NSD plugin + beacon reader + bounded probe).
7. Phase B/C auth, `SecureRequired` default, drop cleartext manifest flags.

---

## Sources

Connection and discovery: [LocalSend protocol](https://github.com/localsend/protocol/blob/main/README.md) · [KDE Connect Android NSD/unicast MR](https://invent.kde.org/network/kdeconnect-android/-/merge_requests/375) · [KDE Connect Bluetooth multiplexing](https://github.com/KDE/kdeconnect-kde/blob/master/core/backends/bluetooth/Multiplexing%20protocol.md) · [Deskflow protocol](https://deskflow.github.io/deskflow/protocol_reference.html) · [Syncthing connection priorities](https://docs.syncthing.net/advanced/device-numconnections.html) · [scrcpy develop.md](https://github.com/Genymobile/scrcpy/blob/master/doc/develop.md) · [Socket.IO connection state recovery](https://socket.io/docs/v4/connection-state-recovery) · [MQTT 5 sessions](https://www.emqx.com/en/blog/mqtt-session) · [AWS backoff and jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/) · [WebSockets stall on Android](https://dev.to/jitchakraborty/websockets-can-stall-without-disconnecting-and-its-worse-on-android-3n6e) · [Android NSD](https://developer.android.com/develop/connectivity/wifi/use-nsd) · [UDP multicast on Android](https://lknuth.dev/writings/udp_multicast_on_android/) · [Android foreground service types](https://developer.android.com/develop/background-work/services/fgs/service-types) · [Wi-Fi low latency](https://source.android.com/docs/core/connect/wifi-low-latency) · [AOA accessory guide](https://developer.android.com/develop/connectivity/usb/accessory) · [libusb packet overflow](https://libusb.sourceforge.io/api-1.0/libusb_packetoverflow.html) · [AP isolation and LocalSend](https://helpforum.sky.com/t5/Broadband/Use-of-LocalSend-app-and-Access-Point-AP-Isolation/td-p/4833305)

File transfer: [LocalSend resume requests](https://github.com/localsend/localsend/issues/1191) · [LocalSend progress stuck](https://github.com/localsend/localsend/issues/259) · [KDE Connect share README](https://raw.githubusercontent.com/KDE/kdeconnect-kde/master/plugins/share/README) · [PairDrop network.js](https://raw.githubusercontent.com/schlagmichdoch/PairDrop/master/public/scripts/network.js) · [Snapdrop large files](https://github.com/RobinLinus/snapdrop/issues/612) · [Syncthing BEP](https://docs.syncthing.net/specs/bep-v1.html) · [Magic Wormhole transit](https://magic-wormhole.readthedocs.io/en/latest/transit.html) · [croc](https://github.com/schollz/croc) · [tus resumable upload](https://tus.io/protocols/resumable-upload) · [bufferedAmount](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/bufferedAmount) · [WebCrypto cannot stream](https://github.com/w3c/webcrypto/issues/250) · [Capacitor blob downloads #5478](https://github.com/ionic-team/capacitor/issues/5478) · [Capacitor blob download PR closed](https://github.com/ionic-team/capacitor/pull/5498) · [Capacitor File Transfer](https://capacitorjs.com/docs/apis/file-transfer) · [Capacitor Filesystem OOM](https://forum.ionicframework.com/t/ionic-save-large-file-capacitor-filesystem-out-of-memory-error/223180) · [StreamSaver in WebView](https://github.com/jimmywarting/StreamSaver.js/pull/270)

Pairing and auth: [RFC 9382 SPAKE2](https://www.rfc-editor.org/rfc/rfc9382.html) · [CPace draft](https://datatracker.ietf.org/doc/draft-irtf-cfrg-cpace/) · [Magic Wormhole attacks](https://magic-wormhole.readthedocs.io/en/latest/attacks.html) · [SPAKE2 interoperability](https://www.lothar.com/blog/57-SPAKE2-Interoperability/) · [noble-curves audits](https://github.com/paulmillr/noble-curves/tree/main/audit) · [filippo.io/cpace](https://pkg.go.dev/filippo.io/cpace) · [gospake2](https://pkg.go.dev/salsa.debian.org/vasudev/gospake2) · [NIST SP 800-121r2 (Bluetooth)](https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-121r2-upd1.pdf) · [KDE Connect protocol](https://github.com/KDE/kdeconnect-meta/blob/work/protocol-schemas/protocol.md) · [KDE advisory 2025-04-18](https://kde.org/info/security/advisory-20250418-3.txt) · [Moonlight-iOS GHSA-g298](https://github.com/moonlight-stream/moonlight-ios/security/advisories/GHSA-g298-gp8q-h6j3) · [Syncthing device IDs](https://docs.syncthing.net/dev/device-ids.html) · [RustDesk encryption](https://deepwiki.com/rustdesk/rustdesk-server/6.3-encryption-and-key-management) · [Windowcast](https://github.com/Droidtop/windowcast) · [OWASP authentication cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) · [getRandomValues on insecure contexts](https://developer.mozilla.org/en-US/docs/Web/API/Crypto/getRandomValues) · [Capawesome Secure Preferences](https://capawesome.io/docs/sdks/capacitor/secure-preferences/) · [weak PAKE scalars (CFRG thread)](https://mailarchive.ietf.org/arch/msg/cfrg/icl1AGo62iq8vQM3-NE8XS3bmvo/)
