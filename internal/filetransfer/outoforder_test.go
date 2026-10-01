package filetransfer

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

// TestOutOfOrderChunkRejected guards the Offset == Transferred check in
// HandleChunk: a chunk whose offset does not match the bytes received so
// far must be dropped without advancing the transfer, and the in-order
// delivery that follows must still complete with the correct contents.
//
// This replaces a file that was named test_out_of_order.go (no _test
// suffix), so it was compiled into the production binary and never ran —
// and it asserted the pre-fix behaviour, that out-of-order chunks corrupt
// the file.
func TestOutOfOrderChunkRejected(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.Send = func(protocol.MessageType, any) error { return nil }

	const id = "aaaaaaaa"
	m.HandleOffer(&protocol.FileOfferMessage{ID: id, Name: "test.txt", Size: 18})
	if err := m.AcceptFile(id); err != nil {
		t.Fatalf("AcceptFile: %v", err)
	}
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

	// Chunk 2 arrives before chunk 1: must be ignored.
	m.HandleChunk(&protocol.FileChunkMessage{ID: id, Offset: 6, Data: enc("CHUNK2")})
	if got := m.GetTransfer(id).Transferred; got != 0 {
		t.Fatalf("out-of-order chunk advanced Transferred to %d, want 0", got)
	}

	// In-order delivery completes normally.
	m.HandleChunk(&protocol.FileChunkMessage{ID: id, Offset: 0, Data: enc("CHUNK1")})
	m.HandleChunk(&protocol.FileChunkMessage{ID: id, Offset: 6, Data: enc("CHUNK2")})
	m.HandleChunk(&protocol.FileChunkMessage{ID: id, Offset: 12, Data: enc("CHUNK3")})

	sum := sha256.Sum256([]byte("CHUNK1CHUNK2CHUNK3"))
	m.HandleComplete(&protocol.FileCompleteMessage{ID: id, Hash: hex.EncodeToString(sum[:])})

	tf := m.GetTransfer(id)
	if !tf.Complete {
		t.Fatalf("transfer not complete after in-order delivery")
	}
	data, err := os.ReadFile(tf.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "CHUNK1CHUNK2CHUNK3" {
		t.Fatalf("file contents = %q, want in-order payload", data)
	}
}
