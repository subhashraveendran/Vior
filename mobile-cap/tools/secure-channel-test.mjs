// secure-channel-test.mjs — cross-language validation of the JS secure
// channel (clients/secure/vior-secure.js + clients/secure/vendor/tweetnacl.js)
// against the Go implementation's committed vectors and public RFC vectors.
//
//   node mobile-cap/tools/secure-channel-test.mjs
//
// What it proves, in order:
//   1. The vendored tweetnacl is genuine: RFC 7748 X25519 vectors and the
//      classic NaCl secretbox vector.
//   2. The hand-rolled SHA-256 / HMAC-SHA256 / HKDF-SHA256 match RFC 6234,
//      RFC 4231 and RFC 5869, plus randomised agreement with node:crypto.
//   3. The handshake initiator reproduces every case in
//      internal/handshake/testdata/handshake_vectors.json exactly
//      (public keys, shared secret, transcript, session key, both MACs).
//   4. The record layer derives the vector direction keys and round-trips
//      frames byte-compatibly (layout, counters, replay/reorder/tamper/short
//      rejection), and frameToken matches an independent node:crypto HKDF.
//
// The vectors file is the wire-format contract; a failure here means Go and
// JS have diverged and shipped clients would break.

import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..', '..');

// --- load the browser scripts exactly as a <script> tag would ---------------
globalThis.self = globalThis; // the UMD/global attach point in a browser
for (const rel of ['clients/secure/vendor/tweetnacl.js', 'clients/secure/vior-secure.js']) {
  const file = path.join(ROOT, rel);
  vm.runInThisContext(fs.readFileSync(file, 'utf8'), { filename: file });
}
const nacl = globalThis.nacl;
const ViorSecure = globalThis.ViorSecure;

// --- tiny test harness ------------------------------------------------------
let passed = 0;
let failed = 0;
function check(name, ok, detail) {
  if (ok) {
    passed++;
  } else {
    failed++;
    console.error(`FAIL  ${name}${detail ? ` — ${detail}` : ''}`);
  }
}
function eq(name, got, want) {
  const g = got instanceof Uint8Array ? hex(got) : got;
  const w = want instanceof Uint8Array ? hex(want) : want;
  check(name, g === w, g === w ? '' : `got ${g}, want ${w}`);
}
function throws(name, fn, substr) {
  try {
    fn();
    check(name, false, 'expected a throw');
  } catch (e) {
    check(name, !substr || String(e.message).includes(substr),
      `threw "${e.message}", want containing "${substr}"`);
  }
}
const hex = (b) => Buffer.from(b).toString('hex');
const unhex = (s) => new Uint8Array(Buffer.from(s, 'hex'));
const ascii = (s) => new Uint8Array(Buffer.from(s, 'latin1'));

// ============================================================================
// 1. Vendored tweetnacl authenticity
// ============================================================================

// RFC 7748 §6.1 X25519 Diffie-Hellman vectors.
{
  const a = unhex('77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a');
  const b = unhex('5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb');
  const KA = nacl.scalarMult.base(a);
  const KB = nacl.scalarMult.base(b);
  eq('rfc7748 K_A', KA, unhex('8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a'));
  eq('rfc7748 K_B', KB, unhex('de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f'));
  eq('rfc7748 shared a*K_B', nacl.scalarMult(a, KB),
    unhex('4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742'));
  eq('rfc7748 shared b*K_A', nacl.scalarMult(b, KA),
    unhex('4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742'));
}

// Classic NaCl crypto_secretbox vector (Bernstein's tests/secretbox.c).
{
  const key = unhex('1b27556473e985d462cd51197a9a46c76009549eac6474f206c4ee0844f68389');
  const nonce = unhex('69696ee955b62b73cd62bda875fc73d68219e0036b7a0b37');
  const msg = unhex(
    'be075fc53c81f2d5cf141316ebeb0c7b5228c52a4c62cbd44b66849b64244ffc' +
    'e5ecbaaf33bd751a1ac728d45e6c61296cdc3c01233561f41db66cce314adb31' +
    '0e3be8250c46f06dceea3a7fa1348057e2f6556ad6b1318a024a838f21af1fde' +
    '048977eb48f59ffd4924ca1c60902e52f0a089bc76897040e082f93776384864' +
    '5e0705');
  const want = unhex(
    'f3ffc7703f9400e52a7dfb4b3d3305d98e993b9f48681273c29650ba32fc76ce' +
    '48332ea7164d96a4476fb8c531a1186ac0dfc17c98dce87b4da7f011ec48c972' +
    '71d2c20f9b928fe2270d6fb863d51738b48eeee314a7cc8ab932164548e526ae' +
    '90224368517acfeabd6bb3732bc0e9da99832b61ca01b6de56244a9e88d5f9b3' +
    '7973f622a43d14a6599b1f654cb45a74e355a5');
  eq('nacl secretbox vector', nacl.secretbox(msg, nonce, key), want);
  eq('nacl secretbox.open vector', nacl.secretbox.open(want, nonce, key), msg);
}

// ============================================================================
// 2. SHA-256 / HMAC / HKDF primitives
// ============================================================================

const H = ViorSecure.hash;

// RFC 6234 / FIPS 180-4 SHA-256.
eq('sha256 empty', H.sha256(new Uint8Array(0)),
  unhex('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'));
eq('sha256 abc', H.sha256(ascii('abc')),
  unhex('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'));
eq('sha256 448-bit', H.sha256(ascii('abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq')),
  unhex('248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1'));
eq('sha256 896-bit', H.sha256(ascii('abcdefghbcdefghicdefghijdefghijkefghijklfghijklmghijklmnhijklmnoijklmnopjklmnopqklmnopqrlmnopqrsmnopqrstnopqrstu')),
  unhex('cf5b16a778af8380036ce59e7b0492370b249b11e8f07a51afac45037afee9d1'));
eq('sha256 million a', H.sha256(new Uint8Array(1000000).fill(0x61)),
  unhex('cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0'));
// Exact block-boundary lengths (55/56/63/64/65) against node:crypto.
for (const n of [55, 56, 63, 64, 65, 119, 120, 127, 128]) {
  const m = new Uint8Array(n).fill(0x42);
  eq(`sha256 len ${n}`, H.sha256(m), new Uint8Array(crypto.createHash('sha256').update(m).digest()));
}

// RFC 4231 HMAC-SHA-256, test cases 1–7.
{
  const cases = [
    ['0b'.repeat(20), ascii('Hi There'),
      'b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7', 32],
    [Buffer.from('Jefe').toString('hex'), ascii('what do ya want for nothing?'),
      '5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843', 32],
    ['aa'.repeat(20), unhex('dd'.repeat(50)),
      '773ea91e36800e46854db8ebd09181a72959098b3ef8c122d9635514ced565fe', 32],
    ['0102030405060708090a0b0c0d0e0f10111213141516171819', unhex('cd'.repeat(50)),
      '82558a389a443c0ea4cc819899f2083a85f0faa3e578f8077a2e3ff46729665b', 32],
    ['0c'.repeat(20), ascii('Test With Truncation'),
      'a3b6167473100ee06e0c796c2955552b', 16],
    ['aa'.repeat(131), ascii('Test Using Larger Than Block-Size Key - Hash Key First'),
      '60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54', 32],
    ['aa'.repeat(131), ascii('This is a test using a larger than block-size key and a larger than block-size data. The key needs to be hashed before being used by the HMAC algorithm.'),
      '9b09ffa71b942fcb27635fbcd5b0e944bfdc63644f0713938a7f51535c3a35e2', 32],
  ];
  cases.forEach(([keyHex, data, want, truncate], i) => {
    const mac = H.hmacSha256(unhex(keyHex), data).subarray(0, truncate);
    eq(`rfc4231 tc${i + 1}`, mac, unhex(want));
  });
}

// RFC 5869 HKDF-SHA-256, test cases 1–3.
{
  const t1prk = H.hkdfExtract(unhex('000102030405060708090a0b0c'), unhex('0b'.repeat(22)));
  eq('rfc5869 tc1 PRK', t1prk,
    unhex('077709362c2e32df0ddc3f0dc47bba6390b6c73bb50f9c3122ec844ad7c2b3e5'));
  eq('rfc5869 tc1 OKM', H.hkdfExpand(t1prk, unhex('f0f1f2f3f4f5f6f7f8f9'), 42),
    unhex('3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865'));

  const longIkm = unhex(Array.from({ length: 80 }, (_, i) => i.toString(16).padStart(2, '0')).join(''));
  const longSalt = unhex(Array.from({ length: 80 }, (_, i) => (i + 0x60).toString(16).padStart(2, '0')).join(''));
  const longInfo = unhex(Array.from({ length: 80 }, (_, i) => (i + 0xb0).toString(16).padStart(2, '0')).join(''));
  eq('rfc5869 tc2 OKM', H.hkdf(longIkm, longSalt, longInfo, 82),
    unhex('b11e398dc80327a1c8e7f78c596a49344f012eda2d4efad8a050cc4c19afa97c' +
      '59045a99cac7827271cb41c65e590e09da3275600c2f09b8367793a9aca3db71' +
      'cc30c58179ec3e87c14c01d5c1f3434f1d87'));

  eq('rfc5869 tc3 OKM (no salt, no info)',
    H.hkdf(unhex('0b'.repeat(22)), null, new Uint8Array(0), 42),
    unhex('8da4e775a563c18f715f802a063c5a31b8a11f5c5ee1879ec3454e5f3c738d2d9d201395faa4b61a96c8'));
}

// Randomised agreement with node:crypto (100 rounds each).
for (let i = 0; i < 100; i++) {
  const msg = crypto.randomBytes(1 + (i * 13) % 300);
  const key = crypto.randomBytes(1 + (i * 7) % 100);
  eq(`random sha256 #${i}`, H.sha256(new Uint8Array(msg)),
    new Uint8Array(crypto.createHash('sha256').update(msg).digest()));
  eq(`random hmac #${i}`, H.hmacSha256(new Uint8Array(key), new Uint8Array(msg)),
    new Uint8Array(crypto.createHmac('sha256', key).update(msg).digest()));
  const salt = i % 3 === 0 ? new Uint8Array(0) : new Uint8Array(crypto.randomBytes(16));
  const info = new Uint8Array(crypto.randomBytes(i % 40));
  const len = 1 + (i * 11) % 96;
  eq(`random hkdf #${i}`, H.hkdf(new Uint8Array(msg), salt, info, len),
    new Uint8Array(Buffer.from(crypto.hkdfSync('sha256', msg, salt, info, len))));
}

// ============================================================================
// 3. Handshake vectors (internal/handshake/testdata/handshake_vectors.json)
// ============================================================================

const vecFile = path.join(ROOT, 'internal', 'handshake', 'testdata', 'handshake_vectors.json');
const vectors = JSON.parse(fs.readFileSync(vecFile, 'utf8'));
check('vector file version 1', vectors.version === 1);
check('vector file has cases', vectors.vectors.length >= 4);

for (const v of vectors.vectors) {
  const name = `vector "${v.name}"`;
  const secret = unhex(v.secret);
  const iSeed = unhex(v.initiatorPrivSeed);
  const rSeed = unhex(v.responderPrivSeed);
  const iNonce = unhex(v.initiatorNonce);
  const rNonce = unhex(v.responderNonce);

  // X25519 sanity before blaming the key schedule (mirrors vectors_test.go).
  eq(`${name}: initiator pub`, nacl.scalarMult.base(iSeed), unhex(v.initiatorPubKey));
  eq(`${name}: responder pub`, nacl.scalarMult.base(rSeed), unhex(v.responderPubKey));
  eq(`${name}: shared secret`, nacl.scalarMult(iSeed, unhex(v.responderPubKey)), unhex(v.sharedSecret));

  // Transcript: "vior-hs v1" || 0x01 || epk_i || epk_r || n_i || n_r.
  const wantT = Buffer.concat([
    Buffer.from('vior-hs v1'), Buffer.from([1]),
    Buffer.from(v.initiatorPubKey, 'hex'), Buffer.from(v.responderPubKey, 'hex'),
    Buffer.from(v.initiatorNonce, 'hex'), Buffer.from(v.responderNonce, 'hex'),
  ]).toString('hex');
  check(`${name}: transcript matches committed`, wantT === v.transcript);

  // Full initiator run with deterministic overrides.
  const hs = ViorSecure.createInitiator(secret, { privateKey: iSeed, nonce: iNonce });
  const initMsg = hs.initMessage();
  check(`${name}: init v`, initMsg.v === 1);
  eq(`${name}: init epk wire b64`, initMsg.epk, Buffer.from(v.initiatorPubKey, 'hex').toString('base64'));
  eq(`${name}: init n wire b64`, initMsg.n, Buffer.from(v.initiatorNonce, 'hex').toString('base64'));

  // secure-resp exactly as the Go server would emit it (std base64, padded).
  const resp = {
    epk: Buffer.from(v.responderPubKey, 'hex').toString('base64'),
    n: Buffer.from(v.responderNonce, 'hex').toString('base64'),
    mac: Buffer.from(v.responderMac, 'hex').toString('base64'),
  };
  const done = hs.finish(resp);
  eq(`${name}: session key`, done.sessionKey, unhex(v.sessionKey));
  eq(`${name}: sessionKey() accessor`, hs.sessionKey(), unhex(v.sessionKey));
  eq(`${name}: confirm mac`, new Uint8Array(Buffer.from(done.message.mac, 'base64')), unhex(v.initiatorMac));

  // Record-layer direction keys pinned by the vectors.
  const dk = ViorSecure.deriveDirectionKeys(done.sessionKey);
  eq(`${name}: send key i2r`, dk.i2r, unhex(v.sendKeyI2R));
  eq(`${name}: send key r2i`, dk.r2i, unhex(v.sendKeyR2I));

  // A responder that does NOT know the secret must be rejected, before any
  // initiator MAC exists.
  const hsBad = ViorSecure.createInitiator(secret, { privateKey: iSeed, nonce: iNonce });
  hsBad.initMessage();
  const badMac = unhex(v.responderMac);
  badMac[0] ^= 0x01;
  throws(`${name}: wrong responder MAC rejected`, () => hsBad.finish({
    epk: unhex(v.responderPubKey), n: unhex(v.responderNonce), mac: badMac,
  }), 'prove knowledge');
  throws(`${name}: sessionKey unavailable after failure`, () => hsBad.sessionKey(), 'not complete');
}

// Handshake input validation.
throws('short secret rejected', () => ViorSecure.createInitiator(new Uint8Array(31)), 'at least 32');
{
  const secret = new Uint8Array(32);
  const hs = ViorSecure.createInitiator(secret);
  throws('finish before init rejected', () => hs.finish({}), 'out of order');
  const hs2 = ViorSecure.createInitiator(secret);
  hs2.initMessage();
  throws('double initMessage rejected', () => { const h = ViorSecure.createInitiator(secret); h.initMessage(); h.initMessage(); }, 'out of order');
  throws('short resp nonce rejected', () => hs2.finish({ epk: new Uint8Array(32), n: new Uint8Array(15), mac: new Uint8Array(32) }), 'malformed');
  const hs3 = ViorSecure.createInitiator(secret);
  hs3.initMessage();
  // An all-zero peer public key is low-order: X25519 output is all zero and
  // Go's crypto/ecdh errors. The JS side must fail the same way.
  throws('low-order peer key rejected', () => hs3.finish({ epk: new Uint8Array(32), n: new Uint8Array(16), mac: new Uint8Array(32) }), 'malformed');
}

// ============================================================================
// 4. Record layer framing (against vector data) + frame token
// ============================================================================

for (const v of vectors.vectors) {
  const name = `record "${v.name}"`;
  const key = unhex(v.sessionKey);
  const client = ViorSecure.channel(key, true);   // the JS client: initiator
  const server = ViorSecure.channel(key, false);  // Go's NewChannel(key, false) twin

  // Frame layout: counter(8 BE) || secretbox. Sealing with the vector's
  // pinned i2r key and counter-0 nonce must be reproducible from primitives.
  const plain = ascii('hello from the initiator \x00\x01\x02');
  const frame = client.seal(plain);
  check(`${name}: overhead is 24`, frame.length === plain.length + 24,
    `len ${frame.length}`);
  eq(`${name}: frame 0 counter prefix`, frame.subarray(0, 8), unhex('0000000000000000'));
  {
    // Independent reconstruction: secretbox under sendKeyI2R with nonce
    // counter||0^16 must equal the sealed body byte-for-byte.
    const nonce = new Uint8Array(24);
    const body = nacl.secretbox(plain, nonce, unhex(v.sendKeyI2R));
    eq(`${name}: frame 0 body vs pinned i2r key`, frame.subarray(8), body);
  }
  eq(`${name}: server opens client frame`, server.open(frame), plain);

  // Reverse direction.
  const reply = ascii('reply from the responder');
  const rframe = server.seal(reply);
  {
    const nonce = new Uint8Array(24);
    const body = nacl.secretbox(reply, nonce, unhex(v.sendKeyR2I));
    eq(`${name}: r2i frame 0 body vs pinned r2i key`, rframe.subarray(8), body);
  }
  eq(`${name}: client opens server frame`, client.open(rframe), reply);

  // Counters increment per frame and are big-endian.
  const f1 = client.seal(plain);
  const f2 = client.seal(plain);
  eq(`${name}: counter 1`, f1.subarray(0, 8), unhex('0000000000000001'));
  eq(`${name}: counter 2`, f2.subarray(0, 8), unhex('0000000000000002'));

  // Replay / reorder / tamper / short-frame rejection.
  eq(`${name}: in-order open`, server.open(f1), plain);
  throws(`${name}: replay rejected`, () => server.open(f1), 'replayed');
  eq(`${name}: skip-ahead ok`, server.open(f2), plain); // strictly greater is fine
  throws(`${name}: reordered (lower counter) rejected`, () => server.open(f1), 'replayed');
  const tampered = new Uint8Array(client.seal(plain));
  tampered[tampered.length - 1] ^= 0x80;
  throws(`${name}: tampered frame rejected`, () => server.open(tampered), 'authentication failed');
  throws(`${name}: short frame rejected`, () => server.open(new Uint8Array(23)), 'too short');
  throws(`${name}: wrong-direction frame rejected`, () => {
    const c2 = ViorSecure.channel(key, true);
    c2.open(client.seal(plain)); // initiator opening its own direction
  }, 'authentication failed');

  // Empty plaintext round trip (a Go Seal(nil) counterpart).
  const empty = ViorSecure.channel(key, true).seal(new Uint8Array(0));
  check(`${name}: empty plaintext frame is exactly overhead`, empty.length === 24);
  eq(`${name}: empty plaintext round trip`,
    ViorSecure.channel(key, false).open(empty), new Uint8Array(0));

  // Frame token: HKDF(sessionKey, salt=nil, "vior-hs v1 frame-token", 32),
  // base64url no padding — verified against an independent node:crypto HKDF.
  const wantTok = Buffer.from(
    crypto.hkdfSync('sha256', key, new Uint8Array(0), Buffer.from('vior-hs v1 frame-token'), 32)
  ).toString('base64url');
  eq(`${name}: frameToken`, ViorSecure.frameToken(key), wantTok);
  check(`${name}: frameToken unpadded`, !ViorSecure.frameToken(key).includes('='));
}

throws('channel rejects short key', () => ViorSecure.channel(new Uint8Array(31), true), '32 bytes');

// Large frame (a video-sized payload) survives the round trip.
{
  const key = unhex(vectors.vectors[0].sessionKey);
  const a = ViorSecure.channel(key, true);
  const b = ViorSecure.channel(key, false);
  const big = new Uint8Array(crypto.randomBytes(256 * 1024));
  eq('256 KiB round trip', b.open(a.seal(big)), big);
}

// ============================================================================
// 5. Helpers: base64 + QR fragment
// ============================================================================

{
  const b = unhex('00fa01fb02fc03fd04');
  eq('std b64 encode pads', ViorSecure.bytesToBase64(unhex('01')), 'AQ==');
  eq('std b64 round trip', ViorSecure.base64ToBytes(ViorSecure.bytesToBase64(b)), b);
  eq('b64url encode no pad', ViorSecure.bytesToBase64url(unhex('01')), 'AQ');
  eq('b64url uses url alphabet', ViorSecure.bytesToBase64url(unhex('fbff')), Buffer.from(unhex('fbff')).toString('base64url'));
  eq('b64url round trip', ViorSecure.base64urlToBytes(ViorSecure.bytesToBase64url(b)), b);
  // Randomised agreement with Buffer for both alphabets.
  for (let i = 0; i < 50; i++) {
    const r = crypto.randomBytes(i);
    eq(`b64 vs Buffer #${i}`, ViorSecure.bytesToBase64(new Uint8Array(r)), r.toString('base64'));
    eq(`b64url vs Buffer #${i}`, ViorSecure.bytesToBase64url(new Uint8Array(r)), r.toString('base64url'));
    eq(`b64 decode vs Buffer #${i}`, ViorSecure.base64ToBytes(r.toString('base64')), new Uint8Array(r));
  }
  throws('b64 rejects junk', () => ViorSecure.base64ToBytes('!$%^'), 'invalid base64');
}

{
  const secret = new Uint8Array(crypto.randomBytes(32));
  const param = Buffer.from(secret).toString('base64url'); // Go base64.RawURLEncoding
  eq('fragment: full URL', ViorSecure.secretFromFragment(`http://192.168.1.20:8000/?x=1#k=${param}`), secret);
  eq('fragment: bare with hash', ViorSecure.secretFromFragment(`#k=${param}`), secret);
  eq('fragment: bare without hash', ViorSecure.secretFromFragment(`k=${param}`), secret);
  eq('fragment: multi-param', ViorSecure.secretFromFragment(`#a=1&k=${param}&b=2`), secret);
  check('fragment: missing k', ViorSecure.secretFromFragment('http://h/#x=1') === null);
  check('fragment: short secret rejected', ViorSecure.secretFromFragment(`#k=${Buffer.alloc(16).toString('base64url')}`) === null);
  check('fragment: invalid b64 rejected', ViorSecure.secretFromFragment('#k=!!!') === null);
  check('fragment: non-string', ViorSecure.secretFromFragment(undefined) === null);
}

// Randomised self-handshake: JS initiator against a JS "responder" built from
// the same primitives, fresh random keys each run (exercises the
// non-deterministic path incl. nacl.randomBytes).
{
  const secret = new Uint8Array(crypto.randomBytes(32));
  const hs = ViorSecure.createInitiator(secret);
  const init = hs.initMessage();
  // Minimal responder from primitives (the Go server's role).
  const rPriv = new Uint8Array(crypto.randomBytes(32));
  const rPub = nacl.scalarMult.base(rPriv);
  const rNonce = new Uint8Array(crypto.randomBytes(16));
  const epkI = ViorSecure.base64ToBytes(init.epk);
  const nI = ViorSecure.base64ToBytes(init.n);
  const ss = nacl.scalarMult(rPriv, epkI);
  const t = new Uint8Array(Buffer.concat([
    Buffer.from('vior-hs v1'), Buffer.from([1]), epkI, rPub, nI, rNonce]));
  const prk = H.hkdfExtract(t, new Uint8Array(Buffer.concat([ss, secret])));
  const kSession = H.hkdfExpand(prk, ascii('vior-hs v1 session'), 32);
  const kConfI = H.hkdfExpand(prk, ascii('vior-hs v1 confirm-i'), 32);
  const kConfR = H.hkdfExpand(prk, ascii('vior-hs v1 confirm-r'), 32);
  const done = hs.finish({ epk: rPub, n: rNonce, mac: kConfR });
  eq('random handshake: session key agreement', done.sessionKey, kSession);
  eq('random handshake: confirm MAC', new Uint8Array(Buffer.from(done.message.mac, 'base64')), kConfI);
  // And the two channels interoperate.
  const c = ViorSecure.channel(done.sessionKey, true);
  const s = ViorSecure.channel(kSession, false);
  const m = new Uint8Array(crypto.randomBytes(777));
  eq('random handshake: channel round trip', s.open(c.seal(m)), m);
}

// ============================================================================

console.log(`\n${passed} checks passed, ${failed} failed`);
if (failed > 0) process.exit(1);
console.log('OK — JS secure-channel client matches the committed Go vectors.');
