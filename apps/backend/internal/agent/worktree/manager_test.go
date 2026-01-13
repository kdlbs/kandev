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

func (s *mockStore) GetWorktreesByTaskID(ctx context.Context, taskID string) ([]*Worktree, error) {
	var result []*Worktree
	for _, wt := range s.worktrees {
		if wt.TaskID == taskID {
			result = append(result, wt)
		}
	}
	return result, nil
}

func (s *mockStore) GetWorktreeByID(ctx context.Context, id string) (*Worktree, error) {
	wt, ok := s.worktrees[id]
	if !ok {
		return nil, nil
	}
	return wt, nil
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

func TestSanitizeForBranch(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		maxLen   int
		expected string
	}{
		{
			name:     "simple title",
			title:    "Fix login bug",
			maxLen:   20,
			expected: "fix-login-bug",
		},
		{
			name:     "title with special chars",
			title:    "Fix: bug #123 (urgent!)",
			maxLen:   20,
			expected: "fix-bug-123-urgent",
		},
		{
			name:     "title exceeding max length",
			title:    "This is a very long task title that needs truncation",
			maxLen:   20,
			expected: "this-is-a-very-long",
		},
		{
			name:     "title with consecutive spaces",
			title:    "Fix   multiple   spaces",
			maxLen:   20,
			expected: "fix-multiple-spaces",
		},
		{
			name:     "empty title",
			title:    "",
			maxLen:   20,
			expected: "",
		},
		{
			name:     "title starting and ending with special chars",
			title:    "---Fix bug---",
			maxLen:   20,
			expected: "fix-bug",
		},
		{
			name:     "title with numbers",
			title:    "Task 123 done",
			maxLen:   20,
			expected: "task-123-done",
		},
		{
			name:     "truncation at boundary",
			title:    "Fix the login page bug",
			maxLen:   15,
			expected: "fix-the-login-p",
		},
		{
			name:     "truncation at hyphen position removes trailing hyphen",
			title:    "Fix the login-page bug",
			maxLen:   13,
			expected: "fix-the-login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeForBranch(tt.title, tt.maxLen)
			if result != tt.expected {
				t.Errorf("SanitizeForBranch(%q, %d) = %q, want %q", tt.title, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestSemanticWorktreeName(t *testing.T) {
	tests := []struct {
		name      string
		taskTitle string
		suffix    string
		expected  string
	}{
		{
			name:      "normal title with suffix",
			taskTitle: "Fix login bug",
			suffix:    "ab12cd34",
			expected:  "fix-login-bug_ab12cd34",
		},
		{
			name:      "long title truncated",
			taskTitle: "This is a very long task title that needs truncation",
			suffix:    "ab12cd34",
			expected:  "this-is-a-very-long_ab12cd34",
		},
		{
			name:      "empty title falls back to suffix only",
			taskTitle: "",
			suffix:    "ab12cd34",
			expected:  "ab12cd34",
		},
		{
			name:      "title with only special chars",
			taskTitle: "!@#$%^&*()",
			suffix:    "ab12cd34",
			expected:  "ab12cd34",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SemanticWorktreeName(tt.taskTitle, tt.suffix)
			if result != tt.expected {
				t.Errorf("SemanticWorktreeName(%q, %q) = %q, want %q", tt.taskTitle, tt.suffix, result, tt.expected)
			}
		})
	}
}
