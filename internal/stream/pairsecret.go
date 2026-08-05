package stream

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The pair code used to be SHA-256("vior-pair:" + machineID), with nothing
// secret in the input. Machine IDs are not secrets: macOS hands out the
// IOPlatformUUID to any unprivileged `ioreg` call, and /etc/machine-id is
// world-readable on most Linux installs. An attacker who could read it — or who
// obtained it from a screen share, a log, or a support bundle — could compute
// the pair code offline and connect without ever guessing.
//
// That defeats the entire point of the code, which is the admission secret for
// the WebSocket. A brute-force throttle does not help when no brute force is
// required.
//
// The code is now an HMAC keyed by a random per-install secret, so knowing the
// machine ID reveals nothing.
//
// # Cost of the fix
//
// The pair code was designed to be stable — a "phone number" the user memorises
// once, surviving reinstalls and a ~/.vior wipe, because the machine ID alone
// regenerated it. Keying it breaks that by construction:
//
//   - Every existing install gets a new code once, on first run after upgrade.
//   - Deleting ~/.vior/pair-secret now yields a different code rather than the
//     same one.
//
// That is the unavoidable price of making the code unpredictable, and it is
// worth paying: a memorable code an attacker can also compute is not an
// admission secret. Users who want a code they choose still have SetPairCode.

const pairSecretFileName = "pair-secret"

// pairSecretSize is the keying material length. 32 bytes is far more than the
// ~20 bits of entropy a 6-digit code can express, but the cost is nil and it
// leaves headroom if the code ever lengthens.
const pairSecretSize = 32

// Initialised at package load, before pairCode = derivePair() runs. Go
// initialises package-level variables in dependency order, so pairSecret is
// populated before derivePair reads it.
var (
	pairSecretMu sync.RWMutex
	pairSecret   = loadOrCreatePairSecret()
)

// pairSecretPath returns ~/.vior/pair-secret, or "" when there is no home
// directory.
func pairSecretPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".vior", pairSecretFileName)
}

// loadOrCreatePairSecret returns the persisted keying material, generating it
// on first run.
//
// With no home directory, or an unwritable ~/.vior, it degrades to an
// ephemeral per-process secret: the pair code then changes on every restart,
// which is disruptive but still safe. The alternative — falling back to the
// unkeyed derivation — would quietly restore the predictability this exists to
// remove, so it is not offered.
func loadOrCreatePairSecret() []byte {
	path := pairSecretPath()
	if path == "" {
		log.Printf("stream: no home dir — pair code will change on each restart")
		return mustNewPairSecret()
	}

	if b, err := os.ReadFile(path); err == nil {
		if s, ok := decodePairSecret(string(b)); ok {
			return s
		}
		log.Printf("stream: %s is invalid — regenerating (pair code will change)", path)
	} else if !os.IsNotExist(err) {
		log.Printf("stream: read %s failed: %v — regenerating", path, err)
	}

	s := mustNewPairSecret()
	if err := writePairSecret(path, s); err != nil {
		log.Printf("stream: persist pair secret failed: %v (pair code will change on restart)", err)
	}
	return s
}

// decodePairSecret parses a stored secret, rejecting anything short enough to
// weaken the keying.
func decodePairSecret(raw string) ([]byte, bool) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil || len(b) < pairSecretSize {
		return nil, false
	}
	return b, true
}

// writePairSecret stores the secret 0600 via temp+rename, so an interrupted
// write cannot leave a truncated file that would be silently regenerated —
// which would rotate the user's pair code for no reason.
func writePairSecret(path string, secret []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(encoded), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func mustNewPairSecret() []byte {
	b := make([]byte, pairSecretSize)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means nothing in this process is
		// trustworthy; refusing to run beats running insecurely.
		panic("stream: cannot generate pair secret: " + err.Error())
	}
	return b
}

// currentPairSecret returns a copy of the active keying material.
func currentPairSecret() []byte {
	pairSecretMu.RLock()
	defer pairSecretMu.RUnlock()
	return append([]byte(nil), pairSecret...)
}
