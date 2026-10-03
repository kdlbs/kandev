package sqlite_test

import (
	"context"
	"testing"
)

// ISSUE-6 review round 1, finding R1-1: listWorkflowScopedSeats used to
// resolve a task's workflow_step_id (stepIDForTask) and workflow_id
// (GetTaskWorkflowID) via two separate read-pool statements. A move
// committing between them — moving the task to a different workflow, not
// just a different step of the same one — could hand back a (workflow,
// step) pair that never existed together: e.g. the OLD workflow_id paired
// with the NEW workflow_step_id. The resulting slate would then mix
// per-task rows from one workflow with template rows from a step in the
// other. taskWorkflowContext now reads both columns in a single SELECT, so
// a single UPDATE moving both columns together always yields a coherent
// pair — these tests seed two full workflows worth of seats and prove the
// post-move slate never mixes them.

// TestTaskWorkflowContext_ReturnsCoherentPairAfterCrossWorkflowMove asserts
// the single-read helper directly: after one UPDATE moves both workflow_id
// and workflow_step_id together, the helper must return exactly that pair,
// never a stale half of it.
func TestTaskWorkflowContext_ReturnsCoherentPairAfterCrossWorkflowMove(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-w1-a", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-w2-b", "wf-2", 0)
	seedParticipantTaskWithWorkflow(t, repo, "task-xmove", "wf-1", "step-w1-a")

	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_id = ?, workflow_step_id = ? WHERE id = ?`,
		"wf-2", "step-w2-b", "task-xmove"); err != nil {
		t.Fatalf("move task across workflows: %v", err)
	}

	workflowID, stepID, err := repo.TaskWorkflowContext(ctx, "task-xmove")
	if err != nil {
		t.Fatalf("TaskWorkflowContext: %v", err)
	}
	if workflowID != "wf-2" || stepID != "step-w2-b" {
		t.Fatalf("TaskWorkflowContext = (%q, %q), want (\"wf-2\", \"step-w2-b\") from one coherent read of the post-move row",
			workflowID, stepID)
	}
}

// TestListAllTaskParticipants_CoherentSlateAfterCrossWorkflowMove is the
// full-slate companion: seats a per-task row and a template row under EACH
// of two workflows, moves the task from (wf-1, step-w1-a) to
// (wf-2, step-w2-b) with one UPDATE of both columns, and asserts the
// resulting slate is exactly wf-2's per-task seat plus step-w2-b's template
// seat — never wf-1's stale per-task seat or step-w1-a's stale template
// seat.
func TestListAllTaskParticipants_CoherentSlateAfterCrossWorkflowMove(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedWorkflowStep(t, repo, "step-w1-a", "wf-1", 0)
	seedWorkflowStep(t, repo, "step-w2-b", "wf-2", 0)
	seedParticipantTaskWithWorkflow(t, repo, "task-xwf-slate", "wf-1", "step-w1-a")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('seat-wf1',  'step-w1-a', 'task-xwf-slate', 'reviewer', 'agent-wf1',    1, 0),
			('seat-wf2',  'step-w2-b', 'task-xwf-slate', 'reviewer', 'agent-wf2',    1, 0),
			('tpl-w1-a',  'step-w1-a', '',               'approver', 'agent-tpl-1',  1, 0),
			('tpl-w2-b',  'step-w2-b', '',               'approver', 'agent-tpl-2',  1, 0)
	`); err != nil {
		t.Fatalf("seed participants: %v", err)
	}

	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET workflow_id = ?, workflow_step_id = ? WHERE id = ?`,
		"wf-2", "step-w2-b", "task-xwf-slate"); err != nil {
		t.Fatalf("move task across workflows: %v", err)
	}

	got, err := repo.ListAllTaskParticipants(ctx, "task-xwf-slate")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	byAgent := map[string]bool{}
	for _, p := range got {
		byAgent[p.AgentProfileID] = true
	}
	if len(got) != 2 {
		t.Fatalf("participants = %+v, want exactly 2 (wf-2's per-task seat + step-w2-b's template seat)", got)
	}
	if !byAgent["agent-wf2"] {
		t.Errorf("missing agent-wf2 (wf-2's per-task reviewer seat): %+v", got)
	}
	if !byAgent["agent-tpl-2"] {
		t.Errorf("missing agent-tpl-2 (step-w2-b's template approver seat): %+v", got)
	}
	if byAgent["agent-wf1"] {
		t.Errorf("agent-wf1 (wf-1's stale per-task seat) leaked into the post-move slate: %+v", got)
	}
	if byAgent["agent-tpl-1"] {
		t.Errorf("agent-tpl-1 (step-w1-a's stale template seat) leaked into the post-move slate: %+v", got)
	}
}
