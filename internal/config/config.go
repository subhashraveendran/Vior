// Package config handles application configuration.
package config

import (
	"net"
	"strconv"
)

const (
	// Version is the current Vior version string. Keep in sync with the
	// tag the npm shim downloads (npm/cli/vior.js VERSION) and the
	// GitHub release tag.
	Version = "v0.2.0"

	// DefaultRefreshRate is the default virtual display refresh rate in Hz.
	DefaultRefreshRate = 60.0

	// DefaultStreamPath is the HTTP path for the MJPEG stream.
	DefaultStreamPath = "/stream"
)

// Config holds all Vior configuration values.
type Config struct {
	// Display settings
	DisplayIndex int `yaml:"display_index" json:"display_index"`

	// Server settings
	Port int    `yaml:"port" json:"port"`
	Host string `yaml:"host" json:"host"`

	// Stream settings
	FrameRate int `yaml:"frame_rate" json:"frame_rate"`
	Quality   int `yaml:"quality" json:"quality"`

	// Transfer settings
	TransferDir string `yaml:"transfer_dir" json:"transfer_dir"`

	// Discovery settings
	DiscoveryPort int  `yaml:"discovery_port" json:"discovery_port"`
	AutoDiscovery bool `yaml:"auto_discovery" json:"auto_discovery"`
}

// Default returns a Config with sensible defaults.
// Port 0 means auto-select a free port.
func Default() *Config {
	return &Config{
		DisplayIndex:  0,
		Port:          0,
		Host:          "0.0.0.0",
		FrameRate:     30,
		Quality:       80,
		TransferDir:   ".",
		DiscoveryPort: 37680,
		AutoDiscovery: true,
	}
}

// FreePort finds an available TCP port.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// DiscoverablePorts are the ports the mobile client probes during LAN
// discovery. The desktop must bind one of these for auto-discovery to
// work — the UDP beacon carries the real port but the Capacitor WebView
// can't open a UDP socket to read it, so HTTP probing of these fixed
// ports is currently the only working discovery path.
var DiscoverablePorts = []int{8080, 8081}

// portAvailable reports whether a TCP port can be bound right now.
func portAvailable(port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// ResolvePort turns a requested port into a concrete one to bind.
//   - requested != 0 → honored as-is (operator override).
//   - requested == 0 → prefer a DiscoverablePort (8080, then 8081) so the
//     phone's fixed-port discovery sweep can actually find the server;
//     fall back to a random free port only if both are taken (in which
//     case discovery won't work, but the server still runs for a
//     manually-entered address).
func ResolvePort(requested int) (int, error) {
	if requested != 0 {
		return requested, nil
	}
	for _, p := range DiscoverablePorts {
		if portAvailable(p) {
			return p, nil
		}
	}
	return FreePort()
}
