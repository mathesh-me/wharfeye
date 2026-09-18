package cmd

import (
	"context"
	"fmt"
	"net"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mathesh-me/wharfeye/internal/engine"
	"github.com/mathesh-me/wharfeye/internal/web"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Launch web dashboard",
	Long:  "Start the WharfEye web dashboard with real-time container monitoring.",
	RunE:  runWeb,
}

func init() {
	webCmd.Flags().IntP("port", "p", 9090, "web server port")
	webCmd.Flags().String("host", "127.0.0.1", "web server host (use 0.0.0.0 to expose on all interfaces)")
	if err := viper.BindPFlag("web.port", webCmd.Flags().Lookup("port")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("web.host", webCmd.Flags().Lookup("host")); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	client, err := detectClient(ctx)
	if err != nil {
		return fmt.Errorf("detecting runtime: %w", err)
	}

	eng := engine.New(client, engine.DefaultConfig())
	scanner := engine.NewScanner(client)

	host := viper.GetString("web.host")
	port := viper.GetInt("web.port")
	addr := fmt.Sprintf("%s:%d", host, port)

	if !isLoopback(host) {
		fmt.Printf("warning: binding to %s exposes the dashboard, including security findings, to the network. It has no authentication.\n", host)
	}

	srv := web.NewServer(eng, scanner, client)
	fmt.Printf("WharfEye web dashboard: http://%s:%d\n", host, port)
	return srv.Start(ctx, addr)
}

// isLoopback reports whether host resolves to a loopback address only.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
