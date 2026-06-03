package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/subhashraveendran/vior/internal/adb"
	"github.com/subhashraveendran/vior/internal/capture"
	"github.com/subhashraveendran/vior/internal/config"
	"github.com/subhashraveendran/vior/internal/discovery"
	"github.com/subhashraveendran/vior/internal/filetransfer"
	"github.com/subhashraveendran/vior/internal/input"
	"github.com/subhashraveendran/vior/internal/network"
	"github.com/subhashraveendran/vior/internal/protocol"
	"github.com/subhashraveendran/vior/internal/session"
	"github.com/subhashraveendran/vior/internal/stream"
	"github.com/subhashraveendran/vior/internal/usb"
	"github.com/subhashraveendran/vior/internal/virtual"
)

type App struct {
	ctx         context.Context
	cfg         *config.Config
	session     *capture.Session
	server      *stream.MJPEGServer
	broadcaster *discovery.Broadcaster
	touchMapper *input.TouchMapper
	fileMgr     *filetransfer.Manager
	usbAcc      *usb.Accessory
	startedAt   time.Time

	// Connected client tracking.
	client   *protocol.Session
	clientMu sync.Mutex
}

func NewApp() *App {
	return &App{
		cfg: config.Default(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) beforeClose(ctx context.Context) bool {
	a.stopEverything()
	return false
}

func (a *App) stopEverything() {
	if a.usbAcc != nil {
		a.usbAcc.Stop()
		a.usbAcc = nil
	}
	if a.broadcaster != nil {
		a.broadcaster.Stop()
		a.broadcaster = nil
	}
	if a.server != nil {
		a.server.Stop()
		a.server = nil
	}
	if a.session != nil {
		a.session.Stop()
		a.session = nil
	}
	// Destroy virtual display and wait for system to process.
	virtual.Destroy()
	time.Sleep(200 * time.Millisecond)
	a.touchMapper = nil
	if a.fileMgr != nil {
		a.fileMgr.Cleanup()
		a.fileMgr = nil
	}
	a.clientMu.Lock()
	a.client = nil
	a.clientMu.Unlock()
	// Keep same port so phone can reconnect to same URL.
}

// ── Server Lifecycle (New Flow) ─────────────────────────────────────

// StartServer starts the HTTP+WebSocket server and discovery broadcaster.
// It does NOT create a virtual display — that happens when a client connects.
func (a *App) StartServer() error {
	if a.server != nil && a.server.IsRunning() {
		return fmt.Errorf("server already running")
	}

	// Auto-select free port if not explicitly set.
	if a.cfg.Port == 0 {
		port, err := config.FreePort()
		if err != nil {
			return fmt.Errorf("failed to find free port: %w", err)
		}
		a.cfg.Port = port
	}

	// Start HTTP+WS server with no frame channel yet (will be set on client connect).
	a.server = stream.NewMJPEGServer(a.cfg.Host, a.cfg.Port, nil, a)
	if err := a.server.Start(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	a.startedAt = time.Now()
	log.Printf("Server started on port %d, waiting for client...", a.cfg.Port)

	// Start discovery broadcaster.
	if a.cfg.AutoDiscovery {
		a.broadcaster = discovery.NewBroadcaster(a.cfg.Port, a.cfg.DiscoveryPort)
		if err := a.broadcaster.Start(); err != nil {
			log.Printf("discovery broadcast failed: %v", err)
		}
	}

	// Start USB Accessory Mode scanning.
	a.usbAcc = usb.NewAccessory()
	a.usbAcc.OnConnect = func(width, height int, dpr float32) {
		log.Printf("USB: device connected %dx%d @%.1fx", width, height, dpr)
		a.handleUSBConnect(width, height, dpr)
	}
	a.usbAcc.OnTouch = func(action byte, x, y float32) {
		if a.touchMapper != nil {
			var act string
			switch action {
			case usb.TouchDown:
				act = "down"
			case usb.TouchMove:
				act = "move"
			case usb.TouchUp:
				act = "up"
			}
			a.touchMapper.HandleTouch(act, float64(x), float64(y))
		}
	}
	a.usbAcc.OnDisconnect = func() {
		log.Println("USB: device disconnected")
		if a.session != nil {
			a.session.Stop()
			a.session = nil
		}
		virtual.Destroy()
		a.touchMapper = nil
		runtime.EventsEmit(a.ctx, "client:disconnected", "usb")
	}
	if err := a.usbAcc.Start(); err != nil {
		log.Printf("USB accessory scan failed: %v", err)
	}

	return nil
}

// StopServer stops the server, capture, virtual display, and discovery.
func (a *App) StopServer() error {
	a.stopEverything()
	log.Println("Server stopped")
	return nil
}

// GetServerStatus returns current server state for the desktop frontend.
func (a *App) GetServerStatus() ServerStatus {
	s := ServerStatus{
		Port: a.cfg.Port,
	}
	if a.server != nil {
		s.Running = a.server.IsRunning()
	}
	if a.broadcaster != nil {
		s.Discovery = a.broadcaster.IsRunning()
	}
	if s.Running {
		// Build URL from first LAN IP.
		ips := discovery.LocalIPs()
		if len(ips) > 0 {
			s.URL = fmt.Sprintf("http://%s:%d", ips[0], a.cfg.Port)
		} else {
			s.URL = fmt.Sprintf("http://localhost:%d", a.cfg.Port)
		}
		qr, err := network.QRCodeDataURL(s.URL)
		if err == nil {
			s.QRCodeDataURL = qr
		}
		s.ClientCount = a.server.ClientCount()
		s.Uptime = int(time.Since(a.startedAt).Seconds())
	}

	// USB status.
	usbStatus := adb.Check()
	s.USBAvailable = usbStatus.Available
	s.USBConnected = usbStatus.Connected

	return s
}

// ── WebSocket Session Handler (implements stream.SessionHandler) ────

func (a *App) OnClientConnect(sess *protocol.Session, hello *protocol.HelloMessage) error {
	a.clientMu.Lock()
	a.client = sess
	a.clientMu.Unlock()

	log.Printf("Client connected: %s %dx%d @%.1fx mode=%s", hello.Name, hello.Width, hello.Height, hello.DPR, hello.Mode)

	// Tear down previous capture before reconfiguring.
	if a.session != nil {
		a.session.Stop()
		a.session = nil
	}

	setup, err := session.Configure(hello)
	if err != nil {
		return err
	}

	a.session = capture.NewSession(setup.DisplayIndex, a.cfg.Quality, a.cfg.FrameRate)
	if err := a.session.Start(); err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}
	a.server.SetFrameCh(a.session.FrameCh)
	a.touchMapper = input.NewTouchMapper(input.DefaultController, setup.DisplayBounds)

	sess.Send(protocol.MsgReady, &protocol.ReadyMessage{
		StreamURL:  config.DefaultStreamPath,
		Resolution: fmt.Sprintf("%dx%d", setup.Width, setup.Height),
		SessionID:  sess.ID,
	})

	runtime.EventsEmit(a.ctx, "client:connected", ClientInfo{
		SessionID:      sess.ID,
		Name:           hello.Name,
		Width:          hello.Width,
		Height:         hello.Height,
		DPR:            hello.DPR,
		ConnectedAt:    time.Now().Format(time.RFC3339),
		ConnectionType: "wifi",
	})

	return nil
}

func (a *App) OnClientResize(sess *protocol.Session, msg *protocol.ResizeMessage) error {
	log.Printf("Client resized: %dx%d @%.1fx", msg.Width, msg.Height, msg.DPR)

	if a.session != nil {
		a.session.Stop()
		a.session = nil
	}

	// Treat resize as a fresh extend-mode setup with new dimensions.
	hello := &protocol.HelloMessage{
		Width:  msg.Width,
		Height: msg.Height,
		DPR:    msg.DPR,
		Mode:   "extend",
	}
	setup, err := session.Configure(hello)
	if err != nil {
		return err
	}

	a.session = capture.NewSession(setup.DisplayIndex, a.cfg.Quality, a.cfg.FrameRate)
	if err := a.session.Start(); err != nil {
		return err
	}
	a.server.SetFrameCh(a.session.FrameCh)
	a.touchMapper = input.NewTouchMapper(input.DefaultController, setup.DisplayBounds)

	sess.Send(protocol.MsgReady, &protocol.ReadyMessage{
		StreamURL:  config.DefaultStreamPath,
		Resolution: fmt.Sprintf("%dx%d", setup.Width, setup.Height),
		SessionID:  sess.ID,
	})
	runtime.EventsEmit(a.ctx, "client:resized", map[string]any{
		"width": msg.Width, "height": msg.Height,
	})
	return nil
}

func (a *App) OnClientInput(_ *protocol.Session, msg *protocol.InputMessage) error {
	if a.touchMapper == nil {
		return nil
	}
	switch msg.Event {
	case "touch":
		return a.touchMapper.HandleTouch(msg.Action, msg.X, msg.Y)
	case "mouse":
		return a.touchMapper.HandleMouse(msg.Action, msg.DX, msg.DY)
	case "scroll":
		return a.touchMapper.HandleScroll(msg.DX, msg.DY)
	case "key":
		return input.DefaultController.TypeKey(msg.Key)
	}
	return nil
}

func (a *App) OnClientDisconnect(sess *protocol.Session) {
	log.Printf("Client disconnected: %s", sess.ID)

	a.clientMu.Lock()
	if a.client == sess {
		a.client = nil
	}
	a.clientMu.Unlock()

	if a.session != nil {
		a.session.Stop()
		a.session = nil
	}
	virtual.Destroy()
	a.touchMapper = nil

	runtime.EventsEmit(a.ctx, "client:disconnected", sess.ID)
}

// ── USB Connection Handler ──────────────────────────────────────────

func (a *App) handleUSBConnect(width, height int, dpr float32) {
	// Check permissions.
	if a.session != nil {
		a.session.Stop()
		a.session = nil
	}

	setup, err := session.Configure(&protocol.HelloMessage{
		Width:  width,
		Height: height,
		DPR:    float64(dpr),
		Mode:   "extend",
	})
	if err != nil {
		log.Printf("USB: configure failed: %v", err)
		return
	}

	a.session = capture.NewSession(setup.DisplayIndex, a.cfg.Quality, a.cfg.FrameRate)
	if err := a.session.Start(); err != nil {
		log.Printf("USB: capture failed: %v", err)
		return
	}
	a.touchMapper = input.NewTouchMapper(input.DefaultController, setup.DisplayBounds)

	// Send ready.
	a.usbAcc.SendReady(width, height)

	// Stream frames over USB.
	go func() {
		for frame := range a.session.FrameCh {
			if err := a.usbAcc.SendFrame(frame); err != nil {
				log.Printf("USB: send frame error: %v", err)
				return
			}
		}
	}()

	// Also hook up to MJPEG server if running (for web client fallback).
	if a.server != nil {
		a.server.SetFrameCh(a.session.FrameCh)
	}

	runtime.EventsEmit(a.ctx, "client:connected", ClientInfo{
		SessionID:      "usb",
		Name:           "Android (USB)",
		Width:          width,
		Height:         height,
		DPR:            float64(dpr),
		ConnectedAt:    time.Now().Format(time.RFC3339),
		ConnectionType: "usb",
	})
}

// ── Display ──────────────────────────────────────────────────────────

type DisplayInfo struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	IsMain bool   `json:"isMain"`
}

func (a *App) ListDisplays() ([]DisplayInfo, error) {
	displays, err := capture.ListDisplays()
	if err != nil {
		return nil, err
	}
	out := make([]DisplayInfo, len(displays))
	for i, d := range displays {
		out[i] = DisplayInfo{
			Index:  d.Index,
			Name:   d.Name,
			Width:  d.Width,
			Height: d.Height,
			IsMain: d.IsMain,
		}
	}
	return out, nil
}

// ── Legacy Stream Control (kept for backward compat with CLI mode) ──

type StreamConfig struct {
	DisplayIndex int `json:"displayIndex"`
	Port         int `json:"port"`
	Quality      int `json:"quality"`
	FPS          int `json:"fps"`
}

type StreamStatus struct {
	Running      bool   `json:"running"`
	DisplayIndex int    `json:"displayIndex"`
	Port         int    `json:"port"`
	URL          string `json:"url"`
}

func (a *App) StartStream(sc StreamConfig) error {
	if a.server != nil && a.server.IsRunning() {
		return fmt.Errorf("stream already running on port %d", a.cfg.Port)
	}

	displays, err := capture.ListDisplays()
	if err != nil {
		return fmt.Errorf("failed to list displays: %w", err)
	}
	if sc.DisplayIndex < 0 || sc.DisplayIndex >= len(displays) {
		return fmt.Errorf("display %d out of range (0-%d)", sc.DisplayIndex, len(displays)-1)
	}

	a.cfg.DisplayIndex = sc.DisplayIndex
	a.cfg.Port = sc.Port
	a.cfg.Quality = sc.Quality
	a.cfg.FrameRate = sc.FPS

	a.session = capture.NewSession(a.cfg.DisplayIndex, a.cfg.Quality, a.cfg.FrameRate)
	if err := a.session.Start(); err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}

	a.server = stream.NewMJPEGServer(a.cfg.Host, a.cfg.Port, a.session.FrameCh, nil)
	if err := a.server.Start(); err != nil {
		a.session.Stop()
		return fmt.Errorf("server failed: %w", err)
	}

	log.Printf("Stream started on port %d (display %d)", a.cfg.Port, a.cfg.DisplayIndex)
	return nil
}

func (a *App) StopStream() error {
	if a.server == nil || !a.server.IsRunning() {
		return fmt.Errorf("no stream running")
	}
	a.session.Stop()
	if err := a.server.Stop(); err != nil {
		return fmt.Errorf("server stop error: %w", err)
	}
	a.session = nil
	a.server = nil
	log.Println("Stream stopped")
	return nil
}

func (a *App) GetStreamStatus() StreamStatus {
	s := StreamStatus{}
	if a.server != nil {
		s.Running = a.server.IsRunning()
	}
	s.DisplayIndex = a.cfg.DisplayIndex
	s.Port = a.cfg.Port
	if s.Running {
		s.URL = fmt.Sprintf("http://localhost:%d", a.cfg.Port)
	}
	return s
}

// ── Virtual Display ──────────────────────────────────────────────────

type VirtualDisplayConfig struct {
	Width       uint32  `json:"width"`
	Height      uint32  `json:"height"`
	RefreshRate float64 `json:"refreshRate"`
	HiDPI       bool    `json:"hidpi"`
}

func (a *App) CreateVirtualDisplay(vdc VirtualDisplayConfig) (uint32, error) {
	if vdc.HiDPI {
		return virtual.CreateHiDPI(vdc.Width, vdc.Height, vdc.RefreshRate)
	}
	return virtual.Create(vdc.Width, vdc.Height, vdc.RefreshRate)
}

func (a *App) DestroyVirtualDisplay() {
	virtual.Destroy()
}

// ── Display Mode ────────────────────────────────────────────────────

func (a *App) MirrorDisplay(source, target int) error {
	return capture.MirrorDisplay(source, target)
}

func (a *App) ExtendDisplay(displayIndex int) error {
	return capture.UnmirrorDisplay(displayIndex)
}

func (a *App) IsMirrored(displayIndex int) (bool, error) {
	return capture.IsMirrored(displayIndex)
}

// ── Config ──────────────────────────────────────────────────────────

type AppConfig struct {
	Port        int    `json:"port"`
	Quality     int    `json:"quality"`
	FrameRate   int    `json:"frameRate"`
	Host        string `json:"host"`
	TransferDir string `json:"transferDir"`
}

func (a *App) GetConfig() AppConfig {
	return AppConfig{
		Port:        a.cfg.Port,
		Quality:     a.cfg.Quality,
		FrameRate:   a.cfg.FrameRate,
		Host:        a.cfg.Host,
		TransferDir: a.cfg.TransferDir,
	}
}

func (a *App) UpdateConfig(ac AppConfig) {
	a.cfg.Port = ac.Port
	a.cfg.Quality = ac.Quality
	a.cfg.FrameRate = ac.FrameRate
	a.cfg.Host = ac.Host
	a.cfg.TransferDir = ac.TransferDir
}

func (a *App) ResetConfig() {
	a.cfg = config.Default()
}

// ── USB/ADB ─────────────────────────────────────────────────────────

func (a *App) GetUSBStatus() adb.Status {
	return adb.Check()
}

func (a *App) SetupUSB() error {
	// Auto-download ADB if not available.
	if !adb.IsAvailable() {
		if err := adb.EnsureADB(); err != nil {
			return fmt.Errorf("failed to install ADB: %w", err)
		}
	}
	status := adb.Check()
	if !status.Available {
		return fmt.Errorf("adb not available")
	}
	if !status.Connected {
		return fmt.Errorf("no Android device connected — plug in via USB and enable USB debugging")
	}
	return adb.SetupForward(a.cfg.Port, a.cfg.Port)
}

func (a *App) TeardownUSB() error {
	return adb.TeardownForward(a.cfg.Port)
}

// DownloadADB downloads ADB if not already available. Called from Settings UI.
func (a *App) DownloadADB() error {
	return adb.EnsureADB()
}

// ── Connected Clients ───────────────────────────────────────────────

type ClientInfo struct {
	SessionID      string  `json:"sessionId"`
	Name           string  `json:"name"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	DPR            float64 `json:"dpr"`
	ConnectedAt    string  `json:"connectedAt"`
	ConnectionType string  `json:"connectionType"`
}

func (a *App) GetConnectedClients() []ClientInfo {
	a.clientMu.Lock()
	defer a.clientMu.Unlock()
	if a.client == nil || a.client.Hello == nil {
		return []ClientInfo{}
	}
	return []ClientInfo{{
		SessionID:      a.client.ID,
		Name:           a.client.Hello.Name,
		Width:          a.client.Hello.Width,
		Height:         a.client.Hello.Height,
		DPR:            a.client.Hello.DPR,
		ConnectedAt:    a.client.CreatedAt.Format(time.RFC3339),
		ConnectionType: "wifi",
	}}
}

// ── Server Status ───────────────────────────────────────────────────

type ServerStatus struct {
	Running       bool   `json:"running"`
	Port          int    `json:"port"`
	URL           string `json:"url"`
	QRCodeDataURL string `json:"qrCodeDataUrl"`
	ClientCount   int    `json:"clientCount"`
	Discovery     bool   `json:"discovery"`
	USBAvailable  bool   `json:"usbAvailable"`
	USBConnected  bool   `json:"usbConnected"`
	Uptime        int    `json:"uptime"`
}

// ── File Transfer ───────────────────────────────────────────────────

func (a *App) ensureFileMgr() {
	if a.fileMgr != nil {
		return
	}
	home, _ := os.UserHomeDir()
	receiveDir := filepath.Join(home, "Downloads", "Vior")
	a.fileMgr = filetransfer.NewManager(receiveDir)
	a.fileMgr.Send = func(msgType protocol.MessageType, data any) error {
		a.clientMu.Lock()
		c := a.client
		a.clientMu.Unlock()
		if c == nil {
			return fmt.Errorf("no client connected")
		}
		return c.Send(msgType, data)
	}
	a.fileMgr.OnFileReceived = func(t *filetransfer.Transfer) {
		runtime.EventsEmit(a.ctx, "file:received", map[string]any{
			"id": t.ID, "name": t.Name, "path": t.Path, "size": t.Transferred,
			"mimeType": t.MimeType, "preview": t.Preview,
		})
	}
	a.fileMgr.OnFileOffer = func(t *filetransfer.Transfer) {
		runtime.EventsEmit(a.ctx, "file:offer", map[string]any{
			"id": t.ID, "name": t.Name, "size": t.Size,
			"mimeType": t.MimeType, "preview": t.Preview,
		})
	}
}

// PickAndSendFile opens a native file picker and sends the selected file.
func (a *App) PickAndSendFile() error {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select file to send",
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // cancelled
	}
	return a.SendFile(path)
}

// SendFile initiates a file transfer to the connected phone.
func (a *App) SendFile(path string) error {
	a.ensureFileMgr()
	_, err := a.fileMgr.OfferFile(path)
	return err
}

// AcceptIncomingFile accepts a pending file offer from the phone.
func (a *App) AcceptIncomingFile(transferID string) error {
	if a.fileMgr == nil {
		return fmt.Errorf("no file manager")
	}
	return a.fileMgr.AcceptFile(transferID)
}

// RejectIncomingFile rejects a pending file offer from the phone.
func (a *App) RejectIncomingFile(transferID string) error {
	if a.fileMgr == nil {
		return fmt.Errorf("no file manager")
	}
	return a.fileMgr.RejectFile(transferID, "rejected by user")
}

// GetActiveTransfers returns all active file transfers.
func (a *App) GetActiveTransfers() []map[string]any {
	if a.fileMgr == nil {
		return nil
	}
	transfers := a.fileMgr.ActiveTransfers()
	result := make([]map[string]any, len(transfers))
	for i, t := range transfers {
		result[i] = map[string]any{
			"id": t.ID, "name": t.Name, "size": t.Size,
			"transferred": t.Transferred, "complete": t.Complete,
			"mimeType": t.MimeType, "preview": t.Preview,
		}
	}
	return result
}

// File transfer WebSocket handlers.
func (a *App) OnClientFileOffer(session *protocol.Session, msg *protocol.FileOfferMessage) error {
	a.ensureFileMgr()
	a.fileMgr.HandleOffer(msg)
	return nil
}

func (a *App) OnClientFileAccept(session *protocol.Session, msg *protocol.FileAcceptMessage) error {
	if a.fileMgr != nil {
		a.fileMgr.HandleAccept(msg)
	}
	return nil
}

func (a *App) OnClientFileReject(session *protocol.Session, msg *protocol.FileRejectMessage) error {
	if a.fileMgr != nil {
		a.fileMgr.HandleReject(msg)
	}
	return nil
}

func (a *App) OnClientFileChunk(session *protocol.Session, msg *protocol.FileChunkMessage) error {
	if a.fileMgr != nil {
		a.fileMgr.HandleChunk(msg)
	}
	return nil
}

func (a *App) OnClientFileComplete(session *protocol.Session, msg *protocol.FileCompleteMessage) error {
	if a.fileMgr != nil {
		a.fileMgr.HandleComplete(msg)
	}
	return nil
}

// ── Snapshot ─────────────────────────────────────────────────────────

func (a *App) TakeSnapshot(displayIndex int) ([]byte, error) {
	return capture.CaptureFrame(displayIndex, a.cfg.Quality)
}

// ── Permissions ─────────────────────────────────────────────────────

// CheckPermissions verifies screen recording permission on macOS.
func (a *App) CheckPermissions() error {
	return capture.CheckScreenRecordingPermission()
}

// ── Version ─────────────────────────────────────────────────────────

func (a *App) GetVersion() string {
	return config.Version
}
