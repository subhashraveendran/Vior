package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/subhashraveendran/vior/internal/virtual"
)

var (
	vWidth      uint32
	vHeight     uint32
	vRefresh    float64
	vHiDPI      bool
	vAutoDetect string
)

var virtualCmd = &cobra.Command{
	Use:   "virtual",
	Short: "Manage virtual displays (macOS only)",
	Long: `Create and destroy virtual displays on macOS using CGVirtualDisplay.

Virtual displays appear as real monitors to macOS and can be streamed
to phones, tablets, or other devices with custom resolutions matching
the target device dimensions.`,
}

var virtualCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a virtual display with custom dimensions",
	Long: `Create a virtual display with the specified width and height.

The display appears immediately in System Settings and can be captured
by 'vior start --display <id>'.

Example (match iPhone 15 Pro dimensions):
  vior virtual create --width 1179 --height 2556

Example (HiDPI Retina scale, logical points):
  vior virtual create --width 393 --height 852 --hidpi

Example (preset for common phone):
  vior virtual create --device iphone`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if vAutoDetect != "" {
			w, h, ok := deviceResolution(vAutoDetect)
			if !ok {
				return fmt.Errorf("unknown device preset: %s (try iphone, iphone-pro, ipad, or ipad-pro)", vAutoDetect)
			}
			vWidth, vHeight = w, h
		}

		if vWidth == 0 || vHeight == 0 {
			return fmt.Errorf("width and height required (use --device for presets)")
		}

		info := virtual.Info{
			Width:       vWidth,
			Height:      vHeight,
			RefreshRate: vRefresh,
			HiDPI:       vHiDPI,
		}

		fmt.Printf("Creating virtual display %dx%d @ %.0fHz...\n", vWidth, vHeight, vRefresh)
		displayID, err := virtual.CreateVirtualDisplay(info)
		if err != nil {
			return fmt.Errorf("failed to create virtual display: %w", err)
		}

		fmt.Printf("Virtual display created (ID: %d)\n", displayID)
		fmt.Println()
		fmt.Printf("Stream it:  vior start --display %d\n", displayID)
		fmt.Println("List all:   vior displays")
		fmt.Println("Destroy:    vior virtual destroy")
		return nil
	},
}

var virtualDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy the current virtual display",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Destroying virtual display...")
		virtual.Destroy()
		fmt.Println("Virtual display destroyed.")
		return nil
	},
}

func init() {
	virtualCmd.AddCommand(virtualCreateCmd)
	virtualCmd.AddCommand(virtualDestroyCmd)
	rootCmd.AddCommand(virtualCmd)

	virtualCreateCmd.Flags().Uint32Var(&vWidth, "width", 0, "width in pixels")
	virtualCreateCmd.Flags().Uint32Var(&vHeight, "height", 0, "height in pixels")
	virtualCreateCmd.Flags().Float64Var(&vRefresh, "refresh", 60, "refresh rate in Hz")
	virtualCreateCmd.Flags().BoolVar(&vHiDPI, "hidpi", false, "treat width/height as logical points (2x Retina)")
	virtualCreateCmd.Flags().StringVarP(&vAutoDetect, "device", "D", "", "device preset (iphone, iphone-pro, ipad, ipad-pro)")
}

func deviceResolution(name string) (uint32, uint32, bool) {
	switch name {
	case "iphone":
		return 1170, 2532, true // iPhone 14
	case "iphone-pro":
		return 1179, 2556, true // iPhone 15 Pro
	case "iphone-plus":
		return 1284, 2778, true // iPhone 14 Plus
	case "ipad":
		return 1640, 2360, true // iPad Air
	case "ipad-pro":
		return 2048, 2732, true // iPad Pro 12.9"
	case "ipad-mini":
		return 1488, 2266, true // iPad Mini
	default:
		return 0, 0, false
	}
}
