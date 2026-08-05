package stream

import (
	"crypto/subtle"
	"encoding/base64"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/subhashraveendran/vior/internal/handshake"
)

// The channel secret is the high-entropy value the secure-channel handshake
// authenticates against. It is delivered to the client out-of-band in the QR
// payload, which is a machine-to-machine channel and can carry 256 bits at no
// UX cost — unlike the 6-digit pair code, which exists to be typed by a human
// and is far too small to resist an offline search. See
// docs/securechan-handshake-architecture.md.
//
// Persistence follows the existing ~/.vior/ convention (pair.txt, server-id):
// the secret is written once at 0600 and reused across restarts.
//
// This is a deliberate deviation from the architecture review, which proposed
// rotating per server start. Rotating would invalidate every saved connection
// on every restart and force a fresh QR scan each time, which fights the
// reconnect behaviour mobile clients already depend on. Stability wins for now;
// RotateSecret gives the user an explicit "revoke everything" action, and
// TTL/single-use secrets remain the deferred follow-up.

const secretFileName = "channel-secret"

// Initialised at package load, mirroring the pairCode = derivePair()
// convention above it, so the secret is available before the first upgrade.
var (
	channelSecretMu sync.RWMutex
	channelSecret   = loadOrCreateSecret()
)

// secretFilePath returns ~/.vior/channel-secret, or "" if there is no home
// directory.
func secretFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".vior", secretFileName)
}

// loadOrCreateSecret returns the persisted channel secret, generating and
// storing one on first run. A missing home directory or an unwritable
// ~/.vior degrades to an ephemeral per-process secret rather than failing:
// the secure channel still works for this run, the QR simply stops being
// valid after a restart.
func loadOrCreateSecret() []byte {
	path := secretFilePath()
	if path == "" {
		log.Printf("stream: no home dir — using an ephemeral channel secret for this run")
		return mustNewSecret()
	}

	if b, err := os.ReadFile(path); err == nil {
		if s, ok := decodeSecret(string(b)); ok {
			return s
		}
		// A corrupt or truncated file must not silently produce a weak
		// secret. Replace it.
		log.Printf("stream: %s is invalid — regenerating the channel secret", path)
	} else if !os.IsNotExist(err) {
		log.Printf("stream: read %s failed: %v — regenerating", path, err)
	}

	s := mustNewSecret()
	if err := writeSecret(path, s); err != nil {
		log.Printf("stream: persist channel secret failed: %v (ephemeral for this run)", err)
	}
	return s
}

// decodeSecret parses a stored secret and enforces the minimum length. It
// returns ok=false for anything that would produce an unsound channel.
func decodeSecret(raw string) ([]byte, bool) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil || len(b) < handshake.MinSecretSize {
		return nil, false
	}
	return b, true
}

// writeSecret stores the secret 0600, creating ~/.vior at 0700. The write goes
// to a temp file first so an interrupted write cannot leave a truncated secret
// that would be silently regenerated on next start.
func writeSecret(path string, secret []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(encoded), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mustNewSecret generates a secret, falling back to a panic only if the
// system CSPRNG fails — at which point nothing else in the process is
// trustworthy either.
func mustNewSecret() []byte {
	s, err := handshake.NewSecret()
	if err != nil {
		// crypto/rand failure is not a recoverable condition for a
		// security boundary; refusing to run beats running insecurely.
		panic("stream: cannot generate channel secret: " + err.Error())
	}
	return s
}

// ChannelSecret returns a copy of the active bootstrap secret.
func ChannelSecret() []byte {
	channelSecretMu.RLock()
	defer channelSecretMu.RUnlock()
	return append([]byte(nil), channelSecret...)
}

// ChannelSecretParam returns the secret encoded for carriage in a QR payload
// or URL. base64url without padding keeps it URL-safe and free of characters
// that would need escaping.
func ChannelSecretParam() string {
	channelSecretMu.RLock()
	defer channelSecretMu.RUnlock()
	return base64.RawURLEncoding.EncodeToString(channelSecret)
}

// SecretMatches reports whether the supplied encoded secret equals the active
// one, in constant time. Used by callers that need to validate a
// client-supplied secret without leaking it through timing.
func SecretMatches(encoded string) bool {
	want := ChannelSecretParam()
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(encoded)), []byte(want)) == 1
}

// RotateSecret generates a fresh secret and persists it, invalidating every
// previously issued QR code. This is the "revoke all devices" action.
func RotateSecret() error {
	s := mustNewSecret()
	channelSecretMu.Lock()
	channelSecret = s
	channelSecretMu.Unlock()

	path := secretFilePath()
	if path == "" {
		return nil
	}
	return writeSecret(path, s)
}
