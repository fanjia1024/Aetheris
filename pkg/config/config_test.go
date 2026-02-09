package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	yaml := `
api:
  port: 9000
  host: "127.0.0.1"
log:
  level: "debug"
`
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.API.Port != 9000 {
		t.Errorf("API.Port: got %d", cfg.API.Port)
	}
	if cfg.API.Host != "127.0.0.1" {
		t.Errorf("API.Host: got %q", cfg.API.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level: got %q", cfg.Log.Level)
	}
}
