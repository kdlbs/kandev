package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestAutomationHistoryPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			archived_at TIMESTAMPTZ
		);
		CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			is_primary INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL
		);
		CREATE TABLE task_session_messages (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL DEFAULT '',
			turn_id TEXT NOT NULL DEFAULT '',
			author_type TEXT NOT NULL DEFAULT 'user',
			content TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'message',
			created_at TIMESTAMPTZ NOT NULL
		)`); err != nil {
		t.Fatalf("create task prerequisites: %v", err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("automation store schema init: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	create := func(workspaceID, name string, createdAt time.Time) *Automation {
		a := &Automation{WorkspaceID: workspaceID, Name: name, WorkflowID: "wf-1", WorkflowStepID: "s-1"}
		if err := store.CreateAutomation(ctx, a); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`UPDATE automations SET created_at = $1, updated_at = $1 WHERE id = $2`,
			createdAt, a.ID); err != nil {
			t.Fatal(err)
		}
		return a
	}
	first := create("ws-1", "first", base)
	second := create("ws-1", "second", base.Add(time.Minute))
	foreign := create("ws-2", "foreign", base)
	trigger := &AutomationTrigger{
		AutomationID: first.ID, Type: TriggerTypeScheduled,
		Config: json.RawMessage(`{"cron_expression":"0 9 * * *"}`), Enabled: true,
	}
	if err := store.CreateTrigger(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	insertTask := func(id string, archived bool) {
		var archivedAt any
		if archived {
			archivedAt = base
		}
		if _, err := database.Exec(`INSERT INTO tasks (id, archived_at) VALUES ($1, $2)`, id, archivedAt); err != nil {
			t.Fatal(err)
		}
	}
	insertTask("task-active", false)
	insertTask("task-archived", true)
	if _, err := database.Exec(`INSERT INTO task_sessions (id, task_id, is_primary, state) VALUES ('session-cancelled', 'task-cancelled', 1, 'CANCELLED')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO tasks (id, archived_at) VALUES ('task-cancelled', NULL)`); err != nil {
		t.Fatal(err)
	}
	runs := []*AutomationRun{
		{AutomationID: first.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-active", Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`)},
		{AutomationID: first.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-archived", Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`)},
		{AutomationID: first.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-cancelled", Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`)},
		{AutomationID: foreign.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)},
	}
	for i, run := range runs {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`UPDATE automation_runs SET created_at = $1 WHERE id = $2`,
			base.Add(time.Duration(i)*time.Minute), run.ID); err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(store, nil, nil)
	definitions, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions.Items) != 2 || definitions.Items[0].ID != first.ID || definitions.Items[1].ID != second.ID {
		t.Fatalf("postgres definitions = %+v", definitions)
	}
	if !definitions.Items[0].Triggers[0].ScheduleAvailable {
		t.Fatalf("postgres scheduled trigger unavailable: %+v", definitions.Items[0].Triggers)
	}
	from := base.Add(time.Minute)
	bounded, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{
		WorkspaceID: "ws-1", CreatedFrom: &from, Limit: 10,
	})
	if err != nil || len(bounded.Items) != 1 || bounded.Items[0].ID != second.ID {
		t.Fatalf("postgres created_from definitions = %+v, err=%v", bounded, err)
	}
	if _, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{
		WorkspaceID: "ws-1", AutomationID: foreign.ID, Limit: 10,
	}); !errors.Is(err, ErrHistoryAutomationNotFound) {
		t.Fatalf("postgres foreign filter error = %v", err)
	}

	runList, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(runList.Items) != 3 {
		t.Fatalf("postgres workspace runs = %+v", runList.Items)
	}
	statuses := map[string]RunStatus{}
	for _, item := range runList.Items {
		if item.SourceStatus != RunStatusTaskCreated {
			t.Fatalf("postgres source status = %q for run %s", item.SourceStatus, item.ID)
		}
		statuses[item.ID] = item.Status
	}
	if statuses[runs[0].ID] != RunStatusTaskCreated ||
		statuses[runs[1].ID] != RunStatusArchived ||
		statuses[runs[2].ID] != RunStatusCancelled {
		t.Fatalf("postgres projected statuses = %+v", statuses)
	}
	if runList.Items[0].ID != runs[2].ID || runList.Items[2].ID != runs[0].ID {
		t.Fatalf("postgres run order = %+v", runList.Items)
	}

	overflowRuns := make([]*AutomationRun, 0, 205)
	for i := 0; i < 205; i++ {
		run := &AutomationRun{AutomationID: first.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)}
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`UPDATE automation_runs SET created_at = $1 WHERE id = $2`,
			base.Add(time.Hour+time.Duration(i)*time.Second), run.ID); err != nil {
			t.Fatal(err)
		}
		overflowRuns = append(overflowRuns, run)
	}
	page, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 200})
	if err != nil || len(page.Items) != 200 || !page.Overflow {
		t.Fatalf("postgres bounded run page = %d items, overflow=%v, err=%v", len(page.Items), page.Overflow, err)
	}
	if page.Items[0].ID != overflowRuns[len(overflowRuns)-1].ID {
		t.Fatalf("postgres bounded page first run = %s", page.Items[0].ID)
	}

	refresh := overflowRuns[0]
	if _, err := database.Exec(`UPDATE automation_runs SET status = 'failed', error_message = 'late failure' WHERE id = $1`, refresh.ID); err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.GetHistoryRun(ctx, "ws-1", refresh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.SourceStatus != RunStatusFailed || refreshed.Status != RunStatusFailed || refreshed.ErrorMessage != "late failure" {
		t.Fatalf("postgres refreshed run = %+v", refreshed)
	}
	if _, err := service.GetHistoryRun(ctx, "ws-2", refresh.ID); !errors.Is(err, ErrHistoryRunNotFound) {
		t.Fatalf("postgres foreign exact run error = %v", err)
	}
}
