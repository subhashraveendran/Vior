package trust

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestAddAndIsTrustedRoundtrip verifies Add persists across a fresh
// New() — i.e. the on-disk file is actually readable by a second
// process starting up.
func TestAddAndIsTrustedRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "trusted.json")
	s := New(path)
	if err := s.Add("dev-1", "iPhone 17"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !s.IsTrusted("dev-1") {
		t.Fatalf("expected dev-1 trusted in-memory")
	}
	// Reload from disk.
	s2 := New(path)
	if !s2.IsTrusted("dev-1") {
		t.Fatalf("expected dev-1 trusted after reload")
	}
}

// TestFilePermissions verifies the trust file is 0600 and the parent
// dir is 0700 — both critical so a co-tenant on the same machine
// can't read the trusted device list.
func TestFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX perms not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "trusted.json")
	s := New(path)
	if err := s.Add("dev-1", "Pixel"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
}

// TestCorruptFileDoesNotCrash exercises the case where the JSON file
// was truncated mid-write or hand-edited into garbage. The store must
// log + start empty, NOT panic, and must quarantine the bad file.
func TestCorruptFileDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s := New(path)
	if s.IsTrusted("anything") {
		t.Fatalf("corrupt file should produce empty store")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("expected quarantine file at %s.corrupt: %v", path, err)
	}
	// And we should still be writable.
	if err := s.Add("dev-x", "phone"); err != nil {
		t.Fatalf("Add after corrupt: %v", err)
	}
	if !s.IsTrusted("dev-x") {
		t.Fatalf("Add after corrupt didn't take")
	}
}

// TestAtomicWrite checks that no .tmp file is left behind after Add
// completes — the rename should have absorbed it.
func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	s := New(path)
	if err := s.Add("dev-1", "Pixel"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file lingered: %v", err)
	}
}

// TestForgetRemoves verifies Forget actually deletes the entry both
// in memory and on disk.
func TestForgetRemoves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	s := New(path)
	_ = s.Add("dev-1", "a")
	_ = s.Add("dev-2", "b")
	if err := s.Forget("dev-1"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if s.IsTrusted("dev-1") {
		t.Fatalf("dev-1 still trusted after Forget")
	}
	s2 := New(path)
	if s2.IsTrusted("dev-1") {
		t.Fatalf("dev-1 still trusted after reload")
	}
	if !s2.IsTrusted("dev-2") {
		t.Fatalf("dev-2 lost during Forget")
	}
}
