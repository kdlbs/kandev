package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestRemoveWorktreeDir_PruneFailureIsWarnLoggedNotSwallowed covers
// review-round-1 finding P2 (docs/specs/session-delete-resource-cleanup:
// "git worktree prune failure silently swallowed after forceRemoveDir
// fallback"). When `git worktree remove` fails, the forceRemoveDir fallback
// succeeds, but the subsequent `git worktree prune` also fails, the failure
// must stay discoverable rather than vanishing at Debug level. Per the
// spec's failure table ("git worktree remove fails | ... Only if both fail
// does the attempt fail") the directory having already been removed means
// the reclamation attempt itself still succeeds — but the stale
// `.git/worktrees/<id>` registration this leaves behind in the source repo
// must be logged at Warn, not Debug.
func TestRemoveWorktreeDir_PruneFailureIsWarnLoggedNotSwallowed(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger: %v", err)
	}

	mgr, err := NewManager(newTestConfig(t), newMockStore(), log)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// repoPath is a real directory but not a git repository, so both
	// `git worktree remove` and the later `git worktree prune` fail against
	// it — exercising the fallback branch's prune-failure logging without
	// needing a real worktree registration to go stale.
	repoPath := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "worktree")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create worktree dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("seed worktree file: %v", err)
	}

	if err := mgr.removeWorktreeDir(context.Background(), worktreePath, repoPath); err != nil {
		t.Fatalf("removeWorktreeDir should succeed once the fallback directory removal succeeds, got: %v", err)
	}

	if _, statErr := os.Stat(worktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree directory should be removed via the forced fallback, stat error = %v", statErr)
	}

	warnLogs := observed.FilterLevelExact(zapcore.WarnLevel).
		FilterMessage("git worktree prune failed after forced directory removal")
	if warnLogs.Len() != 1 {
		t.Fatalf("expected exactly one Warn log for the swallowed prune failure, got %d: %+v",
			warnLogs.Len(), observed.All())
	}

	debugPruneLogs := observed.FilterLevelExact(zapcore.DebugLevel).FilterMessage("git worktree prune failed")
	if debugPruneLogs.Len() != 0 {
		t.Fatalf("prune failure should no longer be logged at Debug, got %+v", debugPruneLogs.All())
	}
}
