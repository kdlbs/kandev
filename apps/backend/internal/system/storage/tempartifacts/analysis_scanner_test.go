package tempartifacts

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/filescan"
)

func TestAnalyzeUsesConfiguredBoundedScanner(t *testing.T) {
	root := t.TempDir()
	artifacts := newFakeArtifactStore()
	quarantine := newFakeQuarantineStore()
	registry := NewRegistry(Config{
		Store: artifacts, TempRoot: root, OwnerPID: 1234,
		NewID: func() string { return "artifact-1" }, NewToken: func() string { return "token-1" },
	})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindImproveBundle, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lease.Path(), "bundle"), []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := NewProvider(ProviderConfig{Registry: registry, Store: quarantine, HomeDir: t.TempDir()})
	var completed atomic.Int32
	provider.scanner = filescan.NewLimiter(1)
	provider.onProgress = func(progress filescan.Progress) {
		if progress.Phase == filescan.RootCompleted {
			completed.Add(1)
		}
	}

	if _, err := provider.Analyze(context.Background()); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if completed.Load() != 1 {
		t.Fatalf("completed roots = %d, want one artifact root", completed.Load())
	}
}
