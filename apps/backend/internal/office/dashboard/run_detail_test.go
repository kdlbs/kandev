package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/dashboard"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// runDetailDeps wires a fresh in-memory repo for run-detail tests.
// The dashboard service is not needed — GetRunDetail and
// ListAgentRunsPaged take the RunDetailRepo directly.
type runDetailDeps struct {
	db   *sqlx.DB
	repo *sqlite.Repository
}

func newRunDetailDeps(t *testing.T) *runDetailDeps {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return &runDetailDeps{db: db, repo: repo}
}

// seedRunDetailRun creates a Run with a fixed requested_at so tests can build
// deterministic ordered pages.
func seedRunDetailRun(
	t *testing.T, deps *runDetailDeps, agentID, status, taskID string, requestedAt time.Time,
) string {
	t.Helper()
	ctx := context.Background()
	run := &officemodels.Run{
		AgentProfileID: agentID,
		Reason:         "task_assigned",
		Payload:        `{"task_id":"` + taskID + `"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := deps.repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := deps.repo.SetRunRequestedAtForTest(ctx, run.ID, requestedAt); err != nil {
		t.Fatalf("set requested_at: %v", err)
	}
	if status != "queued" {
		if err := deps.repo.SetRunStatusForTest(ctx, run.ID, status, nil, nil); err != nil {
			t.Fatalf("set status: %v", err)
		}
	}
	return run.ID
}

// seedRunDetailAgent inserts an agent_profiles row (office-flavour) for the
// run-detail's invocation lookup. Returns the inserted id. The profileID
// argument is preserved for backwards-compat with callers that distinguish
// the legacy CLI profile id; under the merged schema the same value is the
// row id, so we ignore the parameter where it differs from id.
func seedRunDetailAgent(t *testing.T, deps *runDetailDeps, id, profileID string) {
	t.Helper()
	_ = profileID // unused: the office row is its own profile under the merged schema.
	_, err := deps.db.Exec(`
		INSERT INTO agent_profiles (
			id, agent_id, name, agent_display_name, model,
			workspace_id, role, status,
			created_at, updated_at
		) VALUES (?, '', ?, ?, '', ?, ?, ?, datetime('now'), datetime('now'))
	`, id, "Agent", "Agent", "ws-1", "ceo", "working")
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}
}

// seedRunDetailCostEvent inserts a cost row tied to a task so GetRunWithCosts
// can roll it up via the run payload's task_id.
func seedRunDetailCostEvent(t *testing.T, deps *runDetailDeps, taskID string, in, cached, out, cents int64) {
	t.Helper()
	_, err := deps.db.Exec(`
		INSERT INTO office_cost_events (
			id, session_id, task_id, agent_profile_id, project_id, model, provider,
			tokens_in, tokens_cached_in, tokens_out, cost_subcents, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, "cost-"+taskID, "", taskID, "", "", "claude", "anthropic", in, cached, out, cents)
	if err != nil {
		t.Fatalf("insert cost: %v", err)
	}
}

func TestListAgentRunsPaged_FirstPageNoCursor(t *testing.T) {
	deps := newRunDetailDeps(t)
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		// Older runs first so requested_at descends with i.
		seedRunDetailRun(t, deps, "agent-1", "finished", "task-x", base.Add(-time.Duration(i)*time.Minute))
	}

	resp, err := dashboard.ListAgentRunsPaged(context.Background(), deps.repo, "agent-1", "", "", 3)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := len(resp.Runs); got != 3 {
		t.Fatalf("want 3 runs, got %d", got)
	}
	if resp.NextCursor == "" {
		t.Fatalf("want next_cursor on partial page")
	}
	if resp.NextID == "" {
		t.Fatalf("want next_id on partial page")
	}
	// First row must be the most recent.
	if resp.Runs[0].RequestedAt != base.Format(time.RFC3339) {
		t.Errorf("want latest first, got %s", resp.Runs[0].RequestedAt)
	}
}

func TestListAgentRunsPaged_LastPageHasNoCursor(t *testing.T) {
	deps := newRunDetailDeps(t)
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedRunDetailRun(t, deps, "agent-1", "finished", "task-x", base)

	resp, err := dashboard.ListAgentRunsPaged(context.Background(), deps.repo, "agent-1", "", "", 25)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(resp.Runs))
	}
	if resp.NextCursor != "" {
		t.Errorf("want empty next_cursor on last page, got %q", resp.NextCursor)
	}
}

func TestListAgentRunsPaged_EmptyAgent(t *testing.T) {
	deps := newRunDetailDeps(t)
	resp, err := dashboard.ListAgentRunsPaged(context.Background(), deps.repo, "agent-empty", "", "", 25)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Runs) != 0 {
		t.Fatalf("want 0 runs, got %d", len(resp.Runs))
	}
	if resp.NextCursor != "" {
		t.Errorf("want empty next_cursor, got %q", resp.NextCursor)
	}
}

func TestListAgentRunsPaged_TieBreakOnID(t *testing.T) {
	deps := newRunDetailDeps(t)
	// Two runs with identical requested_at — must still be strictly
	// ordered by id DESC so pagination terminates.
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	id1 := seedRunDetailRun(t, deps, "agent-1", "finished", "task-x", at)
	id2 := seedRunDetailRun(t, deps, "agent-1", "finished", "task-y", at)
	want := id1
	if id2 > id1 {
		want = id2
	}

	resp, err := dashboard.ListAgentRunsPaged(context.Background(), deps.repo, "agent-1", "", "", 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(resp.Runs))
	}
	if resp.Runs[0].ID != want {
		t.Errorf("want larger id %s first, got %s", want, resp.Runs[0].ID)
	}
	// Page 2 must return the other row.
	resp2, err := dashboard.ListAgentRunsPaged(
		context.Background(), deps.repo, "agent-1",
		resp.NextCursor, resp.NextID, 1,
	)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(resp2.Runs) != 1 {
		t.Fatalf("want 1 run on page 2, got %d", len(resp2.Runs))
	}
	if resp2.Runs[0].ID == resp.Runs[0].ID {
		t.Errorf("page 2 returned same run as page 1: %s", resp.Runs[0].ID)
	}
}

func TestGetRunDetail_HappyPathWithCosts(t *testing.T) {
	deps := newRunDetailDeps(t)
	seedRunDetailAgent(t, deps, "agent-1", "claude_local")
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	runID := seedRunDetailRun(t, deps, "agent-1", "finished", "task-1", at)
	seedRunDetailCostEvent(t, deps, "task-1", 100, 5, 200, 42)

	resp, err := dashboard.GetRunDetail(context.Background(), deps.repo, "agent-1", runID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if resp.ID != runID {
		t.Errorf("id mismatch: got %s", resp.ID)
	}
	if len(resp.IDShort) != 8 {
		t.Errorf("id_short len = %d, want 8", len(resp.IDShort))
	}
	if resp.AgentID != "agent-1" {
		t.Errorf("agent id mismatch: %s", resp.AgentID)
	}
	if resp.Costs.InputTokens != 100 || resp.Costs.OutputTokens != 200 {
		t.Errorf("costs mismatch: %+v", resp.Costs)
	}
	if resp.Costs.CachedTokens != 5 {
		t.Errorf("cached tokens mismatch: %d", resp.Costs.CachedTokens)
	}
	if resp.Costs.CostSubcents != 42 {
		t.Errorf("cost subcents mismatch: %d", resp.Costs.CostSubcents)
	}
	if len(resp.TasksTouched) != 1 || resp.TasksTouched[0] != "task-1" {
		t.Errorf("want primary task in tasks_touched, got %v", resp.TasksTouched)
	}
	// After ADR 0005 the agent profile id IS the agent instance id, so
	// the adapter slug surfaced in the invocation panel matches the agent
	// row id directly. The legacy "office instance → shallow profile"
	// indirection is gone.
	if resp.Invocation.Adapter != "agent-1" {
		t.Errorf("adapter mismatch: %q", resp.Invocation.Adapter)
	}
}

func TestGetRunDetail_NotFound(t *testing.T) {
	deps := newRunDetailDeps(t)
	_, err := dashboard.GetRunDetail(context.Background(), deps.repo, "agent-1", "nope")
	if err == nil {
		t.Fatal("want error for unknown run id")
	}
	if err != dashboard.ErrRunNotFound {
		t.Errorf("want ErrRunNotFound, got %v", err)
	}
}

func TestGetRunDetail_AgentMismatch(t *testing.T) {
	deps := newRunDetailDeps(t)
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	runID := seedRunDetailRun(t, deps, "agent-1", "finished", "task-1", at)

	_, err := dashboard.GetRunDetail(context.Background(), deps.repo, "agent-2", runID)
	if err != dashboard.ErrRunAgentMismatch {
		t.Errorf("want ErrRunAgentMismatch, got %v", err)
	}
}

func TestGetRunDetail_EventsAreOrdered(t *testing.T) {
	deps := newRunDetailDeps(t)
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	runID := seedRunDetailRun(t, deps, "agent-1", "finished", "task-1", at)

	ctx := context.Background()
	for _, et := range []string{"init", "adapter.invoke", "step", "complete"} {
		if _, err := deps.repo.AppendRunEvent(ctx, runID, et, "info", "{}"); err != nil {
			t.Fatalf("append event %s: %v", et, err)
		}
	}

	resp, err := dashboard.GetRunDetail(ctx, deps.repo, "agent-1", runID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if got := len(resp.Events); got != 4 {
		t.Fatalf("want 4 events, got %d", got)
	}
	wantTypes := []string{"init", "adapter.invoke", "step", "complete"}
	for i, want := range wantTypes {
		if resp.Events[i].EventType != want {
			t.Errorf("event %d: want %s, got %s", i, want, resp.Events[i].EventType)
		}
		if resp.Events[i].Seq != i {
			t.Errorf("event %d: want seq %d, got %d", i, i, resp.Events[i].Seq)
		}
	}
}

// TestGetRunDetail_PromptArtifactsAndSnapshots asserts the new
// inspection fields added in PR 1 of office-heartbeat-rework round-
// trip through the response: assembled_prompt + summary_injected
// (persisted via UpdateRunPromptArtifacts), result_json, plus the
// existing-but-unsurfaced context_snapshot / output_summary.
func TestGetRunDetail_PromptArtifactsAndSnapshots(t *testing.T) {
	deps := newRunDetailDeps(t)
	seedRunDetailAgent(t, deps, "agent-1", "")
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	runID := seedRunDetailRun(t, deps, "agent-1", "finished", "task-1", at)

	ctx := context.Background()
	const (
		wantPrompt  = "## Active focus\nDo the thing."
		wantSummary = "## Active focus\nPrior summary."
		wantResult  = `{"summary":"All done."}`
	)
	if err := deps.repo.UpdateRunPromptArtifacts(ctx, runID, wantPrompt, wantSummary); err != nil {
		t.Fatalf("update prompt artifacts: %v", err)
	}
	if err := deps.repo.UpdateRunResultJSON(ctx, runID, wantResult); err != nil {
		t.Fatalf("update result_json: %v", err)
	}
	if err := deps.repo.UpdateRunOutputSummary(ctx, runID, "free-form output", ""); err != nil {
		t.Fatalf("update output_summary: %v", err)
	}

	resp, err := dashboard.GetRunDetail(ctx, deps.repo, "agent-1", runID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if resp.AssembledPrompt != wantPrompt {
		t.Errorf("assembled_prompt = %q, want %q", resp.AssembledPrompt, wantPrompt)
	}
	if resp.SummaryInjected != wantSummary {
		t.Errorf("summary_injected = %q, want %q", resp.SummaryInjected, wantSummary)
	}
	if resp.ResultJSON != wantResult {
		t.Errorf("result_json = %q, want %q", resp.ResultJSON, wantResult)
	}
	if resp.OutputSummary != "free-form output" {
		t.Errorf("output_summary = %q, want %q", resp.OutputSummary, "free-form output")
	}
	if resp.ContextSnapshot == "" {
		t.Errorf("expected context_snapshot to round-trip from default '{}', got empty string")
	}
}
