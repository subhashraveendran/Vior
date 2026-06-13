//go:build windows

package virtual

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	configuredDisplayName string
	user32                = syscall.NewLazyDLL("user32.dll")
	enumDisplays          = user32.NewProc("EnumDisplayDevicesW")
	enumSettings          = user32.NewProc("EnumDisplaySettingsExW")
	changeSettings        = user32.NewProc("ChangeDisplaySettingsExW")
)

const (
	// Flags for EnumDisplayDevices
	EDD_GET_DEVICE_INTERFACE_NAME = 0x00000001
)

type displayDevice struct {
	cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type devMode struct {
	DeviceName         [32]uint16
	SpecVersion        uint16
	DriverVersion      uint16
	Size               uint16
	DriverExtra        uint16
	Fields             uint32
	PositionX          int32
	PositionY          int32
	DisplayOrientation uint32
	DisplayFixedOutput uint32
	Color              int16
	Duplex             int16
	YResolution        int16
	TTOption           int16
	Collate            int16
	FormName           [32]uint16
	LogPixels          uint16
	BitsPerPel         uint32
	PelsWidth          uint32
	PelsHeight         uint32
	DisplayFlags       uint32
	DisplayFrequency   uint32
	DisplayFlagsEx     uint32
}

const (
	DM_PELSWIDTH           = 0x00080000
	DM_PELSHEIGHT          = 0x00100000
	DM_DISPLAYFREQUENCY    = 0x00400000
	CDS_UPDATEREGISTRY     = 0x00000001
	CDS_TEST               = 0x00000002
	CDS_FULLSCREEN         = 0x00000004
	CDS_GLOBAL             = 0x00000008
	CDS_SET_PRIMARY        = 0x00000010
	CDS_RESET              = 0x40000000
	CDS_NORESET            = 0x10000000
	DISP_CHANGE_SUCCESSFUL = 0
)

// enumerateDisplays returns all display device names.
func enumerateDisplays() ([]string, error) {
	var names []string
	var dd displayDevice
	dd.cb = uint32(unsafe.Sizeof(dd))

	for i := uint32(0); ; i++ {
		ret, _, _ := enumDisplays.Call(
			uintptr(unsafe.Pointer(nil)),
			uintptr(i),
			uintptr(unsafe.Pointer(&dd)),
			0,
		)
		if ret == 0 {
			break
		}
		names = append(names, syscall.UTF16ToString(dd.DeviceName[:]))
	}
	return names, nil
}

// Create attempts to create a virtual display on Windows.
// Windows cannot create virtual displays from user mode without a driver.
// If a dummy plug or IDD driver is already installed, this function will
// detect and configure the available display.
func Create(width, height uint32, refreshRate float64) (uint32, error) {
	// Try to find an existing virtual/secondary display to configure.
	displays, err := enumerateDisplays()
	if err != nil {
		return 0, err
	}

	for idx, name := range displays {
		// Skip primary display (index 0)
		if idx == 0 {
			continue
		}

		var dm devMode
		dm.Size = uint16(unsafe.Sizeof(dm))
		dm.DriverExtra = 0

		devName, _ := syscall.UTF16PtrFromString(name)
		ret, _, _ := enumSettings.Call(
			uintptr(unsafe.Pointer(devName)),
			uintptr(^uint32(0)), // ENUM_CURRENT_SETTINGS = -1
			uintptr(unsafe.Pointer(&dm)),
			0,
		)
		if ret == 0 {
			continue
		}

		// Set the desired resolution.
		dm.Fields = DM_PELSWIDTH | DM_PELSHEIGHT | DM_DISPLAYFREQUENCY
		dm.PelsWidth = uint32(width)
		dm.PelsHeight = uint32(height)
		dm.DisplayFrequency = uint32(refreshRate)

		ret, _, _ = changeSettings.Call(
			uintptr(unsafe.Pointer(devName)),
			uintptr(unsafe.Pointer(&dm)),
			0,
			CDS_UPDATEREGISTRY|CDS_NORESET,
			0,
		)
		if ret != DISP_CHANGE_SUCCESSFUL {
			continue
		}

		// Force display refresh.
		changeSettings.Call(0, 0, 0, 0, 0)

		configuredDisplayName = name
		return uint32(idx), nil
	}

	return 0, fmt.Errorf(
		"no additional display found for virtual output\n\n" +
			"Windows requires a display driver for virtual displays.\n" +
			"Options:\n" +
			"  1. Install a physical dummy HDMI/DP plug (~$10)\n" +
			"  2. Install Microsoft's IDD sample driver\n" +
			"  3. Use usbmmidd or virtual-display-rs\n\n" +
			"Run 'vior virtual setup' for installation instructions.")
}

// CreateHiDPI creates a virtual HiDPI display on Windows.
func CreateHiDPI(logicalWidth, logicalHeight uint32, refreshRate float64) (uint32, error) {
	return Create(logicalWidth*2, logicalHeight*2, refreshRate)
}

// Destroy reverts only the display Vior previously configured.
func Destroy() {
	if configuredDisplayName == "" {
		return
	}
	devName, _ := syscall.UTF16PtrFromString(configuredDisplayName)
	changeSettings.Call(
		uintptr(unsafe.Pointer(devName)),
		0, 0, CDS_RESET, 0,
	)
	configuredDisplayName = ""
}
