// Ambient types for the `ViorSecure` global attached by
// clients/secure/vior-secure.js (loaded via <script> tag after
// clients/secure/vendor/tweetnacl.js — no imports/exports, matching the
// module "none" build; see globals.d.ts for the pattern).
//
// The runtime contract these types describe is pinned by
// internal/handshake/testdata/handshake_vectors.json and enforced by
// `node mobile-cap/tools/secure-channel-test.mjs`.

/** Wire payload of `secure-init` (byte fields are padded standard base64,
 *  as Go's encoding/json renders `[]byte`). */
interface ViorSecureInitPayload {
  v: number;
  epk: string;
  n: string;
}

/** Wire payload of `secure-confirm`. */
interface ViorSecureConfirmPayload {
  mac: string;
}

/** Parsed `data` of the server's `secure-resp`. Fields may be passed straight
 *  from JSON (base64 strings) or as raw bytes. */
interface ViorSecureRespPayload {
  epk: string | Uint8Array;
  n: string | Uint8Array;
  mac: string | Uint8Array;
}

interface ViorSecureFinishResult {
  /** `secure-confirm` payload to send back. */
  message: ViorSecureConfirmPayload;
  /** The 32-byte session key; feed to `ViorSecure.channel(key, true)`. */
  sessionKey: Uint8Array;
}

/** Client (initiator) side of the v1 handshake. Single-use: any failure is
 *  terminal — close the socket and start over from a fresh QR scan. */
interface ViorSecureInitiator {
  /** Produces the `secure-init` payload. Call exactly once, first. */
  initMessage(): ViorSecureInitPayload;
  /** Consumes `secure-resp`; throws if the server cannot prove knowledge of
   *  the QR secret (stale code or MITM) or the message is malformed. */
  finish(resp: ViorSecureRespPayload): ViorSecureFinishResult;
  /** The derived session key; throws before `finish` has succeeded. */
  sessionKey(): Uint8Array;
}

/** One direction-keyed record channel (XSalsa20-Poly1305, counter nonces,
 *  replay rejection) — the byte-exact twin of Go internal/securechan. */
interface ViorSecureChannel {
  /** Encrypts to `counter(8 BE) || secretbox(...)`; +24 bytes overhead. */
  seal(plaintext: Uint8Array): Uint8Array;
  /** Authenticates + decrypts a peer frame; throws on tampering, replay,
   *  reorder, or truncation. */
  open(frame: Uint8Array): Uint8Array;
}

interface ViorSecureHash {
  sha256(msg: Uint8Array): Uint8Array;
  hmacSha256(key: Uint8Array, msg: Uint8Array): Uint8Array;
  hkdfExtract(salt: Uint8Array | null, ikm: Uint8Array): Uint8Array;
  hkdfExpand(prk: Uint8Array, info: Uint8Array, length: number): Uint8Array;
  hkdf(ikm: Uint8Array, salt: Uint8Array | null, info: Uint8Array, length: number): Uint8Array;
}

interface ViorSecureStatic {
  /** Handshake wire version (1). */
  readonly VERSION: number;
  /** Minimum bootstrap-secret length in bytes (32). */
  readonly SECRET_MIN: number;
  /** Per-frame record-layer expansion in bytes (24). */
  readonly OVERHEAD: number;

  /** Starts the client side of the handshake against the QR bootstrap
   *  secret. Throws if the secret is shorter than 32 bytes. */
  createInitiator(secret: Uint8Array | string): ViorSecureInitiator;

  /** Builds the record layer over a completed handshake's session key.
   *  The Vior client passes `isInitiator: true`; the Go server mirrors it
   *  with `securechan.NewChannel(key, false)`. */
  channel(sessionKey: Uint8Array | string, isInitiator: boolean): ViorSecureChannel;

  /** Locally derives the bearer token for GET /stream and /snapshot
   *  (base64url, no padding) — same value the server sends in
   *  `secure-ready`. */
  frameToken(sessionKey: Uint8Array | string): string;

  /** Extracts the bootstrap secret from a scanned QR URL or its `#k=`
   *  fragment; null when absent, undecodable, or shorter than 32 bytes. */
  secretFromFragment(input: string | null | undefined): Uint8Array | null;

  bytesToBase64(bytes: Uint8Array): string;
  base64ToBytes(s: string): Uint8Array;
  bytesToBase64url(bytes: Uint8Array): string;
  base64urlToBytes(s: string): Uint8Array;
  hexToBytes(hex: string): Uint8Array;
  bytesToHex(bytes: Uint8Array): string;

  /** Test-suite hooks; not a stable API. */
  readonly hash: ViorSecureHash;
  deriveDirectionKeys(sessionKey: Uint8Array | string): { i2r: Uint8Array; r2i: Uint8Array };
}

declare const ViorSecure: ViorSecureStatic;
