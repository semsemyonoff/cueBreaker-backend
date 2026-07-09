package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"git.horn/cueBreaker/backend/internal/config"
	"git.horn/cueBreaker/backend/internal/job"
	"git.horn/cueBreaker/backend/internal/server"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting cueBreaker",
		"version", version,
		"input_dir", cfg.InputDir,
		"output_dir", cfg.OutputDir,
		"port", cfg.Port,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jobs := job.NewManager(ctx, nil)

	srv, err := server.New(cfg, jobs, version, logger)
	if err != nil {
		slog.Error("failed to build server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: srv,
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		if err := httpServer.Shutdown(context.Background()); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
