package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seatRow inserts a workflow_step_participants row. taskID "" is a
// template-level seat (applies to every task at the step); a non-empty taskID
// is a per-task override.
func seatRow(t *testing.T, repo *Repository, id, stepID, taskID, role string, decisionRequired bool) {
	t.Helper()
	required := 0
	if decisionRequired {
		required = 1
	}
	if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, ?, 'agent-1', ?, 0)
	`), id, stepID, taskID, role, required); err != nil {
		t.Fatalf("seed seat %s: %v", id, err)
	}
}

// stampTaskQuiet backdates a task's updated_at. CreateTask stamps "now", and
// the candidate query's whole purpose is to select on that column, so every
// fixture has to be aged explicitly.
func stampTaskQuiet(t *testing.T, repo *Repository, taskID string, at time.Time) {
	t.Helper()
	if _, err := repo.db.ExecContext(context.Background(),
		repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), at, taskID); err != nil {
		t.Fatalf("backdate %s: %v", taskID, err)
	}
}

func candidateIDs(t *testing.T, repo *Repository, quietSince time.Time) []string {
	t.Helper()
	var (
		rows   []models.OfficeDecisionWaitCandidate
		cursor *models.OfficeDecisionWaitCursor
	)
	for {
		page, next, err := repo.ListOfficeDecisionWaitCandidates(context.Background(), quietSince, cursor)
		if err != nil {
			t.Fatalf("ListOfficeDecisionWaitCandidates: %v", err)
		}
		rows = append(rows, page...)
		if next == nil {
			break
		}
		cursor = next
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TaskID)
	}
	return ids
}

func hasID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// officeCandidateFixture builds one workspace with a materialised office
// workflow and returns (repo, workspaceID, officeWorkflowID).
func officeCandidateFixture(t *testing.T) (*Repository, string, string) {
	t.Helper()
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	const workspaceID = "ws-decision-wait"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: "decision-wait", OwnerID: "u-1",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	officeWorkflowID, err := repo.EnsureOfficeWorkflow(ctx, workspaceID)
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	return repo, workspaceID, officeWorkflowID
}

func seedCandidateTask(
	t *testing.T, repo *Repository, workspaceID, workflowID, taskID, stepID, projectID string,
	state string,
) {
	t.Helper()
	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          taskID,
		State:          v1.TaskState(state),
		ProjectID:      projectID,
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask %s: %v", taskID, err)
	}
}

// TestListOfficeDecisionWaitCandidates_SelectsOnlyOfficeTasksAtDecisionSeats
// pins the cheap half of REQ-OFFICE-STALL-VISIBILITY-002's predicate. Each
// negative row differs from the positive one in exactly one respect, so a
// broken clause fails a named case rather than merely shrinking a list.
func TestListOfficeDecisionWaitCandidates_SelectsOnlyOfficeTasksAtDecisionSeats(t *testing.T) {
	repo, workspaceID, officeWorkflowID := officeCandidateFixture(t)
	quiet := time.Now().UTC().Add(-2 * time.Hour)

	const (
		reviewStep = "step-review"
		plainStep  = "step-plain"
	)
	seatRow(t, repo, "seat-review", reviewStep, "", "reviewer", true)
	seatRow(t, repo, "seat-plain", plainStep, "", "reviewer", false)

	// Positive: office-by-workflow, decision-required seat, quiet.
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-waiting", reviewStep, "", "IN_PROGRESS")
	// Office by project link rather than workflow — must also qualify.
	seedCandidateTask(t, repo, workspaceID, "wf-kanban", "t-waiting-project", reviewStep, "proj-1", "IN_PROGRESS")
	// Kanban task at the same step: excluded by the Office predicate.
	seedCandidateTask(t, repo, workspaceID, "wf-kanban", "t-kanban", reviewStep, "", "IN_PROGRESS")
	// Office task at a step whose only seat is not decision-required.
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-no-decision-seat", plainStep, "", "IN_PROGRESS")
	// Office task at a step with no seats at all.
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-seatless", "step-none", "", "IN_PROGRESS")
	// Office task in a terminal state.
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-done", reviewStep, "", "COMPLETED")

	for _, id := range []string{
		"t-waiting", "t-waiting-project", "t-kanban",
		"t-no-decision-seat", "t-seatless", "t-done",
	} {
		stampTaskQuiet(t, repo, id, quiet)
	}

	got := candidateIDs(t, repo, time.Now().UTC().Add(-time.Hour))

	for _, want := range []string{"t-waiting", "t-waiting-project"} {
		if !hasID(got, want) {
			t.Errorf("candidate %q missing from %v", want, got)
		}
	}
	for _, unwanted := range []string{"t-kanban", "t-no-decision-seat", "t-seatless", "t-done"} {
		if hasID(got, unwanted) {
			t.Errorf("candidate %q must not be selected; got %v", unwanted, got)
		}
	}
}

func TestListOfficeDecisionWaitCandidates_RecentlyTouchedTaskIsNotQuiet(t *testing.T) {
	repo, workspaceID, officeWorkflowID := officeCandidateFixture(t)
	seatRow(t, repo, "seat-review", "step-review", "", "approver", true)
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-fresh", "step-review", "", "IN_PROGRESS")
	stampTaskQuiet(t, repo, "t-fresh", time.Now().UTC().Add(-5*time.Minute))

	got := candidateIDs(t, repo, time.Now().UTC().Add(-time.Hour))
	if hasID(got, "t-fresh") {
		t.Errorf("a task touched 5 minutes ago must not be a candidate at a 1h threshold; got %v", got)
	}
}

func TestListOfficeDecisionWaitCandidates_PerTaskSeatQualifiesOnlyItsOwnTask(t *testing.T) {
	repo, workspaceID, officeWorkflowID := officeCandidateFixture(t)
	quiet := time.Now().UTC().Add(-2 * time.Hour)

	// The only decision-required seat at this step is a per-task override
	// belonging to t-mine, so t-theirs sits at a step with no seat of its own.
	seatRow(t, repo, "seat-mine", "step-review", "t-mine", "reviewer", true)
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-mine", "step-review", "", "IN_PROGRESS")
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-theirs", "step-review", "", "IN_PROGRESS")
	stampTaskQuiet(t, repo, "t-mine", quiet)
	stampTaskQuiet(t, repo, "t-theirs", quiet)

	got := candidateIDs(t, repo, time.Now().UTC().Add(-time.Hour))
	if !hasID(got, "t-mine") {
		t.Errorf("t-mine holds the per-task decision seat and must be a candidate; got %v", got)
	}
	if hasID(got, "t-theirs") {
		t.Errorf("t-theirs has no seat of its own and must not borrow t-mine's; got %v", got)
	}
}

func TestListOfficeDecisionWaitCandidates_ArchivedTaskIsExcluded(t *testing.T) {
	repo, workspaceID, officeWorkflowID := officeCandidateFixture(t)
	seatRow(t, repo, "seat-review", "step-review", "", "reviewer", true)
	seedCandidateTask(t, repo, workspaceID, officeWorkflowID, "t-archived", "step-review", "", "IN_PROGRESS")
	archivedAt := time.Now().UTC().Add(-3 * time.Hour)
	if _, err := repo.db.ExecContext(context.Background(),
		repo.db.Rebind(`UPDATE tasks SET archived_at = ?, updated_at = ? WHERE id = ?`),
		archivedAt, archivedAt, "t-archived"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	got := candidateIDs(t, repo, time.Now().UTC().Add(-time.Hour))
	if hasID(got, "t-archived") {
		t.Errorf("an archived task must not be surfaced as decision-waiting; got %v", got)
	}
}
