package stream

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// The whole point: the code must not be derivable from the machine ID.
//
// It used to be SHA-256("vior-pair:" + machineID) with nothing secret in the
// input, so anyone who read the machine ID — unprivileged on macOS and Linux —
// could compute the code offline instead of guessing it. This reproduces the
// old derivation and requires that it no longer predicts the new one.
func TestPairCodeIsNotDerivableFromMachineIDAlone(t *testing.T) {
	const machineID = "11111111-2222-3333-4444-555555555555"

	// The pre-fix derivation, reproduced verbatim.
	legacy := func(id string) string {
		sum := sha256.Sum256([]byte("vior-pair:" + id))
		hexed := hex.EncodeToString(sum[:])
		var b strings.Builder
		for _, c := range hexed {
			if c >= '0' && c <= '9' {
				b.WriteByte(byte(c))
				if b.Len() == pairCodeDigits {
					return b.String()
				}
			}
		}
		return b.String()
	}

	secret := mustNewPairSecret()
	got := derivePairWith(secret, machineID)
	if got == legacy(machineID) {
		t.Fatal("the pair code still matches the unkeyed machine-ID derivation — an attacker who reads the machine ID can compute it")
	}
}

// Different installs on identical hardware must not share a code.
func TestPairCodeDiffersPerSecret(t *testing.T) {
	const machineID = "same-machine-id"

	seen := map[string]int{}
	const runs = 50
	for range runs {
		seen[derivePairWith(mustNewPairSecret(), machineID)]++
	}
	// With 10^6 possible codes and 50 draws, a collision is possible but
	// clustering is not. Anything under half distinct means the secret is
	// barely influencing the output.
	if len(seen) < runs/2 {
		t.Fatalf("only %d distinct codes from %d secrets — the key is not driving the derivation", len(seen), runs)
	}
}

// Same secret + same machine must be stable, or the user's code changes under
// them mid-session.
func TestPairCodeIsStableForAGivenSecret(t *testing.T) {
	secret := mustNewPairSecret()
	const machineID = "stable-machine"

	first := derivePairWith(secret, machineID)
	for range 20 {
		if got := derivePairWith(secret, machineID); got != first {
			t.Fatalf("derivation is not deterministic: %q then %q", first, got)
		}
	}
}

// A different machine with the same secret must still differ, so the machine
// ID is not being ignored.
func TestPairCodeDependsOnMachineID(t *testing.T) {
	secret := mustNewPairSecret()
	a := derivePairWith(secret, "machine-a")
	b := derivePairWith(secret, "machine-b")
	if a == b {
		t.Errorf("two machine ids produced the same code (%q) — the id is not part of the input", a)
	}
}

// Shape contract: the UI and the pair_ratelimit path both assume exactly
// pairCodeDigits decimal characters.
func TestPairCodeShape(t *testing.T) {
	for range 200 {
		code := derivePairWith(mustNewPairSecret(), "shape-check")
		if len(code) != pairCodeDigits {
			t.Fatalf("code %q has %d digits, want %d", code, len(code), pairCodeDigits)
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Fatalf("code %q contains a non-digit", code)
			}
		}
	}
}

// The secret must survive its stored encoding, and anything too short to key
// an HMAC properly must be refused rather than silently accepted.
func TestPairSecretEncoding(t *testing.T) {
	secret := mustNewPairSecret()
	if len(secret) != pairSecretSize {
		t.Fatalf("generated secret is %d bytes, want %d", len(secret), pairSecretSize)
	}

	path := t.TempDir() + "/pair-secret"
	if err := writePairSecret(path, secret); err != nil {
		t.Fatalf("writePairSecret: %v", err)
	}

	rawBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	raw := string(rawBytes)
	got, ok := decodePairSecret(raw)
	if !ok {
		t.Fatal("stored secret failed to decode")
	}
	if string(got) != string(secret) {
		t.Fatal("secret did not survive the round trip")
	}

	for _, bad := range []string{"", "short", "!!!not-base64!!!", "aGVsbG8"} {
		if _, ok := decodePairSecret(bad); ok {
			t.Errorf("decodePairSecret(%q) accepted an unusable secret", bad)
		}
	}
}

// Two generated secrets must not be equal.
func TestPairSecretIsFresh(t *testing.T) {
	if string(mustNewPairSecret()) == string(mustNewPairSecret()) {
		t.Fatal("two generated pair secrets were identical")
	}
}

// The active install must have usable keying material.
func TestActivePairSecretIsUsable(t *testing.T) {
	if got := len(currentPairSecret()); got < pairSecretSize {
		t.Fatalf("active pair secret is %d bytes, want >= %d", got, pairSecretSize)
	}
}
