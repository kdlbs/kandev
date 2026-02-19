package worktree

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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

func (s *mockStore) GetWorktreeBySessionID(ctx context.Context, sessionID string) (*Worktree, error) {
	for _, wt := range s.worktrees {
		if wt.SessionID == sessionID {
			return wt, nil
		}
	}
	return nil, nil
}

func (s *mockStore) GetWorktreeByID(ctx context.Context, id string) (*Worktree, error) {
	wt, ok := s.worktrees[id]
	if !ok {
		return nil, nil
	}
	return wt, nil
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

func TestSmallSuffix(t *testing.T) {
	suffix := SmallSuffix(3)
	if len(suffix) == 0 || len(suffix) > 3 {
		t.Fatalf("expected suffix length 1-3, got %d (%q)", len(suffix), suffix)
	}
	if !regexp.MustCompile(`^[a-z0-9]{1,3}$`).MatchString(suffix) {
		t.Fatalf("suffix contains invalid characters: %q", suffix)
	}
}

func TestSmallSuffix_MaxLenCap(t *testing.T) {
	suffix := SmallSuffix(10)
	if len(suffix) != 3 {
		t.Fatalf("expected suffix length 3, got %d (%q)", len(suffix), suffix)
	}
}

func TestNormalizeBranchPrefix(t *testing.T) {
	if got := NormalizeBranchPrefix(""); got != DefaultBranchPrefix {
		t.Fatalf("expected default prefix %q, got %q", DefaultBranchPrefix, got)
	}
	if got := NormalizeBranchPrefix("  feature/ "); got != "feature/" {
		t.Fatalf("expected trimmed prefix %q, got %q", "feature/", got)
	}
}

func TestValidateBranchPrefix(t *testing.T) {
	valid := []string{"feature/", "bugfix-", "release_1.0/", "team/alpha"}
	for _, prefix := range valid {
		if err := ValidateBranchPrefix(prefix); err != nil {
			t.Fatalf("expected prefix %q to be valid: %v", prefix, err)
		}
	}

	invalid := []string{"bad prefix", "feature@{", "foo..bar"}
	for _, prefix := range invalid {
		if err := ValidateBranchPrefix(prefix); err == nil {
			t.Fatalf("expected prefix %q to be invalid", prefix)
		}
	}
}

func TestSemanticBranchName(t *testing.T) {
	cfg := Config{BranchPrefix: "feature/"}
	got := cfg.SemanticBranchName("fix-login", "abc")
	want := "feature/fix-login-abc"
	if got != want {
		t.Fatalf("SemanticBranchName() = %q, want %q", got, want)
	}
}

// TestWorktreeCache_SessionIDKeying tests that cache uses sessionID consistently
func TestWorktreeCache_SessionIDKeying(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a test worktree entry
	sessionID := "test-session-123"
	taskID := "test-task-456"
	wt := &Worktree{
		ID:        "wt-001",
		SessionID: sessionID,
		TaskID:    taskID,
		Path:      "/test/path",
	}

	// Add to cache using sessionID
	mgr.mu.Lock()
	mgr.worktrees[sessionID] = wt
	mgr.mu.Unlock()

	// Verify cache contains entry with sessionID key
	mgr.mu.RLock()
	cached, exists := mgr.worktrees[sessionID]
	mgr.mu.RUnlock()

	if !exists {
		t.Fatal("expected worktree to be in cache with sessionID key")
	}
	if cached.ID != wt.ID {
		t.Errorf("cached worktree ID = %q, want %q", cached.ID, wt.ID)
	}

	// Verify cache does NOT contain entry with taskID key
	mgr.mu.RLock()
	_, existsByTaskID := mgr.worktrees[taskID]
	mgr.mu.RUnlock()

	if existsByTaskID {
		t.Error("cache should not contain entry with taskID key")
	}

	// Simulate cache deletion (as done in removeWorktree)
	mgr.mu.Lock()
	if wt.SessionID != "" {
		delete(mgr.worktrees, wt.SessionID)
	}
	mgr.mu.Unlock()

	// Verify cache no longer contains entry
	mgr.mu.RLock()
	_, stillExists := mgr.worktrees[sessionID]
	mgr.mu.RUnlock()

	if stillExists {
		t.Error("expected worktree to be removed from cache")
	}
}

// TestWorktreeCache_EmptySessionID tests cache deletion with empty sessionID
func TestWorktreeCache_EmptySessionID(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a worktree with empty sessionID
	wt := &Worktree{
		ID:        "wt-002",
		SessionID: "",
		TaskID:    "test-task-789",
		Path:      "/test/path2",
	}

	// Add to cache with a key
	mgr.mu.Lock()
	mgr.worktrees["some-key"] = wt
	mgr.mu.Unlock()

	// Attempt deletion with empty sessionID (should not panic)
	mgr.mu.Lock()
	if wt.SessionID != "" {
		delete(mgr.worktrees, wt.SessionID)
	}
	mgr.mu.Unlock()

	// Verify original entry still exists (wasn't deleted)
	mgr.mu.RLock()
	_, exists := mgr.worktrees["some-key"]
	mgr.mu.RUnlock()

	if !exists {
		t.Error("entry should still exist when sessionID is empty")
	}
}

// TestRepoLocks_ReferenceCountingCleanup tests lock cleanup with reference counting
func TestRepoLocks_ReferenceCountingCleanup(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := "/test/repo"

	// Acquire lock for the first time
	lock1 := mgr.getRepoLock(repoPath)
	if lock1 == nil {
		t.Fatal("expected non-nil lock")
	}

	// Verify lock exists in map with refCount = 1
	mgr.repoLockMu.Lock()
	entry, exists := mgr.repoLocks[repoPath]
	mgr.repoLockMu.Unlock()

	if !exists {
		t.Fatal("expected lock entry to exist in map")
	}
	if entry.refCount != 1 {
		t.Errorf("expected refCount = 1, got %d", entry.refCount)
	}

	// Acquire same lock again
	lock2 := mgr.getRepoLock(repoPath)
	if lock2 != lock1 {
		t.Error("expected same lock instance")
	}

	// Verify refCount increased to 2
	mgr.repoLockMu.Lock()
	entry, exists = mgr.repoLocks[repoPath]
	mgr.repoLockMu.Unlock()

	if !exists {
		t.Fatal("expected lock entry to exist in map")
	}
	if entry.refCount != 2 {
		t.Errorf("expected refCount = 2, got %d", entry.refCount)
	}

	// Release lock once
	mgr.releaseRepoLock(repoPath)

	// Verify refCount decreased to 1
	mgr.repoLockMu.Lock()
	entry, exists = mgr.repoLocks[repoPath]
	mgr.repoLockMu.Unlock()

	if !exists {
		t.Fatal("expected lock entry to still exist in map")
	}
	if entry.refCount != 1 {
		t.Errorf("expected refCount = 1, got %d", entry.refCount)
	}

	// Release lock again
	mgr.releaseRepoLock(repoPath)

	// Verify lock removed from map when refCount reaches 0
	mgr.repoLockMu.Lock()
	_, exists = mgr.repoLocks[repoPath]
	mgr.repoLockMu.Unlock()

	if exists {
		t.Error("expected lock entry to be removed from map when refCount reaches 0")
	}
}

// TestRepoLocks_MultipleRepositories tests lock isolation between repositories
func TestRepoLocks_MultipleRepositories(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repo1 := "/test/repo1"
	repo2 := "/test/repo2"

	// Acquire locks for different repositories
	lock1 := mgr.getRepoLock(repo1)
	lock2 := mgr.getRepoLock(repo2)

	if lock1 == lock2 {
		t.Error("expected different lock instances for different repositories")
	}

	// Verify both locks exist
	mgr.repoLockMu.Lock()
	entry1, exists1 := mgr.repoLocks[repo1]
	entry2, exists2 := mgr.repoLocks[repo2]
	mgr.repoLockMu.Unlock()

	if !exists1 || !exists2 {
		t.Fatal("expected both lock entries to exist")
	}
	if entry1.refCount != 1 || entry2.refCount != 1 {
		t.Error("expected both locks to have refCount = 1")
	}

	// Release lock for repo1
	mgr.releaseRepoLock(repo1)

	// Verify repo1 lock removed, repo2 lock still exists
	mgr.repoLockMu.Lock()
	_, exists1 = mgr.repoLocks[repo1]
	_, exists2 = mgr.repoLocks[repo2]
	mgr.repoLockMu.Unlock()

	if exists1 {
		t.Error("expected repo1 lock to be removed")
	}
	if !exists2 {
		t.Error("expected repo2 lock to still exist")
	}
}

// TestRepoLocks_ReleaseNonexistent tests releasing a lock that doesn't exist
func TestRepoLocks_ReleaseNonexistent(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Release a lock that was never acquired (should not panic)
	mgr.releaseRepoLock("/nonexistent/repo")

	// Verify no locks in map
	mgr.repoLockMu.Lock()
	count := len(mgr.repoLocks)
	mgr.repoLockMu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 locks in map, got %d", count)
	}
}

func writeFakeGitScript(t *testing.T, scriptBody string) string {
	t.Helper()

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "git")
	content := "#!/bin/sh\nset -eu\n\n" + scriptBody + "\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write fake git script: %v", err)
	}
	return scriptDir
}

func TestPullBaseBranch_UsesNonInteractiveGitEnv(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "git-env.log")
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  fetch)
    printf "%s|%s|%s|%s|%s" \
      "${GIT_TERMINAL_PROMPT:-}" \
      "${GCM_INTERACTIVE:-}" \
      "${GIT_ASKPASS:-}" \
      "${SSH_ASKPASS:-}" \
      "${GIT_SSH_COMMAND:-}" > "${KD_GIT_ENV_LOG:?}"
    exit 0
    ;;
  rev-parse)
    if [ "${2:-}" = "--abbrev-ref" ]; then
      echo "master"
    fi
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`)

	t.Setenv("KD_GIT_ENV_LOG", logPath)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := t.TempDir()
	ref := mgr.pullBaseBranch(repoPath, "origin/master")
	if ref != "origin/master" {
		t.Fatalf("pullBaseBranch() ref = %q, want %q", ref, "origin/master")
	}

	envBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading fake git env log: %v", err)
	}

	got := string(envBytes)
	want := "0|Never|echo|/bin/false|ssh -oBatchMode=yes"
	if got != want {
		t.Fatalf("fake git env = %q, want %q", got, want)
	}
}

func TestPullBaseBranch_FetchTimeoutFallsBackQuickly(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  fetch)
    sleep 2
    exit 0
    ;;
  rev-parse)
    if [ "${2:-}" = "--abbrev-ref" ]; then
      echo "master"
    fi
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`)

	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	mgr.fetchTimeout = 100 * time.Millisecond

	repoPath := t.TempDir()
	start := time.Now()
	ref := mgr.pullBaseBranch(repoPath, "master")
	elapsed := time.Since(start)

	if ref != "master" {
		t.Fatalf("pullBaseBranch() ref = %q, want %q", ref, "master")
	}
	// Allow CI scheduling variance while still asserting we timed out
	// well before the fake 2s fetch command completes.
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("pullBaseBranch() took too long: %v", elapsed)
	}
}

func TestPullBaseBranch_PullFailureFallsBackToRemoteRef(t *testing.T) {
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  fetch)
    exit 0
    ;;
  pull)
    echo "Authentication failed" 1>&2
    exit 1
    ;;
  rev-parse)
    if [ "${2:-}" = "--abbrev-ref" ]; then
      echo "master"
      exit 0
    fi
    if [ "${2:-}" = "--verify" ]; then
      exit 0
    fi
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`)

	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	mgr.pullTimeout = 300 * time.Millisecond

	repoPath := t.TempDir()
	ref := mgr.pullBaseBranch(repoPath, "master")
	if ref != "origin/master" {
		t.Fatalf("pullBaseBranch() ref = %q, want %q", ref, "origin/master")
	}
}

func TestClassifyGitFallbackReason_AuthPrompt(t *testing.T) {
	reason := classifyGitFallbackReason(nil, "fatal: could not read Username for 'https://github.com'", nil)
	if reason != "non_interactive_auth_failed" {
		t.Fatalf("classifyGitFallbackReason() = %q, want %q", reason, "non_interactive_auth_failed")
	}
}

func TestClassifyGitFallbackReason_Timeout(t *testing.T) {
	reason := classifyGitFallbackReason(context.DeadlineExceeded, "", context.DeadlineExceeded)
	if !strings.EqualFold(reason, "timeout") {
		t.Fatalf("classifyGitFallbackReason() = %q, want %q", reason, "timeout")
	}
}
