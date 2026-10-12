package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// seedReorderStep inserts a workflow_steps row directly, mirroring the
// pattern in next_queued_task_ordering_pin_test.go: ReorderStepTasks only
// needs the row to exist and hold order_revision, not a full workflow
// repository.
func seedReorderStep(t *testing.T, repo *Repository, stepID, workflowID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`),
		stepID, workflowID, stepID, 0, now, now); err != nil {
		t.Fatalf("insert step %s: %v", stepID, err)
	}
}

func mustCreateReorderTask(t *testing.T, ctx context.Context, repo *Repository, id, workspaceID, workflowID, stepID string) *models.Task {
	t.Helper()
	task := &models.Task{
		ID:             id,
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          id,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask(%s): %v", id, err)
	}
	return task
}

func markQueuedForReorder(t *testing.T, repo *Repository, id, destinationStepID string) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE tasks SET wip_admitted = 0, queued_for_step_id = ? WHERE id = ?`),
		destinationStepID, id); err != nil {
		t.Fatalf("markQueuedForReorder(%s): %v", id, err)
	}
}

func taskPosition(t *testing.T, ctx context.Context, repo *Repository, id string) int {
	t.Helper()
	task, err := repo.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", id, err)
	}
	return task.Position
}

func TestReorderStepTasksRenumbersAdmittedBandAndBumpsRevision(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-reorder")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder", WorkspaceID: "ws-reorder", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-reorder", "wf-reorder")
	mustCreateReorderTask(t, ctx, repo, "task-a", "ws-reorder", "wf-reorder", "step-reorder")
	mustCreateReorderTask(t, ctx, repo, "task-b", "ws-reorder", "wf-reorder", "step-reorder")
	mustCreateReorderTask(t, ctx, repo, "task-c", "ws-reorder", "wf-reorder", "step-reorder")

	tasks, revision, err := repo.ReorderStepTasks(ctx, "step-reorder", ReorderBandAdmitted,
		[]string{"task-c", "task-a", "task-b"})
	if err != nil {
		t.Fatalf("ReorderStepTasks: %v", err)
	}
	if revision != 1 {
		t.Fatalf("revision = %d, want 1 (first reorder of any step)", revision)
	}
	if len(tasks) != 3 {
		t.Fatalf("tasks = %d, want 3", len(tasks))
	}
	want := map[string]int{"task-c": 0, "task-a": 1, "task-b": 2}
	for _, task := range tasks {
		if task.Position != want[task.ID] {
			t.Fatalf("task %s position = %d, want %d", task.ID, task.Position, want[task.ID])
		}
	}
	for id, position := range want {
		if got := taskPosition(t, ctx, repo, id); got != position {
			t.Fatalf("persisted %s position = %d, want %d", id, got, position)
		}
	}
}

// TestReorderStepTasksBandsAreIndependent pins AC-TASKS-KANBAN-TASK-REORDERING-001.3:
// reordering one band must not disturb the other band's relative order, and
// AC-TASKS-KANBAN-TASK-REORDERING-001.15's admitted-first dense renumbering
// across both.
func TestReorderStepTasksBandsAreIndependent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-bands")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-bands", WorkspaceID: "ws-bands", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-bands", "wf-bands")
	mustCreateReorderTask(t, ctx, repo, "admitted-1", "ws-bands", "wf-bands", "step-bands")
	mustCreateReorderTask(t, ctx, repo, "admitted-2", "ws-bands", "wf-bands", "step-bands")
	mustCreateReorderTask(t, ctx, repo, "queued-1", "ws-bands", "wf-bands", "step-bands")
	mustCreateReorderTask(t, ctx, repo, "queued-2", "ws-bands", "wf-bands", "step-bands")
	markQueuedForReorder(t, repo, "queued-1", "step-bands")
	markQueuedForReorder(t, repo, "queued-2", "step-bands")

	if _, _, err := repo.ReorderStepTasks(ctx, "step-bands", ReorderBandAdmitted, []string{"admitted-2", "admitted-1"}); err != nil {
		t.Fatalf("reorder admitted band: %v", err)
	}
	if got := taskPosition(t, ctx, repo, "admitted-2"); got != 0 {
		t.Fatalf("admitted-2 position = %d, want 0", got)
	}
	if got := taskPosition(t, ctx, repo, "admitted-1"); got != 1 {
		t.Fatalf("admitted-1 position = %d, want 1", got)
	}
	// The queued band kept its existing step order (creation order) across
	// the admitted-only reorder, and it renders after the admitted band.
	if got := taskPosition(t, ctx, repo, "queued-1"); got != 2 {
		t.Fatalf("queued-1 position = %d, want 2", got)
	}
	if got := taskPosition(t, ctx, repo, "queued-2"); got != 3 {
		t.Fatalf("queued-2 position = %d, want 3", got)
	}

	if _, _, err := repo.ReorderStepTasks(ctx, "step-bands", ReorderBandQueued, []string{"queued-2", "queued-1"}); err != nil {
		t.Fatalf("reorder queued band: %v", err)
	}
	// The admitted band's just-committed order must survive an unrelated
	// queued-band reorder untouched.
	if got := taskPosition(t, ctx, repo, "admitted-2"); got != 0 {
		t.Fatalf("admitted-2 position after queued reorder = %d, want 0", got)
	}
	if got := taskPosition(t, ctx, repo, "admitted-1"); got != 1 {
		t.Fatalf("admitted-1 position after queued reorder = %d, want 1", got)
	}
	if got := taskPosition(t, ctx, repo, "queued-2"); got != 2 {
		t.Fatalf("queued-2 position = %d, want 2", got)
	}
	if got := taskPosition(t, ctx, repo, "queued-1"); got != 3 {
		t.Fatalf("queued-1 position = %d, want 3", got)
	}
}

func TestReorderStepTasksIsIdempotentWhenMembershipUnchanged(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-idempotent")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-idempotent", WorkspaceID: "ws-idempotent", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-idempotent", "wf-idempotent")
	mustCreateReorderTask(t, ctx, repo, "task-x", "ws-idempotent", "wf-idempotent", "step-idempotent")
	mustCreateReorderTask(t, ctx, repo, "task-y", "ws-idempotent", "wf-idempotent", "step-idempotent")

	order := []string{"task-y", "task-x"}
	if _, rev1, err := repo.ReorderStepTasks(ctx, "step-idempotent", ReorderBandAdmitted, order); err != nil || rev1 != 1 {
		t.Fatalf("first reorder: revision=%d err=%v", rev1, err)
	}
	tasks, rev2, err := repo.ReorderStepTasks(ctx, "step-idempotent", ReorderBandAdmitted, order)
	if err != nil {
		t.Fatalf("second reorder: %v", err)
	}
	if rev2 != 2 {
		t.Fatalf("second reorder revision = %d, want 2 (each commit still advances the counter)", rev2)
	}
	want := map[string]int{"task-y": 0, "task-x": 1}
	for _, task := range tasks {
		if task.Position != want[task.ID] {
			t.Fatalf("task %s position = %d, want %d", task.ID, task.Position, want[task.ID])
		}
	}
}

func TestReorderStepTasksRejectsMembershipDrift(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-drift")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-drift", WorkspaceID: "ws-drift", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-drift", "wf-drift")
	mustCreateReorderTask(t, ctx, repo, "drift-a", "ws-drift", "wf-drift", "step-drift")
	mustCreateReorderTask(t, ctx, repo, "drift-b", "ws-drift", "wf-drift", "step-drift")
	// A third task arrives after the client's snapshot but before the
	// reorder request — the classic AC-TASKS-KANBAN-TASK-REORDERING-001.26
	// race: the submitted set is short of the band's current membership.
	mustCreateReorderTask(t, ctx, repo, "drift-c", "ws-drift", "wf-drift", "step-drift")

	tasks, revision, err := repo.ReorderStepTasks(ctx, "step-drift", ReorderBandAdmitted, []string{"drift-a", "drift-b"})
	if !errors.Is(err, repoerrors.ErrStepChanged) {
		t.Fatalf("err = %v, want ErrStepChanged", err)
	}
	if revision != 0 {
		t.Fatalf("revision = %d, want 0 (step never reordered)", revision)
	}
	if len(tasks) != 3 {
		t.Fatalf("authoritative order = %d tasks, want 3", len(tasks))
	}
	// Each task's own arrival position (creation order: a, b, c) must be
	// untouched by the rejected reorder.
	want := map[string]int{"drift-a": 0, "drift-b": 1, "drift-c": 2}
	for id, wantPosition := range want {
		if got := taskPosition(t, ctx, repo, id); got != wantPosition {
			t.Fatalf("position changed on rejected reorder: %s = %d, want unchanged %d", id, got, wantPosition)
		}
	}
}

// TestReorderStepTasksRejectsCrossBandID pins the AC.18/AC.19 split this
// repository resolves: a submitted id that names a real, current member of
// stepID but the OTHER band is band drift (AC.26's "moved into or out of"),
// not a structurally malformed request, so it is ErrStepChanged rather than
// ErrInvalidReorder.
func TestReorderStepTasksRejectsCrossBandID(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-crossband")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-crossband", WorkspaceID: "ws-crossband", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-crossband", "wf-crossband")
	mustCreateReorderTask(t, ctx, repo, "admitted-only", "ws-crossband", "wf-crossband", "step-crossband")
	mustCreateReorderTask(t, ctx, repo, "queued-only", "ws-crossband", "wf-crossband", "step-crossband")
	markQueuedForReorder(t, repo, "queued-only", "step-crossband")

	_, _, err := repo.ReorderStepTasks(ctx, "step-crossband", ReorderBandAdmitted, []string{"queued-only"})
	if !errors.Is(err, repoerrors.ErrStepChanged) {
		t.Fatalf("err = %v, want ErrStepChanged", err)
	}
}

func TestReorderStepTasksRejectsUnknownID(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-unknown")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-unknown", WorkspaceID: "ws-unknown", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-unknown", "wf-unknown")
	mustCreateReorderTask(t, ctx, repo, "known-task", "ws-unknown", "wf-unknown", "step-unknown")

	_, _, err := repo.ReorderStepTasks(ctx, "step-unknown", ReorderBandAdmitted, []string{"known-task", "ghost-task"})
	if !errors.Is(err, repoerrors.ErrInvalidReorder) {
		t.Fatalf("err = %v, want ErrInvalidReorder", err)
	}
	if got := taskPosition(t, ctx, repo, "known-task"); got != 0 {
		t.Fatalf("position changed on rejected reorder: known-task = %d, want unchanged 0", got)
	}
}

func TestReorderStepTasksRejectsMalformedRequests(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-malformed")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-malformed", WorkspaceID: "ws-malformed", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-malformed", "wf-malformed")
	mustCreateReorderTask(t, ctx, repo, "task-1", "ws-malformed", "wf-malformed", "step-malformed")
	mustCreateReorderTask(t, ctx, repo, "task-2", "ws-malformed", "wf-malformed", "step-malformed")

	cases := []struct {
		name string
		band string
		ids  []string
	}{
		{name: "invalid band value", band: "bogus", ids: []string{"task-1", "task-2"}},
		{name: "empty list", band: ReorderBandAdmitted, ids: []string{}},
		{name: "duplicate id", band: ReorderBandAdmitted, ids: []string{"task-1", "task-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := repo.ReorderStepTasks(ctx, "step-malformed", tc.band, tc.ids)
			if !errors.Is(err, repoerrors.ErrInvalidReorder) {
				t.Fatalf("err = %v, want ErrInvalidReorder", err)
			}
		})
	}
}
