package stream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnablePersistedPairLoadsOverride: when ~/.vior/pair.txt contains
// a valid override (4-8 digits) EnablePersistedPair must replace the
// in-memory pair code with it.
func TestEnablePersistedPairLoadsOverride(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	pairCodeMu.Lock()
	saved := pairCode
	pairCodeMu.Unlock()
	defer func() {
		pairCodeMu.Lock()
		pairCode = saved
		pairCodeMu.Unlock()
	}()

	dir := filepath.Join(os.Getenv("HOME"), ".vior")
	_ = os.MkdirAll(dir, 0o700)
	if err := os.WriteFile(filepath.Join(dir, "pair.txt"), []byte("4242\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	EnablePersistedPair()
	if got := PairCode(); got != "4242" {
		t.Fatalf("PairCode() = %q want %q", got, "4242")
	}
}

// TestEnablePersistedPairKeepsDerivedWhenNoOverride: the file should
// NOT be auto-created. Deleting pair.txt always falls back to the
// machine-derived value (the user's stable "phone number").
func TestEnablePersistedPairKeepsDerivedWhenNoOverride(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	pairCodeMu.Lock()
	saved := pairCode
	pairCode = derivePair()
	derived := pairCode
	pairCodeMu.Unlock()
	defer func() {
		pairCodeMu.Lock()
		pairCode = saved
		pairCodeMu.Unlock()
	}()

	EnablePersistedPair()
	if got := PairCode(); got != derived {
		t.Fatalf("PairCode() = %q want derived %q", got, derived)
	}

	// And pair.txt must NOT have been created — that's the whole point
	// of the new behaviour. A user nuking ~/.vior keeps the same
	// derived code on next launch.
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".vior", "pair.txt")); !os.IsNotExist(err) {
		t.Fatalf("pair.txt should not exist when no override is set, err=%v", err)
	}
}

// TestEnablePersistedPairIgnoresGarbage: a corrupt pair.txt (non-digit
// or wrong length) is ignored — derived value stays in force, file is
// NOT rewritten.
func TestEnablePersistedPairIgnoresGarbage(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	pairCodeMu.Lock()
	saved := pairCode
	pairCode = "9999"
	pairCodeMu.Unlock()
	defer func() {
		pairCodeMu.Lock()
		pairCode = saved
		pairCodeMu.Unlock()
	}()

	pairPath := filepath.Join(os.Getenv("HOME"), ".vior", "pair.txt")
	_ = os.MkdirAll(filepath.Dir(pairPath), 0o700)
	if err := os.WriteFile(pairPath, []byte("not-a-pin"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	EnablePersistedPair()
	if got := PairCode(); got != "9999" {
		t.Fatalf("garbage pair file shouldn't override in-memory code; got %q", got)
	}
	// File should be left exactly as-is.
	b, _ := os.ReadFile(pairPath)
	if strings.TrimSpace(string(b)) != "not-a-pin" {
		t.Fatalf("garbage file was modified, file=%q", b)
	}
}

// TestSetPairCodePersists writes a user override, then verifies a
// subsequent EnablePersistedPair re-loads it (mimicking a process
// restart).
func TestSetPairCodePersists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	pairCodeMu.Lock()
	saved := pairCode
	pairCodeMu.Unlock()
	defer func() {
		pairCodeMu.Lock()
		pairCode = saved
		pairCodeMu.Unlock()
	}()

	if err := SetPairCode("123456"); err != nil {
		t.Fatalf("SetPairCode: %v", err)
	}
	if got := PairCode(); got != "123456" {
		t.Fatalf("PairCode() = %q want %q", got, "123456")
	}

	// Simulate restart: blow away in-memory, reload from disk.
	pairCodeMu.Lock()
	pairCode = derivePair()
	pairCodeMu.Unlock()
	EnablePersistedPair()
	if got := PairCode(); got != "123456" {
		t.Fatalf("after restart PairCode() = %q want %q", got, "123456")
	}
}

// TestSetPairCodeValidation rejects non-numeric / too-short / too-long
// values without touching the on-disk state.
func TestSetPairCodeValidation(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	for _, bad := range []string{"abc", "12", "12a4", "123456789"} {
		if err := SetPairCode(bad); err == nil {
			t.Errorf("SetPairCode(%q) should have failed", bad)
		}
	}
}

// TestSetPairCodeEmptyClears: passing "" wipes the override and falls
// back to the machine-derived code.
func TestSetPairCodeEmptyClears(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	pairCodeMu.Lock()
	saved := pairCode
	pairCodeMu.Unlock()
	defer func() {
		pairCodeMu.Lock()
		pairCode = saved
		pairCodeMu.Unlock()
	}()

	if err := SetPairCode("4242"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SetPairCode(""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := PairCode(); got != derivePair() {
		t.Fatalf("expected derived code after clear, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".vior", "pair.txt")); !os.IsNotExist(err) {
		t.Fatalf("pair.txt should be gone after clear, err=%v", err)
	}
}

// TestDerivePairIsStable: derivePair() must return the same value on
// repeated calls (the machineID memoiser sees to this).
func TestDerivePairIsStable(t *testing.T) {
	a := derivePair()
	b := derivePair()
	if a != b {
		t.Fatalf("derivePair drifted: %q vs %q", a, b)
	}
	if len(a) != pairCodeDigits {
		t.Fatalf("derivePair length = %d want %d", len(a), pairCodeDigits)
	}
	for _, c := range a {
		if c < '0' || c > '9' {
			t.Fatalf("derivePair returned non-digit %q", a)
		}
	}
}
