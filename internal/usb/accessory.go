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

	// Callbacks.
	OnConnect    func(width, height int, dpr float32)
	OnTouch      func(action byte, x, y float32)
	OnDisconnect func()
}

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

		if a.dev != nil {
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

		// Wait for hello from phone.
		a.readInput()
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
	buf := make([]byte, 1024)
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
			w, h, dpr := DecodeHello(data)
			log.Printf("usb: hello %dx%d @%.1fx", w, h, dpr)
			if a.OnConnect != nil {
				a.OnConnect(w, h, dpr)
			}

		case FrameTouch:
			action, x, y := DecodeTouchEvent(data)
			if a.OnTouch != nil {
				a.OnTouch(action, x, y)
			}

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
