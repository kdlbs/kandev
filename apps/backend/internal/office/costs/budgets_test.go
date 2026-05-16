package costs_test

import (
	"context"
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
)

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

func newBudgetTestService(t *testing.T) (*costs.CostService, *sqlite.Repository, func(string, ...interface{})) {
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
	log := logger.Default()
	agents := &repoAgents{repo: repo}
	svc := costs.NewCostService(repo, log, &noopActivity{}, agents, agents)

	execSQL := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec sql: %v", err)
		}
	}
	return svc, repo, execSQL
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
