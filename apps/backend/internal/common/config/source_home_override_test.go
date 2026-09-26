package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWithHomeUsesOverrideForHomeFileDiscovery(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", "")
	t.Setenv("KANDEV_SERVER_PORT", "")
	t.Setenv("KANDEV_BACKEND_PORT", "")
	t.Setenv("KANDEV_PORT", "")
	workingDir := t.TempDir()
	homeDir := t.TempDir()
	t.Chdir(workingDir)
	configPath := filepath.Join(homeDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  port: 40128\n"), 0o600); err != nil {
		t.Fatalf("write home config: %v", err)
	}

	cfg, err := LoadWithHome(homeDir)
	if err != nil {
		t.Fatalf("LoadWithHome: %v", err)
	}
	if cfg.Source.FilePath != configPath {
		t.Fatalf("selected config = %q, want %q", cfg.Source.FilePath, configPath)
	}
	if !cfg.Source.HomeFile {
		t.Fatal("selected override config is not marked as a home file")
	}
	if cfg.Server.Port != 40128 {
		t.Fatalf("server.port = %d, want 40128", cfg.Server.Port)
	}
}

func TestLoadWithHomePreservesWorkingDirectoryPriority(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", "")
	t.Setenv("KANDEV_SERVER_PORT", "")
	t.Setenv("KANDEV_BACKEND_PORT", "")
	t.Setenv("KANDEV_PORT", "")
	workingDir := t.TempDir()
	homeDir := t.TempDir()
	t.Chdir(workingDir)
	workingConfig := filepath.Join(workingDir, "config.yaml")
	if err := os.WriteFile(workingConfig, []byte("server:\n  port: 40129\n"), 0o600); err != nil {
		t.Fatalf("write working-directory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("server:\n  port: 40130\n"), 0o600); err != nil {
		t.Fatalf("write home config: %v", err)
	}

	cfg, err := LoadWithHome(homeDir)
	if err != nil {
		t.Fatalf("LoadWithHome: %v", err)
	}
	if cfg.Source.FilePath != workingConfig {
		t.Fatalf("selected config = %q, want %q", cfg.Source.FilePath, workingConfig)
	}
	if cfg.Source.HomeFile {
		t.Fatal("working-directory config was marked as a home file")
	}
	if cfg.Server.Port != 40129 {
		t.Fatalf("server.port = %d, want 40129", cfg.Server.Port)
	}
}
