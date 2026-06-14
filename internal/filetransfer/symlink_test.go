package filetransfer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOfferDownloadResolvesSymlinks: a compromised desktop frontend
// that registers a symlinked path (e.g. /Downloads/innocent.txt →
// /etc/hosts) must not end up serving the symlink target. The
// resolved path is what we store; the UI's displayed Name keeps the
// original basename so it stays predictable.
func TestOfferDownloadResolvesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; verified on linux/darwin")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.bin")
	if err := os.WriteFile(target, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "bait.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	m := NewManager(dir)
	p, err := m.OfferDownload(link)
	if err != nil {
		t.Fatalf("OfferDownload(symlink): %v", err)
	}
	// macOS resolves /var → /private/var on its own; EvalSymlinks
	// returns the canonical form. Compare against the canonical
	// form of the target rather than the literal join we wrote above.
	wantPath, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval target: %v", err)
	}
	if p.Path != wantPath {
		t.Errorf("stored path = %q, want resolved %q", p.Path, wantPath)
	}
	if p.Name != "bait.txt" {
		t.Errorf("displayed name lost basename: got %q want bait.txt", p.Name)
	}
}

// TestOfferDownloadRejectsSymlinkToDir: a symlink that points at a
// directory must be rejected outright, not silently treated as a
// regular file.
func TestOfferDownloadRejectsSymlinkToDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	dir := t.TempDir()
	subdir := filepath.Join(dir, "stuff")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(dir, "bait.zip")
	if err := os.Symlink(subdir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	m := NewManager(dir)
	_, err := m.OfferDownload(link)
	if err == nil {
		t.Fatal("OfferDownload accepted a symlink-to-directory")
	}
	if !strings.Contains(err.Error(), "not a regular file") && !strings.Contains(err.Error(), "directories") {
		t.Errorf("unexpected error %q — should mention non-regular target", err.Error())
	}
}

// TestOfferDownloadBrokenSymlinkRejected: a dangling symlink must fail
// at EvalSymlinks rather than at open-time later, when the error
// would leak through the HTTP handler.
func TestOfferDownloadBrokenSymlinkRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling.txt")
	if err := os.Symlink(filepath.Join(dir, "does-not-exist"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	m := NewManager(dir)
	if _, err := m.OfferDownload(link); err == nil {
		t.Fatal("OfferDownload accepted a broken symlink")
	}
}
