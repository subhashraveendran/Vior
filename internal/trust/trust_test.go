package trust

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func tempStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "trusted.json")
	return New(path), path
}

// corruptBackups lists trusted.json.corrupt-* next to path.
func corruptBackups(t *testing.T, path string) []string {
	t.Helper()
	m, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return m
}

func tempFiles(t *testing.T, path string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return m
}

// captureLog routes the standard logger into a buffer for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX perms not meaningful on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
}

// ── Basics ──────────────────────────────────────────────────────────

// TestAddAndIsTrustedRoundtrip verifies Add persists across a fresh
// New() — i.e. the on-disk file is actually readable by a second
// process starting up.
func TestAddAndIsTrustedRoundtrip(t *testing.T) {
	s, path := tempStore(t)
	if err := s.Add("dev-1", "iPhone 17"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !s.IsTrusted("dev-1") {
		t.Fatalf("expected dev-1 trusted in-memory")
	}
	s2 := New(path)
	if !s2.IsTrusted("dev-1") {
		t.Fatalf("expected dev-1 trusted after reload")
	}
	e, ok := s2.Get("dev-1")
	if !ok || e.Name != "iPhone 17" || e.FirstSeen.IsZero() || e.LastSeen.IsZero() {
		t.Fatalf("reloaded entry = %+v", e)
	}
}

// TestFilePermissions verifies the trust file is 0600 and the parent
// dir is 0700 — both critical so a co-tenant on the same machine
// can't read the trusted device list.
func TestFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX perms not meaningful on Windows")
	}
	s, path := tempStore(t)
	if err := s.Add("dev-1", "Pixel"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
}

// TestAtomicWrite checks that no temp file is left behind after a write
// completes and that the file is the v2 envelope.
func TestAtomicWrite(t *testing.T) {
	s, path := tempStore(t)
	if err := s.Add("dev-1", "Pixel"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if left := tempFiles(t, path); len(left) != 0 {
		t.Fatalf("temp files lingered: %v", left)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc fileHeader
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("file is not a v2 object: %v", err)
	}
	if doc.Version != FormatVersion || len(doc.Devices) != 1 {
		t.Fatalf("version=%d devices=%d", doc.Version, len(doc.Devices))
	}
}

// TestForgetRemoves verifies Forget actually deletes the entry both
// in memory and on disk, and that forgetting a stranger is a no-op.
func TestForgetRemoves(t *testing.T) {
	s, path := tempStore(t)
	_ = s.Add("dev-1", "a")
	_ = s.Add("dev-2", "b")
	if err := s.Forget("dev-1"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if s.IsTrusted("dev-1") {
		t.Fatalf("dev-1 still trusted after Forget")
	}
	if err := s.Forget("nobody"); err != nil {
		t.Fatalf("Forget unknown: %v", err)
	}
	s2 := New(path)
	if s2.IsTrusted("dev-1") {
		t.Fatalf("dev-1 still trusted after reload")
	}
	if !s2.IsTrusted("dev-2") {
		t.Fatalf("dev-2 lost during Forget")
	}
}

func TestClearWipesTombstonesToo(t *testing.T) {
	s, path := tempStore(t)
	_ = s.Add("dev-1", "a")
	_ = s.Add("dev-2", "b")
	if err := s.Revoke("dev-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if n := len(New(path).List()); n != 0 {
		t.Fatalf("after Clear+reload: %d entries", n)
	}
}

func TestListOrderIsStable(t *testing.T) {
	s, _ := tempStore(t)
	for _, id := range []string{"dev-c", "dev-a", "dev-b"} {
		_ = s.Add(id, id)
	}
	// Force dev-a to be the most recent; its LastSeen must strictly exceed
	// the others, so bump it directly rather than racing the clock.
	s.mu.Lock()
	e := s.data["dev-a"]
	e.LastSeen = e.LastSeen.Add(time.Hour)
	s.data["dev-a"] = e
	s.mu.Unlock()
	got := s.List()
	if got[0].DeviceID != "dev-a" {
		t.Fatalf("first = %s, want dev-a", got[0].DeviceID)
	}
	for range 5 {
		again := s.List()
		for j := range got {
			if again[j].DeviceID != got[j].DeviceID {
				t.Fatalf("List order changed between calls: %v vs %v", got, again)
			}
		}
	}
}

// ── Validation and sanitisation ─────────────────────────────────────

func TestValidDeviceID(t *testing.T) {
	ok := []string{
		"mob-5f1c0c6a-9b6e-4c1b-8f2e-0c1f6d7a9e11", // mobile app
		"web-5f1c0c6a-9b6e-4c1b-8f2e-0c1f6d7a9e11", // browser client
		"web-anon-1727800000000",                   // browser with no storage
		"mob-k3j4h5g61lz9x",                        // Math.random fallback
		strings.Repeat("a", 128),                   // max length
		"dev-1",
	}
	for _, id := range ok {
		if !ValidDeviceID(id) {
			t.Errorf("ValidDeviceID(%q) = false, want true", id)
		}
	}
	bad := []string{
		"", "abc", "-leading", ".dot", "has space", "a/b", "a\\b",
		"id\n", "ünïcode-id", strings.Repeat("a", 129), "tab\tid",
	}
	for _, id := range bad {
		if ValidDeviceID(id) {
			t.Errorf("ValidDeviceID(%q) = true, want false", id)
		}
	}
}

func TestTouchRejectsInvalidIDAndSanitisesText(t *testing.T) {
	s, path := tempStore(t)
	if err := s.Touch("bad id", "x", ""); !errors.Is(err, ErrInvalidDeviceID) {
		t.Fatalf("Touch(bad id) = %v, want ErrInvalidDeviceID", err)
	}
	if err := s.Touch("", "x", ""); err != nil {
		t.Fatalf("Touch(empty) must be a no-op, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected touches must not write the file: %v", err)
	}

	longName := strings.Repeat("é", 70) // 70 runes, 140 bytes
	if err := s.Touch("  dev-1  ", "  Pixel\x00 8\n", longName); err != nil {
		t.Fatal(err)
	}
	e, ok := s.Get("dev-1")
	if !ok {
		t.Fatal("id not trimmed")
	}
	if e.Name != "Pixel 8" {
		t.Errorf("Name = %q, want control chars stripped", e.Name)
	}
	if n := len([]rune(e.Platform)); n != MaxPlatformLen {
		t.Errorf("Platform runes = %d, want %d", n, MaxPlatformLen)
	}
	_ = s.Touch("dev-2", longName, "")
	if n := len([]rune(s.List()[0].Name)); n != MaxNameLen {
		t.Errorf("Name runes = %d, want %d", n, MaxNameLen)
	}
}

// TestLoadValidatesEachRecord seeds a v2 file mixing good and bad
// records: the bad ones are dropped, the rest kept, and the file is
// rewritten without them.
func TestLoadValidatesEachRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	goodKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	seed := `{"version":2,"devices":[
	  {"deviceId":"mob-good","name":"Pixel","platform":"Android","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-02T00:00:00Z"},
	  {"deviceId":"bad id!","name":"x","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-01T00:00:00Z"},
	  {"deviceId":"mob-badtime","name":"x","firstSeen":"yesterday","lastSeen":"2026-01-01T00:00:00Z"},
	  {"deviceId":"mob-notime","name":"x"},
	  {"deviceId":"mob-onlylast","name":"x","lastSeen":"2026-03-01T00:00:00Z"},
	  "not-an-object",
	  42,
	  {"deviceId":"mob-badkey","name":"x","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-01T00:00:00Z","publicKey":"AAAA"},
	  {"deviceId":"mob-goodkey","name":"x","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-01T00:00:00Z","publicKey":"` + goodKey + `","fingerprint":"WRONGFPR"},
	  {"deviceId":"mob-longname","name":"` + strings.Repeat("n", 200) + `","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-01T00:00:00Z"},
	  {"deviceId":"mob-revoked","name":"x","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-05T00:00:00Z","revoked":true},
	  {"deviceId":"mob-good","name":"Older dup","firstSeen":"2025-01-01T00:00:00Z","lastSeen":"2025-01-02T00:00:00Z"}
	]}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	logs := captureLog(t)
	s := New(path)

	want := map[string]bool{"mob-good": true, "mob-onlylast": true, "mob-goodkey": true, "mob-longname": true, "mob-revoked": true}
	got := map[string]Entry{}
	for _, e := range s.List() {
		got[e.DeviceID] = e
	}
	if len(got) != len(want) {
		t.Fatalf("kept %d records %v, want %d", len(got), keys(got), len(want))
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("record %s was dropped", id)
		}
	}
	if got["mob-good"].Name != "Pixel" {
		t.Errorf("duplicate id: kept %q, want the most recently seen", got["mob-good"].Name)
	}
	if e := got["mob-onlylast"]; !e.FirstSeen.Equal(e.LastSeen) {
		t.Errorf("missing firstSeen should be filled from lastSeen: %+v", e)
	}
	if e := got["mob-goodkey"]; e.Fingerprint != Fingerprint(bytes.Repeat([]byte{7}, 32)) {
		t.Errorf("fingerprint must be recomputed from the key, got %q", e.Fingerprint)
	}
	if n := len([]rune(got["mob-longname"].Name)); n != MaxNameLen {
		t.Errorf("long name kept at %d runes, want %d", n, MaxNameLen)
	}
	if e := got["mob-revoked"]; !e.Revoked || e.RevokedAt.IsZero() || s.IsTrusted("mob-revoked") {
		t.Errorf("revoked record: %+v trusted=%v", e, s.IsTrusted("mob-revoked"))
	}
	if n := len(corruptBackups(t, path)); n != 0 {
		t.Errorf("per-record problems must not quarantine the file (%d backups)", n)
	}
	// Logs name the problem, never the value.
	if out := logs.String(); strings.Contains(out, "yesterday") || strings.Contains(out, "AAAA") || strings.Contains(out, "not-an-object") {
		t.Errorf("log leaked record content:\n%s", out)
	}
	if !strings.Contains(logs.String(), "dropping record") {
		t.Errorf("expected drop log lines, got:\n%s", logs.String())
	}
	// Cleanup write happened: a reload sees the same set without re-dropping.
	logs.Reset()
	s2 := New(path)
	if len(s2.List()) != len(want) {
		t.Fatalf("after cleanup reload: %d records", len(s2.List()))
	}
	if strings.Contains(logs.String(), "dropping record") {
		t.Errorf("bad records should have been removed from the file on first load:\n%s", logs.String())
	}
}

func keys(m map[string]Entry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ── Corrupt / unreadable files ──────────────────────────────────────

func TestCorruptFileIsQuarantinedAndStoreStaysWritable(t *testing.T) {
	cases := map[string]string{
		"malformed":                `{not valid json`,
		"truncated":                `[{"deviceId":"mob-x","name":"a","firstSeen":"2026-01-01T00:00:00Z","las`,
		"empty":                    ``,
		"whitespace":               "  \n ",
		"string":                   `"hello"`,
		"number":                   `12`,
		"object-without-version":   `{"devices":[]}`,
		"object-with-v1-version":   `{"version":1,"devices":[]}`,
		"object-wrong-version-typ": `{"version":"2","devices":[]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "trusted.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			logs := captureLog(t)
			s := New(path)
			if len(s.List()) != 0 {
				t.Fatalf("corrupt file should produce empty store")
			}
			backups := corruptBackups(t, path)
			if len(backups) != 1 {
				t.Fatalf("want exactly one quarantine file, got %v", backups)
			}
			if !strings.HasPrefix(filepath.Base(backups[0]), "trusted.json.corrupt-") {
				t.Errorf("backup name = %s", backups[0])
			}
			if runtime.GOOS != "windows" {
				fi, err := os.Stat(backups[0])
				if err != nil {
					t.Fatal(err)
				}
				if perm := fi.Mode().Perm(); perm != 0o600 {
					t.Errorf("backup perm = %o, want 0600", perm)
				}
			}
			saved, err := os.ReadFile(backups[0])
			if err != nil || string(saved) != body {
				t.Errorf("backup content changed: %q (%v)", saved, err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("original must be moved, not copied: %v", err)
			}
			if n := strings.Count(logs.String(), "trust:"); n != 1 {
				t.Errorf("want a single log line, got %d:\n%s", n, logs.String())
			}
			if strings.Contains(logs.String(), "hello") {
				t.Errorf("log leaked file content:\n%s", logs.String())
			}
			if strings.Contains(logs.String(), dir) {
				t.Errorf("log leaked the directory path:\n%s", logs.String())
			}
			// And we should still be writable.
			if err := s.Add("dev-x", "phone"); err != nil {
				t.Fatalf("Add after corrupt: %v", err)
			}
			if !New(path).IsTrusted("dev-x") {
				t.Fatalf("Add after corrupt didn't persist")
			}
		})
	}
}

func TestTwoQuarantinesInOneSecondGetDistinctNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	for range 2 {
		if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		New(path)
	}
	if n := len(corruptBackups(t, path)); n != 2 {
		t.Fatalf("want 2 distinct backups, got %d", n)
	}
}

func TestUnreadableFileIsQuarantined(t *testing.T) {
	skipIfRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	if err := os.WriteFile(path, []byte(`[]`), 0o000); err != nil {
		t.Fatal(err)
	}
	logs := captureLog(t)
	s := New(path)
	backups := corruptBackups(t, path)
	if len(backups) != 1 {
		t.Fatalf("permission-denied file should be moved aside, got %v", backups)
	}
	if !strings.Contains(logs.String(), "permission denied") {
		t.Errorf("log should say why:\n%s", logs.String())
	}
	if err := s.Add("dev-1", "a"); err != nil {
		t.Fatalf("store must be writable after quarantine: %v", err)
	}
	if !New(path).IsTrusted("dev-1") {
		t.Fatal("fresh file not readable")
	}
}

func TestDirectoryInPlaceOfFileIsQuarantined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if len(corruptBackups(t, path)) != 1 {
		t.Fatal("directory should be moved aside")
	}
	if err := s.Add("dev-1", "a"); err != nil {
		t.Fatalf("Add: %v", err)
	}
}

func TestOversizedFileIsQuarantined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxFileSize + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	logs := captureLog(t)
	New(path)
	if len(corruptBackups(t, path)) != 1 {
		t.Fatalf("oversized file should be moved aside:\n%s", logs.String())
	}
}

// TestDegradedRefusesToOverwrite covers the one case where the bad file
// cannot be moved aside (directory not writable): the store must start
// empty, keep working in memory, and refuse to clobber the file.
func TestDegradedRefusesToOverwrite(t *testing.T) {
	skipIfRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	logs := captureLog(t)
	s := New(path)
	if n := strings.Count(logs.String(), "trust:"); n != 1 {
		t.Errorf("want one log line, got %d:\n%s", n, logs.String())
	}
	if strings.Contains(logs.String(), dir) {
		t.Errorf("log leaked the directory path:\n%s", logs.String())
	}
	err := s.Add("dev-1", "a")
	if !errors.Is(err, ErrDegraded) {
		t.Fatalf("Add = %v, want ErrDegraded", err)
	}
	if !s.IsTrusted("dev-1") {
		t.Error("in-memory state must still update so the session works")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{bad" {
		t.Errorf("unreadable file was overwritten: %q", b)
	}
}

// ── Schema versioning ───────────────────────────────────────────────

func TestV1FixtureMigratesToV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	// Exactly what the previous binary wrote (MarshalIndent of []Entry).
	v1 := `[
  {
    "deviceId": "mob-5f1c0c6a-9b6e-4c1b-8f2e-0c1f6d7a9e11",
    "name": "Pixel 8",
    "platform": "Android 15",
    "firstSeen": "2026-09-01T10:00:00Z",
    "lastSeen": "2026-09-30T18:30:00Z"
  },
  {
    "deviceId": "web-anon-1727800000000",
    "name": "Mac Browser",
    "firstSeen": "2026-09-02T10:00:00Z",
    "lastSeen": "2026-09-02T10:05:00Z"
  }
]`
	if err := os.WriteFile(path, []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	logs := captureLog(t)
	s := New(path)
	got := s.List()
	if len(got) != 2 {
		t.Fatalf("loaded %d devices, want 2", len(got))
	}
	if got[0].DeviceID != "mob-5f1c0c6a-9b6e-4c1b-8f2e-0c1f6d7a9e11" || got[0].Name != "Pixel 8" || got[0].Platform != "Android 15" {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if !got[0].FirstSeen.Equal(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)) || !got[0].LastSeen.Equal(time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC)) {
		t.Errorf("timestamps changed: %+v", got[0])
	}
	if got[1].DeviceID != "web-anon-1727800000000" || got[1].Platform != "" {
		t.Errorf("entry 1 = %+v", got[1])
	}
	if !strings.Contains(logs.String(), "migrating") {
		t.Errorf("expected a migration log line:\n%s", logs.String())
	}
	if len(corruptBackups(t, path)) != 0 {
		t.Error("a v1 file is not corrupt")
	}

	// The file is now v2, and a reload gives the same devices.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var hdr fileHeader
	if err := json.Unmarshal(b, &hdr); err != nil || hdr.Version != 2 || len(hdr.Devices) != 2 {
		t.Fatalf("file after migration: version=%d devices=%d err=%v\n%s", hdr.Version, len(hdr.Devices), err, b)
	}
	again := New(path).List()
	for i := range got {
		if again[i] != got[i] {
			t.Errorf("reload[%d] = %+v, want %+v", i, again[i], got[i])
		}
	}
}

func TestV2FileWithoutNewFieldsStaysReadableAndOmitsThem(t *testing.T) {
	s, path := tempStore(t)
	_ = s.Add("dev-1", "a")
	b, _ := os.ReadFile(path)
	for _, f := range []string{"publicKey", "fingerprint", "revoked", "revokedAt"} {
		if bytes.Contains(b, []byte(f)) {
			t.Errorf("unset %s should be omitted from the file:\n%s", f, b)
		}
	}
}

func TestNewerFormatVersionLoadsBestEffort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	seed := `{"version":3,"future":true,"devices":[{"deviceId":"mob-x","name":"a","firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-01T00:00:00Z","extra":1}]}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	logs := captureLog(t)
	s := New(path)
	if !s.IsTrusted("mob-x") {
		t.Fatal("record from a newer format should load")
	}
	if !strings.Contains(logs.String(), "newer") {
		t.Errorf("expected a warning:\n%s", logs.String())
	}
	if len(corruptBackups(t, path)) != 0 {
		t.Error("newer format is not corrupt")
	}
}

// ── Concurrency ─────────────────────────────────────────────────────

// TestConcurrentMutations hammers the store from many goroutines (run
// with -race). Every mutation persists under the same lock, so the file
// must equal the in-memory state when the dust settles.
//
// Devices are forgotten, cleared and re-touched while this runs, so one
// id can legitimately transition to revoked more than once. The callback
// invariant checked here is therefore per transition, not per id: it
// never fires more often than Revoke succeeded for that id, and it fires
// at least once whenever any Revoke of that id succeeded (a nil return
// means the device is revoked, which only a transition can cause).
// TestRevokeFiresOnceUnderContention covers exactly-once for a single
// transition.
func TestConcurrentMutations(t *testing.T) {
	s, path := tempStore(t)
	var (
		countMu  sync.Mutex
		fired    = map[string]int{}
		revokeOK = map[string]int{}
	)
	s.OnRevoked = func(id string) {
		// The lock is released during the callback: the server will want
		// to look the device up while tearing its session down.
		_, _ = s.Get(id)
		countMu.Lock()
		fired[id]++
		countMu.Unlock()
	}
	workers, rounds := 8, 12
	if testing.Short() {
		rounds = 4
	}
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			mine := fmt.Sprintf("dev-%d", w)
			other := fmt.Sprintf("dev-%d", (w+1)%workers)
			for r := range rounds {
				if err := s.Touch(mine, "n", "p"); err != nil && !errors.Is(err, ErrRevoked) {
					t.Errorf("Touch: %v", err)
				}
				_ = s.IsTrusted(other)
				_ = s.List()
				switch r % 4 {
				case 0:
					if err := s.Forget(other); err != nil {
						t.Errorf("Forget: %v", err)
					}
				case 1:
					if err := s.SetPublicKey(mine, bytes.Repeat([]byte{byte(w)}, 32)); err != nil &&
						!errors.Is(err, ErrUnknownDevice) && !errors.Is(err, ErrRevoked) {
						t.Errorf("SetPublicKey: %v", err)
					}
				case 2:
					err := s.Revoke(other)
					switch {
					case err == nil:
						countMu.Lock()
						revokeOK[other]++
						countMu.Unlock()
					case !errors.Is(err, ErrUnknownDevice):
						t.Errorf("Revoke: %v", err)
					}
				case 3:
					if w == 0 {
						if err := s.Clear(); err != nil {
							t.Errorf("Clear: %v", err)
						}
					}
				}
			}
		}(w)
	}
	wg.Wait()

	for id, n := range fired {
		if ok := revokeOK[id]; n > ok {
			t.Errorf("OnRevoked fired %d times for %s but only %d Revoke calls succeeded", n, id, ok)
		}
	}
	for id, ok := range revokeOK {
		if ok > 0 && fired[id] == 0 {
			t.Errorf("Revoke succeeded %d times for %s but OnRevoked never fired", ok, id)
		}
	}

	if left := tempFiles(t, path); len(left) != 0 {
		t.Errorf("temp files lingered: %v", left)
	}
	mem := s.List()
	disk := New(path).List()
	if len(mem) != len(disk) {
		t.Fatalf("memory has %d entries, disk has %d", len(mem), len(disk))
	}
	for i := range mem {
		if mem[i] != disk[i] {
			t.Errorf("entry %d differs: mem=%+v disk=%+v", i, mem[i], disk[i])
		}
	}
}

// ── Identity ────────────────────────────────────────────────────────

func TestFingerprintVectors(t *testing.T) {
	// base32(sha256(pub))[:8], computed independently with Python's
	// hashlib + base64.b32encode.
	cases := []struct {
		pub  []byte
		want string
	}{
		{bytes.Repeat([]byte{0}, 32), "MZUHVLPY"},
		{bytes.Repeat([]byte{1}, 32), "OLGW5BBC"},
		{func() []byte {
			b := make([]byte, 32)
			for i := range b {
				b[i] = byte(i)
			}
			return b
		}(), "MMG42KLG"},
	}
	for _, c := range cases {
		if got := Fingerprint(c.pub); got != c.want {
			t.Errorf("Fingerprint(%x) = %q, want %q", c.pub[:4], got, c.want)
		}
	}
	if fp := Fingerprint(bytes.Repeat([]byte{0}, 32)); len(fp) != 8 || strings.ContainsAny(fp, "=") {
		t.Errorf("fingerprint shape: %q", fp)
	}
}

func TestSetPublicKey(t *testing.T) {
	s, path := tempStore(t)
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPublicKey("dev-1", pub); !errors.Is(err, ErrUnknownDevice) {
		t.Fatalf("unknown device: %v", err)
	}
	_ = s.Add("dev-1", "a")
	if err := s.SetPublicKey("dev-1", pub[:31]); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("short key: %v", err)
	}
	if err := s.SetPublicKey("dev-1", append([]byte(pub), 0)); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("long key: %v", err)
	}
	if err := s.SetPublicKey("dev-1", pub); err != nil {
		t.Fatalf("SetPublicKey: %v", err)
	}
	e, _ := s.Get("dev-1")
	if e.PublicKey != base64.StdEncoding.EncodeToString(pub) || e.Fingerprint != Fingerprint(pub) {
		t.Fatalf("entry = %+v", e)
	}
	if got, ok := s.PublicKey("dev-1"); !ok || !bytes.Equal(got, pub) {
		t.Fatalf("PublicKey() = %x ok=%v", got, ok)
	}
	// Same key again is a no-op; a different key is refused.
	if err := s.SetPublicKey("dev-1", pub); err != nil {
		t.Fatalf("re-pin same key: %v", err)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	if err := s.SetPublicKey("dev-1", other); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("different key: %v, want ErrKeyMismatch", err)
	}
	// Survives a reload; a device without a key reports none.
	s2 := New(path)
	if got, ok := s2.PublicKey("dev-1"); !ok || !bytes.Equal(got, pub) {
		t.Fatalf("after reload PublicKey() = %x ok=%v", got, ok)
	}
	_ = s2.Add("dev-2", "b")
	if _, ok := s2.PublicKey("dev-2"); ok {
		t.Fatal("dev-2 has no key")
	}
	// Forget + re-pair allows a new key.
	_ = s2.Forget("dev-1")
	_ = s2.Add("dev-1", "a")
	if err := s2.SetPublicKey("dev-1", other); err != nil {
		t.Fatalf("new key after forget: %v", err)
	}
}

func TestRevokeSemantics(t *testing.T) {
	s, path := tempStore(t)
	if err := s.Revoke("dev-1"); !errors.Is(err, ErrUnknownDevice) {
		t.Fatalf("Revoke unknown = %v", err)
	}
	_ = s.Add("dev-1", "a")
	pub, _, _ := ed25519.GenerateKey(nil)
	_ = s.SetPublicKey("dev-1", pub)

	if err := s.Revoke("dev-1"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if s.IsTrusted("dev-1") || !s.IsRevoked("dev-1") {
		t.Fatalf("trusted=%v revoked=%v", s.IsTrusted("dev-1"), s.IsRevoked("dev-1"))
	}
	e, ok := s.Get("dev-1")
	if !ok || !e.Revoked || e.RevokedAt.IsZero() || e.PublicKey == "" {
		t.Fatalf("tombstone = %+v", e)
	}
	if err := s.Touch("dev-1", "a", ""); !errors.Is(err, ErrRevoked) {
		t.Fatalf("Touch revoked = %v, want ErrRevoked", err)
	}
	if err := s.SetPublicKey("dev-1", pub); !errors.Is(err, ErrRevoked) {
		t.Fatalf("SetPublicKey revoked = %v, want ErrRevoked", err)
	}
	if s.IsRevoked("nobody") {
		t.Fatal("unknown device is not revoked")
	}

	// Persists.
	s2 := New(path)
	if !s2.IsRevoked("dev-1") || s2.IsTrusted("dev-1") {
		t.Fatal("revocation lost on reload")
	}
	if len(s2.List()) != 1 {
		t.Fatal("tombstone should still be listed")
	}
	// Forget clears the tombstone; the device can pair again.
	if err := s2.Forget("dev-1"); err != nil {
		t.Fatal(err)
	}
	if err := s2.Touch("dev-1", "a", ""); err != nil {
		t.Fatalf("Touch after Forget: %v", err)
	}
	if !s2.IsTrusted("dev-1") || s2.IsRevoked("dev-1") {
		t.Fatal("device should be trusted again")
	}
}

func TestOnRevokedFiresExactlyOnce(t *testing.T) {
	s, _ := tempStore(t)
	_ = s.Add("dev-1", "a")
	var calls []string
	s.OnRevoked = func(id string) {
		calls = append(calls, id)
		// The lock must be released: a server callback will want to look
		// the device up while tearing the session down.
		if !s.IsRevoked(id) {
			t.Errorf("callback sees %s as not revoked", id)
		}
		if _, ok := s.Get(id); !ok {
			t.Errorf("callback cannot Get %s", id)
		}
	}
	done := make(chan error, 1)
	go func() { done <- s.Revoke("dev-1") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Revoke: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Revoke deadlocked with OnRevoked holding the lock")
	}
	if len(calls) != 1 || calls[0] != "dev-1" {
		t.Fatalf("calls = %v, want [dev-1]", calls)
	}
	// Second revoke: no-op, no callback.
	if err := s.Revoke("dev-1"); err != nil {
		t.Fatalf("second Revoke: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("OnRevoked fired %d times, want 1", len(calls))
	}
	// Unknown device: no callback.
	_ = s.Revoke("nobody")
	if len(calls) != 1 {
		t.Fatalf("OnRevoked fired for an unknown device")
	}
	// Callback that itself forgets the device must not deadlock either.
	_ = s.Add("dev-2", "b")
	s.OnRevoked = func(id string) { _ = s.Forget(id) }
	if err := s.Revoke("dev-2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("dev-2"); ok {
		t.Fatal("callback Forget did not apply")
	}
}

// TestRevokeFiresOnceUnderContention has many goroutines revoke the same
// device at the same instant: every call must succeed, the callback must
// run exactly once, and the device must end up revoked on disk.
func TestRevokeFiresOnceUnderContention(t *testing.T) {
	s, path := tempStore(t)
	_ = s.Add("dev-1", "a")
	var fired atomic.Int32
	s.OnRevoked = func(id string) {
		if id != "dev-1" {
			t.Errorf("OnRevoked(%q)", id)
		}
		fired.Add(1)
	}
	const n = 32
	start := make(chan struct{})
	errs := make(chan error, 2*n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			errs <- s.Revoke("dev-1")
		}()
		go func() {
			defer wg.Done()
			<-start
			// Touch races the revocation: it either lands before (nil) or
			// after (ErrRevoked); it must never resurrect the device.
			if err := s.Touch("dev-1", fmt.Sprintf("n%d", i), ""); err != nil && !errors.Is(err, ErrRevoked) {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if got := fired.Load(); got != 1 {
		t.Fatalf("OnRevoked fired %d times, want exactly 1", got)
	}
	if !s.IsRevoked("dev-1") || s.IsTrusted("dev-1") {
		t.Fatal("device not revoked in memory")
	}
	if s2 := New(path); !s2.IsRevoked("dev-1") || s2.IsTrusted("dev-1") {
		t.Fatal("revocation not on disk")
	}
}

func TestRevokeFailsClosedWhenWriteFails(t *testing.T) {
	skipIfRoot(t)
	s, path := tempStore(t)
	_ = s.Add("dev-1", "a")
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	fired := 0
	s.OnRevoked = func(string) { fired++ }
	err := s.Revoke("dev-1")
	if !errors.Is(err, ErrPersist) {
		t.Fatalf("Revoke = %v, want ErrPersist", err)
	}
	if strings.Contains(err.Error(), dir) {
		t.Errorf("error leaks the path: %v", err)
	}
	if !s.IsRevoked("dev-1") || fired != 1 {
		t.Fatalf("revoked=%v fired=%d; must fail closed and still close the session", s.IsRevoked("dev-1"), fired)
	}
}

// ── Location override ───────────────────────────────────────────────

func TestDefaultPathOverride(t *testing.T) {
	prev := DefaultPath
	t.Cleanup(func() { DefaultPath = prev })

	want := filepath.Join(t.TempDir(), "trusted.json")
	DefaultPath = func() string { return want }
	s := Default()
	if err := s.Add("dev-1", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("Default() did not use the override: %v", err)
	}

	// No home directory: memory-only, never writes.
	DefaultPath = func() string { return "" }
	m := Default()
	if err := m.Add("dev-2", "b"); err != nil {
		t.Fatal(err)
	}
	if !m.IsTrusted("dev-2") {
		t.Fatal("memory-only store should still track devices")
	}
	if err := m.Revoke("dev-2"); err != nil {
		t.Fatal(err)
	}
}

func TestErrorsNeverCarryPaths(t *testing.T) {
	skipIfRoot(t)
	s, path := tempStore(t)
	_ = s.Add("dev-1", "a")
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	logs := captureLog(t)
	for name, err := range map[string]error{
		"Touch":  s.Touch("dev-2", "b", ""),
		"Forget": s.Forget("dev-1"),
		"Clear":  s.Clear(),
	} {
		if err == nil {
			t.Errorf("%s: expected a write error", name)
			continue
		}
		if strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "trusted.json") {
			t.Errorf("%s error leaks the path: %v", name, err)
		}
	}
	if !strings.Contains(logs.String(), "saving trusted.json failed") {
		t.Errorf("write failures should be logged:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), dir) {
		t.Errorf("log leaked the directory path:\n%s", logs.String())
	}
}

// TestDefaultPathIsMemoryOnlyInTests guards the real ~/.vior: packages
// that call Default at init (internal/stream) must not read, migrate or
// write the user's trust file while their tests run.
func TestDefaultPathIsMemoryOnlyInTests(t *testing.T) {
	if p := DefaultPath(); p != "" {
		t.Fatalf("DefaultPath() in a test binary = %q, want \"\"", p)
	}
	logs := captureLog(t)
	s := Default()
	if err := s.Add("dev-1", "a"); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Errorf("memory-only Default in tests should be silent:\n%s", logs.String())
	}
}
