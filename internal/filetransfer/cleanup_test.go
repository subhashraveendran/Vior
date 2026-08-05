package filetransfer

import (
	"os"
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

// offerAndAccept drives a transfer to the point where its file handle is open
// and chunks are being written — the state a client can park it in and then
// walk away from.
// HandleOffer auto-accepts when OnFileOffer is nil, so the offer alone reaches
// the open-handle state. Accepting again here would be a *second* accept —
// which the manager now correctly ignores, but which previously orphaned a
// descriptor and is the bug TestRepeatAcceptDoesNotOpenASecondHandle covers.
func offerAndAccept(t *testing.T, m *Manager, id string) {
	t.Helper()
	m.HandleOffer(&protocol.FileOfferMessage{
		ID:       id,
		Name:     id + ".bin",
		Size:     1024,
		MimeType: "application/octet-stream",
	})
	m.mu.Lock()
	tr := m.transfers[id]
	m.mu.Unlock()
	if tr == nil {
		t.Fatalf("offer %s was not registered", id)
	}
	tr.mu.Lock()
	open := tr.file != nil
	tr.mu.Unlock()
	if !open {
		t.Fatalf("offer %s did not reach the open-handle state", id)
	}
}

// A repeat accept must not open a second handle.
//
// AcceptFile used to os.Create afresh every call and overwrite t.file,
// orphaning the previous descriptor and leaving a second empty file on disk
// via uniquePath. POSIX hides that — unlinking an open file succeeds — so it
// surfaced only as a Windows CI failure, where the stranded handle blocked
// t.TempDir() cleanup. Reachable in the product by double-tapping Accept.
func TestRepeatAcceptDoesNotOpenASecondHandle(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	offerAndAccept(t, m, "dup")

	m.mu.Lock()
	tr := m.transfers["dup"]
	m.mu.Unlock()
	tr.mu.Lock()
	firstHandle, firstPath := tr.file, tr.Path
	tr.mu.Unlock()

	for range 3 {
		if err := m.AcceptFile("dup"); err != nil {
			t.Fatalf("repeat AcceptFile: %v", err)
		}
	}

	tr.mu.Lock()
	sameHandle, samePath := tr.file == firstHandle, tr.Path == firstPath
	gotPath := tr.Path
	tr.mu.Unlock()

	if !sameHandle {
		t.Error("a repeat accept replaced the open handle, orphaning the original")
	}
	if !samePath {
		t.Errorf("a repeat accept changed the destination: %q -> %q", firstPath, gotPath)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("repeat accepts left %d files, want 1: %v", len(entries), names)
	}

	m.Cleanup()
}

// Cleanup must close descriptors AND drop the transfer entries.
//
// Closing alone released the file handle but left every Transfer struct
// reachable for the lifetime of the process, so a client that repeatedly
// offered a file, accepted it, sent a chunk and disconnected grew the map
// without bound. Both halves of that leak are asserted here.
func TestCleanupClosesFilesAndDropsTransfers(t *testing.T) {
	m := NewManager(t.TempDir())

	const count = 5
	for i := range count {
		offerAndAccept(t, m, string(rune('a'+i)))
	}

	m.mu.Lock()
	got := len(m.transfers)
	openBefore := 0
	for _, tr := range m.transfers {
		tr.mu.Lock()
		if tr.file != nil {
			openBefore++
		}
		tr.mu.Unlock()
	}
	m.mu.Unlock()

	if got != count {
		t.Fatalf("registered %d transfers, want %d", got, count)
	}
	if openBefore != count {
		t.Fatalf("%d transfers hold an open file, want %d — the test is not reaching the leaking state", openBefore, count)
	}

	m.Cleanup()

	m.mu.Lock()
	remaining := len(m.transfers)
	stillOpen := 0
	for _, tr := range m.transfers {
		tr.mu.Lock()
		if tr.file != nil {
			stillOpen++
		}
		tr.mu.Unlock()
	}
	m.mu.Unlock()

	if stillOpen != 0 {
		t.Errorf("%d file handles survived Cleanup", stillOpen)
	}
	if remaining != 0 {
		t.Errorf("Cleanup left %d transfer entries behind — descriptors were freed but the structs still leak", remaining)
	}
}

// Session teardown can race or repeat (an in-loop Bye plus the post-loop
// defer), so Cleanup has to be safe to call more than once.
func TestCleanupIsIdempotent(t *testing.T) {
	m := NewManager(t.TempDir())
	offerAndAccept(t, m, "x")

	m.Cleanup()
	m.Cleanup() // must not panic on already-closed files or a cleared map

	m.mu.Lock()
	remaining := len(m.transfers)
	m.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("transfers = %d after two Cleanups, want 0", remaining)
	}
}

// Cleanup on a manager that never saw a transfer must be a no-op, not a panic.
func TestCleanupOnEmptyManager(t *testing.T) {
	NewManager(t.TempDir()).Cleanup()
}
