package filetransfer

import (
	"strings"
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

func TestValidTransferID(t *testing.T) {
	valid := []string{
		"deadbeef",
		"0123456789abcdef",
		strings.Repeat("a", 64),
	}
	for _, id := range valid {
		if !validTransferID(id) {
			t.Errorf("validTransferID(%q) = false, want true", id)
		}
	}

	invalid := []struct {
		id  string
		why string
	}{
		{"", "empty — the reported collision key"},
		{"   ", "whitespace only"},
		{"z", "too short and not hex"},
		{"deadbee", "below the minimum length"},
		{strings.Repeat("a", 65), "above the maximum length"},
		{"DEADBEEF", "uppercase is not what generateID emits"},
		{"dead-beef", "punctuation"},
		{"../../etc/passwd", "path traversal shape"},
		{"dead beef", "embedded space"},
		{"__proto__", "prototype-pollution key"},
		{"dead\x00beef", "embedded NUL"},
	}
	for _, tc := range invalid {
		if validTransferID(tc.id) {
			t.Errorf("validTransferID(%q) = true, want false (%s)", tc.id, tc.why)
		}
	}
}

// The ids this package generates must pass its own filter.
func TestGeneratedIDsAreValid(t *testing.T) {
	for range 100 {
		if id := generateID(); !validTransferID(id) {
			t.Fatalf("generateID produced %q, which validTransferID rejects", id)
		}
	}
}

// An invalid id must never reach the transfers map.
func TestHandleOfferRejectsInvalidID(t *testing.T) {
	m := NewManager(t.TempDir())
	var rejected int
	m.Send = func(msgType protocol.MessageType, _ any) error {
		if msgType == protocol.MsgFileReject {
			rejected++
		}
		return nil
	}

	for _, id := range []string{"", "z", "__proto__", "../escape"} {
		m.HandleOffer(&protocol.FileOfferMessage{ID: id, Name: "x.bin", Size: 10})
	}

	m.mu.Lock()
	n := len(m.transfers)
	m.mu.Unlock()
	if n != 0 {
		t.Errorf("registered %d transfers from invalid ids, want 0", n)
	}
	if rejected != 4 {
		t.Errorf("sent %d rejections, want 4 — the peer must be told, not silently ignored", rejected)
	}
}

// A repeated id must not replace a live transfer.
//
// The second offer used to overwrite the first entry outright: the original
// Transfer was dropped while its file handle stayed open, and every subsequent
// chunk was written into the wrong file.
func TestHandleOfferRejectsDuplicateID(t *testing.T) {
	m := NewManager(t.TempDir())
	var rejected int
	m.Send = func(msgType protocol.MessageType, _ any) error {
		if msgType == protocol.MsgFileReject {
			rejected++
		}
		return nil
	}

	const id = "cafebabe"
	m.HandleOffer(&protocol.FileOfferMessage{ID: id, Name: "first.bin", Size: 10})

	m.mu.Lock()
	first := m.transfers[id]
	m.mu.Unlock()
	if first == nil {
		t.Fatal("first offer was not registered")
	}

	m.HandleOffer(&protocol.FileOfferMessage{ID: id, Name: "second.bin", Size: 20})

	m.mu.Lock()
	current := m.transfers[id]
	n := len(m.transfers)
	m.mu.Unlock()

	if current != first {
		t.Error("a duplicate id replaced the live transfer, orphaning its open handle")
	}
	if current.Name != "first.bin" {
		t.Errorf("transfer name = %q, want first.bin", current.Name)
	}
	if n != 1 {
		t.Errorf("transfers = %d, want 1", n)
	}
	if rejected != 1 {
		t.Errorf("sent %d rejections for the duplicate, want 1", rejected)
	}

	m.Cleanup()
}
