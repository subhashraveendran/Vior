# Vior — Transport Security Plan (#51) — 2026-07-16

Scoping document for closing issue **#51** (all traffic is cleartext `ws://`/`http://`
today). This captures the research, the exact migration surface, the architecture
options, a recommendation, and a phased plan. **One architecture decision is required
before implementation** (see §5).

## 1. Problem

Screen frames, injected input, file chunks, and the 6-digit pair code all cross the
LAN unencrypted. There is no `crypto/tls` anywhere in the Go tree; the HTTP+WS server
serves plain HTTP (`internal/stream/stream.go` `Start`, listener at `:489`, server at
`:469`). Clients hardcode `http://`/`ws://` (see §3). Four Android layers exist solely
to permit cleartext.

## 2. The constraint that rules out naive WSS

Vior has **two client types that both connect to the host's raw LAN IP**: any web
browser, and an Android Capacitor WebView. Self-signed transport TLS fails them:

- **Browsers:** there is **no click-through for `wss://`** the way there is for a
  top-level `https://` navigation — a failed WebSocket TLS handshake is rejected
  silently (`ERR_CERT_AUTHORITY_INVALID`). The user would have to first visit
  `https://<ip>:<port>` in the address bar and accept the interstitial (scoped to that
  exact host **and port**), and public CAs will not issue for private IPs. Firefox has
  had this as an open bug for ~15 years (bugzilla 594502).
- **Android WebView:** trusting a self-signed cert via `network_security_config.xml`
  `<trust-anchors>`/`<pin-set>` keys on **hostnames, not raw IP literals** (behaves
  inconsistently/broken for IPs). The "just make it work" `onReceivedSslError` /
  custom-TrustManager override is **MITM-vulnerable and Play-Store-blocked since 2016**.
- A `.local` mDNS **hostname** fixes the Android side but still does **not** make a
  browser auto-trust a self-signed cert.

Conclusion: self-signed `wss://` gives a smooth experience only to the Android app
against a hostname — it does **not** deliver the "connect from any browser" promise.

Sources:
https://developer.android.com/privacy-and-security/security-config
https://bugzilla.mozilla.org/show_bug.cgi?id=594502
https://github.com/flutter/flutter/issues/65841
https://support.google.com/faqs/answer/7071387
https://letsencrypt.org/docs/certificates-for-localhost/

## 3. Migration surface (exact touch points)

**Server / listener** — `internal/stream/stream.go`: `http.Server` at `:469-475`,
`net.Listen("tcp")` at `:489`, `Upgrader` at `:434-436`. No TLS today. Persistent
state convention is `~/.vior/` (`pair.txt` at `:94`, `server-id` at `:195`;
`internal/machineid`, `internal/trust` follow the same pattern).

**URL builders (emit scheme/port):**
- `cmd/vior/cli/start.go:255` (`printQR`), `:242/:245` (`printURLs`) — `http://%s:%d`
- `desktop/app.go:254` (`GetServerStatus`), `:259` (localhost), `:261`
  (`QRCodeDataURL(s.URL + "?pair=" + PairCode())`)
- `internal/network/network.go:19-43` (QR encoders — embed the URL as-is)
- `/info` payload `internal/stream/stream.go:761`; discovery beacon
  `internal/discovery/discovery.go:75-81` (carries `port`, no scheme/fingerprint)

**Clients (hardcoded scheme):**
- Mobile — `connect.ts:224` (`ws://`), `:111/:337/:413/:580` (`http://`),
  `parseAddrInput` `:762`; `discovery.ts:123`; `qr.ts:14/:17`
- Web client — `webclient/client.js:288` (`http://`), `:1233-1235` (derives
  `ws:`/`wss:` from `location.protocol` — already scheme-aware)
- Desktop frontend — uses backend-provided URLs (no direct sockets)

**Android cleartext enablers (remove/adjust post-migration):**
- `mobile-cap/capacitor.config.json:6` (`cleartext`), `:10` (`allowMixedContent`)
- `mobile-cap/android/app/src/main/AndroidManifest.xml:20` (`usesCleartextTraffic`)
- `mobile-cap/android/app/src/main/res/xml/network_security_config.xml:3`
- CI regenerates `android/` and overlays these; overlay sources are the checked-in
  files above (`.github/workflows/build.yml:259-292`).

**Pairing secret plumbing (reused as the PAKE password in Option B):**
- Server: `internal/stream/stream.go` `derivePair` `:62-86`, `PairCode()` `:334`,
  constant-time probe `:778`, hello compare `:934`.
- Clients: `connect.ts:264/:287` (hello `pairCode`), web `client.js:20` (`vior_pair`).

## 4. Options

### Option A — Transport TLS (self-signed cert + fingerprint pinning)
Generate a per-device self-signed cert (IP SANs) in a new `internal/tlscert` under
`~/.vior/`, serve via `http.Server.ServeTLS`, publish the SPKI-SHA256 fingerprint via
QR/`/info`. Android trusts it via network-security-config **(needs a hostname, not a
raw IP)**; **browsers require manual cert import** — so the browser client regresses.
Standard, audited TLS stack; but does not satisfy the dual-client requirement.

### Option B — Application-layer E2E over `ws://` (RECOMMENDED)
Keep `ws://` transport; encrypt the WS payloads end-to-end. Bootstrap the session key
from the existing 6-digit pair code with a **PAKE (SPAKE2)** so a low-entropy code is
safe (blocks offline brute-force; MITM limited to one online guess), then a **NaCl
`secretbox` / libsodium `crypto_secretstream`** record layer for all frames.
- Works **identically and frictionlessly on both clients** — no cert provisioning, no
  OS/browser trust changes, no IP-SAN/hostname gymnastics, no Play-Store risk.
- Turns the pair code into a real authenticator.
- Libraries: Go host — `filippo.io`/`golang.org/x/crypto/nacl` (record) + a SPAKE2 impl
  (`gospake2`); browser/TS — `tweetnacl-js` (7KB, Cure53-audited, byte-compatible with
  Go `nacl`) or `libsodium.js` + `spake2-wasm`. Works on insecure origins (no WebCrypto
  dependency).
- This is what the closest analogs ship: **Deskreen/darkwire** (app-layer E2E in JS
  precisely because WebCrypto is unavailable over plain HTTP), **magic-wormhole**
  (SPAKE2 from a short code), **RustDesk** (NaCl box/secretbox).

### Option C — Hybrid (mDNS hostname + pinned cert for Android, app-layer for browser)
Best-in-class per client but **two security models to maintain**; more complex, more
surface. Not recommended unless the browser client is dropped.

Sources:
https://github.com/dchest/tweetnacl-js
https://github.com/flynn/noise
https://pkg.go.dev/github.com/flynn/noise
https://salsa.debian.org/vasudev/gospake2
https://github.com/okdistribute/spake2-wasm
https://deepwiki.com/pavlobu/deskreen
https://docs.syncthing.net/dev/device-ids.html

## 5. Decision required

**Which transport-security architecture?** Recommendation: **Option B (app-layer E2E,
SPAKE2 + NaCl)** — it is the only approach that keeps the "any browser" client working
and matches how comparable tools solve the no-CA/raw-IP problem.

Trade-off to accept for B: the confidentiality/authentication now lives in **code we
own**, not in the audited TLS stack. The PAKE layer is the delicate part; the available
SPAKE2 libs are **not formally audited** and Go↔JS interop must be validated
byte-for-byte. Mitigations: use audited NaCl primitives for the bulk record layer, keep
the PAKE surface minimal, make the pair code single-use/rate-limited, and get the
handshake independently reviewed before release.

## 6. Phased implementation plan (once Option B is chosen)

1. **`internal/securechan` (Go)** — SPAKE2 handshake keyed off `PairCode()` → NaCl
   `secretbox` session; framing, nonce/counter discipline, replay protection. Unit +
   interop test vectors. (Decision-independent record layer can start immediately.)
2. **Wire into the WS path** — wrap `internal/protocol/session.go` read/write; perform
   the handshake immediately post-`Upgrade`, before `WaitForHello`. Behind a capability
   flag negotiated in a pre-hello message so old clients fail closed with a clear error.
3. **Clients** — shared TS `securechan` module (tweetnacl-js) used by mobile
   (`connect.ts`) and web (`webclient/client.js`); handshake before sending hello.
4. **Discovery/QR** — advertise a `secure: true` capability (beacon + `/info`); QR/URL
   unchanged (still `ws://`, since encryption is app-layer).
5. **Retire cleartext enablers** — only after all clients negotiate the secure channel:
   remove the four Android flags (editing the CI overlay sources) and tighten CSP
   `connect-src`.
6. **Security review + real-device test** — Android app + at least Chrome/Firefox/Safari
   browser clients, before flipping the default and closing #51.

## 7. Deferred items that unlock once §6 lands
- Interactive host approval (blocking Allow/Deny) — safe to make "you're securely
  paired" claims only over the encrypted channel.
- One-time / TTL pair codes (SPAKE2 makes single-use codes natural).
