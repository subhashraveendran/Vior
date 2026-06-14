package usb

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/google/gousb"
)

// Accessory manages the AOA USB connection to an Android device.
type Accessory struct {
	ctx    *gousb.Context
	dev   *gousb.Device
	iface *gousb.Interface
	inEP   *gousb.InEndpoint
	outEP  *gousb.OutEndpoint
	done   func()

	running bool
	mu      sync.Mutex
	stopCh  chan struct{}

	// Heartbeat. lastPong is updated every time the phone replies to
	// our FramePing. The pinger goroutine writes FramePing every
	// pingInterval; if more than pingDeadDuration elapse without a
	// pong, we assume the phone has wedged (frozen WebView, OOM, etc.)
	// and tear down the cable even though the bulk endpoint is still
	// "open" at the OS layer. Without this a stuck phone leaves the
	// desktop showing "USB connected" indefinitely.
	lastPong   time.Time
	lastPongMu sync.Mutex

	// Callbacks.
	OnConnect    func(width, height int, dpr float32)
	OnTouch      func(action byte, x, y float32)
	OnDisconnect func()
}

// Heartbeat tuning. 5s interval gives a 10s outer bound on detecting
// a wedged phone — fast enough that the user gets a meaningful
// "cable connected but phone unresponsive" signal without spamming
// the bulk endpoint when everything is fine.
const (
	pingInterval     = 5 * time.Second
	pingDeadDuration = 10 * time.Second
)

// NewAccessory creates an AOA accessory manager.
func NewAccessory() *Accessory {
	return &Accessory{
		stopCh: make(chan struct{}),
	}
}

// Start begins scanning for Android devices and switching them to accessory mode.
func (a *Accessory) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return fmt.Errorf("already running")
	}
	a.running = true
	a.ctx = gousb.NewContext()

	go a.scanLoop()
	return nil
}

// Stop closes the USB connection.
func (a *Accessory) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return
	}
	a.running = false
	close(a.stopCh)
	a.cleanup()
}

// SendFrame sends a JPEG frame to the connected phone.
func (a *Accessory) SendFrame(jpeg []byte) error {
	if a.outEP == nil {
		return fmt.Errorf("not connected")
	}
	packet := EncodeVideoFrame(jpeg)
	_, err := a.outEP.Write(packet)
	return err
}

// SendReady sends ready notification after display creation.
func (a *Accessory) SendReady(width, height int) error {
	if a.outEP == nil {
		return fmt.Errorf("not connected")
	}
	_, err := a.outEP.Write(EncodeReady(width, height))
	return err
}

// IsConnected reports if a phone is connected via USB.
func (a *Accessory) IsConnected() bool {
	return a.outEP != nil
}

func (a *Accessory) scanLoop() {
	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		a.mu.Lock()
		dev := a.dev
		a.mu.Unlock()
		if dev != nil {
			// Already connected — read input.
			a.readInput()
			continue
		}

		// Look for Android device or already-switched accessory.
		dev, err := a.findAccessory()
		if err != nil {
			dev, err = a.findAndSwitch()
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
		}

		// Open accessory endpoints.
		if err := a.openEndpoints(dev); err != nil {
			log.Printf("usb: open endpoints failed: %v", err)
			dev.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		log.Println("usb: Android device connected in accessory mode")
		a.dev = dev

		// Seed the pong clock to "now" so the heartbeat doesn't
		// immediately trip on the first iteration (before the phone
		// has a chance to reply).
		a.lastPongMu.Lock()
		a.lastPong = time.Now()
		a.lastPongMu.Unlock()
		go a.heartbeatLoop()

		// Wait for hello from phone.
		a.readInput()
	}
}

// heartbeatLoop pings the phone every pingInterval and forces a
// disconnect if no pong arrives within pingDeadDuration. Exits when
// the cable goes away (outEP cleared by cleanup) or the accessory is
// stopped.
func (a *Accessory) heartbeatLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			// Endpoint cleared → cable went away or accessory was
			// torn down. handleDisconnect already fired or will fire
			// — exit cleanly.
			a.mu.Lock()
			outEP := a.outEP
			a.mu.Unlock()
			if outEP == nil {
				return
			}
			a.lastPongMu.Lock()
			elapsed := time.Since(a.lastPong)
			a.lastPongMu.Unlock()
			if elapsed > pingDeadDuration {
				log.Printf("usb: heartbeat dead (%.1fs since last pong) — forcing disconnect", elapsed.Seconds())
				a.handleDisconnect()
				return
			}
			if _, err := a.outEP.Write(EncodePing()); err != nil {
				// A write failure usually means the kernel already
				// noticed the cable yank; readInput will see EOF
				// momentarily. Log and let that path handle teardown.
				log.Printf("usb: heartbeat write failed: %v", err)
				return
			}
		}
	}
}

// findAccessory looks for device already in accessory mode.
func (a *Accessory) findAccessory() (*gousb.Device, error) {
	devs, err := a.ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == gousb.ID(AOAVendorID) &&
			(desc.Product == gousb.ID(AOAProductID) || desc.Product == gousb.ID(AOAProdNoADB))
	})
	if err != nil || len(devs) == 0 {
		// Close any partially opened devices.
		for _, d := range devs {
			d.Close()
		}
		return nil, fmt.Errorf("no accessory device")
	}
	// Return first, close rest.
	for _, d := range devs[1:] {
		d.Close()
	}
	return devs[0], nil
}

// findAndSwitch finds an Android device and switches it to accessory mode.
func (a *Accessory) findAndSwitch() (*gousb.Device, error) {
	devs, err := a.ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// Try all USB devices — filter by attempting AOA protocol.
		return true
	})
	if err != nil {
		return nil, err
	}

	for _, dev := range devs {
		if a.trySwitch(dev) {
			// Device is rebooting into accessory mode.
			dev.Close()
			// Wait for re-enumeration.
			time.Sleep(2 * time.Second)
			return a.findAccessory()
		}
		dev.Close()
	}
	return nil, fmt.Errorf("no android device found")
}

// trySwitch attempts to switch a device to AOA mode.
func (a *Accessory) trySwitch(dev *gousb.Device) bool {
	// Step 1: Check AOA protocol version.
	buf := make([]byte, 2)
	n, err := dev.Control(
		gousb.ControlIn|gousb.ControlVendor,
		AOAGetProtocol, 0, 0, buf)
	if err != nil || n < 2 {
		return false
	}
	version := int(buf[0]) | int(buf[1])<<8
	if version < 1 {
		return false
	}

	// Step 2: Send identification strings.
	for i, s := range AOAStrings {
		_, err := dev.Control(
			gousb.ControlOut|gousb.ControlVendor,
			AOASendString, 0, uint16(i), []byte(s+"\x00"))
		if err != nil {
			return false
		}
	}

	// Step 3: Start accessory mode.
	_, err = dev.Control(
		gousb.ControlOut|gousb.ControlVendor,
		AOAStartAccessory, 0, 0, nil)
	return err == nil
}

func (a *Accessory) openEndpoints(dev *gousb.Device) error {
	dev.SetAutoDetach(true)

	cfg, err := dev.Config(1)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	iface, err := cfg.Interface(0, 0)
	if err != nil {
		return fmt.Errorf("interface: %w", err)
	}

	var inEP *gousb.InEndpoint
	var outEP *gousb.OutEndpoint

	for _, ep := range iface.Setting.Endpoints {
		if ep.Direction == gousb.EndpointDirectionIn {
			inEP, err = iface.InEndpoint(ep.Number)
			if err != nil {
				iface.Close()
				return fmt.Errorf("in endpoint: %w", err)
			}
		} else {
			outEP, err = iface.OutEndpoint(ep.Number)
			if err != nil {
				iface.Close()
				return fmt.Errorf("out endpoint: %w", err)
			}
		}
	}

	if inEP == nil || outEP == nil {
		iface.Close()
		return fmt.Errorf("missing endpoints")
	}

	a.iface = iface
	a.done = func() { iface.Close() }
	a.inEP = inEP
	a.outEP = outEP
	return nil
}

func (a *Accessory) readInput() {
	// Bigger read buffer so multi-byte frames (e.g. a JPEG ack) come in
	// one syscall; MaxFrameSize keeps it bounded.
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		n, err := a.inEP.Read(buf)
		if err != nil {
			if err == io.EOF {
				continue
			}
			log.Printf("usb: read error: %v", err)
			a.handleDisconnect()
			return
		}
		if n == 0 {
			continue
		}

		frameType := buf[0]
		data := buf[1:n]

		switch frameType {
		case FrameHello:
			w, h, dpr, ver, ok := DecodeHello(data)
			if !ok {
				// Peer is some other AOA accessory (or stale Vior with
				// the pre-magic protocol) — bail before we feed garbage
				// to OnConnect / treat any payload as touch coords.
				log.Println("usb: peer is not Vior (magic mismatch)")
				a.handleDisconnect()
				return
			}
			if ver != ProtocolVersion {
				log.Printf("usb: peer is not Vior (protocol version mismatch: got %d, want %d)", ver, ProtocolVersion)
				a.handleDisconnect()
				return
			}
			log.Printf("usb: hello %dx%d @%.1fx (proto v%d, verified)", w, h, dpr, ver)
			// Reply with our matching magic+version so the phone can
			// flip transportMode='usb' (it stays "verifying" until ack).
			if _, err := a.outEP.Write(EncodeHelloAck()); err != nil {
				log.Printf("usb: hello-ack write failed: %v", err)
			}
			if a.OnConnect != nil {
				a.OnConnect(w, h, dpr)
			}

		case FrameTouch:
			action, x, y := DecodeTouchEvent(data)
			// Clamp coordinates to a sane absolute range so a corrupt
			// frame can't drive the cursor to int32-max somewhere off
			// the desktop. Real touch values are always positive and
			// bounded by the virtual display size; 1e5 is a generous
			// upper bound that covers any current display.
			if x < 0 {
				x = 0
			} else if x > 100000 {
				x = 100000
			}
			if y < 0 {
				y = 0
			} else if y > 100000 {
				y = 100000
			}
			if a.OnTouch != nil {
				a.OnTouch(action, x, y)
			}

		case FramePing:
			// Cheap liveness: echo immediately so the peer knows we're
			// still draining its bulk endpoint.
			_, _ = a.outEP.Write(EncodePong())

		case FramePong:
			// Phone is alive. Refresh the watchdog so the heartbeat
			// loop doesn't trip on the next interval.
			a.lastPongMu.Lock()
			a.lastPong = time.Now()
			a.lastPongMu.Unlock()

		case FrameBye:
			a.handleDisconnect()
			return
		}
	}
}

func (a *Accessory) handleDisconnect() {
	log.Println("usb: device disconnected")
	if a.OnDisconnect != nil {
		a.OnDisconnect()
	}
	a.cleanup()
}

func (a *Accessory) cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.done != nil {
		a.done()
		a.done = nil
	}
	if a.dev != nil {
		a.dev.Close()
		a.dev = nil
	}
	a.inEP = nil
	a.outEP = nil
	a.iface = nil
	if a.ctx != nil {
		a.ctx.Close()
		a.ctx = nil
	}
}
