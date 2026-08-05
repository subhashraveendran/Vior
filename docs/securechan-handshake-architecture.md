# Vior — Secure Channel Handshake: Architecture Review (Phase 2)

**Status:** awaiting approval — no implementation has started.
**Predecessor:** `docs/transport-security-plan.md` (§6 phase 1 landed as
`internal/securechan`, commit `d9ea2c4`).
**Scope of this review:** how two peers *agree on* the 32-byte session key that
`securechan.NewChannel` already consumes, and how that handshake is spliced into
the live WebSocket path.

This document exists because the handshake is the one part of the transport-security
work that is genuinely delicate: the record layer uses audited primitives and is
mechanical, but key agreement from a shared secret is where the real security
property is won or lost.

---

## 1. Where we actually are

`internal/securechan` is complete, unit-tested, and **imported by nothing**:

```
$ grep -rn "securechan" --exclude-dir=node_modules . | grep -v internal/securechan/
docs/transport-security-plan.md:131
docs/transport-security-plan.md:137
```

It provides `NewChannel(sharedKey [32]byte, initiator bool) → Seal/Open` with
HKDF-split directional keys, an 8-byte big-endian counter nonce, and
strictly-monotonic replay rejection. Its doc comment explicitly refuses to offer a
"derive the key from the pair code" helper, deferring exactly the question this
document answers.

**Integration surface is small and well-contained.** All WebSocket I/O funnels
through three call sites in `internal/protocol/session.go`:

| Site | Line | Role |
|---|---|---|
| `Session.Send` | `session.go:114` | single write path (`Conn.WriteMessage`, `:125`) |
| `Session.ReadLoop` | `session.go:137` | steady-state read path (`Conn.ReadMessage`, `:158`) |
| `Session.WaitForHello` | `session.go:236` | pre-auth read of the first frame (`:239`) |

Server-side admission lives in `internal/stream/stream.go`: `Upgrade` at `:859`,
`WaitForHello` at `:908`, constant-time pair compare at `:937`.

That means the record layer can be introduced by wrapping **one encode point and
two decode points** — not by touching every message type. This is the strongest
argument that Phase 2 is tractable.

---

## 2. The problem the plan document under-specified

`transport-security-plan.md` §5 recommends SPAKE2 keyed off the 6-digit pair code,
and honestly flags the cost: *"the available SPAKE2 libs are not formally audited
and Go↔JS interop must be validated byte-for-byte."*

That framing accepted an assumption worth challenging: **that the 6-digit pair code
is the only bootstrap secret available.**

It is not. The dominant onboarding path is a QR scan, and a QR code is a
machine-to-machine channel — it can carry 256 bits as cheaply as it carries 20.
Today it deliberately carries only the short code:

```go
// desktop/app.go:261
qr, err := network.QRCodeDataURL(s.URL + "?pair=" + stream.PairCode())
```

The 6 digits exist for *human typing*, not because the channel is narrow. Conflating
"the secret a human types" with "the secret the QR transports" is what forces a PAKE
into the critical path.

### Why this distinction decides the whole design

A plain authenticated Diffie-Hellman (X25519 + a transcript MAC keyed by the shared
secret `S`) is sound **iff `S` has enough entropy**. An active MITM completes a DH
with the client, captures the client's confirmation MAC, and can then brute-force `S`
offline. With `S` = 6 digits that is 10^6 HMAC evaluations — milliseconds. With
`S` = 32 random bytes it is infeasible forever.

A PAKE (SPAKE2/CPace) is precisely the tool that makes low-entropy `S` safe: it
removes the offline attack and limits the MITM to a *single online guess* per
attempt. It is the right answer for a typed 6-digit code — and it is *unnecessary
overhead* for a 256-bit QR-delivered key.

Note this is not a gap a longer human code can close. A 4-word phrase from a
2048-word list is ~44 bits; 2^44 offline MAC evaluations is well within reach of
commodity GPU hardware. Only a PAKE fixes the typed-code path.

---

## 3. Proposed architecture

### 3.1 Split the bootstrap secret by delivery channel

| Path | Secret | Entropy | Key agreement |
|---|---|---|---|
| **QR scan** (primary) | 32 random bytes, fresh per server start | 256 bit | X25519 + HKDF, transcript MAC bound to `S` |
| **Manual typing** (fallback) | 6-digit pair code | ~20 bit | requires a PAKE — **deferred to Phase 2b** |

This is the core recommendation: **ship real E2E for the QR path using only audited
primitives, and treat the typed-code path as a separate, later, clearly-scoped piece
of work.** It removes the unaudited SPAKE2 dependency from the critical path of the
first secure release without pretending the typed path is solved.

### 3.2 Component diagram

```
┌──────────────── Desktop / CLI host (Go) ─────────────────┐
│                                                          │
│  stream.go                                               │
│   ├─ Upgrade (:859)                                      │
│   ├─ ▸ NEW: handshake.Server(conn, secret)  ── returns ──┼──▶ [32]byte
│   ├─ WaitForHello (:908)   ← now reads through Channel   │
│   └─ pair compare (:937)                                 │
│                                                          │
│  internal/securechan       (EXISTS — record layer)       │
│  internal/handshake        (NEW — key agreement)         │
│   ├─ x25519 ephemeral keypair                            │
│   ├─ HKDF-SHA256 transcript binding                      │
│   └─ HMAC-SHA256 mutual confirmation                     │
└──────────────────────────────────────────────────────────┘
                    │  ws://  (still cleartext transport,
                    │          payloads E2E-encrypted)
┌───────────────────┴──── Clients ─────────────────────────┐
│  shared TS module  src/lib/securechan.ts  (tweetnacl-js) │
│   ├─ mobile:  connect.ts                                 │
│   └─ web:     webclient/client.js                        │
└──────────────────────────────────────────────────────────┘
```

`internal/handshake` is proposed as a **separate package** from `securechan`: key
agreement and record framing have different threat models, different review needs,
and different change cadences. `securechan` stays a pure, dependency-light record
layer that knows nothing about pair codes or X25519.

### 3.3 Handshake sequence (QR path)

```
Client                                              Server
  │                                                    │
  │──── WS Upgrade (plain ws://) ─────────────────────▶│
  │                                                    │
  │  generate esk_c, epk_c = X25519(esk_c)             │  generate esk_s, epk_s
  │  n_c = 16 random bytes                             │  n_s = 16 random bytes
  │                                                    │
  │──── {v:1, epk_c, n_c}  "secure-init" ─────────────▶│
  │                                                    │
  │                     ss  = X25519(esk_s, epk_c)     │
  │                     T   = "vior-hs v1"‖epk_c‖epk_s‖n_c‖n_s
  │                     PRK = HKDF-Extract(salt=T, ikm=ss‖S)
  │                     k_sess    = HKDF-Expand(PRK,"session",32)
  │                     k_conf_s  = HKDF-Expand(PRK,"confirm-s",32)
  │                     k_conf_c  = HKDF-Expand(PRK,"confirm-c",32)
  │                                                    │
  │◀─── {epk_s, n_s, mac_s=HMAC(k_conf_s,T)} ──────────│  "secure-resp"
  │                                                    │
  │  derives the same; verifies mac_s in constant time │
  │  ── mismatch ⇒ close, never send mac_c ──          │
  │                                                    │
  │──── {mac_c = HMAC(k_conf_c, T)}  "secure-confirm" ▶│
  │                                                    │  verify mac_c (const-time)
  │                                                    │  ── mismatch ⇒ close ──
  │                                                    │
  │═══ securechan.NewChannel(k_sess, initiator) ═══════│
  │                                                    │
  │──── Seal(hello{pairCode,…}) ──────────────────────▶│  WaitForHello reads via Open
  │◀─── Seal(ready{…}) ────────────────────────────────│
```

**Why the server sends its confirmation first.** It proves server knowledge of `S`
before the client reveals anything, so a client that scanned a stale QR aborts
without handing a MAC to an impostor. The residual offline-attack exposure on
`mac_s` is exactly why `S` must be high-entropy — restated: this ordering is a
usability and fail-fast property, **not** a substitute for entropy.

**Why the pair code is still sent inside `hello`.** Belt-and-braces, and it keeps
`stream.go:937` and the whole trust-store/`deviceId` flow untouched in Phase 2a. The
handshake authenticates the *channel*; the existing check authorises the *device*.
Collapsing the two is a Phase 3 cleanup, not part of this change.

### 3.4 Connection state machine

```
        ┌──────────┐  upgrade ok    ┌──────────────┐
   ─────▶  UPGRADED ├───────────────▶ AWAIT_INIT   │
        └──────────┘                └──────┬───────┘
                                           │ secure-init (≤5s)
                          timeout/         ▼
                          bad version ┌──────────────┐
              ┌──────────────────────┤  AWAIT_CONF  │
              │                      └──────┬───────┘
              ▼                             │ valid mac_c
        ┌──────────┐                        ▼
        │  CLOSED  │◀──── mac mismatch ┌──────────┐
        └──────────┘      read error   │  SECURE  │
              ▲                        └────┬─────┘
              │                             │ hello (decrypted)
              │      Open() → ErrReplay     ▼
              └──────────────────────┬─ AUTHENTICATED ─▶ steady state
                                     │  (existing pair check)
```

Every edge out of a failure is **CLOSED** — the design fails closed by construction.
There is no path from a failed handshake into a plaintext fallback; that is the
single most important property to preserve under future edits.

### 3.5 Version negotiation and old clients

`secure-init` carries `v:1`. A pre-Phase-2 client sends `hello` as its first frame
instead. The server distinguishes them by envelope `type` on the first message and
must respond with a **structured, actionable error** (`{code:"upgrade_required"}`)
rather than a bare close — the plan document's phase 2 note ("old clients fail
closed with a clear error") is a UX requirement, not just a security one.

During rollout the server needs a policy knob:

- `secure=required` — reject cleartext clients (target end state)
- `secure=preferred` — accept both, log and surface cleartext sessions in the UI
- `secure=off` — escape hatch for debugging

Recommendation: land as `preferred`, flip to `required` in Phase 5 once mobile and
web clients both ship, and delete `off` before 1.0 rather than letting a
downgrade switch ossify into the product.

---

## 4. Alternatives considered

| # | Design | Verdict |
|---|---|---|
| **A** | Transport TLS, self-signed + fingerprint pinning | **Rejected** — already analysed in the predecessor doc §2/§4A: browsers have no click-through for `wss://`, and Android trust anchors key on hostnames not raw IPs. Breaks the "any browser" client. |
| **B** | SPAKE2 from the 6-digit code (predecessor's recommendation) | **Deferred, not rejected.** Correct and necessary for the typed-code path. Puts an unaudited Go SPAKE2 lib plus a WASM blob into the critical path of every connection for a property the QR path can get from audited primitives. Right answer for Phase 2b. |
| **C** | **X25519 + high-entropy QR secret** | **Recommended.** Audited primitives only (`x25519`, `HKDF-SHA256`, `HMAC-SHA256`, `nacl/secretbox`; `tweetnacl-js` is Cure53-audited and byte-compatible). No WASM, no new Go crypto dependency — `golang.org/x/crypto` is already vendored for the record layer. |
| **D** | Noise Protocol Framework (`flynn/noise`, NN/NK/XX) | **Close second.** A real framework with a real spec beats a hand-rolled transcript, and `flynn/noise` is mature. Rejected on the *client* side: the JS Noise implementations are markedly less mature than `tweetnacl-js`, and Noise's value is mostly realised when you use its full pattern/rekey machinery. Worth revisiting if the TS side ever grows beyond a single handshake. |
| **E** | Raw ECDH with no secret binding (TOFU) | **Rejected.** Encrypts against passive eavesdroppers only; any active LAN attacker MITMs silently. Given the threat model here is explicitly hostile WiFi (issue #51), this fails the actual requirement. |

**On hand-rolling (the honest objection to C):** option C *is* a bespoke handshake,
and bespoke handshakes are where cryptographic systems usually break. Two things
bound that risk: the construction is a textbook authenticated-DH with full transcript
binding (no novel structure), and the failure mode of getting it subtly wrong with a
256-bit secret is far more forgiving than getting a PAKE subtly wrong with a 20-bit
one. It still must not ship without the independent review called for in §8.

---

## 5. Risk analysis

| Risk | Severity | Mitigation |
|---|---|---|
| Hand-rolled handshake has a subtle flaw | **High** | Textbook authenticated-DH shape; full transcript binding; independent review before flipping to `required`; no novel constructions |
| Go↔JS byte-level interop drift | **High** | Committed test-vector file (`testdata/handshake_vectors.json`) consumed by *both* Go and TS test suites; CI runs both |
| Typed 6-digit path stays cleartext after 2a | **High** | Must be surfaced in the UI as not-encrypted, not silently downgraded. Phase 2b tracked as its own issue before 1.0 |
| QR key parsers reject base64 | **Medium** | `qr.ts:13` matches `[A-F0-9]+`, `settings.ts:140` matches `[0-9A-Z]+` — both **will silently drop** a base64url key. Parser update is a required, easily-missed subtask |
| Nonce/counter reuse across reconnect | **Medium** | Fresh ephemeral keys per connection ⇒ fresh `k_sess` ⇒ counter restart is safe by construction. Must never cache `k_sess` across reconnects |
| Secret in QR leaks via screenshot/shoulder-surf | **Medium** | Rotate `S` per server start; add TTL + single-use in the deferred work already noted in the predecessor §7 |
| Handshake latency hurts reconnect UX | **Low** | One extra RTT + 2 X25519 ops; see §7 |
| `secure=off` ossifies into production | **Medium** | Delete the flag before 1.0; never make it the default |

---

## 6. Edge cases to cover in tests

**Protocol:** malformed/truncated `secure-init`; unknown version; `hello` sent first
(old client); frames sent before handshake completes; duplicate `secure-init`;
oversized init frame (read limit already asserted at `session.go:102`); wrong-secret
MAC mismatch on each side independently; replayed `secure-confirm`.

**Lifecycle:** disconnect mid-handshake; server restart mid-session (client must
re-handshake, not reuse `k_sess`); rapid reconnect storms; sleep/hibernate resume;
Android Doze socket kill (interacts with the app-level ping at `protocol.go:41`);
pair code changed via `SetPairCode` while a session is live.

**Concurrency:** two clients handshaking simultaneously; `Seal` called concurrently
from the ping loop and the app path (already safe per `securechan` doc, needs an
explicit regression test at the session layer); handshake racing `Session.Close`.

**Platform:** browser without WebCrypto over insecure origin (the reason
`tweetnacl-js` was chosen — must be verified, not assumed); Capacitor WebView;
Safari; USB/AOA path (does it bypass this entirely? — **open question, see §9**).

**Resource:** nonce exhaustion (`ErrNonceExhausted` currently has no session-level
handler); memory growth on repeated failed handshakes; unbounded pre-handshake
connection accumulation.

---

## 7. Performance analysis

- **Handshake cost:** 2 X25519 scalar mults per side (~50–60 µs each in Go; ~1–2 ms
  in `tweetnacl-js` on a mid-range phone), plus one extra round trip. On a LAN RTT of
  1–3 ms the added connect latency is ~5–10 ms — imperceptible against the existing
  10 s hello timeout.
- **Steady state:** XSalsa20-Poly1305 runs at ~1–2 GB/s in Go. The bottleneck case is
  the video frame path; at 60 fps × ~64 KB that is ~4 MB/s — roughly 0.3 % of one
  core. **This must be measured, not assumed** — the mobile JS side is the real
  question, since `tweetnacl-js` is ~10–50 MB/s and the per-frame decrypt lands on
  the UI thread.
- **Allocation:** `Seal` allocates a fresh buffer per frame (`securechan.go:131`). At
  60 fps this is measurable GC pressure; a buffer-pool variant is a known follow-up,
  deliberately out of scope here.
- **Bundle size:** `tweetnacl-js` is ~7 KB minified — negligible, and notably smaller
  than a SPAKE2 WASM blob.

Recommended gates: a Go benchmark for `Seal`/`Open` at frame size, and a real-device
mobile measurement of decrypt cost per frame **before** enabling on the video path.

---

## 8. Security analysis

**Gained:** confidentiality and integrity against passive LAN eavesdroppers and
active MITM (for the QR path); replay/reorder protection (already in the record
layer); the QR secret becomes a real channel authenticator rather than a
guessable admission token.

**Explicitly NOT gained in Phase 2a:**
- The typed 6-digit path remains unauthenticated against an active MITM. This is a
  deliberate, scoped deferral and **must be visible to the user**, not buried.
- `/info`, `/download/{id}`, and the discovery beacon remain plain HTTP. The record
  layer covers the WS payload path only. Issues #71/#73 are not fully closed by
  Phase 2a and should not be closed by it.
- Issue #78 (pair code derived from a predictable machine-id) is *upstream* of this
  work: a predictable `S` weakens the typed path further and should be fixed
  independently, regardless of the handshake outcome.

**Secrets handling:** `S` must be generated with `crypto/rand`, stored in `~/.vior/`
with `0600` alongside the existing `pair.txt`/`server-id` convention, rotated per
server start, and never logged. Confirmation MAC comparison must use
`crypto/subtle` — the codebase already sets this precedent at `stream.go:778`
and `:937`.

**Cleartext enablers** (`AndroidManifest.xml:20`, `network_security_config.xml:3`,
`capacitor.config.json:6/:10`) stay until Phase 5. Removing them earlier breaks the
transport, since the transport is still `ws://` by design.

---

## 9. Open questions — I need decisions on these

1. **Scope split — do you accept deferring the typed 6-digit path to 2b?** This is
   the central call. Accepting means the first secure release protects QR-onboarded
   sessions and honestly labels typed-code sessions as unencrypted. Rejecting means
   pulling SPAKE2 into Phase 2a and accepting the unaudited dependency.
2. **Does the USB/AOA transport need the same treatment?** A physical cable is a
   meaningfully different threat model, and `internal/usb` did not surface in the
   predecessor's migration inventory. I lean **no** for Phase 2 — but it should be a
   stated decision, not an omission.
3. **`internal/handshake` as a new package, or fold into `securechan`?** I recommend
   separate, per §3.2.
4. **Rollout default:** land as `secure=preferred`, or go straight to `required`?
5. **Does the web client get the same treatment in 2a, or mobile first?** The web
   client at `webclient/client.js:1233` is already scheme-aware and is the cheaper of
   the two to convert.

---

## 10. Proposed phasing (if approved)

| Phase | Deliverable | Issue |
|---|---|---|
| 2a-1 | `internal/handshake` + test vectors, no wiring | new |
| 2a-2 | High-entropy `S`: generation, `~/.vior/` persistence, QR/URL carriage, **parser updates** | new |
| 2a-3 | Wire into `session.go` Send/ReadLoop/WaitForHello behind `secure=preferred` | new |
| 2a-4 | Shared TS `securechan` + handshake module; web client first, then mobile | new |
| 2a-5 | Benchmarks + real-device frame-path measurement | new |
| 2b | SPAKE2 for the typed-code path | new |
| 3 | Flip to `required`; retire cleartext enablers; close #71/#73 | #71, #73 |

Each row is one branch and one PR, per the repository workflow.

---

## 11. Recommendation

Adopt **Option C** for Phase 2a: X25519 + HKDF-SHA256 transcript binding + HMAC
mutual confirmation, keyed by a 256-bit QR-delivered secret, feeding the existing
`securechan` record layer. Defer SPAKE2 to Phase 2b for the typed-code path, and
label that path as unencrypted in the UI until it lands.

The reason to prefer this over the predecessor document's recommendation is narrow
and specific: it delivers the same end-user security property for the primary
onboarding flow using only audited primitives, and it isolates the genuinely risky
cryptographic engineering into a smaller, separately-reviewable change.

**Stopping here for approval before any implementation, per the architecture-review
process.** Decisions needed on the five items in §9.
