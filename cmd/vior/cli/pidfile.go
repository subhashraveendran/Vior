package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The PID file lets `vior stop` find the running server and lets `vior
// start` refuse to launch a second one. It used to be a bare PID in the
// shared temp directory, world-readable, which meant any local user could
// plant a file that made `vior stop` signal an arbitrary process, and a PID
// recycled after a crash would be signalled blindly. It now lives in the
// per-user ~/.vior directory (0600) and carries enough identity to tell
// "the Vior that wrote this" from "whatever process has that PID now".

// pidRecord is the on-disk content of the PID file.
type pidRecord struct {
	PID int `json:"pid"`
	// Exe is the executable name as the process inspector reports it for
	// the writer. Comparing like with like (both sides come from the same
	// inspector) avoids false mismatches from symlinks or path forms.
	Exe string `json:"exe"`
	// StartID is an opaque process start-time token (Linux: starttime from
	// /proc/<pid>/stat; macOS/BSD: `ps -o lstart`; Windows: creation
	// FILETIME). Empty when the platform could not report it. A PID that
	// is alive but has a different StartID was recycled.
	StartID string `json:"start,omitempty"`
	// Nonce is random per run. The server keeps it in memory and only
	// removes the PID file (or honours a stop request) if the nonce still
	// matches, so it never deletes a successor's file.
	Nonce string `json:"nonce"`
}

// procInfo is what the platform can tell us about a PID.
type procInfo struct {
	Alive   bool
	Name    string // executable base name; "" if unknown
	StartID string // see pidRecord.StartID; "" if unknown
}

// inspectProcess is swapped out by tests; platformInspectProcess lives in
// the per-OS procinfo_*.go files. An error means "could not determine",
// which callers treat as unverifiable rather than as gone.
var inspectProcess = platformInspectProcess

// ownerState classifies the process a PID file points at.
type ownerState int

const (
	ownerGone    ownerState = iota // no such process: stale file
	ownerOther                     // PID alive but a different program, or recycled: stale file
	ownerVior                      // the Vior process that wrote the file
	ownerUnknown                   // alive, identity could not be verified
)

const (
	pidFileName     = "vior.pid"
	stopRequestName = "vior.stop"
	maxPIDFileSize  = 4096
)

// stateDir returns the per-user directory holding the PID file: ~/.vior,
// or the system temp directory only when no home directory is available.
func stateDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".vior")
	}
	return os.TempDir()
}

// pidFilePath returns where the PID file lives. In the temp-dir fallback
// the name carries the uid so users do not collide in a shared directory.
func pidFilePath() string {
	dir := stateDir()
	if dir == os.TempDir() {
		if uid := os.Getuid(); uid >= 0 {
			return filepath.Join(dir, fmt.Sprintf("vior-%d.pid", uid))
		}
	}
	return filepath.Join(dir, pidFileName)
}

// stopRequestPath is the file a Windows `vior stop` writes to ask the
// server for a graceful shutdown (Windows cannot deliver os.Interrupt to
// another process). It sits next to the PID file.
func stopRequestPath(pidPath string) string {
	return filepath.Join(filepath.Dir(pidPath), stopRequestName)
}

// newNonce returns 16 random bytes, hex encoded.
func newNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// selfRecord describes the current process.
func selfRecord() (pidRecord, error) {
	nonce, err := newNonce()
	if err != nil {
		return pidRecord{}, fmt.Errorf("generate PID file nonce: %w", err)
	}
	rec := pidRecord{PID: os.Getpid(), Nonce: nonce}
	if info, err := inspectProcess(rec.PID); err == nil {
		rec.Exe = info.Name
		rec.StartID = info.StartID
	}
	if rec.Exe == "" {
		if exe, err := os.Executable(); err == nil {
			rec.Exe = filepath.Base(exe)
		} else {
			rec.Exe = filepath.Base(os.Args[0])
		}
	}
	return rec, nil
}

// readPIDFile reads and validates a PID file.
func readPIDFile(path string) (*pidRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPIDFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPIDFileSize {
		return nil, errors.New("PID file is too large")
	}
	var rec pidRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("PID file is malformed: %w", err)
	}
	if rec.PID <= 0 {
		return nil, fmt.Errorf("PID file holds an invalid PID %d", rec.PID)
	}
	return &rec, nil
}

// normalizeExeName reduces an executable name or path to a comparable
// form: base name, no .exe suffix, lower case.
func normalizeExeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// filepath.Base only splits on the host separator; handle both so a
	// Windows-style path is reduced correctly in tests on any OS.
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToLower(name)
	return strings.TrimSuffix(name, ".exe")
}

// linuxCommLen is the length Linux truncates a process name to in
// /proc/<pid>/stat and `ps -o comm` (TASK_COMM_LEN minus the NUL).
const linuxCommLen = 15

// nameMatches reports whether a live process name is the executable the
// PID file recorded. With nothing recorded, only "vior" is accepted.
func nameMatches(actual, recorded string) bool {
	a := normalizeExeName(actual)
	r := normalizeExeName(recorded)
	if a == "" {
		return false
	}
	if r == "" {
		r = "vior"
	}
	if a == r {
		return true
	}
	// Linux truncates comm; a long binary name like vior-v0.2.0-linux-amd64
	// shows up as its first 15 bytes.
	return len(a) == linuxCommLen && strings.HasPrefix(r, a)
}

// classifyOwner decides whether the PID file's process is the live Vior
// that wrote it.
func classifyOwner(rec pidRecord) (ownerState, error) {
	info, err := inspectProcess(rec.PID)
	if err != nil {
		return ownerUnknown, err
	}
	if !info.Alive {
		return ownerGone, nil
	}
	if info.Name == "" {
		return ownerUnknown, errors.New("the process name is not available")
	}
	if !nameMatches(info.Name, rec.Exe) {
		return ownerOther, nil
	}
	if rec.StartID != "" && info.StartID != "" && rec.StartID != info.StartID {
		// Same program name but a different start time: the original
		// Vior exited and its PID was reused (possibly by another vior).
		return ownerOther, nil
	}
	return ownerVior, nil
}

// errAlreadyRunning is returned by acquirePIDFile when a live Vior holds
// the PID file.
var errAlreadyRunning = errors.New("vior is already running")

// pidFileRecentGrace protects a PID file another `vior start` has just
// created but not yet written: an empty or partial file younger than this
// is treated as "starting", not as corrupt.
const pidFileRecentGrace = 5 * time.Second

// acquirePIDFile claims the PID file for this process. A file left by a
// process that has exited (or whose PID now belongs to another program)
// is replaced. A file held by a live Vior is never replaced. A file whose
// owner is alive but cannot be verified is replaced only with force.
func acquirePIDFile(path string, force bool) (*pidRecord, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	rec, err := selfRecord()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 3; attempt++ {
		// O_EXCL is the lock: it fails if anything (including a planted
		// symlink) already exists at the path, so two starts cannot both
		// win and a write is never redirected through a link.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, werr := f.Write(data)
			cerr := f.Close()
			if werr == nil {
				werr = cerr
			}
			if werr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("write PID file %s: %w", path, werr)
			}
			return &rec, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create PID file %s: %w", path, err)
		}

		existing, rerr := readPIDFile(path)
		switch {
		case errors.Is(rerr, fs.ErrNotExist):
			continue // removed between our create and read; retry
		case rerr != nil:
			if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) < pidFileRecentGrace && !force {
				return nil, fmt.Errorf("another Vior instance is starting (PID file %s was just created)", path)
			}
			// Corrupt and not fresh: stale.
		default:
			state, cerr := classifyOwner(*existing)
			switch state {
			case ownerVior:
				return nil, fmt.Errorf("%w (PID %d); stop it with 'vior stop' first", errAlreadyRunning, existing.PID)
			case ownerUnknown:
				if !force {
					return nil, fmt.Errorf("PID file %s names process %d, which is running but could not be verified as Vior (%v); if no other Vior is running, re-run with --force", path, existing.PID, cerr)
				}
			}
			// ownerGone / ownerOther (or forced unknown): stale.
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("remove stale PID file %s: %w", path, err)
		}
	}
	return nil, fmt.Errorf("could not claim PID file %s: another Vior instance keeps recreating it", path)
}

// releasePIDFile removes the PID file if it is still ours.
func releasePIDFile(path string, rec *pidRecord) {
	if rec == nil {
		return
	}
	cur, err := readPIDFile(path)
	if err != nil || cur.Nonce != rec.Nonce {
		return
	}
	_ = os.Remove(path)
}

// watchStopRequest polls for a stop-request file carrying nonce and closes
// the returned channel when one appears (removing the file). Closing done
// stops the watcher. A request with any other content is ignored and left
// alone, so a stale request can never stop a later server.
func watchStopRequest(path, nonce string, interval time.Duration, done <-chan struct{}) <-chan struct{} {
	fired := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				data, err := os.ReadFile(path)
				if err != nil || strings.TrimSpace(string(data)) != nonce {
					continue
				}
				_ = os.Remove(path)
				close(fired)
				return
			}
		}
	}()
	return fired
}

// writeStopRequest asks the server identified by rec to shut down.
func writeStopRequest(path string, rec *pidRecord) error {
	tmp := path + "." + fmt.Sprint(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tmp, []byte(rec.Nonce), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
