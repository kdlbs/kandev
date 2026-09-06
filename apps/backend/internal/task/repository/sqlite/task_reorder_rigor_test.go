package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// markHiddenForReorder marks id as a hidden task per Terminology (archived,
// ephemeral, or automation-run), so it is excluded from a reorder's band
// membership.
func markHiddenForReorder(t *testing.T, repo *Repository, id, kind string) {
	t.Helper()
	var stmt string
	switch kind {
	case "archived":
		stmt = `UPDATE tasks SET archived_at = ? WHERE id = ?`
	case "ephemeral":
		stmt = `UPDATE tasks SET is_ephemeral = 1 WHERE id = ?`
	case "automation_run":
		stmt = `UPDATE tasks SET origin = 'automation_run' WHERE id = ?`
	default:
		t.Fatalf("unknown hidden-task kind %q", kind)
	}
	var err error
	if kind == "archived" {
		_, err = repo.db.Exec(repo.db.Rebind(stmt), time.Now().UTC(), id)
	} else {
		_, err = repo.db.Exec(repo.db.Rebind(stmt), id)
	}
	if err != nil {
		t.Fatalf("markHiddenForReorder(%s, %s): %v", id, kind, err)
	}
}

// TestReorderStepTasksDoesNotTouchOtherStepPositions pins
// AC-TASKS-KANBAN-TASK-REORDERING-001.31: reordering one step's band must
// rewrite no task's position in another step.
func TestReorderStepTasksDoesNotTouchOtherStepPositions(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-other-step")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-other-step", WorkspaceID: "ws-other-step", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-reordered", "wf-other-step")
	seedReorderStep(t, repo, "step-bystander", "wf-other-step")
	mustCreateReorderTask(t, ctx, repo, "reordered-a", "ws-other-step", "wf-other-step", "step-reordered")
	mustCreateReorderTask(t, ctx, repo, "reordered-b", "ws-other-step", "wf-other-step", "step-reordered")
	mustCreateReorderTask(t, ctx, repo, "bystander-a", "ws-other-step", "wf-other-step", "step-bystander")
	mustCreateReorderTask(t, ctx, repo, "bystander-b", "ws-other-step", "wf-other-step", "step-bystander")

	if _, _, err := repo.ReorderStepTasks(ctx, "step-reordered", ReorderBandAdmitted, []string{"reordered-b", "reordered-a"}); err != nil {
		t.Fatalf("ReorderStepTasks: %v", err)
	}

	if got := taskPosition(t, ctx, repo, "bystander-a"); got != 0 {
		t.Fatalf("bystander-a position = %d, want unchanged 0", got)
	}
	if got := taskPosition(t, ctx, repo, "bystander-b"); got != 1 {
		t.Fatalf("bystander-b position = %d, want unchanged 1", got)
	}
}

// TestReorderStepTasksExcludesHiddenTasksFromMembership pins
// AC-TASKS-KANBAN-TASK-REORDERING-001.33/.31: an archived, ephemeral, or
// automation-run task is never part of band membership, is never named in a
// valid submission, never has its position rewritten, and never causes the
// step_changed conflict of .19/.26 just by existing in the step.
func TestReorderStepTasksExcludesHiddenTasksFromMembership(t *testing.T) {
	for _, kind := range []string{"archived", "ephemeral", "automation_run"} {
		t.Run(kind, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			wsID, wfID, stepID := "ws-hidden-"+kind, "wf-hidden-"+kind, "step-hidden-"+kind
			seedWorkspace(t, repo, wsID)
			if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: wsID, Name: "Workflow"}); err != nil {
				t.Fatal(err)
			}
			seedReorderStep(t, repo, stepID, wfID)
			mustCreateReorderTask(t, ctx, repo, "visible-a", wsID, wfID, stepID)
			mustCreateReorderTask(t, ctx, repo, "visible-b", wsID, wfID, stepID)
			mustCreateReorderTask(t, ctx, repo, "hidden-task", wsID, wfID, stepID)
			markHiddenForReorder(t, repo, "hidden-task", kind)
			hiddenBefore := taskPosition(t, ctx, repo, "hidden-task")

			// The client never saw hidden-task, so it submits only the two
			// visible members — and that must succeed, not conflict.
			tasks, _, err := repo.ReorderStepTasks(ctx, stepID, ReorderBandAdmitted, []string{"visible-b", "visible-a"})
			if err != nil {
				t.Fatalf("ReorderStepTasks: %v", err)
			}
			if len(tasks) != 2 {
				t.Fatalf("returned tasks = %d, want 2 (hidden task excluded)", len(tasks))
			}
			for _, task := range tasks {
				if task.ID == "hidden-task" {
					t.Fatal("hidden task must not appear in the returned membership")
				}
			}
			if got := taskPosition(t, ctx, repo, "hidden-task"); got != hiddenBefore {
				t.Fatalf("hidden task position = %d, want unchanged %d", got, hiddenBefore)
			}

			// Naming the hidden task explicitly is a structurally malformed
			// request (it resolves to no current non-hidden member), not a
			// step_changed conflict.
			_, _, err = repo.ReorderStepTasks(ctx, stepID, ReorderBandAdmitted, []string{"visible-a", "hidden-task"})
			if err == nil {
				t.Fatal("naming a hidden task: want an error")
			}
		})
	}
}

// TestReorderStepTasksDepartureLeavesRemainingRelativeOrderIntact pins
// AC-TASKS-KANBAN-TASK-REORDERING-001.30: when a task leaves a band (here, a
// cross-step move), the remaining tasks keep their relative order; position
// values need not stay contiguous.
func TestReorderStepTasksDepartureLeavesRemainingRelativeOrderIntact(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-departure")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-departure", WorkspaceID: "ws-departure", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-departure-src", "wf-departure")
	seedReorderStep(t, repo, "step-departure-dst", "wf-departure")
	mustCreateReorderTask(t, ctx, repo, "dep-a", "ws-departure", "wf-departure", "step-departure-src")
	mid := mustCreateReorderTask(t, ctx, repo, "dep-mid", "ws-departure", "wf-departure", "step-departure-src")
	mustCreateReorderTask(t, ctx, repo, "dep-c", "ws-departure", "wf-departure", "step-departure-src")
	beforeA := taskPosition(t, ctx, repo, "dep-a")
	beforeC := taskPosition(t, ctx, repo, "dep-c")
	if beforeA >= beforeC {
		t.Fatalf("fixture invariant broken: dep-a (%d) must precede dep-c (%d)", beforeA, beforeC)
	}

	if _, err := repo.UpdateTaskWithWorkflowStepAdmission(ctx, mid, "step-departure-src", "step-departure-dst", 0); err != nil {
		t.Fatalf("move dep-mid out of the band: %v", err)
	}

	if got := taskPosition(t, ctx, repo, "dep-a"); got != beforeA {
		t.Fatalf("dep-a position = %d, want unchanged %d (no renumbering on departure)", got, beforeA)
	}
	if got := taskPosition(t, ctx, repo, "dep-c"); got != beforeC {
		t.Fatalf("dep-c position = %d, want unchanged %d (no renumbering on departure)", got, beforeC)
	}
	if taskPosition(t, ctx, repo, "dep-a") >= taskPosition(t, ctx, repo, "dep-c") {
		t.Fatal("relative order of remaining tasks was not preserved")
	}
}

// TestReorderStepTasksConcurrentSameBandSerializes pins
// AC-TASKS-KANBAN-TASK-REORDERING-001.25: two reorders of the same band with
// the same starting membership must apply one at a time; the last to commit
// is authoritative and the revision counter advances exactly once per
// commit, with no lost update.
func TestReorderStepTasksConcurrentSameBandSerializes(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-concurrent-same-band")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-concurrent-same-band", WorkspaceID: "ws-concurrent-same-band", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-concurrent-same-band", "wf-concurrent-same-band")
	mustCreateReorderTask(t, ctx, repo, "same-a", "ws-concurrent-same-band", "wf-concurrent-same-band", "step-concurrent-same-band")
	mustCreateReorderTask(t, ctx, repo, "same-b", "ws-concurrent-same-band", "wf-concurrent-same-band", "step-concurrent-same-band")

	orders := [][]string{
		{"same-a", "same-b"},
		{"same-b", "same-a"},
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]int64, len(orders))
	errs := make([]error, len(orders))
	for i, order := range orders {
		wg.Add(1)
		go func(i int, order []string) {
			defer wg.Done()
			<-start
			_, revision, err := repo.ReorderStepTasks(ctx, "step-concurrent-same-band", ReorderBandAdmitted, order)
			results[i], errs[i] = revision, err
		}(i, order)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("reorder %d: %v", i, err)
		}
	}
	// Both submissions had the same starting membership, so both commit
	// (AC.25 does not reject either one); each gets its own revision, and
	// the two revisions must be distinct (no lost update serialized them
	// into the same counter value).
	if results[0] == results[1] {
		t.Fatalf("both commits produced revision %d; want two distinct revisions", results[0])
	}
	// Positions must be dense 0..1 whichever order committed last.
	posA, posB := taskPosition(t, ctx, repo, "same-a"), taskPosition(t, ctx, repo, "same-b")
	if (posA != 0 || posB != 1) && (posA != 1 || posB != 0) {
		t.Fatalf("final positions not dense/consistent: same-a=%d same-b=%d", posA, posB)
	}
}

// TestReorderStepTasksConcurrentDifferentBandsDoNotBlockEachOther pins
// AC-TASKS-KANBAN-TASK-REORDERING-001.37: concurrent reorders naming the
// step's two different bands must both succeed, each ending in its own
// submitted order.
func TestReorderStepTasksConcurrentDifferentBandsDoNotBlockEachOther(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-concurrent-diff-bands")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-concurrent-diff-bands", WorkspaceID: "ws-concurrent-diff-bands", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderStep(t, repo, "step-concurrent-diff-bands", "wf-concurrent-diff-bands")
	mustCreateReorderTask(t, ctx, repo, "diff-admitted-a", "ws-concurrent-diff-bands", "wf-concurrent-diff-bands", "step-concurrent-diff-bands")
	mustCreateReorderTask(t, ctx, repo, "diff-admitted-b", "ws-concurrent-diff-bands", "wf-concurrent-diff-bands", "step-concurrent-diff-bands")
	mustCreateReorderTask(t, ctx, repo, "diff-queued-a", "ws-concurrent-diff-bands", "wf-concurrent-diff-bands", "step-concurrent-diff-bands")
	mustCreateReorderTask(t, ctx, repo, "diff-queued-b", "ws-concurrent-diff-bands", "wf-concurrent-diff-bands", "step-concurrent-diff-bands")
	markQueuedForReorder(t, repo, "diff-queued-a", "step-concurrent-diff-bands")
	markQueuedForReorder(t, repo, "diff-queued-b", "step-concurrent-diff-bands")

	var wg sync.WaitGroup
	start := make(chan struct{})
	var admittedErr, queuedErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, _, admittedErr = repo.ReorderStepTasks(ctx, "step-concurrent-diff-bands", ReorderBandAdmitted, []string{"diff-admitted-b", "diff-admitted-a"})
	}()
	go func() {
		defer wg.Done()
		<-start
		_, _, queuedErr = repo.ReorderStepTasks(ctx, "step-concurrent-diff-bands", ReorderBandQueued, []string{"diff-queued-b", "diff-queued-a"})
	}()
	close(start)
	wg.Wait()

	if admittedErr != nil {
		t.Fatalf("admitted-band reorder: %v", admittedErr)
	}
	if queuedErr != nil {
		t.Fatalf("queued-band reorder: %v", queuedErr)
	}
	if got := taskPosition(t, ctx, repo, "diff-admitted-b"); got != 0 {
		t.Fatalf("diff-admitted-b position = %d, want 0", got)
	}
	if got := taskPosition(t, ctx, repo, "diff-admitted-a"); got != 1 {
		t.Fatalf("diff-admitted-a position = %d, want 1", got)
	}
	if got := taskPosition(t, ctx, repo, "diff-queued-b"); got != 2 {
		t.Fatalf("diff-queued-b position = %d, want 2", got)
	}
	if got := taskPosition(t, ctx, repo, "diff-queued-a"); got != 3 {
		t.Fatalf("diff-queued-a position = %d, want 3", got)
	}
}
