package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nidhogg1024/hverg/internal/config"
	"github.com/nidhogg1024/hverg/internal/router"

	// Import plugins to register them
	_ "github.com/nidhogg1024/hverg/internal/plugin/auth"
	_ "github.com/nidhogg1024/hverg/internal/plugin/transcoder"
)

func main() {
	configPath := flag.String("config", "hverg.yaml", "Path to the configuration file")
	flag.Parse()

	// Load Configuration
	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "path", *configPath, "err", err)
		os.Exit(1)
	}

	// Initialize Router Engine
	engine, err := router.NewEngine(cfg)
	if err != nil {
		slog.Error("Failed to initialize router engine", "err", err)
		os.Exit(1)
	}

	// Setup HTTP Server
	srv := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: engine,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("Hverg Gateway started", "addr", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "err", err)
	}

	slog.Info("Gateway exited")
}
