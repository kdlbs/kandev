package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedGitSnapshotSession(t *testing.T, repo *Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + taskID, Name: "Workspace " + taskID}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + taskID, WorkspaceID: "ws-" + taskID, Name: "Workflow " + taskID}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-" + taskID, WorkflowID: "wf-" + taskID,
		WorkflowStepID: "step-" + taskID, Title: taskID, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

// TestGetGitSnapshotsBySession_OrdersByRecencyRegardlessOfTriggeredBy
// replaces a prior version of this test that asserted the opposite ordering
// (agent_completed always sorts first, regardless of actual recency) as
// correct. That ordering IS the session-delete pre-delete-warning defect
// documented in docs/specs/session-delete-resource-cleanup REVIEW-ROUND: 2:
// an agent turn completes (authoritative "agent_completed" snapshot with
// full file-diff data), then the user hand-edits a file outside the agent
// (a later, genuinely fresher "live_monitor" tick, which never populates
// Files by design — see UpsertLatestLiveGitSnapshot). A consumer that reads
// "the newest snapshot" — the session-delete warning's session.git.snapshots
// consumer — must see the newer row, not have a stale-but-"authoritative"
// row shadow it and silently undercount real uncommitted work to zero.
// Callers that need the newer row's file count when its Files column is
// empty fall back to its Metadata's modified/added/deleted/untracked/renamed
// lists (see apps/web/hooks/use-session-delete-warning.ts), which
// saveGitStatusSnapshot and UpsertLatestLiveGitSnapshot both always persist
// regardless of triggered_by — so recency-first ordering no longer trades
// away file-count accuracy the way it would have before that fallback
// existed.
func TestGetGitSnapshotsBySession_OrdersByRecencyRegardlessOfTriggeredBy(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-recency"
	const sessionID = "session-snapshot-recency"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	authoritativeAt := time.Now().UTC()
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "feature/recency", Ahead: 1,
		Files:       map[string]interface{}{"diff_update_test.txt": map[string]interface{}{"status": "modified"}},
		TriggeredBy: "agent_completed",
		CreatedAt:   authoritativeAt,
	}); err != nil {
		t.Fatalf("create authoritative snapshot: %v", err)
	}

	// A live_monitor row lands strictly AFTER the authoritative one — e.g. the
	// user hand-edited a file via the terminal after the turn completed. It
	// carries no Files data (UpsertLatestLiveGitSnapshot never populates it)
	// but is the genuinely newer state and must sort first.
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, Branch: "feature/recency", Ahead: 1,
		CreatedAt: authoritativeAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create newer live_monitor snapshot: %v", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 5)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapshots))
	}

	newest := snapshots[0]
	if newest.TriggeredBy != TriggeredByLiveMonitor {
		t.Fatalf("newest snapshot triggered_by = %q, want %q (the genuinely newer row must sort first)",
			newest.TriggeredBy, TriggeredByLiveMonitor)
	}
	oldest := snapshots[1]
	if oldest.TriggeredBy != "agent_completed" {
		t.Fatalf("oldest snapshot triggered_by = %q, want agent_completed", oldest.TriggeredBy)
	}
}

// TestGetGitSnapshotsBySession_FilesColumnDecodesToEmptyMapNotNil guards the
// serialization half of the same defect: a genuinely-empty files column
// (persisted as the literal "{}") must decode to a non-nil empty map, not Go
// nil — a nil map marshals to JSON `null`, and a WS consumer that expects
// "files is always an object" (e.g. Object.keys(files) on the frontend)
// would otherwise be surprised by null for a snapshot that legitimately has
// zero uncommitted files.
func TestGetGitSnapshotsBySession_FilesColumnDecodesToEmptyMapNotNil(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-empty-files"
	const sessionID = "session-snapshot-empty-files"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "main", Ahead: 0, Files: nil, TriggeredBy: "agent_completed",
	}); err != nil {
		t.Fatalf("create clean snapshot: %v", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 1)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(snapshots))
	}
	if snapshots[0].Files == nil {
		t.Fatal("Files decoded as Go nil (marshals to JSON null); want a non-nil empty map")
	}
	if len(snapshots[0].Files) != 0 {
		t.Fatalf("Files = %#v, want empty", snapshots[0].Files)
	}
}
