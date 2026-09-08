package costs_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// mustCreateCostEvent inserts a cost event via the real repository, so
// EvaluatePreLaunch's spend-window queries (which join agent_profiles /
// filter project_id, not a manually-seeded fixture table) resolve it.
func mustCreateCostEvent(
	t *testing.T, repo *sqlite.Repository,
	agentID, projectID string, costSubcents int64, occurredAt time.Time, costSource *models.CostSource,
) {
	t.Helper()
	event := &models.CostEvent{
		ID: uuid.NewString(), AgentProfileID: agentID, ProjectID: projectID,
		CostSubcents: costSubcents, OccurredAt: occurredAt, CostSource: costSource,
	}
	if err := repo.CreateCostEvent(context.Background(), event); err != nil {
		t.Fatalf("create cost event: %v", err)
	}
}

// insertRawBudgetPolicy inserts a budget policy row directly via SQL, so
// tests can control created_at (for AC-OFFICE-BUDGET-001.9 ordering) and
// write a stored row validateBudgetPolicyWrite would reject (for
// AC-OFFICE-BUDGET-002.5/.11/.14's "malformed row already in storage" case).
func insertRawBudgetPolicy(
	t *testing.T, execSQL func(string, ...interface{}),
	workspaceID string, scopeType models.BudgetScopeType, scopeID string,
	limitSubcents int64, period models.BudgetPeriod, action models.BudgetActionOnExceed,
	createdAt time.Time,
) string {
	t.Helper()
	id := uuid.NewString()
	execSQL(
		`INSERT INTO office_budget_policies
			(id, workspace_id, scope_type, scope_id, limit_subcents, period, alert_threshold_pct, action_on_exceed, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 80, ?, ?, ?)`,
		id, workspaceID, string(scopeType), scopeID, limitSubcents, string(period), string(action),
		createdAt.UTC().Format(time.RFC3339Nano), createdAt.UTC().Format(time.RFC3339Nano),
	)
	return id
}

func TestEvaluatePreLaunch_NoPolicies_Launches(t *testing.T) {
	svc, _, _ := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("Decision = %v, want Launch", got.Decision)
	}
	if got.WorkspaceDailyBlockingSuperseded {
		t.Error("WorkspaceDailyBlockingSuperseded = true, want false with no policies")
	}
}

func TestEvaluatePreLaunch_UnderLimit_Launches(t *testing.T) {
	svc, _, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1000,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-time.Hour))

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("Decision = %v, want Launch", got.Decision)
	}
}

func TestEvaluatePreLaunch_OverLimitPauseAgent_Blocks(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	policyID := insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1000,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-time.Hour))
	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 1500, at.Add(-30*time.Minute), nil)

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionBlockedByLimit {
		t.Errorf("Decision = %v, want BlockedByLimit", got.Decision)
	}
	if got.DecidingPolicy == nil || got.DecidingPolicy.PolicyID != policyID {
		t.Errorf("DecidingPolicy = %+v, want policy %s", got.DecidingPolicy, policyID)
	}
}

// TestEvaluatePreLaunch_Inert covers AC-OFFICE-BUDGET-006.2: a pre-launch
// evaluation is a pure read against the same spend window the post-hoc
// budgets.go path (EvaluateBudget/CheckPreExecutionBudget) also reads, but
// unlike that path it must never itself write a budget.alert/budget.exceeded
// activity entry or pause the agent -- those side effects belong solely to
// the existing cost-recording path, not to an admission decision. Reuses
// TestEvaluatePreLaunch_OverLimitPauseAgent_Blocks's exact over-limit,
// pause_agent fixture (the shape most likely to tempt a shared side-effect
// path) but swaps in a service built with newBudgetTestServiceWithActivity's
// spy so the absence of any LogActivity call is observable, and asserts the
// agent's stored status is unchanged.
func TestEvaluatePreLaunch_Inert(t *testing.T) {
	spy := &budgetActivitySpy{}
	svc, repo, execSQL := newBudgetTestServiceWithActivity(t, spy)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1000,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-time.Hour))
	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 1500, at.Add(-30*time.Minute), nil)

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionBlockedByLimit {
		t.Fatalf("Decision = %v, want BlockedByLimit (fixture must actually be over limit to be a meaningful inertness check)", got.Decision)
	}

	if len(spy.calls) != 0 {
		t.Errorf("EvaluatePreLaunch logged %d activity entries, want 0: %+v", len(spy.calls), spy.calls)
	}
	agent, err := repo.GetAgentInstance(ctx, "agent-1")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != models.AgentStatusIdle {
		t.Errorf("agent status = %q, want unchanged %q -- a pre-launch evaluation must never pause an agent",
			agent.Status, models.AgentStatusIdle)
	}
}

func TestEvaluatePreLaunch_OverLimitNotifyOnly_Launches(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1000,
		models.BudgetPeriodDaily, models.BudgetActionNotifyOnly, at.Add(-time.Hour))
	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 1500, at.Add(-30*time.Minute), nil)

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("Decision = %v, want Launch (notify_only never blocks on limit alone)", got.Decision)
	}
}

// TestEvaluatePreLaunch_FirstFiringPolicyWins pins AC-OFFICE-BUDGET-001.9:
// policies are evaluated in (created_at ASC, id ASC) order and the FIRST
// one whose limit or degradation test fires decides, even though a
// later-created policy would also fire.
func TestEvaluatePreLaunch_FirstFiringPolicyWins(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 1500, at.Add(-30*time.Minute), nil)

	earlyID := insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1000,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-2*time.Hour))
	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 500,
		models.BudgetPeriodDaily, models.BudgetActionBlockNewTasks, at.Add(-time.Hour))

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionBlockedByLimit {
		t.Fatalf("Decision = %v, want BlockedByLimit", got.Decision)
	}
	if got.DecidingPolicy == nil || got.DecidingPolicy.PolicyID != earlyID {
		t.Errorf("DecidingPolicy = %+v, want the earlier-created policy %s", got.DecidingPolicy, earlyID)
	}
}

// TestEvaluatePreLaunch_MalformedPolicySkipped pins AC-OFFICE-BUDGET-002.5/
// .11/.14: a stored policy the build cannot evaluate is skipped -- excluded
// from ordering/selection and from default-supersession -- not treated as a
// lifetime window or blocking policy.
func TestEvaluatePreLaunch_MalformedPolicySkipped(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 1500, at.Add(-30*time.Minute), nil)

	// Non-positive limit: malformed, must be skipped rather than blocking
	// every run.
	malformedID := insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", -1,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-time.Hour))

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("Decision = %v, want Launch (malformed policy must not block)", got.Decision)
	}
	if got.WorkspaceDailyBlockingSuperseded {
		t.Error("WorkspaceDailyBlockingSuperseded = true, want false: the only daily policy is skipped")
	}
	found := false
	for _, p := range got.Policies {
		if p.PolicyID == malformedID {
			found = true
			if !p.Skipped {
				t.Error("malformed policy result has Skipped = false, want true")
			}
			if !p.SkipIssues.NonPositiveLimit {
				t.Error("malformed policy result SkipIssues.NonPositiveLimit = false, want true")
			}
		}
	}
	if !found {
		t.Error("malformed policy is absent from Policies; observability needs it present with Skipped=true")
	}
}

func TestEvaluatePreLaunch_AgentScopeAppliesOnlyToMatchingAgent(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	createBudgetTestAgent(t, repo, "ws-1", "agent-2")
	mustCreateCostEvent(t, repo, "agent-1", "", 1500, at.Add(-30*time.Minute), nil)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeAgent, "agent-2", 1000,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-time.Hour))

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("Decision = %v, want Launch: agent-2's policy must not apply to agent-1's run", got.Decision)
	}
	if len(got.Policies) != 0 {
		t.Errorf("Policies = %+v, want empty: a non-matching agent policy is not applicable at all", got.Policies)
	}
}

func TestEvaluatePreLaunch_ProjectScopeRequiresHasProject(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "proj-1", 1500, at.Add(-30*time.Minute), nil)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeProject, "proj-1", 1000,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-time.Hour))

	// hasProject=false: a genuinely project-less run must never match a
	// project policy, even though scope_id happens to be non-empty.
	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("Decision = %v, want Launch when hasProject=false", got.Decision)
	}

	// hasProject=true with the matching project: now it applies and fires.
	got, err = svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "proj-1", true, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionBlockedByLimit {
		t.Errorf("Decision = %v, want BlockedByLimit when hasProject=true and project matches", got.Decision)
	}
}

// TestEvaluatePreLaunch_WorkspaceDailyBlockingSupersededIgnoresWhetherItFired
// pins AC-OFFICE-BUDGET-003.4: supersession is by a blocking-capable daily
// policy EXISTING among survivors, not by it having fired.
func TestEvaluatePreLaunch_WorkspaceDailyBlockingSupersededIgnoresWhetherItFired(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 10, at.Add(-30*time.Minute), nil)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1_000_000,
		models.BudgetPeriodDaily, models.BudgetActionPauseAgent, at.Add(-time.Hour))

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("Decision = %v, want Launch: spend is far under the limit", got.Decision)
	}
	if !got.WorkspaceDailyBlockingSuperseded {
		t.Error("WorkspaceDailyBlockingSuperseded = false, want true: a blocking-capable daily policy exists regardless of firing")
	}
}

// TestEvaluatePreLaunch_WorkspaceDailyNotifyOnly_DoesNotSupersedeDefault pins
// AC-OFFICE-BUDGET-003.4's negative case: supersession requires a
// blocking-capable action (pause_agent or block_new_tasks). A workspace-scoped
// daily policy configured notify_only cannot block on its own, so it must not
// suppress the built-in default ceiling either.
func TestEvaluatePreLaunch_WorkspaceDailyNotifyOnly_DoesNotSupersedeDefault(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 10, at.Add(-30*time.Minute), nil)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1_000_000,
		models.BudgetPeriodDaily, models.BudgetActionNotifyOnly, at.Add(-time.Hour))

	got, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if got.WorkspaceDailyBlockingSuperseded {
		t.Error("WorkspaceDailyBlockingSuperseded = true, want false: a notify_only daily policy cannot block, so it must not supersede the default")
	}
}

// TestEvaluatePreLaunch_DegradationBlocksEvenNotifyOnly pins
// AC-OFFICE-BUDGET-004.3: pricing degradation applies regardless of the
// policy's configured action, and only for an unattended run.
func TestEvaluatePreLaunch_DegradationBlocksEvenNotifyOnly(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")
	mustCreateCostEvent(t, repo, "agent-1", "", 500, at.Add(-30*time.Minute), nil)
	unpriced := models.CostSourceUnpriced
	mustCreateCostEvent(t, repo, "agent-1", "", 0, at.Add(-20*time.Minute), &unpriced)

	insertRawBudgetPolicy(t, execSQL, "ws-1", models.BudgetScopeWorkspace, "", 1000,
		models.BudgetPeriodDaily, models.BudgetActionNotifyOnly, at.Add(-time.Hour))

	gotUnattended, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceUnattended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if gotUnattended.Decision != costs.PreLaunchDecisionBlockedByDegradation {
		t.Errorf("unattended Decision = %v, want BlockedByDegradation", gotUnattended.Decision)
	}

	gotAttended, err := svc.EvaluatePreLaunch(ctx, "ws-1", "agent-1", "", false, shared.RunProvenanceAttended, at)
	if err != nil {
		t.Fatalf("EvaluatePreLaunch: %v", err)
	}
	if gotAttended.Decision != costs.PreLaunchDecisionLaunch {
		t.Errorf("attended Decision = %v, want Launch: attended runs are exempt from degradation blocking", gotAttended.Decision)
	}
}
