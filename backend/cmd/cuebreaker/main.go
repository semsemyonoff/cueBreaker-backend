package main

import (
	"log/slog"
	"os"

	"git.horn/cueBreaker/backend/internal/config"
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
}
