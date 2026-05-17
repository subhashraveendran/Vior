package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var virtualSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Print platform-specific virtual display setup instructions",
	Long:  "Generate configuration needed to enable virtual displays on your platform.",
	RunE: func(cmd *cobra.Command, args []string) error {
		switch runtime.GOOS {
		case "linux":
			fmt.Print(linuxSetupGuide)
		case "windows":
			fmt.Print(windowsSetupGuide)
		case "darwin":
			fmt.Print("macOS supports virtual displays natively via CGVirtualDisplay.\nNo additional setup required.\n\n")
			fmt.Println("Create a virtual display:  vior virtual create --device iphone-pro")
			fmt.Println("Stream it:                  vior start --virtual-width 1179 --virtual-height 2556")
			fmt.Println("Destroy it:                 vior virtual destroy")
		default:
			fmt.Printf("Virtual displays are not supported on %s.\n", runtime.GOOS)
		}
		return nil
	},
}

func init() {
	virtualCmd.AddCommand(virtualSetupCmd)
}

const linuxSetupGuide = `=== Linux Virtual Display Setup ===

Virtual displays on Linux require the xf86-video-dummy X11 driver.

1. INSTALL THE DUMMY DRIVER
   Debian/Ubuntu:  sudo apt install xserver-xorg-video-dummy
   Fedora:         sudo dnf install xorg-x11-drv-dummy
   Arch:           sudo pacman -S xf86-video-dummy

2. CREATE CONFIG (/etc/X11/xorg.conf.d/90-vior-dummy.conf):

Section "Device"
    Identifier  "ViorVirtual"
    Driver      "dummy"
    VideoRam    256000
EndSection

Section "Monitor"
    Identifier  "ViorMonitor"
    HorizSync   28.0-80.0
    VertRefresh 48.0-75.0
    Modeline "1920x1080"  148.50 1920 2008 2052 2200 1080 1084 1089 1125 +hsync +vsync
    Modeline "1179x2556"  203.00 1179 1251 1283 1420 2556 2559 2567 2602 +hsync +vsync
EndSection

Section "Screen"
    Identifier  "ViorScreen"
    Device      "ViorVirtual"
    Monitor     "ViorMonitor"
    DefaultDepth 24
    SubSection "Display"
        Depth 24
        Modes "1920x1080" "1179x2556"
    EndSubSection
EndSection

3. RESTART X11
   Log out and log back in (or restart your display manager).

4. VERIFY
   Run: xrandr --query
   Look for a disconnected output (usually named VIRTUAL1 or similar).

5. USE WITH VIOR
   vior virtual create --width 1179 --height 2556
   vior start --display <id>
`

const windowsSetupGuide = `=== Windows Virtual Display Setup ===

Windows requires an Indirect Display Driver (IDD) to create virtual displays.
Two options:

OPTION A — Physical Dummy Plug (~$10, EASIEST)
  Buy a dummy HDMI/DP plug from Amazon. Windows detects it as a real
  monitor. Set any resolution in Display Settings.

OPTION B — Microsoft IDD Sample Driver (FREE, MORE WORK)
  1. Install Visual Studio with WDK (Windows Driver Kit)
  2. Clone:  git clone https://github.com/microsoft/Windows-driver-samples
  3. Build:  video/IndirectDisplay/idd_sample
  4. Install the driver via Device Manager:
     - Action > Add legacy hardware
     - Install from disk > select the .inf file
  5. Virtual displays appear in Settings > Display

OPTION C — Open-Source alternatives
  - virtual-display-rs: github.com/nyantronics/virtual-display-rs
  - Virtual Monitor: github.com/roberterce/Virtual-Monitor

After setup:
  vior displays                   (find your virtual display)
  vior start --display <id>       (stream it)
`
