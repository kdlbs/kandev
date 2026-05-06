package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedTasksTable creates a minimal tasks table and inserts a task row so that
// cost queries (which JOIN on tasks) can resolve the workspace_id.
func seedTasksTable(t *testing.T, repo *sqlite.Repository, taskID, workspaceID string) {
	t.Helper()
	_, err := repo.ExecRaw(context.Background(),
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			project_id TEXT DEFAULT '',
			state TEXT NOT NULL DEFAULT 'TODO',
			title TEXT DEFAULT '',
			description TEXT DEFAULT '',
			identifier TEXT DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	_, err = repo.ExecRaw(context.Background(),
		`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, taskID, workspaceID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

func TestCostEvent_CreateAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedTasksTable(t, repo, "task-1", "ws-1")

	event := &models.CostEvent{
		SessionID:      "session-1",
		TaskID:         "task-1",
		AgentProfileID: "cost-agent-1",
		Model:          "claude-4-sonnet",
		Provider:       "anthropic",
		TokensIn:       1000,
		TokensOut:      500,
		CostSubcents:   10,
		OccurredAt:     time.Now().UTC(),
	}
	if err := repo.CreateCostEvent(ctx, event); err != nil {
		t.Fatalf("create cost: %v", err)
	}

	costs, err := repo.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}
	if costs[0].CostSubcents != 10 {
		t.Errorf("cost_subcents = %d, want 10", costs[0].CostSubcents)
	}
}

func TestCostBreakdowns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedTasksTable(t, repo, "task-bd", "ws-1")

	// Seed an agent profile and a project so the LEFT JOINs in
	// GetCostsByAgent / GetCostsByProject resolve a display name.
	mustExec(t, repo, `INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, created_at, updated_at)
		VALUES ('breakdown-agent-1', 'claude-acp', 'CEO', 'CEO',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	mustExec(t, repo, `INSERT INTO office_projects
		(id, workspace_id, name, created_at, updated_at)
		VALUES ('proj-1', 'ws-1', 'Acme Migration',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	for i := 0; i < 3; i++ {
		event := &models.CostEvent{
			TaskID:         "task-bd",
			AgentProfileID: "breakdown-agent-1",
			Model:          "claude-4-sonnet",
			ProjectID:      "proj-1",
			CostSubcents:   5,
			OccurredAt:     time.Now().UTC(),
		}
		if err := repo.CreateCostEvent(ctx, event); err != nil {
			t.Fatalf("create cost %d: %v", i, err)
		}
	}

	byAgent, err := repo.GetCostsByAgent(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by agent: %v", err)
	}
	if len(byAgent) != 1 || byAgent[0].TotalSubcents != 15 {
		t.Errorf("by agent: got %+v", byAgent)
	}
	if byAgent[0].GroupKey != "breakdown-agent-1" || byAgent[0].GroupLabel != "CEO" {
		t.Errorf("by agent label: got key=%q label=%q, want id+CEO",
			byAgent[0].GroupKey, byAgent[0].GroupLabel)
	}

	byProject, err := repo.GetCostsByProject(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by project: %v", err)
	}
	if len(byProject) != 1 || byProject[0].TotalSubcents != 15 {
		t.Errorf("by project: got %+v", byProject)
	}
	if byProject[0].GroupLabel != "Acme Migration" {
		t.Errorf("by project label = %q, want Acme Migration", byProject[0].GroupLabel)
	}

	byModel, err := repo.GetCostsByModel(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by model: %v", err)
	}
	if len(byModel) != 1 || byModel[0].TotalSubcents != 15 {
		t.Errorf("by model: got %+v", byModel)
	}
	// Provider was empty on the seeded events; group_key is ":model", label
	// falls back to the bare model id.
	if byModel[0].GroupKey != ":claude-4-sonnet" {
		t.Errorf("by model key = %q, want :claude-4-sonnet", byModel[0].GroupKey)
	}
	if byModel[0].GroupLabel != "claude-4-sonnet" {
		t.Errorf("by model label = %q, want claude-4-sonnet (bare model when provider empty)",
			byModel[0].GroupLabel)
	}

	byProvider, err := repo.GetCostsByProvider(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by provider: %v", err)
	}
	if len(byProvider) != 1 || byProvider[0].TotalSubcents != 15 {
		t.Errorf("by provider: got %+v", byProvider)
	}
	// Provider was empty on the seeded events; falls under the "unknown" bucket.
	if byProvider[0].GroupKey != "unknown" || byProvider[0].GroupLabel != "(unknown)" {
		t.Errorf("by provider = (key=%q,label=%q), want (unknown,(unknown))",
			byProvider[0].GroupKey, byProvider[0].GroupLabel)
	}
}

// TestCostBreakdowns_ProviderLabels confirms the friendly brand prefixes
// for the by-model and by-provider queries when the cost event carries a
// resolved provider (anthropic / openai / google).
func TestCostBreakdowns_ProviderLabels(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedTasksTable(t, repo, "task-pl", "ws-1")

	cases := []struct {
		model, provider, wantModelLabel, wantProviderLabel string
	}{
		{"default", "anthropic", "Claude - default", "Claude"},
		{"gpt-5.4-mini", "openai", "OpenAI - gpt-5.4-mini", "OpenAI"},
		{"gemini-3-flash-preview", "google", "Gemini - gemini-3-flash-preview", "Gemini"},
	}
	for _, c := range cases {
		if err := repo.CreateCostEvent(ctx, &models.CostEvent{
			TaskID: "task-pl", AgentProfileID: "agent-pl",
			Model: c.model, Provider: c.provider,
			CostSubcents: 100, OccurredAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed %s: %v", c.model, err)
		}
	}

	byModel, err := repo.GetCostsByModel(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by model: %v", err)
	}
	labels := map[string]string{}
	for _, row := range byModel {
		labels[row.GroupKey] = row.GroupLabel
	}
	for _, c := range cases {
		key := c.provider + ":" + c.model
		if labels[key] != c.wantModelLabel {
			t.Errorf("model label for %s = %q, want %q", key, labels[key], c.wantModelLabel)
		}
	}

	byProvider, err := repo.GetCostsByProvider(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by provider: %v", err)
	}
	provLabels := map[string]string{}
	for _, row := range byProvider {
		provLabels[row.GroupKey] = row.GroupLabel
	}
	for _, c := range cases {
		if provLabels[c.provider] != c.wantProviderLabel {
			t.Errorf("provider label for %s = %q, want %q",
				c.provider, provLabels[c.provider], c.wantProviderLabel)
		}
	}
}

func mustExec(t *testing.T, repo *sqlite.Repository, query string, args ...interface{}) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestSumCosts(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedTasksTable(t, repo, "task-sum", "ws-1")

	for i := 0; i < 3; i++ {
		event := &models.CostEvent{
			TaskID:       "task-sum",
			CostSubcents: 10,
			OccurredAt:   time.Now().UTC(),
		}
		if err := repo.CreateCostEvent(ctx, event); err != nil {
			t.Fatalf("create cost %d: %v", i, err)
		}
	}

	total, err := repo.SumCosts(ctx, "ws-1")
	if err != nil {
		t.Fatalf("SumCosts: %v", err)
	}
	if total != 30 {
		t.Errorf("total = %d, want 30", total)
	}

	// Different workspace should return 0.
	total2, err := repo.SumCosts(ctx, "ws-other")
	if err != nil {
		t.Fatalf("SumCosts other: %v", err)
	}
	if total2 != 0 {
		t.Errorf("total other = %d, want 0", total2)
	}
}

func TestGetCostForAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		event := &models.CostEvent{
			AgentProfileID: "agent-x",
			CostSubcents:   7,
			OccurredAt:     time.Now().UTC(),
		}
		if err := repo.CreateCostEvent(ctx, event); err != nil {
			t.Fatalf("create cost %d: %v", i, err)
		}
	}

	total, err := repo.GetCostForAgent(ctx, "agent-x")
	if err != nil {
		t.Fatalf("GetCostForAgent: %v", err)
	}
	if total != 14 {
		t.Errorf("total = %d, want 14", total)
	}
}

func TestGetCostForProject(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	event := &models.CostEvent{
		ProjectID:    "proj-y",
		CostSubcents: 25,
		OccurredAt:   time.Now().UTC(),
	}
	if err := repo.CreateCostEvent(ctx, event); err != nil {
		t.Fatalf("create cost: %v", err)
	}

	total, err := repo.GetCostForProject(ctx, "proj-y")
	if err != nil {
		t.Fatalf("GetCostForProject: %v", err)
	}
	if total != 25 {
		t.Errorf("total = %d, want 25", total)
	}
}

// TestPeriodAwareRollups confirms the *Since methods filter by
// occurred_at correctly. Seeds events across two calendar months and
// asserts the monthly window captures only this-month rows.
func TestPeriodAwareRollups(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedTasksTable(t, repo, "task-month", "ws-period")

	now := time.Now().UTC()
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	priorMonth := thisMonth.AddDate(0, -1, 0)

	mustCreateEvent := func(t *testing.T, occ time.Time, cost int64) {
		t.Helper()
		ev := &models.CostEvent{
			TaskID:         "task-month",
			AgentProfileID: "agent-period",
			ProjectID:      "proj-period",
			CostSubcents:   cost,
			OccurredAt:     occ,
		}
		if err := repo.CreateCostEvent(ctx, ev); err != nil {
			t.Fatalf("create event: %v", err)
		}
	}
	mustCreateEvent(t, priorMonth.Add(48*time.Hour), 100)
	mustCreateEvent(t, thisMonth.Add(time.Hour), 25)
	mustCreateEvent(t, thisMonth.Add(48*time.Hour), 50)

	lifetimeAgent, err := repo.GetCostForAgent(ctx, "agent-period")
	if err != nil {
		t.Fatalf("GetCostForAgent: %v", err)
	}
	if lifetimeAgent != 175 {
		t.Errorf("lifetime agent total = %d, want 175", lifetimeAgent)
	}

	monthlyAgent, err := repo.GetCostForAgentSince(ctx, "agent-period", thisMonth)
	if err != nil {
		t.Fatalf("GetCostForAgentSince: %v", err)
	}
	if monthlyAgent != 75 {
		t.Errorf("monthly agent total = %d, want 75 (only current month)", monthlyAgent)
	}

	monthlyProject, err := repo.GetCostForProjectSince(ctx, "proj-period", thisMonth)
	if err != nil {
		t.Fatalf("GetCostForProjectSince: %v", err)
	}
	if monthlyProject != 75 {
		t.Errorf("monthly project total = %d, want 75", monthlyProject)
	}

	monthlyWorkspace, err := repo.SumCostsSince(ctx, "ws-period", thisMonth)
	if err != nil {
		t.Fatalf("SumCostsSince: %v", err)
	}
	if monthlyWorkspace != 75 {
		t.Errorf("monthly workspace total = %d, want 75", monthlyWorkspace)
	}

	// Zero `since` is the lifetime path.
	lifetimeWS, err := repo.SumCostsSince(ctx, "ws-period", time.Time{})
	if err != nil {
		t.Fatalf("SumCostsSince zero: %v", err)
	}
	if lifetimeWS != 175 {
		t.Errorf("zero-since workspace total = %d, want 175", lifetimeWS)
	}
}
