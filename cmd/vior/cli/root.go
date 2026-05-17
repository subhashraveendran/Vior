// Package cli implements the Vior command-line interface using cobra.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/subhashraveendran/vior/internal/config"
)

var (
	cfgFile string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "vior",
	Short: "Vior — Extend your view. Stream, control, transfer.",
	Long: `Vior is a cross-platform screen streaming and extended display tool.

Stream your screen to other devices, use a phone or tablet as an
extended display, control input remotely, and transfer files — all
from the command line.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default $HOME/.vior.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

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
		fmt.Printf("vior %s\n", config.Version)
	},
}
