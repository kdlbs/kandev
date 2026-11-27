package sqlite_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// @covers AC-OFFICE-COSTS-001.2
func TestCostsWindowQueries(t *testing.T) {
	repo := newTestRepo(t)
	seedTasksTable(t, repo, "window-task", "window-ws")
	seedTasksTable(t, repo, "foreign-task", "foreign-ws")
	assertCostsWindowQueries(t, repo)
}

func windowSQL(t *testing.T, repo *sqlite.Repository, query string, args ...interface{}) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func seedCostsWindowEvents(t *testing.T, repo *sqlite.Repository, start, before time.Time) {
	t.Helper()
	for _, profile := range []struct{ id, name string }{{"window-agent", "Window Agent"}, {"deleted-profile", "Removed Agent"}} {
		windowSQL(t, repo, `INSERT INTO agent_profiles (id,agent_id,name,agent_display_name,created_at,updated_at) VALUES (?,?,?,?,?,?)`, profile.id, "claude-acp", profile.name, profile.name, start, start)
	}
	for _, id := range []string{"old-project", "new-project"} {
		windowSQL(t, repo, `INSERT INTO office_projects (id,workspace_id,name,created_at,updated_at) VALUES (?,?,?,?,?)`, id, "window-ws", id, start, start)
	}
	windowSQL(t, repo, `UPDATE tasks SET project_id=? WHERE id=?`, "new-project", "window-task")
	for _, row := range []struct{ id, ws string }{{"window-run", "window-ws"}, {"foreign-run", "foreign-ws"}} {
		windowSQL(t, repo, `INSERT INTO office_run_sessions (id,workspace_id,agent_profile_id,run_id,attempt,state,created_at) VALUES (?,?,?,?,?,?,?)`, row.id, row.ws, "window-agent", row.id, 1, "COMPLETED", start)
	}
	rows := []struct {
		id, task, session, agent, model, provider string
		at                                        time.Time
		cost                                      int64
	}{
		{"lower-out", "window-task", "", "window-agent", "excluded", "anthropic", start.Add(-time.Microsecond), 900},
		{"lower-in", "window-task", "", "window-agent", "same", "anthropic", start, 100},
		{"inside", "window-task", "", "window-agent", "same", "openai", start.Add(time.Hour), 200},
		{"zero", "window-task", "", "deleted-profile", "", "", start.Add(time.Hour), 0},
		{"run", "", "window-run", "window-agent", "run", "google", start.Add(time.Hour), 50},
		{"upper-out", "window-task", "", "window-agent", "excluded", "anthropic", before, 700},
		{"future", "window-task", "", "window-agent", "excluded", "anthropic", before.Add(time.Hour), 800},
		{"foreign-task-event", "foreign-task", "", "window-agent", "excluded", "anthropic", start.Add(time.Hour), 600},
		{"foreign-run-event", "", "foreign-run", "window-agent", "excluded", "anthropic", start.Add(time.Hour), 500},
		{"old-ingested-now", "window-task", "", "window-agent", "excluded", "anthropic", start.Add(-time.Hour), 400},
	}
	for _, row := range rows {
		windowSQL(t, repo, `INSERT INTO office_cost_events (id,task_id,session_id,agent_profile_id,project_id,model,provider,occurred_at,cost_subcents,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, row.id, row.task, row.session, row.agent, "old-project", row.model, row.provider, row.at, row.cost, start)
	}
	windowSQL(t, repo, `DELETE FROM agent_profiles WHERE id=?`, "deleted-profile")
	windowSQL(t, repo, `UPDATE office_cost_events SET created_at=? WHERE id=?`, start.Add(-24*time.Hour), "lower-in")
	windowSQL(t, repo, `UPDATE office_cost_events SET created_at=? WHERE id=?`, start.Add(time.Hour), "old-ingested-now")
}

func assertCostsWindowQueries(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	before := start.Add(24 * time.Hour)
	seedCostsWindowEvents(t, repo, start, before)
	// The same instants expressed in another zone must bind identically.
	zone := time.FixedZone("offset", 9*60*60)
	total, err := repo.SumCostsInWindow(ctx, "window-ws", start.In(zone), before.In(zone))
	if err != nil || total != 350 {
		t.Fatalf("total=%d/%v, want 350", total, err)
	}
	lifetime, err := repo.SumCosts(ctx, "window-ws")
	if err != nil || lifetime != 3150 {
		t.Fatalf("lifetime=%d/%v, want 3150", lifetime, err)
	}
	type entry struct {
		label string
		total int64
		count int
	}
	cases := []struct {
		name string
		read func(context.Context, string, time.Time, time.Time) ([]*models.CostBreakdown, error)
		want map[string]entry
	}{
		{"agent", repo.GetCostsByAgentInWindow, map[string]entry{"window-agent": {"Window Agent", 350, 3}, "deleted-profile": {"", 0, 1}}},
		{"project", repo.GetCostsByProjectInWindow, map[string]entry{"new-project": {"new-project", 300, 3}, "": {"", 50, 1}}},
		{"model", repo.GetCostsByModelInWindow, map[string]entry{"anthropic:same": {"Claude - same", 100, 1}, "openai:same": {"OpenAI - same", 200, 1}, "google:run": {"Gemini - run", 50, 1}, ":": {"", 0, 1}}},
		{"provider", repo.GetCostsByProviderInWindow, map[string]entry{"anthropic": {"Claude", 100, 1}, "openai": {"OpenAI", 200, 1}, "google": {"Gemini", 50, 1}, "unknown": {"(unknown)", 0, 1}}},
	}
	for _, tc := range cases {
		rows, readErr := tc.read(ctx, "window-ws", start.In(zone), before.In(zone))
		if readErr != nil {
			t.Fatal(readErr)
		}
		got := map[string]entry{}
		for _, row := range rows {
			got[row.GroupKey] = entry{row.GroupLabel, row.TotalSubcents, row.Count}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s=%+v, want %+v", tc.name, got, tc.want)
		}
		empty, readErr := tc.read(ctx, "window-ws", before, before)
		if readErr != nil || empty == nil || len(empty) != 0 {
			t.Errorf("%s empty=%v/%v", tc.name, empty, readErr)
		}
	}
	empty, err := repo.SumCostsInWindow(ctx, "window-ws", before, before)
	if err != nil || empty != 0 {
		t.Errorf("empty total=%d/%v", empty, err)
	}
}
