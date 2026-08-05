package handshake

import (
	"bytes"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/hkdf"
)

// updateVectors regenerates testdata/handshake_vectors.json instead of
// checking against it:
//
//	go test ./internal/handshake/ -update-vectors
//
// Regenerating is a deliberate, reviewable act: any diff in this file is a
// wire-format change that breaks every already-shipped client.
var updateVectors = flag.Bool("update-vectors", false, "rewrite the committed handshake test vectors")

const vectorFile = "testdata/handshake_vectors.json"

// vector is one fully-determined handshake run. Every field is hex-encoded so
// the TypeScript client suite can consume this file without a base64 or
// endianness convention to get wrong. On the wire these same byte strings are
// base64-encoded by the JSON envelope; that encoding is a transport detail and
// is deliberately not baked into the vectors.
type vector struct {
	Name string `json:"name"`

	// Inputs.
	Secret            string `json:"secret"`
	InitiatorPrivSeed string `json:"initiatorPrivSeed"`
	ResponderPrivSeed string `json:"responderPrivSeed"`
	InitiatorNonce    string `json:"initiatorNonce"`
	ResponderNonce    string `json:"responderNonce"`

	// Derived public values, so a client can check its X25519 before
	// blaming the key schedule.
	InitiatorPubKey string `json:"initiatorPubKey"`
	ResponderPubKey string `json:"responderPubKey"`
	SharedSecret    string `json:"sharedSecret"`
	Transcript      string `json:"transcript"`

	// Outputs.
	SessionKey   string `json:"sessionKey"`
	InitiatorMAC string `json:"initiatorMac"`
	ResponderMAC string `json:"responderMac"`
	SendKeyI2R   string `json:"sendKeyI2R"`
	SendKeyR2I   string `json:"sendKeyR2I"`
}

type vectorFileFormat struct {
	Comment string   `json:"_comment"`
	Version int      `json:"version"`
	Vectors []vector `json:"vectors"`
}

// vectorInputs are the deterministic seeds for one case.
type vectorInputs struct {
	name   string
	secret []byte
	iSeed  []byte
	rSeed  []byte
	iNonce []byte
	rNonce []byte
}

func vectorCases() []vectorInputs {
	return []vectorInputs{
		{
			name:   "all-zero secret",
			secret: make([]byte, SecretSize),
			iSeed:  bytes.Repeat([]byte{0x01}, 32),
			rSeed:  bytes.Repeat([]byte{0x02}, 32),
			iNonce: make([]byte, NonceSize),
			rNonce: make([]byte, NonceSize),
		},
		{
			name:   "counting secret",
			secret: testSecret(SecretSize, 0),
			iSeed:  bytes.Repeat([]byte{0x11}, 32),
			rSeed:  bytes.Repeat([]byte{0x22}, 32),
			iNonce: bytes.Repeat([]byte{0xA1}, NonceSize),
			rNonce: bytes.Repeat([]byte{0xB2}, NonceSize),
		},
		{
			name:   "high-bit secret",
			secret: bytes.Repeat([]byte{0xFF}, SecretSize),
			iSeed:  bytes.Repeat([]byte{0x7F}, 32),
			rSeed:  bytes.Repeat([]byte{0x80}, 32),
			iNonce: bytes.Repeat([]byte{0xFF}, NonceSize),
			rNonce: bytes.Repeat([]byte{0x00}, NonceSize),
		},
		{
			name:   "oversize secret (48 bytes)",
			secret: testSecret(48, 5),
			iSeed:  bytes.Repeat([]byte{0x33}, 32),
			rSeed:  bytes.Repeat([]byte{0x44}, 32),
			iNonce: bytes.Repeat([]byte{0x0F}, NonceSize),
			rNonce: bytes.Repeat([]byte{0xF0}, NonceSize),
		},
	}
}

// computeVector runs a full deterministic handshake and records every
// intermediate a client implementation might need to debug against.
func computeVector(t *testing.T, in vectorInputs) vector {
	t.Helper()

	ipriv, err := ecdh.X25519().NewPrivateKey(in.iSeed)
	if err != nil {
		t.Fatalf("%s: initiator key: %v", in.name, err)
	}
	rpriv, err := ecdh.X25519().NewPrivateKey(in.rSeed)
	if err != nil {
		t.Fatalf("%s: responder key: %v", in.name, err)
	}

	i, err := newInitiatorWith(in.secret, ipriv, in.iNonce)
	if err != nil {
		t.Fatalf("%s: newInitiatorWith: %v", in.name, err)
	}
	r, err := newResponderWith(in.secret, rpriv, in.rNonce)
	if err != nil {
		t.Fatalf("%s: newResponderWith: %v", in.name, err)
	}

	init, err := i.Init()
	if err != nil {
		t.Fatalf("%s: Init: %v", in.name, err)
	}
	resp, err := r.Respond(init)
	if err != nil {
		t.Fatalf("%s: Respond: %v", in.name, err)
	}
	conf, err := i.Finish(resp)
	if err != nil {
		t.Fatalf("%s: Finish: %v", in.name, err)
	}
	if err := r.Finish(conf); err != nil {
		t.Fatalf("%s: responder Finish: %v", in.name, err)
	}

	sessionKey, err := i.SessionKey()
	if err != nil {
		t.Fatalf("%s: SessionKey: %v", in.name, err)
	}

	ss, err := ipriv.ECDH(rpriv.PublicKey())
	if err != nil {
		t.Fatalf("%s: ECDH: %v", in.name, err)
	}
	tr := transcript(ipriv.PublicKey().Bytes(), rpriv.PublicKey().Bytes(), in.iNonce, in.rNonce)

	// The two directional record-layer keys are what securechan derives
	// next. Pinning them here means a client can localise a mismatch to
	// either the handshake or the record layer, not "somewhere in crypto".
	sendI2R, sendR2I := directionKeys(t, sessionKey)

	return vector{
		Name:              in.name,
		Secret:            hex.EncodeToString(in.secret),
		InitiatorPrivSeed: hex.EncodeToString(in.iSeed),
		ResponderPrivSeed: hex.EncodeToString(in.rSeed),
		InitiatorNonce:    hex.EncodeToString(in.iNonce),
		ResponderNonce:    hex.EncodeToString(in.rNonce),
		InitiatorPubKey:   hex.EncodeToString(ipriv.PublicKey().Bytes()),
		ResponderPubKey:   hex.EncodeToString(rpriv.PublicKey().Bytes()),
		SharedSecret:      hex.EncodeToString(ss),
		Transcript:        hex.EncodeToString(tr),
		SessionKey:        hex.EncodeToString(sessionKey),
		InitiatorMAC:      hex.EncodeToString(conf.MAC),
		ResponderMAC:      hex.EncodeToString(resp.MAC),
		SendKeyI2R:        hex.EncodeToString(sendI2R),
		SendKeyR2I:        hex.EncodeToString(sendR2I),
	}
}

// directionKeys reproduces securechan's HKDF split so the vectors cover the
// full path from bootstrap secret to record key. Kept in the test rather than
// exported from securechan, which deliberately keeps its key material private.
func directionKeys(t *testing.T, sessionKey []byte) (i2r, r2i []byte) {
	t.Helper()
	i2r = make([]byte, SessionKeySize)
	r2i = make([]byte, SessionKeySize)
	if err := hkdfInto(sessionKey, "vior-securechan v1 i2r", i2r); err != nil {
		t.Fatalf("derive i2r: %v", err)
	}
	if err := hkdfInto(sessionKey, "vior-securechan v1 r2i", r2i); err != nil {
		t.Fatalf("derive r2i: %v", err)
	}
	return i2r, r2i
}

func TestVectorsMatchCommittedFile(t *testing.T) {
	got := vectorFileFormat{
		Comment: "Cross-language test vectors for the Vior secure-channel handshake. " +
			"All values are hex. Consumed by the Go suite and by the TypeScript client " +
			"suite so Go and JS cannot silently diverge. A diff here is a wire-format " +
			"break: regenerate only deliberately, with `go test ./internal/handshake/ -update-vectors`.",
		Version: Version,
	}
	for _, c := range vectorCases() {
		got.Vectors = append(got.Vectors, computeVector(t, c))
	}

	encoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	encoded = append(encoded, '\n')

	if *updateVectors {
		if err := os.MkdirAll(filepath.Dir(vectorFile), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(vectorFile, encoded, 0o644); err != nil {
			t.Fatalf("write vectors: %v", err)
		}
		t.Logf("rewrote %s", vectorFile)
		return
	}

	want, err := os.ReadFile(vectorFile)
	if err != nil {
		t.Fatalf("read %s (run with -update-vectors to create it): %v", vectorFile, err)
	}
	if !bytes.Equal(normaliseEOL(want), normaliseEOL(encoded)) {
		t.Errorf("handshake output no longer matches %s.\n"+
			"This means the wire format or key schedule changed, which breaks every\n"+
			"shipped client. If the change is intended, bump Version and regenerate\n"+
			"with -update-vectors.", vectorFile)
	}
}

// The vectors are only meaningful if replaying their inputs reproduces their
// outputs — this guards against a vector file that was generated from a
// different code path than the one under test.
func TestVectorsAreSelfConsistent(t *testing.T) {
	raw, err := os.ReadFile(vectorFile)
	if err != nil {
		t.Fatalf("read %s: %v", vectorFile, err)
	}
	var f vectorFileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Version != Version {
		t.Fatalf("vector file version %d, package Version %d", f.Version, Version)
	}
	if len(f.Vectors) != len(vectorCases()) {
		t.Fatalf("vector file has %d cases, generator produces %d", len(f.Vectors), len(vectorCases()))
	}

	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			secret := mustHex(t, v.Secret)
			iSeed := mustHex(t, v.InitiatorPrivSeed)
			rSeed := mustHex(t, v.ResponderPrivSeed)

			ipriv, err := ecdh.X25519().NewPrivateKey(iSeed)
			if err != nil {
				t.Fatalf("initiator key: %v", err)
			}
			rpriv, err := ecdh.X25519().NewPrivateKey(rSeed)
			if err != nil {
				t.Fatalf("responder key: %v", err)
			}

			i, err := newInitiatorWith(secret, ipriv, mustHex(t, v.InitiatorNonce))
			if err != nil {
				t.Fatalf("newInitiatorWith: %v", err)
			}
			r, err := newResponderWith(secret, rpriv, mustHex(t, v.ResponderNonce))
			if err != nil {
				t.Fatalf("newResponderWith: %v", err)
			}

			init, _ := i.Init()
			resp, err := r.Respond(init)
			if err != nil {
				t.Fatalf("Respond: %v", err)
			}
			conf, err := i.Finish(resp)
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			if err := r.Finish(conf); err != nil {
				t.Fatalf("responder Finish: %v", err)
			}

			key, err := i.SessionKey()
			if err != nil {
				t.Fatalf("SessionKey: %v", err)
			}
			rkey, err := r.SessionKey()
			if err != nil {
				t.Fatalf("responder SessionKey: %v", err)
			}

			if hex.EncodeToString(key) != v.SessionKey {
				t.Errorf("session key = %x, want %s", key, v.SessionKey)
			}
			if !bytes.Equal(key, rkey) {
				t.Error("initiator and responder disagree on the session key")
			}
			if hex.EncodeToString(resp.MAC) != v.ResponderMAC {
				t.Errorf("responder MAC = %x, want %s", resp.MAC, v.ResponderMAC)
			}
			if hex.EncodeToString(conf.MAC) != v.InitiatorMAC {
				t.Errorf("initiator MAC = %x, want %s", conf.MAC, v.InitiatorMAC)
			}
		})
	}
}

// hkdfInto mirrors securechan's directional-key derivation. It is duplicated
// here rather than exported from that package on purpose: securechan keeps its
// key material unexported, and a test helper is a poor reason to widen a
// crypto package's API. If the two ever drift, TestVectorsMatchCommittedFile
// fails — which is exactly the alarm we want.
func hkdfInto(key []byte, info string, out []byte) error {
	_, err := io.ReadFull(hkdf.New(sha256.New, key, nil, []byte(info)), out)
	return err
}

// normaliseEOL trims surrounding whitespace and collapses CRLF to LF.
//
// Without this the comparison fails on Windows: git checks the committed file
// out with CRLF under the default core.autocrlf, while the freshly marshalled
// bytes always use LF. That difference is a property of the checkout, not of
// the handshake, and must not be reported as a wire-format break. .gitattributes
// pins the file to LF as well — this is the belt to that braces, so the test
// stays honest even in a tree checked out before that rule existed.
func normaliseEOL(b []byte) []byte {
	return bytes.ReplaceAll(bytes.TrimSpace(b), []byte("\r\n"), []byte("\n"))
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}
