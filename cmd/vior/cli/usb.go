package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/subhashraveendran/vior/internal/adb"
)

var usbPort int

var usbCmd = &cobra.Command{
	Use:   "usb",
	Short: "Manage USB/ADB connection for phone tethering",
	Long:  "Set up or tear down ADB reverse port forwarding so a USB-connected Android device can access the Vior server.",
}

var usbSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up ADB port forwarding for USB connection",
	RunE: func(cmd *cobra.Command, args []string) error {
		status := adb.Check()
		if !status.Available {
			return fmt.Errorf("adb not found in PATH. Install Android Debug Bridge:\n  brew install android-platform-tools  (macOS)\n  apt install adb                      (Linux)")
		}
		if !status.Connected {
			return fmt.Errorf("no Android device connected. Plug in via USB and enable USB debugging.")
		}

		if err := adb.SetupForward(usbPort, usbPort); err != nil {
			return err
		}

		fmt.Printf("USB forwarding active: phone localhost:%d → desktop localhost:%d\n", usbPort, usbPort)
		if status.DeviceName != "" {
			fmt.Printf("Device: %s\n", status.DeviceName)
		}
		fmt.Printf("\nOpen http://localhost:%d on the phone browser.\n", usbPort)
		return nil
	},
}

var usbTeardownCmd = &cobra.Command{
	Use:   "teardown",
	Short: "Remove ADB port forwarding",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := adb.TeardownForward(usbPort); err != nil {
			return err
		}
		fmt.Println("USB forwarding removed.")
		return nil
	},
}

var usbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check ADB and USB connection status",
	Run: func(cmd *cobra.Command, args []string) {
		status := adb.Check()
		if !status.Available {
			fmt.Println("ADB: not installed")
			return
		}
		fmt.Println("ADB: installed")
		if status.Connected {
			fmt.Printf("Device: %s (connected)\n", status.DeviceName)
			if status.Forwarding {
				fmt.Println("Port forwarding: active")
			} else {
				fmt.Println("Port forwarding: inactive")
			}
		} else {
			fmt.Println("Device: none connected")
		}
	},
}

func init() {
	usbCmd.PersistentFlags().IntVar(&usbPort, "port", 8080, "port to forward")
	usbCmd.AddCommand(usbSetupCmd)
	usbCmd.AddCommand(usbTeardownCmd)
	usbCmd.AddCommand(usbStatusCmd)
}
