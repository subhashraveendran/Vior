// Package cli implements the Vior command-line interface using cobra.
package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/subhashraveendran/vior/internal/config"
)

var rootCmd = &cobra.Command{
	Use:   "vior",
	Short: "Phone as second display — no cloud, no accounts",
	Long: `Vior turns your phone or tablet into a real second display,
wireless trackpad, soft keyboard, and file-drop target for your computer.

  vior start                 Start the server and wait for a phone
  vior start --port 8080     Custom port
  vior start --fps 60        Higher frame rate
  vior displays               List connected displays
  vior display extend 1       Make display 1 a separate screen
  vior display mirror --source 1 --target 0   Mirror display 1 onto 0
  vior usb setup              Set up USB cable connection
  vior usb status             Check USB device
  vior stop                   Stop running server
  vior version                Print version

Everything happens over your local Wi-Fi or a USB cable.
No accounts, no telemetry, no cloud. MIT licensed.`,
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolP("version", "V", false, "print version")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(displaysCmd)
	rootCmd.AddCommand(usbCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Vior version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("vior %s  (go1.25.6 %s/%s)\n", config.Version, runtime.GOOS, runtime.GOARCH)
	},
}
