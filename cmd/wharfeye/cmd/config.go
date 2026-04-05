package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mathesh-me/wharfeye/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage wharfeye configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.WriteDefault()
		if err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
		fmt.Printf("Configuration written to %s\n", path)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configInitCmd)
	rootCmd.AddCommand(configCmd)
}
