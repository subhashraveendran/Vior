// Package stream handles serving captured frames as MJPEG over HTTP,
// and manages WebSocket connections for client handshake and input forwarding.
package stream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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

// pairCode is a short hex string generated at server start. Printed to the
// terminal and exposed via /info + embedded in the QR for verification.
var pairCode = generatePairCode()

// pairAttempts tracks failed pair-code submissions per remote IP. A
// script kiddie on the LAN can otherwise brute-force a 6-char hex pair
// (16.7M combos, but file transfers are the prize) in hours by spamming
// hellos. Throttle each IP to maxPairAttempts every pairAttemptWindow.
const (
	maxPairAttempts   = 5
	pairAttemptWindow = time.Minute
)

type pairAttemptBucket struct {
	times []time.Time
}

var (
	pairAttemptsMu sync.Mutex
	pairAttempts   = map[string]*pairAttemptBucket{}
)

// recordPairAttempt registers a failed attempt from ip. Returns true if
// the IP is now over the limit and should be refused.
func recordPairAttempt(ip string) bool {
	if ip == "" {
		return false
	}
	now := time.Now()
	cutoff := now.Add(-pairAttemptWindow)
	pairAttemptsMu.Lock()
	defer pairAttemptsMu.Unlock()
	b := pairAttempts[ip]
	if b == nil {
		b = &pairAttemptBucket{}
		pairAttempts[ip] = b
	}
	// Drop expired entries.
	pruned := b.times[:0]
	for _, t := range b.times {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	pruned = append(pruned, now)
	b.times = pruned
	return len(b.times) > maxPairAttempts
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

func generatePairCode() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "000000"
	}
	return strings.ToUpper(hex.EncodeToString(b))
}

// PairCode returns the active 6-char hex pair code.
func PairCode() string { return pairCode }

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

	// stopDistribute signals the distributeFrames goroutine to stop.
	stopDistribute chan struct{}
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
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		stopDistribute: make(chan struct{}),
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
		go s.distributeFrames()
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
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
	select {
	case <-s.stopDistribute:
	default:
		close(s.stopDistribute)
	}

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
	// Stop old distributor.
	select {
	case <-s.stopDistribute:
	default:
		close(s.stopDistribute)
	}
	// Small delay for old goroutine to exit.
	time.Sleep(10 * time.Millisecond)

	s.frameCh = ch
	s.stopDistribute = make(chan struct{})
	if ch != nil {
		go s.distributeFrames()
	}
}

// ClientCount returns the number of connected MJPEG stream clients.
func (s *MJPEGServer) ClientCount() int {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	return len(s.clients)
}

// Port returns the configured port.
func (s *MJPEGServer) Port() int {
	return s.port
}

func (s *MJPEGServer) distributeFrames() {
	for {
		select {
		case <-s.stopDistribute:
			return
		case frame, ok := <-s.frameCh:
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
	s.clientsMu.Lock()
	delete(s.clients, ch)
	s.clientsMu.Unlock()
	close(ch)
}

const boundary = "vior-frame-boundary"

func (s *MJPEGServer) handleStream(w http.ResponseWriter, r *http.Request) {
	log.Printf("Client connected: %s", r.RemoteAddr)

	ch, err := s.addClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer s.removeClient(ch)

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
			log.Printf("Client disconnected: %s", r.RemoteAddr)
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
	name := friendlyDeviceName()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"name":"%s","version":"%s","platform":"%s","pairCode":"%s"}`, name, "0.1.0", friendlyPlatform(), pairCode)
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

func (s *MJPEGServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	s.frameMu.RLock()
	frame := s.currentFrame
	s.frameMu.RUnlock()

	if frame == nil {
		http.Error(w, "no frame available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(frame)
}

func (s *MJPEGServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	session := protocol.NewSession(conn)
	log.Printf("WebSocket client connected: %s [%s]", r.RemoteAddr, session.ID)

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

	// Wait for hello message.
	hello, err := session.WaitForHello()
	if err != nil {
		log.Printf("ws hello error [%s]: %v", session.ID, err)
		session.Send(protocol.MsgError, &protocol.ErrorMessage{
			Code:    "hello_failed",
			Message: err.Error(),
		})
		return
	}

	log.Printf("Client hello: %s %dx%d @%.1fx [%s]", hello.Name, hello.Width, hello.Height, hello.DPR, session.ID)

	// Admission policy:
	//   1. Already-trusted deviceID (paired before on this server)  → admit
	//   2. Correct pair code present                                → admit + add to trust store
	//   3. Otherwise                                                → reject
	// This way the user only enters the code once per physical device.
	ip := remoteIP(r.RemoteAddr)
	switch {
	case trustedDevices.IsTrusted(hello.DeviceID):
		log.Printf("ws admitted trusted device [%s] id=%s", session.ID, hello.DeviceID)
		_ = trustedDevices.Add(hello.DeviceID, hello.Name) // touch LastSeen
		clearPairAttempts(ip)
	case strings.EqualFold(strings.TrimSpace(hello.PairCode), pairCode):
		if hello.DeviceID != "" {
			if err := trustedDevices.Add(hello.DeviceID, hello.Name); err != nil {
				log.Printf("trust store add failed [%s]: %v", session.ID, err)
			} else {
				log.Printf("ws paired new device [%s] id=%s name=%q", session.ID, hello.DeviceID, hello.Name)
			}
		}
		clearPairAttempts(ip)
	default:
		// Brute-force throttle: 5 wrong codes / minute / IP → 429 +
		// close. Without this a script on the LAN could enumerate the
		// 16M-combo hex pair code in roughly a day.
		over := recordPairAttempt(ip)
		log.Printf("stream: pair mismatch [%s] from %s: got %q want %q (deviceID=%q untrusted) over=%v",
			session.ID, ip, hello.PairCode, pairCode, hello.DeviceID, over)
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

	// Notify handler — this triggers virtual display creation.
	if err := s.handler.OnClientConnect(session, hello); err != nil {
		log.Printf("ws connect handler error [%s]: %v", session.ID, err)
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

// corsHandler wraps an http.Handler with permissive CORS headers.
// Required for Capacitor WebView to load MJPEG/snapshot from local server.
func corsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		h.ServeHTTP(w, r)
	})
}
