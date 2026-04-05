package cmd

import (
	"context"

	"github.com/spf13/viper"

	"github.com/mathesh-me/wharfeye/internal/runtime"
)

// detectClient returns a runtime client based on user flags.
// When runtime is "auto" (default), it connects to ALL available runtimes.
// When explicitly set, it connects to just that one runtime.
func detectClient(ctx context.Context) (runtime.Client, error) {
	runtimeType := viper.GetString("runtime.type")
	socketPath := viper.GetString("runtime.socket")

	// Explicit runtime - single client
	if runtimeType != "" && runtimeType != "auto" {
		return runtime.Detect(ctx, runtimeType, socketPath)
	}

	// Custom socket - single client
	if socketPath != "" {
		return runtime.Detect(ctx, runtimeType, socketPath)
	}

	// Auto-detect all runtimes
	client, _, err := runtime.AutoDetectAll(ctx)
	return client, err
}
