package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// newCleanerWithTasksRoot constructs a HandoffCleaner whose managed
// tasks-base path points at a tmp dir, so tests can write into it
// safely.
func newCleanerWithTasksRoot(t *testing.T, tasksRoot string) *HandoffCleaner {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	mgr := &Manager{
		config: Config{TasksBasePath: tasksRoot},
		logger: log,
	}
	return NewHandoffCleaner(mgr, log)
}

func TestRequireManagedRoot_AcceptsTasksRootDescendants(t *testing.T) {
	tmp := t.TempDir()
	c := newCleanerWithTasksRoot(t, tmp)

	cases := []string{
		filepath.Join(tmp, "task-1", "repo"),
		filepath.Join(tmp, "deeply", "nested", "path"),
		tmp, // the root itself is allowed
	}
	for _, p := range cases {
		if err := c.requireManagedRoot(p); err != nil {
			t.Errorf("expected %q under managed root to be accepted, got error: %v", p, err)
		}
	}
}

func TestRequireManagedRoot_RejectsPathsOutsideRoot(t *testing.T) {
	tasksRoot := t.TempDir()
	otherRoot := t.TempDir()
	c := newCleanerWithTasksRoot(t, tasksRoot)

	rejected := []string{
		"/etc",
		"/Users/somebody/projects/repo",
		filepath.Join(otherRoot, "task-1"),
		filepath.Join(tasksRoot, "..", "outside"),
	}
	for _, p := range rejected {
		err := c.requireManagedRoot(p)
		if err == nil {
			t.Errorf("expected %q to be REJECTED by managed-root guard", p)
		}
	}
}

func TestRequireManagedRoot_EmptyPath(t *testing.T) {
	c := newCleanerWithTasksRoot(t, t.TempDir())
	if err := c.requireManagedRoot(""); err == nil {
		t.Error("empty path must be rejected")
	}
}

func TestRequireManagedRoot_NoRootsConfigured(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	mgr := &Manager{config: Config{TasksBasePath: ""}, logger: log}
	c := NewHandoffCleaner(mgr, log)
	if err := c.requireManagedRoot("/anywhere"); err == nil {
		t.Error("cleaner with no managed roots configured must reject every path")
	}
}

func TestCleanupPlainFolder_RemovesPathOnlyUnderRoot(t *testing.T) {
	tasksRoot := t.TempDir()
	target := filepath.Join(tasksRoot, "task-abc", "scratch")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	c := newCleanerWithTasksRoot(t, tasksRoot)

	if err := c.CleanupPlainFolder(context.Background(), target); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected target to be removed; stat err=%v", err)
	}
}

// REGRESSION: the managed-root guard MUST run before os.RemoveAll. A
// path outside the managed root never touches disk regardless of the
// caller's intent.
func TestCleanupPlainFolder_RejectsPathOutsideManagedRoot(t *testing.T) {
	tasksRoot := t.TempDir()
	outside := t.TempDir()
	canary := filepath.Join(outside, "do-not-touch.txt")
	if err := os.WriteFile(canary, []byte("preserve me"), 0o600); err != nil {
		t.Fatalf("write canary: %v", err)
	}

	c := newCleanerWithTasksRoot(t, tasksRoot)
	err := c.CleanupPlainFolder(context.Background(), outside)
	if err == nil {
		t.Fatal("cleanup must refuse paths outside the managed root")
	}
	if !strings.Contains(err.Error(), "managed-root guard") {
		t.Errorf("error should mention managed-root guard; got: %v", err)
	}
	// Canary survives.
	if _, err := os.Stat(canary); err != nil {
		t.Errorf("canary file was disturbed: %v", err)
	}
}

func TestCleanupPlainFolder_EmptyPath(t *testing.T) {
	c := newCleanerWithTasksRoot(t, t.TempDir())
	if err := c.CleanupPlainFolder(context.Background(), ""); err == nil {
		t.Error("empty path must be rejected")
	}
}

func TestCleanupSingleRepoWorktree_RequiresWorktreeID(t *testing.T) {
	c := newCleanerWithTasksRoot(t, t.TempDir())
	if err := c.CleanupSingleRepoWorktree(context.Background(), ""); err == nil {
		t.Error("empty worktree id must be rejected")
	}
}

func TestCleanupMultiRepoRoot_RejectsPathOutsideManagedRoot(t *testing.T) {
	c := newCleanerWithTasksRoot(t, t.TempDir())
	err := c.CleanupMultiRepoRoot(context.Background(), "/somewhere/else", []string{"wt-1"})
	if err == nil {
		t.Error("multi-repo root outside managed root must be rejected")
	}
}

func TestIsDescendant_HappyAndUnhappyPaths(t *testing.T) {
	cases := []struct {
		root, path string
		want       bool
	}{
		{"/a", "/a/b/c", true},
		{"/a", "/a", true},
		{"/a", "/b", false},
		{"/a/b", "/a", false},
		{"/a", "/a/../b", false}, // traversal
		{"", "/a", false},
		{"/a", "", false},
	}
	for _, tc := range cases {
		got := isDescendant(filepath.Clean(tc.root), filepath.Clean(tc.path))
		if got != tc.want {
			t.Errorf("isDescendant(%q, %q) = %v, want %v", tc.root, tc.path, got, tc.want)
		}
	}
}
