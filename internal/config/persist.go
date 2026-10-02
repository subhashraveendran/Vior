package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Persisted settings live in ~/.vior/config.json. The file holds only what a
// user can change in Settings; everything else in Config is derived or
// internal and always starts from Default().
//
// Format (version 1):
//
//	{
//	  "version": 1,
//	  "quality": 80,          // JPEG quality, 1–100
//	  "frameRate": 30,        // capture fps, 1–120
//	  "port": 0,              // 0 = auto (8080, 8081, then any free port) or 1024–65535
//	  "host": "0.0.0.0",      // bind address, an IP literal
//	  "autoDiscovery": true,  // UDP discovery beacon
//	  "transferDir": "/Users/me/Downloads/Vior"  // absolute; where received files land
//	}
//
// Loading never fails hard: missing fields take their default, a field with
// the wrong type or an out-of-range value is reset to its default (and named
// in the returned warnings), and a file that is not valid JSON at all is moved
// aside to config.json.bak and replaced with defaults.

// FileVersion is the schema version written to config.json.
const FileVersion = 1

// Bounds for user-editable settings. Shared by the loader and by the desktop
// bindings so a value the UI may set is exactly a value the loader accepts.
const (
	MinQuality   = 1
	MaxQuality   = 100
	MinFrameRate = 1
	MaxFrameRate = 120
	// MinPort excludes the privileged range: binding there needs root on
	// Unix and would only ever fail for a desktop user.
	MinPort = 1024
	MaxPort = 65535

	maxHostLen = 64
	maxPathLen = 4096
	// maxFileSize bounds how much of config.json is read. The real file is
	// a few hundred bytes; anything larger is not ours.
	maxFileSize = 64 << 10
)

// fileConfig is the on-disk shape.
type fileConfig struct {
	Version       int    `json:"version"`
	Quality       int    `json:"quality"`
	FrameRate     int    `json:"frameRate"`
	Port          int    `json:"port"`
	Host          string `json:"host"`
	AutoDiscovery bool   `json:"autoDiscovery"`
	TransferDir   string `json:"transferDir"`
}

// DefaultPath returns ~/.vior/config.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vior", "config.json"), nil
}

// DefaultTransferDir returns ~/Downloads/Vior, or "" if the home directory
// cannot be determined.
func DefaultTransferDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Downloads", "Vior")
}

// ValidateQuality reports whether q is a usable JPEG quality.
func ValidateQuality(q int) error {
	if q < MinQuality || q > MaxQuality {
		return fmt.Errorf("quality must be between %d and %d", MinQuality, MaxQuality)
	}
	return nil
}

// ValidateFrameRate reports whether fps is a usable capture rate.
func ValidateFrameRate(fps int) error {
	if fps < MinFrameRate || fps > MaxFrameRate {
		return fmt.Errorf("frame rate must be between %d and %d", MinFrameRate, MaxFrameRate)
	}
	return nil
}

// ValidatePort accepts 0 (auto) or an unprivileged TCP port.
func ValidatePort(p int) error {
	if p == 0 || (p >= MinPort && p <= MaxPort) {
		return nil
	}
	return fmt.Errorf("port must be 0 (auto) or between %d and %d", MinPort, MaxPort)
}

// ValidateHost accepts an IPv4 or IPv6 literal. Hostnames are refused: the
// value is a bind address, and resolving a name at bind time would make the
// listening interface depend on DNS.
func ValidateHost(h string) error {
	if h == "" || len(h) > maxHostLen || net.ParseIP(h) == nil {
		return fmt.Errorf("host must be an IP address")
	}
	return nil
}

// ValidateTransferDir checks a receive directory. It must be an absolute path
// and either lie under the user's home directory (it is created on first use)
// or already exist as a directory. Vior's own state directory (~/.vior, which
// holds the pair code and trust store) is refused.
func ValidateTransferDir(dir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return validateTransferDirIn(dir, home)
}

func validateTransferDirIn(dir, home string) error {
	if dir == "" {
		return fmt.Errorf("transfer folder is empty")
	}
	if len(dir) > maxPathLen || strings.ContainsRune(dir, 0) {
		return fmt.Errorf("transfer folder path is invalid")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("transfer folder must be an absolute path")
	}
	dir = filepath.Clean(dir)

	// Compare against the symlink-resolved form when the directory exists,
	// so a link planted under $HOME cannot point the receive path into
	// ~/.vior.
	resolved := dir
	fi, statErr := os.Stat(dir)
	if statErr == nil {
		if !fi.IsDir() {
			return fmt.Errorf("transfer folder is not a directory")
		}
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			resolved = r
		}
	}

	if home != "" {
		homeClean := filepath.Clean(home)
		homeResolved := homeClean
		if r, err := filepath.EvalSymlinks(homeClean); err == nil {
			homeResolved = r
		}
		for _, stateDir := range []string{
			filepath.Join(homeClean, ".vior"),
			filepath.Join(homeResolved, ".vior"),
		} {
			if within(stateDir, dir) || within(stateDir, resolved) {
				return fmt.Errorf("transfer folder cannot be inside Vior's settings folder")
			}
		}
		if statErr != nil && (within(homeClean, dir) || within(homeResolved, dir)) {
			// Not created yet but under $HOME: fine, AcceptFile creates it.
			return nil
		}
	}
	if statErr != nil {
		return fmt.Errorf("transfer folder does not exist")
	}
	return nil
}

// within reports whether path is root or lies beneath it (lexically).
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ErrCorrupt reports that config.json was unreadable as JSON and has been
// replaced with defaults (the original is kept at config.json.bak).
var ErrCorrupt = errors.New("config file was corrupt; backed up and reset to defaults")

// Load reads the persisted settings at path on top of Default().
//
// It returns a usable Config in every case. warnings name each field that was
// present but invalid and has been reset to its default; they never contain
// the rejected value (a transfer path is a user file path, which must not
// reach the logs). err is non-nil when the file could not be read (the
// defaults are returned and the file is left alone) or was corrupt (ErrCorrupt:
// the file was moved to path+".bak" and defaults written in its place).
// A missing file is not an error.
func Load(path string) (cfg *Config, warnings []string, err error) {
	cfg = Default()
	if path == "" {
		return cfg, nil, nil
	}
	data, err := readBounded(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil, nil
		}
		if !errors.Is(err, errTooLarge) {
			return cfg, nil, err
		}
		// Oversized: treat as corruption below.
		data = nil
	}

	var raw map[string]json.RawMessage
	if data == nil || json.Unmarshal(data, &raw) != nil || raw == nil {
		if bErr := backupAndReset(path, cfg); bErr != nil {
			return cfg, nil, fmt.Errorf("%w (reset failed: %v)", ErrCorrupt, bErr)
		}
		return cfg, nil, ErrCorrupt
	}

	field := func(name string, dst any, validate func() error, apply func()) {
		v, ok := raw[name]
		if !ok {
			return
		}
		if json.Unmarshal(v, dst) != nil {
			warnings = append(warnings, name+": wrong type, using default")
			return
		}
		if validate != nil {
			if err := validate(); err != nil {
				warnings = append(warnings, name+": "+err.Error()+", using default")
				return
			}
		}
		apply()
	}

	var q, fps, port int
	var host, dir string
	var disc bool
	field("quality", &q, func() error { return ValidateQuality(q) }, func() { cfg.Quality = q })
	field("frameRate", &fps, func() error { return ValidateFrameRate(fps) }, func() { cfg.FrameRate = fps })
	field("port", &port, func() error { return ValidatePort(port) }, func() { cfg.Port = port })
	field("host", &host, func() error { return ValidateHost(host) }, func() { cfg.Host = host })
	field("autoDiscovery", &disc, nil, func() { cfg.AutoDiscovery = disc })
	field("transferDir", &dir, func() error { return ValidateTransferDir(dir) }, func() { cfg.TransferDir = filepath.Clean(dir) })
	return cfg, warnings, nil
}

var errTooLarge = errors.New("config file too large")

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("config path is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, errTooLarge
	}
	return data, nil
}

func backupAndReset(path string, cfg *Config) error {
	if err := os.Rename(path, path+".bak"); err != nil {
		return err
	}
	return Save(path, cfg)
}

// Save writes the user-editable subset of cfg to path atomically (temp file
// in the same directory, fsync, rename) with mode 0600. Invalid values are
// refused rather than written, so the file on disk always loads cleanly.
func Save(path string, cfg *Config) error {
	if path == "" {
		return fmt.Errorf("no config path")
	}
	if err := firstErr(
		ValidateQuality(cfg.Quality),
		ValidateFrameRate(cfg.FrameRate),
		ValidatePort(cfg.Port),
		ValidateHost(cfg.Host),
	); err != nil {
		return err
	}
	if cfg.TransferDir != "" {
		if err := ValidateTransferDir(cfg.TransferDir); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(fileConfig{
		Version:       FileVersion,
		Quality:       cfg.Quality,
		FrameRate:     cfg.FrameRate,
		Port:          cfg.Port,
		Host:          cfg.Host,
		AutoDiscovery: cfg.AutoDiscovery,
		TransferDir:   cfg.TransferDir,
	}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	// Best effort: persist the rename itself. Not supported everywhere
	// (Windows cannot open a directory for sync), and the data is already
	// safe in the file, so a failure here is ignored.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
