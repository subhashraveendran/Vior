package stream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnablePersistedPairRoundtrip writes a pair file the first time,
// loads it back the second time. Uses HOME override so the production
// path isn't touched.
func TestEnablePersistedPairRoundtrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Capture and restore current pair code so other tests aren't
	// disturbed by our mutation.
	saved := pairCode
	defer func() { pairCode = saved }()

	pairCode = "AAA111"
	EnablePersistedPair()

	path := filepath.Join(os.Getenv("HOME"), ".vior", "pair.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected pair.txt to be written: %v", err)
	}
	if strings.TrimSpace(string(b)) != "AAA111" {
		t.Fatalf("pair.txt = %q want %q", b, "AAA111")
	}

	// Simulate a server restart by changing the in-memory code, then
	// reloading — EnablePersistedPair should pull the on-disk value
	// back in.
	pairCode = "ZZZ999"
	EnablePersistedPair()
	if pairCode != "AAA111" {
		t.Fatalf("after reload pairCode = %q want AAA111", pairCode)
	}
}

// TestEnablePersistedPairIgnoresGarbage: a half-written or corrupt
// pair.txt should not poison the server's pair code. It should
// regenerate and write a fresh one.
func TestEnablePersistedPairIgnoresGarbage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saved := pairCode
	defer func() { pairCode = saved }()

	pairPath := filepath.Join(os.Getenv("HOME"), ".vior", "pair.txt")
	_ = os.MkdirAll(filepath.Dir(pairPath), 0o700)
	if err := os.WriteFile(pairPath, []byte("xy"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pairCode = "BBB222"
	EnablePersistedPair()
	if pairCode != "BBB222" {
		t.Fatalf("garbage pair file shouldn't override in-memory code; got %q", pairCode)
	}
	// And it should now persist the in-memory value.
	b, _ := os.ReadFile(pairPath)
	if strings.TrimSpace(string(b)) != "BBB222" {
		t.Fatalf("expected regen + persist, file=%q", b)
	}
}
