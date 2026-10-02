//go:build windows

package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
	errInvalidParameter            = syscall.Errno(87)
)

var procQueryFullProcessImageNameW = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")

// platformInspectProcess opens the process with the least privilege that
// still reports exit status, creation time and image path.
func platformInspectProcess(pid int) (procInfo, error) {
	if pid <= 0 {
		return procInfo{}, fmt.Errorf("invalid pid %d", pid)
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		if errors.Is(err, errInvalidParameter) {
			// No process with that id.
			return procInfo{Alive: false}, nil
		}
		return procInfo{}, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer syscall.CloseHandle(h)

	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return procInfo{}, fmt.Errorf("query process %d: %w", pid, err)
	}
	if code != stillActive {
		return procInfo{Alive: false}, nil
	}

	info := procInfo{Alive: true}
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &created, &exited, &kernel, &user); err == nil {
		info.StartID = strconv.FormatInt(created.Nanoseconds(), 10)
	}

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 && size > 0 && int(size) <= len(buf) {
		info.Name = filepath.Base(syscall.UTF16ToString(buf[:size]))
	}
	return info, nil
}
