package costs_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// budgetActivitySpy implements shared.ActivityLogger and records every
// call so tests can assert on *submission* (AC-OFFICE-COSTS-002.2a), never
// with a SELECT against office_activity_log: LogActivity returns nothing,
// so the durability of any one row is not something the contract promises.
// The mutex makes it safe for TestCheckBudget_ConcurrentEvaluation_EmitsOnce,
// which drives LogActivity from multiple goroutines.
type budgetActivitySpy struct {
	mu    sync.Mutex
	calls []budgetActivityCall
}

// budgetActivityCall records one LogActivity invocation observed by
// budgetActivitySpy.
type budgetActivityCall struct {
	action, targetType, targetID, details string
}

func (s *budgetActivitySpy) LogActivity(_ context.Context, _, _, _, action, targetType, targetID, details string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, budgetActivityCall{action, targetType, targetID, details})
}

func (s *budgetActivitySpy) LogActivityWithRun(
	ctx context.Context, wsID, actorType, actorID, action, targetType, targetID, details, _, _ string,
) {
	s.LogActivity(ctx, wsID, actorType, actorID, action, targetType, targetID, details)
}

func (s *budgetActivitySpy) count(action string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c.action == action {
			n++
		}
	}
	return n
}

// claimFaultRepo wraps a real *sqlite.Repository and overrides Claim to
// fail (failLevels) or miss without error (missLevels) for the named
// levels, so tests can drive AC-OFFICE-COSTS-002.5's, .14's and .14a's
// claim-store outcomes deterministically without a fault-injecting SQL
// driver. missLevels stands in for either an already-held claim or a
// foreign-key-violation-as-miss (AC-OFFICE-COSTS-002.14a): both return
// (false, nil), and the counter under test cannot and should not
// distinguish them.
type claimFaultRepo struct {
	*sqlite.Repository
	failLevels map[string]error
	missLevels map[string]bool
}

func (r *claimFaultRepo) Claim(ctx context.Context, policyID, periodKey, level string) (bool, error) {
	if err, ok := r.failLevels[level]; ok {
		return false, err
	}
	if r.missLevels[level] {
		return false, nil
	}
	return r.Repository.Claim(ctx, policyID, periodKey, level)
}

// callOrderRecorder records events in the order they occur, for tests that
// must prove sequencing rather than just eventual outcome (AC-OFFICE-COSTS-002.5's
// claim-then-emit ordering).
type callOrderRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *callOrderRecorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *callOrderRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	copy(out, r.events)
	return out
}

// orderRecordingRepo wraps a real *sqlite.Repository and records each
// Claim call's level into a shared callOrderRecorder.
type orderRecordingRepo struct {
	*sqlite.Repository
	order *callOrderRecorder
}

func (r *orderRecordingRepo) Claim(ctx context.Context, policyID, periodKey, level string) (bool, error) {
	claimed, err := r.Repository.Claim(ctx, policyID, periodKey, level)
	r.order.record("claim:" + level)
	return claimed, err
}

// orderRecordingActivity implements shared.ActivityLogger and records each
// submitted action into the same callOrderRecorder as orderRecordingRepo,
// so a test can assert the interleaving of claims and emissions.
type orderRecordingActivity struct {
	order *callOrderRecorder
}

func (a *orderRecordingActivity) LogActivity(_ context.Context, _, _, _, action, _, _, _ string) {
	a.order.record("emit:" + action)
}

func (a *orderRecordingActivity) LogActivityWithRun(
	ctx context.Context, wsID, actorType, actorID, action, targetType, targetID, details, _, _ string,
) {
	a.LogActivity(ctx, wsID, actorType, actorID, action, targetType, targetID, details)
}

// repoAgents adapts the office repository to shared.AgentReader +
// shared.AgentWriter so budget tests can exercise the real pause-agent
// path end-to-end against an in-memory DB.
type repoAgents struct {
	repo *sqlite.Repository
}

func (a *repoAgents) GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error) {
	return a.repo.GetAgentInstance(ctx, id)
}

func (a *repoAgents) ListAgentInstances(ctx context.Context, wsID string) ([]*models.AgentInstance, error) {
	return a.repo.ListAgentInstances(ctx, wsID)
}

func (a *repoAgents) ListAgentInstancesByIDs(ctx context.Context, ids []string) ([]*models.AgentInstance, error) {
	return a.repo.ListAgentInstancesByIDs(ctx, ids)
}

func (a *repoAgents) UpdateAgentStatusFields(ctx context.Context, agentID, status, pauseReason string) error {
	return a.repo.UpdateAgentStatusFields(ctx, agentID, status, pauseReason)
}

// newBudgetTestRepo builds the in-memory office repo shared by every budget
// test, without wiring a CostService, so fault-injection tests can wrap the
// repo (claimFaultRepo) or swap the activity logger (budgetActivitySpy)
// before constructing the service. queryRow exposes read-only access to the
// underlying connection for tests that assert directly on
// office_budget_claims rather than through the CostService API.
func newBudgetTestRepo(t *testing.T) (*sqlite.Repository, func(string, ...interface{}) *sql.Row, func(string, ...interface{})) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		project_id TEXT DEFAULT '',
		state TEXT NOT NULL DEFAULT 'TODO',
		title TEXT DEFAULT '',
		description TEXT DEFAULT '',
		identifier TEXT DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create tasks: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	execSQL := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec sql: %v", err)
		}
	}
	queryRow := func(query string, args ...interface{}) *sql.Row {
		t.Helper()
		return db.QueryRow(query, args...)
	}
	return repo, queryRow, execSQL
}

func newBudgetTestService(t *testing.T) (*costs.CostService, *sqlite.Repository, func(string, ...interface{})) {
	t.Helper()
	return newBudgetTestServiceWithActivity(t, &noopActivity{})
}

// newBudgetTestServiceWithActivity is the same setup as newBudgetTestService
// but lets project-scope tests supply a spy activity logger to observe which
// budget.alert / budget.exceeded rows fire.
func newBudgetTestServiceWithActivity(
	t *testing.T, activity shared.ActivityLogger,
) (*costs.CostService, *sqlite.Repository, func(string, ...interface{})) {
	t.Helper()
	repo, _, execSQL := newBudgetTestRepo(t)
	log := logger.Default()
	agents := &repoAgents{repo: repo}
	svc := costs.NewCostService(repo, log, activity, agents, agents)
	return svc, repo, execSQL
}

// insertBudgetTestTask inserts a minimal task row carrying a project_id, so
// project-scoped cost rollups (which join office_cost_events to tasks on
// project_id) resolve. Agent-scoped tests don't need this: GetCostForAgentSince
// reads office_cost_events directly with no join.
func insertBudgetTestTask(t *testing.T, execSQL func(string, ...interface{}), taskID, workspaceID, projectID string) {
	t.Helper()
	execSQL(
		`INSERT INTO tasks (id, workspace_id, project_id) VALUES (?, ?, ?)`,
		taskID, workspaceID, projectID,
	)
}

func createBudgetTestAgent(t *testing.T, repo *sqlite.Repository, wsID, agentID string) {
	t.Helper()
	agent := &models.AgentInstance{
		ID:          agentID,
		WorkspaceID: wsID,
		Name:        "test-" + agentID,
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create test agent: %v", err)
	}
}

// insertBudgetTestCostEvent inserts a cost event directly via SQL for
// budget rollup tests. costSubcents is hundredths of a cent.
func insertBudgetTestCostEvent(t *testing.T, execSQL func(string, ...interface{}), agentID, taskID string, costSubcents int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	execSQL(
		`INSERT INTO office_cost_events (id, agent_profile_id, task_id, cost_subcents, occurred_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), agentID, taskID, costSubcents, now, now,
	)
}

func TestCheckBudget_UnderThreshold(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-1",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(30))

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].AlertFired {
		t.Error("alert should not fire under threshold")
	}
	if results[0].LimitExceed {
		t.Error("limit should not be exceeded")
	}
	if results[0].AgentPaused {
		t.Error("agent should not be paused")
	}
}

func TestCheckBudget_OverThreshold_Alert(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-1",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(850))

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].AlertFired {
		t.Error("alert should fire at 85% of limit")
	}
	if results[0].LimitExceed {
		t.Error("limit should not be exceeded")
	}
}

func TestCheckBudget_OverLimit_PauseAgent(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()

	createBudgetTestAgent(t, repo, "ws-1", "agent-1")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-1",
		LimitSubcents:     500,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(600))

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].LimitExceed {
		t.Error("limit should be exceeded")
	}
	if !results[0].AgentPaused {
		t.Error("agent should be paused")
	}

	agent, err := repo.GetAgentInstance(ctx, "agent-1")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != "paused" {
		t.Errorf("status = %q, want paused", agent.Status)
	}
	if agent.PauseReason != "budget_exceeded" {
		t.Errorf("pause_reason = %q, want budget_exceeded", agent.PauseReason)
	}
}

func TestCheckBudget_NoPolicies(t *testing.T) {
	svc, _, _ := newBudgetTestService(t)
	ctx := context.Background()

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %d", len(results))
	}
}

// TestCheckPreExecutionBudget_NotifyOnlyDoesNotBlock asserts the spec
// behaviour: an exceeded notify_only policy logs an alert but does not
// block the next run.
func TestCheckPreExecutionBudget_NotifyOnlyDoesNotBlock(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-notify")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-notify",
		LimitSubcents:     500,
		Period:            "total",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-notify", "task-1", int64(1000))

	allowed, reason, err := svc.CheckPreExecutionBudget(ctx, "agent-notify", "", "ws-1")
	if err != nil {
		t.Fatalf("CheckPreExecutionBudget: %v", err)
	}
	if !allowed {
		t.Errorf("notify_only exceedence must allow new runs; reason=%q", reason)
	}
}

// TestCheckPreExecutionBudget_BlockNewTasksBlocks confirms block_new_tasks
// returns allowed=false (the current session continues, but new runs are
// gated).
func TestCheckPreExecutionBudget_BlockNewTasksBlocks(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-block")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-block",
		LimitSubcents:     500,
		Period:            "total",
		AlertThresholdPct: 80,
		ActionOnExceed:    "block_new_tasks",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-block", "task-1", int64(1000))

	allowed, reason, err := svc.CheckPreExecutionBudget(ctx, "agent-block", "", "ws-1")
	if err != nil {
		t.Fatalf("CheckPreExecutionBudget: %v", err)
	}
	if allowed {
		t.Error("block_new_tasks exceedence must block")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

// TestCheckPreExecutionBudget_PauseAgentBlocks mirrors the existing
// pause_agent path through the new code path. Spec parity guard.
func TestCheckPreExecutionBudget_PauseAgentBlocks(t *testing.T) {
	svc, repo, execSQL := newBudgetTestService(t)
	ctx := context.Background()
	createBudgetTestAgent(t, repo, "ws-1", "agent-pause")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-pause",
		LimitSubcents:     500,
		Period:            "total",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestCostEvent(t, execSQL, "agent-pause", "task-1", int64(1000))

	allowed, _, err := svc.CheckPreExecutionBudget(ctx, "agent-pause", "", "ws-1")
	if err != nil {
		t.Fatalf("CheckPreExecutionBudget: %v", err)
	}
	if allowed {
		t.Error("pause_agent exceedence must block")
	}
}

// TestEvaluateProjectBudget_AlertAtThreshold covers the reassignment defect:
// a project-scoped notify_only policy must fire budget.alert when evaluated
// directly, without waiting for a future cost event or agent launch.
func TestEvaluateProjectBudget_AlertAtThreshold(t *testing.T) {
	spy := &budgetActivitySpy{}
	svc, _, execSQL := newBudgetTestServiceWithActivity(t, spy)
	ctx := context.Background()

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "project",
		ScopeID:           "proj-1",
		LimitSubcents:     1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestTask(t, execSQL, "task-1", "ws-1", "proj-1")
	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(850))

	if err := svc.EvaluateProjectBudget(ctx, "ws-1", "proj-1"); err != nil {
		t.Fatalf("EvaluateProjectBudget: %v", err)
	}

	if !hasBudgetActivity(spy.calls, "budget.alert", "proj-1") {
		t.Errorf("expected budget.alert for proj-1, got calls=%+v", spy.calls)
	}
}

// TestEvaluateProjectBudget_ExceededAtLimit covers the limit-exceeded branch.
func TestEvaluateProjectBudget_ExceededAtLimit(t *testing.T) {
	spy := &budgetActivitySpy{}
	svc, _, execSQL := newBudgetTestServiceWithActivity(t, spy)
	ctx := context.Background()

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "project",
		ScopeID:           "proj-1",
		LimitSubcents:     500,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestTask(t, execSQL, "task-1", "ws-1", "proj-1")
	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(600))

	if err := svc.EvaluateProjectBudget(ctx, "ws-1", "proj-1"); err != nil {
		t.Fatalf("EvaluateProjectBudget: %v", err)
	}

	if !hasBudgetActivity(spy.calls, "budget.exceeded", "proj-1") {
		t.Errorf("expected budget.exceeded for proj-1, got calls=%+v", spy.calls)
	}
}

// TestEvaluateProjectBudget_PauseAgentPolicyDoesNotPause locks in the design
// decision: a project-scoped pause_agent policy never pauses an agent.
// evaluatePolicy only pauses when ScopeType==agent, so this reuses that
// existing gate rather than adding new pause logic for project scope. The
// agent instance shares its ID with the policy's ScopeID so a regression
// that mistakenly treated ScopeID as an agent ID would find and pause it.
func TestEvaluateProjectBudget_PauseAgentPolicyDoesNotPause(t *testing.T) {
	spy := &budgetActivitySpy{}
	svc, repo, execSQL := newBudgetTestServiceWithActivity(t, spy)
	ctx := context.Background()

	createBudgetTestAgent(t, repo, "ws-1", "proj-1")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "project",
		ScopeID:           "proj-1",
		LimitSubcents:     500,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestTask(t, execSQL, "task-1", "ws-1", "proj-1")
	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(600))

	if err := svc.EvaluateProjectBudget(ctx, "ws-1", "proj-1"); err != nil {
		t.Fatalf("EvaluateProjectBudget: %v", err)
	}

	agent, err := repo.GetAgentInstance(ctx, "proj-1")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status == "paused" {
		t.Error("project-scoped pause_agent policy must not pause an agent")
	}
}

// TestEvaluateProjectBudget_WorkspacePoliciesNotEvaluated locks in the
// decision to skip scope=workspace policies: reassignment doesn't change the
// workspace total, so re-evaluating them would emit a duplicate alert row on
// every reassignment in an already over-budget workspace.
func TestEvaluateProjectBudget_WorkspacePoliciesNotEvaluated(t *testing.T) {
	spy := &budgetActivitySpy{}
	svc, _, execSQL := newBudgetTestServiceWithActivity(t, spy)
	ctx := context.Background()

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "workspace",
		ScopeID:           "",
		LimitSubcents:     500,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestTask(t, execSQL, "task-1", "ws-1", "proj-1")
	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(600))

	if err := svc.EvaluateProjectBudget(ctx, "ws-1", "proj-1"); err != nil {
		t.Fatalf("EvaluateProjectBudget: %v", err)
	}

	if len(spy.calls) != 0 {
		t.Errorf("expected no activity for workspace-scoped policies, got calls=%+v", spy.calls)
	}
}

// TestEvaluateProjectBudget_EmptyProjectIDSkips covers clearing a project
// (projectID=="") — there is no destination to evaluate.
func TestEvaluateProjectBudget_EmptyProjectIDSkips(t *testing.T) {
	spy := &budgetActivitySpy{}
	svc, _, execSQL := newBudgetTestServiceWithActivity(t, spy)
	ctx := context.Background()

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "project",
		ScopeID:           "proj-1",
		LimitSubcents:     500,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertBudgetTestTask(t, execSQL, "task-1", "ws-1", "proj-1")
	insertBudgetTestCostEvent(t, execSQL, "agent-1", "task-1", int64(600))

	if err := svc.EvaluateProjectBudget(ctx, "ws-1", ""); err != nil {
		t.Fatalf("EvaluateProjectBudget: %v", err)
	}

	if len(spy.calls) != 0 {
		t.Errorf("expected no activity when projectID is empty, got calls=%+v", spy.calls)
	}
}

func hasBudgetActivity(calls []budgetActivityCall, action, targetID string) bool {
	for _, c := range calls {
		if c.action == action && c.targetID == targetID {
			return true
		}
	}
	return false
}
