// Package machineid returns a stable per-host identifier suitable for
// deriving things like the Vior pair code. The ID is read from the OS
// where possible (so it survives a reinstall of the app and even a
// home-directory wipe) and falls back to a per-user random UUID at
// ~/.vior/machine-id only when no OS source is available.
//
// macOS  → IOPlatformUUID via `ioreg -rd1 -c IOPlatformExpertDevice`
// Linux  → /etc/machine-id (else /var/lib/dbus/machine-id)
// Windows→ HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid (via reg)
// other  → ~/.vior/machine-id (generated once)
//
// The result is memoised: the OS commands are cheap but the pair-code
// derivation runs on every /info request, so a single sync.Once keeps it
// out of the hot path.
package machineid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

var (
	once   sync.Once
	cached string
)

// ID returns a stable per-machine identifier. It never returns an empty
// string — if every OS source fails, a random UUID is generated and
// persisted at ~/.vior/machine-id.
func ID() string {
	once.Do(func() {
		cached = detect()
	})
	return cached
}

func detect() string {
	if id := fromOS(); id != "" {
		return id
	}
	// Last-resort fallback: per-user random UUID stored on disk. This
	// only kicks in when the OS lookup failed (e.g. ioreg missing in a
	// sandbox, /etc/machine-id stripped from a minimal container, reg
	// blocked by group policy). It's stable per home directory.
	if id := loadOrCreateFallback(); id != "" {
		return id
	}
	// Truly degenerate: no home dir, no OS source. Return an ephemeral
	// random ID — the pair code will then change across restarts, which
	// is the same behaviour as the original 6-hex implementation.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "vior-fallback"
	}
	return hex.EncodeToString(b)
}

func fromOS() string {
	switch runtime.GOOS {
	case "darwin":
		return darwinID()
	case "linux":
		return linuxID()
	case "windows":
		return windowsID()
	}
	return ""
}

// darwinID parses `ioreg -rd1 -c IOPlatformExpertDevice` for the
// IOPlatformUUID line. Format example:
//
//	"IOPlatformUUID" = "12345678-1234-1234-1234-1234567890AB"
func darwinID() string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)
	m := re.FindStringSubmatch(string(out))
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func linuxID() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil {
			id := strings.TrimSpace(string(b))
			if id != "" {
				return id
			}
		}
	}
	return ""
}

// windowsID shells out to `reg query` to avoid taking a hard dep on
// golang.org/x/sys/windows/registry. Output:
//
//	HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography
//	    MachineGuid    REG_SZ    abcd1234-...
func windowsID() string {
	out, err := exec.Command("reg", "query",
		`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography`,
		"/v", "MachineGuid").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "MachineGuid") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		return fields[len(fields)-1]
	}
	return ""
}

// loadOrCreateFallback returns the contents of ~/.vior/machine-id,
// creating it (random UUIDv4-ish hex) on first call.
func loadOrCreateFallback() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".vior")
	path := filepath.Join(dir, "machine-id")
	if b, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(b))
		if id != "" {
			return id
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	_ = os.WriteFile(path, []byte(id), 0o600)
	return id
}
