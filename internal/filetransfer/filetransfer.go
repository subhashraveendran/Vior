// Package filetransfer handles bidirectional file transfer over WebSocket.
// Files are chunked, base64-encoded, and sent through the existing WS connection.
package filetransfer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/subhashraveendran/vior/internal/protocol"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"
)

const (
	// ChunkSize is the max bytes per file chunk (48KB base64 = ~64KB on wire).
	ChunkSize = 48 * 1024

	// MaxPreviewSize is the max thumbnail dimension.
	MaxPreviewSize = 200
)

// Transfer tracks a single file transfer in progress.
type Transfer struct {
	ID       string
	Name     string
	Size     int64
	MimeType string
	Preview  string // base64 thumbnail
	Path     string // local file path (for sending) or dest path (for receiving)

	// Progress.
	Transferred int64
	Complete    bool
	Hash        string

	// Receive buffer.
	file *os.File
	mu   sync.Mutex
}

// Manager handles file transfers for a session.
type Manager struct {
	// Active transfers by ID.
	transfers map[string]*Transfer
	mu        sync.Mutex

	// Where to save received files.
	ReceiveDir string

	// Sender function (inject the WS send).
	Send func(msgType protocol.MessageType, data any) error

	// Callbacks.
	OnFileReceived func(t *Transfer)
	OnFileOffer    func(t *Transfer) // called when remote offers a file
}

// NewManager creates a file transfer manager.
func NewManager(receiveDir string) *Manager {
	return &Manager{
		transfers:  make(map[string]*Transfer),
		ReceiveDir: receiveDir,
	}
}

// ── Sending ─────────────────────────────────────────────────────────

// OfferFile prepares a file for transfer and sends a file-offer message.
func (m *Manager) OfferFile(path string) (*Transfer, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("cannot transfer directories")
	}

	id := generateID()
	mimeType := detectMimeType(path)
	preview := generatePreview(path, mimeType)

	t := &Transfer{
		ID:       id,
		Name:     filepath.Base(path),
		Size:     fi.Size(),
		MimeType: mimeType,
		Preview:  preview,
		Path:     path,
	}

	m.mu.Lock()
	m.transfers[id] = t
	m.mu.Unlock()

	// Send offer.
	return t, m.Send(protocol.MsgFileOffer, &protocol.FileOfferMessage{
		ID:       id,
		Name:     t.Name,
		Size:     t.Size,
		MimeType: mimeType,
		Preview:  preview,
	})
}

// HandleAccept processes a file-accept and starts sending chunks.
func (m *Manager) HandleAccept(msg *protocol.FileAcceptMessage) {
	m.mu.Lock()
	t := m.transfers[msg.ID]
	m.mu.Unlock()
	if t == nil {
		return
	}

	go m.sendChunks(t)
}

// HandleReject processes a file-reject.
func (m *Manager) HandleReject(msg *protocol.FileRejectMessage) {
	m.mu.Lock()
	delete(m.transfers, msg.ID)
	m.mu.Unlock()
}

func (m *Manager) sendChunks(t *Transfer) {
	f, err := os.Open(t.Path)
	if err != nil {
		log.Printf("file transfer: open error: %v", err)
		return
	}
	defer f.Close()

	hasher := sha256.New()
	buf := make([]byte, ChunkSize)
	var offset int64

	for {
		n, err := f.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			hasher.Write(chunk)

			encoded := base64.StdEncoding.EncodeToString(chunk)
			m.Send(protocol.MsgFileChunk, &protocol.FileChunkMessage{
				ID:     t.ID,
				Offset: offset,
				Data:   encoded,
			})

			offset += int64(n)
			t.mu.Lock()
			t.Transferred = offset
			t.mu.Unlock()
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("file transfer: read error: %v", err)
			return
		}
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	t.mu.Lock()
	t.Complete = true
	t.Hash = hash
	t.mu.Unlock()

	m.Send(protocol.MsgFileComplete, &protocol.FileCompleteMessage{
		ID:   t.ID,
		Hash: hash,
	})
}

// ── Receiving ───────────────────────────────────────────────────────

// HandleOffer processes an incoming file offer from remote.
func (m *Manager) HandleOffer(msg *protocol.FileOfferMessage) {
	t := &Transfer{
		ID:       msg.ID,
		Name:     sanitizeFilename(msg.Name),
		Size:     msg.Size,
		MimeType: msg.MimeType,
		Preview:  msg.Preview,
	}

	m.mu.Lock()
	m.transfers[msg.ID] = t
	m.mu.Unlock()

	if m.OnFileOffer != nil {
		m.OnFileOffer(t)
		return
	}

	// Auto-accept if no callback.
	m.AcceptFile(msg.ID)
}

// AcceptFile accepts an incoming file offer.
func (m *Manager) AcceptFile(id string) error {
	m.mu.Lock()
	t := m.transfers[id]
	m.mu.Unlock()
	if t == nil {
		return fmt.Errorf("unknown transfer: %s", id)
	}

	// Create receive directory.
	os.MkdirAll(m.ReceiveDir, 0755)
	destPath := filepath.Join(m.ReceiveDir, t.Name)
	destPath = uniquePath(destPath)

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	t.mu.Lock()
	t.Path = destPath
	t.file = f
	t.mu.Unlock()

	return m.Send(protocol.MsgFileAccept, &protocol.FileAcceptMessage{ID: id})
}

// RejectFile rejects an incoming file offer.
func (m *Manager) RejectFile(id, reason string) error {
	m.mu.Lock()
	delete(m.transfers, id)
	m.mu.Unlock()

	return m.Send(protocol.MsgFileReject, &protocol.FileRejectMessage{ID: id, Reason: reason})
}

// HandleChunk processes an incoming file chunk.
func (m *Manager) HandleChunk(msg *protocol.FileChunkMessage) {
	m.mu.Lock()
	t := m.transfers[msg.ID]
	m.mu.Unlock()
	if t == nil {
		return
	}

	data, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		log.Printf("file transfer: decode chunk error: %v", err)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return
	}

	t.file.Write(data)
	t.Transferred += int64(len(data))
}

// HandleComplete processes a file-complete message.
func (m *Manager) HandleComplete(msg *protocol.FileCompleteMessage) {
	m.mu.Lock()
	t := m.transfers[msg.ID]
	m.mu.Unlock()
	if t == nil {
		return
	}

	t.mu.Lock()
	if t.file != nil {
		t.file.Close()
		t.file = nil
	}
	t.Complete = true
	t.Hash = msg.Hash
	t.mu.Unlock()

	log.Printf("File received: %s (%d bytes)", t.Path, t.Transferred)

	if m.OnFileReceived != nil {
		m.OnFileReceived(t)
	}
}

// GetTransfer returns a transfer by ID.
func (m *Manager) GetTransfer(id string) *Transfer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transfers[id]
}

// ActiveTransfers returns all in-progress transfers.
func (m *Manager) ActiveTransfers() []*Transfer {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*Transfer, 0, len(m.transfers))
	for _, t := range m.transfers {
		result = append(result, t)
	}
	return result
}

// Cleanup closes any open files.
func (m *Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.transfers {
		t.mu.Lock()
		if t.file != nil {
			t.file.Close()
			t.file = nil
		}
		t.mu.Unlock()
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func detectMimeType(path string) string {
	ext := filepath.Ext(path)
	if mtype := mime.TypeByExtension(ext); mtype != "" {
		return mtype
	}
	// Fallback: read first 512 bytes.
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return http.DetectContentType(buf[:n])
}

func generatePreview(path, mimeType string) string {
	if strings.HasPrefix(mimeType, "image/") {
		return imagePreview(path)
	}
	// Videos, docs — no inline preview for now.
	return ""
}

func imagePreview(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return ""
	}

	// Resize to thumbnail.
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w > MaxPreviewSize || h > MaxPreviewSize {
		scale := float64(MaxPreviewSize) / float64(max(w, h))
		w = int(float64(w) * scale)
		h = int(float64(h) * scale)
	}

	// Simple nearest-neighbor resize.
	thumb := image.NewRGBA(image.Rect(0, 0, w, h))
	srcBounds := img.Bounds()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcX := srcBounds.Min.X + x*srcBounds.Dx()/w
			srcY := srcBounds.Min.Y + y*srcBounds.Dy()/h
			thumb.Set(x, y, img.At(srcX, srcY))
		}
	}

	// Encode as JPEG base64.
	var buf strings.Builder
	b64 := base64.NewEncoder(base64.StdEncoding, &buf)
	jpeg.Encode(b64, thumb, &jpeg.Options{Quality: 60})
	b64.Close()

	return "data:image/jpeg;base64," + buf.String()
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")
	if name == "" || name == "." {
		name = "received_file"
	}
	return name
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return path
}
