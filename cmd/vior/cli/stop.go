package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func pidFile() string {
	return os.TempDir() + string(os.PathSeparator) + "vior.pid"
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running Vior streaming session",
	Long:  "Stop the currently running Vior streaming session by sending an interrupt signal to the process.",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(pidFile())
		if err != nil {
			return fmt.Errorf("no running Vior session found (PID file missing at %s)", pidFile())
		}

		pidStr := strings.TrimSpace(string(data))
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			return fmt.Errorf("invalid PID file: %s", pidStr)
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("process %d not found: %w", pid, err)
		}

		if err := proc.Signal(os.Interrupt); err != nil {
			return fmt.Errorf("failed to stop process %d: %w", pid, err)
		}

		fmt.Printf("Sent stop signal to Vior (PID %d)\n", pid)
		return nil
	},
}
