//go:build unix

package trust

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestFIFOInPlaceOfFileDoesNotBlock: opening a named pipe for reading
// blocks until a writer appears, which would hang server start-up. The
// loader must notice it is not a regular file and move it aside.
func TestFIFOInPlaceOfFileDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	captureLog(t)
	done := make(chan *Store, 1)
	go func() { done <- New(path) }()
	select {
	case s := <-done:
		if len(corruptBackups(t, path)) != 1 {
			t.Fatal("FIFO should be moved aside")
		}
		if err := s.Add("dev-1", "a"); err != nil {
			t.Fatalf("Add: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("New blocked opening a FIFO")
	}
}
