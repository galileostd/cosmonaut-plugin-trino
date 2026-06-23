// Cosmonaut Plugin — Apache Trino
// Implements the Cosmonaut PluginService gRPC interface for Trino.
package main

import (
	"context"
	"log/slog"
	"os"

	sdkserver "github.com/galileostd/cosmonaut-sdk/go/server"
	"github.com/galileostd/cosmonaut-plugin-trino/internal/plugin"
)

func main() {
	addr := envOr("COSMONAUT_PLUGIN_ADDR", ":50051")

	slog.Info("starting cosmonaut-plugin-trino", "addr", addr)

	srv := sdkserver.New(addr, plugin.New())

	if err := srv.Serve(context.Background()); err != nil {
		slog.Error("plugin server error", "err", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
