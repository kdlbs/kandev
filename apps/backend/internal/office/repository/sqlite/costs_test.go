package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
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

func TestCreateCostTables_DoesNotCreateUnusedSessionIDIndex(t *testing.T) {
	repo := newTestRepo(t)

	var name string
	err := repo.ReaderDB().QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'office_cost_events' AND sql LIKE '%session_id%'`,
	).Scan(&name)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("session_id index = %q, err=%v, want no index", name, err)
	}
}

func TestCostEvent_CreateAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedTasksTable(t, repo, "task-1", "ws-1")

	tokensOut := int64(500)
	event := &models.CostEvent{
		SessionID:      "session-1",
		TaskID:         "task-1",
		AgentProfileID: "cost-agent-1",
		Model:          "claude-4-sonnet",
		Provider:       "anthropic",
		TokensIn:       1000,
		TokensOut:      &tokensOut,
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
	if costs[0].TokensOut == nil || *costs[0].TokensOut != 500 {
		t.Errorf("tokens_out = %v, want 500", costs[0].TokensOut)
	}
}

// TestCostEvent_TokensOutNullRoundTrips confirms a NULL TokensOut (the
// "never measured" shape — see costContractVersion's v2→v3 doc comment in
// prompt_usage_cost.go) survives INSERT and SELECT as nil, not a silent 0.
func TestCostEvent_TokensOutNullRoundTrips(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedTasksTable(t, repo, "task-null-out", "ws-null-out")

	event := &models.CostEvent{
		SessionID:      "session-null-out",
		TaskID:         "task-null-out",
		AgentProfileID: "cost-agent-null-out",
		Model:          "opus",
		Provider:       "anthropic",
		TokensIn:       1000,
		TokensOut:      nil,
		CostSubcents:   767,
		Estimated:      true,
		OccurredAt:     time.Now().UTC(),
	}
	if err := repo.CreateCostEvent(ctx, event); err != nil {
		t.Fatalf("create cost: %v", err)
	}

	costs, err := repo.ListCostEvents(ctx, "ws-null-out")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}
	if costs[0].TokensOut != nil {
		t.Errorf("tokens_out = %v, want nil (NULL round-trip)", *costs[0].TokensOut)
	}
	if costs[0].CostSubcents != 767 {
		t.Errorf("cost_subcents = %d, want 767 (real money attached to an unmeasured tokens_out row)", costs[0].CostSubcents)
	}
}

// TestCreateCostEvent_DuplicateUsageEventID confirms redelivery of the same
// prompt-usage bus event (same UsageEventID) is reported as
// ErrDuplicateUsageEvent rather than inserting a second row or a raw driver
// error, and that unrelated rows with a NULL UsageEventID never collide.
func TestCreateCostEvent_DuplicateUsageEventID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedTasksTable(t, repo, "task-dup", "ws-dup")

	usageEventID := "usage-evt-1"
	first := &models.CostEvent{
		TaskID:       "task-dup",
		CostSubcents: 10,
		UsageEventID: &usageEventID,
		OccurredAt:   time.Now().UTC(),
	}
	if err := repo.CreateCostEvent(ctx, first); err != nil {
		t.Fatalf("create first: %v", err)
	}

	second := &models.CostEvent{
		TaskID:       "task-dup",
		CostSubcents: 10,
		UsageEventID: &usageEventID,
		OccurredAt:   time.Now().UTC(),
	}
	err := repo.CreateCostEvent(ctx, second)
	if !errors.Is(err, sqlite.ErrDuplicateUsageEvent) {
		t.Fatalf("create second (duplicate) err = %v, want ErrDuplicateUsageEvent", err)
	}

	costs, err := repo.ListCostEvents(ctx, "ws-dup")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1 (duplicate must not insert)", len(costs))
	}

	// Two rows with no UsageEventID (manual entries, or rows predating this
	// field) must not collide with each other.
	for i := 0; i < 2; i++ {
		if err := repo.CreateCostEvent(ctx, &models.CostEvent{
			TaskID: "task-dup", CostSubcents: 5, OccurredAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("create nil-usage-event row %d: %v", i, err)
		}
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
	// GetCostsByProject attributes by the task's live project_id, not the
	// event snapshot, so the task itself must carry the assignment.
	mustExec(t, repo, `UPDATE tasks SET project_id = 'proj-1' WHERE id = 'task-bd'`)

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

// TestGetCostsByProject_FollowsLiveReassignment confirms a task's entire
// cost history moves to a project the moment the task is assigned to it,
// even though the cost events were recorded before the assignment existed.
// office_cost_events.project_id is a write-time snapshot (event_subscribers.go)
// and must never be consulted for attribution; only the task's current
// project_id is authoritative. This is the regression for the "assigning a
// finished task to a project doesn't move its cost" report.
//
// It also covers the project-budget reads (GetCostForProject,
// GetCostForProjectSince), which switched from the snapshot filter to the
// same live tasks.project_id join in the same change. The seeded event's
// snapshot project_id is left empty (never set on the event below) so these
// assertions actually discriminate the live join from the old snapshot
// filter: a snapshot-based implementation would return 0 in both the
// "before" and "after" case, since the snapshot column never changes.
func TestGetCostsByProject_FollowsLiveReassignment(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedTasksTable(t, repo, "task-reassign", "ws-reassign")
	mustExec(t, repo, `INSERT INTO office_projects
		(id, workspace_id, name, created_at, updated_at)
		VALUES ('proj-reassign', 'ws-reassign', 'Kandev',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	windowStart := time.Now().UTC().Add(-time.Hour)

	// Recorded while the task had no project assigned, so the snapshot
	// column (office_cost_events.project_id) is empty on this row.
	event := &models.CostEvent{
		TaskID:       "task-reassign",
		CostSubcents: 2013,
		OccurredAt:   time.Now().UTC(),
	}
	if err := repo.CreateCostEvent(ctx, event); err != nil {
		t.Fatalf("create cost: %v", err)
	}

	byProject, err := repo.GetCostsByProject(ctx, "ws-reassign")
	if err != nil {
		t.Fatalf("by project (before assignment): %v", err)
	}
	if len(byProject) != 1 || byProject[0].GroupKey != "" || byProject[0].TotalSubcents != 2013 {
		t.Fatalf("before assignment: got %+v, want one unassigned row of 2013", byProject)
	}

	budgetTotal, err := repo.GetCostForProject(ctx, "proj-reassign")
	if err != nil {
		t.Fatalf("budget total (before assignment): %v", err)
	}
	if budgetTotal != 0 {
		t.Errorf("budget total (before assignment) = %d, want 0 (task not yet assigned to the project)",
			budgetTotal)
	}
	budgetSince, err := repo.GetCostForProjectSince(ctx, "proj-reassign", windowStart)
	if err != nil {
		t.Fatalf("budget since (before assignment): %v", err)
	}
	if budgetSince != 0 {
		t.Errorf("budget since (before assignment) = %d, want 0 (task not yet assigned to the project)",
			budgetSince)
	}

	// The user assigns the (already-finished) task to the project. No new
	// cost event is recorded.
	mustExec(t, repo, `UPDATE tasks SET project_id = 'proj-reassign' WHERE id = 'task-reassign'`)

	byProject, err = repo.GetCostsByProject(ctx, "ws-reassign")
	if err != nil {
		t.Fatalf("by project (after assignment): %v", err)
	}
	if len(byProject) != 1 {
		t.Fatalf("after assignment: got %+v, want a single row", byProject)
	}
	if byProject[0].GroupKey != "proj-reassign" || byProject[0].GroupLabel != "Kandev" {
		t.Errorf("after assignment: got key=%q label=%q, want proj-reassign/Kandev",
			byProject[0].GroupKey, byProject[0].GroupLabel)
	}
	if byProject[0].TotalSubcents != 2013 {
		t.Errorf("after assignment: total = %d, want 2013 (unchanged, just re-attributed)",
			byProject[0].TotalSubcents)
	}

	budgetTotal, err = repo.GetCostForProject(ctx, "proj-reassign")
	if err != nil {
		t.Fatalf("budget total (after assignment): %v", err)
	}
	if budgetTotal != 2013 {
		t.Errorf("budget total (after assignment) = %d, want 2013 (budget must count the reassigned event)",
			budgetTotal)
	}
	budgetSince, err = repo.GetCostForProjectSince(ctx, "proj-reassign", windowStart)
	if err != nil {
		t.Fatalf("budget since (after assignment): %v", err)
	}
	if budgetSince != 2013 {
		t.Errorf("budget since (after assignment) = %d, want 2013 (budget must count the reassigned event)",
			budgetSince)
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

	seedTasksTable(t, repo, "task-proj-y", "ws-proj-y")
	mustExec(t, repo, `UPDATE tasks SET project_id = 'proj-y' WHERE id = 'task-proj-y'`)

	event := &models.CostEvent{
		TaskID: "task-proj-y",
		// Leave the event snapshot empty so this assertion requires the live
		// task.project_id join instead of passing from the legacy field.
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
	mustExec(t, repo, `UPDATE tasks SET project_id = 'proj-period' WHERE id = 'task-month'`)

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
