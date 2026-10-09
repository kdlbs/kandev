package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// TestClaimNextEligibleRun_AgentCeilingClampedByWorkspaceCeiling pins
// AC-OFFICE-LAUNCH-SAFETY-001.9: an agent's own max_concurrent_sessions
// is clamped to the workspace (and instance) ceiling, so raising it on
// one agent cannot raise the real bound. A single agent in a workspace
// with its own cap set above the workspace ceiling must still be
// blocked once the workspace ceiling is reached, and the block must be
// attributed to agent_ceiling specifically: agentCeiling's clamp is what
// makes evalAgentCeilingGate the one that binds, ahead of the
// independent evalWorkspaceCeilingGate later in claimGateBlocks'
// precedence order. Without the clamp, agentCeiling returns the agent's
// raw (unclamped) cap, agent_ceiling never blocks, and the same claim
// attempt is instead attributed to workspace_ceiling — a passing suite
// either way unless a test isolates the attribution like this one does.
func TestClaimNextEligibleRun_AgentCeilingClampedByWorkspaceCeiling(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	const ws = "solo-ws-clamp"
	// The agent's own cap (10) is set well above the workspace ceiling
	// (2), with the instance ceiling (100) higher still, so only the
	// clamp inside agentCeiling can make agent_ceiling the gate that
	// binds.
	seedAgentInWorkspace(t, repo, "solo", ws, 10)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{MaxConcurrentWorkspace: 2, MaxConcurrentInstance: 100})

	first := queueRunInWorkspace(t, repo, "solo-first", "solo", ws, base)
	second := queueRunInWorkspace(t, repo, "solo-second", "solo", ws, base.Add(time.Minute))
	queueRunInWorkspace(t, repo, "solo-third", "solo", ws, base.Add(2*time.Minute))

	claimed1, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed1.ID != first.ID {
		t.Fatalf("first claim = %q, want %q", claimed1.ID, first.ID)
	}
	claimed2, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed2.ID != second.ID {
		t.Fatalf("second claim = %q, want %q", claimed2.ID, second.ID)
	}

	beforeAgent := deferredTotalForGate(t, "agent_ceiling")
	beforeWorkspace := deferredTotalForGate(t, "workspace_ceiling")

	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("third claim err = %v, want sql.ErrNoRows", err)
	}

	afterAgent := deferredTotalForGate(t, "agent_ceiling")
	afterWorkspace := deferredTotalForGate(t, "workspace_ceiling")

	if afterAgent-beforeAgent != 1 {
		t.Errorf("office_launch_deferred_total[agent_ceiling] increased by %d, want 1 "+
			"(the agent's cap of 10 must be clamped down to the workspace ceiling of 2, "+
			"so agent_ceiling — not workspace_ceiling — is the gate that blocks)",
			afterAgent-beforeAgent)
	}
	if afterWorkspace-beforeWorkspace != 0 {
		t.Errorf("office_launch_deferred_total[workspace_ceiling] increased by %d, want 0 "+
			"(agent_ceiling must block first, per claimGateBlocks' precedence, once the clamp is in effect)",
			afterWorkspace-beforeWorkspace)
	}
}

// TestClaimNextEligibleRun_AgentCeilingClampedByInstanceCeiling pins the
// other half of agentCeiling's clamp: minInt(maxSessions,
// minInt(MaxConcurrentWorkspace, MaxConcurrentInstance)) also clamps to
// the instance ceiling, not just the workspace ceiling. The workspace
// ceiling is set high enough to never bind, so only the instance term
// inside agentCeiling's own minInt can make agent_ceiling the gate that
// blocks. Without that term, agentCeiling would return the agent's raw
// cap, agent_ceiling would never block here, and the same claim attempt
// would instead be caught by the separate evalInstanceCeilingGate later
// in claimGateBlocks' precedence order — attributed to instance_ceiling
// instead.
func TestClaimNextEligibleRun_AgentCeilingClampedByInstanceCeiling(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	const ws = "solo-ws-clamp-instance"
	// The agent's own cap (10) and the workspace ceiling (100) are both
	// set well above the instance ceiling (2), so only the clamp inside
	// agentCeiling can make agent_ceiling the gate that binds.
	seedAgentInWorkspace(t, repo, "solo-instance", ws, 10)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{MaxConcurrentWorkspace: 100, MaxConcurrentInstance: 2})

	first := queueRunInWorkspace(t, repo, "solo-instance-first", "solo-instance", ws, base)
	second := queueRunInWorkspace(t, repo, "solo-instance-second", "solo-instance", ws, base.Add(time.Minute))
	queueRunInWorkspace(t, repo, "solo-instance-third", "solo-instance", ws, base.Add(2*time.Minute))

	claimed1, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed1.ID != first.ID {
		t.Fatalf("first claim = %q, want %q", claimed1.ID, first.ID)
	}
	claimed2, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed2.ID != second.ID {
		t.Fatalf("second claim = %q, want %q", claimed2.ID, second.ID)
	}

	beforeAgent := deferredTotalForGate(t, "agent_ceiling")
	beforeInstance := deferredTotalForGate(t, "instance_ceiling")

	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("third claim err = %v, want sql.ErrNoRows", err)
	}

	afterAgent := deferredTotalForGate(t, "agent_ceiling")
	afterInstance := deferredTotalForGate(t, "instance_ceiling")

	if afterAgent-beforeAgent != 1 {
		t.Errorf("office_launch_deferred_total[agent_ceiling] increased by %d, want 1 "+
			"(the agent's cap of 10 must be clamped down to the instance ceiling of 2, "+
			"so agent_ceiling — not instance_ceiling — is the gate that blocks)",
			afterAgent-beforeAgent)
	}
	if afterInstance-beforeInstance != 0 {
		t.Errorf("office_launch_deferred_total[instance_ceiling] increased by %d, want 0 "+
			"(agent_ceiling must block first, per claimGateBlocks' precedence, once the clamp is in effect)",
			afterInstance-beforeInstance)
	}
}
