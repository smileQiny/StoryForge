package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"storyforge/internal/config"
	"storyforge/internal/logging"
	"storyforge/internal/tui"
	"storyforge/web/api"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tui" {
		if err := tui.Run(os.Args[2:]); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		return
	}

	cfg := config.Load()
	logger := logging.NewLogger(cfg.LogLevel)

	handler, err := api.NewHandler(logger)
	if err != nil {
		logger.Error("failed to initialize HTTP handler", "error", err)
		return
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
		}
	}()

	logger.Info("starting storyforge HTTP server", "addr", cfg.Addr, "log_level", cfg.LogLevel.String(), "version", version)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server exited unexpectedly", "error", err)
	}
}
