package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
// @covers AC-TASKS-RUNTIME-CLEANUP-001.9
func TestBranchExistsDistinguishesMissingRefFromInspectionFailure(t *testing.T) {
	mgr := newCaptureTestManager(t)
	repoPath := initGitRepoForWorktreeTest(t)

	ctx := context.Background()

	// 1. Existing branch returns (true, nil)
	exists, err := mgr.branchExists(ctx, repoPath, "refs/heads/main")
	if err != nil || !exists {
		t.Fatalf("branchExists(main) = (%v, %v), want (true, nil)", exists, err)
	}

	// 2. Missing branch in valid repo returns (false, nil)
	exists, err = mgr.branchExists(ctx, repoPath, "refs/heads/nonexistent")
	if err != nil || exists {
		t.Fatalf("branchExists(nonexistent) = (%v, %v), want (false, nil)", exists, err)
	}

	// 3. Invalid repository path returns (false, err)
	invalidRepo := filepath.Join(t.TempDir(), "not-a-repo")
	if err := os.MkdirAll(invalidRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	exists, err = mgr.branchExists(ctx, invalidRepo, "refs/heads/main")
	if err == nil || exists {
		t.Fatalf("branchExists(invalidRepo) = (%v, %v), want (false, error)", exists, err)
	}

	// 4. Missing directory returns (false, err)
	nonExistentDir := filepath.Join(t.TempDir(), "does-not-exist")
	exists, err = mgr.branchExists(ctx, nonExistentDir, "refs/heads/main")
	if err == nil || exists {
		t.Fatalf("branchExists(nonExistentDir) = (%v, %v), want (false, error)", exists, err)
	}

	// 5. Canceled context returns (false, context.Canceled)
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	exists, err = mgr.branchExists(canceledCtx, repoPath, "refs/heads/main")
	if !errors.Is(err, context.Canceled) || exists {
		t.Fatalf("branchExists(canceled) = (%v, %v), want (false, context.Canceled)", exists, err)
	}

	// 6. Expired deadline returns timeout error
	deadlineCtx, cancelDeadline := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancelDeadline()
	exists, err = mgr.branchExists(deadlineCtx, repoPath, "refs/heads/main")
	if err == nil || exists {
		t.Fatalf("branchExists(deadline) = (%v, %v), want (false, timeout error)", exists, err)
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
// @covers AC-TASKS-RUNTIME-CLEANUP-001.9
// @covers AC-TASKS-RUNTIME-CLEANUP-001.32
func TestCleanupInspectionFailurePreservesSnapshot(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-insp", "session-insp", "completed")
	wt := createReferenceCleanupWorktree(t, mgr, "task-insp", "session-insp")

	ctx := context.Background()

	// 1. Missing repository returns classified error at branch_lookup
	missingRepoWT := *wt
	missingRepoWT.RepositoryPath = filepath.Join(t.TempDir(), "nonexistent-repo")
	_, err := mgr.cleanupWorktreesWithReceipt(ctx, []*Worktree{&missingRepoWT}, false, WorktreeCleanupOptions{})
	if err == nil {
		t.Fatal("cleanup with missing repo succeeded, want error")
	}
	var inspErr *CleanupInspectionError
	if !errors.As(err, &inspErr) {
		t.Fatalf("error %v is not a CleanupInspectionError", err)
	}
	if inspErr.Stage != CleanupInspectionStageBranch {
		t.Fatalf("stage = %q, want %q", inspErr.Stage, CleanupInspectionStageBranch)
	}
	if inspErr.Reason != CleanupInspectionReasonRepoUnavailable {
		t.Fatalf("reason = %q, want %q", inspErr.Reason, CleanupInspectionReasonRepoUnavailable)
	}

	// 2. Changed commit returns commit_mismatch
	advancedWT := *wt
	advancedWT.CleanupHeadOID = "1111111111111111111111111111111111111111"
	_, err = mgr.cleanupWorktreesWithReceipt(ctx, []*Worktree{&advancedWT}, false, WorktreeCleanupOptions{})
	if err == nil {
		t.Fatal("cleanup with advanced commit succeeded, want error")
	}
	if !errors.As(err, &inspErr) {
		t.Fatalf("error %v is not a CleanupInspectionError", err)
	}
	if inspErr.Stage != CleanupInspectionStageCommit {
		t.Fatalf("stage = %q, want %q", inspErr.Stage, CleanupInspectionStageCommit)
	}
	if inspErr.Reason != CleanupInspectionReasonCommitMismatch {
		t.Fatalf("reason = %q, want %q", inspErr.Reason, CleanupInspectionReasonCommitMismatch)
	}
}

func TestClassifyGitInspectionReason_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantReason string
	}{
		{
			name: "missing executable path error",
			err: &os.PathError{
				Op:   "fork/exec",
				Path: "/missing/git",
				Err:  os.ErrNotExist,
			},
			wantReason: CleanupInspectionReasonGitStartFailed,
		},
		{
			name: "missing directory path error",
			err: &os.PathError{
				Op:   "stat",
				Path: "/missing/repo",
				Err:  os.ErrNotExist,
			},
			wantReason: CleanupInspectionReasonRepoUnavailable,
		},
		{
			name:       "canceled context",
			err:        context.Canceled,
			wantReason: CleanupInspectionReasonContextCanceled,
		},
		{
			name:       "deadline exceeded",
			err:        context.DeadlineExceeded,
			wantReason: CleanupInspectionReasonDeadlineExceeded,
		},
		{
			name:       "not a git repository error message",
			err:        errors.New("fatal: not a git repository (or any of the parent directories): .git"),
			wantReason: CleanupInspectionReasonRepoUnavailable,
		},
		{
			name:       "cannot change directory error message",
			err:        errors.New("fatal: cannot change to '/missing/path': No such file or directory"),
			wantReason: CleanupInspectionReasonRepoUnavailable,
		},
		{
			name:       "ordinary command failure",
			err:        errors.New("fatal: bad object HEAD"),
			wantReason: CleanupInspectionReasonCommandFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyGitInspectionReason(tc.err)
			if got != tc.wantReason {
				t.Fatalf("classifyGitInspectionReason(%v) = %q, want %q", tc.err, got, tc.wantReason)
			}
		})
	}
}
