// Package trust persists the set of devices that have completed a pair-code
// handshake with this server, and holds the per-device identity (Ed25519
// public key, fingerprint, revocation) that the next authentication phase
// builds on.
//
// # Trust model today
//
// The pair code is the only admission authority: every hello carries it and
// the server compares it before anything else (stream.handleWS). This store
// is metadata — who paired, when, from which platform — shown in Settings →
// Trusted devices. IsTrusted never short-circuits the pair check.
//
// # Identity groundwork (docs/connection-architecture.md §2.8, Phase A)
//
// A record may carry an Ed25519 public key pinned at first pairing, a
// fingerprint derived from it for the UI, and a revocation tombstone. Nothing
// in the server reads these yet. Admission is expected to use them like so:
//
//  1. First pairing — a hello with a valid pair code and a devicePub: Touch
//     the record, then SetPublicKey to pin the key. A later pairing from the
//     same deviceId with a different key fails with ErrKeyMismatch; the server
//     must not re-pin silently (SSH-style "key changed" warning). The user
//     forgets the device and pairs again.
//  2. Reconnect — a hello carrying a signature instead of the code: Get the
//     record, refuse when it is missing or IsRevoked, verify the signature
//     with PublicKey, then Touch.
//  3. Revoke marks the record and calls OnRevoked synchronously, so the server
//     can close a live session for that device. A revoked record is a
//     tombstone: Touch and SetPublicKey return ErrRevoked, IsTrusted is false,
//     and only Forget (followed by a fresh pairing) clears it. Clear removes
//     tombstones too.
//
// # On-disk format
//
// ~/.vior/trusted.json, mode 0600 inside a 0700 directory, written atomically
// (temp file in the same directory, fsync, rename) under a mutex. Version 2
// is an object:
//
//	{"version": 2, "devices": [{"deviceId": "...", ...}, ...]}
//
// Version 1 — every binary before this one — was a bare JSON array of the
// same records and is still read; the file is rewritten as version 2 on load.
// A corrupt, truncated, wrong-typed or unreadable file is moved aside to
// trusted.json.corrupt-<unixtime> (0600) and the store starts empty. A single
// record that fails validation is dropped and the rest are kept. If the bad
// file cannot be moved aside the store refuses to write, so unreadable trust
// data is never overwritten.
//
// Downgrade: the previous binary reads only the bare array. Given a v2 file
// it renames it to trusted.json.corrupt and starts empty, so a downgrade
// loses the list until devices pair again, but the data is kept on disk.
//
// # Location and logging
//
// DefaultPath picks the file; it is a variable so tests and embedders can
// redirect it, and inside a test binary (testing.Testing) it returns ""
// so packages that call Default at init never touch the real ~/.vior.
// Log lines name the file by its base name only, and returned errors are
// path-free sentinels; neither ever includes record values.
package trust

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

// FormatVersion is the on-disk schema version this binary writes.
const FormatVersion = 2

const (
	// MaxNameLen bounds Entry.Name, in runes. Longer names are truncated.
	MaxNameLen = 64
	// MaxPlatformLen bounds Entry.Platform, in runes.
	MaxPlatformLen = 32
	// maxFileSize caps how much of trusted.json is read. 10k devices with
	// maximal fields is ~3 MiB; anything larger is not a trust file.
	maxFileSize = 8 << 20
)

// deviceIDRe is the shape of a device id this store accepts: the clients
// send "mob-<uuid>", "web-<uuid>" or "web-anon-<digits>", and the planned
// key-derived id is 64 hex chars. Anything that could act as a path
// fragment, a log-line injection or an unbounded map key is refused.
var deviceIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{3,127}$`)

// ValidDeviceID reports whether id is acceptable as a trusted-device key.
// Admission should apply the same rule to hello.deviceId.
func ValidDeviceID(id string) bool { return deviceIDRe.MatchString(id) }

// Errors returned by Store methods. They never carry file paths, so they
// are safe to surface to the UI; the detailed cause is logged instead.
var (
	ErrInvalidDeviceID  = errors.New("trust: invalid device id")
	ErrUnknownDevice    = errors.New("trust: unknown device")
	ErrRevoked          = errors.New("trust: device is revoked")
	ErrInvalidPublicKey = errors.New("trust: public key must be 32 bytes (Ed25519)")
	ErrKeyMismatch      = errors.New("trust: a different public key is already pinned for this device")
	ErrPersist          = errors.New("trust: could not save the trusted device list")
	ErrDegraded         = errors.New("trust: trusted device list is unreadable and could not be moved aside; refusing to overwrite it")
)

// Entry is one trusted device record. Platform is the friendly platform
// string the mobile sends in its hello (e.g. "iOS", "Android" — empty when
// older builds didn't ship it).
type Entry struct {
	DeviceID  string    `json:"deviceId"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform,omitempty"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`

	// PublicKey is the device's Ed25519 public key, standard base64. Empty
	// until the device presents one (SetPublicKey).
	PublicKey string `json:"publicKey,omitempty"`
	// Fingerprint is derived from PublicKey (see Fingerprint). It is
	// recomputed on load and never trusted from the file.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Revoked marks a tombstone: the device was explicitly revoked and must
	// not be admitted or re-paired until it is forgotten.
	Revoked   bool      `json:"revoked,omitempty"`
	RevokedAt time.Time `json:"revokedAt,omitzero"`
}

// fileV2 is the on-disk envelope written by this binary.
type fileV2 struct {
	Version int     `json:"version"`
	Devices []Entry `json:"devices"`
}

// fileHeader reads the envelope with each record kept raw, so one bad
// record can be dropped without losing the rest.
type fileHeader struct {
	Version int               `json:"version"`
	Devices []json.RawMessage `json:"devices"`
}

// Store is the in-memory + on-disk set of trusted devices. Every method is
// safe for concurrent use; mutations and the file write they cause happen
// under one lock, so the file on disk always reflects the latest mutation.
type Store struct {
	// OnRevoked, when set, is called synchronously by Revoke after the
	// record has transitioned to revoked, with the store lock released, so
	// a live session for that device can be closed. It fires exactly once
	// per transition: the transition is decided under the store lock, so
	// concurrent Revoke calls for one device fire it once, and revoking an
	// already revoked device is a no-op. (A device that is forgotten and
	// pairs again can be revoked again — that is a new transition.) It
	// still fires when the write failed: closing the session is the part
	// that must not be skipped. Set it once, before the store is shared
	// across goroutines (right after Default or New).
	OnRevoked func(deviceID string)

	path string // "" = memory only
	mu   sync.RWMutex
	data map[string]Entry // keyed by DeviceID
	// degraded is set when the existing file could neither be read nor
	// moved aside. Writes are refused (ErrDegraded) so that data is kept.
	degraded bool
}

// DefaultPath returns the location of the trust file, ~/.vior/trusted.json,
// or "" (memory only) when there is no home directory or the process is a
// test binary. Tests and embedders may replace it before calling Default.
var DefaultPath = func() string {
	if testing.Testing() {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".vior", "trusted.json")
}

// Default returns a store backed by DefaultPath(). Without a home directory
// the store is memory-only for the life of the process.
func Default() *Store {
	path := DefaultPath()
	if path == "" && !testing.Testing() {
		log.Printf("trust: no home directory — trusted devices will not persist")
	}
	return New(path)
}

// New returns a store at the given path. The file is loaded eagerly; a
// missing file is not an error (empty store). An empty path gives a
// memory-only store.
func New(path string) *Store {
	s := &Store{path: path, data: map[string]Entry{}}
	s.load()
	return s
}

// ── Loading ─────────────────────────────────────────────────────────

func (s *Store) load() {
	if s.path == "" {
		return
	}
	b, err := readBounded(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		s.quarantine(describeReadError(err))
		return
	}
	version, raws, err := parseFile(b)
	if err != nil {
		s.quarantine(err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for i, raw := range raws {
		e, err := decodeEntry(raw)
		if err != nil {
			dropped++
			log.Printf("trust: dropping record %d: %v", i, err)
			continue
		}
		// Duplicate ids (hand-merged files): keep the most recently seen.
		if prev, ok := s.data[e.DeviceID]; ok && prev.LastSeen.After(e.LastSeen) {
			continue
		}
		s.data[e.DeviceID] = e
	}

	switch {
	case version > FormatVersion:
		log.Printf("trust: trusted.json is format v%d, newer than this build (v%d); loaded %d devices, unknown fields will be dropped on the next write",
			version, FormatVersion, len(s.data))
	case version < FormatVersion:
		log.Printf("trust: migrating trusted.json from v%d to v%d (%d devices, %d dropped)",
			version, FormatVersion, len(s.data), dropped)
		if err := s.persistLocked(); err != nil {
			log.Printf("trust: migration write deferred: %v", err)
		}
	case dropped > 0:
		if err := s.persistLocked(); err != nil {
			log.Printf("trust: cleanup write deferred: %v", err)
		}
	}
}

// readBounded reads the trust file, refusing non-regular files and
// anything over maxFileSize.
func readBounded(path string) ([]byte, error) {
	// Stat before Open: opening a FIFO would block New forever.
	if fi, err := os.Stat(path); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, errNotRegular
	}
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
		return nil, errNotRegular
	}
	if fi.Size() > maxFileSize {
		return nil, errTooLarge
	}
	return io.ReadAll(io.LimitReader(f, maxFileSize+1))
}

var (
	errNotRegular = errors.New("not a regular file")
	errTooLarge   = errors.New("larger than the 8 MiB limit")
)

// describeReadError turns a read failure into a short reason without the
// path (the caller logs the path once) or any file content.
func describeReadError(err error) string {
	switch {
	case errors.Is(err, errNotRegular), errors.Is(err, errTooLarge):
		return err.Error()
	case errors.Is(err, fs.ErrPermission):
		return "not readable (permission denied)"
	}
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return "unreadable (" + pe.Err.Error() + ")"
	}
	return "unreadable"
}

// parseFile accepts the v1 bare array or the v2 object and returns the
// raw records. The error, when non-nil, is a short reason that contains
// no file content.
func parseFile(b []byte) (version int, raws []json.RawMessage, err error) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return 0, nil, errors.New("empty")
	}
	switch trimmed[0] {
	case '[':
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return 0, nil, errors.New("malformed JSON (v1 array)")
		}
		return 1, list, nil
	case '{':
		var hdr fileHeader
		if err := json.Unmarshal(trimmed, &hdr); err != nil {
			return 0, nil, errors.New("malformed JSON (v2 object)")
		}
		if hdr.Version < 2 {
			return 0, nil, errors.New("object without a valid version field")
		}
		return hdr.Version, hdr.Devices, nil
	default:
		return 0, nil, errors.New("not a JSON array or object")
	}
}

// decodeEntry parses one raw record, normalises the free-text fields and
// validates identity, timestamps and key material. The returned error
// names the problem without quoting the record.
func decodeEntry(raw json.RawMessage) (Entry, error) {
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Entry{}, classifyDecodeError(err)
	}
	e.DeviceID = strings.TrimSpace(e.DeviceID)
	if !ValidDeviceID(e.DeviceID) {
		return Entry{}, errors.New("invalid device id")
	}
	e.Name = sanitizeText(e.Name, MaxNameLen)
	e.Platform = sanitizeText(e.Platform, MaxPlatformLen)

	switch {
	case e.FirstSeen.IsZero() && e.LastSeen.IsZero():
		return Entry{}, errors.New("missing timestamps")
	case e.FirstSeen.IsZero():
		e.FirstSeen = e.LastSeen
	case e.LastSeen.IsZero():
		e.LastSeen = e.FirstSeen
	}

	if e.PublicKey != "" {
		pub, err := decodePublicKey(e.PublicKey)
		if err != nil {
			return Entry{}, err
		}
		e.PublicKey = base64.StdEncoding.EncodeToString(pub)
		e.Fingerprint = Fingerprint(pub)
	} else {
		e.Fingerprint = ""
	}

	if e.Revoked {
		if e.RevokedAt.IsZero() {
			e.RevokedAt = e.LastSeen
		}
	} else {
		e.RevokedAt = time.Time{}
	}
	return e, nil
}

// classifyDecodeError maps a json error to a reason that carries field
// names at most — never the offending value.
func classifyDecodeError(err error) error {
	if _, ok := errors.AsType[*time.ParseError](err); ok {
		return errors.New("unparseable timestamp")
	}
	if ute, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		if ute.Field != "" {
			return fmt.Errorf("field %q has the wrong type", ute.Field)
		}
		return errors.New("record is not an object")
	}
	return errors.New("malformed record")
}

// sanitizeText drops control characters and invalid UTF-8, trims
// whitespace and truncates to max runes.
func sanitizeText(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		runes := []rune(s)
		s = strings.TrimSpace(string(runes[:max]))
	}
	return s
}

func decodePublicKey(enc string) (ed25519.PublicKey, error) {
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(enc))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, ErrInvalidPublicKey
	}
	return ed25519.PublicKey(pub), nil
}

// quarantine moves the unusable file aside so the user can recover it,
// then leaves the store empty. One log line either way.
func (s *Store) quarantine(reason string) {
	backup := fmt.Sprintf("%s.corrupt-%d", s.path, time.Now().Unix())
	for n := 1; ; n++ {
		if _, err := os.Lstat(backup); errors.Is(err, fs.ErrNotExist) {
			break
		}
		backup = fmt.Sprintf("%s.corrupt-%d-%d", s.path, time.Now().Unix(), n)
	}
	if err := os.Rename(s.path, backup); err != nil {
		s.mu.Lock()
		s.degraded = true
		s.mu.Unlock()
		log.Printf("trust: %s is %s and could not be moved aside (%v); starting empty and refusing to overwrite it until it is fixed or removed",
			s.fileName(), reason, errWithoutPath(err))
		return
	}
	if fi, err := os.Lstat(backup); err == nil && fi.Mode().IsRegular() {
		_ = os.Chmod(backup, 0o600)
	}
	log.Printf("trust: %s is %s; moved to %s and starting empty — paired devices will re-appear as they reconnect",
		s.fileName(), reason, filepath.Base(backup))
}

// fileName is how log lines refer to the trust file: the base name only,
// so logs never reveal the user's home directory layout.
func (s *Store) fileName() string { return filepath.Base(s.path) }

// errWithoutPath strips the path from an *fs.PathError or *os.LinkError
// so a log line names the file once, not three times.
func errWithoutPath(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	if le, ok := errors.AsType[*os.LinkError](err); ok {
		return le.Err
	}
	return err
}

// ── Writing ─────────────────────────────────────────────────────────

// persistLocked writes the current state. The caller holds s.mu for
// writing. Detailed failures are logged here; the returned error is a
// sentinel without paths, safe to show in the UI.
func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	if s.degraded {
		return ErrDegraded
	}
	doc := fileV2{Version: FormatVersion, Devices: s.sortedLocked()}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		log.Printf("trust: encode failed: %v", err)
		return ErrPersist
	}
	if err := writeAtomic(s.path, b); err != nil {
		log.Printf("trust: saving %s failed: %v", s.fileName(), errWithoutPath(err))
		return ErrPersist
	}
	return nil
}

// writeAtomic replaces path with data: a uniquely named 0600 temp file in
// the same directory, fsync, rename, then a best-effort directory sync so
// the rename itself survives a crash.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	// Parent dir gets 0700 — keeps a curious roommate on the same Mac
	// from reading the trusted device list (the file is 0600; this just
	// blocks `ls` enumeration of ~/.vior/). If the dir already existed
	// with looser perms (older Vior versions used 0755), tighten it.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Only ever tighten: a directory the owner made read-only on purpose
	// must stay that way so the write below fails instead of being
	// silently re-enabled.
	if fi, err := os.Stat(dir); err == nil && fi.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(dir, fi.Mode().Perm()&0o700)
	}

	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp") // 0600
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(err error) error {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir flushes the directory entry after a rename. Windows has no
// directory fsync; elsewhere a failure here is not worth failing the write.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// sortedLocked returns the entries most-recently-seen first, ties by id,
// so the file and List are stable between runs.
func (s *Store) sortedLocked() []Entry {
	out := make([]Entry, 0, len(s.data))
	for _, e := range s.data {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		return out[i].DeviceID < out[j].DeviceID
	})
	return out
}

// ── Queries ─────────────────────────────────────────────────────────

// IsTrusted reports whether a device has paired before and is not
// revoked. It is informational: admission still requires the pair code.
func (s *Store) IsTrusted(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[deviceID]
	return ok && !e.Revoked
}

// IsRevoked reports whether the device has a revocation tombstone. An
// unknown device is not revoked; callers that need "known and allowed"
// should use Get.
func (s *Store) IsRevoked(deviceID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[deviceID]
	return ok && e.Revoked
}

// Get returns the record for a device, revoked or not.
func (s *Store) Get(deviceID string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[deviceID]
	return e, ok
}

// PublicKey returns the pinned Ed25519 key for a device, or false when the
// device is unknown or has no key yet.
func (s *Store) PublicKey(deviceID string) (ed25519.PublicKey, bool) {
	s.mu.RLock()
	e, ok := s.data[deviceID]
	s.mu.RUnlock()
	if !ok || e.PublicKey == "" {
		return nil, false
	}
	pub, err := decodePublicKey(e.PublicKey)
	if err != nil {
		return nil, false
	}
	return pub, true
}

// List returns a snapshot of all devices, most recently seen first,
// including revoked tombstones (Entry.Revoked tells them apart).
func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedLocked()
}

// ── Mutations ───────────────────────────────────────────────────────

// Add records or touches a trusted device. Idempotent; updates LastSeen
// and preserves FirstSeen on re-add. Wraps Touch for legacy callers
// that only have name (no platform string).
func (s *Store) Add(deviceID, name string) error {
	return s.Touch(deviceID, name, "")
}

// Touch records or refreshes a trusted device with both name and
// platform metadata. Used on every admission so the Settings UI shows
// up-to-date "Last seen" / platform / name for each row. An empty id is a
// no-op; an id that fails ValidDeviceID returns ErrInvalidDeviceID; a
// revoked device returns ErrRevoked and is left untouched.
func (s *Store) Touch(deviceID, name, platform string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}
	if !ValidDeviceID(deviceID) {
		return ErrInvalidDeviceID
	}
	name = sanitizeText(name, MaxNameLen)
	platform = sanitizeText(platform, MaxPlatformLen)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	e, ok := s.data[deviceID]
	if !ok {
		e = Entry{DeviceID: deviceID, Name: name, Platform: platform, FirstSeen: now}
	} else if e.Revoked {
		return ErrRevoked
	}
	if name != "" {
		e.Name = name
	}
	if platform != "" {
		e.Platform = platform
	}
	e.LastSeen = now
	s.data[deviceID] = e
	return s.persistLocked()
}

// SetPublicKey pins the device's Ed25519 public key. The device must
// already be trusted (Touch first). Pinning the same key again is a
// no-op; a different key is refused with ErrKeyMismatch — the device has
// to be forgotten and paired again, so a key change is always a visible,
// deliberate act.
func (s *Store) SetPublicKey(deviceID string, pub []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return ErrInvalidPublicKey
	}
	enc := base64.StdEncoding.EncodeToString(pub)

	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[deviceID]
	switch {
	case !ok:
		return ErrUnknownDevice
	case e.Revoked:
		return ErrRevoked
	case e.PublicKey == enc:
		return nil
	case e.PublicKey != "":
		return ErrKeyMismatch
	}
	e.PublicKey = enc
	e.Fingerprint = Fingerprint(pub)
	s.data[deviceID] = e
	return s.persistLocked()
}

// Revoke marks a device as revoked and, on the transition, calls
// OnRevoked after releasing the lock. Revoking an unknown device returns
// ErrUnknownDevice; revoking twice is a no-op that returns nil and does
// not call OnRevoked.
//
// Revoke fails closed: when the file cannot be written the device is
// still revoked in memory for the rest of this process (IsTrusted is
// false, Touch returns ErrRevoked) and OnRevoked still runs, and the
// write error (ErrPersist or ErrDegraded) is returned so the UI can say
// the revocation may not survive a restart. Trust is never left in
// place because a disk write failed.
func (s *Store) Revoke(deviceID string) error {
	s.mu.Lock()
	e, ok := s.data[deviceID]
	if !ok {
		s.mu.Unlock()
		return ErrUnknownDevice
	}
	if e.Revoked {
		s.mu.Unlock()
		return nil
	}
	e.Revoked = true
	e.RevokedAt = time.Now().UTC()
	s.data[deviceID] = e
	err := s.persistLocked()
	cb := s.OnRevoked
	s.mu.Unlock()

	if cb != nil {
		cb(deviceID)
	}
	return err
}

// Forget removes a device from the trusted list, tombstone included.
// Forgetting an unknown device is a no-op.
func (s *Store) Forget(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[deviceID]; !ok {
		return nil
	}
	delete(s.data, deviceID)
	return s.persistLocked()
}

// Clear removes every device from the trusted list, tombstones included.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = map[string]Entry{}
	return s.persistLocked()
}

// ── Identity helpers ────────────────────────────────────────────────

// Fingerprint derives the short, human-comparable form of a public key
// shown next to a device: the first 8 base32 characters of SHA-256(pub),
// Syncthing-style (~40 bits — enough to spot a swapped key by eye, not a
// substitute for verifying the full key).
func Fingerprint(pub []byte) string {
	sum := sha256.Sum256(pub)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])[:8]
}
