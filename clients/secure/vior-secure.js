// vior-secure.js — client (initiator) side of Vior's secure channel.
//
// Plain browser script: no imports, no exports. Loading it attaches a single
// `ViorSecure` global, so the same file works in both consumers:
//
//   - mobile-cap (Capacitor): loaded via <script> tag; the TS sources are
//     compiled with bare `tsc`, module "none", and see the global through
//     mobile-cap/src/js/vior-secure.d.ts.
//   - the embedded web client (internal/stream/webclient): a dependency-free
//     go:embed bundle, same <script> tag pattern.
//
// This file is the canonical copy; clients consume it from clients/secure/.
// It must be loaded AFTER clients/secure/vendor/tweetnacl.js, which provides
// X25519 (nacl.scalarMult) and XSalsa20-Poly1305 (nacl.secretbox).
//
// It is the JS counterpart of two Go packages and must match them
// byte-for-byte on the wire:
//
//   internal/handshake/handshake.go   — the v1 authenticated-DH handshake
//   internal/securechan/securechan.go — the record layer
//
// Any change here that alters bytes on the wire is a protocol break; the
// authoritative cross-language vectors live in
// internal/handshake/testdata/handshake_vectors.json and are enforced by
// mobile-cap/tools/secure-channel-test.mjs (run: node mobile-cap/tools/secure-channel-test.mjs).
//
// SHA-256 / HMAC / HKDF are implemented locally below (tweetnacl has no
// SHA-256). They are validated against RFC 6234 / RFC 4231 / RFC 5869
// vectors, against Node's crypto module on random inputs, and against the
// committed handshake vectors — see the test script above.
(function (root) {
  'use strict';

  // tweetnacl may be loaded before or after this file in exotic setups;
  // resolve it lazily so script order is only checked at first use.
  function getNacl() {
    var n = root.nacl;
    if (!n || !n.scalarMult || !n.secretbox) {
      throw new Error('ViorSecure: tweetnacl not loaded (load vendor/tweetnacl.js first)');
    }
    return n;
  }

  // -------------------------------------------------------------------------
  // Byte helpers
  // -------------------------------------------------------------------------

  function isBytes(x) {
    return x instanceof Uint8Array;
  }

  function toBytes(x, what) {
    if (isBytes(x)) return x;
    if (typeof x === 'string') return base64ToBytes(x);
    if (Array.isArray(x)) return Uint8Array.from(x);
    throw new Error('ViorSecure: ' + (what || 'value') + ' must be a Uint8Array or base64 string');
  }

  function concatBytes() {
    var total = 0, i;
    for (i = 0; i < arguments.length; i++) total += arguments[i].length;
    var out = new Uint8Array(total);
    var off = 0;
    for (i = 0; i < arguments.length; i++) {
      out.set(arguments[i], off);
      off += arguments[i].length;
    }
    return out;
  }

  function asciiBytes(s) {
    var out = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) out[i] = s.charCodeAt(i) & 0xff;
    return out;
  }

  // Constant-time byte-array equality (accumulated XOR, single branch at end).
  function ctEqual(a, b) {
    if (a.length !== b.length) return false;
    var v = 0;
    for (var i = 0; i < a.length; i++) v |= a[i] ^ b[i];
    return v === 0;
  }

  function isAllZero(b) {
    var v = 0;
    for (var i = 0; i < b.length; i++) v |= b[i];
    return v === 0;
  }

  function hexToBytes(hex) {
    if (typeof hex !== 'string' || hex.length % 2 !== 0 || /[^0-9a-fA-F]/.test(hex)) {
      throw new Error('ViorSecure: invalid hex');
    }
    var out = new Uint8Array(hex.length / 2);
    for (var i = 0; i < out.length; i++) {
      out[i] = parseInt(hex.substr(i * 2, 2), 16);
    }
    return out;
  }

  function bytesToHex(b) {
    var s = '';
    for (var i = 0; i < b.length; i++) s += (b[i] < 16 ? '0' : '') + b[i].toString(16);
    return s;
  }

  // -------------------------------------------------------------------------
  // Base64 (hand-rolled: atob/btoa mangle binary via UTF-16 strings and are
  // not available in every embedding; this stays dependency-free).
  //
  // The wire (Go encoding/json []byte) uses standard base64 WITH padding.
  // The QR fragment and the frame token use base64url WITHOUT padding
  // (Go base64.RawURLEncoding). Decoding accepts both alphabets' own
  // characters and tolerates missing padding.
  // -------------------------------------------------------------------------

  var B64_STD = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  var B64_URL = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';

  function b64EncodeWith(alphabet, pad, bytes) {
    var out = '';
    var i;
    for (i = 0; i + 2 < bytes.length; i += 3) {
      var n = (bytes[i] << 16) | (bytes[i + 1] << 8) | bytes[i + 2];
      out += alphabet[(n >> 18) & 63] + alphabet[(n >> 12) & 63] + alphabet[(n >> 6) & 63] + alphabet[n & 63];
    }
    var rem = bytes.length - i;
    if (rem === 1) {
      out += alphabet[(bytes[i] >> 2) & 63] + alphabet[(bytes[i] << 4) & 63];
      if (pad) out += '==';
    } else if (rem === 2) {
      var m = (bytes[i] << 8) | bytes[i + 1];
      out += alphabet[(m >> 10) & 63] + alphabet[(m >> 4) & 63] + alphabet[(m << 2) & 63];
      if (pad) out += '=';
    }
    return out;
  }

  function b64DecodeAny(s) {
    if (typeof s !== 'string') throw new Error('ViorSecure: base64 input must be a string');
    // Normalise: url alphabet -> std, strip padding.
    var t = s.replace(/-/g, '+').replace(/_/g, '/').replace(/=+$/, '');
    if (/[^A-Za-z0-9+/]/.test(t)) throw new Error('ViorSecure: invalid base64');
    if (t.length % 4 === 1) throw new Error('ViorSecure: invalid base64 length');
    var outLen = Math.floor((t.length * 3) / 4);
    var out = new Uint8Array(outLen);
    var o = 0, buf = 0, bits = 0;
    for (var i = 0; i < t.length; i++) {
      buf = (buf << 6) | B64_STD.indexOf(t[i]);
      bits += 6;
      if (bits >= 8) {
        bits -= 8;
        out[o++] = (buf >> bits) & 0xff;
      }
    }
    return out;
  }

  function bytesToBase64(bytes) { return b64EncodeWith(B64_STD, true, bytes); }
  function base64ToBytes(s) { return b64DecodeAny(s); }
  function bytesToBase64url(bytes) { return b64EncodeWith(B64_URL, false, bytes); }
  function base64urlToBytes(s) { return b64DecodeAny(s); }

  // -------------------------------------------------------------------------
  // SHA-256 (FIPS 180-4) — one-shot over a Uint8Array.
  //
  // tweetnacl provides no SHA-256, and WebCrypto is unavailable on the
  // insecure origins Vior serves from, so this is implemented here.
  // Correctness is enforced by mobile-cap/tools/secure-channel-test.mjs:
  // RFC 6234 vectors (incl. the million-'a' case), RFC 4231 HMAC vectors,
  // RFC 5869 HKDF vectors, randomised comparison against Node crypto, and
  // the committed cross-language handshake vectors.
  // -------------------------------------------------------------------------

  var SHA256_K = new Uint32Array([
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
  ]);

  function sha256(msg) {
    var len = msg.length;
    // Padded length: message + 0x80 + zeros to 56 mod 64 + 8-byte bit length.
    var padded = new Uint8Array(((len + 8) >> 6 << 6) + 64);
    padded.set(msg);
    padded[len] = 0x80;
    // 64-bit big-endian bit length. JS numbers are exact to 2^53, far beyond
    // any message this client ever hashes.
    var bitLenHi = Math.floor(len / 0x20000000); // len*8 / 2^32
    var bitLenLo = (len << 3) >>> 0;
    var dv = new DataView(padded.buffer);
    dv.setUint32(padded.length - 8, bitLenHi);
    dv.setUint32(padded.length - 4, bitLenLo);

    var h0 = 0x6a09e667, h1 = 0xbb67ae85, h2 = 0x3c6ef372, h3 = 0xa54ff53a;
    var h4 = 0x510e527f, h5 = 0x9b05688c, h6 = 0x1f83d9ab, h7 = 0x5be0cd19;
    var w = new Uint32Array(64);

    for (var off = 0; off < padded.length; off += 64) {
      var i;
      for (i = 0; i < 16; i++) w[i] = dv.getUint32(off + i * 4);
      for (i = 16; i < 64; i++) {
        var x = w[i - 15], y = w[i - 2];
        var s0 = ((x >>> 7) | (x << 25)) ^ ((x >>> 18) | (x << 14)) ^ (x >>> 3);
        var s1 = ((y >>> 17) | (y << 15)) ^ ((y >>> 19) | (y << 13)) ^ (y >>> 10);
        w[i] = (w[i - 16] + s0 + w[i - 7] + s1) | 0;
      }

      var a = h0, b = h1, c = h2, d = h3, e = h4, f = h5, g = h6, h = h7;
      for (i = 0; i < 64; i++) {
        var S1 = ((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7));
        var ch = (e & f) ^ (~e & g);
        var t1 = (h + S1 + ch + SHA256_K[i] + w[i]) | 0;
        var S0 = ((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10));
        var maj = (a & b) ^ (a & c) ^ (b & c);
        var t2 = (S0 + maj) | 0;
        h = g; g = f; f = e; e = (d + t1) | 0;
        d = c; c = b; b = a; a = (t1 + t2) | 0;
      }
      h0 = (h0 + a) | 0; h1 = (h1 + b) | 0; h2 = (h2 + c) | 0; h3 = (h3 + d) | 0;
      h4 = (h4 + e) | 0; h5 = (h5 + f) | 0; h6 = (h6 + g) | 0; h7 = (h7 + h) | 0;
    }

    var out = new Uint8Array(32);
    var odv = new DataView(out.buffer);
    odv.setUint32(0, h0 >>> 0); odv.setUint32(4, h1 >>> 0);
    odv.setUint32(8, h2 >>> 0); odv.setUint32(12, h3 >>> 0);
    odv.setUint32(16, h4 >>> 0); odv.setUint32(20, h5 >>> 0);
    odv.setUint32(24, h6 >>> 0); odv.setUint32(28, h7 >>> 0);
    return out;
  }

  // HMAC-SHA256 (RFC 2104 / FIPS 198-1).
  function hmacSha256(key, msg) {
    var BLOCK = 64;
    var k = key.length > BLOCK ? sha256(key) : key;
    var ipad = new Uint8Array(BLOCK);
    var opad = new Uint8Array(BLOCK);
    for (var i = 0; i < BLOCK; i++) {
      var kb = i < k.length ? k[i] : 0;
      ipad[i] = kb ^ 0x36;
      opad[i] = kb ^ 0x5c;
    }
    return sha256(concatBytes(opad, sha256(concatBytes(ipad, msg))));
  }

  // HKDF-SHA256 (RFC 5869). A null/empty salt means a HashLen of zero bytes,
  // matching Go's x/crypto/hkdf with a nil salt.
  function hkdfExtract(salt, ikm) {
    if (!salt || salt.length === 0) salt = new Uint8Array(32);
    return hmacSha256(salt, ikm);
  }

  function hkdfExpand(prk, info, length) {
    if (length > 255 * 32) throw new Error('ViorSecure: HKDF length too large');
    var out = new Uint8Array(length);
    var t = new Uint8Array(0);
    var counter = 1;
    for (var off = 0; off < length; off += 32) {
      t = hmacSha256(prk, concatBytes(t, info, Uint8Array.of(counter++)));
      out.set(t.subarray(0, Math.min(32, length - off)), off);
    }
    return out;
  }

  function hkdf(ikm, salt, info, length) {
    return hkdfExpand(hkdfExtract(salt, ikm), info, length);
  }

  // -------------------------------------------------------------------------
  // Handshake (initiator) — mirrors internal/handshake/handshake.go.
  //
  //   T   = "vior-hs v1" || version || epk_i || epk_r || n_i || n_r
  //   ss  = X25519(esk_i, epk_r)
  //   PRK = HKDF-Extract(SHA-256, salt = T, ikm = ss || S)
  //   k_session   = HKDF-Expand(PRK, "vior-hs v1 session",   32)
  //   k_confirm_i = HKDF-Expand(PRK, "vior-hs v1 confirm-i", 32)
  //   k_confirm_r = HKDF-Expand(PRK, "vior-hs v1 confirm-r", 32)
  //
  // The confirmation MAC each side sends IS its expanded confirm key (the
  // transcript is already bound in via the HKDF salt); comparison is
  // constant-time. The responder confirms first: finish() verifies the
  // responder's MAC before this side's own MAC is ever produced.
  // -------------------------------------------------------------------------

  var VERSION = 1;
  var SECRET_MIN = 32;
  var NONCE_SIZE = 16;
  var PUBKEY_SIZE = 32;
  var MAC_SIZE = 32;
  var KEY_SIZE = 32;

  var TRANSCRIPT_LABEL = asciiBytes('vior-hs v1');
  var INFO_SESSION = asciiBytes('vior-hs v1 session');
  var INFO_CONFIRM_I = asciiBytes('vior-hs v1 confirm-i');
  var INFO_CONFIRM_R = asciiBytes('vior-hs v1 confirm-r');
  var INFO_FRAME_TOKEN = asciiBytes('vior-hs v1 frame-token');

  function transcript(epkI, epkR, nonceI, nonceR) {
    return concatBytes(TRANSCRIPT_LABEL, Uint8Array.of(VERSION), epkI, epkR, nonceI, nonceR);
  }

  function deriveSchedule(sharedSecret, bootstrapSecret, t) {
    var prk = hkdfExtract(t, concatBytes(sharedSecret, bootstrapSecret));
    return {
      session: hkdfExpand(prk, INFO_SESSION, KEY_SIZE),
      confirmI: hkdfExpand(prk, INFO_CONFIRM_I, MAC_SIZE),
      confirmR: hkdfExpand(prk, INFO_CONFIRM_R, MAC_SIZE)
    };
  }

  // createInitiator(secretBytes[, testOverrides]) drives the client side.
  //
  //   var hs = ViorSecure.createInitiator(secret);
  //   ws.send({type: 'secure-init', data: hs.initMessage()});
  //   // on secure-resp:
  //   var done = hs.finish(respData);       // throws on auth failure
  //   ws.send({type: 'secure-confirm', data: done.message});
  //   var ch = ViorSecure.channel(done.sessionKey, true);
  //
  // initMessage()/finish() return JSON-ready objects whose byte fields are
  // standard base64 strings with padding, exactly what the Go server's
  // encoding/json []byte fields emit and expect. finish() accepts the parsed
  // secure-resp data with base64-string or Uint8Array fields.
  //
  // testOverrides {privateKey, nonce} exists ONLY for the deterministic
  // vector suite (the JS twin of Go's newInitiatorWith); production callers
  // must not pass it.
  function createInitiator(secretBytes, testOverrides) {
    var nacl = getNacl();
    var secret = toBytes(secretBytes, 'secret');
    if (secret.length < SECRET_MIN) {
      throw new Error('ViorSecure: bootstrap secret must be at least 32 bytes');
    }
    var priv = testOverrides && testOverrides.privateKey
      ? toBytes(testOverrides.privateKey, 'privateKey')
      : nacl.randomBytes(32);
    var nonce = testOverrides && testOverrides.nonce
      ? toBytes(testOverrides.nonce, 'nonce')
      : nacl.randomBytes(NONCE_SIZE);
    if (priv.length !== 32 || nonce.length !== NONCE_SIZE) {
      throw new Error('ViorSecure: malformed override');
    }
    var pub = nacl.scalarMult.base(priv);

    // 'new' -> 'await-response' -> 'done' | 'failed'; every failure is
    // terminal, mirroring the Go state machine.
    var state = 'new';
    var sessionKey = null;

    function fail(msg) {
      state = 'failed';
      sessionKey = null;
      return new Error(msg);
    }

    return {
      // The secure-init payload. Must be called exactly once, first.
      initMessage: function () {
        if (state !== 'new') throw fail('ViorSecure: initMessage called out of order');
        state = 'await-response';
        return { v: VERSION, epk: bytesToBase64(pub), n: bytesToBase64(nonce) };
      },

      // Consumes the secure-resp payload; verifies the responder knew the
      // secret BEFORE producing our own MAC. Returns
      //   {message: {mac}, sessionKey: Uint8Array(32)}.
      // A throw is terminal: close the connection and re-scan; do not retry.
      finish: function (resp) {
        if (state !== 'await-response') throw fail('ViorSecure: finish called out of order');
        if (!resp) throw fail('ViorSecure: malformed response');
        var epkR, nonceR, macR;
        try {
          epkR = toBytes(resp.epk, 'epk');
          nonceR = toBytes(resp.n, 'n');
          macR = toBytes(resp.mac, 'mac');
        } catch (e) {
          throw fail('ViorSecure: malformed response');
        }
        if (epkR.length !== PUBKEY_SIZE || nonceR.length !== NONCE_SIZE || macR.length !== MAC_SIZE) {
          throw fail('ViorSecure: malformed response');
        }

        var ss = getNacl().scalarMult(priv, epkR);
        // crypto/ecdh rejects an all-zero shared secret (a low-order peer
        // key); mirror that so both ends fail identically.
        if (isAllZero(ss)) throw fail('ViorSecure: malformed response');

        var t = transcript(pub, epkR, nonce, nonceR);
        var ks = deriveSchedule(ss, secret, t);
        if (!ctEqual(macR, ks.confirmR)) {
          throw fail('ViorSecure: peer failed to prove knowledge of the shared secret');
        }

        sessionKey = ks.session;
        state = 'done';
        return {
          message: { mac: bytesToBase64(ks.confirmI) },
          sessionKey: new Uint8Array(sessionKey)
        };
      },

      // The derived key, once the handshake has completed.
      sessionKey: function () {
        if (state !== 'done') throw new Error('ViorSecure: handshake not complete');
        return new Uint8Array(sessionKey);
      }
    };
  }

  // -------------------------------------------------------------------------
  // Record layer — mirrors internal/securechan/securechan.go.
  //
  // Direction keys: HKDF-SHA256(sessionKey, salt=nil, info) with
  //   "vior-securechan v1 i2r"  (initiator -> responder)
  //   "vior-securechan v1 r2i"  (responder -> initiator)
  //
  // Frame layout:
  //   counter(8, big-endian uint64) || secretbox(plaintext,
  //       nonce = counter(8 BE) || 16 zero bytes, directionKey)
  //
  // Overhead = 8 (counter) + 16 (Poly1305 tag) = 24 bytes. The receiver
  // rejects any counter not strictly greater than the highest accepted
  // (replay/reorder protection over the ordered WebSocket stream).
  // -------------------------------------------------------------------------

  var INFO_I2R = asciiBytes('vior-securechan v1 i2r');
  var INFO_R2I = asciiBytes('vior-securechan v1 r2i');
  var COUNTER_PREFIX = 8;
  var SECRETBOX_OVERHEAD = 16;
  var OVERHEAD = COUNTER_PREFIX + SECRETBOX_OVERHEAD;
  var NONCE_BOX = 24;
  var MAX_UINT64 = 0xffffffffffffffffn;

  function deriveDirectionKeys(sessionKey) {
    var key = toBytes(sessionKey, 'sessionKey');
    if (key.length !== KEY_SIZE) throw new Error('ViorSecure: shared key must be 32 bytes');
    return {
      i2r: hkdf(key, null, INFO_I2R, KEY_SIZE),
      r2i: hkdf(key, null, INFO_R2I, KEY_SIZE)
    };
  }

  function counterNonce(counter) {
    var nonce = new Uint8Array(NONCE_BOX);
    new DataView(nonce.buffer).setBigUint64(0, counter);
    return nonce;
  }

  // channel(sessionKey, isInitiator) -> {seal, open}. The Vior client is the
  // handshake initiator, so it passes isInitiator = true (the Go server
  // constructs its mirror with NewChannel(key, false)).
  function channel(sessionKey, isInitiator) {
    var keys = deriveDirectionKeys(sessionKey);
    var sendKey = isInitiator ? keys.i2r : keys.r2i;
    var recvKey = isInitiator ? keys.r2i : keys.i2r;

    var sendCounter = 0n;
    var recvHighest = 0n;
    var recvSeen = false;

    return {
      seal: function (plaintext) {
        if (!isBytes(plaintext)) throw new Error('ViorSecure: plaintext must be a Uint8Array');
        if (sendCounter === MAX_UINT64) throw new Error('ViorSecure: nonce space exhausted');
        var nonce = counterNonce(sendCounter);
        var box = getNacl().secretbox(plaintext, nonce, sendKey);
        var out = new Uint8Array(COUNTER_PREFIX + box.length);
        out.set(nonce.subarray(0, COUNTER_PREFIX), 0);
        out.set(box, COUNTER_PREFIX);
        sendCounter++;
        return out;
      },

      open: function (frame) {
        if (!isBytes(frame)) throw new Error('ViorSecure: frame must be a Uint8Array');
        if (frame.length < OVERHEAD) throw new Error('ViorSecure: frame too short');
        var counter = new DataView(frame.buffer, frame.byteOffset, frame.byteLength).getBigUint64(0);
        if (recvSeen && counter <= recvHighest) {
          throw new Error('ViorSecure: replayed or out-of-order frame');
        }
        var plaintext = getNacl().secretbox.open(frame.subarray(COUNTER_PREFIX), counterNonce(counter), recvKey);
        if (!plaintext) throw new Error('ViorSecure: authentication failed');
        recvHighest = counter;
        recvSeen = true;
        return plaintext;
      }
    };
  }

  // -------------------------------------------------------------------------
  // Frame token — mirrors handshake.FrameToken: HKDF-SHA256(sessionKey,
  // salt=nil, "vior-hs v1 frame-token", 32), base64url without padding.
  // Authorises GET /stream and /snapshot; arrives from the server inside
  // secure-ready, but the client can also derive it locally.
  // -------------------------------------------------------------------------

  function frameToken(sessionKey) {
    var key = toBytes(sessionKey, 'sessionKey');
    if (key.length !== KEY_SIZE) throw new Error('ViorSecure: session key must be 32 bytes');
    return bytesToBase64url(hkdf(key, null, INFO_FRAME_TOKEN, 32));
  }

  // -------------------------------------------------------------------------
  // QR fragment — the desktop appends "#k=<base64url secret>" to the QR URL
  // (see stream.ChannelSecretParam). Accepts a full URL, a bare fragment
  // with or without the leading '#', or a multi-param fragment
  // ("a=1&k=...&b=2"). Returns Uint8Array or null when no usable k= exists.
  // -------------------------------------------------------------------------

  function secretFromFragment(input) {
    if (typeof input !== 'string') return null;
    var frag = input;
    var hash = input.indexOf('#');
    if (hash !== -1) frag = input.slice(hash + 1);
    var parts = frag.split('&');
    for (var i = 0; i < parts.length; i++) {
      if (parts[i].slice(0, 2) === 'k=') {
        try {
          var secret = base64urlToBytes(parts[i].slice(2));
          return secret.length >= SECRET_MIN ? secret : null;
        } catch (e) {
          return null;
        }
      }
    }
    return null;
  }

  // -------------------------------------------------------------------------
  // Public surface
  // -------------------------------------------------------------------------

  root.ViorSecure = {
    VERSION: VERSION,
    SECRET_MIN: SECRET_MIN,
    OVERHEAD: OVERHEAD,

    createInitiator: createInitiator,
    channel: channel,
    frameToken: frameToken,
    secretFromFragment: secretFromFragment,

    bytesToBase64: bytesToBase64,
    base64ToBytes: base64ToBytes,
    bytesToBase64url: bytesToBase64url,
    base64urlToBytes: base64urlToBytes,
    hexToBytes: hexToBytes,
    bytesToHex: bytesToHex,

    // Exposed for the cross-language test suite
    // (mobile-cap/tools/secure-channel-test.mjs); not part of the stable API.
    hash: {
      sha256: sha256,
      hmacSha256: hmacSha256,
      hkdfExtract: hkdfExtract,
      hkdfExpand: hkdfExpand,
      hkdf: hkdf
    },
    deriveDirectionKeys: deriveDirectionKeys
  };
})(typeof self !== 'undefined' ? self : globalThis);
