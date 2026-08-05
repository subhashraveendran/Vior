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
	ctx   *gousb.Context
	dev   *gousb.Device
	iface *gousb.Interface
	inEP  *gousb.InEndpoint
	outEP *gousb.OutEndpoint
	done  func()

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

// Stop closes the USB connection and tears down the shared libusb
// context. Only Stop closes a.ctx — per-cable-disconnect cleanup must
// NOT, or the scanner dies permanently after the first unplug.
func (a *Accessory) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return
	}
	a.running = false
	close(a.stopCh)
	// cleanupLocked (not cleanup) because we already hold a.mu — calling
	// the lock-taking cleanup() here would self-deadlock on the
	// non-reentrant mutex.
	a.cleanupLocked()
	if a.ctx != nil {
		a.ctx.Close()
		a.ctx = nil
	}
}

// SendFrame sends a JPEG frame to the connected phone.
func (a *Accessory) SendFrame(jpeg []byte) error {
	return a.writeOutLocked(EncodeVideoFrame(jpeg))
}

// SendReady sends ready notification after display creation.
func (a *Accessory) SendReady(width, height int) error {
	return a.writeOutLocked(EncodeReady(width, height))
}

// IsConnected reports if a phone is connected via USB.
func (a *Accessory) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.outEP != nil
}

// writeOutLocked is the single safe path for writing to the USB bulk
// OUT endpoint from any goroutine other than the owning scanLoop /
// readInput. The endpoint can be cleared by cleanup() at any moment;
// callers that grabbed a stale a.outEP pointer would otherwise nil-
// dereference. We cache the pointer under the mutex, release the
// mutex before issuing the (potentially slow) Write, and accept that
// a Close() in the cleanup window produces a Write error rather than
// a panic.
func (a *Accessory) writeOutLocked(packet []byte) error {
	a.mu.Lock()
	outEP := a.outEP
	a.mu.Unlock()
	if outEP == nil {
		return fmt.Errorf("not connected")
	}
	_, err := outEP.Write(packet)
	return err
}

func (a *Accessory) scanLoop() {
	// A panic in any USB callback (gousb, OnConnect/OnTouch handlers)
	// must not take down the whole desktop process. Recover, log, and
	// let Stop()/the OS reclaim the context.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("usb: scanLoop panic recovered: %v", r)
		}
	}()
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
		a.mu.Lock()
		a.dev = dev
		a.mu.Unlock()

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
	defer func() {
		if r := recover(); r != nil {
			log.Printf("usb: heartbeatLoop panic recovered: %v", r)
		}
	}()
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
			// Use the cached local — a.outEP is racy after the unlock
			// above and cleanup() can nil it between the check and
			// the Write below, causing a nil-pointer panic.
			if _, err := outEP.Write(EncodePing()); err != nil {
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
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return nil, fmt.Errorf("usb context closed")
	}
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
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
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return nil, fmt.Errorf("usb context closed")
	}
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// Try all USB devices — filter by attempting AOA protocol.
		return true
	})
	// OpenDevices can return successfully-opened devices alongside an
	// error, so the slice has to be released even on the error path.
	// Closing every device exactly once, whatever happens, is the only
	// arrangement that does not leak: the previous loop closed each
	// device as it went but abandoned the remainder as soon as a switch
	// succeeded, so the devices after the matching one stayed open for
	// the process lifetime. Repeated switches then exhausted the OS
	// handle limit.
	closed := make([]bool, len(devs))
	defer func() {
		for i, d := range devs {
			if !closed[i] {
				d.Close()
			}
		}
	}()
	if err != nil {
		return nil, err
	}

	for i, dev := range devs {
		switched := a.trySwitch(dev)
		// The device is closed here either way: on success it is
		// rebooting into accessory mode and must be re-found by its new
		// identity, and on failure it is simply not ours.
		dev.Close()
		closed[i] = true
		if switched {
			// Release the rest before waiting — re-enumeration takes
			// seconds, and there is no reason to hold handles across it.
			for j := i + 1; j < len(devs); j++ {
				devs[j].Close()
				closed[j] = true
			}
			time.Sleep(2 * time.Second)
			return a.findAccessory()
		}
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

	// The config claim and the interface are released on every failure
	// path, and handed to a.done only once everything has succeeded.
	//
	// Previously each error site closed whatever it happened to remember:
	// the cfg.Interface failure released nothing, and both endpoint
	// failures released the interface but never the config. A claimed
	// config is held by the kernel, so every failed connect made the NEXT
	// connect to the same device fail too — a cable that failed once
	// stayed unusable until the process exited. A single commit flag is
	// harder to get wrong than four hand-maintained cleanup paths.
	committed := false
	defer func() {
		if !committed {
			cfg.Close()
		}
	}()

	iface, err := cfg.Interface(0, 0)
	if err != nil {
		return fmt.Errorf("interface: %w", err)
	}
	defer func() {
		if !committed {
			iface.Close()
		}
	}()

	var inEP *gousb.InEndpoint
	var outEP *gousb.OutEndpoint

	for _, ep := range iface.Setting.Endpoints {
		if ep.Direction == gousb.EndpointDirectionIn {
			inEP, err = iface.InEndpoint(ep.Number)
			if err != nil {
				return fmt.Errorf("in endpoint: %w", err)
			}
		} else {
			outEP, err = iface.OutEndpoint(ep.Number)
			if err != nil {
				return fmt.Errorf("out endpoint: %w", err)
			}
		}
	}

	if inEP == nil || outEP == nil {
		return fmt.Errorf("missing endpoints")
	}

	// Assign connection state under the mutex — scanLoop runs this while
	// cleanup() on another goroutine may be niling the same fields.
	a.mu.Lock()
	a.iface = iface
	// Close the interface AND the config claim on teardown. The old
	// teardown func closed only the interface, leaking one gousb.Config
	// (and its kernel-driver reattach) per connect/disconnect cycle.
	a.done = func() { iface.Close(); cfg.Close() }
	a.inEP = inEP
	a.outEP = outEP
	a.mu.Unlock()
	// Ownership now belongs to a.done; stand the deferred releases down.
	committed = true
	return nil
}

func (a *Accessory) readInput() {
	// Snapshot the IN endpoint under the mutex. cleanup() can nil a.inEP
	// from another goroutine; reading the field directly on every loop
	// iteration is a data race and could nil-deref mid-teardown.
	a.mu.Lock()
	inEP := a.inEP
	a.mu.Unlock()
	if inEP == nil {
		return
	}
	// Bigger read buffer so multi-byte frames (e.g. a JPEG ack) come in
	// one syscall; MaxFrameSize keeps it bounded.
	buf := make([]byte, 64*1024)
	// pending carries bytes across reads so a frame split over two bulk
	// transfers is reassembled rather than lost.
	var pending []byte
	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		n, err := inEP.Read(buf)
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

		// Accumulate, then drain every complete frame.
		//
		// A bulk transfer is not a frame boundary. The old code took
		// buf[0] as the type and buf[1:n] as the payload, so exactly one
		// frame survived per read: two touch events delivered in a single
		// transfer meant the second was discarded, and a frame split
		// across two transfers was mangled. A peer that batched its
		// writes could therefore lose most of its input — enough to make
		// touch look broken.
		pending = append(pending, buf[:n]...)

		consumed, stop, badType, ok := splitFrames(pending, a.dispatchFrame)
		if !ok {
			// Unrecoverable: without a length there is no way to find
			// the next boundary, so the stream cannot be resynchronised.
			// Fail closed rather than guess.
			log.Printf("usb: unknown inbound frame type 0x%02x — dropping connection", badType)
			a.handleDisconnect()
			return
		}
		if stop {
			return
		}

		// Compact in place; copy handles the overlap.
		pending = append(pending[:0], pending[consumed:]...)

		// Only an incomplete frame can remain, and the largest inbound
		// frame is 18 bytes. Anything beyond that is a peer dribbling
		// bytes that never form a frame, which must not grow the buffer
		// without bound.
		if len(pending) > maxPendingBytes {
			log.Printf("usb: %d bytes of unparsable input buffered — dropping connection", len(pending))
			a.handleDisconnect()
			return
		}
	}
}

// maxPendingBytes bounds the reassembly buffer. The largest inbound frame is
// 18 bytes, so a legitimate remainder never exceeds 17; the rest is slack.
const maxPendingBytes = 64

// splitFrames walks buf, invoking fn once per complete frame with the frame
// type and its payload (the type byte excluded).
//
// It returns how many bytes formed complete frames — the caller keeps the
// remainder for the next read — whether fn asked to stop, and ok=false with
// the offending type when a frame type has no known length. An unknown type is
// unrecoverable rather than skippable: without a length there is no way to
// locate the next boundary, so the stream can never be resynchronised.
//
// Split out from the read loop so the framing can be tested without a USB
// endpoint, which is where the interesting cases live — several frames in one
// transfer, and a frame straddling two.
func splitFrames(buf []byte, fn func(frameType byte, payload []byte) bool) (consumed int, stop bool, badType byte, ok bool) {
	for consumed < len(buf) {
		frameType := buf[consumed]
		size, known := InboundFrameSize(frameType)
		if !known {
			return consumed, false, frameType, false
		}
		if len(buf)-consumed < size {
			break // remainder of this frame is still in flight
		}
		if !fn(frameType, buf[consumed+1:consumed+size]) {
			return consumed + size, true, 0, true
		}
		consumed += size
	}
	return consumed, false, 0, true
}

// dispatchFrame handles one complete frame. data excludes the type byte and
// is exactly the payload length for that type, so decoders no longer see a
// whole bulk transfer's worth of trailing bytes.
//
// It reports whether the read loop should keep going; false means the
// connection has been torn down.
func (a *Accessory) dispatchFrame(frameType byte, data []byte) bool {
	switch frameType {
	case FrameHello:
		w, h, dpr, ver, ok := DecodeHello(data)
		if !ok {
			// Peer is some other AOA accessory (or stale Vior with
			// the pre-magic protocol) — bail before we feed garbage
			// to OnConnect / treat any payload as touch coords.
			log.Println("usb: peer is not Vior (magic mismatch)")
			a.handleDisconnect()
			return false
		}
		if ver != ProtocolVersion {
			log.Printf("usb: peer is not Vior (protocol version mismatch: got %d, want %d)", ver, ProtocolVersion)
			a.handleDisconnect()
			return false
		}
		log.Printf("usb: hello %dx%d @%.1fx (proto v%d, verified)", w, h, dpr, ver)
		// Reply with our matching magic+version so the phone can
		// flip transportMode='usb' (it stays "verifying" until ack).
		// Goes through writeOutLocked so a concurrent cleanup() can't
		// nil a.outEP between the check and the Write.
		if err := a.writeOutLocked(EncodeHelloAck()); err != nil {
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
		// still draining its bulk endpoint. writeOutLocked guards
		// against a concurrent cleanup() niling a.outEP.
		_ = a.writeOutLocked(EncodePong())

	case FramePong:
		// Phone is alive. Refresh the watchdog so the heartbeat
		// loop doesn't trip on the next interval.
		a.lastPongMu.Lock()
		a.lastPong = time.Now()
		a.lastPongMu.Unlock()

	case FrameHelloAck, FrameReady:
		// Host-originated frames. A peer echoing them back is not a
		// protocol violation worth dropping the cable over, but there
		// is nothing to do with them either.

	case FrameBye:
		a.handleDisconnect()
		return false
	}
	return true
}

func (a *Accessory) handleDisconnect() {
	log.Println("usb: device disconnected")
	if a.OnDisconnect != nil {
		a.OnDisconnect()
	}
	a.cleanup()
}

// cleanup tears down the current cable connection (device, interface,
// config, endpoints) but leaves the shared libusb context alive so the
// scanner can find the next device. Takes a.mu.
func (a *Accessory) cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanupLocked()
}

// cleanupLocked is the body of cleanup; callers must already hold a.mu.
// It deliberately does NOT close a.ctx — that is Stop's job. Closing the
// context here (the old behaviour) killed USB scanning permanently after
// the first disconnect.
func (a *Accessory) cleanupLocked() {
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
}
