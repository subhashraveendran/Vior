package filetransfer

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

func receiveOne(t *testing.T, m *Manager, id, payload string) string {
	t.Helper()
	m.HandleOffer(&protocol.FileOfferMessage{ID: id, Name: id + ".txt", Size: int64(len(payload))})
	if err := m.AcceptFile(id); err != nil {
		t.Fatalf("AcceptFile: %v", err)
	}
	m.HandleChunk(&protocol.FileChunkMessage{ID: id, Offset: 0, Data: base64.StdEncoding.EncodeToString([]byte(payload))})
	tf := m.GetTransfer(id)
	if tf == nil {
		t.Fatalf("transfer %s missing after chunk", id)
	}
	return tf.Path
}

// TestCompleteWithoutHashIsKeptUnverified covers the sender that cannot
// hash (a browser client on an insecure origin has no WebCrypto): hash ""
// must complete the transfer as unverified, not delete the file as a
// mismatch. The mobile client sent "" unconditionally, so before this
// behaviour every phone→desktop transfer was deleted on arrival.
func TestCompleteWithoutHashIsKeptUnverified(t *testing.T) {
	m := NewManager(t.TempDir())
	m.Send = func(protocol.MessageType, any) error { return nil }
	const id = "bbbbbbbb"
	path := receiveOne(t, m, id, "hello")

	m.HandleComplete(&protocol.FileCompleteMessage{ID: id, Hash: ""})

	tf := m.GetTransfer(id)
	if tf == nil || !tf.Complete {
		t.Fatalf("transfer dropped or incomplete after empty-hash complete")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file removed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("contents = %q, want hello", data)
	}
	want := sha256.Sum256([]byte("hello"))
	if tf.Hash != hex.EncodeToString(want[:]) {
		t.Fatalf("Hash = %q, want the computed digest", tf.Hash)
	}
}

// TestWrongHashStillRejected guards the other side of the change: a
// non-empty hash that does not match still deletes the file.
func TestWrongHashStillRejected(t *testing.T) {
	m := NewManager(t.TempDir())
	m.Send = func(protocol.MessageType, any) error { return nil }
	const id = "cccccccc"
	path := receiveOne(t, m, id, "hello")

	m.HandleComplete(&protocol.FileCompleteMessage{ID: id, Hash: "deadbeef"})

	if m.GetTransfer(id) != nil {
		t.Fatalf("transfer kept after hash mismatch")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("corrupt file not removed (stat err = %v)", err)
	}
}

// TestServeDownloadOutsideReceiveDir: desktop→phone must work for a file
// picked from anywhere. ServeDownload used to require the file to sit
// inside ReceiveDir, which 403'd every normal pick.
func TestServeDownloadOutsideReceiveDir(t *testing.T) {
	m := NewManager(t.TempDir())
	src := t.TempDir()
	path := filepath.Join(src, "report.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := m.OfferDownload(path)
	if err != nil {
		t.Fatalf("OfferDownload: %v", err)
	}

	rr := httptest.NewRecorder()
	m.ServeDownload(rr, httptest.NewRequest("GET", "/download/"+p.ID, nil), p.ID)

	if rr.Code != 200 {
		t.Fatalf("status = %d, want 200 (body %q)", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "%PDF-1.4 test" {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

// TestServeDownloadRefusesSwappedPath: the authorization is "this exact
// file was offered". Replacing it with a symlink after the offer must not
// redirect the download to the link target.
func TestServeDownloadRefusesSwappedPath(t *testing.T) {
	m := NewManager(t.TempDir())
	src := t.TempDir()
	path := filepath.Join(src, "pick.txt")
	secret := filepath.Join(src, "secret.txt")
	if err := os.WriteFile(path, []byte("picked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := m.OfferDownload(path)
	if err != nil {
		t.Fatalf("OfferDownload: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, path); err != nil {
		t.Skipf("symlink unsupported here: %v", err)
	}

	rr := httptest.NewRecorder()
	m.ServeDownload(rr, httptest.NewRequest("GET", "/download/"+p.ID, nil), p.ID)

	if rr.Code != 403 {
		t.Fatalf("status = %d, want 403 (body %q)", rr.Code, rr.Body.String())
	}
}

// TestIncompleteTransferRejected: a complete that arrives before every
// advertised byte was written must not mark the transfer received — with
// or without a sender hash. HandleChunk drops chunks silently on write
// errors and overshoots, so the byte count is the only signal left when
// the sender cannot hash.
func TestIncompleteTransferRejected(t *testing.T) {
	for _, hash := range []string{"", "matches-partial"} {
		m := NewManager(t.TempDir())
		m.Send = func(protocol.MessageType, any) error { return nil }
		const id = "dddddddd"
		m.HandleOffer(&protocol.FileOfferMessage{ID: id, Name: "big.bin", Size: 10})
		if err := m.AcceptFile(id); err != nil {
			t.Fatalf("AcceptFile: %v", err)
		}
		m.HandleChunk(&protocol.FileChunkMessage{ID: id, Offset: 0, Data: base64.StdEncoding.EncodeToString([]byte("hello"))})
		path := m.GetTransfer(id).Path

		h := hash
		if h == "matches-partial" {
			sum := sha256.Sum256([]byte("hello"))
			h = hex.EncodeToString(sum[:])
		}
		m.HandleComplete(&protocol.FileCompleteMessage{ID: id, Hash: h})

		if m.GetTransfer(id) != nil {
			t.Fatalf("hash=%q: incomplete transfer (5 of 10 bytes) was kept", hash)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("hash=%q: partial file left on disk (stat err = %v)", hash, err)
		}
	}
}
