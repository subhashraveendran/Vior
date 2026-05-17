package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/subhashraveendran/vior/internal/capture"
)

var displaysCmd = &cobra.Command{
	Use:   "displays",
	Short: "List available displays",
	Long:  "Enumerate all connected displays and their properties.",
	RunE: func(cmd *cobra.Command, args []string) error {
		displays, err := capture.ListDisplays()
		if err != nil {
			return fmt.Errorf("failed to detect displays: %w", err)
		}

		fmt.Printf("Found %d display(s):\n\n", len(displays))
		for _, d := range displays {
			main := ""
			if d.IsMain {
				main = " (main)"
			}
			fmt.Printf("  [%d] %s%s — %dx%d\n", d.Index, d.Name, main, d.Width, d.Height)
		}
		return nil
	},
}
