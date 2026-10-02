package gocache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestCleanupDeletesContentsWithoutQuarantine(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.MaxBytes = 1
	provider := New(Config{
		HomeDir:  home,
		TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	environment, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	cachePath := environment["GOCACHE"]
	artifact := filepath.Join(cachePath, "00", "compiled-artifact")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
		t.Fatalf("create cache shard: %v", err)
	}
	if err := os.WriteFile(artifact, []byte("compiled bytes"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache artifact still exists: %v", err)
	}
	if !hasValidMarker(cachePath) {
		t.Fatal("cleanup did not preserve the ownership marker")
	}
	if result.QuarantineEntry != nil {
		t.Fatalf("cleanup created a quarantine entry: %#v", result.QuarantineEntry)
	}
	if _, err := os.Stat(filepath.Join(home, "trash", "go-cache")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup created Go-cache trash: %v", err)
	}
}
