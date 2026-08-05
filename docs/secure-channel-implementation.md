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
| **Confidentiality** | The MJPEG stream is plain HTTP. A passive listener on the same network reads the screen regardless of any access check | **No** — needs a decision, §7 |

Until confidentiality is addressed, the UI must not tell a user their session
is protected without qualification. `ServerStatus.Secure` reports the
WebSocket's real state and nothing broader.

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

1. **Screen frames are still cleartext.** §2. The largest remaining gap, and it
   needs an architecture decision before it can be closed.
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

- Encrypt or relocate the video path (§7.1) — needs the decision in §2
- Client implementations (§8)
- SPAKE2 for the typed short code
- TTL / single-use secrets
- `Seal` currently allocates per frame; a buffer pool matters if video ever
  moves onto this path
- Delete `SecureOff` before 1.0
