package filetransfer

import (
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

// offerAndAccept drives a transfer to the point where its file handle is open
// and chunks are being written — the state a client can park it in and then
// walk away from.
func offerAndAccept(t *testing.T, m *Manager, id string) {
	t.Helper()
	m.HandleOffer(&protocol.FileOfferMessage{
		ID:       id,
		Name:     id + ".bin",
		Size:     1024,
		MimeType: "application/octet-stream",
	})
	if err := m.AcceptFile(id); err != nil {
		t.Fatalf("AcceptFile(%s): %v", id, err)
	}
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
