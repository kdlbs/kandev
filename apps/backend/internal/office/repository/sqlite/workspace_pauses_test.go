package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func newPauseTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	return newSearchTestRepo(t)
}

func mustCreatePause(t *testing.T, repo *sqlite.Repository, workspaceID string) *models.WorkspacePause {
	t.Helper()
	pause := &models.WorkspacePause{
		WorkspaceID:   workspaceID,
		Reason:        "incident",
		CreatedBy:     "user-1",
		CreatedByKind: "user",
	}
	activity := &models.ActivityEntry{
		WorkspaceID: workspaceID,
		ActorType:   models.ActivityActorUser,
		ActorID:     "user-1",
		Action:      models.ActivityActionWorkspacePaused,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    workspaceID,
		Details:     "incident",
	}
	if err := repo.CreateWorkspacePauseWithActivity(context.Background(), pause, activity); err != nil {
		t.Fatalf("create pause: %v", err)
	}
	return pause
}

// TestGetActiveWorkspacePause_NoRow proves AC-001.1: a workspace with no
// unreleased pause record reads as running (nil, nil), not an error.
func TestGetActiveWorkspacePause_NoRow(t *testing.T) {
	repo := newPauseTestRepo(t)
	pause, err := repo.GetActiveWorkspacePause(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("get active pause: %v", err)
	}
	if pause != nil {
		t.Fatalf("expected no active pause, got %+v", pause)
	}
}

// TestCreateWorkspacePauseWithActivity_CreatesExactlyOneRecord proves
// AC-001.2: a pause request on a running workspace creates exactly one
// pause record plus its paired activity log entry, atomically.
func TestCreateWorkspacePauseWithActivity_CreatesExactlyOneRecord(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	mustCreatePause(t, repo, "ws-1")

	active, err := repo.GetActiveWorkspacePause(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get active pause: %v", err)
	}
	if active == nil {
		t.Fatal("expected an active pause after create")
	}
	if active.Reason != "incident" || active.CreatedBy != "user-1" || active.CreatedByKind != "user" {
		t.Fatalf("pause provenance mismatch: %+v", active)
	}
	if active.ReleasedAt != nil {
		t.Fatalf("expected ReleasedAt nil, got %v", active.ReleasedAt)
	}

	entries, err := repo.ListActivityEntries(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("activity entry count = %d, want 1", len(entries))
	}
	if entries[0].Action != models.ActivityActionWorkspacePaused {
		t.Fatalf("activity action = %q, want workspace_paused", entries[0].Action)
	}
}

// TestCreateWorkspacePauseWithActivity_RejectsSecondPause proves the
// partial unique index enforces at most one unreleased pause per
// workspace: a second pause request while one is active is rejected with
// ErrWorkspaceAlreadyPaused and writes no additional pause row or activity
// entry (the whole transaction rolls back).
func TestCreateWorkspacePauseWithActivity_RejectsSecondPause(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	mustCreatePause(t, repo, "ws-1")

	second := &models.WorkspacePause{
		WorkspaceID:   "ws-1",
		Reason:        "second attempt",
		CreatedBy:     "user-2",
		CreatedByKind: "user",
	}
	activity := &models.ActivityEntry{
		WorkspaceID: "ws-1",
		ActorType:   models.ActivityActorUser,
		ActorID:     "user-2",
		Action:      models.ActivityActionWorkspacePaused,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    "ws-1",
	}
	err := repo.CreateWorkspacePauseWithActivity(ctx, second, activity)
	if !errors.Is(err, sqlite.ErrWorkspaceAlreadyPaused) {
		t.Fatalf("expected ErrWorkspaceAlreadyPaused, got %v", err)
	}

	entries, err := repo.ListActivityEntries(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("activity entry count = %d, want 1 (rejected attempt writes nothing)", len(entries))
	}
}

// TestCreateWorkspacePauseWithActivity_ActivityWriteFailureRollsBackPauseInsert
// proves the two writes commit or fail together: when the paired
// activity-log insert fails (here, by dropping its table so the second
// statement in the transaction errors while the first would otherwise
// succeed), the whole transaction rolls back and the workspace is left
// exactly as it was, not paused with an audit gap.
func TestCreateWorkspacePauseWithActivity_ActivityWriteFailureRollsBackPauseInsert(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	mustExec(t, repo, `DROP TABLE office_activity_log`)

	pause := &models.WorkspacePause{
		WorkspaceID:   "ws-1",
		Reason:        "incident",
		CreatedBy:     "user-1",
		CreatedByKind: "user",
	}
	activity := &models.ActivityEntry{
		WorkspaceID: "ws-1",
		ActorType:   models.ActivityActorUser,
		ActorID:     "user-1",
		Action:      models.ActivityActionWorkspacePaused,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    "ws-1",
	}
	err := repo.CreateWorkspacePauseWithActivity(ctx, pause, activity)
	if err == nil {
		t.Fatal("expected an error when the paired activity-log write fails")
	}
	if errors.Is(err, sqlite.ErrWorkspaceAlreadyPaused) {
		t.Fatalf("expected a plain write failure, not ErrWorkspaceAlreadyPaused: %v", err)
	}

	active, getErr := repo.GetActiveWorkspacePause(ctx, "ws-1")
	if getErr != nil {
		t.Fatalf("get active pause: %v", getErr)
	}
	if active != nil {
		t.Fatalf("expected no pause row: the insert must roll back with its failed paired activity write, got %+v", active)
	}
}

// TestReleaseWorkspacePauseWithActivity_ActivityWriteFailureRollsBackRelease
// proves the release side of the same guarantee (AC-006.10): when the
// paired activity-log insert fails, the CAS release itself must roll
// back too, leaving the workspace paused rather than silently resumed
// with no audit trail.
func TestReleaseWorkspacePauseWithActivity_ActivityWriteFailureRollsBackRelease(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	pause := mustCreatePause(t, repo, "ws-1")
	mustExec(t, repo, `DROP TABLE office_activity_log`)

	released, err := repo.ReleaseWorkspacePauseWithActivity(ctx, pause.ID, "ws-1", "user-1", "user", "resolved")
	if err == nil {
		t.Fatal("expected an error when the paired activity-log write fails")
	}
	if released {
		t.Fatal("expected released=false when the transaction fails")
	}

	active, getErr := repo.GetActiveWorkspacePause(ctx, "ws-1")
	if getErr != nil {
		t.Fatalf("get active pause: %v", getErr)
	}
	if active == nil {
		t.Fatal("expected the pause to remain active: a failed release must leave the workspace paused, not resumed")
	}
	if active.ID != pause.ID {
		t.Fatalf("active pause id = %q, want unchanged %q", active.ID, pause.ID)
	}
}

// TestReleaseWorkspacePauseWithActivity_ReleasesAndLogsResumed proves the
// ordinary resume path: the CAS wins, the record is released, and a
// workspace_resumed activity entry is written in the same transaction.
func TestReleaseWorkspacePauseWithActivity_ReleasesAndLogsResumed(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	pause := mustCreatePause(t, repo, "ws-1")

	released, err := repo.ReleaseWorkspacePauseWithActivity(ctx, pause.ID, "ws-1", "user-1", "user", "resolved")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released {
		t.Fatal("expected release to report true")
	}

	active, err := repo.GetActiveWorkspacePause(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get active pause: %v", err)
	}
	if active != nil {
		t.Fatalf("expected no active pause after release, got %+v", active)
	}

	entries, err := repo.ListActivityEntries(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("activity entry count = %d, want 2 (pause + resume)", len(entries))
	}
	if entries[0].Action != models.ActivityActionWorkspaceResumed {
		t.Fatalf("latest activity action = %q, want workspace_resumed", entries[0].Action)
	}
}

// TestReleaseWorkspacePauseWithActivity_LosingRaceLogsNoop proves AC-005.8:
// a second release of an already-released pause id loses the CAS, reports
// false, and still writes an auditable workspace_pause_noop entry rather
// than silently doing nothing.
func TestReleaseWorkspacePauseWithActivity_LosingRaceLogsNoop(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	pause := mustCreatePause(t, repo, "ws-1")

	released, err := repo.ReleaseWorkspacePauseWithActivity(ctx, pause.ID, "ws-1", "user-1", "user", "first")
	if err != nil || !released {
		t.Fatalf("first release: released=%v err=%v", released, err)
	}

	released, err = repo.ReleaseWorkspacePauseWithActivity(ctx, pause.ID, "ws-1", "user-2", "user", "second")
	if err != nil {
		t.Fatalf("second release: %v", err)
	}
	if released {
		t.Fatal("expected second release to lose the CAS and report false")
	}

	entries, err := repo.ListActivityEntries(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("activity entry count = %d, want 3 (pause + resume + noop)", len(entries))
	}
	if entries[0].Action != models.ActivityActionWorkspacePauseNoop {
		t.Fatalf("latest activity action = %q, want workspace_pause_noop", entries[0].Action)
	}
}

// TestListInflightRunsForWorkspace_ReturnsQueuedAndClaimed proves the halt
// sweep's run discovery: queued/claimed runs for the workspace's agents are
// returned with their payload task id, taskless runs report an empty task
// id, and terminal runs are excluded.
func TestListInflightRunsForWorkspace_ReturnsQueuedAndClaimed(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	mustExec(t, repo, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES ('agent-1', 'a', 'Agent', 'Agent', 'ws-1', 'engineer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		       ('agent-2', 'a', 'Agent', 'Agent', 'ws-2', 'engineer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	mustExec(t, repo, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, coalesced_count, context_snapshot, requested_at)
		VALUES
			('run-queued', 'agent-1', 'task_assigned', '{"task_id":"task-1"}', 'queued', 1, '{}', CURRENT_TIMESTAMP),
			('run-claimed', 'agent-1', 'task_assigned', '{"task_id":"task-2"}', 'claimed', 1, '{}', CURRENT_TIMESTAMP),
			('run-taskless', 'agent-1', 'wakeup', '{}', 'queued', 1, '{}', CURRENT_TIMESTAMP),
			('run-finished', 'agent-1', 'task_assigned', '{"task_id":"task-3"}', 'finished', 1, '{}', CURRENT_TIMESTAMP),
			('run-other-ws', 'agent-2', 'task_assigned', '{"task_id":"task-4"}', 'queued', 1, '{}', CURRENT_TIMESTAMP)
	`)

	runs, err := repo.ListInflightRunsForWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list inflight runs: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("inflight run count = %d, want 3: %+v", len(runs), runs)
	}
	byID := map[string]string{}
	for _, run := range runs {
		byID[run.RunID] = run.TaskID
	}
	if byID["run-queued"] != "task-1" || byID["run-claimed"] != "task-2" || byID["run-taskless"] != "" {
		t.Fatalf("unexpected run task ids: %+v", byID)
	}
}

// TestCancelRunsForWorkspace_CancelsGivenRuns proves the halt sweep's run
// cancellation only touches the run ids it was given and reports how many
// actually transitioned.
func TestCancelRunsForWorkspace_CancelsGivenRuns(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	mustExec(t, repo, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES ('agent-1', 'a', 'Agent', 'Agent', 'ws-1', 'engineer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	mustExec(t, repo, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, coalesced_count, context_snapshot, requested_at)
		VALUES ('run-a', 'agent-1', 'task_assigned', '{}', 'queued', 1, '{}', CURRENT_TIMESTAMP),
		       ('run-b', 'agent-1', 'task_assigned', '{}', 'claimed', 1, '{}', CURRENT_TIMESTAMP)
	`)

	cancelled, err := repo.CancelRunsForWorkspace(ctx, []string{"run-a", "run-b"}, "workspace_paused")
	if err != nil {
		t.Fatalf("cancel runs: %v", err)
	}
	if cancelled != 2 {
		t.Fatalf("cancelled count = %d, want 2", cancelled)
	}
}

// TestReleaseCheckoutsForWorkspace_ClearsCheckoutColumns proves the halt
// sweep releases task checkouts held by the runs it cancelled.
func TestReleaseCheckoutsForWorkspace_ClearsCheckoutColumns(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "task-1", "ws-1", "Task", "", "")
	mustExec(t, repo, `
		UPDATE tasks SET checkout_agent_id = 'agent-1', checkout_run_id = 'run-a', checkout_at = CURRENT_TIMESTAMP
		WHERE id = 'task-1'
	`)

	if err := repo.ReleaseCheckoutsForWorkspace(ctx, []string{"run-a"}); err != nil {
		t.Fatalf("release checkouts: %v", err)
	}

	var checkoutAgent string
	if err := repo.ReaderDB().Get(&checkoutAgent, `SELECT COALESCE(checkout_agent_id, '') FROM tasks WHERE id = 'task-1'`); err != nil {
		t.Fatalf("read checkout: %v", err)
	}
	if checkoutAgent != "" {
		t.Fatalf("checkout_agent_id = %q, want cleared", checkoutAgent)
	}
}

// TestCreatePauseSkippedRoutineRun_DedupsPerPause proves the partial
// unique index idx_office_routine_run_pause_once: the first blocked fire
// under a pause records a skipped run, and every subsequent fire under
// the SAME pause is a no-op (AC-002.13) rather than a duplicate row.
func TestCreatePauseSkippedRoutineRun_DedupsPerPause(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	mustExec(t, repo, `
		INSERT INTO office_routines (id, workspace_id, name, task_template, created_at, updated_at)
		VALUES ('routine-1', 'ws-1', 'Nightly', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)

	created, err := repo.CreatePauseSkippedRoutineRun(ctx, "routine-1", "", "cron", "pause-1")
	if err != nil {
		t.Fatalf("first skip: %v", err)
	}
	if !created {
		t.Fatal("expected first skip to create a row")
	}

	created, err = repo.CreatePauseSkippedRoutineRun(ctx, "routine-1", "", "cron", "pause-1")
	if err != nil {
		t.Fatalf("second skip: %v", err)
	}
	if created {
		t.Fatal("expected second skip under the same pause to be a no-op")
	}

	var count int
	if err := repo.ReaderDB().Get(&count, `SELECT COUNT(*) FROM office_routine_runs WHERE routine_id = 'routine-1'`); err != nil {
		t.Fatalf("count routine runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("routine run row count = %d, want 1", count)
	}
}

// TestListLiveRoutineTaskIDsForWorkspace_ExcludesTerminal proves the halt
// sweep's second task discovery source only surfaces heavy-routine tasks
// still in flight, distinguishing them from the Office-run-derived source.
func TestListLiveRoutineTaskIDsForWorkspace_ExcludesTerminal(t *testing.T) {
	repo := newPauseTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "task-live", "ws-1", "Live", "", "")
	insertTask(t, repo, ctx, "task-done", "ws-1", "Done", "", "")
	mustExec(t, repo, `UPDATE tasks SET state = 'COMPLETED' WHERE id = 'task-done'`)

	mustExec(t, repo, `
		INSERT INTO office_routines (id, workspace_id, name, task_template, created_at, updated_at)
		VALUES ('routine-1', 'ws-1', 'Heavy', '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	mustExec(t, repo, `
		INSERT INTO office_routine_runs (id, routine_id, source, status, linked_task_id, created_at)
		VALUES ('rr-1', 'routine-1', 'cron', 'task_created', 'task-live', CURRENT_TIMESTAMP),
		       ('rr-2', 'routine-1', 'cron', 'task_created', 'task-done', CURRENT_TIMESTAMP)
	`)

	ids, err := repo.ListLiveRoutineTaskIDsForWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list live routine task ids: %v", err)
	}
	if len(ids) != 1 || ids[0] != "task-live" {
		t.Fatalf("live task ids = %v, want [task-live]", ids)
	}
}
