// Package handshake establishes the 32-byte session key that
// internal/securechan consumes, from a bootstrap secret the two peers
// already share out-of-band (the QR payload).
//
// # Why this is a separate package from securechan
//
// Key agreement and record framing have different threat models, different
// review needs, and different change cadences. securechan stays a pure,
// dependency-light record layer that knows nothing about pair codes, QR
// payloads, or elliptic curves; everything delicate lives here, in one small
// file that can be reviewed on its own.
//
// # Construction
//
// A textbook authenticated Diffie-Hellman. Both sides generate an ephemeral
// X25519 keypair and a random nonce, exchange public halves, and bind the
// bootstrap secret into the key schedule:
//
//	T    = "vior-hs v1" || version || epk_i || epk_r || n_i || n_r
//	ss   = X25519(esk_self, epk_peer)
//	PRK  = HKDF-Extract(salt = T, ikm = ss || S)
//	k_session   = HKDF-Expand(PRK, "…session",   32)
//	k_confirm_i = HKDF-Expand(PRK, "…confirm-i", 32)
//	k_confirm_r = HKDF-Expand(PRK, "…confirm-r", 32)
//
// Each side then proves knowledge of S by MAC-ing the transcript under its
// own confirmation key. The responder confirms first, so an initiator holding
// a stale secret aborts without ever handing its own MAC to an impostor.
//
// # The entropy requirement is load-bearing
//
// This construction is sound only because S is high-entropy. An active MITM
// can complete a DH with either peer, capture a confirmation MAC, and then
// brute-force S offline — so with a 6-digit code (10^6 candidates) it would
// offer no protection at all. Defeating an offline search over a low-entropy
// secret is exactly what a PAKE is for, and that is deliberately NOT what
// this package does. MinSecretSize is enforced at construction rather than
// documented as a caution, because a caller passing a typed pair code here
// would get a channel that merely looks encrypted.
//
// See docs/securechan-handshake-architecture.md for the full rationale, the
// rejected alternatives, and the deferred typed-code (SPAKE2) path.
package handshake

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	// Version is the wire version of this handshake. It is carried in the
	// init message and bound into the transcript, so a peer cannot be
	// steered into a different version's key schedule.
	Version = 1

	// SecretSize is the length of a freshly generated bootstrap secret.
	SecretSize = 32

	// MinSecretSize is the smallest bootstrap secret this package accepts.
	// See the package doc: below this, the construction is not sound.
	MinSecretSize = 32

	// SessionKeySize is the length of the derived session key, matching
	// securechan.KeySize.
	SessionKeySize = 32

	// NonceSize is the length of each side's random handshake nonce. The
	// nonces guarantee a distinct transcript per run even in the
	// (impossible-by-construction, but cheap to defend) event of ephemeral
	// key reuse.
	NonceSize = 16

	// PubKeySize is the length of an X25519 public key.
	PubKeySize = 32

	// MACSize is the length of a confirmation tag (HMAC-SHA256).
	MACSize = 32
)

// Domain-separation labels. Versioned so a future wire change rotates the
// whole schedule cleanly rather than colliding with v1 keys.
const (
	transcriptLabel   = "vior-hs v1"
	infoSession       = "vior-hs v1 session"
	infoConfirmInit   = "vior-hs v1 confirm-i"
	infoConfirmRespon = "vior-hs v1 confirm-r"
)

var (
	// ErrShortSecret is returned when the bootstrap secret is below
	// MinSecretSize. This is a hard error, not a warning: see the package
	// doc for why a short secret makes the whole construction hollow.
	ErrShortSecret = errors.New("handshake: bootstrap secret must be at least 32 bytes")

	// ErrBadVersion is returned when the peer announces an unsupported
	// handshake version.
	ErrBadVersion = errors.New("handshake: unsupported version")

	// ErrMalformed is returned when a message field has the wrong length or
	// is not a valid public key.
	ErrMalformed = errors.New("handshake: malformed message")

	// ErrAuth is returned when a confirmation MAC does not verify. In
	// practice this means the peer does not know the bootstrap secret —
	// a wrong/stale QR code, or an active MITM.
	ErrAuth = errors.New("handshake: peer failed to prove knowledge of the shared secret")

	// ErrState is returned when the handshake steps are driven out of
	// order, or reused after completion or failure.
	ErrState = errors.New("handshake: called out of order")

	// ErrIncomplete is returned by SessionKey before the handshake has
	// successfully completed.
	ErrIncomplete = errors.New("handshake: not complete")
)

// state tracks progress so a caller cannot skip a step, repeat one, or read a
// session key out of a run that never authenticated. Every failure is
// terminal — there is deliberately no path back to a usable state.
type state int

const (
	stateNew state = iota
	stateAwaitResponse
	stateAwaitConfirm
	stateDone
	stateFailed
)

// Init is the initiator's opening message ("secure-init").
type Init struct {
	Version int    `json:"v"`
	PubKey  []byte `json:"epk"`
	Nonce   []byte `json:"n"`
}

// Response is the responder's reply ("secure-resp"). MAC proves the responder
// knows the bootstrap secret.
type Response struct {
	PubKey []byte `json:"epk"`
	Nonce  []byte `json:"n"`
	MAC    []byte `json:"mac"`
}

// Confirm is the initiator's closing message ("secure-confirm").
type Confirm struct {
	MAC []byte `json:"mac"`
}

// NewSecret generates a fresh bootstrap secret suitable for carriage in a QR
// payload.
func NewSecret() ([]byte, error) {
	s := make([]byte, SecretSize)
	if _, err := rand.Read(s); err != nil {
		return nil, fmt.Errorf("handshake: generate secret: %w", err)
	}
	return s, nil
}

// keySchedule is the set of keys both sides derive from the same inputs.
type keySchedule struct {
	session  [SessionKeySize]byte
	confirmI [MACSize]byte
	confirmR [MACSize]byte
}

// Initiator drives the client side of the handshake. It is not safe for
// concurrent use; the session layer drives it from a single goroutine before
// any other traffic flows.
type Initiator struct {
	secret []byte
	priv   *ecdh.PrivateKey
	nonce  []byte
	state  state
	keys   keySchedule
}

// NewInitiator creates the client side of a handshake against secret.
func NewInitiator(secret []byte) (*Initiator, error) {
	if len(secret) < MinSecretSize {
		return nil, ErrShortSecret
	}
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("handshake: generate ephemeral key: %w", err)
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("handshake: generate nonce: %w", err)
	}
	return newInitiatorWith(secret, priv, nonce)
}

// newInitiatorWith is the deterministic constructor used by the test-vector
// suite. Production callers use NewInitiator.
func newInitiatorWith(secret []byte, priv *ecdh.PrivateKey, nonce []byte) (*Initiator, error) {
	if len(secret) < MinSecretSize {
		return nil, ErrShortSecret
	}
	if len(nonce) != NonceSize {
		return nil, ErrMalformed
	}
	return &Initiator{
		secret: append([]byte(nil), secret...),
		priv:   priv,
		nonce:  append([]byte(nil), nonce...),
		state:  stateNew,
	}, nil
}

// Init produces the opening message. It must be called exactly once, first.
func (i *Initiator) Init() (*Init, error) {
	if i.state != stateNew {
		return nil, ErrState
	}
	i.state = stateAwaitResponse
	return &Init{
		Version: Version,
		PubKey:  i.priv.PublicKey().Bytes(),
		Nonce:   append([]byte(nil), i.nonce...),
	}, nil
}

// Finish verifies the responder's confirmation and produces the initiator's
// own. A non-nil error means the handshake failed and the connection must be
// closed — there is no retry and no downgrade.
func (i *Initiator) Finish(resp *Response) (*Confirm, error) {
	if i.state != stateAwaitResponse {
		return nil, ErrState
	}
	if resp == nil || len(resp.Nonce) != NonceSize || len(resp.MAC) != MACSize {
		i.state = stateFailed
		return nil, ErrMalformed
	}
	peer, err := ecdh.X25519().NewPublicKey(resp.PubKey)
	if err != nil {
		i.state = stateFailed
		return nil, ErrMalformed
	}
	// ECDH rejects an all-zero shared secret, which is what a low-order
	// peer key would produce. That check is why this uses crypto/ecdh
	// rather than a raw curve25519.X25519 call.
	ss, err := i.priv.ECDH(peer)
	if err != nil {
		i.state = stateFailed
		return nil, ErrMalformed
	}

	t := transcript(i.priv.PublicKey().Bytes(), resp.PubKey, i.nonce, resp.Nonce)
	ks, err := derive(ss, i.secret, t)
	if err != nil {
		i.state = stateFailed
		return nil, err
	}
	// Verify the responder before revealing our own MAC, so a stale secret
	// aborts without handing an impostor anything to attack offline.
	if !hmac.Equal(resp.MAC, ks.confirmR[:]) {
		i.state = stateFailed
		return nil, ErrAuth
	}

	i.keys = *ks
	i.state = stateDone
	return &Confirm{MAC: append([]byte(nil), ks.confirmI[:]...)}, nil
}

// SessionKey returns the derived key once the handshake has completed.
func (i *Initiator) SessionKey() ([]byte, error) {
	if i.state != stateDone {
		return nil, ErrIncomplete
	}
	return append([]byte(nil), i.keys.session[:]...), nil
}

// Responder drives the server side of the handshake. Not safe for concurrent
// use; one Responder belongs to one connection.
type Responder struct {
	secret []byte
	priv   *ecdh.PrivateKey
	nonce  []byte
	state  state
	keys   keySchedule
}

// NewResponder creates the server side of a handshake against secret.
func NewResponder(secret []byte) (*Responder, error) {
	if len(secret) < MinSecretSize {
		return nil, ErrShortSecret
	}
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("handshake: generate ephemeral key: %w", err)
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("handshake: generate nonce: %w", err)
	}
	return newResponderWith(secret, priv, nonce)
}

// newResponderWith is the deterministic constructor used by the test-vector
// suite. Production callers use NewResponder.
func newResponderWith(secret []byte, priv *ecdh.PrivateKey, nonce []byte) (*Responder, error) {
	if len(secret) < MinSecretSize {
		return nil, ErrShortSecret
	}
	if len(nonce) != NonceSize {
		return nil, ErrMalformed
	}
	return &Responder{
		secret: append([]byte(nil), secret...),
		priv:   priv,
		nonce:  append([]byte(nil), nonce...),
		state:  stateNew,
	}, nil
}

// Respond consumes the initiator's opening message and produces the
// responder's reply, including its confirmation MAC.
func (r *Responder) Respond(init *Init) (*Response, error) {
	if r.state != stateNew {
		return nil, ErrState
	}
	if init == nil || len(init.Nonce) != NonceSize {
		r.state = stateFailed
		return nil, ErrMalformed
	}
	if init.Version != Version {
		r.state = stateFailed
		return nil, ErrBadVersion
	}
	peer, err := ecdh.X25519().NewPublicKey(init.PubKey)
	if err != nil {
		r.state = stateFailed
		return nil, ErrMalformed
	}
	ss, err := r.priv.ECDH(peer)
	if err != nil {
		r.state = stateFailed
		return nil, ErrMalformed
	}

	t := transcript(init.PubKey, r.priv.PublicKey().Bytes(), init.Nonce, r.nonce)
	ks, err := derive(ss, r.secret, t)
	if err != nil {
		r.state = stateFailed
		return nil, err
	}

	r.keys = *ks
	r.state = stateAwaitConfirm
	return &Response{
		PubKey: r.priv.PublicKey().Bytes(),
		Nonce:  append([]byte(nil), r.nonce...),
		MAC:    append([]byte(nil), ks.confirmR[:]...),
	}, nil
}

// Finish verifies the initiator's confirmation. A non-nil error means the
// connection must be closed.
func (r *Responder) Finish(c *Confirm) error {
	if r.state != stateAwaitConfirm {
		return ErrState
	}
	if c == nil || len(c.MAC) != MACSize {
		r.state = stateFailed
		return ErrMalformed
	}
	if !hmac.Equal(c.MAC, r.keys.confirmI[:]) {
		r.state = stateFailed
		return ErrAuth
	}
	r.state = stateDone
	return nil
}

// SessionKey returns the derived key once the handshake has completed.
func (r *Responder) SessionKey() ([]byte, error) {
	if r.state != stateDone {
		return nil, ErrIncomplete
	}
	return append([]byte(nil), r.keys.session[:]...), nil
}

// transcript builds the value both sides bind into the key schedule and MAC.
// Every component is fixed-length, so plain concatenation is unambiguous and
// no length prefixes are needed; the caller-supplied lengths are validated
// before this is reached.
func transcript(epkI, epkR, nonceI, nonceR []byte) []byte {
	t := make([]byte, 0, len(transcriptLabel)+1+2*PubKeySize+2*NonceSize)
	t = append(t, transcriptLabel...)
	t = append(t, byte(Version))
	t = append(t, epkI...)
	t = append(t, epkR...)
	t = append(t, nonceI...)
	t = append(t, nonceR...)
	return t
}

// derive runs the key schedule. The transcript is the HKDF salt and the
// concatenation of the DH output with the bootstrap secret is the input key
// material — so an attacker must know BOTH the DH secret and S to reach any
// derived key.
func derive(dhSecret, bootstrap, t []byte) (*keySchedule, error) {
	ikm := make([]byte, 0, len(dhSecret)+len(bootstrap))
	ikm = append(ikm, dhSecret...)
	ikm = append(ikm, bootstrap...)

	prk := hkdf.Extract(sha256.New, ikm, t)

	ks := &keySchedule{}
	for _, f := range []struct {
		info string
		out  []byte
	}{
		{infoSession, ks.session[:]},
		{infoConfirmInit, ks.confirmI[:]},
		{infoConfirmRespon, ks.confirmR[:]},
	} {
		if _, err := io.ReadFull(hkdf.Expand(sha256.New, prk, []byte(f.info)), f.out); err != nil {
			return nil, fmt.Errorf("handshake: key derivation: %w", err)
		}
	}
	return ks, nil
}
