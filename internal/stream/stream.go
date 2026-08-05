// Package stream handles serving captured frames as MJPEG over HTTP,
// and manages WebSocket connections for client handshake and input forwarding.
package stream

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/subhashraveendran/vior/internal/config"
	"github.com/subhashraveendran/vior/internal/machineid"
	"github.com/subhashraveendran/vior/internal/protocol"
	"github.com/subhashraveendran/vior/internal/trust"
)

// trustedDevices is the shared list of devices that have completed a
// pair-code handshake. A single process-wide store is sufficient — the
// underlying file is locked by the OS on writes and there's only ever
// one server per machine.
var trustedDevices = trust.Default()

// TrustedDevices exposes the store for callers that want to list or
// forget devices from the UI (e.g. Settings → Trusted devices).
func TrustedDevices() *trust.Store { return trustedDevices }

// pairCode is the 6-digit numeric "phone number" for this Vior install.
// It is derived deterministically from the machine UUID — the user can
// memorise it once and it survives reinstalls + ~/.vior/pair.txt wipes.
// A user-set override (SetPairCode) is read from ~/.vior/pair.txt if
// EnablePersistedPair was called.
var (
	pairCodeMu sync.RWMutex
	pairCode   = derivePair()
)

// pairCodeDigits is the number of decimal digits in the derived pair
// code. 6 digits = 1 000 000 combinations: at the existing per-IP
// throttle (5/min) a single attacker needs ~138 days to enumerate;
// the global throttle adds another order of magnitude when the
// attacker fans out across IPs. 4 digits (10 000 combos) was the
// previous value and was brute-forced in ~13 minutes from 256 IPs.
const pairCodeDigits = 6

// derivePair returns the stable per-machine pair code. Strategy:
// SHA-256("vior-pair:" + machineID), walk the lower-case hex digest
// collecting decimal digits 0–9 until we have pairCodeDigits. SHA-256
// hex is 64 chars long and on average ~25 of them are decimal digits,
// so collecting 6 succeeds almost always; the fallback (Uint32 of the
// first 4 bytes mod 10^pairCodeDigits) is defence in depth.
func derivePair() string {
	id := machineid.ID()
	sum := sha256.Sum256([]byte("vior-pair:" + id))
	hexed := hex.EncodeToString(sum[:])
	var b strings.Builder
	for _, c := range hexed {
		if c >= '0' && c <= '9' {
			b.WriteByte(byte(c))
			if b.Len() == pairCodeDigits {
				return b.String()
			}
		}
	}
	mod := uint32(1)
	for i := 0; i < pairCodeDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", pairCodeDigits, binary.BigEndian.Uint32(sum[:4])%mod)
}

// pairFilePath returns ~/.vior/pair.txt or "" if no home directory.
func pairFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".vior", "pair.txt")
}

var pairOverrideRe = regexp.MustCompile(`^[0-9]{4,8}$`)

// EnablePersistedPair loads the user-override pair code from
// ~/.vior/pair.txt if it exists. The default machine-derived code is
// never persisted, so deleting the file always falls back cleanly to
// the same derived value (the user's "phone number"). Call once before
// the first WS upgrade.
func EnablePersistedPair() {
	path := pairFilePath()
	if path == "" {
		log.Printf("stream: persisted pair disabled (no home dir)")
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("stream: read %s failed: %v (using derived pair)", path, err)
		}
		// Never log the actual pair code — it's the admission secret and
		// this line runs on the default (no-override) path. See the
		// redaction at the pair-mismatch site for the same rule.
		log.Printf("stream: using machine-derived pair code")
		return
	}
	code := strings.TrimSpace(string(b))
	if !pairOverrideRe.MatchString(code) {
		log.Printf("stream: %s contents invalid (%q); using derived pair", path, code)
		return
	}
	pairCodeMu.Lock()
	pairCode = code
	pairCodeMu.Unlock()
	log.Printf("stream: loaded persisted pair-code override from %s", path)
}

// SetPairCode persists a user-chosen pair code (4–8 digits) to
// ~/.vior/pair.txt atomically and updates the in-memory value. Returns
// an error for invalid input or filesystem failures. Pass an empty
// string to clear the override and fall back to the machine-derived
// default.
func SetPairCode(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		path := pairFilePath()
		if path == "" {
			return fmt.Errorf("no home directory")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		pairCodeMu.Lock()
		pairCode = derivePair()
		pairCodeMu.Unlock()
		return nil
	}
	if !pairOverrideRe.MatchString(s) {
		return fmt.Errorf("pair code must be 4-8 digits (0-9)")
	}
	path := pairFilePath()
	if path == "" {
		return fmt.Errorf("no home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(s), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	pairCodeMu.Lock()
	pairCode = s
	pairCodeMu.Unlock()
	return nil
}

// serverID is a stable per-install ID persisted at ~/.vior/server-id so
// mobiles can detect when the same desktop reappears at a different IP
// (DHCP lease renewal, Wi-Fi/Ethernet hand-off). Exposed via /info.
var serverID = loadOrCreateServerID()

func loadOrCreateServerID() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Worst case: ephemeral ID for this run. Mobile can't pin it
		// across IP changes, but everything else still works.
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			// Extremely unlikely — crypto/rand failure. Fall back to
			// a time-based unique value as last resort.
			return "srv-" + hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		}
		return "srv-" + hex.EncodeToString(b)
	}
	dir := filepath.Join(home, ".vior")
	path := filepath.Join(dir, "server-id")
	if b, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(b))
		// Validate: must be "srv-" prefix + 16 hex chars (8 bytes).
		// Reject anything that's not a valid server ID to prevent
		// injection via a tampered server-id file.
		if strings.HasPrefix(id, "srv-") && len(id) == 20 {
			if _, err := hex.DecodeString(id[4:]); err == nil {
				return id
			}
		}
	}
	_ = os.MkdirAll(dir, 0o700)
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "srv-fallback"
	}
	id := "srv-" + hex.EncodeToString(b)
	_ = os.WriteFile(path, []byte(id), 0o600)
	return id
}

// ServerID returns the stable per-install server identifier.
func ServerID() string { return serverID }

// pairAttempts tracks failed pair-code submissions per remote IP. The
// per-IP throttle was always there; the global throttle is the new
// belt against a distributed brute force where each source IP only
// burns ≤maxPairAttempts before rotating. 6-digit codes plus a global
// 60-burst/min ceiling push enumeration beyond practical attack
// windows for LAN-only auth.
const (
	maxPairAttempts       = 5
	maxGlobalPairAttempts = 60
	pairAttemptWindow     = time.Minute
)

type pairAttemptBucket struct {
	times []time.Time
}

var (
	pairAttemptsMu     sync.Mutex
	pairAttempts       = map[string]*pairAttemptBucket{}
	globalPairAttempts = &pairAttemptBucket{}
)

func init() {
	go func() {
		for {
			time.Sleep(pairAttemptWindow)
			pairAttemptsMu.Lock()
			cutoff := time.Now().Add(-pairAttemptWindow)
			for ip, b := range pairAttempts {
				pruned := b.times[:0]
				for _, t := range b.times {
					if t.After(cutoff) {
						pruned = append(pruned, t)
					}
				}
				if len(pruned) == 0 {
					delete(pairAttempts, ip)
				} else {
					b.times = pruned
					pairAttempts[ip] = b
				}
			}
			pairAttemptsMu.Unlock()
		}
	}()
}

// recordPairAttempt registers a failed attempt from ip. Returns true if
// the IP is now over the per-IP limit OR the global server-wide limit.
// The global bucket catches a distributed brute force that would slip
// past per-IP throttling by rotating source IPs.
func recordPairAttempt(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-pairAttemptWindow)
	pairAttemptsMu.Lock()
	defer pairAttemptsMu.Unlock()

	// Global ceiling: record every failed attempt regardless of source.
	globalPairAttempts.times = pruneBefore(globalPairAttempts.times, cutoff)
	globalPairAttempts.times = append(globalPairAttempts.times, now)
	overGlobal := len(globalPairAttempts.times) > maxGlobalPairAttempts

	// Per-IP bucket only when we have an IP — empty source still counts
	// against the global ceiling above so a missing client header can't
	// be used to fly under the radar.
	if ip == "" {
		return overGlobal
	}
	b := pairAttempts[ip]
	if b == nil {
		b = &pairAttemptBucket{}
		pairAttempts[ip] = b
	}
	b.times = pruneBefore(b.times, cutoff)
	b.times = append(b.times, now)
	return overGlobal || len(b.times) > maxPairAttempts
}

// pruneBefore returns t with every entry before cutoff dropped, reusing
// the input slice's backing array.
func pruneBefore(t []time.Time, cutoff time.Time) []time.Time {
	pruned := t[:0]
	for _, x := range t {
		if x.After(cutoff) {
			pruned = append(pruned, x)
		}
	}
	return pruned
}

// clearPairAttempts drops the bucket for ip — called after a successful
// admission so an honest device that mistyped once doesn't carry the
// counter into a permanent ban.
func clearPairAttempts(ip string) {
	if ip == "" {
		return
	}
	pairAttemptsMu.Lock()
	delete(pairAttempts, ip)
	pairAttemptsMu.Unlock()
}

// remoteIP extracts the bare IP (no port) from a RemoteAddr like
// "192.168.1.50:54321" or an IPv6 "[::1]:54321".
func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// PairCode returns the active pair code: the user-override if set, else
// the machine-derived 4-digit numeric default.
func PairCode() string {
	pairCodeMu.RLock()
	defer pairCodeMu.RUnlock()
	return pairCode
}

const (
	// maxClients limits concurrent MJPEG stream connections.
	maxClients = 16

	// writeTimeout is the deadline for writing each frame to a client.
	writeTimeout = 5 * time.Second
)

// SessionHandler handles WebSocket session lifecycle events.
// Implement this to react when a client connects with its screen dimensions.
type SessionHandler interface {
	OnClientConnect(session *protocol.Session, hello *protocol.HelloMessage) error
	OnClientInput(session *protocol.Session, msg *protocol.InputMessage) error
	OnClientResize(session *protocol.Session, msg *protocol.ResizeMessage) error
	OnClientDisconnect(session *protocol.Session)
	OnClientFileOffer(session *protocol.Session, msg *protocol.FileOfferMessage) error
	OnClientFileAccept(session *protocol.Session, msg *protocol.FileAcceptMessage) error
	OnClientFileReject(session *protocol.Session, msg *protocol.FileRejectMessage) error
	OnClientFileChunk(session *protocol.Session, msg *protocol.FileChunkMessage) error
	OnClientFileComplete(session *protocol.Session, msg *protocol.FileCompleteMessage) error
	OnClientDownloadAccept(session *protocol.Session, msg *protocol.DownloadAcceptMessage) error
	OnClientDownloadReject(session *protocol.Session, msg *protocol.DownloadRejectMessage) error
	OnClientDownloadComplete(session *protocol.Session, msg *protocol.DownloadCompleteMessage) error

	// ServeDownload writes the pending file body for HTTP GET /download/{id}.
	// Implementations should write 404 if the id is unknown / already served,
	// and stream the file body (chunked) without buffering it fully in memory.
	ServeDownload(w http.ResponseWriter, r *http.Request, id string)
}

// MJPEGServer streams JPEG frames to HTTP clients and manages WebSocket sessions.
type MJPEGServer struct {
	port    int
	host    string
	server  *http.Server
	running bool
	mu      sync.RWMutex

	// frameCh receives JPEG frames from capture session.
	frameCh <-chan []byte

	// current holds the latest frame for new clients.
	currentFrame []byte
	frameMu      sync.RWMutex

	// clients tracks connected MJPEG stream consumers.
	clients   map[chan []byte]struct{}
	clientsMu sync.Mutex

	// WebSocket support.
	handler  SessionHandler
	upgrader websocket.Upgrader
	wsConn   *protocol.Session // current WebSocket client (single client mode)
	wsConnMu sync.Mutex

	// frameClientIP is the remote IP of the currently paired WS client.
	// /snapshot and /stream serve raw screen frames, so they must only
	// answer the already-authenticated client (or loopback, i.e. the
	// desktop's own preview) — otherwise any LAN peer could scrape the
	// screen with no pairing. Guarded by frameClientMu. Empty = nobody
	// paired = frames closed.
	frameClientIP string
	frameClientMu sync.RWMutex

	// frameToken authorises the frame endpoints for a secure session.
	// Derived from the session key and delivered only inside the sealed
	// channel, so holding it proves the bearer completed the handshake.
	// Empty for cleartext sessions, which fall back to IP-only checks.
	// Guarded by frameClientMu alongside frameClientIP.
	frameToken string

	// setFrameMu serializes SetFrameCh calls end-to-end. distMu alone is
	// not enough: SetFrameCh must release distMu while it waits for the
	// old distributor to exit (<-done), and two concurrent callers could
	// both observe distRunning, both wait on the same done, then both
	// install a distributor — leaking one and desyncing the done/stop
	// channels. A coarse op-lock makes SetFrameCh atomic; the exiting
	// distributor's defer only needs distMu, so there is no deadlock.
	setFrameMu sync.Mutex

	// Distributor lifecycle. All four fields are guarded by distMu.
	// They were previously read/written lock-free from Start, Stop,
	// SetFrameCh and the distributor goroutine simultaneously — a data
	// race that could double-close stopDistribute or observe a torn
	// frameCh swap. distributeFrames now receives its channels as
	// parameters so it never reads these fields after launch; the only
	// shared write it makes is distRunning=false on exit, under distMu.
	distMu         sync.Mutex
	stopDistribute chan struct{}
	distDone       chan struct{} // closed when the distributor goroutine exits
	distRunning    bool
}

// NewMJPEGServer creates a new MJPEG streaming server.
func NewMJPEGServer(host string, port int, frameCh <-chan []byte, handler SessionHandler) *MJPEGServer {
	return &MJPEGServer{
		port:    port,
		host:    host,
		frameCh: frameCh,
		clients: make(map[chan []byte]struct{}),
		handler: handler,
		upgrader: websocket.Upgrader{
			CheckOrigin: checkWSOrigin,
		},
		stopDistribute: make(chan struct{}),
		distDone:       make(chan struct{}),
	}
}

// Start begins the HTTP server and frame distribution.
func (s *MJPEGServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", s.handleStream)
	mux.HandleFunc("/snapshot", s.handleSnapshot)
	mux.HandleFunc("/info", s.handleInfo)
	if s.handler != nil {
		mux.HandleFunc("/ws", s.handleWebSocket)
		// /download/{id} → mobile fetches a file the desktop offered via
		// the IncomingFile WS notification. Streamed via io.Copy so 2GB
		// files don't blow up the heap. Auth: only the single connected
		// WS client knows the id (just sent over the trusted WS), so a
		// LAN-snooper would have to also intercept the WS to obtain it.
		mux.HandleFunc("/download/", s.handleDownload)
	}
	// Serve embedded web client for all other paths.
	mux.Handle("/", webClientHandler())

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      corsHandler(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // Disabled — we use per-frame deadlines instead.
		IdleTimeout:  120 * time.Second,
	}

	// Distribute frames to all connected clients.
	if s.frameCh != nil {
		s.distMu.Lock()
		fc := s.frameCh
		stop := s.stopDistribute
		done := make(chan struct{})
		s.distDone = done
		s.distRunning = true
		s.distMu.Unlock()
		go s.distributeFrames(fc, stop, done)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	// If we bound port 0 (ephemeral), write the OS-assigned port back so
	// Port() and every caller (banner, URLs, QR, discovery beacon) report
	// the real port instead of 0.
	if s.port == 0 {
		if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
			s.mu.Lock()
			s.port = tcpAddr.Port
			s.mu.Unlock()
		}
	}

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("stream: server error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the server.
func (s *MJPEGServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil
	}
	s.running = false

	// Signal frame distributor to stop.
	s.distMu.Lock()
	select {
	case <-s.stopDistribute:
	default:
		close(s.stopDistribute)
	}
	s.distMu.Unlock()

	// Close WebSocket session if active.
	s.wsConnMu.Lock()
	if s.wsConn != nil {
		s.wsConn.Close()
		s.wsConn = nil
	}
	s.wsConnMu.Unlock()

	// Close all MJPEG client channels.
	s.clientsMu.Lock()
	for ch := range s.clients {
		delete(s.clients, ch)
		close(ch)
	}
	s.clientsMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// IsRunning reports if server is active.
func (s *MJPEGServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// SetFrameCh replaces the frame channel (e.g. when a new capture session starts
// after a client connects with different resolution).
func (s *MJPEGServer) SetFrameCh(ch <-chan []byte) {
	// Serialize the whole operation so concurrent callers can't both wait
	// on the same distributor's done and then both install a new one.
	s.setFrameMu.Lock()
	defer s.setFrameMu.Unlock()

	s.distMu.Lock()
	if s.distRunning {
		stop := s.stopDistribute
		done := s.distDone
		select {
		case <-stop:
		default:
			close(stop)
		}
		// Release the lock before waiting so the exiting distributor can
		// take distMu to clear distRunning — holding it here would
		// deadlock against that defer.
		s.distMu.Unlock()
		<-done
		s.distMu.Lock()
	}

	s.frameCh = ch
	s.stopDistribute = make(chan struct{})
	s.distDone = make(chan struct{})
	if ch != nil {
		s.distRunning = true
		fc := s.frameCh
		stop := s.stopDistribute
		done := s.distDone
		s.distMu.Unlock()
		go s.distributeFrames(fc, stop, done)
		return
	}
	s.distRunning = false
	s.distMu.Unlock()
}

// ClientCount returns the number of connected MJPEG stream clients.
func (s *MJPEGServer) ClientCount() int {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	return len(s.clients)
}

// ClientSecure reports whether the currently connected WebSocket client
// negotiated an encrypted channel. The desktop surfaces this so the UI states
// the connection's actual security rather than assuming it — under
// SecurePreferred a legacy client is admitted in cleartext, and the user
// deserves to know which one they have.
func (s *MJPEGServer) ClientSecure() bool {
	s.wsConnMu.Lock()
	defer s.wsConnMu.Unlock()
	return s.wsConn != nil && s.wsConn.IsSecure()
}

// Port returns the configured port.
func (s *MJPEGServer) Port() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

// distributeFrames fans captured frames out to all connected MJPEG
// clients. It receives its channels as parameters (captured at launch
// under distMu) so it never races a concurrent SetFrameCh swap of the
// server's shared fields.
func (s *MJPEGServer) distributeFrames(frameCh <-chan []byte, stop <-chan struct{}, done chan struct{}) {
	defer func() {
		s.distMu.Lock()
		s.distRunning = false
		s.distMu.Unlock()
		close(done)
	}()
	for {
		select {
		case <-stop:
			return
		case frame, ok := <-frameCh:
			if !ok {
				return
			}
			// Store latest frame.
			s.frameMu.Lock()
			s.currentFrame = frame
			s.frameMu.Unlock()

			// Fan out to all connected clients.
			s.clientsMu.Lock()
			for ch := range s.clients {
				select {
				case ch <- frame:
				default:
					// Client too slow — frame dropped for this client.
				}
			}
			s.clientsMu.Unlock()
		}
	}
}

func (s *MJPEGServer) addClient() (chan []byte, error) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	if len(s.clients) >= maxClients {
		return nil, fmt.Errorf("max clients (%d) reached", maxClients)
	}

	ch := make(chan []byte, 2)
	s.clients[ch] = struct{}{}
	return ch, nil
}

func (s *MJPEGServer) removeClient(ch chan []byte) {
	// Close-once: only the goroutine that removes ch from the map closes
	// it. Stop() also deletes+closes every client under clientsMu, so a
	// handleStream defer that fires after Stop finds ch already gone and
	// skips the close — without this guard the two paths double-closed
	// the channel and panicked on shutdown with a viewer connected.
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	if _, ok := s.clients[ch]; ok {
		delete(s.clients, ch)
		close(ch)
	}
}

func (s *MJPEGServer) generateBoundary() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "vior-frame-boundary"
	}
	return "vior-boundary-" + hex.EncodeToString(b)
}

func (s *MJPEGServer) handleStream(w http.ResponseWriter, r *http.Request) {
	if !s.frameClientAuthorized(r) {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}
	log.Printf("stream: MJPEG client connected: %s", r.RemoteAddr)

	ch, err := s.addClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer s.removeClient(ch)

	boundary := s.generateBoundary()
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	for {
		select {
		case frame, ok := <-ch:
			if !ok {
				return
			}

			// Apply per-frame write deadline so stalled clients don't
			// hold goroutines and memory forever.
			if rc, ok := w.(interface{ SetWriteDeadline(time.Time) error }); ok {
				rc.SetWriteDeadline(time.Now().Add(writeTimeout))
			}

			_, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", boundary, len(frame))
			if err != nil {
				return
			}
			if _, err = w.Write(frame); err != nil {
				return
			}
			if _, err = fmt.Fprintf(w, "\r\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-ctx.Done():
			log.Printf("stream: MJPEG client disconnected: %s", r.RemoteAddr)
			return
		}
	}
}

func (s *MJPEGServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	if s.handler == nil {
		http.Error(w, "downloads disabled", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/download/")
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/\\") {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.handler.ServeDownload(w, r, id)
}

func (s *MJPEGServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		// deviceId is the stable server-install ID. Mobiles save it and
		// use it to re-find the server at a new IP after DHCP drift.
		"name":     friendlyDeviceName(),
		"version":  config.Version,
		"platform": friendlyPlatform(),
		"deviceId": serverID,

		// Transport-security capability. A client uses this to decide
		// whether to attempt the handshake and whether to warn the user.
		// Only the policy is published — never the channel secret, which
		// travels solely in the QR payload. "secureRequired" tells an old
		// client it will be rejected before it wastes a connection.
		"secure":         GetSecurityMode() != SecureOff,
		"secureMode":     GetSecurityMode().String(),
		"secureRequired": GetSecurityMode() == SecureRequired,
	}
	// Pairing probe. Previously /info published the raw pairCode to any
	// LAN client, which nullified the whole pairing scheme (anyone could
	// read the code and connect). Instead, a client that ALREADY knows
	// the code passes it as ?probe= and we confirm — constant-time, and
	// rate-limited so this can't become a brute-force oracle. The code
	// itself is never returned.
	if probe := strings.TrimSpace(r.URL.Query().Get("probe")); probe != "" {
		ip := remoteIP(r.RemoteAddr)
		if subtle.ConstantTimeCompare([]byte(probe), []byte(PairCode())) == 1 {
			resp["paired"] = true
			clearPairAttempts(ip)
		} else if recordPairAttempt(ip) {
			http.Error(w, "too many pairing probes", http.StatusTooManyRequests)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("stream: /info encode failed: %v", err)
	}
}

func friendlyDeviceName() string {
	// Try to get friendly computer name.
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("scutil", "--get", "ComputerName").Output()
		if err == nil {
			name := strings.TrimSpace(string(out))
			if name != "" {
				return name
			}
		}
	}
	hostname, _ := os.Hostname()
	// Clean up hostname — remove .local suffix.
	hostname = strings.TrimSuffix(hostname, ".local")
	hostname = strings.ReplaceAll(hostname, "-", " ")
	return hostname
}

func friendlyPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

// frameClientAuthorized reports whether r may read raw screen frames.
// Only the paired WS client's IP (or loopback, i.e. the desktop's own
// preview) is allowed — otherwise any LAN peer could scrape the screen
// with no pairing at all.
// frameClientAuthorized gates the raw-screen endpoints (/stream, /snapshot).
//
// Loopback is always allowed — that is the desktop rendering its own preview.
// For a secure session the caller must present the session's frame token,
// which only the peer that completed the handshake can hold; the IP must
// still match as a second constraint. Cleartext sessions fall back to the
// historical IP-only check, which is weak but is all a legacy client can
// satisfy.
//
// Note this controls ACCESS, not confidentiality: /stream is plain HTTP, so a
// passive listener on the same network still sees the frames regardless of
// this check. Closing that gap needs the video path itself to be encrypted —
// see docs/securechan-handshake-architecture.md.
func (s *MJPEGServer) frameClientAuthorized(r *http.Request) bool {
	ip := remoteIP(r.RemoteAddr)
	if pip := net.ParseIP(ip); pip != nil && pip.IsLoopback() {
		return true
	}
	s.frameClientMu.RLock()
	authorizedIP := s.frameClientIP
	token := s.frameToken
	s.frameClientMu.RUnlock()

	if authorizedIP == "" || ip != authorizedIP {
		return false
	}
	if token == "" {
		// Cleartext session: no token exists to present.
		return true
	}
	supplied := r.URL.Query().Get("t")
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) == 1
}

func (s *MJPEGServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.frameClientAuthorized(r) {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}
	s.frameMu.RLock()
	frame := s.currentFrame
	s.frameMu.RUnlock()

	if frame == nil {
		http.Error(w, "no frame available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(frame)
}

func (s *MJPEGServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("stream: ws upgrade error: %v", err)
		return
	}

	session := protocol.NewSession(conn)
	log.Printf("stream: ws client connected: %s [%s]", r.RemoteAddr, session.ID)

	// Only one client at a time.
	s.wsConnMu.Lock()
	if s.wsConn != nil {
		s.wsConnMu.Unlock()
		session.Send(protocol.MsgError, &protocol.ErrorMessage{
			Code:    "occupied",
			Message: "Another device is already connected",
		})
		session.Close()
		return
	}
	s.wsConn = session
	s.wsConnMu.Unlock()

	defer func() {
		s.wsConnMu.Lock()
		if s.wsConn == session {
			s.wsConn = nil
		}
		s.wsConnMu.Unlock()
		// Revoke frame authorization for this client's IP so a later
		// unpaired peer at the same address can't keep scraping. The
		// token goes with it — it is scoped to this session's key, so
		// leaving it live would outlast the channel that justified it.
		s.frameClientMu.Lock()
		if s.frameClientIP == remoteIP(r.RemoteAddr) {
			s.frameClientIP = ""
			s.frameToken = ""
		}
		s.frameClientMu.Unlock()
		// sync.Once on the session guarantees OnClientDisconnect runs
		// exactly once even when an in-loop Bye and the post-loop
		// defer both arrive — without this guard, the App handler
		// would tear the virtual display down twice and race the
		// macOS CGVirtualDisplay teardown.
		session.FireDisconnect(func() {
			s.handler.OnClientDisconnect(session)
		})
		session.Close()
		log.Printf("stream: WebSocket client disconnected: %s [%s]", r.RemoteAddr, session.ID)
	}()

	// Establish the encrypted channel (or admit a legacy cleartext client
	// where policy allows) and read hello. The handshake deliberately runs
	// before hello so the pair code is sealed rather than exposed.
	hello, frameToken, err := s.negotiateSecure(session)
	if err != nil {
		log.Printf("stream: ws negotiation error [%s]: %v", session.ID, err)
		// negotiateSecure has already sent a specific error for the
		// cases the client can act on (upgrade_required, secure_failed);
		// this covers the rest without leaking internals.
		session.Send(protocol.MsgError, &protocol.ErrorMessage{
			Code:    "hello_failed",
			Message: "Connection setup failed.",
		})
		return
	}

	log.Printf("stream: client hello: %s %dx%d @%.1fx [%s]", hello.Name, hello.Width, hello.Height, hello.DPR, session.ID)

	// Admission policy: the pair code is the only authority. A
	// previously-trusted deviceID by itself is NOT enough — an
	// attacker who learns a legitimate deviceID (from logs, screen
	// shares, LAN sniffing of an unprotected ws) could otherwise
	// impersonate that device and skip authentication entirely.
	// Mobile clients already cache and resend the pair code on every
	// connect (see connect.ts), so this only costs UX in the rare
	// case of a tampered-mobile cache.
	//
	// The trust store is retained as informational metadata (LastSeen,
	// Settings UI list, pair history) but no longer short-circuits
	// admission.
	ip := remoteIP(r.RemoteAddr)
	switch {
	// Constant-time compare of the admission secret, matching the
	// /info?probe= path (handleInfo). A plain == here would leak the
	// pair code one byte at a time via response-timing to a LAN peer.
	case subtle.ConstantTimeCompare([]byte(strings.TrimSpace(hello.PairCode)), []byte(PairCode())) == 1:
		if hello.DeviceID != "" {
			if err := trustedDevices.Touch(hello.DeviceID, hello.Name, hello.Platform); err != nil {
				log.Printf("trust: store add failed [%s]: %v", session.ID, err)
			} else {
				log.Printf("trust: paired device [%s] id=%s name=%q platform=%q",
					session.ID, hello.DeviceID, hello.Name, hello.Platform)
			}
		}
		clearPairAttempts(ip)
	default:
		// Brute-force throttle: 5 wrong codes / minute / IP → 429 +
		// close. Without this a script on the LAN could enumerate the
		// 1M-combo (6-digit decimal) pair code in roughly a day.
		over := recordPairAttempt(ip)
		// Never log the real pair code (the admission secret). Log only
		// the length of the rejected guess — enough to debug a "wrong
		// number of digits" client bug without leaking the code to
		// anyone who can read the desktop logs.
		log.Printf("stream: pair mismatch [%s] from %s: got %d-digit code (deviceID=%q untrusted) over=%v",
			session.ID, ip, len(strings.TrimSpace(hello.PairCode)), hello.DeviceID, over)
		if over {
			session.Send(protocol.MsgError, &protocol.ErrorMessage{
				Code:    "rate_limited",
				Message: "Too many failed pair attempts. Wait a minute and try again.",
			})
		} else {
			session.Send(protocol.MsgError, &protocol.ErrorMessage{
				Code:    "pair_mismatch",
				Message: "Pair code missing or incorrect. Check the code shown on the desktop.",
			})
		}
		return
	}

	// Admitted. Authorize this client for /snapshot and /stream so the raw
	// screen frames are only served to the paired device (plus loopback
	// for the desktop's own preview).
	//
	// For secure sessions the frame token is the real authenticator: it
	// was derived from the session key and delivered inside the sealed
	// channel, so only the peer that completed the handshake holds it. The
	// IP is retained as a second constraint, but on its own it is weak —
	// it is shared behind NAT and spoofable on exactly the hostile
	// networks this work exists to defend against.
	s.frameClientMu.Lock()
	s.frameClientIP = ip
	s.frameToken = frameToken
	s.frameClientMu.Unlock()

	// Notify handler — this triggers virtual display creation.
	if err := s.handler.OnClientConnect(session, hello); err != nil {
		log.Printf("stream: connect handler error [%s]: %v", session.ID, err)
		session.Send(protocol.MsgError, &protocol.ErrorMessage{
			Code:    "setup_failed",
			Message: err.Error(),
		})
		return
	}

	// Enter read loop for input messages.
	wsHandler := &wsMessageAdapter{handler: s.handler}
	session.ReadLoop(wsHandler)
}

// wsMessageAdapter adapts SessionHandler to protocol.MessageHandler.
type wsMessageAdapter struct {
	handler SessionHandler
}

func (a *wsMessageAdapter) OnHello(session *protocol.Session, msg *protocol.HelloMessage) error {
	return nil
}

func (a *wsMessageAdapter) OnInput(session *protocol.Session, msg *protocol.InputMessage) error {
	return a.handler.OnClientInput(session, msg)
}

func (a *wsMessageAdapter) OnResize(session *protocol.Session, msg *protocol.ResizeMessage) error {
	return a.handler.OnClientResize(session, msg)
}

func (a *wsMessageAdapter) OnBye(session *protocol.Session) error {
	// Do NOT call OnClientDisconnect here. ReadLoop will return as
	// soon as the underlying conn closes (either because we close it
	// below or because the client hung up after Bye), and
	// handleWebSocket's defer is the canonical disconnect path. Calling
	// it here would invoke virtual.Destroy + capture.Stop twice, which
	// races and leaves a ghost virtual display on macOS.
	return session.Close()
}

func (a *wsMessageAdapter) OnFileOffer(session *protocol.Session, msg *protocol.FileOfferMessage) error {
	return a.handler.OnClientFileOffer(session, msg)
}

func (a *wsMessageAdapter) OnFileAccept(session *protocol.Session, msg *protocol.FileAcceptMessage) error {
	return a.handler.OnClientFileAccept(session, msg)
}

func (a *wsMessageAdapter) OnFileReject(session *protocol.Session, msg *protocol.FileRejectMessage) error {
	return a.handler.OnClientFileReject(session, msg)
}

func (a *wsMessageAdapter) OnFileChunk(session *protocol.Session, msg *protocol.FileChunkMessage) error {
	return a.handler.OnClientFileChunk(session, msg)
}

func (a *wsMessageAdapter) OnFileComplete(session *protocol.Session, msg *protocol.FileCompleteMessage) error {
	return a.handler.OnClientFileComplete(session, msg)
}

func (a *wsMessageAdapter) OnDownloadAccept(session *protocol.Session, msg *protocol.DownloadAcceptMessage) error {
	return a.handler.OnClientDownloadAccept(session, msg)
}

func (a *wsMessageAdapter) OnDownloadReject(session *protocol.Session, msg *protocol.DownloadRejectMessage) error {
	return a.handler.OnClientDownloadReject(session, msg)
}

func (a *wsMessageAdapter) OnDownloadComplete(session *protocol.Session, msg *protocol.DownloadCompleteMessage) error {
	return a.handler.OnClientDownloadComplete(session, msg)
}

// checkWSOrigin restricts WebSocket upgrades to clients that either
// (a) sent no Origin header (native mobile / desktop clients that open a
// raw WS), or (b) sent an Origin whose host resolves to a loopback,
// link-local, or RFC1918 private IP. A LAN-only relay must not accept
// upgrades initiated by a public web page — that page could otherwise
// ride a victim's pre-validated pair-code session to inject input or
// scrape frames from a phone that visits a malicious URL.
func checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isPrivateHost(u.Hostname())
}

// hostnameOnly strips an optional :port from a Host header value,
// returning just the host. Handles bare hosts, host:port, and
// [ipv6]:port. Falls back to the raw value if there's no port.
func hostnameOnly(hostport string) string {
	if hostport == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	// No port — strip surrounding brackets from a bare IPv6 literal.
	return strings.Trim(hostport, "[]")
}

// isPrivateHost reports whether host is localhost or a loopback /
// link-local / RFC1918 private IP literal. Public hostnames and public
// IPs are rejected. Empty is rejected. Shared by the WS origin check
// and the Host-header (DNS-rebinding) guard.
func isPrivateHost(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}

// corsHandler wraps an http.Handler with a same-network CORS policy.
// Capacitor WebView, the desktop Wails shell and the optional Vite dev
// server all need to load /stream, /snapshot and /download/* from the
// local server, but the previous "Access-Control-Allow-Origin: *"
// echo let any web page on the open internet fetch the same endpoints
// once a victim had ever paired — leaking screen frames, the /info
// pair code, and (with a guessed/sniffed id) downloadable files.
//
// New policy: echo back only origins that pass checkWSOrigin
// (loopback, link-local, RFC1918, capacitor://, file://). Native
// clients with no Origin header skip CORS entirely.
func corsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DNS-rebinding defense: the Origin allowlist checks the calling
		// page's host, but a rebound DNS name (attacker.com → victim LAN
		// IP) drives requests whose Host header is the attacker's domain.
		// Reject any Host that isn't a private/loopback literal or
		// localhost — a legitimate LAN client always addresses the
		// server by its IP. Hostname() strips the :port.
		if !isPrivateHost(hostnameOnly(r.Host)) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if isAllowedHTTPOrigin(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			// Disallowed origins: no CORS headers; the browser will
			// reject the response — without leaking that the endpoint
			// exists.
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// isAllowedHTTPOrigin extends checkWSOrigin with the two non-network
// schemes the Capacitor WebView uses on Android (capacitor://) and
// older builds use on file system load (file://).
func isAllowedHTTPOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u == nil {
		return false
	}
	switch u.Scheme {
	case "capacitor", "file":
		return true
	}
	r := &http.Request{Header: http.Header{}}
	r.Header.Set("Origin", origin)
	return checkWSOrigin(r)
}
