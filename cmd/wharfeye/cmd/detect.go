package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mathesh-me/wharfeye/internal/runtime"
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Diagnose which container runtimes are available on this system",
	Long: `Probe the system for all available container runtimes and show their status.
Useful for verifying auto-detection on machines with multiple runtimes
installed, or debugging connection issues.

Detection order: Docker > Podman (rootful) > Podman (rootless) > containerd.
Use --runtime to override auto-detection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		_, infos, err := runtime.AutoDetectAll(ctx)
		if err != nil {
			return fmt.Errorf("detecting runtimes: %w", err)
		}

		fmt.Printf("Detected %d runtime(s):\n\n", len(infos))
		for i, info := range infos {
			fmt.Printf("  [%d] %s\n", i+1, info.Name)
			fmt.Printf("      Version:        %s\n", info.Version)
			fmt.Printf("      Socket:         %s\n", info.SocketPath)
			if info.StorageDriver != "" {
				fmt.Printf("      Storage Driver: %s\n", info.StorageDriver)
			}
			fmt.Printf("      OS/Arch:        %s/%s\n", info.OS, info.Arch)
			if i < len(infos)-1 {
				fmt.Println()
			}
		}

		// show inaccessible sockets with fix instructions
		inaccessible := runtime.DetectInaccessible()
		if len(inaccessible) > 0 {
			fmt.Printf("\n\nInaccessible socket(s) (permission denied):\n\n")
			for _, s := range inaccessible {
				fmt.Printf("  [!] %s\n", s.Name)
				fmt.Printf("      Socket: %s\n", s.Socket)
				fmt.Printf("      Fix:    %s\n\n", s.Fix)
			}
		}

		// about sudo if not root and some sockets were inaccessible
		if len(inaccessible) > 0 && os.Getuid() != 0 {
			fmt.Println("  Tip: Run with sudo to access all runtimes, or apply the fixes above for non-root access.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(detectCmd)
}
