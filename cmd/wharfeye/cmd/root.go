package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mathesh-me/wharfeye/internal/config"
	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/tui"
)

var (
	cfgFile     string
	runtimeFlag string
	socketFlag  string
)

var rootCmd = &cobra.Command{
	Use:   "wharfeye",
	Short: "a cli and web based tool to inspect, audit, and optimize containers",
	Long:  "a cli and web based tool to inspect container metrics, audit security configurations, and get performance recommendations across Docker, Podman, and containerd.",
	RunE:  runTUI,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/wharfeye/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&runtimeFlag, "runtime", "", "container runtime: auto, docker, podman, containerd")
	rootCmd.PersistentFlags().StringVar(&socketFlag, "socket", "", "runtime socket path")

	if err := viper.BindPFlag("runtime.type", rootCmd.PersistentFlags().Lookup("runtime")); err != nil {
		slog.Error("binding runtime flag", "error", err)
	}
	if err := viper.BindPFlag("runtime.socket", rootCmd.PersistentFlags().Lookup("socket")); err != nil {
		slog.Error("binding socket flag", "error", err)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	client, err := detectClient(ctx)
	if err != nil {
		return fmt.Errorf("detecting runtime: %w", err)
	}

	eng := engine.New(client, engine.DefaultConfig())
	scanner := engine.NewScanner(client)
	model := tui.NewModel(eng, scanner)

	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}

	return nil
}

func initConfig() {
	config.SetDefaults()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(config.ConfigDir())
		viper.AddConfigPath(".")
	}

	viper.SetEnvPrefix("WHARFEYE")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			slog.Error("reading config", "error", err)
			os.Exit(1)
		}
	}
}
