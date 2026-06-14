package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/subhashraveendran/vior/internal/capture"
)

var (
	mirrorSource int
	mirrorTarget int
)

var displayCmd = &cobra.Command{
	Use:   "display",
	Short: "Manage display arrangement (extend / mirror)",
	Long:  "Mirror or extend displays. Control how virtual displays relate to your main screen.",
}

var mirrorCmd = &cobra.Command{
	Use:   "mirror",
	Short: "Mirror one display onto another",
	Long: `Mirror a display onto another. Both screens show the same content.

Example — make virtual display mirror main display:
  vior display mirror --source 1 --target 0`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mirrorSource == mirrorTarget {
			return fmt.Errorf("source and target must be different")
		}
		if err := capture.MirrorDisplay(mirrorSource, mirrorTarget); err != nil {
			return err
		}
		fmt.Printf("Display %d now mirrors display %d\n", mirrorSource, mirrorTarget)
		return nil
	},
}

var extendCmd = &cobra.Command{
	Use:   "extend",
	Short: "Extend display (separate screen)",
	Long: `Stop mirroring — the display becomes a separate extended screen.

Example:
  vior display extend 1`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var idx int
		if _, err := fmt.Sscanf(args[0], "%d", &idx); err != nil || idx < 0 {
			return fmt.Errorf("invalid display index: %q", args[0])
		}
		if err := capture.UnmirrorDisplay(idx); err != nil {
			return err
		}
		fmt.Printf("Display %d is now extended\n", idx)
		return nil
	},
}

func init() {
	mirrorCmd.Flags().IntVarP(&mirrorSource, "source", "s", 0, "source display index (the one to mirror)")
	mirrorCmd.Flags().IntVarP(&mirrorTarget, "target", "t", 0, "target display index (what it mirrors)")

	displayCmd.AddCommand(mirrorCmd)
	displayCmd.AddCommand(extendCmd)
	rootCmd.AddCommand(displayCmd)
}
