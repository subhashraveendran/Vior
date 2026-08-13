# Vior — Secure Channel: Implementation Notes

Covers what phase 2a actually ships on the Go side, how to operate it, and —
most importantly — **what it does not protect**, which turned out to be more
than the original plan assumed.

## 1. Purpose

Encrypt the WebSocket payload path end-to-end so that the pair code, input
events, file chunks, and control messages are no longer readable by anyone on
the same network. Bootstrapped from a high-entropy secret delivered in the QR
payload; see `docs/securechan-handshake-architecture.md` for why that beats a
PAKE over the 6-digit code for the primary flow.

## 2. Finding: screen frames do not travel over the WebSocket

This is the significant discovery from implementation, and it materially
changes what "the connection is encrypted" is allowed to mean.

Screen capture does **not** use the WebSocket. It is a separate plain-HTTP
MJPEG stream:

```
capture → frameCh → distributeFrames → per-client chan
                                     ↓
   GET /stream    multipart/x-mixed-replace   (stream.go handleStream)
   GET /snapshot  image/jpeg                  (stream.go handleSnapshot)
```

The original plan (`docs/transport-security-plan.md` §1) states that "screen
frames, injected input, file chunks, and the 6-digit pair code all cross the
LAN unencrypted", and §6 phase 2 prescribes wrapping
`internal/protocol/session.go` read/write. **Wrapping the session encrypts
everything on that list except the screen frames**, because frames never pass
through it.

Two distinct problems follow, and they need to be kept apart:

| | Problem | Fixed here? |
|---|---|---|
| **Authorization** | `/stream` and `/snapshot` were gated on source IP alone — shared behind NAT, and spoofable on exactly the hostile networks this work targets | **Yes** — frame token, §4 |
| **Confidentiality** | The MJPEG stream is plain HTTP. A passive listener on the same network reads the screen regardless of any access check | **Yes, for secure sessions** — frames ride the sealed WebSocket, §11 |

Cleartext (legacy) sessions still stream plain-HTTP MJPEG, so under
`SecurePreferred` a session's protection depends on which kind of client
connected. `ServerStatus.Secure` reports the WebSocket's real state — and for
a secure session that state now covers the video too.

## 3. Architecture as implemented

```
WS upgrade
   ↓
negotiateSecure                      internal/stream/secure.go
   ├─ first frame is secure-init → completeHandshake
   │     ├─ handshake.Responder      internal/handshake
   │     ├─ securechan.NewChannel    internal/securechan
   │     ├─ session.EnableSecure
   │     └─ send secure-ready {frameToken}   ← first sealed message
   │     └─ WaitForHello                     ← sealed
   │
   ├─ first frame is hello + policy preferred → admit cleartext (logged loudly)
   └─ first frame is hello + policy required  → error upgrade_required
   ↓
pair-code check (unchanged) → OnClientConnect → ReadLoop
```

The handshake runs **before** hello by design, so the pair code is sealed
rather than sent in front of the encryption.

### Message flow

| Message | Sealed? | Notes |
|---|---|---|
| `secure-init`, `secure-resp`, `secure-confirm` | no | carry only ephemeral public keys and MACs, none of which are secret |
| `secure-ready` | **yes** | first sealed message; carries the frame token |
| `hello` and everything after | **yes** | includes the pair code |

### Two implementation details that are load-bearing

**Sealing happens under the write lock.** `securechan` rejects any counter not
strictly greater than the last accepted one. If two goroutines sealed
concurrently and then raced to write, the peer would see counter N+1 before N
and drop N as a replay, killing the channel. `Session.Send` therefore holds
`mu` across both seal and write so seal order is wire order.

**Both read failures are fatal.** An unsealed frame on a secure channel, or a
sealed frame that fails to open, drops the session. Continuing to serve input
events and file chunks on a channel that can no longer be authenticated is
strictly worse than forcing a reconnect.

## 4. Frame token

Derived from the session key (`handshake.FrameToken`, HKDF with its own label)
and delivered only inside the sealed channel. Clients pass it as `/stream?t=…`.

- Loopback is always allowed — that is the desktop's own preview.
- Secure session: token must match (constant-time) **and** the IP must match.
- Cleartext session: no token exists, so the historical IP-only check applies.
- Cleared on disconnect alongside `frameClientIP`.

This closes the IP-spoofing hole. It does **not** make frames confidential.

## 5. Configuration

`stream.SetSecurityMode()`:

| Mode | Behaviour |
|---|---|
| `SecurePreferred` (default) | accept both; cleartext sessions logged as such |
| `SecureRequired` | reject clients that do not handshake, with `upgrade_required` |
| `SecureOff` | handshake disabled; debugging escape hatch, delete before 1.0 |

The channel secret persists at `~/.vior/channel-secret` (0600, base64url,
atomic write via temp+rename), following the `pair.txt` / `server-id`
convention. `stream.RotateSecret()` invalidates every previously issued QR —
the "revoke all devices" action.

> **Deviation from the architecture review.** The review proposed rotating the
> secret per server start. That would invalidate every saved connection on
> every restart and force a fresh QR scan each time, fighting the reconnect
> behaviour mobile clients depend on. Persistence won; rotation is explicit and
> user-driven instead. TTL/single-use secrets remain deferred.

`/info` advertises `secure`, `secureMode`, `secureRequired`. The secret itself
is never published there — it travels only in the QR.

There is deliberately **no exported "does this secret match" helper**. A client
never sends the channel secret; it proves knowledge through the handshake MAC,
which is the whole reason the scheme resists a network attacker. A comparison
helper would invite the opposite pattern — accept the secret as a request
parameter and compare it — putting it on the wire in cleartext and dismantling
the design it appears to support.

## 6. Testing

`internal/handshake`: 30 tests plus committed cross-language vectors.

`internal/stream/secure_test.go` drives the real `handleWebSocket` over a live
WebSocket:

- full handshake → sealed hello admitted
- wrong secret → client rejects the server MAC; forged confirm → `secure_failed`
- `SecureRequired` + cleartext hello → `upgrade_required`
- `SecurePreferred` + cleartext hello → admitted (rollout compatibility)
- cleartext injection after handshake → session dropped
- replayed sealed frame → session dropped
- `frameClientAuthorized` matrix: token required, token does not override IP,
  cleartext fallback, nobody-paired, loopback preview
- secret encode/decode round trip and short-secret rejection

## 7. Known limitations

1. **Screen frames are cleartext for cleartext sessions only.** Secure
   sessions now receive frames over the sealed channel (§11); a legacy
   client's MJPEG stream remains plain HTTP until `SecureRequired` retires
   that path.
2. **No client implementation yet.** The Go server negotiates, but no shipped
   client speaks the handshake, so in practice every session is still
   cleartext under `SecurePreferred`. See §8.
3. **Typed 6-digit path is unprotected.** `MinSecretSize` makes this a hard
   error rather than a silent weakness, but a typed connection still gets no
   encryption until the deferred PAKE work.
4. **`/info`, `/download/{id}`, discovery beacon remain plain HTTP.** The
   `/download` id is now genuinely hard to obtain (it is only sent sealed), so
   the existing comment on that route became true rather than aspirational —
   but the body itself is unencrypted.
5. **No benchmarks yet.** Per-frame cost on the mobile JS side is still
   unmeasured.

## 8. What blocks the client

Both clients need a JS implementation of X25519, HKDF-SHA256, HMAC-SHA256, and
XSalsa20-Poly1305, and neither can use WebCrypto: `crypto.subtle` is
unavailable on insecure origins, which is exactly what `http://192.168.x.x` is.
(`crypto.getRandomValues` *is* available, so randomness is fine.)

The web client is a dependency-free file embedded via `go:embed`, and the
mobile build is `tsc` without bundling, so an npm import would not resolve at
runtime either. That leaves a decision:

- **Vendor `tweetnacl-js`** (public domain, Cure53-audited, ~7 KB) as a clearly
  demarcated third-party file. Safest cryptographically.
- **Add a bundler** for mobile and vendor only for the web client.
- **Hand-write the primitives.** Not recommended — writing X25519 and
  XSalsa20-Poly1305 from scratch for a security boundary is the highest-risk
  option available.

This touches the repository's rule against copying third-party code, so it is
flagged rather than decided.

## 9. Migration notes

No behaviour changes for existing clients: the default `SecurePreferred` admits
them exactly as before. The one observable change is that `/stream` now
requires `?t=…` **when the session is secure** — which only happens with a
client that has already negotiated, so no existing client can be affected.

`ServerStatus` gained `secure` and `secureMode`; the checked-in Wails bindings
in `desktop/frontend/wailsjs/go/models.ts` were updated to match.

## 10. Future improvements

- Client implementations (§8) — the server side of the sealed video path
  (§11) is waiting on them
- SPAKE2 for the typed short code
- TTL / single-use secrets
- Delete `SecureOff` before 1.0

(Resolved since first written: video for secure sessions moved onto the
sealed channel, and `SealTo` + a `sync.Pool` removed the per-frame `Seal`
allocation — both in §11.)

## 11. Sealed frame transport (issue #87)

Screen frames for a **secure** session travel as sealed binary WebSocket
messages on the same connection as everything else — the RustDesk-style
single encrypted stream recommended in
`docs/transport-security-reference-comparison.md` §6. This closes the
confidentiality gap identified in §2 for exactly the sessions that can
close it.

### Wire framing

The sealed *plaintext* of every message on a secure channel is discriminated
by its first byte:

| First byte | Meaning |
|---|---|
| `{` (0x7B) | JSON `Envelope` — every existing control message |
| `0x01` (`protocol.FramePrefixJPEG`) | screen frame; the rest is a complete JPEG |

JSON can never begin with 0x01, so one byte routes the message. Frames are
server→client only, sent via `Session.SendFrame`. This is still protocol v1 —
no shipped client spoke the secure protocol before frames were added, so
`handshake.Version` is unchanged.

### Delivery semantics

`MJPEGServer` registers the secure session as a distributor client (the same
bounded fan-out the MJPEG handlers use) and pumps it through
`pumpSecureFrames`, which drains the buffer to the newest frame before each
send — **latest-frame-wins**. Video never queues unboundedly, and because
each `SendFrame` is one bounded critical section on the session write lock,
it never head-of-line-blocks input or file messages.

An oversized frame (sealed size over the 1 MiB secure limit) is skipped with
`ErrFrameTooLarge` — logged once, session unharmed; the next frame supersedes
it. `SendFrame` on a cleartext session returns `ErrNotSecure` and frames stay
on the MJPEG path, preserving the §9 migration behaviour.

### Message size limits

The 128 KiB read limit still applies pre-handshake and for cleartext
sessions. `EnableSecure` raises it to 1 MiB — the peer has proven knowledge
of the channel secret, and sealed frames need the headroom. `SendFrame`
enforces the same 1 MiB bound outbound.

### Plain-HTTP endpoints while a secure session is active

`/stream` and `/snapshot` refuse **all non-loopback requests with 403** while
a secure session is active — even the paired client presenting a valid frame
token. Serving the same pixels over plain HTTP would hand a passive listener
everything the sealed channel protects. Loopback is exempt: the desktop's own
preview renders from localhost and pays no encryption cost. Cleartext
sessions keep the §4 behaviour (IP fallback) unchanged.

Consequently the `ReadyMessage.streamUrl` field is now **empty for secure
sessions** (`stream.StreamPathFor`). It previously advertised a token-less
`/stream` a secure client could never have fetched.

### Performance

`securechan.SealTo` seals into a caller-supplied buffer, and `SendFrame`
draws both its buffers (prefixed plaintext + sealed output) from a
`sync.Pool`, so the steady-state 30fps path allocates nothing per frame.
`BenchmarkSealFrame` in `internal/securechan` measures 100/300/600 KB frames
with and without buffer reuse.
