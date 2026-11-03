package worktree

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func createConflictFixtureWorktree(t *testing.T, mgr *Manager, repoPath, taskID, sessionID string) *Worktree {
	t.Helper()
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         taskID,
		SessionID:      sessionID,
		TaskTitle:      taskID,
		RepositoryID:   "repository",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		IntegrationRef: "main",
		TaskDirName:    taskID,
		RepoName:       "repository",
	})
	if err != nil {
		t.Fatalf("create worktree %s: %v", taskID, err)
	}
	return wt
}

func cleanupInspectionErrors(err error) []*CleanupInspectionError {
	var inspections []*CleanupInspectionError
	stack := []error{err}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if inspection, ok := node.(*CleanupInspectionError); ok {
			inspections = append(inspections, inspection)
		}
		switch nested := node.(type) {
		case interface{ Unwrap() []error }:
			stack = append(stack, nested.Unwrap()...)
		case interface{ Unwrap() error }:
			if next := nested.Unwrap(); next != nil {
				stack = append(stack, next)
			}
		}
	}
	return inspections
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.4
// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.6
func TestCleanupConflictsExposeEveryAffectedResource(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	repoPath := initGitRepoWithRemote(t)
	ctx := context.Background()

	seedReferenceCleanupSession(t, store, "task-competing", "session-competing", models.TaskSessionStateCompleted)
	seedReferenceCleanupSession(t, store, "task-commit", "session-commit", models.TaskSessionStateCompleted)
	seedReferenceCleanupSession(t, store, "task-safe", "session-safe", models.TaskSessionStateCompleted)
	competing := createConflictFixtureWorktree(t, mgr, repoPath, "task-competing", "session-competing")
	commitMismatch := createConflictFixtureWorktree(t, mgr, repoPath, "task-commit", "session-commit")
	safe := createConflictFixtureWorktree(t, mgr, repoPath, "task-safe", "session-safe")

	// Model a target whose recorded branch is registered at another checkout.
	competing.Branch = commitMismatch.Branch
	competing.CleanupHeadOID = runGit(t, repoPath, "rev-parse", "refs/heads/"+commitMismatch.Branch)
	// Model a target whose immutable recorded commit no longer matches its ref.
	commitMismatch.CleanupHeadOID = "1111111111111111111111111111111111111111"

	err := mgr.CleanupWorktreesPreservingBranches(ctx, []*Worktree{competing, commitMismatch, safe})
	require.Error(t, err)

	inspections := cleanupInspectionErrors(err)
	require.Len(t, inspections, 2, "each conflicting resource must be reported: %v", err)
	byWorktree := make(map[string]*CleanupInspectionError, len(inspections))
	for _, inspection := range inspections {
		byWorktree[inspection.WorktreeID] = inspection
	}

	competingErr := byWorktree[competing.ID]
	require.NotNil(t, competingErr, "competing worktree %s missing from %v", competing.ID, err)
	require.Equal(t, CleanupInspectionReasonCompetingRegistration, competingErr.Reason)
	require.Equal(t, CleanupComparisonRecordedRegistration, competingErr.Comparison)
	require.Equal(t, CleanupInspectionActionInspectRegistration, competingErr.Action)

	commitErr := byWorktree[commitMismatch.ID]
	require.NotNil(t, commitErr, "commit worktree %s missing from %v", commitMismatch.ID, err)
	require.Equal(t, CleanupInspectionReasonCommitMismatch, commitErr.Reason)
	require.Equal(t, CleanupComparisonRecordedCommit, commitErr.Comparison)
	require.Equal(t, CleanupInspectionActionCompareCommit, commitErr.Action)

	if _, statErr := os.Stat(safe.Path); !os.IsNotExist(statErr) {
		t.Fatalf("safe sibling remains after batch cleanup: %v", statErr)
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.5
func TestCleanupInspectionMetadataPreservesWrappedErrors(t *testing.T) {
	root := errors.New("git refused")
	inspection := &CleanupInspectionError{
		Stage:      CleanupInspectionStageCommit,
		Reason:     CleanupInspectionReasonCommitMismatch,
		WorktreeID: "worktree-1",
		Comparison: CleanupComparisonRecordedCommitAbsent,
		Action:     CleanupInspectionActionInspectMissing,
		Err:        root,
	}
	wrapped := errors.Join(errors.New("sibling"), inspection)
	if !errors.Is(wrapped, root) {
		t.Fatal("wrapping lost the underlying inspection cause")
	}
	if !errors.Is(wrapped, inspection) {
		t.Fatal("wrapping lost the typed inspection error")
	}
	message := inspection.Error()
	require.Contains(t, message, "worktree-1")
	require.Contains(t, message, CleanupComparisonRecordedCommitAbsent)
	require.Contains(t, message, CleanupInspectionActionInspectMissing)
	require.NotContains(t, message, root.Error())
}
