package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/config"
	appLogger "github.com/UtopiaXC/UtopiaImages/internal/logger"
	transportHttp "github.com/UtopiaXC/UtopiaImages/internal/transport/http"
)

func main() {
	configPath := "config/config.yaml"
	if p := os.Getenv("UIMG_CONFIG_PATH"); p != "" {
		configPath = p
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize layered rolling logger
	logCfg := appLogger.Config{
		Level:          cfg.Logging.Level,
		Dir:            cfg.Logging.Dir,
		MaxSizeMB:      cfg.Logging.MaxSizeMB,
		MaxBackups:     cfg.Logging.MaxBackups,
		MaxAgeDays:     cfg.Logging.MaxAgeDays,
		Compress:       cfg.Logging.Compress,
		PermanentLevel: cfg.Logging.PermanentLevel,
	}
	if err := appLogger.InitLogger(logCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize logger: %v\n", err)
	}

	server, err := transportHttp.NewServer(cfg, configPath)
	if err != nil {
		slog.Error("Failed to initialize server", "error", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Engine(),
	}

	go func() {
		slog.Info("Utopia Images server listening", "addr", addr, "mode", cfg.Server.Mode)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}
	slog.Info("Server exited cleanly")
}
