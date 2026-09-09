package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// workingStatusAgent creates a minimal office agent at the given status.
func workingStatusAgent(t *testing.T, repo *sqlite.Repository, id, status string) *models.AgentInstance {
	t.Helper()
	agent := &models.AgentInstance{
		ID:          id,
		WorkspaceID: "ws-working",
		Name:        id,
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatus(status),
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func statusOf(t *testing.T, repo *sqlite.Repository, id string) models.AgentStatus {
	t.Helper()
	got, err := repo.GetAgentInstance(context.Background(), id)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	return got.Status
}

// setWorkingRunID seeds the owning-run bookkeeping column directly, since
// CreateAgentInstance has no field for it (production writers are
// MarkAgentWorking/ClearAgentWorking only).
func setWorkingRunID(t *testing.T, db *sqlx.DB, agentID, runID string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE agent_profiles SET working_run_id = ? WHERE id = ?`, runID, agentID); err != nil {
		t.Fatalf("seed working_run_id: %v", err)
	}
}

// TestMarkAgentWorking_FromIdle is the core DR-14 write: before this method
// existed, no production code path could ever produce this status.
func TestMarkAgentWorking_FromIdle(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-idle-to-working", "idle")

	changed, err := repo.MarkAgentWorking(ctx, "agent-idle-to-working", "run-1")
	if err != nil {
		t.Fatalf("mark working: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true for an idle agent")
	}
	if got := statusOf(t, repo, "agent-idle-to-working"); got != models.AgentStatusWorking {
		t.Fatalf("status = %q, want %q", got, models.AgentStatusWorking)
	}
}

func TestClearAgentWorking_ReturnsToIdle(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-working-to-idle", "working")
	setWorkingRunID(t, db, "agent-working-to-idle", "run-1")

	changed, err := repo.ClearAgentWorking(ctx, "agent-working-to-idle", "run-1")
	if err != nil {
		t.Fatalf("clear working: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true for a working agent whose runID matches")
	}
	if got := statusOf(t, repo, "agent-working-to-idle"); got != models.AgentStatusIdle {
		t.Fatalf("status = %q, want %q", got, models.AgentStatusIdle)
	}
}

// TestClearAgentWorking_MismatchedRunID_DoesNotClobberSuccessor is the
// interleaving regression: a stale or duplicate terminal event for a run
// that has already finished must not be able to reset an agent a SUCCESSOR
// run has since marked working. Before working_run_id existed, this exact
// sequence flipped a live run's agent back to "idle" — reintroducing DR-14's
// invisibility bug in a narrower window.
func TestClearAgentWorking_MismatchedRunID_DoesNotClobberSuccessor(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-interleaved", "idle")

	// Run A launches and marks the agent working.
	if _, err := repo.MarkAgentWorking(ctx, "agent-interleaved", "run-a"); err != nil {
		t.Fatalf("mark working (run-a): %v", err)
	}

	// Run A finishes via another path (e.g. ReapStaleCheckouts), the agent
	// goes idle, and a successor Run B is claimed and launched for the same
	// agent before Run A's own (late/duplicate) terminal event arrives.
	if _, err := repo.ClearAgentWorking(ctx, "agent-interleaved", "run-a"); err != nil {
		t.Fatalf("clear working (run-a): %v", err)
	}
	if _, err := repo.MarkAgentWorking(ctx, "agent-interleaved", "run-b"); err != nil {
		t.Fatalf("mark working (run-b): %v", err)
	}
	if got := statusOf(t, repo, "agent-interleaved"); got != models.AgentStatusWorking {
		t.Fatalf("status after run-b launch = %q, want %q", got, models.AgentStatusWorking)
	}

	// Run A's stale/duplicate terminal event now arrives and tries to clear
	// using its own (no-longer-current) run id.
	changed, err := repo.ClearAgentWorking(ctx, "agent-interleaved", "run-a")
	if err != nil {
		t.Fatalf("stale clear (run-a): %v", err)
	}
	if changed {
		t.Fatal("stale clear reported a change: it must be a no-op once run-b owns \"working\"")
	}
	if got := statusOf(t, repo, "agent-interleaved"); got != models.AgentStatusWorking {
		t.Fatalf("status after stale run-a clear = %q, want %q (run-b still in flight)", got, models.AgentStatusWorking)
	}

	// Run B's own terminal event correctly clears it.
	changed, err = repo.ClearAgentWorking(ctx, "agent-interleaved", "run-b")
	if err != nil {
		t.Fatalf("clear working (run-b): %v", err)
	}
	if !changed {
		t.Fatal("run-b's own clear should have changed the status")
	}
	if got := statusOf(t, repo, "agent-interleaved"); got != models.AgentStatusIdle {
		t.Fatalf("status after run-b clear = %q, want %q", got, models.AgentStatusIdle)
	}
}

// TestClearAgentWorking_DoesNotResurrectPausedAgent guards the highest-risk
// interaction in DR-14. HandleAgentFailure clears "working" on the same code
// path that auto-pauses an agent after consecutive failures. If the reset
// were an unconditional UPDATE rather than a CAS, a paused agent would be
// silently flipped back to idle and would resume picking up runs.
func TestClearAgentWorking_DoesNotResurrectPausedAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, status := range []string{"paused", "stopped", "pending_approval"} {
		id := "agent-guard-" + status
		workingStatusAgent(t, repo, id, status)

		changed, err := repo.ClearAgentWorking(ctx, id, "run-1")
		if err != nil {
			t.Fatalf("%s: clear working: %v", status, err)
		}
		if changed {
			t.Fatalf("%s: changed = true, want false: only a working agent may be reset", status)
		}
		if got := statusOf(t, repo, id); string(got) != status {
			t.Fatalf("%s: status = %q, want it left untouched", status, got)
		}
	}
}

// TestMarkAgentWorking_SkipsNonIdleAgent covers the reverse guard: a user
// pausing an agent between the scheduler's isAgentActive check and the launch
// must not have that pause overwritten by the launch's status write.
func TestMarkAgentWorking_SkipsNonIdleAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-paused-at-launch", "paused")

	changed, err := repo.MarkAgentWorking(ctx, "agent-paused-at-launch", "run-1")
	if err != nil {
		t.Fatalf("mark working: %v", err)
	}
	if changed {
		t.Fatal("changed = true, want false: a paused agent must stay paused")
	}
	if got := statusOf(t, repo, "agent-paused-at-launch"); got != models.AgentStatusPaused {
		t.Fatalf("status = %q, want paused", got)
	}
}

// TestClearAgentWorking_IsIdempotent is what lets the reset be called from
// several terminal paths for the same run without ordering constraints.
func TestClearAgentWorking_IsIdempotent(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-double-clear", "working")
	setWorkingRunID(t, db, "agent-double-clear", "run-1")

	if _, err := repo.ClearAgentWorking(ctx, "agent-double-clear", "run-1"); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	changed, err := repo.ClearAgentWorking(ctx, "agent-double-clear", "run-1")
	if err != nil {
		t.Fatalf("second clear: %v", err)
	}
	if changed {
		t.Fatal("second clear reported a change, want a no-op")
	}
	if got := statusOf(t, repo, "agent-double-clear"); got != models.AgentStatusIdle {
		t.Fatalf("status = %q, want idle", got)
	}
}
