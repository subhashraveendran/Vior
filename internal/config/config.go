// Package config handles application configuration.
package config

import "net"

const (
	// Version is the current Vior version string.
	Version = "v0.1.0-dev"

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
