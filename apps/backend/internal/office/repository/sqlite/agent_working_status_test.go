package sqlite_test

import (
	"context"
	"testing"

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

// TestMarkAgentWorking_FromIdle is the core DR-14 write: before this method
// existed, no production code path could ever produce this status.
func TestMarkAgentWorking_FromIdle(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-idle-to-working", "idle")

	changed, err := repo.MarkAgentWorking(ctx, "agent-idle-to-working")
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
	repo := newTestRepo(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-working-to-idle", "working")

	changed, err := repo.ClearAgentWorking(ctx, "agent-working-to-idle")
	if err != nil {
		t.Fatalf("clear working: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true for a working agent")
	}
	if got := statusOf(t, repo, "agent-working-to-idle"); got != models.AgentStatusIdle {
		t.Fatalf("status = %q, want %q", got, models.AgentStatusIdle)
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

		changed, err := repo.ClearAgentWorking(ctx, id)
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

	changed, err := repo.MarkAgentWorking(ctx, "agent-paused-at-launch")
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
	repo := newTestRepo(t)
	ctx := context.Background()
	workingStatusAgent(t, repo, "agent-double-clear", "working")

	if _, err := repo.ClearAgentWorking(ctx, "agent-double-clear"); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	changed, err := repo.ClearAgentWorking(ctx, "agent-double-clear")
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
