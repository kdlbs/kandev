package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedParticipantTask inserts a task bound to the given workflow step.
func seedParticipantTask(t *testing.T, repo *sqlite.Repository, taskID, stepID string) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO tasks (id, workspace_id, workflow_step_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', ?, 'Task', datetime('now'), datetime('now'))
	`, taskID, stepID); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

func TestGetTaskWorkflowStepID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "wp-bound", "step-1")
	seedParticipantTask(t, repo, "wp-unbound", "")

	cases := []struct {
		name   string
		taskID string
		want   string
	}{
		{"bound to a step", "wp-bound", "step-1"},
		{"empty step", "wp-unbound", ""},
		{"missing task", "wp-missing", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.GetTaskWorkflowStepID(ctx, tc.taskID)
			if err != nil {
				t.Fatalf("GetTaskWorkflowStepID: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAddTaskParticipant_IsIdempotentPerNaturalKey(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "ap-1", "step-1")

	if err := repo.AddTaskParticipant(ctx, "ap-1", "agent-r", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	got, err := repo.ListTaskParticipants(ctx, "ap-1", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("participants = %+v, want 1", got)
	}
	want := sqlite.Participant{TaskID: "ap-1", AgentProfileID: "agent-r", Role: "reviewer", DecisionRequired: true}
	if got[0] != want {
		t.Errorf("participant = %+v, want %+v", got[0], want)
	}

	// A second identical call adds no row.
	if err := repo.AddTaskParticipant(ctx, "ap-1", "agent-r", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant (repeat): %v", err)
	}
	if n := participantRowCount(t, repo, "ap-1"); n != 1 {
		t.Errorf("rows = %d, want 1 (idempotent)", n)
	}

	// A different role or agent is a distinct participant.
	if err := repo.AddTaskParticipant(ctx, "ap-1", "agent-r", "approver"); err != nil {
		t.Fatalf("AddTaskParticipant (other role): %v", err)
	}
	if err := repo.AddTaskParticipant(ctx, "ap-1", "agent-s", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant (other agent): %v", err)
	}
	if n := participantRowCount(t, repo, "ap-1"); n != 3 {
		t.Errorf("rows = %d, want 3", n)
	}
}

// seedAutoSeat inserts an engine auto-cast (provenance="auto") participant
// row directly, bypassing AddTaskParticipant, to simulate the on_enter
// ensure_participant_seat action having already run before a manual
// registration arrives.
func seedAutoSeat(t *testing.T, repo *sqlite.Repository, id, stepID, taskID, role, agentID string) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES (?, ?, ?, ?, ?, 1, 0, 'auto')
	`, id, stepID, taskID, role, agentID); err != nil {
		t.Fatalf("seed auto seat: %v", err)
	}
}

// TestAddTaskParticipant_ClaimsUndecidedAutoSeat covers the seat-provenance
// race: the engine's on_enter ensure_participant_seat action auto-casts a
// fallback reviewer, then a human manually registers a different reviewer
// moments later. Without claiming, this produces two seats for one role, so
// an all_approve quorum guard needing 2 decisions never resolves from a
// single human decision. The manual call must claim the undecided auto
// seat in place instead of inserting a second one.
func TestAddTaskParticipant_ClaimsUndecidedAutoSeat(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "ap-claim", "step-1")
	seedAutoSeat(t, repo, "auto-seat-1", "step-1", "ap-claim", "reviewer", "agent-auto")

	if err := repo.AddTaskParticipant(ctx, "ap-claim", "agent-human", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}

	if n := participantRowCount(t, repo, "ap-claim"); n != 1 {
		t.Fatalf("rows = %d, want 1 (claimed in place, not duplicated)", n)
	}
	got, err := repo.ListTaskParticipants(ctx, "ap-claim", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 1 || got[0].AgentProfileID != "agent-human" {
		t.Fatalf("participants = %+v, want single row claimed by agent-human", got)
	}

	var provenance string
	if err := repo.ReaderDB().Get(&provenance,
		`SELECT provenance FROM workflow_step_participants WHERE id = 'auto-seat-1'`); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if provenance != "manual" {
		t.Errorf("provenance = %q, want %q after claim", provenance, "manual")
	}
}

// TestAddTaskParticipant_DoesNotClaimADecidedAutoSeat: once the auto-cast
// seat already has a decision recorded against it, claiming would silently
// reassign a seat whose vote is already on the record — so a second manual
// registration must insert a distinct seat instead.
func TestAddTaskParticipant_DoesNotClaimADecidedAutoSeat(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "ap-decided", "step-1")
	seedAutoSeat(t, repo, "auto-seat-2", "step-1", "ap-decided", "reviewer", "agent-auto")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_decisions (id, task_id, step_id, participant_id, decision, decided_at)
		VALUES ('dec-1', 'ap-decided', 'step-1', 'auto-seat-2', 'approved', datetime('now'))
	`); err != nil {
		t.Fatalf("seed decision: %v", err)
	}

	if err := repo.AddTaskParticipant(ctx, "ap-decided", "agent-human", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}

	if n := participantRowCount(t, repo, "ap-decided"); n != 2 {
		t.Fatalf("rows = %d, want 2 (decided auto seat must not be claimed)", n)
	}
	var provenance string
	if err := repo.ReaderDB().Get(&provenance,
		`SELECT provenance FROM workflow_step_participants WHERE id = 'auto-seat-2'`); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if provenance != "auto" {
		t.Errorf("provenance = %q, want unchanged %q", provenance, "auto")
	}
}

// TestAddTaskParticipant_MultiReviewerManualSeatsUntouched preserves the
// legitimate multi-reviewer scenario TestAddTaskParticipant_IsIdempotentPerNaturalKey
// already covers for distinct agents: two manually-added seats (both
// provenance="manual") for the same role are never collapsed by the claim
// logic, since neither carries provenance="auto".
func TestAddTaskParticipant_MultiReviewerManualSeatsUntouched(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "ap-multi", "step-1")

	if err := repo.AddTaskParticipant(ctx, "ap-multi", "agent-one", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant (first): %v", err)
	}
	if err := repo.AddTaskParticipant(ctx, "ap-multi", "agent-two", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant (second): %v", err)
	}

	if n := participantRowCount(t, repo, "ap-multi"); n != 2 {
		t.Fatalf("rows = %d, want 2 distinct manual reviewer seats", n)
	}
}

// A task with no workflow step cannot own a participant row, so the write
// is silently skipped rather than erroring.
func TestAddTaskParticipant_SkipsTasksWithoutAStep(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "ap-nostep", "")

	if err := repo.AddTaskParticipant(ctx, "ap-nostep", "agent-r", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant = %v, want nil", err)
	}
	if n := participantRowCount(t, repo, "ap-nostep"); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}

	// Same for a task that does not exist at all.
	if err := repo.AddTaskParticipant(ctx, "ap-ghost", "agent-r", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant(missing task) = %v, want nil", err)
	}
	if n := participantRowCount(t, repo, "ap-ghost"); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}
}

func TestRemoveTaskParticipant(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "rp-1", "step-1")
	for _, spec := range []struct{ agent, role string }{
		{"agent-r", "reviewer"},
		{"agent-r", "approver"},
		{"agent-s", "reviewer"},
	} {
		if err := repo.AddTaskParticipant(ctx, "rp-1", spec.agent, spec.role); err != nil {
			t.Fatalf("AddTaskParticipant(%v): %v", spec, err)
		}
	}
	// A template-level row for the same step must survive per-task removal.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('tpl-row', 'step-1', '', 'reviewer', 'agent-r', 0, 0)
	`); err != nil {
		t.Fatalf("seed template row: %v", err)
	}

	if err := repo.RemoveTaskParticipant(ctx, "rp-1", "agent-r", "reviewer"); err != nil {
		t.Fatalf("RemoveTaskParticipant: %v", err)
	}
	if n := participantRowCount(t, repo, "rp-1"); n != 2 {
		t.Errorf("per-task rows = %d, want 2", n)
	}
	var templateRows int
	if err := repo.ReaderDB().Get(&templateRows,
		`SELECT COUNT(*) FROM workflow_step_participants WHERE task_id = ''`); err != nil {
		t.Fatalf("count template rows: %v", err)
	}
	if templateRows != 1 {
		t.Errorf("template rows = %d, want the template row untouched", templateRows)
	}

	// Removing a row that is not there is not an error.
	if err := repo.RemoveTaskParticipant(ctx, "rp-1", "agent-nobody", "reviewer"); err != nil {
		t.Fatalf("RemoveTaskParticipant(missing) = %v, want nil", err)
	}
	if n := participantRowCount(t, repo, "rp-1"); n != 2 {
		t.Errorf("per-task rows = %d, want 2 after a no-op delete", n)
	}
}

func TestListTaskParticipants_FiltersRoleAndMergesTemplateRows(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "lp-1", "step-1")
	seedParticipantTask(t, repo, "lp-other", "step-2")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			-- template rows for the same step
			('t-rev-shared', 'step-1', '',       'reviewer', 'agent-shared', 0, 0),
			('t-rev-only',   'step-1', '',       'reviewer', 'agent-tpl',    0, 2),
			('t-app',        'step-1', '',       'approver', 'agent-app',    0, 0),
			-- per-task row that overrides the shared template entry
			('z-rev-shared', 'step-1', 'lp-1',   'reviewer', 'agent-shared', 1, 1),
			-- another task's row on a different step
			('o-rev',        'step-2', 'lp-other','reviewer','agent-other',  1, 0)
	`); err != nil {
		t.Fatalf("seed participants: %v", err)
	}

	got, err := repo.ListTaskParticipants(ctx, "lp-1", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("participants = %+v, want 2 (shared merged, approver filtered out)", got)
	}
	// position ASC then id ASC: the template shared row (pos 0) is seen
	// first and then replaced in place by the per-task row.
	if got[0].AgentProfileID != "agent-shared" || !got[0].DecisionRequired {
		t.Errorf("first = %+v, want agent-shared with the per-task decision_required", got[0])
	}
	if got[1].AgentProfileID != "agent-tpl" || got[1].DecisionRequired {
		t.Errorf("second = %+v, want the template-only agent-tpl", got[1])
	}
	for _, p := range got {
		if p.TaskID != "lp-1" {
			t.Errorf("TaskID = %q, want every row projected onto lp-1", p.TaskID)
		}
		if p.Role != "reviewer" {
			t.Errorf("Role = %q, want reviewer", p.Role)
		}
		if !p.CreatedAt.IsZero() {
			t.Errorf("CreatedAt = %v, want the zero time (not stored on the step table)", p.CreatedAt)
		}
	}

	approvers, err := repo.ListTaskParticipants(ctx, "lp-1", "approver")
	if err != nil {
		t.Fatalf("ListTaskParticipants(approver): %v", err)
	}
	if len(approvers) != 1 || approvers[0].AgentProfileID != "agent-app" {
		t.Errorf("approvers = %+v, want just agent-app", approvers)
	}

	unknown, err := repo.ListTaskParticipants(ctx, "lp-1", "nosuchrole")
	if err != nil {
		t.Fatalf("ListTaskParticipants(unknown role): %v", err)
	}
	if unknown == nil {
		t.Fatal("returned nil slice, want empty non-nil")
	}
	if len(unknown) != 0 {
		t.Errorf("participants = %+v, want none", unknown)
	}
}

func TestListTaskParticipants_TaskWithoutAStepIsEmpty(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "lp-nostep", "")
	// A row keyed on the empty step must not leak into an unstepped task.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('n-1', '', 'lp-nostep', 'reviewer', 'agent-r', 1, 0)
	`); err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	got, err := repo.ListTaskParticipants(ctx, "lp-nostep", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("participants = %+v, want none for a task with no workflow step", got)
	}

	all, err := repo.ListAllTaskParticipants(ctx, "lp-nostep")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("participants = %+v, want none", all)
	}
}

func TestListAllTaskParticipants_SpansRoles(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "la-1", "step-1")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('r-1', 'step-1', 'la-1', 'reviewer', 'agent-rev', 1, 0),
			('a-1', 'step-1', 'la-1', 'approver', 'agent-app', 0, 0),
			('n-1', 'step-1', 'la-1', 'runner',   'agent-run', 0, 0)
	`); err != nil {
		t.Fatalf("seed participants: %v", err)
	}

	got, err := repo.ListAllTaskParticipants(ctx, "la-1")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("participants = %+v, want all three roles", got)
	}
	// ORDER BY role ASC.
	roles := []string{got[0].Role, got[1].Role, got[2].Role}
	if roles[0] != "approver" || roles[1] != "reviewer" || roles[2] != "runner" {
		t.Errorf("roles = %v, want approver,reviewer,runner", roles)
	}
	if !got[1].DecisionRequired {
		t.Error("reviewer DecisionRequired = false, want true")
	}
	if got[0].DecisionRequired {
		t.Error("approver DecisionRequired = true, want false")
	}
}

func participantRowCount(t *testing.T, repo *sqlite.Repository, taskID string) int {
	t.Helper()
	var n int
	if err := repo.ReaderDB().Get(&n,
		`SELECT COUNT(*) FROM workflow_step_participants WHERE task_id = ?`, taskID); err != nil {
		t.Fatalf("count participants: %v", err)
	}
	return n
}
