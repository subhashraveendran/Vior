package filetransfer

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

// TestOutOfOrderChunks sends chunks out of order and verifies file corruption
func TestOutOfOrderChunks(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.Send = func(protocol.MessageType, any) error { return nil }

	// Offer a file with 3 chunks (total 30 bytes)
	m.HandleOffer(&protocol.FileOfferMessage{ID: "test", Name: "test.txt", Size: 30})
	if err := m.AcceptFile("test"); err != nil {
		t.Fatalf("AcceptFile: %v", err)
	}

	// Create chunk data: "CHUNK1" (6 bytes each)
	// If sent in order: "CHUNK1" + "CHUNK2" + "CHUNK3"
	// If sent out of order: "CHUNK2" + "CHUNK1" + "CHUNK3"

	chunk1 := base64.StdEncoding.EncodeToString([]byte("CHUNK1"))
	chunk2 := base64.StdEncoding.EncodeToString([]byte("CHUNK2"))
	chunk3 := base64.StdEncoding.EncodeToString([]byte("CHUNK3"))

	// Send chunks out of order: 2, 1, 3
	// Offset claims are: 6, 0, 12 (but receiver ignores them)
	m.HandleChunk(&protocol.FileChunkMessage{ID: "test", Offset: 6, Data: chunk2})
	m.HandleChunk(&protocol.FileChunkMessage{ID: "test", Offset: 0, Data: chunk1})
	m.HandleChunk(&protocol.FileChunkMessage{ID: "test", Offset: 12, Data: chunk3})

	// Compute what the hash would be for out-of-order data
	h := sha256.New()
	h.Write([]byte("CHUNK2"))
	h.Write([]byte("CHUNK1"))
	h.Write([]byte("CHUNK3"))
	badHash := hex.EncodeToString(h.Sum(nil))

	// Send complete with the bad hash (matching the out-of-order data)
	// This will succeed because the receiver's incremental hash matches!
	m.HandleComplete(&protocol.FileCompleteMessage{ID: "test", Hash: badHash})

	tf := m.GetTransfer("test")
	if !tf.Complete {
		t.Errorf("Expected transfer to be marked complete")
	}

	// Verify the file contents are actually corrupted (out of order)
	data, err := os.ReadFile(tf.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(data) != "CHUNK2CHUNK1CHUNK3" {
		t.Errorf("File is not corrupted as expected: got %q", string(data))
	}

	if string(data) == "CHUNK1CHUNK2CHUNK3" {
		t.Errorf("File was not corrupted - this should have failed!")
	}

	t.Logf("SUCCESS: File was corrupted to %q (should be CHUNK1CHUNK2CHUNK3)", string(data))
}
