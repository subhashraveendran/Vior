// Package discovery provides LAN auto-discovery via UDP broadcast.
// The server broadcasts its presence so that client apps can find it automatically.
package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
)

const (
	// DefaultPort is the UDP port used for discovery broadcasts.
	DefaultPort = 37680

	// BroadcastInterval is how often beacons are sent.
	BroadcastInterval = 2 * time.Second

	// Magic is the header used to identify Vior discovery packets.
	Magic = "VIOR"

	// ProtocolVersion is the current discovery protocol version.
	ProtocolVersion = 1
)

// Beacon is the UDP broadcast payload sent by the server.
type Beacon struct {
	Magic    string `json:"magic"`
	Version  int    `json:"version"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Platform string `json:"platform"`
}

// Broadcaster advertises the Vior server on the LAN via UDP broadcast.
type Broadcaster struct {
	httpPort      int
	discoveryPort int
	stop          chan struct{}
	running       bool
	mu            sync.Mutex
}

// NewBroadcaster creates a new discovery broadcaster.
func NewBroadcaster(httpPort, discoveryPort int) *Broadcaster {
	if discoveryPort == 0 {
		discoveryPort = DefaultPort
	}
	return &Broadcaster{
		httpPort:      httpPort,
		discoveryPort: discoveryPort,
	}
}

// Start begins broadcasting discovery beacons on the LAN.
func (b *Broadcaster) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return fmt.Errorf("broadcaster already running")
	}

	b.stop = make(chan struct{})
	b.running = true

	hostname, _ := os.Hostname()
	// Crop hostname to avoid exceeding the typical 1500-byte MTU
	// (beacon JSON with long hostnames can trigger IP fragmentation).
	if len(hostname) > 64 {
		hostname = hostname[:64]
	}
	beacon := Beacon{
		Magic:    Magic,
		Version:  ProtocolVersion,
		Name:     hostname,
		Port:     b.httpPort,
		Platform: runtime.GOOS,
	}

	payload, err := json.Marshal(beacon)
	if err != nil {
		return fmt.Errorf("marshal beacon: %w", err)
	}

	go b.broadcastLoop(payload)
	return nil
}

// Stop stops broadcasting.
func (b *Broadcaster) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running {
		return
	}
	close(b.stop)
	b.running = false
}

// IsRunning reports whether the broadcaster is active.
func (b *Broadcaster) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

func (b *Broadcaster) broadcastLoop(payload []byte) {
	ticker := time.NewTicker(BroadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
			b.sendBroadcast(payload)
		}
	}
}

func (b *Broadcaster) sendBroadcast(payload []byte) {
	addrs := broadcastAddresses()
	for _, addr := range addrs {
		dst := &net.UDPAddr{IP: addr, Port: b.discoveryPort}
		conn, err := net.DialUDP("udp4", nil, dst)
		if err != nil {
			continue
		}
		conn.Write(payload)
		conn.Close()
	}
}

// broadcastAddresses returns the broadcast address for each network interface.
func broadcastAddresses() []net.IP {
	var result []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		// Fallback to global broadcast.
		return []net.IP{net.IPv4bcast}
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLoopback() {
				continue
			}
			// Compute broadcast: IP | ^Mask
			ip := ipnet.IP.To4()
			mask := ipnet.Mask
			bcast := make(net.IP, 4)
			for i := range ip {
				bcast[i] = ip[i] | ^mask[i]
			}
			result = append(result, bcast)
		}
	}

	if len(result) == 0 {
		return []net.IP{net.IPv4bcast}
	}
	return result
}

// LocalIPs returns all non-loopback IPv4 addresses on this machine.
func LocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips
}
