package costs_test

import (
	"context"
	"errors"
	"expvar"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
)

// claimFailureCount reads the current value of budget_claim_failures_total's
// "op=claim" entry. Tests must compare before/after deltas, never the
// absolute value: the expvar.Map is process-global and accumulates across
// every test in this package.
func claimFailureCount(t *testing.T) int64 {
	t.Helper()
	v := expvar.Get("budget_claim_failures_total")
	if v == nil {
		return 0
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("budget_claim_failures_total is not an expvar.Map, got %T", v)
	}
	kv := m.Get("op=claim")
	if kv == nil {
		return 0
	}
	iv, ok := kv.(*expvar.Int)
	if !ok {
		t.Fatalf("op=claim counter is not an expvar.Int, got %T", kv)
	}
	return iv.Value()
}

// REQ-OFFICE-COSTS-002: budget notifications fire on crossings, not on
// every evaluation. See docs/specs/office/requirements/costs.md and
// docs/specs/office/system-design/costs-03.md / costs-04.md.

func newIdempotencyTestPolicy(scopeID string, limit int64) *models.BudgetPolicy {
	return &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           scopeID,
		LimitSubcents:     limit,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
}

// monthlyPeriodKey mirrors the RFC3339-UTC month-start rendering the
// service computes internally, for tests that assert on stored claim rows.
func monthlyPeriodKey() string {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
}

func TestCheckBudget_EvaluateTwice_AlertEmitsOnce(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-alert-twice")
	agents := &repoAgents{repo: repo}
	spy := &budgetActivitySpy{}
	svc := costs.NewCostService(repo, logger.Default(), spy, agents, agents)

	policy := newIdempotencyTestPolicy("agent-alert-twice", 1000)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-alert-twice", "task-1", 850)

	first, err := svc.CheckBudget(ctx, "ws-1", "agent-alert-twice", "proj-1")
	if err != nil {
		t.Fatalf("first CheckBudget: %v", err)
	}
	if !first[0].AlertFired || !first[0].AlertSubmitted {
		t.Fatalf("first evaluation: AlertFired=%v AlertSubmitted=%v, want true/true", first[0].AlertFired, first[0].AlertSubmitted)
	}

	second, err := svc.CheckBudget(ctx, "ws-1", "agent-alert-twice", "proj-1")
	if err != nil {
		t.Fatalf("second CheckBudget: %v", err)
	}
	if !second[0].AlertFired {
		t.Error("second evaluation should still report the level reached")
	}
	if second[0].AlertSubmitted {
		t.Error("second evaluation must not resubmit an already-claimed level")
	}

	if got := spy.count("budget.alert"); got != 1 {
		t.Fatalf("budget.alert submissions = %d, want 1", got)
	}
}

func TestCheckBudget_EvaluateTwice_ExceededEmitsOnce(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-exceed-twice")
	agents := &repoAgents{repo: repo}
	spy := &budgetActivitySpy{}
	svc := costs.NewCostService(repo, logger.Default(), spy, agents, agents)

	policy := newIdempotencyTestPolicy("agent-exceed-twice", 500)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-exceed-twice", "task-1", 600)

	for i := 0; i < 2; i++ {
		if _, err := svc.CheckBudget(ctx, "ws-1", "agent-exceed-twice", "proj-1"); err != nil {
			t.Fatalf("CheckBudget[%d]: %v", i, err)
		}
	}

	if got := spy.count("budget.exceeded"); got != 1 {
		t.Fatalf("budget.exceeded submissions = %d, want 1", got)
	}
}

// TestCheckBudget_ExceededClaimsCompanionAlert covers AC-OFFICE-COSTS-002.5
// on a healthy store: spend jumping straight past the limit still holds
// the alert-level claim, so a later evaluation landing back in the alert
// band does not emit a budget.alert describing a spend reduction.
func TestCheckBudget_ExceededClaimsCompanionAlert(t *testing.T) {
	repo, queryRow, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-companion")
	agents := &repoAgents{repo: repo}
	spy := &budgetActivitySpy{}
	svc := costs.NewCostService(repo, logger.Default(), spy, agents, agents)

	policy := newIdempotencyTestPolicy("agent-companion", 500)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	// Jumps straight past the limit; the alert band is never reached on its
	// own by a rollup query, only inferred via the companion claim.
	insertBudgetTestCostEvent(t, execSQL, "agent-companion", "task-1", 600)

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-companion", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if !results[0].ExceededSubmitted {
		t.Fatal("expected the exceeded row to be submitted")
	}
	if results[0].AlertSubmitted {
		t.Fatal("the alert-level field must be false: only budget.exceeded was emitted (AC-OFFICE-COSTS-002.13)")
	}

	var count int
	if err := queryRow(
		`SELECT COUNT(*) FROM office_budget_claims WHERE policy_id = ? AND period_key = ? AND level = 'alert'`,
		policy.ID, monthlyPeriodKey(),
	).Scan(&count); err != nil {
		t.Fatalf("count companion claim: %v", err)
	}
	if count != 1 {
		t.Fatalf("companion alert-level claim rows = %d, want 1", count)
	}
}

// TestCheckBudget_CompanionAlertClaimFault_StillSubmitsExceeded covers the
// AC-OFFICE-COSTS-002.5 fault-injection carve-out: when the companion
// alert-level claim errors, the evaluation still submits budget.exceeded
// (already earned by the exceeded-level claim) and the failure is logged
// and counted like any other claim-store error. The test deliberately does
// NOT assert the companion claim was recorded, per the spec's own
// instruction — that is the failure this test injects.
func TestCheckBudget_CompanionAlertClaimFault_StillSubmitsExceeded(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-companion-fault")
	faulty := &claimFaultRepo{Repository: repo, failLevels: map[string]error{
		"alert": errors.New("injected companion claim failure"),
	}}
	agents := &repoAgents{repo: repo}
	spy := &budgetActivitySpy{}
	svc := costs.NewCostService(faulty, logger.Default(), spy, agents, agents)

	policy := newIdempotencyTestPolicy("agent-companion-fault", 500)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-companion-fault", "task-1", 600)

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-companion-fault", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if !results[0].ExceededSubmitted {
		t.Fatal("budget.exceeded must still be submitted despite the companion claim failure")
	}
	if got := spy.count("budget.exceeded"); got != 1 {
		t.Fatalf("budget.exceeded submissions = %d, want 1", got)
	}
}

// TestCheckBudget_ClaimStoreFault_EmitsAndReportsSubmittedTrue covers
// AC-OFFICE-COSTS-002.14: a claim-store error degrades to emit-anyway, and
// the result's submitted field is true even though no claim was held — the
// case a "held the claim and submitted" reading gets backwards.
func TestCheckBudget_ClaimStoreFault_EmitsAndReportsSubmittedTrue(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-fault")
	injected := errors.New("injected claim store failure")
	faulty := &claimFaultRepo{Repository: repo, failLevels: map[string]error{
		"alert": injected,
	}}
	agents := &repoAgents{repo: repo}
	spy := &budgetActivitySpy{}
	svc := costs.NewCostService(faulty, logger.Default(), spy, agents, agents)

	policy := newIdempotencyTestPolicy("agent-fault", 1000)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-fault", "task-1", 850)

	before := claimFailureCount(t)

	// Two evaluations, both against the always-failing claim store: a
	// broken store degrades to duplicate notifications, never to silence.
	for i := 0; i < 2; i++ {
		results, err := svc.CheckBudget(ctx, "ws-1", "agent-fault", "proj-1")
		if err != nil {
			t.Fatalf("CheckBudget[%d]: %v", i, err)
		}
		if !results[0].AlertSubmitted {
			t.Fatalf("evaluation %d: AlertSubmitted = false, want true (fail-open emission)", i)
		}
	}
	if got := spy.count("budget.alert"); got != 2 {
		t.Fatalf("budget.alert submissions under a faulted store = %d, want 2 (duplicates permitted)", got)
	}
	if delta := claimFailureCount(t) - before; delta != 2 {
		t.Fatalf("budget_claim_failures_total delta = %d, want 2 (AC-OFFICE-COSTS-002.14: one per faulted evaluation)", delta)
	}
}

// TestCheckBudget_AlreadyClaimedMiss_NoFailureCounterDelta covers the
// AC-OFFICE-COSTS-002.14a half of the failure counter: a Claim call
// returning (claimed=false, err=nil) — whether because an earlier
// evaluation already holds it or because the referenced policy was deleted
// mid-evaluation (a foreign-key violation, classified identically) — is an
// ordinary miss, not a claim-store failure, and must not move
// budget_claim_failures_total.
func TestCheckBudget_AlreadyClaimedMiss_NoFailureCounterDelta(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-miss")
	missy := &claimFaultRepo{Repository: repo, missLevels: map[string]bool{"alert": true}}
	agents := &repoAgents{repo: repo}
	spy := &budgetActivitySpy{}
	svc := costs.NewCostService(missy, logger.Default(), spy, agents, agents)

	policy := newIdempotencyTestPolicy("agent-miss", 1000)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-miss", "task-1", 850)

	before := claimFailureCount(t)

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-miss", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if results[0].AlertSubmitted {
		t.Fatal("a (claimed=false, err=nil) miss must not submit")
	}
	if got := spy.count("budget.alert"); got != 0 {
		t.Fatalf("budget.alert submissions = %d, want 0", got)
	}
	if delta := claimFailureCount(t) - before; delta != 0 {
		t.Fatalf("budget_claim_failures_total delta = %d, want 0 (AC-OFFICE-COSTS-002.14a: a miss is not a store failure)", delta)
	}
}

// TestCheckBudget_ExceededClaimsCompanionBeforeEmit covers AC-OFFICE-COSTS-002.5's
// claim-then-emit ordering: costs-03.md's Claim-then-emit step 2 records both
// the exceeded claim and its alert-level companion before the emission
// decision. Asserting call order (not just eventual outcome) is what proves
// the ordering — outcome alone cannot distinguish this from a companion claim
// recorded after the emit.
func TestCheckBudget_ExceededClaimsCompanionBeforeEmit(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-order")
	order := &callOrderRecorder{}
	orderedRepo := &orderRecordingRepo{Repository: repo, order: order}
	agents := &repoAgents{repo: repo}
	svc := costs.NewCostService(orderedRepo, logger.Default(), &orderRecordingActivity{order: order}, agents, agents)

	policy := newIdempotencyTestPolicy("agent-order", 500)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-order", "task-1", 600)

	if _, err := svc.CheckBudget(ctx, "ws-1", "agent-order", "proj-1"); err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}

	got := order.snapshot()
	want := []string{"claim:exceeded", "claim:alert", "emit:budget.exceeded"}
	if len(got) != len(want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call order = %v, want %v (both claims must precede the emission)", got, want)
		}
	}
}

// TestCheckBudget_PauseAgent_ReArmsDespiteExistingClaim covers
// AC-OFFICE-COSTS-002.12: suppressing a notification must not suppress
// enforcement. An evaluation landing on an already-claimed level must still
// pause an agent the user has since unpaused.
func TestCheckBudget_PauseAgent_ReArmsDespiteExistingClaim(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-rearm")

	policy := newIdempotencyTestPolicy("agent-rearm", 500)
	policy.ActionOnExceed = "pause_agent"
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-rearm", "task-1", 600)

	first, err := svc.CheckBudget(ctx, "ws-1", "agent-rearm", "proj-1")
	if err != nil {
		t.Fatalf("first CheckBudget: %v", err)
	}
	if !first[0].AgentPaused {
		t.Fatal("first evaluation should pause the agent")
	}

	if err := repo.UpdateAgentStatusFields(ctx, "agent-rearm", "idle", ""); err != nil {
		t.Fatalf("unpause agent: %v", err)
	}

	second, err := svc.CheckBudget(ctx, "ws-1", "agent-rearm", "proj-1")
	if err != nil {
		t.Fatalf("second CheckBudget: %v", err)
	}
	if !second[0].AgentPaused {
		t.Fatal("second evaluation must still pause the agent despite the level already being claimed")
	}

	agent, err := repo.GetAgentInstance(ctx, "agent-rearm")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != "paused" {
		t.Fatalf("agent status = %q, want paused", agent.Status)
	}
}

// TestCheckBudget_ConcurrentEvaluation_EmitsOnce covers
// AC-OFFICE-COSTS-002.10 on a healthy store: the claim table's primary key
// resolves the race, so concurrent evaluations of the same policy, period
// and level submit exactly one activity row.
func TestCheckBudget_ConcurrentEvaluation_EmitsOnce(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-concurrent")
	agents := &repoAgents{repo: repo}
	spy := &budgetActivitySpy{}
	svc := costs.NewCostService(repo, logger.Default(), spy, agents, agents)

	policy := newIdempotencyTestPolicy("agent-concurrent", 1000)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-concurrent", "task-1", 850)

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := svc.CheckBudget(ctx, "ws-1", "agent-concurrent", "proj-1"); err != nil {
				t.Errorf("CheckBudget: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := spy.count("budget.alert"); got != 1 {
		t.Fatalf("budget.alert submissions under concurrent evaluation = %d, want 1", got)
	}
}

// TestCheckBudget_PeriodKeyMatchesSpendBoundary covers AC-OFFICE-COSTS-002.6a:
// the claim recorded for a monthly policy is keyed to the same window
// boundary the spend rollup used, rendered as RFC3339 UTC.
func TestCheckBudget_PeriodKeyMatchesSpendBoundary(t *testing.T) {
	repo, queryRow, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-period")
	agents := &repoAgents{repo: repo}
	svc := costs.NewCostService(repo, logger.Default(), &noopActivity{}, agents, agents)

	policy := newIdempotencyTestPolicy("agent-period", 1000)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-period", "task-1", 850)

	if _, err := svc.CheckBudget(ctx, "ws-1", "agent-period", "proj-1"); err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}

	var gotKey string
	if err := queryRow(
		`SELECT period_key FROM office_budget_claims WHERE policy_id = ? AND level = 'alert'`, policy.ID,
	).Scan(&gotKey); err != nil {
		t.Fatalf("query claim period_key: %v", err)
	}
	if gotKey != monthlyPeriodKey() {
		t.Fatalf("claim period_key = %q, want %q (must match the spend rollup's boundary)", gotKey, monthlyPeriodKey())
	}
}

// TestCheckBudget_LifetimePolicyClaimsLifetimeKey covers the "lifetime"
// literal branch of AC-OFFICE-COSTS-002.6: a policy whose spend window
// never resets (period "total") gets a claim that never resets either.
func TestCheckBudget_LifetimePolicyClaimsLifetimeKey(t *testing.T) {
	repo, queryRow, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-lifetime")
	agents := &repoAgents{repo: repo}
	svc := costs.NewCostService(repo, logger.Default(), &noopActivity{}, agents, agents)

	policy := newIdempotencyTestPolicy("agent-lifetime", 1000)
	policy.Period = "total"
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-lifetime", "task-1", 850)

	if _, err := svc.CheckBudget(ctx, "ws-1", "agent-lifetime", "proj-1"); err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}

	var gotKey string
	if err := queryRow(
		`SELECT period_key FROM office_budget_claims WHERE policy_id = ? AND level = 'alert'`, policy.ID,
	).Scan(&gotKey); err != nil {
		t.Fatalf("query claim period_key: %v", err)
	}
	if gotKey != "lifetime" {
		t.Fatalf("claim period_key = %q, want %q", gotKey, "lifetime")
	}
}

// TestCheckBudget_SpendBelowLevelKeepsExistingClaim covers
// AC-OFFICE-COSTS-002.15: nothing in the evaluation flow ever releases a
// claim, even when a later evaluation finds spend below the level it
// claimed.
func TestCheckBudget_SpendBelowLevelKeepsExistingClaim(t *testing.T) {
	repo, _, _ := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-keep-claim")
	agents := &repoAgents{repo: repo}
	svc := costs.NewCostService(repo, logger.Default(), &noopActivity{}, agents, agents)

	policy := newIdempotencyTestPolicy("agent-keep-claim", 1000)
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	periodKey := monthlyPeriodKey()
	if claimed, err := repo.Claim(ctx, policy.ID, periodKey, "alert"); err != nil || !claimed {
		t.Fatalf("seed claim: claimed=%v err=%v", claimed, err)
	}

	// No cost events: spend is 0, well below the alert level, so this
	// evaluation takes the "no claim, no emission" branch entirely.
	if _, err := svc.CheckBudget(ctx, "ws-1", "agent-keep-claim", "proj-1"); err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}

	claimed, err := repo.Claim(ctx, policy.ID, periodKey, "alert")
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if claimed {
		t.Fatal("existing claim must survive an evaluation where spend does not reach the level")
	}
}

// TestCheckPreExecutionBudget_StillDeniesAfterAlreadyClaimed covers
// AC-OFFICE-COSTS-002.12: suppressing a notification must not suppress
// enforcement. block_new_tasks keeps denying on every subsequent check
// regardless of whether a claim already exists.
func TestCheckPreExecutionBudget_StillDeniesAfterAlreadyClaimed(t *testing.T) {
	repo, _, execSQL := newBudgetTestRepo(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-enforce")
	agents := &repoAgents{repo: repo}
	svc := costs.NewCostService(repo, logger.Default(), &noopActivity{}, agents, agents)

	policy := newIdempotencyTestPolicy("agent-enforce", 500)
	policy.ActionOnExceed = "block_new_tasks"
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-enforce", "task-1", 600)

	if _, err := svc.CheckBudget(ctx, "ws-1", "agent-enforce", "proj-1"); err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}

	allowed, reason, err := svc.CheckPreExecutionBudget(ctx, "agent-enforce", "", "ws-1")
	if err != nil {
		t.Fatalf("CheckPreExecutionBudget: %v", err)
	}
	if allowed {
		t.Errorf("block_new_tasks must still deny once the notification is already claimed; reason=%q", reason)
	}
}
