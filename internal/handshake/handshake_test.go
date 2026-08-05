package handshake

import (
	"bytes"
	"crypto/ecdh"
	"errors"
	"testing"

	"github.com/subhashraveendran/vior/internal/securechan"
)

// testSecret builds a deterministic bootstrap secret of the given length.
func testSecret(n int, seed byte) []byte {
	s := make([]byte, n)
	for i := range s {
		s[i] = byte(i)*3 + seed
	}
	return s
}

// run drives a full handshake and returns both derived session keys.
func run(t *testing.T, initSecret, respSecret []byte) (ikey, rkey []byte, err error) {
	t.Helper()
	i, err := NewInitiator(initSecret)
	if err != nil {
		return nil, nil, err
	}
	r, err := NewResponder(respSecret)
	if err != nil {
		return nil, nil, err
	}
	init, err := i.Init()
	if err != nil {
		return nil, nil, err
	}
	resp, err := r.Respond(init)
	if err != nil {
		return nil, nil, err
	}
	conf, err := i.Finish(resp)
	if err != nil {
		return nil, nil, err
	}
	if err := r.Finish(conf); err != nil {
		return nil, nil, err
	}
	ikey, err = i.SessionKey()
	if err != nil {
		return nil, nil, err
	}
	rkey, err = r.SessionKey()
	if err != nil {
		return nil, nil, err
	}
	return ikey, rkey, nil
}

func TestHandshakeAgreesOnSessionKey(t *testing.T) {
	s := testSecret(SecretSize, 0)
	ikey, rkey, err := run(t, s, s)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if !bytes.Equal(ikey, rkey) {
		t.Fatalf("session keys differ:\n init %x\n resp %x", ikey, rkey)
	}
	if len(ikey) != SessionKeySize {
		t.Fatalf("session key length = %d, want %d", len(ikey), SessionKeySize)
	}
	var zero [SessionKeySize]byte
	if bytes.Equal(ikey, zero[:]) {
		t.Fatal("session key is all zeroes")
	}
}

// The derived key must actually drive the record layer — this is the seam
// where a size or direction mismatch would otherwise surface only in
// production.
func TestDerivedKeyDrivesSecurechan(t *testing.T) {
	s := testSecret(SecretSize, 9)
	ikey, rkey, err := run(t, s, s)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	ic, err := securechan.NewChannel(ikey, true)
	if err != nil {
		t.Fatalf("NewChannel initiator: %v", err)
	}
	rc, err := securechan.NewChannel(rkey, false)
	if err != nil {
		t.Fatalf("NewChannel responder: %v", err)
	}

	msg := []byte(`{"type":"hello","data":{"width":1170}}`)
	frame, err := ic.Seal(msg)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := rc.Open(frame)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("round trip mismatch: got %q want %q", got, msg)
	}

	// And the reverse direction, since the two ends must have opposite
	// initiator flags for the directional keys to line up.
	reply := []byte(`{"type":"ready"}`)
	frame2, err := rc.Seal(reply)
	if err != nil {
		t.Fatalf("Seal responder: %v", err)
	}
	got2, err := ic.Open(frame2)
	if err != nil {
		t.Fatalf("Open initiator: %v", err)
	}
	if !bytes.Equal(got2, reply) {
		t.Fatalf("reverse mismatch: got %q want %q", got2, reply)
	}
}

func TestEachRunDerivesADistinctKey(t *testing.T) {
	s := testSecret(SecretSize, 0)
	k1, _, err := run(t, s, s)
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	k2, _, err := run(t, s, s)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	// Ephemeral keys per connection are what make securechan's counter
	// restart safe across reconnects. If this ever fails, nonce reuse
	// across sessions becomes possible.
	if bytes.Equal(k1, k2) {
		t.Fatal("two runs produced the same session key — ephemerals are not fresh")
	}
}

func TestWrongSecretFailsAtInitiator(t *testing.T) {
	good := testSecret(SecretSize, 0)
	bad := testSecret(SecretSize, 1)

	i, err := NewInitiator(good)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	r, err := NewResponder(bad)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	init, err := i.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	resp, err := r.Respond(init)
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	// The initiator must reject before producing its own MAC.
	conf, err := i.Finish(resp)
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("Finish error = %v, want ErrAuth", err)
	}
	if conf != nil {
		t.Fatal("initiator leaked a confirmation MAC to an unauthenticated peer")
	}
}

// An attacker who relays messages but does not know the secret must not be
// able to complete the handshake with the responder.
func TestMITMWithoutSecretCannotComplete(t *testing.T) {
	real := testSecret(SecretSize, 0)
	guess := testSecret(SecretSize, 2)

	r, err := NewResponder(real)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	attacker, err := NewInitiator(guess)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	init, err := attacker.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	resp, err := r.Respond(init)
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	// Attacker cannot verify the responder, but suppose it pushes a forged
	// confirmation anyway.
	forged := &Confirm{MAC: make([]byte, MACSize)}
	if err := r.Finish(forged); !errors.Is(err, ErrAuth) {
		t.Fatalf("Finish with forged MAC = %v, want ErrAuth", err)
	}
	if _, err := r.SessionKey(); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("SessionKey after failure = %v, want ErrIncomplete", err)
	}
	_ = resp
}

func TestTamperedResponseFailsAuth(t *testing.T) {
	s := testSecret(SecretSize, 0)

	for _, tc := range []struct {
		name   string
		mutate func(*Response)
	}{
		{"nonce flipped", func(r *Response) { r.Nonce[0] ^= 0xFF }},
		{"mac flipped", func(r *Response) { r.MAC[0] ^= 0xFF }},
		{"pubkey flipped", func(r *Response) { r.PubKey[0] ^= 0xFF }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i, err := NewInitiator(s)
			if err != nil {
				t.Fatalf("NewInitiator: %v", err)
			}
			r, err := NewResponder(s)
			if err != nil {
				t.Fatalf("NewResponder: %v", err)
			}
			init, err := i.Init()
			if err != nil {
				t.Fatalf("Init: %v", err)
			}
			resp, err := r.Respond(init)
			if err != nil {
				t.Fatalf("Respond: %v", err)
			}
			tc.mutate(resp)
			if _, err := i.Finish(resp); !errors.Is(err, ErrAuth) {
				t.Fatalf("Finish = %v, want ErrAuth", err)
			}
		})
	}
}

// A tampered init message changes the transcript, so the responder's MAC no
// longer matches what the honest initiator computes.
func TestTamperedInitBreaksTranscript(t *testing.T) {
	s := testSecret(SecretSize, 0)
	i, err := NewInitiator(s)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	r, err := NewResponder(s)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	init, err := i.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	tampered := &Init{
		Version: init.Version,
		PubKey:  append([]byte(nil), init.PubKey...),
		Nonce:   append([]byte(nil), init.Nonce...),
	}
	tampered.Nonce[0] ^= 0xFF

	resp, err := r.Respond(tampered)
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if _, err := i.Finish(resp); !errors.Is(err, ErrAuth) {
		t.Fatalf("Finish = %v, want ErrAuth", err)
	}
}

func TestShortSecretRejected(t *testing.T) {
	// A 6-digit pair code is the exact misuse this guard exists to stop.
	for _, secret := range [][]byte{nil, []byte("123456"), testSecret(MinSecretSize-1, 0)} {
		if _, err := NewInitiator(secret); !errors.Is(err, ErrShortSecret) {
			t.Fatalf("NewInitiator(%d bytes) = %v, want ErrShortSecret", len(secret), err)
		}
		if _, err := NewResponder(secret); !errors.Is(err, ErrShortSecret) {
			t.Fatalf("NewResponder(%d bytes) = %v, want ErrShortSecret", len(secret), err)
		}
	}
}

func TestVersionMismatchRejected(t *testing.T) {
	s := testSecret(SecretSize, 0)
	r, err := NewResponder(s)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	i, err := NewInitiator(s)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	init, err := i.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	init.Version = Version + 1
	if _, err := r.Respond(init); !errors.Is(err, ErrBadVersion) {
		t.Fatalf("Respond = %v, want ErrBadVersion", err)
	}
}

func TestMalformedMessagesRejected(t *testing.T) {
	s := testSecret(SecretSize, 0)

	t.Run("nil init", func(t *testing.T) {
		r, _ := NewResponder(s)
		if _, err := r.Respond(nil); !errors.Is(err, ErrMalformed) {
			t.Fatalf("Respond(nil) = %v, want ErrMalformed", err)
		}
	})
	t.Run("short nonce", func(t *testing.T) {
		r, _ := NewResponder(s)
		i, _ := NewInitiator(s)
		init, _ := i.Init()
		init.Nonce = init.Nonce[:NonceSize-1]
		if _, err := r.Respond(init); !errors.Is(err, ErrMalformed) {
			t.Fatalf("Respond = %v, want ErrMalformed", err)
		}
	})
	t.Run("short pubkey", func(t *testing.T) {
		r, _ := NewResponder(s)
		i, _ := NewInitiator(s)
		init, _ := i.Init()
		init.PubKey = init.PubKey[:PubKeySize-1]
		if _, err := r.Respond(init); !errors.Is(err, ErrMalformed) {
			t.Fatalf("Respond = %v, want ErrMalformed", err)
		}
	})
	t.Run("nil response", func(t *testing.T) {
		i, _ := NewInitiator(s)
		if _, err := i.Init(); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if _, err := i.Finish(nil); !errors.Is(err, ErrMalformed) {
			t.Fatalf("Finish(nil) = %v, want ErrMalformed", err)
		}
	})
	t.Run("nil confirm", func(t *testing.T) {
		i, _ := NewInitiator(s)
		r, _ := NewResponder(s)
		init, _ := i.Init()
		if _, err := r.Respond(init); err != nil {
			t.Fatalf("Respond: %v", err)
		}
		if err := r.Finish(nil); !errors.Is(err, ErrMalformed) {
			t.Fatalf("Finish(nil) = %v, want ErrMalformed", err)
		}
	})
}

// An all-zero public key is a low-order point; crypto/ecdh must reject the
// resulting all-zero shared secret rather than deriving a predictable key.
func TestLowOrderPublicKeyRejected(t *testing.T) {
	s := testSecret(SecretSize, 0)
	r, err := NewResponder(s)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	init := &Init{
		Version: Version,
		PubKey:  make([]byte, PubKeySize), // all zeroes
		Nonce:   make([]byte, NonceSize),
	}
	if _, err := r.Respond(init); !errors.Is(err, ErrMalformed) {
		t.Fatalf("Respond with low-order key = %v, want ErrMalformed", err)
	}
}

func TestStateMachineRejectsMisuse(t *testing.T) {
	s := testSecret(SecretSize, 0)

	t.Run("init twice", func(t *testing.T) {
		i, _ := NewInitiator(s)
		if _, err := i.Init(); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if _, err := i.Init(); !errors.Is(err, ErrState) {
			t.Fatalf("second Init = %v, want ErrState", err)
		}
	})
	t.Run("finish before init", func(t *testing.T) {
		i, _ := NewInitiator(s)
		if _, err := i.Finish(&Response{
			PubKey: make([]byte, PubKeySize),
			Nonce:  make([]byte, NonceSize),
			MAC:    make([]byte, MACSize),
		}); !errors.Is(err, ErrState) {
			t.Fatalf("Finish before Init = %v, want ErrState", err)
		}
	})
	t.Run("responder confirm before respond", func(t *testing.T) {
		r, _ := NewResponder(s)
		if err := r.Finish(&Confirm{MAC: make([]byte, MACSize)}); !errors.Is(err, ErrState) {
			t.Fatalf("Finish before Respond = %v, want ErrState", err)
		}
	})
	t.Run("respond twice", func(t *testing.T) {
		i, _ := NewInitiator(s)
		r, _ := NewResponder(s)
		init, _ := i.Init()
		if _, err := r.Respond(init); err != nil {
			t.Fatalf("Respond: %v", err)
		}
		if _, err := r.Respond(init); !errors.Is(err, ErrState) {
			t.Fatalf("second Respond = %v, want ErrState", err)
		}
	})
	t.Run("session key before completion", func(t *testing.T) {
		i, _ := NewInitiator(s)
		if _, err := i.SessionKey(); !errors.Is(err, ErrIncomplete) {
			t.Fatalf("SessionKey = %v, want ErrIncomplete", err)
		}
		r, _ := NewResponder(s)
		if _, err := r.SessionKey(); !errors.Is(err, ErrIncomplete) {
			t.Fatalf("SessionKey = %v, want ErrIncomplete", err)
		}
	})
	t.Run("no recovery after failure", func(t *testing.T) {
		i, _ := NewInitiator(s)
		r, _ := NewResponder(s)
		init, _ := i.Init()
		resp, _ := r.Respond(init)
		resp.MAC[0] ^= 0xFF
		if _, err := i.Finish(resp); !errors.Is(err, ErrAuth) {
			t.Fatalf("Finish = %v, want ErrAuth", err)
		}
		// A failed run is terminal — a retry must not resurrect it.
		if _, err := i.Finish(resp); !errors.Is(err, ErrState) {
			t.Fatalf("retry after failure = %v, want ErrState", err)
		}
		if _, err := i.SessionKey(); !errors.Is(err, ErrIncomplete) {
			t.Fatalf("SessionKey after failure = %v, want ErrIncomplete", err)
		}
	})
}

// Confirmation keys must be distinct per direction, otherwise a reflected
// responder MAC would authenticate the initiator to itself.
func TestConfirmationKeysAreDirectional(t *testing.T) {
	s := testSecret(SecretSize, 0)
	i, _ := NewInitiator(s)
	r, _ := NewResponder(s)
	init, _ := i.Init()
	resp, err := r.Respond(init)
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	// Reflect the responder's own MAC back at it as the initiator's.
	if err := r.Finish(&Confirm{MAC: resp.MAC}); !errors.Is(err, ErrAuth) {
		t.Fatalf("reflected MAC accepted (= %v), want ErrAuth", err)
	}
}

func TestNewSecretIsFreshAndCorrectlySized(t *testing.T) {
	a, err := NewSecret()
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}
	b, err := NewSecret()
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}
	if len(a) != SecretSize {
		t.Fatalf("secret length = %d, want %d", len(a), SecretSize)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two secrets were identical")
	}
	if len(a) < MinSecretSize {
		t.Fatal("generated secret is below MinSecretSize")
	}
}

// Deterministic constructors must produce a byte-identical schedule for fixed
// inputs. This is the property the cross-language vectors depend on.
func TestDeterministicWithFixedInputs(t *testing.T) {
	s := testSecret(SecretSize, 0)
	ipriv, rpriv := fixedKeys(t)
	inonce := bytes.Repeat([]byte{0xA1}, NonceSize)
	rnonce := bytes.Repeat([]byte{0xB2}, NonceSize)

	derive1 := func() []byte {
		i, err := newInitiatorWith(s, ipriv, inonce)
		if err != nil {
			t.Fatalf("newInitiatorWith: %v", err)
		}
		r, err := newResponderWith(s, rpriv, rnonce)
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
		k, err := i.SessionKey()
		if err != nil {
			t.Fatalf("SessionKey: %v", err)
		}
		return k
	}

	if a, b := derive1(), derive1(); !bytes.Equal(a, b) {
		t.Fatalf("fixed inputs produced different keys:\n %x\n %x", a, b)
	}
}

// fixedKeys returns two deterministic X25519 private keys for vector tests.
func fixedKeys(t *testing.T) (initiator, responder *ecdh.PrivateKey) {
	t.Helper()
	iseed := bytes.Repeat([]byte{0x11}, 32)
	rseed := bytes.Repeat([]byte{0x22}, 32)
	i, err := ecdh.X25519().NewPrivateKey(iseed)
	if err != nil {
		t.Fatalf("initiator key: %v", err)
	}
	r, err := ecdh.X25519().NewPrivateKey(rseed)
	if err != nil {
		t.Fatalf("responder key: %v", err)
	}
	return i, r
}
