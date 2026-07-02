package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("CUEBREAKER_INPUT_DIR", "")
	t.Setenv("CUEBREAKER_OUTPUT_DIR", "")
	t.Setenv("CUEBREAKER_PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.InputDir != defaultInputDir {
		t.Errorf("InputDir = %q, want %q", cfg.InputDir, defaultInputDir)
	}
	if cfg.OutputDir != defaultOutputDir {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, defaultOutputDir)
	}
	if cfg.Port != defaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, defaultPort)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("CUEBREAKER_INPUT_DIR", "/custom/input")
	t.Setenv("CUEBREAKER_OUTPUT_DIR", "/custom/output")
	t.Setenv("CUEBREAKER_PORT", "8080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.InputDir != "/custom/input" {
		t.Errorf("InputDir = %q, want %q", cfg.InputDir, "/custom/input")
	}
	if cfg.OutputDir != "/custom/output" {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, "/custom/output")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want %d", cfg.Port, 8080)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{"not a number", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
		{"too large", "70000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CUEBREAKER_PORT", tt.port)
			if _, err := Load(); err == nil {
				t.Errorf("Load() with CUEBREAKER_PORT=%q: expected error, got nil", tt.port)
			}
		})
	}
}
