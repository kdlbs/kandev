package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// AC-OFFICE-LOOP-LIVENESS-002.7/.8/.10: SetRunSessionID persists the
// session id a launch produced, is a no-op on an empty id (so a caller
// can distinguish "wrote nothing" from a real write), and — given the
// single-launch-in-flight invariant — an unconditional non-empty write
// is "last non-empty wins".
func TestSetRunSessionID_PersistsNonEmptyID(t *testing.T) {
	repo := newTestRepo(t)
	run := &models.Run{ID: "run-session-1", AgentProfileID: "agent-1", Status: "claimed"}
	mustCreateRun(t, repo, run)

	wrote, err := repo.SetRunSessionID(context.Background(), run.ID, "sess-abc")
	if err != nil {
		t.Fatalf("SetRunSessionID: %v", err)
	}
	if !wrote {
		t.Fatal("expected wrote=true for existing run with non-empty session id")
	}

	got := mustGetRun(t, repo, run.ID)
	if got.SessionID != "sess-abc" {
		t.Fatalf("session_id = %q, want sess-abc", got.SessionID)
	}
}

func TestSetRunSessionID_EmptyIDIsNoop(t *testing.T) {
	repo := newTestRepo(t)
	run := &models.Run{ID: "run-session-2", AgentProfileID: "agent-1", Status: "claimed", SessionID: "sess-existing"}
	mustCreateRun(t, repo, run)

	wrote, err := repo.SetRunSessionID(context.Background(), run.ID, "")
	if err != nil {
		t.Fatalf("SetRunSessionID: %v", err)
	}
	if wrote {
		t.Fatal("expected wrote=false for empty session id")
	}

	got := mustGetRun(t, repo, run.ID)
	if got.SessionID != "sess-existing" {
		t.Fatalf("session_id = %q, want unchanged sess-existing", got.SessionID)
	}
}

func TestSetRunSessionID_UnknownRunIsNoop(t *testing.T) {
	repo := newTestRepo(t)

	wrote, err := repo.SetRunSessionID(context.Background(), "run-does-not-exist", "sess-x")
	if err != nil {
		t.Fatalf("SetRunSessionID: %v", err)
	}
	if wrote {
		t.Fatal("expected wrote=false for unknown run id")
	}
}

// A cancel that lands between claim and this write already recorded a
// terminal shape off the run's session id at that instant; letting this
// write land anyway would make the persisted row disagree with the
// classification that was already counted for it.
func TestSetRunSessionID_LostRaceAgainstConcurrentCancelIsNoop(t *testing.T) {
	repo := newTestRepo(t)
	run := &models.Run{ID: "run-session-4", AgentProfileID: "agent-1", Status: "claimed"}
	mustCreateRun(t, repo, run)

	cancelled, err := repo.CancelRun(context.Background(), run.ID, "lost_race")
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if !cancelled {
		t.Fatal("expected CancelRun to cancel the still-claimed run")
	}

	wrote, err := repo.SetRunSessionID(context.Background(), run.ID, "sess-late")
	if err != nil {
		t.Fatalf("SetRunSessionID: %v", err)
	}
	if wrote {
		t.Fatal("expected wrote=false once the run is no longer claimed")
	}

	got := mustGetRun(t, repo, run.ID)
	if got.SessionID != "" {
		t.Fatalf("session_id = %q, want unchanged empty string on the cancelled row", got.SessionID)
	}
	if got.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
}

// AC-OFFICE-LOOP-LIVENESS-002.10: a later, non-empty write overwrites an
// earlier one — "last non-empty wins" for the relaunch-keeps-latest case.
func TestSetRunSessionID_LaterWriteOverwritesEarlier(t *testing.T) {
	repo := newTestRepo(t)
	run := &models.Run{ID: "run-session-3", AgentProfileID: "agent-1", Status: "claimed"}
	mustCreateRun(t, repo, run)

	if _, err := repo.SetRunSessionID(context.Background(), run.ID, "sess-first"); err != nil {
		t.Fatalf("first SetRunSessionID: %v", err)
	}
	if _, err := repo.SetRunSessionID(context.Background(), run.ID, "sess-second"); err != nil {
		t.Fatalf("second SetRunSessionID: %v", err)
	}

	got := mustGetRun(t, repo, run.ID)
	if got.SessionID != "sess-second" {
		t.Fatalf("session_id = %q, want sess-second", got.SessionID)
	}
}
