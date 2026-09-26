package automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestListWorkspaceRuns_InterleavesAutomationsNewestFirstAndExcludesOtherWorkspaces(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	sweep := &Automation{WorkspaceID: "ws-1", Name: "nightly sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	triage := &Automation{WorkspaceID: "ws-1", Name: "pr triage", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	foreign := &Automation{WorkspaceID: "ws-2", Name: "someone else's", WorkflowID: "wf-9", WorkflowStepID: "s-9"}
	for _, a := range []*Automation{sweep, triage, foreign} {
		if err := store.CreateAutomation(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	base := time.Now().UTC().Truncate(time.Second)
	seed := func(a *Automation, at time.Time) *AutomationRun {
		t.Helper()
		r := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)}
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
		setRunCreatedAt(t, store, r.ID, at)
		return r
	}
	oldest := seed(sweep, base)
	middle := seed(triage, base.Add(time.Minute))
	newest := seed(sweep, base.Add(2*time.Minute))
	seed(foreign, base.Add(3*time.Minute))

	got, err := store.ListWorkspaceRuns(ctx, "ws-1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 runs from ws-1 only, got %d", len(got))
	}
	wantOrder := []string{newest.ID, middle.ID, oldest.ID}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Fatalf("run %d: expected %s (newest-first), got %s", i, want, got[i].ID)
		}
	}
	if got[0].AutomationName != "nightly sweep" {
		t.Errorf("newest run: expected nightly sweep, got %q", got[0].AutomationName)
	}
	if got[1].AutomationName != "pr triage" {
		t.Errorf("middle run: expected pr triage, got %q", got[1].AutomationName)
	}
	if string(got[0].TriggerData) != `{}` {
		t.Errorf("expected trigger_data hydrated from its stored JSON, got %q", string(got[0].TriggerData))
	}
}

// The workspace feed must not tell a different story about the same run
// than the automation's own page does. Mirrors
// TestListRuns_DerivesArchivedCancelledAndActiveStatus.
func TestListWorkspaceRuns_DerivesArchivedCancelledAndActiveStatus(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-active", false)
	insertTask(t, store, "task-archived", true)
	insertTask(t, store, "task-cancelled", false)
	insertPrimarySession(t, store, "task-cancelled", "CANCELLED")
	insertTask(t, store, "task-cancelled-and-archived", true)
	insertPrimarySession(t, store, "task-cancelled-and-archived", "CANCELLED")
	insertTask(t, store, "task-resumed", false)
	insertStaleSession(t, store, "task-resumed", "CANCELLED")
	insertPrimarySession(t, store, "task-resumed", "COMPLETED")

	active := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-active", TriggerData: json.RawMessage(`{}`)}
	archived := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-archived", TriggerData: json.RawMessage(`{}`)}
	cancelled := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-cancelled", TriggerData: json.RawMessage(`{}`)}
	cancelledAndArchived := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-cancelled-and-archived", TriggerData: json.RawMessage(`{}`)}
	resumed := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-resumed", TriggerData: json.RawMessage(`{}`)}
	missing := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-missing", TriggerData: json.RawMessage(`{}`)}
	emptyTaskID := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "", TriggerData: json.RawMessage(`{}`)}
	succeededOnArchived := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TaskID: "task-archived", TriggerData: json.RawMessage(`{}`)}
	for _, r := range []*AutomationRun{active, archived, cancelled, cancelledAndArchived, resumed, missing, emptyTaskID, succeededOnArchived} {
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.ListWorkspaceRuns(ctx, "ws-1", 50)
	if err != nil {
		t.Fatal(err)
	}
	statusByID := map[string]RunStatus{}
	for _, r := range got {
		statusByID[r.ID] = r.Status
	}
	for _, tc := range []struct {
		name string
		id   string
		want RunStatus
	}{
		{"active task run", active.ID, RunStatusTaskCreated},
		{"archived task run", archived.ID, RunStatusArchived},
		{"cancelled task run", cancelled.ID, RunStatusCancelled},
		{"cancelled-and-archived task run (archived wins)", cancelledAndArchived.ID, RunStatusArchived},
		{"resumed-after-cancel task run (stale session ignored)", resumed.ID, RunStatusTaskCreated},
		{"missing task run", missing.ID, RunStatusCancelled},
		{"empty task_id run", emptyTaskID.ID, RunStatusCancelled},
		{"already-succeeded run on now-archived task", succeededOnArchived.ID, RunStatusSucceeded},
	} {
		if s := statusByID[tc.id]; s != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.name, tc.want, s)
		}
	}
}

// A run-mode automation hides its task, so in the workspace feed the
// summary is the entire visible outcome of the run.
func TestListWorkspaceRuns_CarriesTheAgentsLastMessageAsSummary(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "reporter", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-report", false)
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSucceeded,
		TaskID:       "task-report",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	seed := func(id, author, content string, at time.Time) {
		t.Helper()
		if _, err := store.db.Exec(
			`INSERT INTO task_session_messages (id, task_id, author_type, content, type, created_at) VALUES (?,?,?,?,?,?)`,
			id, "task-report", author, content, "message", at); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Now().UTC()
	seed("m1", "user", "run the sweep", base)
	seed("m2", "agent", "Working on it.", base.Add(time.Minute))
	seed("m3", "agent", "Sweep complete across all 32 specs.", base.Add(2*time.Minute))

	runs, err := store.ListWorkspaceRuns(ctx, "ws-1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Summary != "Sweep complete across all 32 specs." {
		t.Errorf("expected the agent's LAST message, got %q", runs[0].Summary)
	}
}

// TestListWorkspaceRuns_FallsBackWhenTasksTableAbsent mirrors
// TestListRuns_FallsBackWhenTasksTableAbsent: the workspace feed still has
// to attribute each run even on the fallback path, so the automations join
// must survive the missing-tasks-table retry.
func TestListWorkspaceRuns_FallsBackWhenTasksTableAbsent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-xyz",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.ListWorkspaceRuns(ctx, "ws-1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != RunStatusTaskCreated {
		t.Fatalf("expected 1 run with status task_created when tasks table is absent, got %+v", got)
	}
	if got[0].AutomationName != "A" {
		t.Errorf("expected automation_name on the fallback path too, got %q", got[0].AutomationName)
	}
}

// The workspace feed spans every automation, so an uncapped limit is a
// whole-history dump over the socket — the cap is a server-side guarantee,
// not a client courtesy.
func TestListWorkspaceRuns_LimitIsRespectedAndCapped(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxWorkspaceRunsLimit+10; i++ {
		if err := store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusSucceeded,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.ListWorkspaceRuns(ctx, "ws-1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("expected the requested 5 runs, got %d", len(got))
	}

	got, err = store.ListWorkspaceRuns(ctx, "ws-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 50 {
		t.Fatalf("expected the default of 50 runs for a zero limit, got %d", len(got))
	}

	got, err = store.ListWorkspaceRuns(ctx, "ws-1", 10000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != maxWorkspaceRunsLimit {
		t.Fatalf("expected the limit capped at %d, got %d", maxWorkspaceRunsLimit, len(got))
	}
}

// The feed shows two lines of prose, so the tail of a long agent message is
// truncated server-side rather than shipping the whole report to every row.
// Without this the substr could be dropped and every test would stay green.
func TestListWorkspaceRuns_TruncatesALongSummary(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	createTasksTable(t, store)

	a := &Automation{WorkspaceID: "ws-1", Name: "verbose", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-long", false)
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSucceeded,
		TaskID:       "task-long",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 500)
	if _, err := store.db.Exec(
		`INSERT INTO task_session_messages (id, task_id, author_type, content, type, created_at) VALUES (?,?,?,?,?,?)`,
		"m-long", "task-long", "agent", long, "message", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListWorkspaceRuns(ctx, "ws-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if got := len(runs[0].Summary); got != 280 {
		t.Errorf("expected the summary truncated to 280 chars, got %d", got)
	}
}

// A run keeps its task_id after the task row is deleted. Reporting it would
// render the feed entry as a link to a transcript that no longer exists.
func TestListWorkspaceRuns_DropsTaskIDWhenTheTaskIsGone(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	createTasksTable(t, store)

	a := &Automation{WorkspaceID: "ws-1", Name: "gone", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	// Deliberately no task row for this id.
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-deleted",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListWorkspaceRuns(ctx, "ws-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].TaskID != "" {
		t.Errorf("expected no task id for a deleted task, got %q", runs[0].TaskID)
	}
	if runs[0].Status != RunStatusCancelled {
		t.Errorf("expected the derived cancelled status, got %q", runs[0].Status)
	}
}

// The runs list used to read each automation's health out of the capped
// workspace feed. Past the cap an automation's newest run falls outside the
// window and its row claims "No runs yet" — the one thing a health indicator
// must never get wrong. Summaries answer per automation, so a noisy neighbour
// cannot push a quiet automation's last run out of view.
