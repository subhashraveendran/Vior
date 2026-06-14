// Package filetransfer handles bidirectional file transfer over WebSocket.
// Files are chunked, base64-encoded, and sent through the existing WS connection.
package filetransfer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
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
	"time"

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

	// MaxDownloadSize is the upper sanity limit for HTTP-served file
	// transfers (desktop → mobile). Matches the UI hint shown in the
	// Files drop-zone copy.
	MaxDownloadSize = 2 * 1024 * 1024 * 1024 // 2 GiB
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

	// lastProgressAt is the Transferred value at which OnFileProgress
	// was last fired, so HandleChunk can coalesce notifications to
	// roughly one per progressEmitStep bytes.
	lastProgressAt int64

	// Receive buffer.
	file *os.File
	mu   sync.Mutex
	hash hash.Hash // incremental SHA-256 for receiver-side integrity

	// Cancellation for sending goroutines.
	ctx    context.Context
	cancel context.CancelFunc
}

// PendingDownload tracks a file the desktop has offered to the mobile
// over the HTTP-download path. The body stays on disk until either the
// mobile fetches it via GET /download/{id} or it expires/is cancelled.
type PendingDownload struct {
	ID       string
	Name     string
	Size     int64
	MimeType string
	Path     string
	Preview  string // base64 thumbnail for images, empty otherwise
	Accepted bool
	Served   bool
	mu       sync.Mutex
}

// Manager handles file transfers for a session.
type Manager struct {
	// Active transfers by ID.
	transfers map[string]*Transfer
	mu        sync.Mutex

	// Pending HTTP-download offers (desktop → mobile).
	pending   map[string]*PendingDownload
	pendingMu sync.Mutex

	// Where to save received files.
	ReceiveDir string

	// Sender function (inject the WS send).
	Send func(msgType protocol.MessageType, data any) error

	// Callbacks.
	OnFileReceived func(t *Transfer)
	OnFileOffer    func(t *Transfer) // called when remote offers a file

	// OnFileProgress, if set, is invoked mid-stream on the receiving
	// side at most once per progressEmitStep bytes (or on the final
	// chunk). Lets the desktop UI render a live progress bar instead
	// of jumping from 0 → 100 only on file-complete.
	OnFileProgress func(t *Transfer)

	// OnDownloadDone fires after the mobile reports it finished the GET
	// (or we hit ServeDownload to completion). Lets the desktop UI mark
	// the row green.
	OnDownloadDone func(p *PendingDownload)
}

// progressEmitStep coalesces per-chunk progress notifications so a fast
// 200 MB transfer doesn't spam the desktop event bus with ~4000
// notifications. 256 KiB ≈ ~1% on a 25 MB file, which is enough
// granularity for a UI bar without dominating the event loop.
const progressEmitStep = 256 * 1024

// NewManager creates a file transfer manager.
func NewManager(receiveDir string) *Manager {
	return &Manager{
		transfers:  make(map[string]*Transfer),
		pending:    make(map[string]*PendingDownload),
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

	if m.Send == nil {
		return t, nil
	}
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

	// Multi-file concurrency safe — each file transfer gets its own
	// goroutine and no shared mutable state beyond what the Manager
	// already guards.
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.ctx = ctx
	t.cancel = cancel
	t.mu.Unlock()
	go m.sendChunks(t)
}

// HandleReject processes a file-reject.
func (m *Manager) HandleReject(msg *protocol.FileRejectMessage) {
	m.mu.Lock()
	t := m.transfers[msg.ID]
	delete(m.transfers, msg.ID)
	m.mu.Unlock()
	if t != nil {
		t.mu.Lock()
		if t.cancel != nil {
			t.cancel()
		}
		t.mu.Unlock()
	}
}

func (m *Manager) sendChunks(t *Transfer) {
	t.mu.Lock()
	ctx := t.ctx
	t.mu.Unlock()

	f, err := os.Open(t.Path)
	if err != nil {
		log.Printf("filetransfer: open error: %v", err)
		return
	}
	defer f.Close()

	hasher := sha256.New()
	buf := make([]byte, ChunkSize)
	var offset int64

	for {
		select {
		case <-ctx.Done():
			log.Printf("filetransfer: cancelled %s", t.Name)
			return
		default:
		}

		n, err := f.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			hasher.Write(chunk)

			encoded := base64.StdEncoding.EncodeToString(chunk)
			sendErr := m.Send(protocol.MsgFileChunk, &protocol.FileChunkMessage{
				ID:     t.ID,
				Offset: offset,
				Data:   encoded,
			})
			if sendErr != nil {
				log.Printf("filetransfer: send error: %v", sendErr)
				return
			}

			offset += int64(n)
			t.mu.Lock()
			t.Transferred = offset
			t.mu.Unlock()

			// Throttle: 1 ms yield between chunks. The previous 5 ms was
			// throttling a local-LAN transfer to a ~9.6 MB/s ceiling
			// (48 KiB chunks × 200 chunks/s), well below what either the
			// JSON-WS encoder or the receiver can handle. We can't
			// remove the sleep entirely because the JSON-WS path doesn't
			// back-pressure naturally — gorilla/websocket buffers
			// writes in memory — so a hot tight loop on a giant file
			// would blow up the write buffer. 1 ms (≈ 48 MB/s ceiling)
			// matches local-LAN saturation while keeping the goroutine
			// cooperatively yielding to the WS reader.
			time.Sleep(1 * time.Millisecond)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("filetransfer: read error: %v", err)
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

	log.Printf("filetransfer: sent %s (%d bytes)", t.Name, offset)
}

// ── Receiving ───────────────────────────────────────────────────────

// HandleOffer processes an incoming file offer from remote.
func (m *Manager) HandleOffer(msg *protocol.FileOfferMessage) {
	// Enforce the same upper bound the desktop→mobile path uses
	// (MaxDownloadSize / 2 GiB). Without this the WS-chunked path is
	// happy to write multi-terabyte garbage to ~/Downloads/Vior.
	if msg.Size < 0 || msg.Size > MaxDownloadSize {
		log.Printf("filetransfer: rejecting offer %s (%d bytes > max %d)", msg.ID, msg.Size, MaxDownloadSize)
		if m.Send != nil {
			_ = m.Send(protocol.MsgFileReject, &protocol.FileRejectMessage{
				ID:     msg.ID,
				Reason: fmt.Sprintf("file too large (max %d bytes)", MaxDownloadSize),
			})
		}
		return
	}
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
	destPath := filepath.Join(m.ReceiveDir, sanitizeFilename(t.Name))
	// Belt-and-suspenders: filepath.Join + the sanitizer should already
	// keep the dest inside ReceiveDir, but a mid-stream rename of
	// ReceiveDir or an exotic Unicode glyph that survives sanitization
	// could still slip out. Re-derive the cleaned absolute path and
	// confirm it lives under ReceiveDir before we open the file.
	absRoot, _ := filepath.Abs(m.ReceiveDir)
	absDest, _ := filepath.Abs(destPath)
	if rel, err := filepath.Rel(absRoot, absDest); err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return fmt.Errorf("filetransfer: refusing to write outside %s (got %s)", m.ReceiveDir, destPath)
	}
	destPath = uniquePath(destPath)

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	t.mu.Lock()
	t.Path = destPath
	t.file = f
	t.hash = sha256.New()
	t.mu.Unlock()

	if err := m.Send(protocol.MsgFileAccept, &protocol.FileAcceptMessage{ID: id}); err != nil {
		t.mu.Lock()
		if t.file != nil {
			t.file.Close()
			t.file = nil
		}
		t.mu.Unlock()
		return fmt.Errorf("send accept: %w", err)
	}
	return nil
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
		log.Printf("filetransfer: decode chunk error: %v", err)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return
	}

	// Guard against a misbehaving sender that keeps streaming past the
	// advertised Size — without this the receive file would grow
	// without bound on an unauthenticated socket.
	if t.Size > 0 && t.Transferred+int64(len(data)) > t.Size {
		log.Printf("filetransfer: chunk overshoots advertised size for %s (%d > %d) — closing", t.ID, t.Transferred+int64(len(data)), t.Size)
		t.file.Close()
		t.file = nil
		return
	}

	if _, err := t.file.Write(data); err != nil {
		log.Printf("filetransfer: write error for %s: %v", t.ID, err)
		t.file.Close()
		t.file = nil
		return
	}
	t.hash.Write(data)
	t.Transferred += int64(len(data))

	// Coalesced progress callback — fires at most once per
	// progressEmitStep bytes, plus once at completion (via
	// HandleComplete). We snapshot Transferred under the lock and
	// invoke the user callback after releasing it so the callback
	// can re-enter Manager methods (e.g. ActiveTransfers) without
	// deadlocking.
	var firedAt int64
	var emit bool
	if m.OnFileProgress != nil {
		if t.Transferred-t.lastProgressAt >= progressEmitStep ||
			(t.Size > 0 && t.Transferred >= t.Size) {
			t.lastProgressAt = t.Transferred
			firedAt = t.Transferred
			emit = true
		}
	}
	if emit {
		// Capture the fields we want to expose; the callback runs after
		// we return from this method (and release the mutex via defer).
		go func(id, name, mime, preview, path string, size, transferred int64) {
			m.OnFileProgress(&Transfer{
				ID: id, Name: name, MimeType: mime, Preview: preview,
				Path: path, Size: size, Transferred: transferred,
			})
		}(t.ID, t.Name, t.MimeType, t.Preview, t.Path, t.Size, firedAt)
	}
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

	// Verify integrity: compare sender's claimed hash against the
	// incremental hash we've been computing as chunks arrived.
	got := hex.EncodeToString(t.hash.Sum(nil))
	if got != msg.Hash {
		log.Printf("filetransfer: SHA-256 mismatch for %s (got %s, want %s)", t.ID, got, msg.Hash)
		t.mu.Unlock()
		return
	}

	t.Complete = true
	t.Hash = msg.Hash
	t.mu.Unlock()

	log.Printf("filetransfer: received %s (%d bytes)", t.Path, t.Transferred)

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
	for _, t := range m.transfers {
		t.mu.Lock()
		if t.file != nil {
			t.file.Close()
			t.file = nil
		}
		t.mu.Unlock()
	}
	m.mu.Unlock()

	m.pendingMu.Lock()
	m.pending = make(map[string]*PendingDownload)
	m.pendingMu.Unlock()
}

// ── HTTP Download Path (desktop → mobile) ────────────────────────────

// OfferDownload registers a local file as a pending download served
// over HTTP. Returns the entry so the caller can push an
// IncomingFileMessage over the WS to the mobile.
func (m *Manager) OfferDownload(path string) (*PendingDownload, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("cannot transfer directories")
	}
	if fi.Size() > MaxDownloadSize {
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", fi.Size(), MaxDownloadSize)
	}
	mimeType := detectMimeType(path)
	p := &PendingDownload{
		ID:       generateID(),
		Name:     filepath.Base(path),
		Size:     fi.Size(),
		MimeType: mimeType,
		Path:     path,
		Preview:  generatePreview(path, mimeType),
	}
	m.pendingMu.Lock()
	m.pending[p.ID] = p
	m.pendingMu.Unlock()
	return p, nil
}

// GetPending looks up a pending HTTP-download offer.
func (m *Manager) GetPending(id string) *PendingDownload {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	return m.pending[id]
}

// MarkDownloadAccepted flips Accepted=true so concurrent GETs after a
// reject can be 410'd.
func (m *Manager) MarkDownloadAccepted(id string) {
	m.pendingMu.Lock()
	p := m.pending[id]
	m.pendingMu.Unlock()
	if p == nil {
		return
	}
	p.mu.Lock()
	p.Accepted = true
	p.mu.Unlock()
}

// CancelDownload removes a pending entry without serving it. Called on
// MsgDownloadReject so the file can no longer be fetched.
func (m *Manager) CancelDownload(id string) {
	m.pendingMu.Lock()
	delete(m.pending, id)
	m.pendingMu.Unlock()
}

// ServeDownload streams the pending file body to w. Safe for files up
// to MaxDownloadSize — uses http.ServeContent so range requests and
// chunked transfer-encoding work for free.
func (m *Manager) ServeDownload(w http.ResponseWriter, r *http.Request, id string) {
	p := m.GetPending(id)
	if p == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	p.mu.Lock()
	served := p.Served
	p.Served = true
	p.mu.Unlock()
	if served {
		// Single-shot: protects against the mobile retrying after the
		// file is already gone (e.g. desktop side cancelled).
		http.Error(w, "already served", http.StatusGone)
		return
	}

	f, err := os.Open(p.Path)
	if err != nil {
		http.Error(w, "open failed", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Resolve symlinks so a crafted path through a symlink inside
	// ReceiveDir can't reach files outside it.
	realPath, err := filepath.EvalSymlinks(p.Path)
	if err != nil {
		http.Error(w, "resolve failed", http.StatusInternalServerError)
		return
	}
	absRoot, _ := filepath.Abs(m.ReceiveDir)
	absReal, _ := filepath.Abs(realPath)
	if rel, err := filepath.Rel(absRoot, absReal); err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		http.Error(w, "path blocked", http.StatusForbidden)
		return
	}

	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", p.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", p.Name))
	// http.ServeContent streams via io.Copy under the hood — no full
	// buffering — and handles range requests transparently.
	http.ServeContent(w, r, p.Name, fi.ModTime(), f)

	if m.OnDownloadDone != nil {
		m.OnDownloadDone(p)
	}
}

// CompleteDownload marks a pending download done and removes it. Called
// when the mobile reports MsgDownloadComplete.
func (m *Manager) CompleteDownload(id string) *PendingDownload {
	m.pendingMu.Lock()
	p := m.pending[id]
	delete(m.pending, id)
	m.pendingMu.Unlock()
	return p
}

// PendingDownloads returns a snapshot of pending HTTP-download offers.
func (m *Manager) PendingDownloads() []*PendingDownload {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	out := make([]*PendingDownload, 0, len(m.pending))
	for _, p := range m.pending {
		out = append(out, p)
	}
	return out
}

// ── Helpers ─────────────────────────────────────────────────────────

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
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
	if w <= 0 || h <= 0 {
		return ""
	}
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

// sanitizeFilename strips anything that would let a sender escape the
// ReceiveDir or trigger surprising behaviour on the host filesystem.
// Rules:
//   - filepath.Base + strip "../"/"..\\" segments so a sender can't pivot
//     up the tree (e.g. "../../.ssh/authorized_keys").
//   - Drop NULs and ASCII control chars (some macOS APIs and shells
//     barf on them; truncation attacks on \x00 are a classic).
//   - Replace path separators, ":" (macOS resource-fork shorthand), and
//     other reserved chars with "_".
//   - Strip leading dots so the file isn't hidden + leading dashes so
//     it can't be mistaken for a CLI flag if anyone shells over it.
//   - Cap at 255 bytes — common UFS/ext4/APFS NAME_MAX limit.
//   - Reject Windows reserved device names (CON, AUX, NUL, PRN, COM1…)
//     defensively, even though we're not on Windows — a synced
//     Downloads folder might be.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	// Remove "../" and "..\\" path-traversal fragments before they can
	// be re-joined into a path.
	name = strings.ReplaceAll(name, "..", "")
	// Replace anything risky with "_". Includes NULs, control chars,
	// path separators, colon, and the Windows-reserved < > " | ? *.
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == 0, r < 0x20, r == 0x7f:
			b.WriteByte('_')
		case r == '/', r == '\\', r == ':':
			b.WriteByte('_')
		case r == '<', r == '>', r == '"', r == '|', r == '?', r == '*':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	name = b.String()
	// Strip leading dots (".env", ".bashrc") and leading dashes
	// ("-rf" looking like a flag). Trailing dots/spaces confuse Windows.
	name = strings.TrimLeft(name, ". -")
	name = strings.TrimRight(name, ". ")
	if name == "" {
		name = "received_file"
	}
	// Reject Windows reserved device names case-insensitively (the
	// receive dir might live on a synced cloud folder that gets shared
	// to a Windows machine).
	upper := strings.ToUpper(name)
	stem := upper
	if dot := strings.Index(stem, "."); dot > 0 {
		stem = stem[:dot]
	}
	switch stem {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}
	// NAME_MAX-ish cap. Truncate from the middle so the extension
	// survives — if it doesn't have an extension, just take the first
	// 255 bytes.
	const maxLen = 255
	if len(name) > maxLen {
		ext := filepath.Ext(name)
		if len(ext) > 0 && len(ext) < 32 {
			keep := maxLen - len(ext)
			if keep < 1 {
				keep = 1
			}
			name = name[:keep] + ext
		} else {
			name = name[:maxLen]
		}
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
	// 1000 files with the same name — extremely unlikely. Append a
	// nanosecond timestamp as last-resort disambiguator.
	return fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
}
