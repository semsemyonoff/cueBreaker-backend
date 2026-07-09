// Package config loads cueBreaker's runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultInputDir  = "/input"
	defaultOutputDir = "/output"
	defaultPort      = 5000
)

// Config holds cueBreaker's runtime configuration.
type Config struct {
	InputDir  string
	OutputDir string
	Port      int
}

// Load reads configuration from environment variables, applying defaults for
// CUEBREAKER_INPUT_DIR, CUEBREAKER_OUTPUT_DIR, and CUEBREAKER_PORT.
func Load() (Config, error) {
	cfg := Config{
		InputDir:  defaultInputDir,
		OutputDir: defaultOutputDir,
		Port:      defaultPort,
	}

	if v := os.Getenv("CUEBREAKER_INPUT_DIR"); v != "" {
		cfg.InputDir = v
	}
	if v := os.Getenv("CUEBREAKER_OUTPUT_DIR"); v != "" {
		cfg.OutputDir = v
	}
	if v := os.Getenv("CUEBREAKER_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid CUEBREAKER_PORT %q: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("invalid CUEBREAKER_PORT %q: out of range", v)
		}
		cfg.Port = port
	}

	return cfg, nil
}
