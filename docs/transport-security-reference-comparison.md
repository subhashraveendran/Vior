# How comparable projects solve transport security — and what it changes here

Engineering analysis of the reference projects on the two questions currently
blocking Vior's secure channel: **how the video path is protected**, and **how
a browser client does crypto on an insecure origin**.

Ideas only — no code, documentation, or assets were taken from any of these
projects. Each is reachable from the citations at the end.

## 1. Summary of findings

| Project | Transport security | Video path | Bootstrap secret | Browser client? |
|---|---|---|---|---|
| **RustDesk** | Curve25519 key exchange → XSalsa20-Poly1305 (`secretbox`) over the connection | **Same encrypted stream** as input, audio and files | Password (salted SHA-256 challenge), Ed25519 server key signed by rendezvous server | No |
| **Deskreen** | Application-layer E2E in JavaScript, inspired by darkwire.io | Over the E2E'd WebRTC/socket path | Session-scoped, QR/link delivered | **Yes** |
| **KDE Connect** | TLS with self-signed certs, device UUID as certificate CN, certificate pinning at pair time | n/a (no screen streaming) | Certificate pinning on pairing | No |
| **Input Leap / Barrier** | Optional TLS; `--disable-crypto` escape hatch. Android client uses TOFU cert pinning | n/a (input only) | TOFU / fingerprint | No |
| **Vior (today)** | X25519 → HKDF → XSalsa20-Poly1305 on the WebSocket | **Separate plain-HTTP MJPEG — not encrypted** | 256-bit QR secret | **Yes** |

Two conclusions fall straight out of that table.

## 2. On the video path (issue #87)

**RustDesk puts everything on one encrypted stream.** After the Curve25519
exchange it switches the connection to `secretbox` and, from that point, video
frames, audio, keyboard input and file transfers all travel over the same
encrypted stream rather than separate channels.

Vior is the outlier here, and not deliberately — the split exists because the
MJPEG endpoint predates the secure channel, not because anyone chose a
two-channel security model. **No reference project runs a second, unencrypted
channel for its most sensitive data.**

That is strong evidence for **option 1 in #87** (move frames onto the encrypted
stream) over option 3 (label video as unencrypted). It does not make the
performance concerns disappear — RustDesk is native Rust on both ends and never
pays a JavaScript per-frame decrypt cost on a phone's UI thread, which is
exactly Vior's hard constraint. But "ship it cleartext and label it" is not a
position any comparable project has taken.

Also worth noting: the projects that *don't* encrypt by default are the
input-only ones (Barrier historically, Input Leap's `--disable-crypto`). The
moment a project carries the screen, encryption stops being optional.

## 3. On browser crypto (the client blocker)

**Deskreen hit precisely this problem and solved it the way this repo's notes
proposed.** Serving the client over HTTP without SSL makes
`window.crypto.subtle` unavailable, so they moved off WebCrypto to a pure-JS
library (`node-forge`), keeping end-to-end encryption on an insecure origin.

This confirms three things:

1. The constraint is real and well-known, not a misreading of the spec.
2. The answer the industry reaches for is **a pure-JS crypto library**, not
   hand-written primitives. Deskreen did not write its own AES.
3. Vior's situation is the same shape, so the same class of solution applies.

It does **not** by itself pick `node-forge` over `tweetnacl-js` for Vior. The
record layer is already NaCl `secretbox` on the Go side, and `tweetnacl-js` is
byte-compatible with it, Cure53-audited, and ~7 KB against node-forge's much
larger surface. `tweetnacl-js` remains the better fit *for this design* — but
the decision to depend on a vendored third-party crypto library rather than
write primitives is now supported by precedent rather than only by my judgement.

## 4. On TLS (why Vior differs, and why that is still correct)

KDE Connect and Input Leap both use TLS with self-signed certificates plus
pinning, and both work well. Neither has a browser client — KDE Connect pins a
certificate whose CN is the device UUID, which a native client can validate
however it likes.

That is the whole reason Vior cannot copy them, and it re-confirms the analysis
in `docs/transport-security-plan.md` §2: a browser will not accept a
self-signed cert for `wss://` without a manual interstitial that does not exist
for WebSockets, and Android's network-security-config trust anchors key on
hostnames rather than raw IP literals. The projects that get to use TLS are
exactly the ones that never have to satisfy a browser.

So: application-layer E2E remains the right call for Vior, and Deskreen — the
only reference project that also targets browsers — made the same call.

## 5. Issues found and fixed while doing this comparison

Auditing the implementation against these findings surfaced four defects,
all now fixed:

1. **Channel secret was in the URL query string.** `?…&k=<secret>` is
   transmitted to the server, lands in access logs, and rides in the `Referer`
   header of every subresource a page loaded from that URL requests. Moved to
   the **URL fragment** (`#k=…`), which is never sent to the server at all.
   JavaScript still reads it via `location.hash`.
2. **Frame endpoints lacked hardening headers.** `/stream` had no
   `X-Content-Type-Options`, and neither frame endpoint set `Referrer-Policy`.
   The frame token has to travel as a query parameter because an `<img>` tag
   cannot set request headers, so `no-referrer` is what stops that URL leaking
   onward. Both added.
3. **`/info` advertised no handshake version.** A client could not distinguish
   "this server speaks a protocol I do not know" from "my secret is stale" —
   two failures needing very different messages. Added `secureVersion`, with a
   test asserting `/info` advertises capability while never publishing the
   secret or the pair code in any encoding.
4. **A seal failure left the session alive.** Counter exhaustion makes the
   channel permanently unusable, but `Send` returned an error and let callers
   keep retrying against a dead channel. Now marks the session closed and drops
   the connection.

## 6. Recommendation

- **#87:** adopt option 1 — move frames onto the encrypted channel — with the
  per-frame cost measured on a real mid-range Android device *before*
  committing. RustDesk's single-stream design is the precedent; the open
  question is purely whether JavaScript can afford it, and that is a
  measurement, not a debate.
- **Client crypto:** vendor `tweetnacl-js` as a clearly demarcated third-party
  file. Precedent supports depending on an audited pure-JS library; the choice
  between it and node-forge follows from the record layer already being NaCl.

## Sources

- [RustDesk — Security and Authentication (DeepWiki)](https://deepwiki.com/rustdesk/rustdesk/2.5-security-and-authentication)
- [RustDesk — Client-Server Communication (DeepWiki)](https://deepwiki.com/rustdesk/rustdesk/2.3-client-server-communication)
- [RustDesk — high-level explanation of end-to-end encryption (discussion)](https://github.com/rustdesk/rustdesk/discussions/2239)
- [Deskreen (project site)](https://deskreen.com/)
- [KDE Connect — desktop repository](https://github.com/kde/kdeconnect-kde)
- [KDE Connect / Valent — protocol reference](https://valent.andyholmes.ca/documentation/protocol.html)
- [Input Leap — data encryption issue](https://github.com/input-leap/input-leap/issues/384)
