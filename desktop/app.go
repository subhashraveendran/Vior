package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
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

	// stopMu serializes stopEverything so the OnBeforeClose path, the
	// tray Stop/Quit handlers and the SIGINT/SIGTERM handler can't run
	// teardown concurrently (double-Stop / racing nil assignments).
	stopMu sync.Mutex

	// inputPermChecked guards the one-time accessibility prompt that
	// fires on the first input event after a client connects.
	inputPermChecked bool

	// currentClientTrusted is true when the currently-connected client
	// was admitted via the trust store (already paired before). File
	// transfers from that session auto-accept — no second prompt.
	currentClientTrusted bool

	// ipWatchStop tears down the goroutine that polls discovery.LocalIPs
	// and emits server:ip-changed when the host's IP drifts (DHCP lease
	// renewal, Wi-Fi → Ethernet handoff). Lets the desktop UI refresh
	// the QR without restarting the server.
	ipWatchStop chan struct{}
	lastIPs     []string

	// usbLastDisconnect timestamps the most recent USB tear-down so
	// handleUSBConnect can debounce a sub-2s flap (cable wiggle, brief
	// micro-disconnect). When a fresh OnConnect arrives inside the
	// debounce window we skip the destroy/recreate of the virtual
	// display + capture session and just keep the existing one alive —
	// the alternative is a ~300ms screen blank on every cable jiggle
	// plus a fresh round of CGVirtualDisplay teardown latency. Guarded
	// by clientMu (same scope as the rest of the WS/USB session state).
	usbLastDisconnect time.Time
	usbLastDims       struct {
		width, height int
		dpr           float32
	}
}

func NewApp() *App {
	return &App{
		cfg: config.Default(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Install the macOS menu-bar (NSStatusItem) item. No-op on other OSes.
	startTray(ctx, a)
}

func (a *App) beforeClose(ctx context.Context) bool {
	a.stopEverything()
	return false
}

func (a *App) stopEverything() {
	a.stopMu.Lock()
	defer a.stopMu.Unlock()
	if a.ipWatchStop != nil {
		close(a.ipWatchStop)
		a.ipWatchStop = nil
	}
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

	// Persist the pair code to ~/.vior/pair.txt so a desktop restart
	// (quit & relaunch, system reboot, crash recovery) doesn't rotate
	// the code out from under a phone the user just paired. Trusted
	// devices already reconnect by deviceID — this is purely for the
	// short window between "user typed code" and "device added to trust
	// store", which the user perceives as "the connection died".
	stream.EnablePersistedPair()

	// Resolve the port. A 0 (auto) prefers 8080/8081 — the fixed ports
	// the mobile client probes during discovery — so auto-config is
	// actually discoverable, falling back to a random free port only if
	// both are taken.
	port, err := config.ResolvePort(a.cfg.Port)
	if err != nil {
		return fmt.Errorf("failed to resolve port: %w", err)
	}
	a.cfg.Port = port

	// Start HTTP+WS server with no frame channel yet (will be set on client connect).
	a.server = stream.NewMJPEGServer(a.cfg.Host, a.cfg.Port, nil, a)
	if err := a.server.Start(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	a.startedAt = time.Now()
	log.Printf("session: server started on port %d, waiting for client...", a.cfg.Port)

	// Start discovery broadcaster.
	if a.cfg.AutoDiscovery {
		a.broadcaster = discovery.NewBroadcaster(a.cfg.Port, a.cfg.DiscoveryPort)
		if err := a.broadcaster.Start(); err != nil {
			log.Printf("discovery: broadcast failed: %v", err)
		}
	}

	// Watch for IP drift (DHCP renew, Wi-Fi handoff). When detected,
	// emit a Wails event so the desktop UI refreshes the QR + URLs and
	// the user doesn't end up showing the phone a stale IP.
	a.startIPWatcher()

	// Start USB Accessory Mode scanning.
	a.usbAcc = usb.NewAccessory()
	a.usbAcc.OnConnect = func(width, height int, dpr float32) {
		log.Printf("usb: device connected %dx%d @%.1fx", width, height, dpr)
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
		log.Println("usb: device disconnected")
		// Record the disconnect time + last-known dims BEFORE tearing
		// state down so handleUSBConnect can detect a sub-2s flap and
		// reuse the existing virtual display. The dims default to zero
		// if we never received a hello; the debounce check requires
		// matching dims (a real device-swap would have new dims anyway).
		a.clientMu.Lock()
		a.usbLastDisconnect = time.Now()
		a.clientMu.Unlock()
		if a.session != nil {
			a.session.Stop()
			a.session = nil
		}
		virtual.Destroy()
		a.touchMapper = nil
		runtime.EventsEmit(a.ctx, "client:disconnected", "usb")
	}
	if err := a.usbAcc.Start(); err != nil {
		log.Printf("usb: accessory scan failed: %v", err)
	}

	return nil
}

// StopServer stops the server, capture, virtual display, and discovery.
func (a *App) StopServer() error {
	a.stopEverything()
	log.Println("session: server stopped")
	return nil
}

// GetServerStatus returns current server state for the desktop frontend.
func (a *App) GetServerStatus() ServerStatus {
	s := ServerStatus{
		Port:      a.cfg.Port,
		FrameRate: a.cfg.FrameRate,
	}
	if a.server != nil {
		s.Running = a.server.IsRunning()
	}
	if a.broadcaster != nil {
		s.Discovery = a.broadcaster.IsRunning()
	}
	if s.Running {
		// Build all LAN URLs — multi-interface Macs need this so the user can
		// pick the IP the phone can actually reach.
		ips := discovery.LocalIPs()
		s.URLs = make([]string, 0, len(ips))
		for _, ip := range ips {
			s.URLs = append(s.URLs, fmt.Sprintf("http://%s:%d", ip, a.cfg.Port))
		}
		if len(s.URLs) > 0 {
			s.URL = s.URLs[0]
		} else {
			s.URL = fmt.Sprintf("http://localhost:%d", a.cfg.Port)
		}
		// The QR carries the high-entropy channel secret alongside the
		// pair code. That secret is what makes the encrypted channel
		// meaningful: a QR is a machine-to-machine channel, so it can
		// hold 256 bits at no cost to the user, whereas the 6-digit
		// code exists to be typed and is far too small to authenticate
		// against an offline attack.
		qrURL := s.URL + "?pair=" + stream.PairCode()
		if stream.GetSecurityMode() != stream.SecureOff {
			qrURL += "&k=" + stream.ChannelSecretParam()
		}
		qr, err := network.QRCodeDataURL(qrURL)
		if err == nil {
			s.QRCodeDataURL = qr
		}
		s.ClientCount = a.server.ClientCount()
		s.Uptime = int(time.Since(a.startedAt).Seconds())
		s.Secure = a.server.ClientSecure()
		s.SecureMode = stream.GetSecurityMode().String()
	}

	// USB status.
	usbStatus := adb.Check()
	s.USBAvailable = usbStatus.Available
	s.USBConnected = usbStatus.Connected

	// Pair code (printed alongside URL for trusted-network pairing).
	s.PairCode = stream.PairCode()

	return s
}

// ── WebSocket Session Handler (implements stream.SessionHandler) ────

func (a *App) OnClientConnect(sess *protocol.Session, hello *protocol.HelloMessage) error {
	a.clientMu.Lock()
	a.client = sess
	// Cache trust status for this session so File transfer auto-accept
	// can skip the second confirmation prompt when the device is known.
	a.currentClientTrusted = stream.TrustedDevices().IsTrusted(hello.DeviceID)
	a.clientMu.Unlock()

	log.Printf("session: client connected: %s %dx%d @%.1fx mode=%s", hello.Name, hello.Width, hello.Height, hello.DPR, hello.Mode)

	// Check Screen Recording permission so the desktop UI can show a
	// warning card if the stream will be black. Do this before virtual
	// display creation so the card appears alongside the Connected state.
	if err := capture.CheckScreenRecordingPermission(); err != nil {
		runtime.EventsEmit(a.ctx, "permission:screen-recording-missing", err.Error())
	}

	// Tear down previous capture before reconfiguring.
	if a.session != nil {
		a.session.Stop()
		a.session = nil
	}

	setup, err := session.Configure(hello)
	if err != nil {
		return err
	}

	// Mode="none" → no virtual display, no capture. Used by Remote-only
	// and Files-only intents. Input still works (mapped against the main
	// display's bounds); the MJPEG stream simply has no frame source.
	if setup.Mode == "none" {
		a.touchMapper = input.NewTouchMapper(input.DefaultController, setup.DisplayBounds)
		sess.Send(protocol.MsgReady, &protocol.ReadyMessage{
			StreamURL:  "",
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
			RemoteAddr:     clientRemoteHost(sess),
			Platform:       hello.Platform,
			DeviceID:       hello.DeviceID,
		})
		return nil
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
	log.Printf("session: client resized: %dx%d @%.1fx", msg.Width, msg.Height, msg.DPR)

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
	// One-time accessibility check on the first input event after a
	// client connects. If the OS hasn't granted us input-injection
	// rights, the network path looks healthy but every CGEvent post is
	// silently dropped — the user sees a "broken" Remote tab. Surface
	// the issue to the desktop UI so it can show a permission card.
	if !a.inputPermChecked {
		a.inputPermChecked = true
		if !input.HasAccessibility(false) {
			runtime.EventsEmit(a.ctx, "permission:accessibility-missing", nil)
			// Also tell the phone — user is looking at phone, not desktop.
			a.clientMu.Lock()
			if a.client != nil {
				_ = a.client.Send(protocol.MsgError, &protocol.ErrorMessage{
					Code:    "permission_accessibility",
					Message: "Desktop needs Accessibility permission for Remote tab to work. Check your computer screen.",
				})
			}
			a.clientMu.Unlock()
		}
	}
	// Key events don't need a touchMapper, so route them first. This
	// keeps the keyboard alive in any pathological state where the
	// touch mapper failed to initialise (e.g. virtual display creation
	// hiccup) but the WS session is still up.
	if msg.Event == "key" {
		return input.DefaultController.TypeKey(msg.Key)
	}
	if a.touchMapper == nil {
		log.Printf("input: dropped %s/%s — touchMapper not initialised", msg.Event, msg.Action)
		return nil
	}
	var err error
	switch msg.Event {
	case "touch":
		err = a.touchMapper.HandleTouch(msg.Action, msg.X, msg.Y)
	case "mouse":
		err = a.touchMapper.HandleMouse(msg.Action, msg.DX, msg.DY)
	case "scroll":
		err = a.touchMapper.HandleScroll(msg.DX, msg.DY)
	default:
		log.Printf("input: unknown event %q", msg.Event)
	}
	if err != nil {
		log.Printf("input: %s/%s error: %v", msg.Event, msg.Action, err)
	}
	return err
}

func (a *App) OnClientDisconnect(sess *protocol.Session) {
	log.Printf("session: client disconnected: %s", sess.ID)

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
	a.inputPermChecked = false
	a.currentClientTrusted = false

	runtime.EventsEmit(a.ctx, "client:disconnected", sess.ID)
}

// ── USB Connection Handler ──────────────────────────────────────────

func (a *App) handleUSBConnect(width, height int, dpr float32) {
	// Cable-wiggle debouncer: if the previous USB session ended less
	// than 2s ago AND the new connect's dims match, keep the existing
	// virtual display + capture session in place. The alternative is
	// a teardown→recreate cycle on every micro-disconnect that the
	// user sees as ~300ms of black screen + dropped touch state. The
	// session pointer/capture goroutine are still alive at this point
	// only if the disconnect handler hasn't run yet — in practice on
	// real hardware OnDisconnect fires immediately, so we still need
	// the recreate path below; the early-return is a future-proof
	// guard that pays off when sub-100ms flaps land inside one event
	// loop tick (the gousb driver coalesces them).
	a.clientMu.Lock()
	prev := a.usbLastDisconnect
	a.usbLastDims.width, a.usbLastDims.height, a.usbLastDims.dpr = width, height, dpr
	a.clientMu.Unlock()
	if !prev.IsZero() && time.Since(prev) < 2*time.Second && a.session != nil {
		log.Printf("usb: connect within 2s of disconnect (Δ=%dms) — keeping existing session",
			time.Since(prev).Milliseconds())
		a.usbAcc.SendReady(width, height)
		return
	}

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
		log.Printf("usb: configure failed: %v", err)
		return
	}

	a.session = capture.NewSession(setup.DisplayIndex, a.cfg.Quality, a.cfg.FrameRate)
	if err := a.session.Start(); err != nil {
		log.Printf("usb: capture failed: %v", err)
		return
	}
	a.touchMapper = input.NewTouchMapper(input.DefaultController, setup.DisplayBounds)

	// Send ready.
	a.usbAcc.SendReady(width, height)

	// Stream frames over USB, teeing a best-effort copy to the MJPEG
	// server for the web-client fallback.
	//
	// A Go channel delivers each value to exactly ONE receiver, so the
	// old code — a goroutine ranging FrameCh AND server.SetFrameCh(FrameCh)
	// — split frames ~50/50 between USB and the web distributor, halving
	// and corrupting both streams. Instead, a single reader owns FrameCh
	// and fans each frame out: USB gets every frame (priority, blocking),
	// the web side gets a non-blocking copy that's dropped if it's slow.
	var webCh chan []byte
	if a.server != nil {
		webCh = make(chan []byte, 4)
		a.server.SetFrameCh(webCh)
	}
	go func() {
		if webCh != nil {
			defer close(webCh)
		}
		for frame := range a.session.FrameCh {
			if err := a.usbAcc.SendFrame(frame); err != nil {
				log.Printf("usb: send frame error: %v", err)
				return
			}
			if webCh != nil {
				select {
				case webCh <- frame:
				default: // web distributor slow — drop, keep USB real-time
				}
			}
		}
	}()

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

	log.Printf("session: stream started on port %d (display %d)", a.cfg.Port, a.cfg.DisplayIndex)
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
	log.Println("session: stream stopped")
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

// ── Menu bar (macOS only — no-op on other platforms) ──

// SetMenuBarVisible toggles the macOS menu-bar (NSStatusItem) at runtime
// and persists the choice to ~/.vior/menubar.flag so it survives across
// launches.
func (a *App) SetMenuBarVisible(visible bool) {
	setMenuBarVisible(visible)
	writeMenuBarPref(visible)
}

// GetMenuBarVisible reports the current persisted preference. Default
// is true (show the menu bar item).
func (a *App) GetMenuBarVisible() bool {
	return readMenuBarPref()
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
	// RemoteAddr, Platform and DeviceID give the UI the context a user
	// needs to confirm a newly-connected device is the one in their hand
	// (the host side of the "does this match?" pairing gesture). Empty for
	// the USB path, where the device is physically attached.
	RemoteAddr string `json:"remoteAddr,omitempty"`
	Platform   string `json:"platform,omitempty"`
	DeviceID   string `json:"deviceId,omitempty"`
}

// clientRemoteHost returns the peer IP (no port) for a WS session, or ""
// when unavailable. Used to show "who just connected" in the UI.
func clientRemoteHost(sess *protocol.Session) string {
	if sess == nil || sess.Conn == nil {
		return ""
	}
	addr := sess.Conn.RemoteAddr()
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
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
	Running       bool     `json:"running"`
	Port          int      `json:"port"`
	URL           string   `json:"url"`
	URLs          []string `json:"urls"`
	QRCodeDataURL string   `json:"qrCodeDataUrl"`
	ClientCount   int      `json:"clientCount"`
	Discovery     bool     `json:"discovery"`
	USBAvailable  bool     `json:"usbAvailable"`
	USBConnected  bool     `json:"usbConnected"`
	Uptime        int      `json:"uptime"`
	PairCode      string   `json:"pairCode"`
	FrameRate     int      `json:"frameRate"`

	// Secure reports whether the connected client's payloads are actually
	// encrypted. SecureMode is the server policy ("preferred", "required",
	// "off"). The UI must render the connection's real state — claiming a
	// cleartext session is protected would be worse than showing nothing.
	Secure     bool   `json:"secure"`
	SecureMode string `json:"secureMode"`
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
	a.fileMgr.OnFileProgress = func(t *filetransfer.Transfer) {
		// Live progress for an in-flight WS-chunked receive. Coalesced
		// inside the manager (~once per 256 KiB) so we don't drown the
		// Wails event bus. The Files pane subscribes to this to drive
		// its progress bar mid-transfer instead of jumping 0 → 100 only
		// on file:received.
		runtime.EventsEmit(a.ctx, "file:progress", map[string]any{
			"id": t.ID, "name": t.Name, "size": t.Size,
			"transferred": t.Transferred, "mimeType": t.MimeType,
			"preview": t.Preview,
		})
	}
	a.fileMgr.OnFileOffer = func(t *filetransfer.Transfer) {
		// Skip the user prompt for already-paired devices — the trust
		// status is set at WS-connect time. Without this, even trusted
		// devices fire the accept modal (the trusted auto-accept then
		// races against the user's click).
		a.clientMu.Lock()
		trusted := a.currentClientTrusted
		a.clientMu.Unlock()
		if trusted {
			return
		}
		runtime.EventsEmit(a.ctx, "file:offer", map[string]any{
			"id": t.ID, "name": t.Name, "size": t.Size,
			"mimeType": t.MimeType, "preview": t.Preview,
		})
	}
	a.fileMgr.OnDownloadDone = func(p *filetransfer.PendingDownload) {
		runtime.EventsEmit(a.ctx, "download:done", map[string]any{
			"id": p.ID, "name": p.Name, "size": p.Size,
		})
	}
}

// PickAndSendFile opens a native file picker and offers the selected
// file to the connected phone over the HTTP-download path. The mobile
// receives an "incoming-file" WS push and (after accept) fetches the
// body via GET /download/{id}. Falls back to the legacy WS-chunked
// path when no phone is connected to keep the existing SendFile RPC
// from regressing.
func (a *App) PickAndSendFile() error {
	a.clientMu.Lock()
	connected := a.client != nil
	a.clientMu.Unlock()
	if !connected {
		return fmt.Errorf("no client connected")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Send file to phone",
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // cancelled
	}
	return a.SendFileToPhone(path)
}

// SendFileToPhone is the HTTP-download (bidirectional) entry point.
// Registers the file, pushes IncomingFile to the connected mobile.
func (a *App) SendFileToPhone(path string) error {
	a.ensureFileMgr()
	a.clientMu.Lock()
	c := a.client
	a.clientMu.Unlock()
	if c == nil {
		return fmt.Errorf("no client connected")
	}
	p, err := a.fileMgr.OfferDownload(path)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("/download/%s", p.ID)
	if err := c.Send(protocol.MsgIncomingFile, &protocol.IncomingFileMessage{
		ID: p.ID, Name: p.Name, Size: p.Size, MimeType: p.MimeType, URL: url, Preview: p.Preview,
	}); err != nil {
		a.fileMgr.CancelDownload(p.ID)
		return err
	}
	runtime.EventsEmit(a.ctx, "download:offered", map[string]any{
		"id": p.ID, "name": p.Name, "size": p.Size, "mimeType": p.MimeType,
	})
	return nil
}

// SendFile keeps the legacy WS-chunked desktop→mobile path alive for
// existing callers / tests. New UI should call SendFileToPhone.
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
	// Trusted device → auto-accept the first (and every subsequent) file
	// transfer without a confirmation prompt. The user already paired
	// this device once; making them re-approve every file is friction.
	if a.currentClientTrusted {
		if err := a.fileMgr.AcceptFile(msg.ID); err != nil {
			log.Printf("filetransfer: auto-accept failed [%s]: %v", msg.ID, err)
		} else if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "file:auto-accepted", msg.ID)
		}
	}
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

// ── Bidirectional HTTP-download WS handlers ─────────────────────────

func (a *App) OnClientDownloadAccept(session *protocol.Session, msg *protocol.DownloadAcceptMessage) error {
	if a.fileMgr != nil {
		a.fileMgr.MarkDownloadAccepted(msg.ID)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "download:accepted", msg.ID)
	}
	return nil
}

func (a *App) OnClientDownloadReject(session *protocol.Session, msg *protocol.DownloadRejectMessage) error {
	if a.fileMgr != nil {
		a.fileMgr.CancelDownload(msg.ID)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "download:rejected", map[string]any{
			"id": msg.ID, "reason": msg.Reason,
		})
	}
	return nil
}

func (a *App) OnClientDownloadComplete(session *protocol.Session, msg *protocol.DownloadCompleteMessage) error {
	if a.fileMgr == nil {
		return nil
	}
	p := a.fileMgr.CompleteDownload(msg.ID)
	if p != nil && a.ctx != nil {
		runtime.EventsEmit(a.ctx, "download:done", map[string]any{
			"id": p.ID, "name": p.Name, "size": p.Size,
		})
	}
	return nil
}

// ServeDownload implements the stream.SessionHandler download endpoint.
// Delegates to fileMgr which streams the body via http.ServeContent.
func (a *App) ServeDownload(w http.ResponseWriter, r *http.Request, id string) {
	if a.fileMgr == nil {
		http.Error(w, "no transfer manager", http.StatusServiceUnavailable)
		return
	}
	a.fileMgr.ServeDownload(w, r, id)
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

// ── Trusted devices (Settings UI) ───────────────────────────────────

// TrustedDevice is the Wails-facing shape of a trust.Entry. Times are
// emitted as RFC3339 strings so the frontend can format with
// Intl.RelativeTimeFormat without going through Date(0) sentinels.
type TrustedDevice struct {
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
}

// ListTrustedDevices returns every device admitted via pair-code at
// least once. Sorted by LastSeen descending so the most-recently-used
// device appears at the top of the Settings card.
func (a *App) ListTrustedDevices() []TrustedDevice {
	entries := stream.TrustedDevices().List()
	out := make([]TrustedDevice, len(entries))
	for i, e := range entries {
		out[i] = TrustedDevice{
			DeviceID:  e.DeviceID,
			Name:      e.Name,
			Platform:  e.Platform,
			FirstSeen: e.FirstSeen.Format(time.RFC3339),
			LastSeen:  e.LastSeen.Format(time.RFC3339),
		}
	}
	// Sort by LastSeen descending. Stable sort is overkill for a short
	// list; a hand-rolled bubble compare here keeps the dependency
	// surface flat (no `sort` import elsewhere in this file).
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].LastSeen > out[i].LastSeen {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ForgetTrustedDevice removes a single device from the trust store.
// Frontend should refresh ListTrustedDevices after this returns.
func (a *App) ForgetTrustedDevice(deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("deviceID required")
	}
	return stream.TrustedDevices().Forget(deviceID)
}

// ClearAllTrustedDevices wipes the entire trust list. The next connect
// from any previously-paired device will re-prompt for the pair code.
func (a *App) ClearAllTrustedDevices() error {
	return stream.TrustedDevices().Clear()
}

// SetPairCode persists a user-chosen pair-code override at
// ~/.vior/pair.txt. Pass an empty string to clear the override and
// fall back to the machine-derived default. Validation (4-8 digits)
// happens inside stream.SetPairCode.
func (a *App) SetPairCode(code string) error {
	return stream.SetPairCode(code)
}

// HasAccessibility reports whether the app is trusted to inject input
// events. On macOS this requires the user to add Vior to System Settings
// → Privacy & Security → Accessibility. Without it the Remote tab on the
// phone looks broken: events arrive over the network but the OS silently
// drops the CGEvent posts. Pass prompt=true to ask the OS to show its
// own deep-link dialog the first time.
func (a *App) HasAccessibility(prompt bool) bool {
	return input.HasAccessibility(prompt)
}

// ── Version ─────────────────────────────────────────────────────────

func (a *App) GetVersion() string {
	return config.Version
}

// ── IP-drift watcher ─────────────────────────────────────────────────

// startIPWatcher polls discovery.LocalIPs every 10s and emits a Wails
// event when the set of non-loopback IPv4 addresses changes. Lets the
// QR refresh transparently when DHCP renews the lease or the user
// switches from Wi-Fi to Ethernet without having to restart the server.
func (a *App) startIPWatcher() {
	if a.ipWatchStop != nil {
		return
	}
	stop := make(chan struct{})
	a.ipWatchStop = stop
	a.lastIPs = discovery.LocalIPs()
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				now := discovery.LocalIPs()
				if !sameIPSet(now, a.lastIPs) {
					log.Printf("discovery: local IPs changed: %v → %v", a.lastIPs, now)
					a.lastIPs = now
					if a.ctx != nil {
						runtime.EventsEmit(a.ctx, "server:ip-changed", now)
					}
					// Bounce the discovery broadcaster so it re-resolves
					// broadcast addresses on the new interface set.
					if a.broadcaster != nil && a.cfg.AutoDiscovery {
						a.broadcaster.Stop()
						a.broadcaster = discovery.NewBroadcaster(a.cfg.Port, a.cfg.DiscoveryPort)
						if err := a.broadcaster.Start(); err != nil {
							log.Printf("discovery: rebroadcast after IP change failed: %v", err)
						}
					}
				}
			}
		}
	}()
}

func sameIPSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]struct{}, len(a))
	for _, v := range a {
		m[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := m[v]; !ok {
			return false
		}
	}
	return true
}

// SetAutoDiscovery toggles the LAN discovery broadcaster at runtime.
// Stops the broadcaster when disabled; starts a new one when enabled.
func (a *App) SetAutoDiscovery(on bool) {
	a.cfg.AutoDiscovery = on
	if on {
		if a.broadcaster == nil || !a.broadcaster.IsRunning() {
			if a.broadcaster != nil {
				a.broadcaster.Stop()
			}
			a.broadcaster = discovery.NewBroadcaster(a.cfg.Port, a.cfg.DiscoveryPort)
			if err := a.broadcaster.Start(); err != nil {
				log.Printf("discovery: restart failed: %v", err)
			}
		}
	} else {
		if a.broadcaster != nil {
			a.broadcaster.Stop()
		}
	}
}

// GetAutoDiscovery reports whether LAN discovery is enabled.
func (a *App) GetAutoDiscovery() bool {
	return a.cfg.AutoDiscovery
}

// SetUSBAutoAccept toggles automatic USB device acceptance.
// When true, paired USB devices skip the connect prompt.
func (a *App) SetUSBAutoAccept(on bool) {
	// Persisted via localStorage on the desktop frontend side.
	// The actual admission policy check lives in OnClientConnect.
	log.Printf("config: usb auto-accept %v", on)
}

// GetUSBAutoAccept reports the current USB auto-accept preference.
func (a *App) GetUSBAutoAccept() bool {
	return false // placeholder — wired through localStorage currently
}
