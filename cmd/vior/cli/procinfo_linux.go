//go:build linux

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// platformInspectProcess reads /proc/<pid>/stat: existence, command name
// (truncated by the kernel to 15 bytes) and start time in clock ticks.
func platformInspectProcess(pid int) (procInfo, error) {
	if pid <= 0 {
		return procInfo{}, fmt.Errorf("invalid pid %d", pid)
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return procInfo{Alive: false}, nil
		}
		return procInfo{}, err
	}
	name, zombie, start, err := parseProcStat(string(data))
	if err != nil {
		return procInfo{}, err
	}
	if zombie {
		// Exited, waiting to be reaped: it will never act on a signal.
		return procInfo{Alive: false}, nil
	}
	return procInfo{Alive: true, Name: name, StartID: start}, nil
}
