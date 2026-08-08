package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestListAutomationSummaries_SurvivesAFeedFullOfSomeoneElsesRuns(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	quiet := &Automation{WorkspaceID: "ws-1", Name: "weekly audit", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	noisy := &Automation{WorkspaceID: "ws-1", Name: "pr triage", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	for _, a := range []*Automation{quiet, noisy} {
		if err := store.CreateAutomation(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	base := time.Now().UTC().Truncate(time.Second)
	quietRun := &AutomationRun{
		AutomationID: quiet.ID, TriggerType: TriggerTypeScheduled,
		Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, quietRun); err != nil {
		t.Fatal(err)
	}
	setRunCreatedAt(t, store, quietRun.ID, base)

	// More than the feed's cap, all newer than the quiet automation's only run.
	for i := 0; i < maxWorkspaceRunsLimit+10; i++ {
		r := &AutomationRun{
			AutomationID: noisy.ID, TriggerType: TriggerTypeScheduled,
			Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`),
		}
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
		setRunCreatedAt(t, store, r.ID, base.Add(time.Duration(i+1)*time.Minute))
	}

	summaries, err := store.ListAutomationSummaries(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	byAutomation := map[string]*AutomationSummary{}
	for _, s := range summaries {
		byAutomation[s.AutomationID] = s
	}
	got := byAutomation[quiet.ID]
	if got == nil || got.LastRun == nil {
		t.Fatalf("the quiet automation's last run must survive a full feed, got %+v", got)
	}
	if got.LastRun.ID != quietRun.ID {
		t.Errorf("expected %s, got %s", quietRun.ID, got.LastRun.ID)
	}
}

// One row per automation, carrying its newest run by the same ordering the
// feed sorts by — so "last said" on the list is the entry that leads the feed.
func TestListAutomationSummaries_ReportsTheNewestRunPerAutomation(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	foreign := &Automation{WorkspaceID: "ws-2", Name: "not mine", WorkflowID: "wf-9", WorkflowStepID: "s-9"}
	for _, x := range []*Automation{a, foreign} {
		if err := store.CreateAutomation(ctx, x); err != nil {
			t.Fatal(err)
		}
	}

	base := time.Now().UTC().Truncate(time.Second)
	older := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusFailed, TriggerData: json.RawMessage(`{}`)}
	newer := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)}
	elsewhere := &AutomationRun{AutomationID: foreign.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)}
	for _, r := range []*AutomationRun{older, newer, elsewhere} {
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	setRunCreatedAt(t, store, older.ID, base)
	setRunCreatedAt(t, store, newer.ID, base.Add(time.Minute))

	summaries, err := store.ListAutomationSummaries(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary for ws-1's single automation, got %d", len(summaries))
	}
	if summaries[0].AutomationID != a.ID {
		t.Fatalf("another workspace's automation leaked in: %+v", summaries[0])
	}
	if summaries[0].LastRun == nil || summaries[0].LastRun.ID != newer.ID {
		t.Fatalf("expected the newest run %s, got %+v", newer.ID, summaries[0].LastRun)
	}
	if string(summaries[0].LastRun.TriggerData) != `{}` {
		t.Errorf("expected trigger_data hydrated from its stored JSON, got %q", string(summaries[0].LastRun.TriggerData))
	}
}

// The open count backs the list's "won't fire — still running" reason, so it
// has to mean exactly what the concurrency cap means. A run whose task was
// archived or cancelled is not holding anything.
func TestListAutomationSummaries_CountsOpenRunsTheWayTheCapDoes(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-open", false)
	insertTask(t, store, "task-archived", true)
	insertTask(t, store, "task-cancelled", false)
	insertPrimarySession(t, store, "task-cancelled", "CANCELLED")

	for _, taskID := range []string{"task-open", "task-archived", "task-cancelled"} {
		r := &AutomationRun{
			AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
			Status: RunStatusTaskCreated, TaskID: taskID, TriggerData: json.RawMessage(`{}`),
		}
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	summaries, err := store.ListAutomationSummaries(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].OpenRuns != 1 {
		t.Errorf("expected 1 open run (archived and cancelled are closed out), got %d", summaries[0].OpenRuns)
	}
	capCount, err := store.CountActiveRuns(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summaries[0].OpenRuns != capCount {
		t.Errorf("summary says %d open, the cap says %d — these must never disagree",
			summaries[0].OpenRuns, capCount)
	}
}

// An automation that has never run gets no summary row at all; the list treats
// a missing row as "no runs yet" rather than inventing one per automation.
func TestListAutomationSummaries_OmitsAutomationsThatHaveNeverRun(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	never := &Automation{WorkspaceID: "ws-1", Name: "brand new", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, never); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListAutomationSummaries(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected no summaries for a workspace whose automations never ran, got %+v", summaries)
	}
}

// Same fallback the run lists take: automation-only tests run without a tasks
// table, and the query must degrade to the stored status rather than error.
func TestListAutomationSummaries_FallsBackWhenTasksTableAbsent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	r := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
		Status: RunStatusTaskCreated, TaskID: "task-1", TriggerData: json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, r); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListAutomationSummaries(ctx, "ws-1")
	if err != nil {
		t.Fatalf("expected a graceful fallback without a tasks table, got %v", err)
	}
	if len(summaries) != 1 || summaries[0].LastRun == nil {
		t.Fatalf("expected one summary with its run, got %+v", summaries)
	}
	if summaries[0].LastRun.Status != RunStatusTaskCreated {
		t.Errorf("expected the stored status, got %q", summaries[0].LastRun.Status)
	}
	if summaries[0].OpenRuns != 1 {
		t.Errorf("expected the raw open count of 1, got %d", summaries[0].OpenRuns)
	}
}

// Two queries would be two snapshots. A run created between them reads as a
// still-open last run with an open count of zero, so the row renders idle and
// the client never starts polling — permanently stale until a manual refresh.
// One statement cannot disagree with itself.
func TestListAutomationSummaries_OpenCountAgreesWithTheRunItReports(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-open", false)
	open := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
		Status: RunStatusTaskCreated, TaskID: "task-open", TriggerData: json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, open); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListAutomationSummaries(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	got := summaries[0]
	if got.LastRun == nil || got.LastRun.Status != RunStatusTaskCreated {
		t.Fatalf("expected the open run as the last run, got %+v", got.LastRun)
	}
	if got.OpenRuns != 1 {
		t.Errorf("a summary reporting an open last run must count it: got %d", got.OpenRuns)
	}
}

// The feed and the summary must pick the same run when timestamps tie, or the
// list's "last said" contradicts the entry leading the automation's activity.
func TestListAutomationSummaries_BreaksTiedTimestampsLikeTheFeed(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	tied := time.Now().UTC().Truncate(time.Second)
	var ids []string
	for i := 0; i < 3; i++ {
		r := &AutomationRun{
			AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
			Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`),
		}
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
		setRunCreatedAt(t, store, r.ID, tied)
		ids = append(ids, r.ID)
	}

	feed, err := store.ListWorkspaceRuns(ctx, "ws-1", 50)
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := store.ListAutomationSummaries(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != len(ids) || len(summaries) != 1 || summaries[0].LastRun == nil {
		t.Fatalf("unexpected shape: feed=%d summaries=%d", len(feed), len(summaries))
	}
	if summaries[0].LastRun.ID != feed[0].ID {
		t.Errorf("summary's last run %s must be the run leading the feed %s",
			summaries[0].LastRun.ID, feed[0].ID)
	}
}

// The detail page needs the same authoritative open count the list uses: its
// own run window is capped, so an open run older than the window would leave
// the page reporting that nothing is in flight.
func TestGetAutomationSummary_AnswersForOneAutomation(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	mine := &Automation{WorkspaceID: "ws-1", Name: "mine", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	other := &Automation{WorkspaceID: "ws-1", Name: "other", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	for _, x := range []*Automation{mine, other} {
		if err := store.CreateAutomation(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	insertTask(t, store, "task-mine", false)
	insertTask(t, store, "task-other", false)
	for _, spec := range []struct {
		automationID string
		taskID       string
	}{{mine.ID, "task-mine"}, {other.ID, "task-other"}} {
		r := &AutomationRun{
			AutomationID: spec.automationID, TriggerType: TriggerTypeScheduled,
			Status: RunStatusTaskCreated, TaskID: spec.taskID, TriggerData: json.RawMessage(`{}`),
		}
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.GetAutomationSummary(ctx, mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.AutomationID != mine.ID {
		t.Fatalf("expected this automation's summary, got %+v", got)
	}
	if got.OpenRuns != 1 {
		t.Errorf("expected 1 open run for this automation alone, got %d", got.OpenRuns)
	}
}

// An automation that has never run has no summary — nil, not an error and not
// a zero-valued row the caller would render as real.
func TestGetAutomationSummary_NilWhenItHasNeverRun(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "brand new", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetAutomationSummary(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil for an automation that has never run, got %+v", got)
	}
}
