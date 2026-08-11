package sqlite

// Coverage for the archive/unarchive CAS surface of task.go. The cascade
// stamp is the interesting part: ArchiveTaskIfActive only fires on an active
// row, UnarchiveTaskByCascade only restores rows its own cascade archived,
// and UnarchiveTask only restores rows no cascade owns.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const archiveWorkspaceID = "ws-archive"

func newRepoForArchiveTests(t *testing.T, taskIDList ...string) *Repository {
	t.Helper()
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, archiveWorkspaceID)
	for _, id := range taskIDList {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: id, WorkspaceID: archiveWorkspaceID, Title: id,
		}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	return repo
}

// archivedCascadeID reads the raw stamp column so the assertions do not
// depend on whichever SELECT projection happens to include it.
func archivedCascadeID(t *testing.T, repo *Repository, taskID string) string {
	t.Helper()
	var cascade string
	if err := repo.db.QueryRowContext(context.Background(), repo.db.Rebind(
		`SELECT COALESCE(archived_by_cascade_id, '') FROM tasks WHERE id = ?`), taskID).Scan(&cascade); err != nil {
		t.Fatalf("read archived_by_cascade_id for %s: %v", taskID, err)
	}
	return cascade
}

func TestArchiveTaskIfActiveOnlyFiresOnActiveRows(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-arch-cas")
	ctx := context.Background()

	changed, err := repo.ArchiveTaskIfActive(ctx, "task-arch-cas", "cascade-1")
	if err != nil {
		t.Fatalf("ArchiveTaskIfActive: %v", err)
	}
	if !changed {
		t.Fatal("ArchiveTaskIfActive = false on an active row, want true")
	}
	got, err := repo.GetTask(ctx, "task-arch-cas")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ArchivedAt == nil {
		t.Fatal("ArchivedAt = nil after archiving")
	}
	firstArchivedAt := *got.ArchivedAt
	if cascade := archivedCascadeID(t, repo, "task-arch-cas"); cascade != "cascade-1" {
		t.Errorf("archived_by_cascade_id = %q, want cascade-1", cascade)
	}

	// A second cascade must not re-stamp an already-archived row.
	changed, err = repo.ArchiveTaskIfActive(ctx, "task-arch-cas", "cascade-2")
	if err != nil {
		t.Fatalf("second ArchiveTaskIfActive: %v", err)
	}
	if changed {
		t.Error("ArchiveTaskIfActive = true on an already-archived row, want false")
	}
	if cascade := archivedCascadeID(t, repo, "task-arch-cas"); cascade != "cascade-1" {
		t.Errorf("archived_by_cascade_id = %q, want cascade-1 preserved", cascade)
	}
	got, err = repo.GetTask(ctx, "task-arch-cas")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if !got.ArchivedAt.Equal(firstArchivedAt) {
		t.Errorf("ArchivedAt = %v, want the first value %v preserved", *got.ArchivedAt, firstArchivedAt)
	}

	// An unknown id is reported as "not changed", not as an error.
	changed, err = repo.ArchiveTaskIfActive(ctx, "task-arch-nowhere", "cascade-3")
	if err != nil {
		t.Fatalf("ArchiveTaskIfActive(unknown): %v", err)
	}
	if changed {
		t.Error("ArchiveTaskIfActive(unknown) = true, want false")
	}
}

func TestArchiveTaskIfActiveWithoutCascadeIDLeavesStampEmpty(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-arch-nocascade")
	ctx := context.Background()

	changed, err := repo.ArchiveTaskIfActive(ctx, "task-arch-nocascade", "")
	if err != nil {
		t.Fatalf("ArchiveTaskIfActive: %v", err)
	}
	if !changed {
		t.Fatal("ArchiveTaskIfActive = false, want true")
	}
	if cascade := archivedCascadeID(t, repo, "task-arch-nocascade"); cascade != "" {
		t.Errorf("archived_by_cascade_id = %q, want empty for a manual archive", cascade)
	}
}

func TestUnarchiveTaskByCascadeRestoresOnlyItsOwnRows(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-owned", "task-other-cascade", "task-manual")
	ctx := context.Background()

	for id, cascade := range map[string]string{
		"task-owned":         "cascade-a",
		"task-other-cascade": "cascade-b",
		"task-manual":        "",
	} {
		if _, err := repo.ArchiveTaskIfActive(ctx, id, cascade); err != nil {
			t.Fatalf("ArchiveTaskIfActive(%s): %v", id, err)
		}
	}

	restored, err := repo.UnarchiveTaskByCascade(ctx, "task-owned", "cascade-a")
	if err != nil {
		t.Fatalf("UnarchiveTaskByCascade: %v", err)
	}
	if !restored {
		t.Fatal("UnarchiveTaskByCascade = false on its own row, want true")
	}
	owned, err := repo.GetTask(ctx, "task-owned")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if owned.ArchivedAt != nil {
		t.Errorf("ArchivedAt = %v, want nil after restoration", *owned.ArchivedAt)
	}
	if cascade := archivedCascadeID(t, repo, "task-owned"); cascade != "" {
		t.Errorf("archived_by_cascade_id = %q, want it cleared", cascade)
	}

	// A row another cascade owns, and a manually archived row, both survive.
	for _, id := range []string{"task-other-cascade", "task-manual"} {
		restored, err = repo.UnarchiveTaskByCascade(ctx, id, "cascade-a")
		if err != nil {
			t.Fatalf("UnarchiveTaskByCascade(%s): %v", id, err)
		}
		if restored {
			t.Errorf("UnarchiveTaskByCascade(%s) = true, want false — cascade-a does not own it", id)
		}
		still, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if still.ArchivedAt == nil {
			t.Errorf("%s ArchivedAt = nil, want it still archived", id)
		}
	}

	if _, err := repo.UnarchiveTaskByCascade(ctx, "task-owned", ""); err == nil {
		t.Error("UnarchiveTaskByCascade with an empty cascade id returned nil error")
	}
}

func TestUnarchiveTaskRestoresOnlyUnstampedArchivedRows(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-manual-arch", "task-cascade-arch", "task-active")
	ctx := context.Background()

	if _, err := repo.ArchiveTaskIfActive(ctx, "task-manual-arch", ""); err != nil {
		t.Fatalf("ArchiveTaskIfActive(manual): %v", err)
	}
	if _, err := repo.ArchiveTaskIfActive(ctx, "task-cascade-arch", "cascade-x"); err != nil {
		t.Fatalf("ArchiveTaskIfActive(cascade): %v", err)
	}

	restored, err := repo.UnarchiveTask(ctx, "task-manual-arch")
	if err != nil {
		t.Fatalf("UnarchiveTask: %v", err)
	}
	if !restored {
		t.Fatal("UnarchiveTask = false on a manually archived row, want true")
	}
	manual, err := repo.GetTask(ctx, "task-manual-arch")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if manual.ArchivedAt != nil {
		t.Errorf("ArchivedAt = %v, want nil", *manual.ArchivedAt)
	}

	// A cascade-stamped row is off limits to the manual path.
	restored, err = repo.UnarchiveTask(ctx, "task-cascade-arch")
	if err != nil {
		t.Fatalf("UnarchiveTask(cascade-stamped): %v", err)
	}
	if restored {
		t.Error("UnarchiveTask = true on a cascade-stamped row, want false")
	}
	stamped, err := repo.GetTask(ctx, "task-cascade-arch")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stamped.ArchivedAt == nil {
		t.Error("cascade-stamped ArchivedAt = nil, want it still archived")
	}

	// An already-active row reports no change.
	restored, err = repo.UnarchiveTask(ctx, "task-active")
	if err != nil {
		t.Fatalf("UnarchiveTask(active): %v", err)
	}
	if restored {
		t.Error("UnarchiveTask = true on an already-active row, want false")
	}
}

func TestArchiveTaskReportsMissingRowWithSentinel(t *testing.T) {
	repo := newRepoForArchiveTests(t)

	err := repo.ArchiveTask(context.Background(), "task-arch-absent")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("ArchiveTask(missing) error = %v, want ErrTaskNotFound", err)
	}
}

func TestListTasksForAutoArchivePicksOnlyStaleActiveTasksOnEligibleSteps(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, archiveWorkspaceID)
	workflows := newAutoOriginWorkflowStore(t, repo)
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-auto-archive", WorkspaceID: archiveWorkspaceID, Name: "Auto archive",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := workflows.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: "step-auto-archive", WorkflowID: "wf-auto-archive", Name: "Done",
		Position: 0, AutoArchiveAfterHours: 1,
	}); err != nil {
		t.Fatalf("CreateStep(eligible): %v", err)
	}
	if err := workflows.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: "step-no-auto-archive", WorkflowID: "wf-auto-archive", Name: "Work",
		Position: 1, AutoArchiveAfterHours: 0,
	}); err != nil {
		t.Fatalf("CreateStep(ineligible): %v", err)
	}

	rows := []struct {
		id      string
		stepID  string
		updated time.Time
	}{
		{"task-stale-eligible", "step-auto-archive", time.Now().UTC().Add(-48 * time.Hour)},
		{"task-fresh-eligible", "step-auto-archive", time.Now().UTC()},
		{"task-stale-ineligible", "step-no-auto-archive", time.Now().UTC().Add(-48 * time.Hour)},
		{"task-stale-archived", "step-auto-archive", time.Now().UTC().Add(-48 * time.Hour)},
	}
	for _, row := range rows {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: row.id, WorkspaceID: archiveWorkspaceID, WorkflowID: "wf-auto-archive",
			WorkflowStepID: row.stepID, Title: row.id,
		}); err != nil {
			t.Fatalf("CreateTask(%s): %v", row.id, err)
		}
	}
	if err := repo.ArchiveTask(ctx, "task-stale-archived"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	// Pin updated_at last: ArchiveTask bumps it.
	for _, row := range rows {
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
			`UPDATE tasks SET updated_at = ? WHERE id = ?`), row.updated, row.id); err != nil {
			t.Fatalf("pin updated_at for %s: %v", row.id, err)
		}
	}

	got, err := repo.ListTasksForAutoArchive(ctx)
	if err != nil {
		t.Fatalf("ListTasksForAutoArchive: %v", err)
	}
	assertIDsEqual(t, "ListTasksForAutoArchive", taskIDs(got), []string{"task-stale-eligible"})
}

func TestListArchivedTasksWithActiveSessionsReturnsOnlyStuckTasks(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-arch-running", "task-arch-done", "task-active-running")
	ctx := context.Background()

	sessions := []struct {
		id     string
		taskID string
		state  models.TaskSessionState
	}{
		{"session-arch-running", "task-arch-running", models.TaskSessionStateRunning},
		{"session-arch-done", "task-arch-done", models.TaskSessionStateCompleted},
		{"session-active-running", "task-active-running", models.TaskSessionStateRunning},
	}
	for _, session := range sessions {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: session.id, TaskID: session.taskID, State: session.state,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.id, err)
		}
	}
	for _, id := range []string{"task-arch-running", "task-arch-done"} {
		if err := repo.ArchiveTask(ctx, id); err != nil {
			t.Fatalf("ArchiveTask(%s): %v", id, err)
		}
	}

	got, err := repo.ListArchivedTasksWithActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListArchivedTasksWithActiveSessions: %v", err)
	}
	assertIDsEqual(t, "ListArchivedTasksWithActiveSessions", got, []string{"task-arch-running"})
}

func TestIncrementTaskSequenceAndWorkspaceTaskPrefix(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, archiveWorkspaceID)

	first, err := repo.IncrementTaskSequence(ctx, archiveWorkspaceID)
	if err != nil {
		t.Fatalf("IncrementTaskSequence: %v", err)
	}
	second, err := repo.IncrementTaskSequence(ctx, archiveWorkspaceID)
	if err != nil {
		t.Fatalf("second IncrementTaskSequence: %v", err)
	}
	if second != first+1 {
		t.Errorf("sequence went %d -> %d, want a strict +1 increment", first, second)
	}

	// A second workspace keeps its own counter.
	seedWorkspace(t, repo, "ws-archive-other")
	other, err := repo.IncrementTaskSequence(ctx, "ws-archive-other")
	if err != nil {
		t.Fatalf("IncrementTaskSequence(other workspace): %v", err)
	}
	if other != first {
		t.Errorf("other workspace sequence = %d, want it to start at %d independently", other, first)
	}

	// A seeded workspace has no office workflow and falls back to the KAN
	// prefix default baked into the COALESCE.
	prefix, officeWorkflowID, err := repo.GetWorkspaceTaskPrefix(ctx, archiveWorkspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceTaskPrefix: %v", err)
	}
	if prefix != "KAN" {
		t.Errorf("prefix = %q, want the KAN default", prefix)
	}
	if officeWorkflowID != "" {
		t.Errorf("officeWorkflowID = %q, want empty for a workspace with no office workflow", officeWorkflowID)
	}

	officeID, err := repo.EnsureOfficeWorkflow(ctx, archiveWorkspaceID)
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	_, officeWorkflowID, err = repo.GetWorkspaceTaskPrefix(ctx, archiveWorkspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceTaskPrefix after office workflow: %v", err)
	}
	if officeWorkflowID != officeID {
		t.Errorf("officeWorkflowID = %q, want the stamped %q", officeWorkflowID, officeID)
	}
}
