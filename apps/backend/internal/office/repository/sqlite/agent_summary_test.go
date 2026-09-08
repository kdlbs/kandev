package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedSummaryRun inserts a runs row with explicit timestamps. Timestamps
// are passed as plain "YYYY-MM-DD HH:MM:SS" strings so every comparison
// in the summary queries (which mix stored values with datetime('now'))
// compares like-for-like.
func seedSummaryRun(
	t *testing.T, repo *sqlite.Repository,
	id, agentID, status, requestedAt, claimedAt, finishedAt, payload string,
) {
	t.Helper()
	var claimed, finished interface{}
	if claimedAt != "" {
		claimed = claimedAt
	}
	if finishedAt != "" {
		finished = finishedAt
	}
	if payload == "" {
		payload = "{}"
	}
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO runs
			(id, agent_profile_id, reason, payload, status, error_message, requested_at, claimed_at, finished_at)
		VALUES (?, ?, 'test', ?, ?, '', ?, ?, ?)
	`, id, agentID, payload, status, requestedAt, claimed, finished); err != nil {
		t.Fatalf("seed run %s: %v", id, err)
	}
}

// seedSummaryRunWithOutcome is seedSummaryRun plus an explicit outcome
// value, for the task-delivery-ledger Office run outcome bucketing tests.
// outcome == "" writes SQL NULL (pre-activation / failed-path shape).
func seedSummaryRunWithOutcome(
	t *testing.T, repo *sqlite.Repository,
	id, agentID, status, requestedAt, outcome string,
) {
	t.Helper()
	var outcomeArg interface{}
	if outcome != "" {
		outcomeArg = outcome
	}
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO runs
			(id, agent_profile_id, reason, payload, status, error_message, outcome, requested_at)
		VALUES (?, ?, 'test', '{}', ?, '', ?, ?)
	`, id, agentID, status, outcomeArg, requestedAt); err != nil {
		t.Fatalf("seed run %s: %v", id, err)
	}
}

// seedCostEvent inserts an office_cost_events row with every summed column set.
func seedCostEvent(
	t *testing.T, repo *sqlite.Repository,
	id, agentID, taskID, occurredAt string, in, cachedIn, out, subcents int,
) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO office_cost_events
			(id, session_id, task_id, agent_profile_id, project_id, model, provider,
			 tokens_in, tokens_cached_in, tokens_out, cost_subcents, estimated, occurred_at, created_at)
		VALUES (?, 'sess', ?, ?, '', 'model', 'provider', ?, ?, ?, ?, 0, ?, ?)
	`, id, taskID, agentID, in, cachedIn, out, subcents, occurredAt, occurredAt); err != nil {
		t.Fatalf("seed cost event %s: %v", id, err)
	}
}

// seedActivity inserts an office_activity_log row.
func seedActivity(t *testing.T, repo *sqlite.Repository, id, actorID, targetType, targetID, createdAt string) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO office_activity_log
			(id, workspace_id, actor_type, actor_id, action, target_type, target_id, details, created_at)
		VALUES (?, 'ws-1', 'agent', ?, 'worked', ?, ?, '{}', ?)
	`, id, actorID, targetType, targetID, createdAt); err != nil {
		t.Fatalf("seed activity %s: %v", id, err)
	}
}

// TestRunCountsByDayForAgent_BucketsStatuses covers the five-bucket
// contract in docs/specs/task-delivery-ledger/spec.md, "Office run
// outcome § Repository query": succeeded requires status='finished' AND
// outcome='processed'; any other non-NULL outcome on a finished run is
// skipped; a finished run with outcome IS NULL (pre-activation, or a
// terminal path with no established semantic label) is unclassified;
// failed/other are unchanged by this card.
func TestRunCountsByDayForAgent_BucketsStatuses(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Now().UTC()

	today, yday, ancient := dayStamp(base, 0, 10), dayStamp(base, 1, 10), dayStamp(base, 60, 10)
	seedSummaryRunWithOutcome(t, repo, "r-fin", "agent-a", "finished", today, "processed")
	seedSummaryRunWithOutcome(t, repo, "r-skip", "agent-a", "finished", today, "budget_blocked")
	seedSummaryRunWithOutcome(t, repo, "r-unclassified", "agent-a", "finished", today, "")
	seedSummaryRun(t, repo, "r-fail", "agent-a", "failed", today, "", "", "")
	seedSummaryRun(t, repo, "r-timeout", "agent-a", "timed_out", today, "", "", "")
	seedSummaryRun(t, repo, "r-queued", "agent-a", "queued", today, "", "", "")
	seedSummaryRun(t, repo, "r-cancel", "agent-a", "cancelled", today, "", "", "")
	seedSummaryRunWithOutcome(t, repo, "r-yday", "agent-a", "finished", yday, "")
	seedSummaryRunWithOutcome(t, repo, "r-old", "agent-a", "finished", ancient, "processed")
	seedSummaryRunWithOutcome(t, repo, "r-other", "agent-b", "finished", today, "processed")

	rows, err := repo.RunCountsByDayForAgent(ctx, "agent-a", 14)
	if err != nil {
		t.Fatalf("RunCountsByDayForAgent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (today + yesterday, the 60-day-old row is past the cutoff): %+v",
			len(rows), rows)
	}
	if rows[0].Date != dayDate(base, 1) {
		t.Errorf("rows[0].Date = %q, want %q (ORDER BY date ascending)", rows[0].Date, dayDate(base, 1))
	}
	if want := (sqlite.AgentRunDayRow{Date: dayDate(base, 1), Unclassified: 1}); rows[0] != want {
		t.Errorf("yesterday = %+v, want %+v (NULL outcome is unclassified, not succeeded)", rows[0], want)
	}
	want := sqlite.AgentRunDayRow{
		Date: dayDate(base, 0), Succeeded: 1, Skipped: 1, Unclassified: 1, Failed: 2, Other: 2,
	}
	if rows[1] != want {
		t.Errorf("today = %+v, want %+v (timed_out counts as failed; queued+cancelled as other)", rows[1], want)
	}
}

// TestRunCountsByDayForAgent_UnrecognisedOutcomeCountsSkipped covers the
// bucketing-is-total scenario: a finished run whose outcome holds a value
// outside the eight named ones still counts in skipped, and the query
// never errors.
func TestRunCountsByDayForAgent_UnrecognisedOutcomeCountsSkipped(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Now().UTC()
	today := dayStamp(base, 0, 10)

	seedSummaryRunWithOutcome(t, repo, "r-weird", "agent-a", "finished", today, "some_future_value")

	rows, err := repo.RunCountsByDayForAgent(ctx, "agent-a", 14)
	if err != nil {
		t.Fatalf("RunCountsByDayForAgent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(rows), rows)
	}
	want := sqlite.AgentRunDayRow{Date: dayDate(base, 0), Skipped: 1}
	if rows[0] != want {
		t.Errorf("today = %+v, want %+v", rows[0], want)
	}
}

func TestRunCountsByDayForAgent_NonPositiveDaysDefaultsTo14(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Now().UTC()

	seedSummaryRun(t, repo, "r-in", "agent-a", "finished", dayStamp(base, 10, 10), "", "", "")
	seedSummaryRun(t, repo, "r-out", "agent-a", "finished", dayStamp(base, 20, 10), "", "", "")

	rows, err := repo.RunCountsByDayForAgent(ctx, "agent-a", 0)
	if err != nil {
		t.Fatalf("RunCountsByDayForAgent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (14-day default window): %+v", len(rows), rows)
	}
	if rows[0].Date != dayDate(base, 10) {
		t.Errorf("date = %q, want %q", rows[0].Date, dayDate(base, 10))
	}
}

func TestTasksByPriorityByDayForAgent(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	base := time.Now().UTC()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, priority, created_at, updated_at) VALUES
			('pt-crit',   'ws-1', 'a', 'critical', datetime('now'), datetime('now')),
			('pt-high',   'ws-1', 'b', 'high',     datetime('now'), datetime('now')),
			('pt-med',    'ws-1', 'c', 'medium',   datetime('now'), datetime('now')),
			('pt-low',    'ws-1', 'd', 'low',      datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	today, yday := dayStamp(base, 0, 10), dayStamp(base, 1, 10)
	// Two activity rows on the same (day, task) must collapse to one count.
	seedActivity(t, repo, "a-1", "agent-a", "task", "pt-crit", today)
	seedActivity(t, repo, "a-2", "agent-a", "task", "pt-crit", dayStamp(base, 0, 14))
	seedActivity(t, repo, "a-3", "agent-a", "task", "pt-high", today)
	seedActivity(t, repo, "a-4", "agent-a", "task", "pt-med", yday)
	seedActivity(t, repo, "a-5", "agent-a", "task", "pt-low", yday)
	// Filtered out: wrong actor, wrong target type, empty target, unjoinable target.
	seedActivity(t, repo, "a-6", "agent-b", "task", "pt-crit", today)
	seedActivity(t, repo, "a-7", "agent-a", "run", "pt-crit", today)
	seedActivity(t, repo, "a-8", "agent-a", "task", "", today)
	seedActivity(t, repo, "a-9", "agent-a", "task", "no-such-task", today)

	rows, err := repo.TasksByPriorityByDayForAgent(ctx, "agent-a", 14)
	if err != nil {
		t.Fatalf("TasksByPriorityByDayForAgent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 days: %+v", len(rows), rows)
	}
	wantYday := sqlite.AgentTaskPriorityDayRow{Date: dayDate(base, 1), Medium: 1, Low: 1}
	if rows[0] != wantYday {
		t.Errorf("yesterday = %+v, want %+v", rows[0], wantYday)
	}
	wantToday := sqlite.AgentTaskPriorityDayRow{Date: dayDate(base, 0), Critical: 1, High: 1}
	if rows[1] != wantToday {
		t.Errorf("today = %+v, want %+v (two activity rows on one task count once)", rows[1], wantToday)
	}
}

func TestTasksByStatusByDayForAgent(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	base := time.Now().UTC()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, state, created_at, updated_at) VALUES
			('st-todo',   'ws-1', 'a', 'TODO',        datetime('now'), datetime('now')),
			('st-prog',   'ws-1', 'b', 'IN_PROGRESS', datetime('now'), datetime('now')),
			('st-review', 'ws-1', 'c', 'REVIEW',      datetime('now'), datetime('now')),
			('st-done',   'ws-1', 'd', 'COMPLETED',   datetime('now'), datetime('now')),
			('st-block',  'ws-1', 'e', 'BLOCKED',     datetime('now'), datetime('now')),
			('st-cancel', 'ws-1', 'f', 'CANCELLED',   datetime('now'), datetime('now')),
			('st-backlog','ws-1', 'g', 'BACKLOG',     datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	today := dayStamp(base, 0, 10)
	for i, id := range []string{
		"st-todo", "st-prog", "st-review", "st-done", "st-block", "st-cancel", "st-backlog",
	} {
		seedActivity(t, repo, string(rune('A'+i)), "agent-a", "task", id, today)
	}
	seedActivity(t, repo, "other", "agent-b", "task", "st-todo", today)

	rows, err := repo.TasksByStatusByDayForAgent(ctx, "agent-a", 0)
	if err != nil {
		t.Fatalf("TasksByStatusByDayForAgent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 day: %+v", len(rows), rows)
	}
	want := sqlite.AgentTaskStatusDayRow{
		Date: dayDate(base, 0), Todo: 1, InProgress: 1, InReview: 1,
		Done: 1, Blocked: 1, Cancelled: 1, Backlog: 1,
	}
	if rows[0] != want {
		t.Errorf("row = %+v, want %+v", rows[0], want)
	}
}

func TestRecentTasksForAgent(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, identifier, state, created_at, updated_at) VALUES
			('rt-1', 'ws-1', 'First task',  'KAN-1', 'IN_PROGRESS', datetime('now'), datetime('now')),
			('rt-2', 'ws-1', 'Second task', '',      'TODO',        datetime('now'), datetime('now')),
			('rt-3', 'ws-1', 'Third task',  'KAN-3', 'COMPLETED',   datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	seedActivity(t, repo, "ra-1", "agent-a", "task", "rt-1", "2026-01-01 10:00:00")
	seedActivity(t, repo, "ra-2", "agent-a", "task", "rt-1", "2026-01-05 10:00:00")
	seedActivity(t, repo, "ra-3", "agent-a", "task", "rt-2", "2026-01-03 10:00:00")
	seedActivity(t, repo, "ra-4", "agent-b", "task", "rt-3", "2026-01-09 10:00:00")

	rows, err := repo.RecentTasksForAgent(ctx, "agent-a", 10)
	if err != nil {
		t.Fatalf("RecentTasksForAgent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (agent-b's task excluded): %+v", len(rows), rows)
	}
	// MAX(created_at) DESC: rt-1's latest activity (Jan 5) beats rt-2's (Jan 3).
	if rows[0].TaskID != "rt-1" || rows[1].TaskID != "rt-2" {
		t.Fatalf("order = %q,%q, want rt-1,rt-2", rows[0].TaskID, rows[1].TaskID)
	}
	if rows[0].Identifier != "KAN-1" {
		t.Errorf("Identifier = %q, want KAN-1", rows[0].Identifier)
	}
	if rows[0].Title != "First task" {
		t.Errorf("Title = %q, want 'First task'", rows[0].Title)
	}
	if rows[0].State != "IN_PROGRESS" {
		t.Errorf("State = %q, want IN_PROGRESS", rows[0].State)
	}
	// MAX() strips the column's declared type, so the value arrives as the
	// raw stored text rather than the driver's RFC3339 rendering.
	if rows[0].LastActiveAt != "2026-01-05 10:00:00" {
		t.Errorf("LastActiveAt = %q, want the MAX activity timestamp 2026-01-05 10:00:00", rows[0].LastActiveAt)
	}
	if rows[1].Identifier != "" {
		t.Errorf("rt-2 Identifier = %q, want empty", rows[1].Identifier)
	}
}

func TestRecentTasksForAgent_NonPositiveLimitDefaultsTo10(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	base := time.Now().UTC()

	for i := 0; i < 12; i++ {
		id := string(rune('a' + i))
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO tasks (id, workspace_id, title, state, created_at, updated_at)
			VALUES (?, 'ws-1', 'title', 'TODO', datetime('now'), datetime('now'))
		`, id); err != nil {
			t.Fatalf("seed task %s: %v", id, err)
		}
		seedActivity(t, repo, "act-"+id, "agent-a", "task", id, dayStamp(base, i, 10))
	}

	rows, err := repo.RecentTasksForAgent(ctx, "agent-a", 0)
	if err != nil {
		t.Fatalf("RecentTasksForAgent: %v", err)
	}
	if len(rows) != 10 {
		t.Errorf("rows = %d, want the default limit of 10", len(rows))
	}
}

func TestCostAggregateForAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedCostEvent(t, repo, "c-1", "agent-a", "t-1", "2026-01-01 10:00:00", 100, 10, 20, 5)
	seedCostEvent(t, repo, "c-2", "agent-a", "t-2", "2026-01-02 10:00:00", 200, 20, 40, 7)
	seedCostEvent(t, repo, "c-3", "agent-b", "t-3", "2026-01-03 10:00:00", 999, 99, 99, 99)

	got, err := repo.CostAggregateForAgent(ctx, "agent-a")
	if err != nil {
		t.Fatalf("CostAggregateForAgent: %v", err)
	}
	want := sqlite.AgentCostAggregate{
		InputTokens: 300, OutputTokens: 60, CachedTokens: 30, TotalCostSubcents: 12,
	}
	if got != want {
		t.Errorf("aggregate = %+v, want %+v", got, want)
	}
}

func TestCostAggregateForAgent_NoEventsReturnsZeroValue(t *testing.T) {
	repo := newTestRepo(t)

	got, err := repo.CostAggregateForAgent(context.Background(), "agent-silent")
	if err != nil {
		t.Fatalf("CostAggregateForAgent: %v", err)
	}
	if got != (sqlite.AgentCostAggregate{}) {
		t.Errorf("aggregate = %+v, want the zero value", got)
	}
}

func TestRecentRunCostsForAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// r-1: finished window, two matching cost events.
	seedSummaryRun(t, repo, "r-1", "agent-a", "finished",
		"2026-01-01 09:00:00", "2026-01-01 10:00:00", "2026-01-01 12:00:00", `{"task_id":"t-1"}`)
	seedCostEvent(t, repo, "c-1", "agent-a", "t-1", "2026-01-01 10:30:00", 100, 0, 10, 3)
	seedCostEvent(t, repo, "c-2", "agent-a", "t-1", "2026-01-01 11:30:00", 200, 0, 20, 4)
	// Outside the claimed→finished window, so excluded.
	seedCostEvent(t, repo, "c-3", "agent-a", "t-1", "2026-01-01 09:30:00", 999, 0, 999, 999)
	seedCostEvent(t, repo, "c-4", "agent-a", "t-1", "2026-01-01 13:30:00", 888, 0, 888, 888)
	// r-2: later run, one matching event.
	seedSummaryRun(t, repo, "r-2", "agent-a", "finished",
		"2026-01-02 09:00:00", "2026-01-02 10:00:00", "2026-01-02 12:00:00", `{"task_id":"t-2"}`)
	seedCostEvent(t, repo, "c-5", "agent-a", "t-2", "2026-01-02 11:00:00", 50, 0, 5, 1)
	// r-3: no cost events at all — must not appear (INNER JOIN).
	seedSummaryRun(t, repo, "r-3", "agent-a", "finished",
		"2026-01-03 09:00:00", "2026-01-03 10:00:00", "2026-01-03 12:00:00", `{"task_id":"t-3"}`)
	// A different agent's run + event must not leak in.
	seedSummaryRun(t, repo, "r-other", "agent-b", "finished",
		"2026-01-04 09:00:00", "2026-01-04 10:00:00", "2026-01-04 12:00:00", `{"task_id":"t-4"}`)
	seedCostEvent(t, repo, "c-6", "agent-b", "t-4", "2026-01-04 11:00:00", 77, 0, 77, 77)

	rows, err := repo.RecentRunCostsForAgent(ctx, "agent-a", 10)
	if err != nil {
		t.Fatalf("RecentRunCostsForAgent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (r-3 has no cost events): %+v", len(rows), rows)
	}
	// ORDER BY requested_at DESC.
	if rows[0].RunID != "r-2" || rows[1].RunID != "r-1" {
		t.Fatalf("order = %q,%q, want r-2,r-1", rows[0].RunID, rows[1].RunID)
	}
	if rows[0].RequestedAt != "2026-01-02T09:00:00Z" {
		t.Errorf("RequestedAt = %q, want 2026-01-02T09:00:00Z", rows[0].RequestedAt)
	}
	wantR1 := sqlite.AgentRunCostRow{
		RunID: "r-1", RequestedAt: "2026-01-01T09:00:00Z",
		InputTokens: 300, OutputTokens: 30, CostSubcents: 7,
	}
	if rows[1] != wantR1 {
		t.Errorf("r-1 = %+v, want %+v (only the in-window events)", rows[1], wantR1)
	}
	wantR2 := sqlite.AgentRunCostRow{
		RunID: "r-2", RequestedAt: "2026-01-02T09:00:00Z",
		InputTokens: 50, OutputTokens: 5, CostSubcents: 1,
	}
	if rows[0] != wantR2 {
		t.Errorf("r-2 = %+v, want %+v", rows[0], wantR2)
	}
}

func TestRecentRunCostsForAgent_UnclaimedRunHasNoLowerBound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// claimed_at NULL → the lower bound is skipped, so an event from
	// before the run was requested still matches.
	seedSummaryRun(t, repo, "r-unclaimed", "agent-a", "queued",
		"2026-01-05 09:00:00", "", "2026-01-05 12:00:00", `{"task_id":"t-1"}`)
	seedCostEvent(t, repo, "c-early", "agent-a", "t-1", "2026-01-05 08:00:00", 11, 0, 22, 33)

	rows, err := repo.RecentRunCostsForAgent(ctx, "agent-a", 0)
	if err != nil {
		t.Fatalf("RecentRunCostsForAgent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(rows), rows)
	}
	if rows[0].InputTokens != 11 || rows[0].OutputTokens != 22 || rows[0].CostSubcents != 33 {
		t.Errorf("row = %+v, want the pre-claim event counted", rows[0])
	}
}

func TestRecentRunCostsForAgent_RespectsLimit(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for i := 1; i <= 4; i++ {
		runID := "r-" + string(rune('0'+i))
		taskID := "t-" + string(rune('0'+i))
		day := "2026-01-0" + string(rune('0'+i))
		seedSummaryRun(t, repo, runID, "agent-a", "finished",
			day+" 09:00:00", day+" 10:00:00", day+" 12:00:00", `{"task_id":"`+taskID+`"}`)
		seedCostEvent(t, repo, "c-"+runID, "agent-a", taskID, day+" 11:00:00", 1, 0, 1, 1)
	}

	rows, err := repo.RecentRunCostsForAgent(ctx, "agent-a", 2)
	if err != nil {
		t.Fatalf("RecentRunCostsForAgent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (limit)", len(rows))
	}
	if rows[0].RunID != "r-4" {
		t.Errorf("newest = %q, want r-4", rows[0].RunID)
	}
}

func TestLatestRunForAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedSummaryRun(t, repo, "lr-old", "agent-a", "finished",
		"2026-01-01 09:00:00", "2026-01-01 10:00:00", "2026-01-01 12:00:00", `{"task_id":"t-1"}`)
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs
			(id, agent_profile_id, reason, payload, status, error_message, requested_at, claimed_at, finished_at)
		VALUES ('lr-new', 'agent-a', 'task_assigned', '{"task_id":"t-2"}', 'failed', 'boom',
		        '2026-01-02 09:00:00', NULL, NULL)
	`); err != nil {
		t.Fatalf("seed newest run: %v", err)
	}
	seedSummaryRun(t, repo, "lr-other", "agent-b", "finished",
		"2026-01-09 09:00:00", "", "", "{}")

	got, err := repo.LatestRunForAgent(ctx, "agent-a")
	if err != nil {
		t.Fatalf("LatestRunForAgent: %v", err)
	}
	if got == nil {
		t.Fatal("LatestRunForAgent = nil, want the newest run")
	}
	if got.ID != "lr-new" {
		t.Fatalf("ID = %q, want lr-new (requested_at DESC)", got.ID)
	}
	if got.AgentProfileID != "agent-a" {
		t.Errorf("AgentProfileID = %q, want agent-a", got.AgentProfileID)
	}
	if got.Reason != "task_assigned" {
		t.Errorf("Reason = %q, want task_assigned", got.Reason)
	}
	if got.Payload != `{"task_id":"t-2"}` {
		t.Errorf("Payload = %q, want the stored JSON", got.Payload)
	}
	if got.Status != "failed" {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.ErrorMessage != "boom" {
		t.Errorf("ErrorMessage = %q, want boom", got.ErrorMessage)
	}
	if got.RequestedAt.Format("2006-01-02 15:04:05") != "2026-01-02 09:00:00" {
		t.Errorf("RequestedAt = %v, want 2026-01-02 09:00:00", got.RequestedAt)
	}
	if got.ClaimedAt != nil {
		t.Errorf("ClaimedAt = %v, want nil", got.ClaimedAt)
	}
	if got.FinishedAt != nil {
		t.Errorf("FinishedAt = %v, want nil", got.FinishedAt)
	}

	// The older run's nullable timestamps must round-trip as non-nil.
	if _, err := repo.ExecRaw(ctx, `DELETE FROM runs WHERE id = 'lr-new'`); err != nil {
		t.Fatalf("delete newest: %v", err)
	}
	got, err = repo.LatestRunForAgent(ctx, "agent-a")
	if err != nil {
		t.Fatalf("LatestRunForAgent after delete: %v", err)
	}
	if got == nil || got.ID != "lr-old" {
		t.Fatalf("got %+v, want lr-old", got)
	}
	if got.ClaimedAt == nil || got.ClaimedAt.Format("15:04:05") != "10:00:00" {
		t.Errorf("ClaimedAt = %v, want 10:00:00", got.ClaimedAt)
	}
	if got.FinishedAt == nil || got.FinishedAt.Format("15:04:05") != "12:00:00" {
		t.Errorf("FinishedAt = %v, want 12:00:00", got.FinishedAt)
	}
}

func TestLatestRunForAgent_NoRunsReturnsNilNil(t *testing.T) {
	repo := newTestRepo(t)

	got, err := repo.LatestRunForAgent(context.Background(), "agent-without-runs")
	if err != nil {
		t.Fatalf("LatestRunForAgent = %v, want nil error for the no-rows case", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}
