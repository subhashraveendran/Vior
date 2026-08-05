package filetransfer

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subhashraveendran/vior/internal/protocol"
)

// TestSanitizeFilenameTraversal: the classic attack — a sender claims
// name "../etc/passwd". The output must not contain any path separator
// AND must not contain ".." after sanitization.
func TestSanitizeFilenameTraversal(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"..\\..\\windows\\system32\\drivers\\etc\\hosts",
		"foo/../../bar",
		"./.bashrc",
	}
	for _, in := range cases {
		got := sanitizeFilename(in)
		if strings.ContainsAny(got, "/\\") {
			t.Errorf("sanitize(%q) = %q contains separator", in, got)
		}
		if strings.Contains(got, "..") {
			t.Errorf("sanitize(%q) = %q contains ..", in, got)
		}
	}
}

func TestSanitizeFilenameControlChars(t *testing.T) {
	in := "ev\x00il\x01name\x7f.txt"
	got := sanitizeFilename(in)
	for _, r := range got {
		if r == 0 || r < 0x20 || r == 0x7f {
			t.Errorf("sanitize left control char %q in %q", r, got)
		}
	}
}

func TestSanitizeFilenameDotsAndDashes(t *testing.T) {
	if got := sanitizeFilename(".bashrc"); strings.HasPrefix(got, ".") {
		t.Errorf("expected leading dot stripped, got %q", got)
	}
	if got := sanitizeFilename("-rf"); strings.HasPrefix(got, "-") {
		t.Errorf("expected leading dash stripped, got %q", got)
	}
}

func TestSanitizeFilenameEmpty(t *testing.T) {
	if got := sanitizeFilename(""); got == "" {
		t.Errorf("expected fallback name, got empty")
	}
	if got := sanitizeFilename("..."); got == "" {
		t.Errorf("expected fallback name for all-dots, got empty")
	}
}

func TestSanitizeFilenameWindowsReserved(t *testing.T) {
	if got := sanitizeFilename("CON"); got == "CON" || got == "con" {
		t.Errorf("expected CON to be prefixed, got %q", got)
	}
	if got := sanitizeFilename("nul.txt"); strings.EqualFold(got, "nul.txt") {
		t.Errorf("expected nul.txt to be prefixed, got %q", got)
	}
}

func TestSanitizeFilenameLength(t *testing.T) {
	long := strings.Repeat("a", 400) + ".txt"
	got := sanitizeFilename(long)
	if len(got) > 255 {
		t.Errorf("sanitize length = %d, want <=255", len(got))
	}
	if !strings.HasSuffix(got, ".txt") {
		t.Errorf("sanitize dropped extension: %q", got)
	}
}

// TestOfferDownloadRejectsOversize verifies the HTTP-download path
// refuses to register a file larger than MaxDownloadSize.
func TestOfferDownloadRejectsOversize(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Use Truncate to create a sparse file the size of MaxDownloadSize+1.
	if err := f.Truncate(MaxDownloadSize + 1); err != nil {
		t.Skipf("filesystem rejected truncate to %d bytes: %v", MaxDownloadSize+1, err)
	}
	f.Close()
	m := NewManager(dir)
	if _, err := m.OfferDownload(big); err == nil {
		t.Fatalf("expected error for oversize file")
	}
}

// TestHandleOfferRejectsOversize: the WS-chunked upload path was
// previously unbounded. Now an offer >MaxDownloadSize must be rejected
// before any disk file is opened.
func TestHandleOfferRejectsOversize(t *testing.T) {
	var sent []protocol.MessageType
	m := NewManager(t.TempDir())
	m.Send = func(t protocol.MessageType, _ any) error {
		sent = append(sent, t)
		return nil
	}
	m.HandleOffer(&protocol.FileOfferMessage{
		ID:   "aaaaaaaa",
		Name: "huge.bin",
		Size: MaxDownloadSize + 1,
	})
	if len(sent) != 1 || sent[0] != protocol.MsgFileReject {
		t.Fatalf("expected single file-reject, got %v", sent)
	}
	if m.GetTransfer("aaaaaaaa") != nil {
		t.Fatalf("oversize offer must not register a transfer")
	}
}

// TestHandleChunkOverrunStopsWriting: even if the sender lied about
// Size, chunks past the advertised total must be dropped instead of
// growing the file unboundedly.
func TestHandleChunkOverrunStopsWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows holds the file lock past Close() long enough that t.TempDir cleanup fails; logic is platform-agnostic and verified on linux/darwin")
	}
	dir := t.TempDir()
	m := NewManager(dir)
	m.Send = func(protocol.MessageType, any) error { return nil }
	// Honest 10-byte offer.
	m.HandleOffer(&protocol.FileOfferMessage{ID: "bbbbbbbb", Name: "a.bin", Size: 10})
	if err := m.AcceptFile("bbbbbbbb"); err != nil {
		t.Fatalf("AcceptFile: %v", err)
	}
	// Honest chunk.
	m.HandleChunk(&protocol.FileChunkMessage{ID: "bbbbbbbb", Offset: 0, Data: "AAAAAAAAAA=="}) // ~10 bytes
	// Dishonest extra chunk.
	m.HandleChunk(&protocol.FileChunkMessage{ID: "bbbbbbbb", Offset: 10, Data: "BBBBBBBBBBBB"})
	t2 := m.GetTransfer("bbbbbbbb")
	if t2.Transferred > 10 {
		t.Errorf("Transferred=%d, expected ≤10 after overrun guard", t2.Transferred)
	}
	// Release the open file handle so Windows TempDir cleanup can unlink it.
	t.Cleanup(func() {
		t2.mu.Lock()
		if t2.file != nil {
			t2.file.Close()
			t2.file = nil
		}
		t2.mu.Unlock()
	})
}

// TestHandleChunkFiresProgress: a receive that crosses the
// progressEmitStep boundary must trigger OnFileProgress at least once
// before file-complete. Guards against the "bar only fills at the end"
// UX regression on the desktop side.
func TestHandleChunkFiresProgress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file-lock quirk in t.TempDir cleanup; logic is platform-agnostic")
	}
	dir := t.TempDir()
	m := NewManager(dir)
	m.Send = func(protocol.MessageType, any) error { return nil }
	var calls int32
	m.OnFileProgress = func(*Transfer) { atomic.AddInt32(&calls, 1) }

	// Offer a file larger than progressEmitStep so a single chunk
	// payload triggers the boundary.
	total := int64(progressEmitStep + ChunkSize)
	m.HandleOffer(&protocol.FileOfferMessage{ID: "cccccccc", Name: "big.bin", Size: total})
	if err := m.AcceptFile("cccccccc"); err != nil {
		t.Fatalf("AcceptFile: %v", err)
	}

	// Stream raw bytes in ChunkSize segments.
	payload := make([]byte, ChunkSize)
	for off := int64(0); off < total; off += int64(len(payload)) {
		end := int64(len(payload))
		if off+end > total {
			end = total - off
		}
		m.HandleChunk(&protocol.FileChunkMessage{
			ID:     "cccccccc",
			Offset: off,
			Data:   base64.StdEncoding.EncodeToString(payload[:end]),
		})
	}
	// OnFileProgress fires in a goroutine — give it a beat.
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&calls) == 0 {
		t.Errorf("expected at least one OnFileProgress call, got 0")
	}

	// Release any remaining file handle.
	t.Cleanup(func() {
		got := m.GetTransfer("cccccccc")
		if got == nil {
			return
		}
		got.mu.Lock()
		if got.file != nil {
			got.file.Close()
			got.file = nil
		}
		got.mu.Unlock()
	})
}

// TestAcceptFileBlocksTraversalEvenAfterSanitize is paranoia — confirm
// the post-sanitize abs-path guard in AcceptFile actually catches an
// attempt to escape ReceiveDir via a creative name. The sanitizer
// already strips most attacks; this verifies the second layer.
func TestAcceptFileWritesInsideReceiveDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows holds the file lock past Close() long enough that t.TempDir cleanup fails; logic is platform-agnostic and verified on linux/darwin")
	}
	dir := t.TempDir()
	m := NewManager(dir)
	m.Send = func(protocol.MessageType, any) error { return nil }
	m.HandleOffer(&protocol.FileOfferMessage{ID: "dddddddd", Name: "../escape.txt", Size: 4})
	if err := m.AcceptFile("dddddddd"); err != nil {
		t.Fatalf("AcceptFile rejected legitimate-after-sanitize name: %v", err)
	}
	got := m.GetTransfer("dddddddd")
	if !strings.HasPrefix(got.Path, dir) {
		t.Errorf("Path %q is not inside ReceiveDir %q", got.Path, dir)
	}
	// Release the open file handle so Windows TempDir cleanup can unlink it.
	t.Cleanup(func() {
		got.mu.Lock()
		if got.file != nil {
			got.file.Close()
			got.file = nil
		}
		got.mu.Unlock()
	})
}
