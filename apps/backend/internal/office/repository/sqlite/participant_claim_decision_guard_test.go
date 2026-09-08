package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// claimAutoSeat's UPDATE carries a NOT EXISTS condition over decisions against
// the seat it is reassigning. That condition can only add something the
// selection did not already provide when a decision commits between the two
// statements, and the seat exclusion closes that window for every decision
// carrying a role. A roleless decision takes neither the exclusion nor the
// seat validation, so for that path the condition is the whole defense.
//
// Racing real goroutines there would reach the window but is not forced to,
// so such a test would pass most of the time by never entering it. These
// tests use the package's yield point instead.

// seedRolelessDecision writes a decision against seatID carrying no
// participant role, through the supplied transaction.
func seedRolelessDecision(t *testing.T, ctx context.Context, tx *sqlx.Tx, id, taskID, stepID, seatID string) {
	t.Helper()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO workflow_step_decisions
			(id, task_id, step_id, participant_id, decision, decided_at)
		VALUES (?, ?, ?, ?, 'approve', '2026-01-01 00:00:00')
	`), id, taskID, stepID, seatID); err != nil {
		t.Fatalf("seed roleless decision: %v", err)
	}
}

type seatRow struct {
	AgentProfileID   string `db:"agent_profile_id"`
	Provenance       string `db:"provenance"`
	DecisionRequired int    `db:"decision_required"`
	Position         int    `db:"position"`
	CreatedAt        string `db:"created_at"`
}

func readSeat(t *testing.T, repo *sqlite.Repository, seatID string) seatRow {
	t.Helper()
	var got seatRow
	if err := repo.ReaderDB().Get(&got, `
		SELECT agent_profile_id, provenance, decision_required, position,
		       COALESCE(created_at, '') AS created_at
		FROM workflow_step_participants WHERE id = ?
	`, seatID); err != nil {
		t.Fatalf("read seat %s: %v", seatID, err)
	}
	return got
}

// TestAddTaskParticipant_ClaimDecisionGuard_RolelessDecisionBlocksReassignment
// is AC-OFFICE-SEAT-GUARD-001.2 and -002.1: a decision that exists against the
// selected seat by the time the reassigning update runs must stop the claim,
// leaving the registration to write its own seat instead.
//
// Removing the NOT EXISTS condition from claimAutoSeat must make this fail.
func TestAddTaskParticipant_ClaimDecisionGuard_RolelessDecisionBlocksReassignment(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "guard-task", "guard-step")
	seedAutoSeat(t, repo, "guard-seat", "guard-step", "guard-task", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-registering")

	before := readSeat(t, repo, "guard-seat")

	restore := sqlite.SetClaimWindowHook(func(hookCtx context.Context, tx *sqlx.Tx, seatID string) {
		if seatID != "guard-seat" {
			return
		}
		seedRolelessDecision(t, hookCtx, tx, "guard-decision", "guard-task", "guard-step", seatID)
	})
	defer restore()

	result, err := repo.AddTaskParticipant(ctx, "guard-task", "agent-registering", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant must report success (AC-OFFICE-SEAT-GUARD-001.6): %v", err)
	}

	// AC-OFFICE-SEAT-GUARD-001.5: indistinguishable from finding no claimable
	// seat. Nothing was displaced, so nothing is reported as displaced.
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Errorf("outcome = %v, want Inserted", result.Outcome)
	}
	if result.DisplacedAgentProfileID != "" {
		t.Errorf("displaced agent = %q, want empty", result.DisplacedAgentProfileID)
	}

	// AC-OFFICE-SEAT-GUARD-001.4: the decided seat is untouched in every field.
	after := readSeat(t, repo, "guard-seat")
	if after != before {
		t.Errorf("decided seat mutated:\n got %+v\nwant %+v", after, before)
	}

	// AC-OFFICE-SEAT-GUARD-001.2: the registering agent still gets a seat.
	seats, err := repo.ListTaskParticipants(ctx, "guard-task", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(seats) != 2 {
		t.Fatalf("reviewer seats = %d, want 2 (the decided one plus a fresh one)", len(seats))
	}
	found := false
	for _, s := range seats {
		if s.AgentProfileID == "agent-registering" {
			found = true
		}
	}
	if !found {
		t.Error("the registering agent was left unseated")
	}
}

// AC-OFFICE-SEAT-GUARD-001.8: a superseded decision blocks the reassignment
// exactly as an active one does. Superseded rows are the audit trail of a
// reworked review, and a seat that has ever been decided carries an
// attribution history that reassigning it would falsify.
func TestAddTaskParticipant_ClaimDecisionGuard_SupersededDecisionAlsoBlocks(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "sup-task", "sup-step")
	seedAutoSeat(t, repo, "sup-seat", "sup-step", "sup-task", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-registering")

	restore := sqlite.SetClaimWindowHook(func(hookCtx context.Context, tx *sqlx.Tx, seatID string) {
		if seatID != "sup-seat" {
			return
		}
		if _, err := tx.ExecContext(hookCtx, tx.Rebind(`
			INSERT INTO workflow_step_decisions
				(id, task_id, step_id, participant_id, decision, decided_at, superseded_at)
			VALUES (?, ?, ?, ?, 'approve', '2026-01-01 00:00:00', '2026-01-02 00:00:00')
		`), "sup-decision", "sup-task", "sup-step", seatID); err != nil {
			t.Fatalf("seed superseded decision: %v", err)
		}
	})
	defer restore()

	result, err := repo.AddTaskParticipant(ctx, "sup-task", "agent-registering", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Errorf("outcome = %v, want Inserted: a superseded decision must block too", result.Outcome)
	}
	if got := readSeat(t, repo, "sup-seat").AgentProfileID; got != "agent-auto" {
		t.Errorf("seat agent = %q, want agent-auto", got)
	}
}

// AC-OFFICE-SEAT-GUARD-001.9: the registering agent is seated by the seat it
// wrote, so repeating the registration is governed by the already-seated rule
// and writes nothing further. No retry and no second claim attempt.
func TestAddTaskParticipant_ClaimDecisionGuard_RepeatRegistrationWritesNothing(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "rep-task", "rep-step")
	seedAutoSeat(t, repo, "rep-seat", "rep-step", "rep-task", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-registering")

	restore := sqlite.SetClaimWindowHook(func(hookCtx context.Context, tx *sqlx.Tx, seatID string) {
		if seatID != "rep-seat" {
			return
		}
		seedRolelessDecision(t, hookCtx, tx, "rep-decision", "rep-task", "rep-step", seatID)
	})
	if _, err := repo.AddTaskParticipant(ctx, "rep-task", "agent-registering", "reviewer"); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	restore()

	countAfterFirst := participantRowCount(t, repo, "rep-task")
	result, err := repo.AddTaskParticipant(ctx, "rep-task", "agent-registering", "reviewer")
	if err != nil {
		t.Fatalf("repeat registration: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeUnchanged {
		t.Errorf("outcome = %v, want Unchanged", result.Outcome)
	}
	if got := participantRowCount(t, repo, "rep-task"); got != countAfterFirst {
		t.Errorf("rows = %d, want %d: the repeat must write nothing", got, countAfterFirst)
	}
}

// AC-OFFICE-SEAT-GUARD-002.3: with the hook unset the claim path behaves
// exactly as it does today, so an undecided seat is still claimed in place.
func TestAddTaskParticipant_ClaimDecisionGuard_HookUnsetLeavesClaimUnchanged(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "unset-task", "unset-step")
	seedAutoSeat(t, repo, "unset-seat", "unset-step", "unset-task", "reviewer", "agent-auto")
	seedParticipantAgent(t, repo, "agent-registering")

	result, err := repo.AddTaskParticipant(ctx, "unset-task", "agent-registering", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeClaimed {
		t.Fatalf("outcome = %v, want Claimed", result.Outcome)
	}
	if result.DisplacedAgentProfileID != "agent-auto" {
		t.Errorf("displaced = %q, want agent-auto", result.DisplacedAgentProfileID)
	}
	if n := participantRowCount(t, repo, "unset-task"); n != 1 {
		t.Errorf("rows = %d, want 1 (claimed in place)", n)
	}
}
