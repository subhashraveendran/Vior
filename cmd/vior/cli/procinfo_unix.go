//go:build unix && !linux

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// psPath is absolute so a hostile PATH cannot substitute another ps.
const psPath = "/bin/ps"

// platformInspectProcess (macOS, BSD) checks existence with kill(pid, 0)
// and reads name and start time from ps.
func platformInspectProcess(pid int) (procInfo, error) {
	if pid <= 0 {
		return procInfo{}, fmt.Errorf("invalid pid %d", pid)
	}
	if err := syscall.Kill(pid, 0); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return procInfo{Alive: false}, nil
		}
		// EPERM: exists but belongs to another user. Fall through to ps,
		// which can still report it.
		if !errors.Is(err, syscall.EPERM) {
			return procInfo{}, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, psPath, "-o", psColumns, "-p", strconv.Itoa(pid))
	// lstart is locale-formatted; pin it so both sides of a comparison
	// produce the same string.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		// ps exits 1 with no output when the PID vanished between the
		// kill probe and now.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(strings.TrimSpace(string(out))) == 0 {
			if kerr := syscall.Kill(pid, 0); errors.Is(kerr, syscall.ESRCH) {
				return procInfo{Alive: false}, nil
			}
		}
		return procInfo{}, fmt.Errorf("ps: %w", err)
	}
	state, lstart, comm, err := parsePSLine(string(out))
	if err != nil {
		return procInfo{}, err
	}
	if strings.HasPrefix(state, "Z") {
		return procInfo{Alive: false}, nil
	}
	return procInfo{Alive: true, Name: filepath.Base(comm), StartID: lstart}, nil
}
