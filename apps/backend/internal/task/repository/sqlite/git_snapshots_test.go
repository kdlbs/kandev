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

// TestGetGitSnapshotsBySession_PrefersAuthoritativeOverRacingLiveMonitor
// reproduces the session-delete pre-delete-warning defect found in QA
// (docs/specs/session-delete-resource-cleanup): saveGitStatusSnapshot writes
// an authoritative "agent_completed" snapshot with full file-diff data on
// turn completion, but a periodic live-git-status-monitor tick can land
// microseconds-to-seconds later with a newer created_at and an always-empty
// files column (UpsertLatestLiveGitSnapshot never populates Files). A caller
// that reads "the newest snapshot" — like the session-delete warning's
// session.git.snapshots consumer — must not have that newer, less complete
// row shadow the authoritative one.
func TestGetGitSnapshotsBySession_PrefersAuthoritativeOverRacingLiveMonitor(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	const taskID = "task-snapshot-race"
	const sessionID = "session-snapshot-race"
	seedGitSnapshotSession(t, repo, taskID, sessionID)

	authoritativeAt := time.Now().UTC()
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch: "feature/race", Ahead: 1,
		Files:       map[string]interface{}{"diff_update_test.txt": map[string]interface{}{"status": "modified"}},
		TriggeredBy: "agent_completed",
		CreatedAt:   authoritativeAt,
	}); err != nil {
		t.Fatalf("create authoritative snapshot: %v", err)
	}

	// The live_monitor row lands AFTER the authoritative one (later
	// created_at) and — as UpsertLatestLiveGitSnapshot always does — carries
	// no file data. This is the exact race observed in QA.
	if err := repo.UpsertLatestLiveGitSnapshot(ctx, &models.GitSnapshot{
		SessionID: sessionID, Branch: "feature/race", Ahead: 1,
		CreatedAt: authoritativeAt.Add(time.Millisecond),
	}); err != nil {
		t.Fatalf("create racing live_monitor snapshot: %v", err)
	}

	snapshots, err := repo.GetGitSnapshotsBySession(ctx, sessionID, 5)
	if err != nil {
		t.Fatalf("GetGitSnapshotsBySession: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapshots))
	}

	newest := snapshots[0]
	if newest.TriggeredBy != "agent_completed" {
		t.Fatalf("newest snapshot triggered_by = %q, want agent_completed (live_monitor race must not shadow it)",
			newest.TriggeredBy)
	}
	if len(newest.Files) != 1 {
		t.Fatalf("newest snapshot files = %#v, want the authoritative diff, not the empty live_monitor row", newest.Files)
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
