package configloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherDetectsFileChange(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	watcher, err := NewWatcher(loader)
	if err != nil {
		t.Fatalf("NewWatcher() error: %v", err)
	}
	watcher.SetDebounceDuration(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = watcher.Start(ctx)
	}()

	// Wait for watcher to register paths.
	time.Sleep(50 * time.Millisecond)

	// Write a new agent file.
	agentsDir := filepath.Join(base, "workspaces", "default", "agents")
	writeFile(t, filepath.Join(agentsDir, "new-agent.yml"), `
name: new-agent
role: worker
budget_monthly_cents: 1000
`)

	// Wait for debounce + reload.
	time.Sleep(200 * time.Millisecond)

	agents := loader.GetAgents("default")
	found := false
	for _, a := range agents {
		if a.Name == "new-agent" {
			found = true
		}
	}
	if !found {
		t.Error("watcher did not detect new agent file")
	}

	// Cleanup.
	cancel()
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestWatcherDetectsFileDeletion(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Verify ceo exists.
	agents := loader.GetAgents("default")
	hasCEO := false
	for _, a := range agents {
		if a.Name == "ceo" {
			hasCEO = true
		}
	}
	if !hasCEO {
		t.Fatal("expected ceo agent to exist before deletion")
	}

	watcher, err := NewWatcher(loader)
	if err != nil {
		t.Fatalf("NewWatcher() error: %v", err)
	}
	watcher.SetDebounceDuration(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = watcher.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Delete the ceo agent file.
	ceoPath := filepath.Join(base, "workspaces", "default", "agents", "ceo.yml")
	if err := os.Remove(ceoPath); err != nil {
		t.Fatalf("remove ceo.yml: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	agents = loader.GetAgents("default")
	for _, a := range agents {
		if a.Name == "ceo" {
			t.Error("watcher did not detect ceo file deletion")
		}
	}

	cancel()
	_ = watcher.Stop()
}

func TestWatcherStopIsIdempotent(t *testing.T) {
	base := setupTestWorkspace(t)
	loader := NewConfigLoader(base)
	_ = loader.Load()

	watcher, err := NewWatcher(loader)
	if err != nil {
		t.Fatalf("NewWatcher() error: %v", err)
	}

	// Stop without ever starting should not panic.
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}
