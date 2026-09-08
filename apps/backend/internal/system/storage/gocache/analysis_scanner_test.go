package gocache

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/filescan"
)

func TestAnalyzeUsesConfiguredBoundedScanner(t *testing.T) {
	home := t.TempDir()
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings},
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	t.Setenv("GOCACHE", env["GOCACHE"])
	if err := os.WriteFile(filepath.Join(env["GOCACHE"], "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	var completed atomic.Int32
	provider.config.Scanner = filescan.NewLimiter(1)
	provider.config.OnProgress = func(progress filescan.Progress) {
		if progress.Phase == filescan.RootCompleted {
			completed.Add(1)
		}
	}

	if _, err := provider.Analyze(context.Background()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if completed.Load() != 1 {
		t.Fatalf("completed roots = %d, want one managed-cache root", completed.Load())
	}
}

func TestAnalyzeKeepsProgressMonotonicAcrossManagedAndUnmanagedCaches(t *testing.T) {
	home := t.TempDir()
	unmanagedPath := filepath.Join(t.TempDir(), "go-build")
	if err := os.MkdirAll(unmanagedPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanagedPath, "unmanaged"), []byte("unmanaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOCACHE", unmanagedPath)
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	provider := New(Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"),
		Settings: staticSettings{settings: settings}, Scanner: filescan.NewLimiter(1),
	})
	env, err := provider.ExecutionEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExecutionEnvironment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(env["GOCACHE"], "managed"), []byte("managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	var progressMu sync.Mutex
	var bytesScanned []int64
	provider.config.OnProgress = func(progress filescan.Progress) {
		progressMu.Lock()
		bytesScanned = append(bytesScanned, progress.BytesScanned)
		progressMu.Unlock()
	}

	if _, err := provider.Analyze(context.Background()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	for index := 1; index < len(bytesScanned); index++ {
		if bytesScanned[index] < bytesScanned[index-1] {
			t.Fatalf("progress bytes regressed at %d: %v", index, bytesScanned)
		}
	}
}
