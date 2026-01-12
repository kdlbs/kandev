package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func newTestConfig(t *testing.T) Config {
	tmpDir := t.TempDir()
	return Config{
		Enabled:      true,
		BasePath:     tmpDir,
		MaxPerRepo:   10,
		BranchPrefix: "kandev/",
	}
}

// mockStore implements Store for testing
type mockStore struct {
	worktrees map[string]*Worktree
}

func newMockStore() *mockStore {
	return &mockStore{
		worktrees: make(map[string]*Worktree),
	}
}

func (s *mockStore) CreateWorktree(ctx context.Context, wt *Worktree) error {
	s.worktrees[wt.ID] = wt
	return nil
}

func (s *mockStore) GetWorktreeByTaskID(ctx context.Context, taskID string) (*Worktree, error) {
	for _, wt := range s.worktrees {
		if wt.TaskID == taskID {
			return wt, nil
		}
	}
	return nil, nil
}

func (s *mockStore) GetWorktreesByRepositoryID(ctx context.Context, repoID string) ([]*Worktree, error) {
	var result []*Worktree
	for _, wt := range s.worktrees {
		if wt.RepositoryID == repoID {
			result = append(result, wt)
		}
	}
	return result, nil
}

func (s *mockStore) UpdateWorktree(ctx context.Context, wt *Worktree) error {
	s.worktrees[wt.ID] = wt
	return nil
}

func (s *mockStore) DeleteWorktree(ctx context.Context, id string) error {
	delete(s.worktrees, id)
	return nil
}

func (s *mockStore) ListActiveWorktrees(ctx context.Context) ([]*Worktree, error) {
	var result []*Worktree
	for _, wt := range s.worktrees {
		if wt.Status == StatusActive {
			result = append(result, wt)
		}
	}
	return result, nil
}

func TestNewManager(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if !mgr.IsEnabled() {
		t.Error("expected manager to be enabled")
	}
}

func TestNewManager_DisabledConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{
		Enabled:  false,
		BasePath: tmpDir,
	}
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if mgr.IsEnabled() {
		t.Error("expected manager to be disabled")
	}
}

func TestManager_IsValid(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test non-existent path
	if mgr.IsValid("/nonexistent/path") {
		t.Error("expected false for non-existent path")
	}

	// Create a mock worktree directory
	worktreePath := filepath.Join(cfg.BasePath, "test-worktree")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}

	// Without .git file - should be invalid
	if mgr.IsValid(worktreePath) {
		t.Error("expected false for directory without .git file")
	}

	// With proper .git file
	gitFile := filepath.Join(worktreePath, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/path/.git/worktrees/test"), 0644); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}

	if !mgr.IsValid(worktreePath) {
		t.Error("expected true for valid worktree directory")
	}
}

