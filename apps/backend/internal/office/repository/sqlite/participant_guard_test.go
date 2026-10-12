package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seatProvenanceAndAgent reads one seat's agent profile and provenance
// directly off the row, so a case can prove a seat was left alone rather
// than trusting the office-side projection, which carries neither field.
func seatProvenanceAndAgent(t *testing.T, repo *sqlite.Repository, seatID string) (agentID, provenance string) {
	t.Helper()
	row := repo.ReaderDB().QueryRow(
		`SELECT agent_profile_id, provenance FROM workflow_step_participants WHERE id = ?`, seatID)
	if err := row.Scan(&agentID, &provenance); err != nil {
		t.Fatalf("read seat %s: %v", seatID, err)
	}
	return agentID, provenance
}

// TestAddTaskParticipant_RejectsEmptyAgentProfileID covers
// AC-OFFICE-SEAT-ASSURANCE-001.1 and -001.2. The store refuses a
// registration naming no agent, writing no seat, claiming no seat and
// promoting no seat's provenance. The refusal is a sentinel the caller can
// match on, which is what distinguishes it from the nil-error "unchanged"
// the store reports for a task at no step or no task at all.
func TestAddTaskParticipant_RejectsEmptyAgentProfileID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "guard-empty", "step-1")
	seedAutoSeat(t, repo, "auto-guard-1", "step-1", "guard-empty", "reviewer", "agent-auto")

	result, err := repo.AddTaskParticipant(ctx, "guard-empty", "", "reviewer")
	if !errors.Is(err, sqlite.ErrEmptyAgentProfileID) {
		t.Fatalf("AddTaskParticipant error = %v, want ErrEmptyAgentProfileID", err)
	}
	if result != (sqlite.ParticipantWriteResult{}) {
		t.Fatalf("result = %+v, want zero value", result)
	}

	// No seat written: the slate still holds exactly the seeded auto seat.
	if n := participantRowCount(t, repo, "guard-empty"); n != 1 {
		t.Fatalf("rows = %d, want 1 (no seat written)", n)
	}
	// No seat claimed and no provenance promoted.
	agentID, provenance := seatProvenanceAndAgent(t, repo, "auto-guard-1")
	if agentID != "agent-auto" {
		t.Fatalf("seat agent = %q, want agent-auto (not claimed)", agentID)
	}
	if provenance != "auto" {
		t.Fatalf("seat provenance = %q, want auto (not promoted)", provenance)
	}
}

// TestAddTaskParticipant_EmptyAgentRejectionIsDeterministic covers
// AC-OFFICE-SEAT-ASSURANCE-001.4: the rejection carries no state, so the
// same call rejected twice yields the same failure and the store neither
// retries it nor requires a caller to.
func TestAddTaskParticipant_EmptyAgentRejectionIsDeterministic(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "guard-twice", "step-1")
	seedAutoSeat(t, repo, "auto-guard-2", "step-1", "guard-twice", "reviewer", "agent-auto")

	for i := range 2 {
		result, err := repo.AddTaskParticipant(ctx, "guard-twice", "", "reviewer")
		if !errors.Is(err, sqlite.ErrEmptyAgentProfileID) {
			t.Fatalf("call %d error = %v, want ErrEmptyAgentProfileID", i+1, err)
		}
		if result != (sqlite.ParticipantWriteResult{}) {
			t.Fatalf("call %d result = %+v, want zero value", i+1, result)
		}
	}

	if n := participantRowCount(t, repo, "guard-twice"); n != 1 {
		t.Fatalf("rows = %d, want 1 (no seat written by either call)", n)
	}
	agentID, provenance := seatProvenanceAndAgent(t, repo, "auto-guard-2")
	if agentID != "agent-auto" || provenance != "auto" {
		t.Fatalf("seat = (%q, %q), want (agent-auto, auto)", agentID, provenance)
	}
}

// TestAddTaskParticipant_EmptyAgentGuardRunsBeforeTransaction covers
// AC-OFFICE-SEAT-ASSURANCE-001.3. Closing the database makes any transaction
// attempt fail, so the sentinel proves that the guard returns before the
// store touches the database.
func TestAddTaskParticipant_EmptyAgentGuardRunsBeforeTransaction(t *testing.T) {
	repo := newSearchTestRepo(t)
	if err := repo.ReaderDB().Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	result, err := repo.AddTaskParticipant(context.Background(), "guard-closed", "", "reviewer")
	if !errors.Is(err, sqlite.ErrEmptyAgentProfileID) {
		t.Fatalf("AddTaskParticipant error = %v, want ErrEmptyAgentProfileID", err)
	}
	if result != (sqlite.ParticipantWriteResult{}) {
		t.Fatalf("result = %+v, want zero value", result)
	}
}

// TestAddTaskParticipant_WhitespaceAgentIsNotRejectedByGuard covers
// AC-OFFICE-SEAT-ASSURANCE-001.5. Only the empty identifier is this
// guard's business. A whitespace identifier names no agent profile and
// stays governed by AC-OFFICE-SEAT-PROVENANCE-005.8: it claims no seat and
// displaces no auto seat. Pinning this keeps the store's guard and the
// registration surface's from drifting about what "empty" means — a guard
// that trimmed would reject a value the surface accepts.
func TestAddTaskParticipant_WhitespaceAgentIsNotRejectedByGuard(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "guard-ws", "step-1")
	seedAutoSeat(t, repo, "auto-guard-3", "step-1", "guard-ws", "reviewer", "agent-auto")

	if _, err := repo.AddTaskParticipant(ctx, "guard-ws", " ", "reviewer"); err != nil {
		t.Fatalf("AddTaskParticipant(%q) error = %v, want no error: only the empty identifier is rejected", " ", err)
	}

	// -005.8 still governs: the auto seat is neither claimed nor promoted.
	agentID, provenance := seatProvenanceAndAgent(t, repo, "auto-guard-3")
	if agentID != "agent-auto" {
		t.Fatalf("seat agent = %q, want agent-auto (whitespace agent cannot claim)", agentID)
	}
	if provenance != "auto" {
		t.Fatalf("seat provenance = %q, want auto (not promoted)", provenance)
	}
}

// TestAddTaskParticipant_EmptyAgentDistinguishableFromUnchanged covers
// AC-OFFICE-SEAT-ASSURANCE-001.2's second sentence. The store reports a
// task standing at no step, and a task that does not exist, as Unchanged
// with a nil error (AC-OFFICE-SEAT-PROVENANCE-005.2, -005.6). The empty
// identifier must not land in that same bucket, or a caller could not tell
// "you called wrong" from "nothing to do" without matching a message.
func TestAddTaskParticipant_EmptyAgentDistinguishableFromUnchanged(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "guard-nostep", "")
	seedParticipantAgent(t, repo, "agent-real")

	// Task at no step: Unchanged, nil error.
	result, err := repo.AddTaskParticipant(ctx, "guard-nostep", "agent-real", "reviewer")
	if err != nil {
		t.Fatalf("no-step task error = %v, want nil", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeUnchanged {
		t.Fatalf("no-step outcome = %v, want Unchanged", result.Outcome)
	}

	// Task that does not exist: Unchanged, nil error.
	result, err = repo.AddTaskParticipant(ctx, "guard-absent", "agent-real", "reviewer")
	if err != nil {
		t.Fatalf("absent task error = %v, want nil", err)
	}
	if result.Outcome != sqlite.ParticipantWriteOutcomeUnchanged {
		t.Fatalf("absent task outcome = %v, want Unchanged", result.Outcome)
	}

	// Empty identifier: a sentinel failure, not Unchanged.
	_, err = repo.AddTaskParticipant(ctx, "guard-nostep", "", "reviewer")
	if !errors.Is(err, sqlite.ErrEmptyAgentProfileID) {
		t.Fatalf("empty agent error = %v, want ErrEmptyAgentProfileID", err)
	}
}
