// Package trust persists the set of devices that have successfully
// completed a pair-code handshake with this server. Once a device is
// trusted, subsequent hellos from the same deviceID are admitted
// without re-prompting for the pair code — same trust model as Spotify
// Connect, scrcpy, or any other LAN companion app.
package trust

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one trusted device record. Platform is the friendly
// platform string the mobile sends in its hello (e.g. "iOS", "Android"
// — empty when older builds didn't ship it).
type Entry struct {
	DeviceID  string    `json:"deviceId"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform,omitempty"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// Store is the in-memory + on-disk set of trusted devices.
type Store struct {
	path string
	mu   sync.RWMutex
	data map[string]Entry // keyed by DeviceID
}

// Default returns a store backed by ~/.vior/trusted.json.
func Default() *Store {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return New(filepath.Join(home, ".vior", "trusted.json"))
}

// New returns a store at the given path. The file is loaded eagerly;
// missing file is not an error (empty store).
func New(path string) *Store {
	s := &Store{path: path, data: map[string]Entry{}}
	s.load()
	return s
}

func (s *Store) load() {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("trust: read %s failed: %v (starting empty)", s.path, err)
		}
		return
	}
	var list []Entry
	if err := json.Unmarshal(b, &list); err != nil {
		// File got corrupted (truncated mid-write, manual edit, disk
		// fault). Don't crash and don't refuse to admit anyone — just
		// start fresh so the user can re-pair. The corrupt file is
		// renamed aside so they can recover it if needed.
		log.Printf("trust: %s is corrupt (%v); quarantining and starting empty", s.path, err)
		_ = os.Rename(s.path, s.path+".corrupt")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range list {
		if e.DeviceID == "" {
			continue
		}
		s.data[e.DeviceID] = e
	}
}

func (s *Store) save() error {
	s.mu.RLock()
	list := make([]Entry, 0, len(s.data))
	for _, e := range s.data {
		list = append(list, e)
	}
	s.mu.RUnlock()
	dir := filepath.Dir(s.path)
	// Parent dir gets 0700 — keeps a curious roommate on the same Mac
	// from reading the trusted device list (file is already 0600, this
	// just blocks `ls` enumeration of ~/.vior/).
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Belt and suspenders: if the dir already existed with looser
	// perms (older Vior versions used 0755), tighten it on every save.
	_ = os.Chmod(dir, 0o700)
	tmp := s.path + ".tmp"
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// IsTrusted reports whether a device has been trusted before.
func (s *Store) IsTrusted(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[deviceID]
	return ok
}

// Add records or touches a trusted device. Idempotent; updates LastSeen
// and preserves FirstSeen on re-add. Wraps Touch for legacy callers
// that only have name (no platform string).
func (s *Store) Add(deviceID, name string) error {
	return s.Touch(deviceID, name, "")
}

// Touch records or refreshes a trusted device with both name and
// platform metadata. Used on every admission so the Settings UI shows
// up-to-date "Last seen" / platform / name for each row.
func (s *Store) Touch(deviceID, name, platform string) error {
	if deviceID == "" {
		return nil
	}
	s.mu.Lock()
	now := time.Now().UTC()
	e, ok := s.data[deviceID]
	if !ok {
		e = Entry{DeviceID: deviceID, Name: name, Platform: platform, FirstSeen: now}
	}
	if name != "" {
		e.Name = name
	}
	if platform != "" {
		e.Platform = platform
	}
	e.LastSeen = now
	s.data[deviceID] = e
	s.mu.Unlock()
	return s.save()
}

// Forget removes a device from the trusted list.
func (s *Store) Forget(deviceID string) error {
	s.mu.Lock()
	delete(s.data, deviceID)
	s.mu.Unlock()
	return s.save()
}

// Clear removes every device from the trusted list.
func (s *Store) Clear() error {
	s.mu.Lock()
	s.data = map[string]Entry{}
	s.mu.Unlock()
	return s.save()
}

// List returns a snapshot of all trusted devices.
func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.data))
	for _, e := range s.data {
		out = append(out, e)
	}
	return out
}
