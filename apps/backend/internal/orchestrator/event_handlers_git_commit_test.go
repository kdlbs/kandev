package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// Regression: task_session_commits was written only by archive capture, which
// needed the agent process still running and fired at most once per task -
// 13 rows against 74 sessions in production. handleGitCommitCreated is now
// the primary write path: it must persist every observed commit, not merely
// forward it to the WebSocket.
func TestHandleGitCommitCreated_PersistsCommit(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-commit-live", "s-commit-live", "step1")

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())

	svc.handleGitCommitCreated(ctx, watcher.GitEventData{
		TaskID:    "t-commit-live",
		SessionID: "s-commit-live",
		Commit: &lifecycle.GitCommitData{
			CommitSHA:      "abc123",
			ParentSHA:      "def456",
			Message:        "feat: live capture",
			AuthorName:     "Kandev Agent",
			AuthorEmail:    "agent@kandev.local",
			FilesChanged:   2,
			Insertions:     10,
			Deletions:      1,
			CommittedAt:    "2026-04-02T03:04:05Z",
			RepositoryName: "myrepo",
		},
	})

	commits, err := testRepo.GetSessionCommits(ctx, "s-commit-live")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("commit rows = %d, want 1", len(commits))
	}
	got := commits[0]
	if got.CommitSHA != "abc123" {
		t.Errorf("CommitSHA = %q, want abc123", got.CommitSHA)
	}
	if got.Insertions != 10 || got.Deletions != 1 || got.FilesChanged != 2 {
		t.Errorf("stats = (%d, %d, %d), want (2, 10, 1)", got.FilesChanged, got.Insertions, got.Deletions)
	}
}

// Regression: the same commit can now be observed by more than one trigger
// (live event, per-turn sweep, archive). A second observation of the same
// (session_id, commit_sha) must not duplicate the row.
func TestHandleGitCommitCreated_DuplicateEventDoesNotDuplicateRow(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-commit-dup", "s-commit-dup", "step1")

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())

	event := watcher.GitEventData{
		TaskID:    "t-commit-dup",
		SessionID: "s-commit-dup",
		Commit: &lifecycle.GitCommitData{
			CommitSHA:   "same-sha",
			Message:     "feat: observed twice",
			CommittedAt: "2026-04-02T03:04:05Z",
		},
	}
	svc.handleGitCommitCreated(ctx, event)
	svc.handleGitCommitCreated(ctx, event)

	commits, err := testRepo.GetSessionCommits(ctx, "s-commit-dup")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("commit rows after duplicate event = %d, want 1", len(commits))
	}
}

// Regression: handleGitCommitCreated must not panic or block when
// data.Commit is nil (e.g. a malformed upstream event).
func TestHandleGitCommitCreated_NilCommitIsNoop(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-commit-nil", "s-commit-nil", "step1")

	svc := createTestService(testRepo, newMockStepGetter(), newMockTaskRepo())
	svc.handleGitCommitCreated(ctx, watcher.GitEventData{
		TaskID:    "t-commit-nil",
		SessionID: "s-commit-nil",
	})

	commits, err := testRepo.GetSessionCommits(ctx, "s-commit-nil")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("commit rows = %d, want 0", len(commits))
	}
}
