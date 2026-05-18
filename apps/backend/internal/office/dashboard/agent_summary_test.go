package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/dashboard"
)

// summaryFixtureAgent is the agent id every test in this file uses
// for activity / runs / costs. Stable so the assertions don't have to
// recompute it for each row.
const summaryFixtureAgent = "agent-A"

// seedSummaryRun inserts a single runs row at the supplied
// timestamp with the supplied status. Reason and payload default to
// sane values so callers only specify the fields under test. Distinct
// from run_detail_test.go's seedRun helper, which writes via the repo
// (this one writes raw SQL so we control claimed_at + finished_at).
func seedSummaryRun(t *testing.T, db *sqlx.DB, runID, agentID, status, taskID string, requestedAt time.Time) {
	t.Helper()
	payload := `{"task_id":"` + taskID + `"}`
	// Give the run a synthetic 1h duration: claimed_at = requested_at,
	// finished_at = requested_at + 1h. The cost-events join uses
	// occurred_at BETWEEN claimed_at AND finished_at, so cost events
	// seeded a few minutes after the run's start need that window to
	// be open enough to include them.
	finishedAt := requestedAt.Add(1 * time.Hour)
	_, err := db.Exec(`
		INSERT INTO runs
			(id, agent_profile_id, reason, payload, status, coalesced_count,
			 retry_count, requested_at, claimed_at, finished_at)
		VALUES (?, ?, 'task_assigned', ?, ?, 1, 0, ?, ?, ?)
	`, runID, agentID, payload, status, requestedAt.UTC().Format(time.RFC3339),
		requestedAt.UTC().Format(time.RFC3339), finishedAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed run %s: %v", runID, err)
	}
}

// seedSummaryActivity inserts an office_activity_log row attributing
// the action to the agent. The dashboard summary uses these rows to
// compute the per-day priority/status charts and the recent-tasks
// list.
func seedSummaryActivity(t *testing.T, db *sqlx.DB, agentID, taskID string, createdAt time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO office_activity_log
			(id, workspace_id, actor_type, actor_id, action,
			 target_type, target_id, details, run_id, session_id, created_at)
		VALUES (?, 'ws-summary', 'agent', ?, 'task.update', 'task', ?, '{}', '', '', ?)
	`, "act-"+taskID+"-"+createdAt.Format("20060102150405"), agentID, taskID,
		createdAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed activity for %s: %v", taskID, err)
	}
}

// seedSummaryCost inserts a single cost row for the agent. Used to
// pin the cost_aggregate JSON.
func seedSummaryCost(
	t *testing.T, db *sqlx.DB, eventID, agentID, taskID string,
	tokensIn, tokensOut, cachedIn, costCents int, occurredAt time.Time,
) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO office_cost_events
			(id, session_id, task_id, agent_profile_id, project_id, model, provider,
			 tokens_in, tokens_cached_in, tokens_out, cost_subcents, occurred_at, created_at)
		VALUES (?, '', ?, ?, '', 'sonnet', 'anthropic', ?, ?, ?, ?, ?, ?)
	`, eventID, taskID, agentID, tokensIn, cachedIn, tokensOut, costCents,
		occurredAt.UTC().Format(time.RFC3339), occurredAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed cost event %s: %v", eventID, err)
	}
}

// fetchAgentSummary calls the new GET endpoint and returns the parsed
// response. Status code != 200 fails the test so callers can focus on
// the JSON shape.
func fetchAgentSummary(t *testing.T, deps *testDeps, agentID string, days int) dashboard.AgentDashboardSummary {
	t.Helper()
	url := "/api/v1/office/agents/" + agentID + "/summary"
	if days > 0 {
		url += "?days=" + strconv.Itoa(days)
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("agent summary: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp dashboard.AgentDashboardSummary
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	return resp
}

func TestGetAgentSummary_EmptyAgent(t *testing.T) {
	deps := newTestDeps(t)

	resp := fetchAgentSummary(t, deps, summaryFixtureAgent, 14)

	if resp.AgentID != summaryFixtureAgent {
		t.Errorf("agent_id = %q, want %q", resp.AgentID, summaryFixtureAgent)
	}
	if resp.Days != 14 {
		t.Errorf("days = %d, want 14", resp.Days)
	}
	if resp.LatestRun != nil {
		t.Errorf("expected no latest_run, got %#v", resp.LatestRun)
	}
	if len(resp.RunActivity) != 14 {
		t.Errorf("run_activity length = %d, want 14 (padded)", len(resp.RunActivity))
	}
	if len(resp.SuccessRate) != 14 {
		t.Errorf("success_rate length = %d, want 14", len(resp.SuccessRate))
	}
	if resp.CostAggregate.TotalCostSubcents != 0 {
		t.Errorf("cost_aggregate.total_cost_subcents = %d, want 0",
			resp.CostAggregate.TotalCostSubcents)
	}
	if len(resp.RecentTasks) != 0 {
		t.Errorf("recent_tasks = %d, want 0", len(resp.RecentTasks))
	}
	if len(resp.RecentRunCosts) != 0 {
		t.Errorf("recent_run_costs = %d, want 0", len(resp.RecentRunCosts))
	}
}

func TestGetAgentSummary_DaysClampedToWindow(t *testing.T) {
	deps := newTestDeps(t)

	resp := fetchAgentSummary(t, deps, summaryFixtureAgent, 999)
	if resp.Days != 90 {
		t.Errorf("days = %d, want clamped to 90", resp.Days)
	}
	if len(resp.RunActivity) != 90 {
		t.Errorf("run_activity = %d, want 90 entries", len(resp.RunActivity))
	}
}

func TestGetAgentSummary_LatestRunCard(t *testing.T) {
	deps := newTestDeps(t)
	now := time.Now().UTC()

	// Insert two runs; the most recent should win.
	seedSummaryRun(t, deps.db, "run-old", summaryFixtureAgent, "finished", "task-1",
		now.Add(-2*time.Hour))
	seedSummaryRun(t, deps.db, "run-newest-id", summaryFixtureAgent, "failed", "task-2",
		now.Add(-30*time.Minute))

	resp := fetchAgentSummary(t, deps, summaryFixtureAgent, 14)
	if resp.LatestRun == nil {
		t.Fatal("expected latest_run, got nil")
	}
	if resp.LatestRun.RunID != "run-newest-id" {
		t.Errorf("latest run id = %q, want run-newest-id", resp.LatestRun.RunID)
	}
	if resp.LatestRun.RunIDShort != "run-newe" {
		t.Errorf("short id = %q, want first 8 chars", resp.LatestRun.RunIDShort)
	}
	if resp.LatestRun.Status != "failed" {
		t.Errorf("status = %q, want failed", resp.LatestRun.Status)
	}
	if resp.LatestRun.TaskID != "task-2" {
		t.Errorf("task_id = %q, want task-2", resp.LatestRun.TaskID)
	}
}

// TestGetAgentSummary_RunActivityBuckets seeds 3 succeeded + 1 failed
// run across 2 days and pins the per-day stacked counts.
func TestGetAgentSummary_RunActivityBuckets(t *testing.T) {
	deps := newTestDeps(t)
	now := time.Now().UTC()
	d0 := now
	d1 := now.AddDate(0, 0, -1)

	seedSummaryRun(t, deps.db, "r1", summaryFixtureAgent, "finished", "t1", d0)
	seedSummaryRun(t, deps.db, "r2", summaryFixtureAgent, "finished", "t1", d0)
	seedSummaryRun(t, deps.db, "r3", summaryFixtureAgent, "finished", "t2", d1)
	seedSummaryRun(t, deps.db, "r4", summaryFixtureAgent, "failed", "t2", d1)

	resp := fetchAgentSummary(t, deps, summaryFixtureAgent, 14)

	bucketsByDate := map[string]dashboard.AgentRunActivityDay{}
	for _, b := range resp.RunActivity {
		bucketsByDate[b.Date] = b
	}
	d0Key := d0.Format("2006-01-02")
	d1Key := d1.Format("2006-01-02")
	if got := bucketsByDate[d0Key]; got.Succeeded != 2 || got.Total != 2 {
		t.Errorf("today bucket %#v, want succeeded=2 total=2", got)
	}
	if got := bucketsByDate[d1Key]; got.Succeeded != 1 || got.Failed != 1 || got.Total != 2 {
		t.Errorf("yesterday bucket %#v, want succeeded=1 failed=1 total=2", got)
	}

	// Success rate — today is 2/2, yesterday is 1/2.
	srByDate := map[string]dashboard.AgentSuccessRateDay{}
	for _, sr := range resp.SuccessRate {
		srByDate[sr.Date] = sr
	}
	if sr := srByDate[d0Key]; sr.Succeeded != 2 || sr.Total != 2 {
		t.Errorf("success rate today %#v, want 2/2", sr)
	}
	if sr := srByDate[d1Key]; sr.Succeeded != 1 || sr.Total != 2 {
		t.Errorf("success rate yesterday %#v, want 1/2", sr)
	}
}

// TestGetAgentSummary_TaskBuckets pins the priority + status charts.
func TestGetAgentSummary_TaskBuckets(t *testing.T) {
	deps := newTestDeps(t)
	now := time.Now().UTC()

	insertTestTask(t, deps.db, "task-crit", "ws-summary", "Critical", "TODO", 4)
	insertTestTask(t, deps.db, "task-high", "ws-summary", "High", "IN_PROGRESS", 3)
	insertTestTask(t, deps.db, "task-low", "ws-summary", "Low", "COMPLETED", 1)

	seedSummaryActivity(t, deps.db, summaryFixtureAgent, "task-crit", now)
	seedSummaryActivity(t, deps.db, summaryFixtureAgent, "task-high", now)
	seedSummaryActivity(t, deps.db, summaryFixtureAgent, "task-low", now)

	resp := fetchAgentSummary(t, deps, summaryFixtureAgent, 14)

	dayKey := now.Format("2006-01-02")
	var pri *dashboard.AgentTaskPriorityDay
	for i := range resp.TasksByPriority {
		if resp.TasksByPriority[i].Date == dayKey {
			pri = &resp.TasksByPriority[i]
		}
	}
	if pri == nil {
		t.Fatalf("today not in tasks_by_priority")
	}
	if pri.Critical != 1 || pri.High != 1 || pri.Low != 1 {
		t.Errorf("priority bucket %#v, want critical=1 high=1 low=1", pri)
	}

	var st *dashboard.AgentTaskStatusDay
	for i := range resp.TasksByStatus {
		if resp.TasksByStatus[i].Date == dayKey {
			st = &resp.TasksByStatus[i]
		}
	}
	if st == nil {
		t.Fatalf("today not in tasks_by_status")
	}
	if st.Todo != 1 || st.InProgress != 1 || st.Done != 1 {
		t.Errorf("status bucket %#v, want todo=1 in_progress=1 done=1", st)
	}
}

// TestGetAgentSummary_RecentTasksAndCosts pins the recent_tasks list,
// the cost aggregate and per-run cost rollups.
func TestGetAgentSummary_RecentTasksAndCosts(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insertTestTask(t, deps.db, "task-a", "ws-summary", "Task A", "IN_PROGRESS", 3)
	insertTestTask(t, deps.db, "task-b", "ws-summary", "Task B", "COMPLETED", 2)

	seedSummaryActivity(t, deps.db, summaryFixtureAgent, "task-a", now.Add(-1*time.Hour))
	seedSummaryActivity(t, deps.db, summaryFixtureAgent, "task-b", now.Add(-30*time.Minute))

	// Run + cost events for the per-run rollup.
	seedSummaryRun(t, deps.db, "run-cost-1", summaryFixtureAgent, "finished", "task-a",
		now.Add(-50*time.Minute))
	seedSummaryCost(t, deps.db, "ce-1", summaryFixtureAgent, "task-a",
		100, 200, 50, 12, now.Add(-45*time.Minute))
	seedSummaryCost(t, deps.db, "ce-2", summaryFixtureAgent, "task-a",
		10, 20, 5, 1, now.Add(-44*time.Minute))

	// Sanity check: repository helper should also see the agg.
	if _, err := deps.repo.CostAggregateForAgent(ctx, summaryFixtureAgent); err != nil {
		t.Fatalf("repo CostAggregateForAgent: %v", err)
	}

	resp := fetchAgentSummary(t, deps, summaryFixtureAgent, 14)

	if len(resp.RecentTasks) != 2 {
		t.Fatalf("recent_tasks = %d, want 2", len(resp.RecentTasks))
	}
	if resp.RecentTasks[0].TaskID != "task-b" {
		t.Errorf("most-recent task id = %q, want task-b", resp.RecentTasks[0].TaskID)
	}
	if resp.RecentTasks[0].Status != "done" {
		t.Errorf("task-b status = %q, want done", resp.RecentTasks[0].Status)
	}

	if resp.CostAggregate.InputTokens != 110 ||
		resp.CostAggregate.OutputTokens != 220 ||
		resp.CostAggregate.CachedTokens != 55 ||
		resp.CostAggregate.TotalCostSubcents != 13 {
		t.Errorf("cost_aggregate = %#v, want sums of seeded events", resp.CostAggregate)
	}

	if len(resp.RecentRunCosts) != 1 {
		t.Fatalf("recent_run_costs = %d, want 1", len(resp.RecentRunCosts))
	}
	rc := resp.RecentRunCosts[0]
	if rc.RunID != "run-cost-1" {
		t.Errorf("run cost run_id = %q, want run-cost-1", rc.RunID)
	}
	if rc.InputTokens != 110 || rc.OutputTokens != 220 || rc.CostSubcents != 13 {
		t.Errorf("run cost rollup = %#v, want 110/220/13", rc)
	}
	if rc.RunIDShort != "run-cost" {
		t.Errorf("run_id_short = %q, want first 8 chars", rc.RunIDShort)
	}
}
