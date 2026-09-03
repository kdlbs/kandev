package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestAddTaskParticipant_ReportsOutcomeAndStep exercises the three outcomes
// AddTaskParticipant's result value carries (system-design "The
// registration writer reports its outcome"): a claim carries the step it
// landed at and the displaced agent; a plain insert carries the step and no
// displaced agent; a task with no step reports Unchanged with an empty
// step.
func TestAddTaskParticipant_ReportsOutcomeAndStep(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "out-claim", "step-1")
	seedAutoSeat(t, repo, "auto-out", "step-1", "out-claim", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-human")
	result, err := repo.AddTaskParticipant(ctx, "out-claim", "agent-human", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeClaimed {
		t.Fatalf("outcome = %q, want %q", result.Outcome, sqlite.ParticipantWriteOutcomeClaimed)
	}
	if result.StepID != "step-1" {
		t.Errorf("StepID = %q, want %q", result.StepID, "step-1")
	}
	if result.DisplacedAgentProfileID != "agent-auto" {
		t.Errorf("DisplacedAgentProfileID = %q, want %q", result.DisplacedAgentProfileID, "agent-auto")
	}

	seedParticipantTask(t, repo, "out-insert", "step-1")
	result, err = repo.AddTaskParticipant(ctx, "out-insert", "agent-plain", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q", result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if result.StepID != "step-1" {
		t.Errorf("StepID = %q, want %q", result.StepID, "step-1")
	}
	if result.DisplacedAgentProfileID != "" {
		t.Errorf("DisplacedAgentProfileID = %q, want empty on an insert", result.DisplacedAgentProfileID)
	}

	seedParticipantTask(t, repo, "out-nostep", "")
	result, err = repo.AddTaskParticipant(ctx, "out-nostep", "agent-plain", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeUnchanged {
		t.Fatalf("outcome = %q, want %q", result.Outcome, sqlite.ParticipantWriteOutcomeUnchanged)
	}
	if result.StepID != "" {
		t.Errorf("StepID = %q, want empty on unchanged", result.StepID)
	}
}

// TestAddTaskParticipant_UnknownAgentCannotDisplaceACastSeat is
// AC-OFFICE-SEAT-PROVENANCE-005.8: an agent profile id with no matching
// agent_profiles row must not be able to claim the sole undecided auto
// seat. It still gets its own manual seat written — 005.8 only forbids
// displacing a cast seat, not the registration succeeding.
func TestAddTaskParticipant_UnknownAgentCannotDisplaceACastSeat(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "unk-1", "step-1")
	seedAutoSeat(t, repo, "auto-unk", "step-1", "unk-1", "reviewer", "agent-auto")
	// Deliberately no seedParticipantAgent call for "agent-ghost".

	result, err := repo.AddTaskParticipant(ctx, "unk-1", "agent-ghost", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q (unknown agent still gets its own seat)", result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if n := participantRowCount(t, repo, "unk-1"); n != 2 {
		t.Fatalf("rows = %d, want 2 (the auto seat plus the unknown agent's own seat)", n)
	}
	var provenance, agent string
	if err := repo.ReaderDB().Get(&provenance,
		`SELECT provenance FROM workflow_step_participants WHERE id = 'auto-unk'`); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if provenance != "auto" {
		t.Errorf("auto seat provenance = %q, want unchanged %q", provenance, "auto")
	}
	if err := repo.ReaderDB().Get(&agent,
		`SELECT agent_profile_id FROM workflow_step_participants WHERE id = 'auto-unk'`); err != nil {
		t.Fatalf("read agent: %v", err)
	}
	if agent != "agent-auto" {
		t.Errorf("auto seat agent = %q, want unchanged %q (not displaced by an unknown agent)", agent, "agent-auto")
	}
}

// TestAddTaskParticipant_PromotesSoleUndecidedAutoSeatInPlace is
// AC-OFFICE-SEAT-PROVENANCE-002.3's promotion branch: when the named agent
// already holds the slate's sole undecided auto seat, its provenance flips
// to manual in place — no new row, no displaced agent, no activity-worthy
// claim.
func TestAddTaskParticipant_PromotesSoleUndecidedAutoSeatInPlace(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "promo-1", "step-1")
	seedAutoSeat(t, repo, "auto-promo", "step-1", "promo-1", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-auto")

	result, err := repo.AddTaskParticipant(ctx, "promo-1", "agent-auto", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeUnchanged {
		t.Fatalf("outcome = %q, want %q (a promotion is not a claim)", result.Outcome, sqlite.ParticipantWriteOutcomeUnchanged)
	}
	if n := participantRowCount(t, repo, "promo-1"); n != 1 {
		t.Fatalf("rows = %d, want 1 (promoted in place, not duplicated)", n)
	}
	var provenance string
	if err := repo.ReaderDB().Get(&provenance,
		`SELECT provenance FROM workflow_step_participants WHERE id = 'auto-promo'`); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if provenance != "manual" {
		t.Errorf("provenance = %q, want %q after promotion", provenance, "manual")
	}
}

// TestAddTaskParticipant_PromotionConvergencePair is the design's own
// "Testing" section worked example, and the one that actually catches a
// regression: from a slate holding one auto seat for agent A, registering
// A then B, and registering B then A, must both converge on two manual
// seats naming A and B. Before the promotion exists, the first ordering
// stops at one seat (B silently replaces A's confirmed choice instead of
// adding a second seat) — a test that only checks "promotion flips
// provenance" would pass against that regression; this pair does not.
func TestAddTaskParticipant_PromotionConvergencePair(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	seedParticipantAgent(t, repo, "agent-a")
	seedParticipantAgent(t, repo, "agent-b")

	t.Run("A then B", func(t *testing.T) {
		seedParticipantTask(t, repo, "conv-ab", "step-1")
		seedAutoSeat(t, repo, "auto-conv-ab", "step-1", "conv-ab", "reviewer", "agent-a")

		if _, err := repo.AddTaskParticipant(ctx, "conv-ab", "agent-a", "reviewer"); err != nil {
			t.Fatalf("AddTaskParticipant(A): %v", err)
		}
		if _, err := repo.AddTaskParticipant(ctx, "conv-ab", "agent-b", "reviewer"); err != nil {
			t.Fatalf("AddTaskParticipant(B): %v", err)
		}
		assertTwoManualSeatsNamingBoth(t, repo, "conv-ab")
	})

	t.Run("B then A", func(t *testing.T) {
		seedParticipantTask(t, repo, "conv-ba", "step-1")
		seedAutoSeat(t, repo, "auto-conv-ba", "step-1", "conv-ba", "reviewer", "agent-a")

		if _, err := repo.AddTaskParticipant(ctx, "conv-ba", "agent-b", "reviewer"); err != nil {
			t.Fatalf("AddTaskParticipant(B): %v", err)
		}
		if _, err := repo.AddTaskParticipant(ctx, "conv-ba", "agent-a", "reviewer"); err != nil {
			t.Fatalf("AddTaskParticipant(A): %v", err)
		}
		assertTwoManualSeatsNamingBoth(t, repo, "conv-ba")
	})
}

func assertTwoManualSeatsNamingBoth(t *testing.T, repo *sqlite.Repository, taskID string) {
	t.Helper()
	got, err := repo.ListTaskParticipants(context.Background(), taskID, "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("participants = %+v, want 2", got)
	}
	names := map[string]bool{}
	for _, p := range got {
		names[p.AgentProfileID] = true
	}
	if !names["agent-a"] || !names["agent-b"] {
		t.Fatalf("participants = %+v, want both agent-a and agent-b seated", got)
	}
	var provenances []string
	if err := repo.ReaderDB().Select(&provenances,
		`SELECT provenance FROM workflow_step_participants WHERE task_id = ?`, taskID); err != nil {
		t.Fatalf("read provenances: %v", err)
	}
	for _, p := range provenances {
		if p != "manual" {
			t.Errorf("provenance = %q, want every surviving seat to read manual", p)
		}
	}
}

// TestAddTaskParticipant_DecisionStoreReadFailureAbortsTheTransaction is
// AC-OFFICE-SEAT-PROVENANCE-005.4: when the decision store cannot be read
// while a claim (or the -002.3 promotion, which reuses the same read) is
// being evaluated, the registration must claim nothing, write nothing, and
// surface the failure — never fall through to inserting a second seat.
// Dropping workflow_step_decisions makes every read against it fail,
// standing in for a genuine storage error.
func TestAddTaskParticipant_DecisionStoreReadFailureAbortsTheTransaction(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "fail-1", "step-1")
	seedAutoSeat(t, repo, "auto-fail", "step-1", "fail-1", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-human")
	if _, err := repo.ExecRaw(ctx, `DROP TABLE workflow_step_decisions`); err != nil {
		t.Fatalf("drop workflow_step_decisions: %v", err)
	}

	_, err := repo.AddTaskParticipant(ctx, "fail-1", "agent-human", "reviewer")
	if err == nil {
		t.Fatal("AddTaskParticipant = nil error, want the decision-store read failure surfaced")
	}

	// The slate must be exactly as found: still one auto seat, unclaimed.
	if n := participantRowCount(t, repo, "fail-1"); n != 1 {
		t.Fatalf("rows = %d, want 1 (no partial write on failure)", n)
	}
	var provenance, agent string
	if err := repo.ReaderDB().Get(&provenance,
		`SELECT provenance FROM workflow_step_participants WHERE id = 'auto-fail'`); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if provenance != "auto" {
		t.Errorf("provenance = %q, want unchanged %q", provenance, "auto")
	}
	if err := repo.ReaderDB().Get(&agent,
		`SELECT agent_profile_id FROM workflow_step_participants WHERE id = 'auto-fail'`); err != nil {
		t.Fatalf("read agent: %v", err)
	}
	if agent != "agent-auto" {
		t.Errorf("agent = %q, want unchanged %q", agent, "agent-auto")
	}
}

// TestAddTaskParticipant_FallsThroughToInsertWhenClaimTargetVanishes is
// AC-OFFICE-SEAT-PROVENANCE-004.8: when the seat a registration selected to
// claim is removed before the claim is applied, the registration writes a
// new manual seat for its named agent rather than completing having
// written nothing.
func TestAddTaskParticipant_FallsThroughToInsertWhenClaimTargetVanishes(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "vanish-1", "step-1")
	seedAutoSeat(t, repo, "auto-vanish", "step-1", "vanish-1", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-human")

	// Simulate the claim target being decided (equivalent to removed, for
	// the conditional UPDATE's purposes: the row still exists but is no
	// longer claimable) after findClaimableAutoSeat's own upfront check
	// would have selected it. A recorded decision closes the same window
	// the conditional UPDATE guards against a genuine removal.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_decisions (id, task_id, step_id, participant_id, decision, decided_at)
		VALUES ('dec-vanish', 'vanish-1', 'step-1', 'auto-vanish', 'approved', datetime('now'))
	`); err != nil {
		t.Fatalf("seed decision: %v", err)
	}

	result, err := repo.AddTaskParticipant(ctx, "vanish-1", "agent-human", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q", result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if n := participantRowCount(t, repo, "vanish-1"); n != 2 {
		t.Fatalf("rows = %d, want 2 (the decided auto seat plus a fresh manual seat)", n)
	}
}

// TestAddTaskParticipant_DoesNotClaimAnAutoSeatAtAnEarlierStep is
// AC-OFFICE-SEAT-PROVENANCE-003.1/003.3: the claim search is scoped to the
// task's current step only, deliberately narrower than EnsureRoleSeat's
// any-step existence check. An auto seat cast at a step the task has since
// left must not be claimable; the registration writes a new manual seat at
// the current step instead, leaving the earlier auto seat untouched.
func TestAddTaskParticipant_DoesNotClaimAnAutoSeatAtAnEarlierStep(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "step-scope-1", "step-2") // task now stands at step-2
	seedAutoSeat(t, repo, "auto-step1", "step-1", "step-scope-1", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-human")

	result, err := repo.AddTaskParticipant(ctx, "step-scope-1", "agent-human", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q (the earlier-step auto seat is not claimable)", result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if n := participantRowCount(t, repo, "step-scope-1"); n != 2 {
		t.Fatalf("rows = %d, want 2 (earlier-step auto seat untouched, new manual seat at the current step)", n)
	}

	var provenance, agent string
	if err := repo.ReaderDB().Get(&provenance,
		`SELECT provenance FROM workflow_step_participants WHERE id = 'auto-step1'`); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if provenance != "auto" {
		t.Errorf("earlier-step seat provenance = %q, want unchanged %q", provenance, "auto")
	}
	if err := repo.ReaderDB().Get(&agent,
		`SELECT agent_profile_id FROM workflow_step_participants WHERE id = 'auto-step1'`); err != nil {
		t.Fatalf("read agent: %v", err)
	}
	if agent != "agent-auto" {
		t.Errorf("earlier-step seat agent = %q, want unchanged %q (not displaced)", agent, "agent-auto")
	}

	var newSeatStepID string
	if err := repo.ReaderDB().Get(&newSeatStepID,
		`SELECT step_id FROM workflow_step_participants WHERE task_id = 'step-scope-1' AND agent_profile_id = 'agent-human'`); err != nil {
		t.Fatalf("read new seat step: %v", err)
	}
	if newSeatStepID != "step-2" {
		t.Errorf("new manual seat step_id = %q, want %q (the task's current step)", newSeatStepID, "step-2")
	}
}

// TestCancelDisplacedParticipantRun_CancelsOnlyTheFanOutRunForThatAgentStep
// is AC-OFFICE-SEAT-PROVENANCE-006.6: a claim cancels the queued/claimed
// run the step-entry fan-out addressed to the displaced agent at that step,
// leaves an already-finished run alone, and does not touch a run for a
// different agent, task, step or reason.
func TestCancelDisplacedParticipantRun_CancelsOnlyTheFanOutRunForThatAgentStep(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedFanOutRun(t, repo, "run-target", "agent-displaced", "task-1", "step-1", "task_assigned", "queued")
	seedFanOutRun(t, repo, "run-other-agent", "agent-other", "task-1", "step-1", "task_assigned", "queued")
	seedFanOutRun(t, repo, "run-other-step", "agent-displaced", "task-1", "step-2", "task_assigned", "queued")
	seedFanOutRun(t, repo, "run-other-reason", "agent-displaced", "task-1", "step-1", "task_comment", "queued")
	seedFanOutRun(t, repo, "run-finished", "agent-displaced", "task-1", "step-1", "task_assigned", "queued")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE runs SET status = 'completed', finished_at = datetime('now') WHERE id = 'run-finished'`,
	); err != nil {
		t.Fatalf("finish run-finished: %v", err)
	}

	cancelled, err := repo.CancelDisplacedParticipantRun(ctx, "task-1", "step-1", "agent-displaced")
	if err != nil {
		t.Fatalf("CancelDisplacedParticipantRun: %v", err)
	}
	if cancelled != 1 {
		t.Fatalf("cancelled = %d, want 1", cancelled)
	}

	assertRunStatus(t, repo, "run-target", "cancelled")
	assertRunStatus(t, repo, "run-other-agent", "queued")
	assertRunStatus(t, repo, "run-other-step", "queued")
	assertRunStatus(t, repo, "run-other-reason", "queued")
	assertRunStatus(t, repo, "run-finished", "completed")
}

// TestCancelDisplacedParticipantRun_NoMatchIsNotAFailure covers the
// ordinary case a seat registered outside a step entry hits: the fan-out
// queued nothing for that agent, so the selector matches no run. Zero rows
// cancelled is a success like any other.
func TestCancelDisplacedParticipantRun_NoMatchIsNotAFailure(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	cancelled, err := repo.CancelDisplacedParticipantRun(ctx, "task-none", "step-none", "agent-none")
	if err != nil {
		t.Fatalf("CancelDisplacedParticipantRun: %v", err)
	}
	if cancelled != 0 {
		t.Errorf("cancelled = %d, want 0", cancelled)
	}
}

func seedFanOutRun(t *testing.T, repo *sqlite.Repository, id, agentID, taskID, stepID, reason, status string) {
	t.Helper()
	payload := `{"task_id":"` + taskID + `","workflow_step_id":"` + stepID + `"}`
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
	`, id, agentID, reason, payload, status); err != nil {
		t.Fatalf("seed run %s: %v", id, err)
	}
}

func assertRunStatus(t *testing.T, repo *sqlite.Repository, id, want string) {
	t.Helper()
	var got string
	if err := repo.ReaderDB().Get(&got, `SELECT status FROM runs WHERE id = ?`, id); err != nil {
		t.Fatalf("read run %s status: %v", id, err)
	}
	if got != want {
		t.Errorf("run %s status = %q, want %q", id, got, want)
	}
}
