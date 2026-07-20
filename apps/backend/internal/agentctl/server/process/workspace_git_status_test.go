package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestUnquoteGitPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain path", "simple-path.txt", "simple-path.txt"},
		{"path with spaces", `"path with spaces/file.md"`, "path with spaces/file.md"},
		{"path with tab", `"path\twith\ttab"`, "path\twith\ttab"},
		{"path with backslash", `"path\\backslash"`, `path\backslash`},
		{"path with quotes", `"path \"quoted\""`, `path "quoted"`},
		{"empty quotes", `""`, ""},
		{"single char not quoted", "a", "a"},
		{"mismatched quote", `"not closed`, `"not closed`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unquoteGitPath(tt.in)
			if got != tt.want {
				t.Errorf("unquoteGitPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestApplyPorcelainOutput_CancellationStopsLargeLoop(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx := &cancelAfterErrChecksContext{Context: baseCtx, remaining: 2, cancel: cancel}
	wt := NewWorkspaceTracker(t.TempDir(), newTestLogger(t))
	update := &types.GitStatusUpdate{Files: make(map[string]types.FileInfo)}

	err := wt.applyPorcelainOutput(ctx, []byte("?? first.txt\n?? second.txt\n?? third.txt\n"), update)

	if err != context.Canceled {
		t.Fatalf("applyPorcelainOutput() error = %v, want %v", err, context.Canceled)
	}
	if _, ok := update.Files["first.txt"]; !ok {
		t.Fatal("first entry was not parsed before cancellation")
	}
	if _, ok := update.Files["second.txt"]; ok {
		t.Fatal("entry parsed after cancellation")
	}
}

func TestGetGitStatus_PathsWithSpaces(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Create a directory and file with spaces in the path.
	dir := filepath.Join(repoDir, "path with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	writeFile(t, dir, "file.md", "initial content")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "Add file with spaces in path")

	// Modify the file to create an unstaged change.
	writeFile(t, dir, "file.md", "modified content")

	status, err := wt.getGitStatus(ctx)
	if err != nil {
		t.Fatalf("failed to get git status: %v", err)
	}

	expectedPath := "path with spaces/file.md"

	// The file should appear in Modified with an unquoted path.
	found := false
	for _, p := range status.Modified {
		if p == expectedPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in Modified list, got %v", expectedPath, status.Modified)
	}

	// The Files map key should be the unquoted path.
	fileInfo, exists := status.Files[expectedPath]
	if !exists {
		t.Fatalf("expected Files map to contain key %q, got keys: %v",
			expectedPath, mapKeys(status.Files))
	}
	if fileInfo.Status != "modified" {
		t.Errorf("expected status=modified, got %q", fileInfo.Status)
	}

	// Diff content should be populated (enrichWithDiffData uses the same key).
	if fileInfo.Diff == "" {
		t.Error("expected non-empty Diff for file with spaces in path")
	}
}

func TestGetGitStatus_UntrackedFileWithSpaces(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Create an untracked file with spaces in the path.
	dir := filepath.Join(repoDir, "dir with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	writeFile(t, dir, "new file.txt", "hello world")

	status, err := wt.getGitStatus(ctx)
	if err != nil {
		t.Fatalf("failed to get git status: %v", err)
	}

	expectedPath := "dir with spaces/new file.txt"

	found := false
	for _, p := range status.Untracked {
		if p == expectedPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in Untracked list, got %v", expectedPath, status.Untracked)
	}

	fileInfo, exists := status.Files[expectedPath]
	if !exists {
		t.Fatalf("expected Files map to contain key %q, got keys: %v",
			expectedPath, mapKeys(status.Files))
	}
	if fileInfo.Diff == "" {
		t.Error("expected non-empty synthetic Diff for untracked file with spaces")
	}
}

// TestGetGitStatus_FreshBypassesStaleCache simulates the bug class where the
// poll loop missed a HEAD change (paused mode, dropped tick) and left
// currentStatus.Files holding pre-commit entries. fresh=true must re-run
// `git status --porcelain` and return ground truth, not the cached snapshot.
func TestGetGitStatus_FreshBypassesStaleCache(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	// Commit a file so we have something to modify.
	writeFile(t, repoDir, "tracked.txt", "v1")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add tracked")

	// Dirty the worktree and prime the cache by running an update.
	writeFile(t, repoDir, "tracked.txt", "v2")
	wt.updateGitStatus(ctx)

	cached, err := wt.GetGitStatus(ctx, false)
	if err != nil {
		t.Fatalf("priming GetGitStatus failed: %v", err)
	}
	if _, ok := cached.Files["tracked.txt"]; !ok {
		t.Fatalf("expected priming run to cache tracked.txt as modified; got Files=%v", mapKeys(cached.Files))
	}

	// Simulate the bug: commit the file but DO NOT refresh the tracker (this
	// is what happens when the poll loop is paused or drops a tick at the
	// exact moment HEAD moves).
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "commit v2")

	// Precondition: cache must still hold the pre-commit entry — confirms the
	// stale-cache scenario the fresh path is supposed to bypass.
	stale, err := wt.GetGitStatus(ctx, false)
	if err != nil {
		t.Fatalf("stale GetGitStatus failed: %v", err)
	}
	if _, ok := stale.Files["tracked.txt"]; !ok {
		t.Fatalf("expected cache to still hold pre-commit entry (test setup invalid); got Files=%v", mapKeys(stale.Files))
	}

	// fresh=true must bypass the cache and produce a clean status.
	fresh, err := wt.GetGitStatus(ctx, true)
	if err != nil {
		t.Fatalf("fresh GetGitStatus failed: %v", err)
	}
	if len(fresh.Files) != 0 {
		t.Errorf("fresh=true should reflect the clean worktree; got Files=%v", mapKeys(fresh.Files))
	}
	if len(fresh.Modified) != 0 {
		t.Errorf("fresh=true should produce empty Modified; got %v", fresh.Modified)
	}

	// Contract: a fresh read MUST NOT mutate the shared cache. The poll loop
	// owns currentStatus; subscribe-time fresh reads short-circuit it without
	// writing back, so already-subscribed observers still see the cached
	// stream until the poll loop catches up.
	afterFresh, err := wt.GetGitStatus(ctx, false)
	if err != nil {
		t.Fatalf("post-fresh GetGitStatus failed: %v", err)
	}
	if _, ok := afterFresh.Files["tracked.txt"]; !ok {
		t.Errorf("fresh=true must not overwrite the cache; expected stale entry to remain, got Files=%v", mapKeys(afterFresh.Files))
	}
}

// mapKeys returns the keys of a map for diagnostic output.
func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestCarryAheadBehind covers the carry-forward fallback used when the
// ahead/behind git command fails (timeout, missing upstream). The contract:
// preserve prior counts when HEAD is unchanged, leave them zero when HEAD
// moved (the prior counts are stale by definition) or when prior is empty.
func TestCarryAheadBehind(t *testing.T) {
	head := "abc123"
	tests := []struct {
		name       string
		prior      types.GitStatusUpdate
		updateHead string
		wantAhead  int
		wantBehind int
	}{
		{
			name:       "same head preserves counts",
			prior:      types.GitStatusUpdate{HeadCommit: head, Ahead: 1, Behind: 3},
			updateHead: head,
			wantAhead:  1,
			wantBehind: 3,
		},
		{
			name:       "different head drops counts",
			prior:      types.GitStatusUpdate{HeadCommit: head, Ahead: 1, Behind: 3},
			updateHead: "def456",
			wantAhead:  0,
			wantBehind: 0,
		},
		{
			name:       "empty prior head no-op",
			prior:      types.GitStatusUpdate{Ahead: 9, Behind: 9},
			updateHead: head,
			wantAhead:  0,
			wantBehind: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := &types.GitStatusUpdate{HeadCommit: tt.updateHead}
			carryAheadBehind(update, tt.prior)
			if update.Ahead != tt.wantAhead || update.Behind != tt.wantBehind {
				t.Errorf("ahead/behind = %d/%d, want %d/%d", update.Ahead, update.Behind, tt.wantAhead, tt.wantBehind)
			}
		})
	}
}
