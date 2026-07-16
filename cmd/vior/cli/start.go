package cli

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/subhashraveendran/vior/internal/capture"
	"github.com/subhashraveendran/vior/internal/config"
	"github.com/subhashraveendran/vior/internal/discovery"
	"github.com/subhashraveendran/vior/internal/input"
	"github.com/subhashraveendran/vior/internal/network"
	"github.com/subhashraveendran/vior/internal/protocol"
	"github.com/subhashraveendran/vior/internal/session"
	"github.com/subhashraveendran/vior/internal/stream"
	"github.com/subhashraveendran/vior/internal/virtual"
)

var (
	displayIndex   int
	port           int
	quality        int
	fps            int
	virtualWidth   int
	virtualHeight  int
	virtualRefresh float64
	noWebSocket    bool
	noDiscovery    bool
	// persistPair defaults to true now (was opt-in). A rotating pair
	// code on every server restart broke trusted-device reconnect for
	// any mobile that had only been admitted via pair code (not yet
	// promoted to the trust store) — the user would type the new code,
	// curse, and assume the connection had "died" forever. The trust
	// store still admits paired devices by deviceID, so this only
	// affects the first-connect window; setting --persist-pair=false
	// restores the old behaviour for users who explicitly want it.
	persistPair    bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start screen streaming",
	Long: `Start the Vior server and wait for a client to connect.

By default, the server starts in WebSocket mode: it waits for a phone/tablet
to connect and report its screen dimensions, then auto-creates a virtual display.

Use --virtual-width and --virtual-height to skip WebSocket handshake and stream
a pre-configured virtual display directly (legacy mode).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Default()
		// Resolve the port up front so the banner, URLs, QR and discovery
		// beacon all show the REAL bound port. A 0 (the default) prefers
		// 8080/8081 — the ports the mobile client actually probes — so
		// `vior start` with no --port is discoverable, and we never print
		// the bogus "port 0" / http://ip:0 the old default produced.
		resolvedPort, err := config.ResolvePort(port)
		if err != nil {
			return fmt.Errorf("failed to resolve port: %w", err)
		}
		cfg.Port = resolvedPort
		cfg.Quality = quality
		cfg.FrameRate = fps

		// Write PID file so 'vior stop' can find us.
		if err := os.WriteFile(pidFile(), []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer os.Remove(pidFile())

		// Opt-in: load/store the pair code at ~/.vior/pair.txt so the
		// same code survives restarts. Surprises pair-only users less.
		if persistPair {
			stream.EnablePersistedPair()
		}

		// Legacy mode: explicit virtual display dimensions provided.
		legacyMode := noWebSocket || (virtualWidth > 0 && virtualHeight > 0)

		if legacyMode {
			return runLegacyMode(cfg)
		}
		return runWebSocketMode(cfg)
	},
}

// runWebSocketMode starts the server and waits for a client to connect via WebSocket.
func runWebSocketMode(cfg *config.Config) error {
	handler := &cliSessionHandler{cfg: cfg}

	// Start HTTP+WS server with no frame channel (set when client connects).
	server := stream.NewMJPEGServer(cfg.Host, cfg.Port, nil, handler)
	handler.server = server
	if err := server.Start(); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	fmt.Printf("\n══════════════════════════════════════════\n")
	fmt.Printf("  Vior server — port %d\n", cfg.Port)
	fmt.Printf("  Pair code:  %s\n", stream.PairCode())
	fmt.Printf("══════════════════════════════════════════\n\n")
	printURLs(cfg.Port)
	fmt.Println()
	printQR(cfg.Port)

	// Start discovery broadcaster.
	var broadcaster *discovery.Broadcaster
	if !noDiscovery {
		broadcaster = discovery.NewBroadcaster(cfg.Port, discovery.DefaultPort)
		if err := broadcaster.Start(); err != nil {
			log.Printf("discovery broadcast failed: %v", err)
		} else {
			fmt.Printf("Discovery broadcasting on UDP port %d\n", discovery.DefaultPort)
		}
	}

	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	// Wait for interrupt.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nStopping...")
	if broadcaster != nil {
		broadcaster.Stop()
	}
	if handler.session != nil {
		handler.session.Stop()
	}
	if err := server.Stop(); err != nil {
		log.Printf("server stop error: %v", err)
	}
	virtual.Destroy()
	fmt.Println("Stopped.")
	return nil
}

// runLegacyMode creates a virtual display upfront and starts streaming immediately.
func runLegacyMode(cfg *config.Config) error {
	// Auto-create virtual display if requested.
	if virtualWidth > 0 && virtualHeight > 0 {
		info := virtual.Info{
			Width:       uint32(virtualWidth),
			Height:      uint32(virtualHeight),
			RefreshRate: virtualRefresh,
		}
		_, err := virtual.CreateVirtualDisplay(info)
		if err != nil {
			return fmt.Errorf("failed to create virtual display: %w", err)
		}
		fmt.Printf("Created virtual display %dx%d\n", virtualWidth, virtualHeight)
	}

	// Refresh display list.
	displays, err := capture.ListDisplays()
	if err != nil {
		return fmt.Errorf("failed to detect displays: %w", err)
	}

	if displayIndex == 0 && len(displays) > 1 {
		cfg.DisplayIndex = len(displays) - 1
	} else {
		cfg.DisplayIndex = displayIndex
	}

	if cfg.DisplayIndex < 0 || cfg.DisplayIndex >= len(displays) {
		return fmt.Errorf("display %d not found (available: 0-%d)", cfg.DisplayIndex, len(displays)-1)
	}

	d := displays[cfg.DisplayIndex]
	fmt.Printf("Streaming display [%d] %s (%dx%d)\n", d.Index, d.Name, d.Width, d.Height)

	// Start capture session.
	session := capture.NewSession(cfg.DisplayIndex, cfg.Quality, cfg.FrameRate)
	if err := session.Start(); err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}

	// Start MJPEG server (no WebSocket handler in legacy mode).
	server := stream.NewMJPEGServer(cfg.Host, cfg.Port, session.FrameCh, nil)
	if err := server.Start(); err != nil {
		session.Stop()
		return fmt.Errorf("server failed: %w", err)
	}

	fmt.Printf("Stream running on port %d\n", cfg.Port)
	fmt.Println()
	printURLs(cfg.Port)
	fmt.Println()
	printQR(cfg.Port)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nStopping...")
	session.Stop()
	if err := server.Stop(); err != nil {
		log.Printf("server stop error: %v", err)
	}
	if virtualWidth > 0 && virtualHeight > 0 {
		virtual.Destroy()
		fmt.Println("Virtual display destroyed.")
	}
	fmt.Println("Stopped.")
	return nil
}

func init() {
	startCmd.Flags().IntVarP(&displayIndex, "display", "d", 0, "display index to stream (see 'vior displays')")
	startCmd.Flags().IntVarP(&port, "port", "p", 0, "port to serve on (0 = auto-select free port)")
	startCmd.Flags().IntVarP(&quality, "quality", "q", 80, "JPEG quality 1-100")
	startCmd.Flags().IntVarP(&fps, "fps", "f", 30, "frames per second")
	startCmd.Flags().IntVar(&virtualWidth, "virtual-width", 0, "create virtual display with this pixel width (macOS)")
	startCmd.Flags().IntVar(&virtualHeight, "virtual-height", 0, "create virtual display with this pixel height (macOS)")
	startCmd.Flags().Float64Var(&virtualRefresh, "virtual-refresh", 60, "virtual display refresh rate")
	startCmd.Flags().BoolVar(&noWebSocket, "no-websocket", false, "disable WebSocket mode (legacy streaming)")
	startCmd.Flags().BoolVar(&noDiscovery, "no-discovery", false, "disable UDP discovery broadcasting")
	startCmd.Flags().BoolVar(&persistPair, "persist-pair", true, "load/save pair code at ~/.vior/pair.txt so it survives restarts (default true; pass --persist-pair=false to rotate on every launch)")
}

func printURLs(port int) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		fmt.Printf("  http://localhost:%d\n", port)
		return
	}

	fmt.Println("Open on your phone:")
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			fmt.Printf("  http://%s:%d\n", ipnet.IP.String(), port)
		}
	}
	fmt.Printf("  http://localhost:%d\n", port)
}

func printQR(port int) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			url := fmt.Sprintf("http://%s:%d", ipnet.IP.String(), port)
			qr, err := network.QRCodePlain(url)
			if err == nil {
				fmt.Print(qr)
			}
			return
		}
	}
}

// ── CLI Session Handler ─────────────────────────────────────────────

// cliSessionHandler implements stream.SessionHandler for CLI mode.
type cliSessionHandler struct {
	cfg         *config.Config
	server      *stream.MJPEGServer
	session     *capture.Session
	touchMapper *input.TouchMapper
}

func (h *cliSessionHandler) OnClientConnect(sess *protocol.Session, hello *protocol.HelloMessage) error {
	fmt.Printf("\nClient connected: %s %dx%d @%.1fx mode=%s\n", hello.Name, hello.Width, hello.Height, hello.DPR, hello.Mode)

	if h.session != nil {
		h.session.Stop()
		h.session = nil
	}

	setup, err := session.Configure(hello)
	if err != nil {
		return err
	}

	h.session = capture.NewSession(setup.DisplayIndex, h.cfg.Quality, h.cfg.FrameRate)
	if err := h.session.Start(); err != nil {
		return fmt.Errorf("capture failed: %w", err)
	}
	h.server.SetFrameCh(h.session.FrameCh)
	h.touchMapper = input.NewTouchMapper(input.DefaultController, setup.DisplayBounds)

	sess.Send(protocol.MsgReady, &protocol.ReadyMessage{
		StreamURL:  config.DefaultStreamPath,
		Resolution: fmt.Sprintf("%dx%d", setup.Width, setup.Height),
		SessionID:  sess.ID,
	})

	fmt.Printf("Streaming to client (%s mode, %dx%d).\n", setup.Mode, setup.Width, setup.Height)
	return nil
}

func (h *cliSessionHandler) OnClientInput(_ *protocol.Session, msg *protocol.InputMessage) error {
	if msg.Event == "key" {
		return input.DefaultController.TypeKey(msg.Key)
	}
	if h.touchMapper == nil {
		log.Printf("OnClientInput dropped %s/%s: touchMapper not initialised", msg.Event, msg.Action)
		return nil
	}
	var err error
	switch msg.Event {
	case "touch":
		err = h.touchMapper.HandleTouch(msg.Action, msg.X, msg.Y)
	case "mouse":
		err = h.touchMapper.HandleMouse(msg.Action, msg.DX, msg.DY)
	case "scroll":
		err = h.touchMapper.HandleScroll(msg.DX, msg.DY)
	}
	if err != nil {
		log.Printf("OnClientInput %s/%s error: %v", msg.Event, msg.Action, err)
	}
	return err
}

func (h *cliSessionHandler) OnClientResize(session *protocol.Session, msg *protocol.ResizeMessage) error {
	fmt.Printf("Client resized: %dx%d\n", msg.Width, msg.Height)
	// For CLI, recreate virtual display with new dimensions.
	if h.session != nil {
		h.session.Stop()
		h.session = nil
	}
	virtual.Destroy()
	time.Sleep(300 * time.Millisecond)

	info := virtual.Info{Width: uint32(msg.Width), Height: uint32(msg.Height), RefreshRate: config.DefaultRefreshRate}
	displayID, err := virtual.CreateVirtualDisplay(info)
	if err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	vdIdx := capture.FindDisplayIndexByID(displayID)
	if vdIdx < 0 {
		displays, _ := capture.ListDisplays()
		vdIdx = len(displays) - 1
	}
	capture.UnmirrorDisplay(vdIdx)

	displays, _ := capture.ListDisplays()
	if vdIdx >= len(displays) {
		return fmt.Errorf("display index out of range")
	}

	h.session = capture.NewSession(vdIdx, h.cfg.Quality, h.cfg.FrameRate)
	h.session.Start()
	h.server.SetFrameCh(h.session.FrameCh)

	d := displays[vdIdx]
	h.touchMapper = input.NewTouchMapper(input.DefaultController, d.Bounds)

	session.Send(protocol.MsgReady, &protocol.ReadyMessage{
		StreamURL:  config.DefaultStreamPath,
		Resolution: fmt.Sprintf("%dx%d", msg.Width, msg.Height),
		SessionID:  session.ID,
	})
	return nil
}

func (h *cliSessionHandler) OnClientFileOffer(session *protocol.Session, msg *protocol.FileOfferMessage) error {
	log.Printf("File offered: %s (%d bytes)", msg.Name, msg.Size)
	return nil
}

func (h *cliSessionHandler) OnClientFileAccept(session *protocol.Session, msg *protocol.FileAcceptMessage) error {
	return nil
}

func (h *cliSessionHandler) OnClientFileReject(session *protocol.Session, msg *protocol.FileRejectMessage) error {
	return nil
}

func (h *cliSessionHandler) OnClientFileChunk(session *protocol.Session, msg *protocol.FileChunkMessage) error {
	return nil
}

func (h *cliSessionHandler) OnClientFileComplete(session *protocol.Session, msg *protocol.FileCompleteMessage) error {
	return nil
}

func (h *cliSessionHandler) OnClientDownloadAccept(session *protocol.Session, msg *protocol.DownloadAcceptMessage) error {
	return nil
}

func (h *cliSessionHandler) OnClientDownloadReject(session *protocol.Session, msg *protocol.DownloadRejectMessage) error {
	return nil
}

func (h *cliSessionHandler) OnClientDownloadComplete(session *protocol.Session, msg *protocol.DownloadCompleteMessage) error {
	return nil
}

func (h *cliSessionHandler) ServeDownload(w http.ResponseWriter, r *http.Request, id string) {
	http.Error(w, "downloads unavailable in CLI mode", http.StatusNotFound)
}

func (h *cliSessionHandler) OnClientDisconnect(session *protocol.Session) {
	fmt.Printf("\nClient disconnected: %s\n", session.ID)
	if h.session != nil {
		h.session.Stop()
		h.session = nil
	}
	virtual.Destroy()
	h.touchMapper = nil
	fmt.Println("Waiting for next client...")
}
