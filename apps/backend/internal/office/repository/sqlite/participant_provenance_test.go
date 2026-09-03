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

// AC-OFFICE-SEAT-PROVENANCE-004.8 ("the seat a registration selected to
// claim is removed before the claim is applied") is NOT the same scenario
// as a decision landing on the seat before the claim search runs — that is
// TestAddTaskParticipant_DoesNotClaimADecidedAutoSeat
// (participants_ops_test.go), where findClaimableAutoSeat's own decision
// check excludes the seat before any candidate is even selected, so
// claimAutoSeat's conditional UPDATE is never reached at all. A prior
// version of this file had a test with this AC's name that seeded a
// decision and asserted the fallthrough — that test never actually
// exercised claimAutoSeat's zero-rows-affected branch; it was a duplicate
// of the DoesNotClaimADecidedAutoSeat case above under the wrong name.
//
// AC-004.8's actual scenario — a candidate already selected by
// findClaimableAutoSeat, then vanishing before claimAutoSeat's UPDATE
// commits — cannot be forced deterministically against this single-writer
// SQLite pool: RemoveTaskParticipant takes no lock, but SQLite still
// serializes the whole transaction, so a second call can only run strictly
// before or strictly after AddTaskParticipant's transaction, never inside
// it. The real proof is
// TestPostgresAddTaskParticipant_ConvergesWithConcurrentRemoveOfClaimTarget
// (participant_claim_postgres_test.go): RemoveTaskParticipant does not
// acquire ParticipantRoleSeatLockKey, so on PostgreSQL it can commit its
// DELETE between AddTaskParticipant's SELECT and its conditional UPDATE
// under genuine cross-connection concurrency, hitting exactly the
// zero-rows-affected branch this AC protects.

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

// TestAddTaskParticipant_MultipleAutoSeatsClaimsNoneInsertsFresh is
// AC-OFFICE-SEAT-PROVENANCE-002.6: when the role slate holds more than one
// "auto" seat, the registration claims none of them and writes a new
// "manual" seat — the count alone decides, with no ordering or tiebreak.
// findClaimableAutoSeat's len(candidates) != 1 guard is the only production
// branch this exercises; before this test, every seeded scenario in this
// package used exactly one auto seat per slate.
func TestAddTaskParticipant_MultipleAutoSeatsClaimsNoneInsertsFresh(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "multi-auto-1", "step-1")
	seedAutoSeat(t, repo, "auto-multi-a", "step-1", "multi-auto-1", "reviewer", "agent-auto-a")
	seedAutoSeat(t, repo, "auto-multi-b", "step-1", "multi-auto-1", "reviewer", "agent-auto-b")
	seedParticipantAgent(t, repo, "agent-human")

	result, err := repo.AddTaskParticipant(ctx, "multi-auto-1", "agent-human", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q (two auto seats means neither is claimable)", result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if n := participantRowCount(t, repo, "multi-auto-1"); n != 3 {
		t.Fatalf("rows = %d, want 3 (both auto seats untouched, one new manual seat)", n)
	}

	for _, seatID := range []string{"auto-multi-a", "auto-multi-b"} {
		var provenance string
		if err := repo.ReaderDB().Get(&provenance,
			`SELECT provenance FROM workflow_step_participants WHERE id = ?`, seatID); err != nil {
			t.Fatalf("read provenance for %s: %v", seatID, err)
		}
		if provenance != "auto" {
			t.Errorf("seat %s provenance = %q, want unchanged %q", seatID, provenance, "auto")
		}
	}
}

// TestAddTaskParticipant_NonStandardProvenanceIsNotClaimable is
// AC-OFFICE-SEAT-PROVENANCE-005.3: a seat whose stored provenance is
// neither "auto" nor "manual" is not claimable, and reading it does not
// fail the read. findClaimableAutoSeat's positive `provenance = 'auto'`
// filter satisfies this by construction; this test pins that a row outside
// the two known values is inert rather than crashing the query or being
// silently treated as claimable.
func TestAddTaskParticipant_NonStandardProvenanceIsNotClaimable(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "legacy-prov-1", "step-1")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES ('legacy-seat', 'step-1', 'legacy-prov-1', 'reviewer', 'agent-legacy', 1, 0, 'legacy')
	`); err != nil {
		t.Fatalf("seed legacy-provenance seat: %v", err)
	}
	seedParticipantAgent(t, repo, "agent-human")

	result, err := repo.AddTaskParticipant(ctx, "legacy-prov-1", "agent-human", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Fatalf("outcome = %q, want %q (a non-auto, non-manual seat is not claimable)", result.Outcome, sqlite.ParticipantWriteOutcomeInserted)
	}
	if n := participantRowCount(t, repo, "legacy-prov-1"); n != 2 {
		t.Fatalf("rows = %d, want 2 (legacy seat untouched, one new manual seat)", n)
	}

	var provenance, agent string
	if err := repo.ReaderDB().Get(&provenance,
		`SELECT provenance FROM workflow_step_participants WHERE id = 'legacy-seat'`); err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if provenance != "legacy" {
		t.Errorf("legacy seat provenance = %q, want unchanged %q", provenance, "legacy")
	}
	if err := repo.ReaderDB().Get(&agent,
		`SELECT agent_profile_id FROM workflow_step_participants WHERE id = 'legacy-seat'`); err != nil {
		t.Fatalf("read agent: %v", err)
	}
	if agent != "agent-legacy" {
		t.Errorf("legacy seat agent = %q, want unchanged %q (not displaced)", agent, "agent-legacy")
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
