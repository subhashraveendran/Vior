// Package securechan implements the application-layer encrypted framing Vior
// uses once two peers share a session key. It sits directly on top of the
// (plain ws://) WebSocket message stream and provides confidentiality,
// integrity, and replay protection using NaCl secretbox (XSalsa20-Poly1305) —
// the same primitive as tweetnacl-js `secretbox`, so the Go host and the
// browser / Capacitor clients interoperate byte-for-byte.
//
// Scope: this package is deliberately the RECORD layer only. It assumes a
// 32-byte session key already exists between the two peers. Establishing that
// key from the low-entropy 6-digit pair code — which must go through a PAKE
// (SPAKE2) so the code can't be brute-forced offline — is a separate handshake
// step tracked in docs/transport-security-plan.md. Deriving a channel key
// directly from the pair code is NOT sound and this package intentionally does
// not offer a helper that would invite it.
//
// Framing. Each direction gets its own key (HKDF-SHA256 of the shared key with
// a direction label), so a counter value never produces the same (key, nonce)
// pair in both directions. A frame is:
//
//	counter(8, big-endian) || secretbox(plaintext, nonce=counter||0…, dirKey)
//
// The counter is monotonic per sender; the receiver rejects any counter that is
// not strictly greater than the highest it has already accepted, which gives
// replay and reorder protection over the reliable, ordered WebSocket/TCP stream.
package securechan

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/nacl/secretbox"
)

const (
	// KeySize is the shared session-key length (and each derived direction key).
	KeySize = 32
	// nonceSize is the secretbox nonce length.
	nonceSize = 24
	// counterPrefix is the number of leading frame bytes carrying the nonce
	// counter (big-endian uint64).
	counterPrefix = 8
	// Overhead is the per-frame expansion: the counter prefix plus the
	// secretbox Poly1305 tag.
	Overhead = counterPrefix + secretbox.Overhead
)

// hkdfInfo labels separate the two directional keys derived from one shared
// key. Versioned so a future wire change can rotate the derivation cleanly.
const (
	infoInitiatorToResponder = "vior-securechan v1 i2r"
	infoResponderToInitiator = "vior-securechan v1 r2i"
)

var (
	// ErrShortKey is returned when the shared key is not KeySize bytes.
	ErrShortKey = errors.New("securechan: shared key must be 32 bytes")
	// ErrShortFrame is returned when a frame is too small to contain a counter
	// and tag.
	ErrShortFrame = errors.New("securechan: frame too short")
	// ErrReplay is returned when a frame's counter is not strictly greater than
	// the highest already accepted (replay or reorder).
	ErrReplay = errors.New("securechan: replayed or out-of-order frame")
	// ErrDecrypt is returned when authentication/decryption fails (wrong key or
	// tampered ciphertext).
	ErrDecrypt = errors.New("securechan: authentication failed")
	// ErrNonceExhausted is returned once a sender has used all 2^64 counters.
	ErrNonceExhausted = errors.New("securechan: nonce space exhausted")
)

// Channel is one peer's view of an encrypted, replay-protected byte channel.
// Seal and Open are each safe for concurrent use, and safe to call
// concurrently with one another (send and receive state are independent).
type Channel struct {
	sendMu      sync.Mutex
	sendKey     [KeySize]byte
	sendCounter uint64

	recvMu      sync.Mutex
	recvKey     [KeySize]byte
	recvHighest uint64
	recvSeen    bool
}

// NewChannel derives a Channel from a shared 32-byte session key. The two peers
// must pass the same key but opposite `initiator` values so their send/receive
// keys line up (the initiator's send key equals the responder's receive key).
func NewChannel(sharedKey []byte, initiator bool) (*Channel, error) {
	if len(sharedKey) != KeySize {
		return nil, ErrShortKey
	}
	c := &Channel{}
	sendInfo, recvInfo := infoInitiatorToResponder, infoResponderToInitiator
	if !initiator {
		sendInfo, recvInfo = recvInfo, sendInfo
	}
	if err := deriveKey(sharedKey, sendInfo, c.sendKey[:]); err != nil {
		return nil, err
	}
	if err := deriveKey(sharedKey, recvInfo, c.recvKey[:]); err != nil {
		return nil, err
	}
	return c, nil
}

// deriveKey fills out with HKDF-SHA256(sharedKey, info).
func deriveKey(sharedKey []byte, info string, out []byte) error {
	r := hkdf.New(sha256.New, sharedKey, nil, []byte(info))
	if _, err := io.ReadFull(r, out); err != nil {
		return fmt.Errorf("securechan: key derivation: %w", err)
	}
	return nil
}

// Seal encrypts plaintext into a self-framed message. The returned slice is a
// fresh allocation the caller owns.
func (c *Channel) Seal(plaintext []byte) ([]byte, error) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendCounter == math.MaxUint64 {
		return nil, ErrNonceExhausted
	}
	var nonce [nonceSize]byte
	binary.BigEndian.PutUint64(nonce[:counterPrefix], c.sendCounter)

	out := make([]byte, counterPrefix, counterPrefix+len(plaintext)+secretbox.Overhead)
	binary.BigEndian.PutUint64(out, c.sendCounter)
	out = secretbox.Seal(out, plaintext, &nonce, &c.sendKey)

	c.sendCounter++
	return out, nil
}

// Open authenticates and decrypts a frame produced by the peer's Seal. It
// rejects frames whose counter is not strictly greater than the highest
// already accepted, so replays and reordered frames fail closed.
func (c *Channel) Open(frame []byte) ([]byte, error) {
	if len(frame) < Overhead {
		return nil, ErrShortFrame
	}
	counter := binary.BigEndian.Uint64(frame[:counterPrefix])

	c.recvMu.Lock()
	defer c.recvMu.Unlock()
	if c.recvSeen && counter <= c.recvHighest {
		return nil, ErrReplay
	}

	var nonce [nonceSize]byte
	binary.BigEndian.PutUint64(nonce[:counterPrefix], counter)
	plaintext, ok := secretbox.Open(nil, frame[counterPrefix:], &nonce, &c.recvKey)
	if !ok {
		return nil, ErrDecrypt
	}

	c.recvHighest = counter
	c.recvSeen = true
	return plaintext, nil
}
