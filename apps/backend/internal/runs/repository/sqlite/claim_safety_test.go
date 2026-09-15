package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// setAgentCap overrides the max_concurrent_sessions column seedClaimAgent
// left at its schema default (1), so ceiling tests can exercise a cap
// other than 1 without a dedicated seeding helper.
func setAgentCap(t *testing.T, repo *runssqlite.Repository, agentID string, cap int) {
	t.Helper()
	if _, err := repo.Writer().Exec(
		`UPDATE agent_profiles SET max_concurrent_sessions = ? WHERE id = ?`, cap, agentID,
	); err != nil {
		t.Fatalf("set agent cap for %q: %v", agentID, err)
	}
}

// countLedgerRows counts office_launch_ledger rows for a run, the durable
// record REQ-OFFICE-LAUNCH-SAFETY-002 requires every successful claim to
// append.
func countLedgerRows(t *testing.T, repo *runssqlite.Repository, runID string) int {
	t.Helper()
	var count int
	if err := repo.Writer().Get(&count,
		`SELECT COUNT(*) FROM office_launch_ledger WHERE run_id = ?`, runID,
	); err != nil {
		t.Fatalf("count ledger rows for %q: %v", runID, err)
	}
	return count
}

// TestClaimNextEligibleRun_AgentCeilingAllowsUpToConfiguredCap pins the
// per-agent ceiling to something other than the implicit cap=1 every other
// test in this package exercises: a1's own max_concurrent_sessions (2)
// must let a second claim through, but a third must defer.
func TestClaimNextEligibleRun_AgentCeilingAllowsUpToConfiguredCap(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedClaimAgent(t, repo, "a1")
	setAgentCap(t, repo, "a1", 2)

	first := queueRunAt(t, repo, "first", "a1", base)
	second := queueRunAt(t, repo, "second", "a1", base.Add(time.Minute))
	queueRunAt(t, repo, "third", "a1", base.Add(2*time.Minute))

	claimed1, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed1.ID != first.ID {
		t.Errorf("first claim = %q, want %q", claimed1.ID, first.ID)
	}
	claimed2, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed2.ID != second.ID {
		t.Errorf("second claim = %q, want %q", claimed2.ID, second.ID)
	}
	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("third claim err = %v, want sql.ErrNoRows (cap=2 already used)", err)
	}
}

// TestClaimNextEligibleRun_MissingAgentProfileDefers pins
// AC-OFFICE-LAUNCH-SAFETY-001.8: an agent id with no agent_profiles row
// must defer rather than being treated as unbounded.
func TestClaimNextEligibleRun_MissingAgentProfileDefers(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	queueRunAt(t, repo, "no-profile", "ghost-agent", time.Now().UTC())

	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows (missing agent_profiles row defers)", err)
	}
}

// TestClaimNextEligibleRun_NonPositiveAgentCapFloorsToOne pins the floor
// on a stored max_concurrent_sessions of zero or negative: unlike a
// missing agent_profiles row (unbounded-looking input, deferred), an
// explicit non-positive value is a configuration mistake reachable
// through the agent-facing modify-agent action and must still let the
// agent's queue drain at a cap of 1, not defer forever.
func TestClaimNextEligibleRun_NonPositiveAgentCapFloorsToOne(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedClaimAgent(t, repo, "a1")
	setAgentCap(t, repo, "a1", 0)

	first := queueRunAt(t, repo, "first", "a1", base)
	queueRunAt(t, repo, "second", "a1", base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v (want cap=0 floored to 1, not treated as unbounded-deferred)", err)
	}
	if claimed.ID != first.ID {
		t.Errorf("first claim = %q, want %q", claimed.ID, first.ID)
	}
	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second claim err = %v, want sql.ErrNoRows (floored cap=1 already used)", err)
	}
}

// TestClaimNextEligibleRun_WorkspaceCeilingDefersAcrossAgents pins the
// workspace-wide ceiling: two different agents sharing a workspace must
// still be capped by MaxConcurrentWorkspace even though neither agent is
// individually at capacity.
func TestClaimNextEligibleRun_WorkspaceCeilingDefersAcrossAgents(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const sharedWorkspace = "shared-ws"
	seedAgentInWorkspace(t, repo, "a1", sharedWorkspace, 5)
	seedAgentInWorkspace(t, repo, "a2", sharedWorkspace, 5)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{MaxConcurrentWorkspace: 1})

	first := queueRunInWorkspace(t, repo, "ws-first", "a1", sharedWorkspace, base)
	queueRunInWorkspace(t, repo, "ws-second", "a2", sharedWorkspace, base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed.ID != first.ID {
		t.Errorf("first claim = %q, want %q", claimed.ID, first.ID)
	}
	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second claim err = %v, want sql.ErrNoRows (workspace ceiling=1 already used)", err)
	}
}

// TestClaimNextEligibleRun_InstanceCeilingDefersAcrossWorkspaces pins the
// instance-wide ceiling as the broadest gate: it must bind even across
// unrelated workspaces and agents.
func TestClaimNextEligibleRun_InstanceCeilingDefersAcrossWorkspaces(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentInWorkspace(t, repo, "a1", "ws-1", 5)
	seedAgentInWorkspace(t, repo, "a2", "ws-2", 5)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{
		MaxConcurrentWorkspace: 5,
		MaxConcurrentInstance:  1,
	})

	first := queueRunInWorkspace(t, repo, "inst-first", "a1", "ws-1", base)
	queueRunInWorkspace(t, repo, "inst-second", "a2", "ws-2", base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed.ID != first.ID {
		t.Errorf("first claim = %q, want %q", claimed.ID, first.ID)
	}
	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second claim err = %v, want sql.ErrNoRows (instance ceiling=1 already used)", err)
	}
}

// TestClaimNextEligibleRun_WorkspaceBudgetDefersAfterHourlyLimit pins the
// rolling-hour workspace launch budget: it is counted from the durable
// office_launch_ledger, not runs.claimed_at, so it must still bind on a
// second run even though the two runs use different agents.
func TestClaimNextEligibleRun_WorkspaceBudgetDefersAfterHourlyLimit(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const ws = "budget-ws"
	seedAgentInWorkspace(t, repo, "a1", ws, 5)
	seedAgentInWorkspace(t, repo, "a2", ws, 5)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{
		MaxConcurrentWorkspace: 5,
		MaxConcurrentInstance:  5,
		WorkspaceBudgetPerHour: 1,
	})

	first := queueRunInWorkspace(t, repo, "budget-first", "a1", ws, base)
	queueRunInWorkspace(t, repo, "budget-second", "a2", ws, base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed.ID != first.ID {
		t.Errorf("first claim = %q, want %q", claimed.ID, first.ID)
	}
	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second claim err = %v, want sql.ErrNoRows (workspace budget=1/hr already used)", err)
	}
}

// TestClaimNextEligibleRun_RoutineBudgetDefersAfterHourlyLimit pins the
// per-routine budget, which is stricter than (and independent of) the
// workspace budget.
func TestClaimNextEligibleRun_RoutineBudgetDefersAfterHourlyLimit(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const ws = "routine-ws"
	const routineID = "routine-1"
	seedAgentInWorkspace(t, repo, "a1", ws, 5)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{
		MaxConcurrentWorkspace: 5,
		MaxConcurrentInstance:  5,
		WorkspaceBudgetPerHour: 100,
		RoutineBudgetPerHour:   1,
	})

	first := mustCreateRun(t, repo, &models.Run{
		ID:             "routine-first",
		AgentProfileID: "a1",
		WorkspaceID:    ws,
		RoutineID:      routineID,
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
	})
	setRequestedAt(t, repo, first.ID, base)
	mustCreateRun(t, repo, &models.Run{
		ID:             "routine-second",
		AgentProfileID: "a1",
		WorkspaceID:    ws,
		RoutineID:      routineID,
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
	})
	setRequestedAt(t, repo, "routine-second", base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed.ID != first.ID {
		t.Errorf("first claim = %q, want %q", claimed.ID, first.ID)
	}
	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second claim err = %v, want sql.ErrNoRows (routine budget=1/hr already used)", err)
	}
}

// TestClaimNextEligibleRun_HumanRootedRunsExemptFromWorkspaceBudget pins
// AC-OFFICE-LAUNCH-SAFETY-005.6: a human-rooted run tests its own
// human_rooted flag, not the exhausted workspace budget.
func TestClaimNextEligibleRun_HumanRootedRunsExemptFromWorkspaceBudget(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	const ws = "exempt-ws"
	seedAgentInWorkspace(t, repo, "a1", ws, 5)
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{
		MaxConcurrentWorkspace: 5,
		MaxConcurrentInstance:  5,
		WorkspaceBudgetPerHour: 1,
	})

	nonHuman := mustCreateRun(t, repo, &models.Run{
		ID:             "non-human",
		AgentProfileID: "a1",
		WorkspaceID:    ws,
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
		HumanRooted:    false,
	})
	setRequestedAt(t, repo, nonHuman.ID, base)
	human := mustCreateRun(t, repo, &models.Run{
		ID:             "human-rooted",
		AgentProfileID: "a1",
		WorkspaceID:    ws,
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
		HumanRooted:    true,
	})
	setRequestedAt(t, repo, human.ID, base.Add(time.Minute))

	claimed1, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claimed1.ID != nonHuman.ID {
		t.Errorf("first claim = %q, want %q", claimed1.ID, nonHuman.ID)
	}

	// The workspace budget (1/hr) is now exhausted by the non-human run,
	// but the human-rooted run must still claim.
	claimed2, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("second claim: %v (human-rooted run must be exempt from the exhausted budget)", err)
	}
	if claimed2.ID != human.ID {
		t.Errorf("second claim = %q, want %q", claimed2.ID, human.ID)
	}
}

// TestClaimNextEligibleRun_AgeBasedPromotionReordersClaim pins
// AC-OFFICE-BACKPRESSURE-002.1/002.2: a run queued strictly longer than
// PromotionAge claims as one class better, which can flip the winner
// against a newer, nominally higher-priority-class run.
func TestClaimNextEligibleRun_AgeBasedPromotionReordersClaim(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedClaimAgent(t, repo, "a1")
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{PromotionAge: 10 * time.Minute})

	now := time.Now().UTC()
	stale := mustCreateRun(t, repo, &models.Run{
		ID:             "stale-periodic",
		AgentProfileID: "a1",
		WorkspaceID:    claimTestWorkspace("a1"),
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
		PriorityClass:  models.PriorityClassPeriodic,
	})
	setRequestedAt(t, repo, stale.ID, now.Add(-20*time.Minute))
	fresh := mustCreateRun(t, repo, &models.Run{
		ID:             "fresh-event",
		AgentProfileID: "a1",
		WorkspaceID:    claimTestWorkspace("a1"),
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
		PriorityClass:  models.PriorityClassEvent,
	})
	setRequestedAt(t, repo, fresh.ID, now.Add(-time.Minute))

	// Without promotion, class ordering alone picks fresh (event=2 < periodic=3).
	// Promoted, stale is clamped to class 2 (periodic-1) and wins the tie by age.
	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != stale.ID {
		t.Errorf("claimed %q, want %q (age-promoted periodic run should win the tie)", claimed.ID, stale.ID)
	}
}

// TestClaimNextEligibleRun_AgeBasedPromotionIsNoOpForHumanClass pins
// AC-OFFICE-BACKPRESSURE-002.2 verbatim: "a human run's promotion is a
// no-op." Two human-class (0) runs, one aged past PromotionAge and one
// fresh: a no-op promotion leaves both at class 0, so FIFO (requested_at)
// picks the older one. An underflowing promotion (GREATEST(0-1, 1) = 1)
// would instead demote the aged run to class 1 (recovery), leaving the
// fresh, untouched class-0 run to win the claim ahead of it — exactly
// backwards from "no-op".
func TestClaimNextEligibleRun_AgeBasedPromotionIsNoOpForHumanClass(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedClaimAgent(t, repo, "a1")
	repo.SetClaimSafetyLimits(runssqlite.ClaimSafetyLimits{PromotionAge: 10 * time.Minute})

	now := time.Now().UTC()
	staleHuman := mustCreateRun(t, repo, &models.Run{
		ID:             "stale-human",
		AgentProfileID: "a1",
		WorkspaceID:    claimTestWorkspace("a1"),
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
		PriorityClass:  models.PriorityClassHuman,
	})
	setRequestedAt(t, repo, staleHuman.ID, now.Add(-20*time.Minute))
	freshHuman := mustCreateRun(t, repo, &models.Run{
		ID:             "fresh-human",
		AgentProfileID: "a1",
		WorkspaceID:    claimTestWorkspace("a1"),
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
		PriorityClass:  models.PriorityClassHuman,
	})
	setRequestedAt(t, repo, freshHuman.ID, now.Add(-time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != staleHuman.ID {
		t.Errorf("claimed %q, want %q (aged human-class run must stay class 0 and win FIFO, not be demoted below the fresh human run)", claimed.ID, staleHuman.ID)
	}
}

// TestClaimNextEligibleRun_AppendsLedgerRowOnClaim pins
// REQ-OFFICE-LAUNCH-SAFETY-002: a successful claim durably records itself
// in office_launch_ledger, independent of runs.claimed_at.
func TestClaimNextEligibleRun_AppendsLedgerRowOnClaim(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	const ws = "ledger-ws"
	const routineID = "ledger-routine"
	seedAgentInWorkspace(t, repo, "a1", ws, 5)

	run := mustCreateRun(t, repo, &models.Run{
		ID:             "ledger-run",
		AgentProfileID: "a1",
		WorkspaceID:    ws,
		RoutineID:      routineID,
		CausationID:    "cause-1",
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
		HumanRooted:    true,
	})

	if _, err := repo.ClaimNextEligibleRun(ctx); err != nil {
		t.Fatalf("claim: %v", err)
	}

	if got := countLedgerRows(t, repo, run.ID); got != 1 {
		t.Fatalf("ledger rows for %q = %d, want 1", run.ID, got)
	}

	var (
		gotWorkspace   string
		gotCausation   string
		gotRoutine     string
		gotHumanRooted bool
	)
	row := repo.Writer().QueryRowx(
		`SELECT workspace_id, causation_id, routine_id, human_rooted
		 FROM office_launch_ledger WHERE run_id = ?`, run.ID,
	)
	if err := row.Scan(&gotWorkspace, &gotCausation, &gotRoutine, &gotHumanRooted); err != nil {
		t.Fatalf("scan ledger row: %v", err)
	}
	if gotWorkspace != ws {
		t.Errorf("ledger workspace_id = %q, want %q", gotWorkspace, ws)
	}
	if gotCausation != "cause-1" {
		t.Errorf("ledger causation_id = %q, want cause-1", gotCausation)
	}
	if gotRoutine != routineID {
		t.Errorf("ledger routine_id = %q, want %q", gotRoutine, routineID)
	}
	if !gotHumanRooted {
		t.Error("ledger human_rooted = false, want true")
	}
}

// TestClaimNextEligibleRun_DoesNotStarveBehindMoreThanOnePageOfBlockedRuns
// is the Review round 1 finding 2 regression test: the claim scan used to
// fetch only the top claimCandidateBatchSize=20 queued rows and give up
// if none of those cleared every gate. A single saturated agent with
// more than 20 queued rows ranked ahead of everything else permanently
// occupied the whole candidate window, starving every other workspace's
// claim attempts indefinitely — the opposite of what age-based promotion
// (REQ-OFFICE-BACKPRESSURE-002) exists to prevent. This seeds 25 blocked
// rows for one already-at-ceiling agent, ranked ahead of one fully
// claimable run for a different, unrelated agent, and asserts the
// claimable run is still found.
func TestClaimNextEligibleRun_DoesNotStarveBehindMoreThanOnePageOfBlockedRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	seedClaimAgent(t, repo, "saturated")
	setAgentCap(t, repo, "saturated", 1)

	// Fill the saturated agent's ceiling (cap=1) with an already-claimed
	// run, so every further run it queues is permanently blocked by
	// evalAgentCeilingGate.
	already := queueRunAt(t, repo, "already-claimed", "saturated", base.Add(-time.Hour))
	primed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("prime claim: %v", err)
	}
	if primed.ID != already.ID {
		t.Fatalf("prime claim = %q, want %q", primed.ID, already.ID)
	}

	// More than one candidate page's worth of blocked rows, all ranked
	// ahead of the free agent's run below.
	for i := 0; i < 25; i++ {
		queueRunAt(t, repo, fmt.Sprintf("blocked-%02d", i), "saturated", base.Add(time.Duration(i+1)*time.Minute))
	}

	seedClaimAgent(t, repo, "free")
	freeRun := queueRunAt(t, repo, "free-run", "free", base.Add(time.Hour))

	got, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v (a claimable run exists but was not found)", err)
	}
	if got.ID != freeRun.ID {
		t.Errorf("claimed = %q, want %q — an unrelated agent's backlog must not starve a claimable run",
			got.ID, freeRun.ID)
	}
}

// seedAgentInWorkspace seeds an agent_profiles row for agentID in an
// explicit workspace (rather than claimTestWorkspace's per-agent
// derivation), so cross-agent ceiling/budget tests can share one
// workspace, and sets its max_concurrent_sessions ceiling.
func seedAgentInWorkspace(t *testing.T, repo *runssqlite.Repository, agentID, workspaceID string, cap int) {
	t.Helper()
	seedAgentProfile(t, repo.Writer(), agentID, workspaceID)
	setAgentCap(t, repo, agentID, cap)
}

// queueRunInWorkspace is queueRunAt with an explicit workspace instead of
// claimTestWorkspace's per-agent derivation, for tests that deliberately
// share one workspace across agents.
func queueRunInWorkspace(
	t *testing.T, repo *runssqlite.Repository, id, agentID, workspaceID string, requestedAt time.Time,
) *models.Run {
	t.Helper()
	run := mustCreateRun(t, repo, &models.Run{
		ID:             id,
		AgentProfileID: agentID,
		WorkspaceID:    workspaceID,
		Reason:         "task_assigned",
		Payload:        `{"task_id":"` + id + `"}`,
		Status:         "queued",
		CoalescedCount: 1,
	})
	setRequestedAt(t, repo, run.ID, requestedAt)
	return run
}
