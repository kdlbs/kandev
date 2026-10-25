package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// ISSUE-6 / AC-OFFICE-SEAT-READ-SCOPE-001.1-.2: the office read projections
// (ListTaskParticipants, ListAllTaskParticipants) must see a per-task seat
// across the task's ENTIRE current workflow, the same way the workflow
// engine's own quorum slate (gatherParticipantSlate) does — not just at the
// task's current step. Before the fix, a reviewer/approver seated at one
// step (e.g. Work) vanished from every reader of these two methods the
// moment the task moved to another step of the same workflow (e.g. Review),
// including agent decision authorization and the approval gate.

// seedParticipantTaskWithWorkflow inserts a task bound to both a workflow
// and a current step within it — seedParticipantTask (participants_ops_test.go)
// leaves workflow_id at its schema default (”) and so cannot exercise the
// JOIN-scoped branch of the fix, only its any-step fallback.
func seedParticipantTaskWithWorkflow(t *testing.T, repo *sqlite.Repository, taskID, workflowID, stepID string) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', ?, ?, 'Task', datetime('now'), datetime('now'))
	`, taskID, workflowID, stepID); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

// seedWorkflowStep inserts a workflow_steps row so the fix's JOIN
// (workflow_step_participants.step_id -> workflow_steps.id) has a
// workflow_id to resolve.
func seedWorkflowStep(t *testing.T, repo *sqlite.Repository, stepID, workflowID string, position int) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO workflow_steps (id, workflow_id, position, name)
		VALUES (?, ?, ?, ?)
	`, stepID, workflowID, position, stepID); err != nil {
		t.Fatalf("seed workflow step %s: %v", stepID, err)
	}
}

// moveTaskToStep updates a task's current step in place, simulating a
// workflow step transition without touching workflow_id.
func moveTaskToStep(t *testing.T, repo *sqlite.Repository, taskID, stepID string) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(),
		`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, stepID, taskID); err != nil {
		t.Fatalf("move task %s to step %s: %v", taskID, stepID, err)
	}
}

// TestListAllTaskParticipants_SeesSeatAfterStepMove is the core regression
// for ISSUE-6: a reviewer seated while the task stood on "Work" must still
// be visible via both ListAllTaskParticipants and ListTaskParticipants after
// the task moves to "Review" in the same workflow.
func TestListAllTaskParticipants_SeesSeatAfterStepMove(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-work", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-review", "wf-1", 1)
	seedParticipantTaskWithWorkflow(t, repo, "task-moved", "wf-1", "step-work")
	seedManualSeat(t, repo, "seat-rev", "step-work", "task-moved", "reviewer", "agent-rev")

	moveTaskToStep(t, repo, "task-moved", "step-review")

	all, err := repo.ListAllTaskParticipants(ctx, "task-moved")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(all) != 1 || all[0].AgentProfileID != "agent-rev" || all[0].Role != "reviewer" {
		t.Fatalf("ListAllTaskParticipants = %+v, want the reviewer seat cast before the move", all)
	}
	if all[0].TaskID != "task-moved" {
		t.Errorf("TaskID = %q, want task-moved", all[0].TaskID)
	}

	byRole, err := repo.ListTaskParticipants(ctx, "task-moved", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(byRole) != 1 || byRole[0].AgentProfileID != "agent-rev" {
		t.Fatalf("ListTaskParticipants(reviewer) = %+v, want the seat cast before the move", byRole)
	}
}

// TestListAllTaskParticipants_DedupSameRoleAgentAcrossSteps covers a stale
// per-task row left behind at an earlier step alongside a fresh one at the
// current step for the same (role, agent): the slate must collapse to one
// row, and it must be the CURRENT step's row (its decision_required wins),
// mirroring the engine's canonicalizeByTaskRoleAgent (prefer the evaluating
// step).
func TestListAllTaskParticipants_DedupSameRoleAgentAcrossSteps(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-work", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-review", "wf-1", 1)
	seedParticipantTaskWithWorkflow(t, repo, "task-dedup", "wf-1", "step-work")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('seat-old', 'step-work',   'task-dedup', 'reviewer', 'agent-rev', 0, 0),
			('seat-new', 'step-review', 'task-dedup', 'reviewer', 'agent-rev', 1, 0)
	`); err != nil {
		t.Fatalf("seed participants: %v", err)
	}

	moveTaskToStep(t, repo, "task-dedup", "step-review")

	got, err := repo.ListTaskParticipants(ctx, "task-dedup", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("participants = %+v, want exactly one deduped row", got)
	}
	if !got[0].DecisionRequired {
		t.Errorf("DecisionRequired = false, want the current-step row's value (true) to win over the stale one: %+v", got[0])
	}
}

// TestListAllTaskParticipants_TemplateRowOnlyCountsAtCurrentStep pins that
// template-level rows (task_id="") remain step-scoped even though per-task
// rows are now workflow-scoped: a template reviewer configured on "Work"
// must not leak onto a task now sitting on "Review", matching the engine's
// own ListStepParticipants(stepID, "") call for the template half of its
// slate.
func TestListAllTaskParticipants_TemplateRowOnlyCountsAtCurrentStep(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-work", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-review", "wf-1", 1)
	seedParticipantTaskWithWorkflow(t, repo, "task-tpl", "wf-1", "step-work")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('tpl-work', 'step-work', '', 'reviewer', 'agent-tpl', 0, 0)
	`); err != nil {
		t.Fatalf("seed template row: %v", err)
	}

	moveTaskToStep(t, repo, "task-tpl", "step-review")

	got, err := repo.ListTaskParticipants(ctx, "task-tpl", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("participants = %+v, want none: the template row belongs to step-work, not the task's current step", got)
	}
}

// TestListAllTaskParticipants_ExcludesPerTaskRowInDifferentWorkflow pins the
// JOIN's workflow boundary: a per-task row whose step belongs to a
// DIFFERENT workflow than the task's current one must not appear, even
// though it names the same task_id. This is the guard against a task that
// changed workflow templates picking up a stale override left behind under
// the old one.
func TestListAllTaskParticipants_ExcludesPerTaskRowInDifferentWorkflow(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-wf1", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-wf2", "wf-2", 0)
	seedParticipantTaskWithWorkflow(t, repo, "task-xwf", "wf-1", "step-wf1")
	// A row left over under a different workflow's step, naming this task.
	seedManualSeat(t, repo, "seat-foreign", "step-wf2", "task-xwf", "reviewer", "agent-foreign")

	got, err := repo.ListAllTaskParticipants(ctx, "task-xwf")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("participants = %+v, want none: the seat belongs to a different workflow", got)
	}
}

// TestListAllTaskParticipants_FallsBackToAnyStepWhenWorkflowIDEmpty covers a
// task row with no workflow_id recorded (legacy data, or a fixture that
// never set it) — the same fallback the engine's gatherParticipantSlate
// takes when it has no WorkflowScopedParticipantStore: per-task rows are
// visible at ANY step for that task, not just the current one.
func TestListAllTaskParticipants_FallsBackToAnyStepWhenWorkflowIDEmpty(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "task-legacy", "step-work") // workflow_id left at '' default
	seedManualSeat(t, repo, "seat-legacy", "step-work", "task-legacy", "reviewer", "agent-rev")

	moveTaskToStep(t, repo, "task-legacy", "step-review")

	got, err := repo.ListAllTaskParticipants(ctx, "task-legacy")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(got) != 1 || got[0].AgentProfileID != "agent-rev" {
		t.Fatalf("participants = %+v, want the seat still visible under the any-step fallback", got)
	}
}

// TestRemoveTaskParticipant_AfterStepMoveStillRemoves pins that removal
// stays step-agnostic (RemoveTaskParticipant already filters on task_id +
// role + agent only) once combined with the now-workflow-scoped read: after
// removing, the seat must disappear from ListAllTaskParticipants too.
func TestRemoveTaskParticipant_AfterStepMoveStillRemoves(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-work", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-review", "wf-1", 1)
	seedParticipantTaskWithWorkflow(t, repo, "task-rm", "wf-1", "step-work")
	seedManualSeat(t, repo, "seat-rm", "step-work", "task-rm", "reviewer", "agent-rev")

	moveTaskToStep(t, repo, "task-rm", "step-review")

	if err := repo.RemoveTaskParticipant(ctx, "task-rm", "agent-rev", "reviewer"); err != nil {
		t.Fatalf("RemoveTaskParticipant: %v", err)
	}

	got, err := repo.ListAllTaskParticipants(ctx, "task-rm")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("participants = %+v, want none after removal", got)
	}
}

// TestAddTaskParticipant_ReplaceAfterStepMove pins that replacing a seat's
// occupant (remove then add a different agent) after a step move produces
// exactly the new occupant, not both.
func TestAddTaskParticipant_ReplaceAfterStepMove(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-work", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-review", "wf-1", 1)
	seedParticipantTaskWithWorkflow(t, repo, "task-replace", "wf-1", "step-work")
	seedParticipantAgent(t, repo, "agent-new")
	seedManualSeat(t, repo, "seat-old", "step-work", "task-replace", "reviewer", "agent-old")

	moveTaskToStep(t, repo, "task-replace", "step-review")

	if err := repo.RemoveTaskParticipant(ctx, "task-replace", "agent-old", "reviewer"); err != nil {
		t.Fatalf("RemoveTaskParticipant: %v", err)
	}
	if _, err := repo.AddTaskParticipant(ctx, "task-replace", "agent-new", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}

	got, err := repo.ListAllTaskParticipants(ctx, "task-replace")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(got) != 1 || got[0].AgentProfileID != "agent-new" {
		t.Fatalf("participants = %+v, want only agent-new", got)
	}
}

// TestAddTaskParticipant_ReAddingSameIdentityAfterStepMoveIsNoOp is
// AC-OFFICE-SEAT-READ-SCOPE-001.2: re-registering the SAME (role, agent) after
// a step move must be recognized as the identity the workflow-scoped slate
// already sees, reporting Unchanged rather than inserting a duplicate row —
// probeExistingIdentity's old exact-step-only check would otherwise insert
// a second seat for an occupant the engine already counts once.
func TestAddTaskParticipant_ReAddingSameIdentityAfterStepMoveIsNoOp(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-work", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-review", "wf-1", 1)
	seedParticipantTaskWithWorkflow(t, repo, "task-readd", "wf-1", "step-work")
	seedParticipantAgent(t, repo, "agent-rev")
	seedManualSeat(t, repo, "seat-rev", "step-work", "task-readd", "reviewer", "agent-rev")

	moveTaskToStep(t, repo, "task-readd", "step-review")

	result, err := repo.AddTaskParticipant(ctx, "task-readd", "agent-rev", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeUnchanged {
		t.Fatalf("Outcome = %q, want %q (no duplicate seat for an identity the workflow slate already holds)",
			result.Outcome, sqlite.ParticipantWriteOutcomeUnchanged)
	}
	if n := participantRowCount(t, repo, "task-readd"); n != 1 {
		t.Fatalf("participant row count = %d, want 1 (no duplicate row)", n)
	}
}

// TestAddTaskParticipant_ReAddingSameIdentityFallsBackToAnyStepWhenWorkflowIDEmpty
// mirrors the re-add no-op above for a task with no workflow_id recorded,
// pinning that the write-path widening takes the same any-step fallback the
// read-path does.
func TestAddTaskParticipant_ReAddingSameIdentityFallsBackToAnyStepWhenWorkflowIDEmpty(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "task-readd-legacy", "step-work") // workflow_id left at '' default
	seedParticipantAgent(t, repo, "agent-rev")
	seedManualSeat(t, repo, "seat-rev", "step-work", "task-readd-legacy", "reviewer", "agent-rev")

	moveTaskToStep(t, repo, "task-readd-legacy", "step-review")

	result, err := repo.AddTaskParticipant(ctx, "task-readd-legacy", "agent-rev", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeUnchanged {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, sqlite.ParticipantWriteOutcomeUnchanged)
	}
	if n := participantRowCount(t, repo, "task-readd-legacy"); n != 1 {
		t.Fatalf("participant row count = %d, want 1 (no duplicate row)", n)
	}
}

// TestListTaskParticipantsAtCurrentStep_StaysStepScopedAfterMove pins the
// capacity-only read's contract (AC-OFFICE-SESSION-TERM-002.3): unlike
// ListAllTaskParticipants, this method must NOT see a seat left behind at a
// step the task no longer stands on.
func TestListTaskParticipantsAtCurrentStep_StaysStepScopedAfterMove(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-work", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-review", "wf-1", 1)
	seedParticipantTaskWithWorkflow(t, repo, "task-capacity", "wf-1", "step-work")
	seedManualSeat(t, repo, "seat-rev", "step-work", "task-capacity", "reviewer", "agent-rev")

	moveTaskToStep(t, repo, "task-capacity", "step-review")

	got, err := repo.ListTaskParticipantsAtCurrentStep(ctx, "task-capacity")
	if err != nil {
		t.Fatalf("ListTaskParticipantsAtCurrentStep: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("participants = %+v, want none: the seat is at a step the task has left", got)
	}
}
