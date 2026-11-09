package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func createHistoryAutomation(t *testing.T, store *Store, workspaceID, name string, createdAt time.Time) *Automation {
	t.Helper()
	a := &Automation{WorkspaceID: workspaceID, Name: name, WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE automations SET created_at = ?, updated_at = ? WHERE id = ?`,
		createdAt.UTC(), createdAt.UTC(), a.ID); err != nil {
		t.Fatal(err)
	}
	return a
}

func createHistoryTrigger(t *testing.T, store *Store, automationID string, kind TriggerType, config string) *AutomationTrigger {
	t.Helper()
	trigger := &AutomationTrigger{AutomationID: automationID, Type: kind, Config: json.RawMessage(config), Enabled: true}
	if err := store.CreateTrigger(context.Background(), trigger); err != nil {
		t.Fatal(err)
	}
	return trigger
}

func TestAutomationHistoryDefinitions(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	first := createHistoryAutomation(t, store, "ws-1", "first", base)
	second := createHistoryAutomation(t, store, "ws-1", "second", base.Add(time.Minute))
	foreign := createHistoryAutomation(t, store, "ws-2", "foreign", base)
	createHistoryTrigger(t, store, first.ID, TriggerTypeScheduled, `{"cron_expression":"0 9 * * *","timezone":"Europe/Berlin"}`)
	createHistoryTrigger(t, store, first.ID, TriggerTypeWebhook, `{"dedup_key":"body.id"}`)
	createHistoryTrigger(t, store, second.ID, TriggerTypeScheduled, `{"cron_expression":`)

	service := NewService(store, nil, nil)
	list, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Overflow {
		t.Fatalf("definitions = %+v, want two workspace definitions without overflow", list)
	}
	if list.Items[0].ID != first.ID || list.Items[1].ID != second.ID {
		t.Fatalf("definition order = %s, %s; want creation order", list.Items[0].ID, list.Items[1].ID)
	}
	if len(list.Items[0].Triggers) != 2 {
		t.Fatalf("first definition triggers = %+v", list.Items[0].Triggers)
	}
	scheduled := list.Items[0].Triggers[0]
	if !scheduled.ScheduleAvailable || scheduled.CronExpression != "0 9 * * *" || scheduled.Timezone != "Europe/Berlin" {
		t.Fatalf("scheduled trigger projection = %+v", scheduled)
	}
	webhook := list.Items[0].Triggers[1]
	if webhook.ScheduleAvailable || webhook.CronExpression != "" {
		t.Fatalf("webhook trigger must not expose configuration: %+v", webhook)
	}
	malformed := list.Items[1].Triggers[0]
	if malformed.ScheduleAvailable || malformed.Kind != TriggerTypeScheduled {
		t.Fatalf("malformed schedule must stay present but unavailable: %+v", malformed)
	}

	filtered, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{
		WorkspaceID: "ws-1", AutomationID: first.ID, Limit: 10,
	})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != first.ID {
		t.Fatalf("automation-filtered definitions = %+v, err=%v", filtered, err)
	}
	if _, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{
		WorkspaceID: "ws-1", AutomationID: foreign.ID, Limit: 10,
	}); !errors.Is(err, ErrHistoryAutomationNotFound) {
		t.Fatalf("foreign automation filter error = %v, want ErrHistoryAutomationNotFound", err)
	}

	from := base.Add(time.Minute)
	window, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{
		WorkspaceID: "ws-1", CreatedFrom: &from, Limit: 10,
	})
	if err != nil || len(window.Items) != 1 || window.Items[0].ID != second.ID {
		t.Fatalf("inclusive created_from definitions = %+v, err=%v", window, err)
	}
	before := base.Add(time.Minute)
	bounded, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{
		WorkspaceID: "ws-1", CreatedBefore: &before, Limit: 10,
	})
	if err != nil || len(bounded.Items) != 1 || bounded.Items[0].ID != first.ID {
		t.Fatalf("exclusive created_before definitions = %+v, err=%v", bounded, err)
	}

	overflow, err := service.ListHistoryAutomations(ctx, HistoryDefinitionFilter{WorkspaceID: "ws-1", Limit: 1})
	if err != nil || len(overflow.Items) != 1 || !overflow.Overflow {
		t.Fatalf("bounded definitions = %+v, err=%v; want overflow", overflow, err)
	}
}

func TestAutomationHistoryScopeAndBounds(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()
	base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	a := createHistoryAutomation(t, store, "ws-1", "sweep", base)
	foreign := createHistoryAutomation(t, store, "ws-2", "foreign", base)
	first := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)}
	second := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusFailed, TriggerData: json.RawMessage(`{}`)}
	elsewhere := &AutomationRun{AutomationID: foreign.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)}
	for _, run := range []*AutomationRun{first, second, elsewhere} {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	setRunCreatedAt(t, store, first.ID, base)
	setRunCreatedAt(t, store, second.ID, base.Add(time.Minute))
	setRunCreatedAt(t, store, elsewhere.ID, base.Add(2*time.Minute))

	service := NewService(store, nil, nil)
	list, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Items[0].ID != second.ID || list.Items[1].ID != first.ID {
		t.Fatalf("workspace runs = %+v, want newest-first workspace rows", list.Items)
	}
	if _, err := service.ListHistoryRuns(ctx, HistoryRunFilter{
		WorkspaceID: "ws-1", AutomationID: foreign.ID, Limit: 10,
	}); !errors.Is(err, ErrHistoryAutomationNotFound) {
		t.Fatalf("foreign run filter error = %v, want ErrHistoryAutomationNotFound", err)
	}

	from := base.Add(time.Minute)
	after, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", CreatedFrom: &from, Limit: 10})
	if err != nil || len(after.Items) != 1 || after.Items[0].ID != second.ID {
		t.Fatalf("inclusive created_from runs = %+v, err=%v", after, err)
	}
	before := base.Add(time.Minute)
	beforeList, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", CreatedBefore: &before, Limit: 10})
	if err != nil || len(beforeList.Items) != 1 || beforeList.Items[0].ID != first.ID {
		t.Fatalf("exclusive created_before runs = %+v, err=%v", beforeList, err)
	}

	// Equal timestamps must resolve by identity so a bounded page is stable.
	equal := base.Add(2 * time.Minute)
	setRunCreatedAt(t, store, second.ID, equal)
	setRunCreatedAt(t, store, first.ID, equal)
	equalPage, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 2})
	if err != nil || len(equalPage.Items) != 2 {
		t.Fatalf("equal timestamp runs = %+v, err=%v", equalPage, err)
	}
	if equalPage.Items[0].ID <= equalPage.Items[1].ID {
		t.Fatalf("equal timestamps must be ordered by descending identity: %s, %s",
			equalPage.Items[0].ID, equalPage.Items[1].ID)
	}
}

func TestAutomationHistoryRunObservations(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()
	base := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	a := createHistoryAutomation(t, store, "ws-1", "sweep", base)
	insertTask(t, store, "task-active", false)
	insertTask(t, store, "task-archived", true)
	insertPrimarySession(t, store, "task-cancelled", "CANCELLED")

	bound := &AutomationRun{
		AutomationID: a.ID, TriggerID: "trigger-1", TriggerType: TriggerTypeGitHubPR,
		TaskID: "task-active", SessionID: "session-1", TurnID: "turn-1",
		Status: RunStatusTaskCreated, DeliveryStatus: ManagedDeliveryAccepted,
		RetryGroupID: "group-1", RetryParentRunID: "parent-1", AttemptNumber: 2,
		Summary: "task-level fallback", TriggerData: json.RawMessage(`{}`),
	}
	archived := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-archived", Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`)}
	cancelled := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-cancelled", Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`)}
	skipped := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeWebhook, Status: RunStatusSkipped, ErrorMessage: "suppressed", TriggerData: json.RawMessage(`{}`)}
	cancelledRetry := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusScheduledRetry, RetryState: RetryStateCancelled, TriggerData: json.RawMessage(`{}`)}
	for _, run := range []*AutomationRun{bound, archived, cancelled, skipped, cancelledRetry} {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		setRunCreatedAt(t, store, run.ID, base)
	}
	if _, err := store.db.Exec(`INSERT INTO task_session_messages
		(id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('message-1', 'task-active', 'turn-1', 'agent', 'exact turn result', 'message', ?)`, base); err != nil {
		t.Fatal(err)
	}

	service := NewService(store, nil, nil)
	list, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]HistoryRun{}
	for _, item := range list.Items {
		byID[item.ID] = item
	}
	active := byID[bound.ID]
	if active.SourceStatus != RunStatusTaskCreated || active.Status != RunStatusTaskCreated {
		t.Fatalf("active statuses = source %q observed %q", active.SourceStatus, active.Status)
	}
	if active.Summary != "exact turn result" {
		t.Fatalf("run summary = %q, want exact-turn message", active.Summary)
	}
	if active.SessionID != "session-1" || active.TurnID != "turn-1" || active.RetryGroupID != "group-1" || active.RetryParentRunID != "parent-1" {
		t.Fatalf("run identity links = %+v", active)
	}
	if active.AttemptNumber == nil || *active.AttemptNumber != 2 {
		t.Fatalf("attempt number = %v, want 2", active.AttemptNumber)
	}
	if active.DeliveryStatus != ManagedDeliveryAccepted {
		t.Fatalf("delivery status = %q", active.DeliveryStatus)
	}
	if got := byID[archived.ID]; got.SourceStatus != RunStatusTaskCreated || got.Status != RunStatusArchived {
		t.Fatalf("archived statuses = source %q observed %q", got.SourceStatus, got.Status)
	}
	if got := byID[cancelled.ID]; got.SourceStatus != RunStatusTaskCreated || got.Status != RunStatusCancelled {
		t.Fatalf("cancelled statuses = source %q observed %q", got.SourceStatus, got.Status)
	}
	if got := byID[skipped.ID]; got.TaskID != "" || got.Status != RunStatusSkipped || got.ErrorMessage != "suppressed" {
		t.Fatalf("taskless skipped run = %+v", got)
	}
	if got := byID[cancelledRetry.ID]; got.Status != RunStatusCancelled || got.RetryState != RetryStateCancelled {
		t.Fatalf("cancelled retry run = %+v", got)
	}

	beforeCount := countHistoryRows(t, store)
	if _, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if afterCount := countHistoryRows(t, store); afterCount != beforeCount {
		t.Fatalf("history reads changed domain rows: before %d after %d", beforeCount, afterCount)
	}

	overflowRuns := make([]*AutomationRun, 0, 205)
	for i := 0; i < 205; i++ {
		run := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`)}
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		setRunCreatedAt(t, store, run.ID, base.Add(time.Duration(i+1)*time.Second))
		overflowRuns = append(overflowRuns, run)
	}
	page, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 200 || !page.Overflow {
		t.Fatalf("bounded run page = %d items, overflow=%v; want 200 and overflow", len(page.Items), page.Overflow)
	}
	newest := overflowRuns[len(overflowRuns)-1]
	if page.Items[0].ID != newest.ID {
		t.Fatalf("bounded page did not start at newest run: %s != %s", page.Items[0].ID, newest.ID)
	}
}

func countHistoryRows(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	if err := store.db.Get(&count, `SELECT COUNT(*) FROM automation_runs`); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestAutomationHistoryExactRefresh(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()
	base := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	a := createHistoryAutomation(t, store, "ws-1", "sweep", base)
	insertTask(t, store, "task-refresh", false)
	run := &AutomationRun{
		AutomationID: a.ID, TaskID: "task-refresh", TurnID: "turn-refresh",
		TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil, nil)

	first, err := service.GetHistoryRun(ctx, "ws-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != RunStatusTaskCreated || first.SourceStatus != RunStatusTaskCreated || first.Summary != "" {
		t.Fatalf("initial exact run = %+v", first)
	}
	if _, err := store.db.Exec(`UPDATE tasks SET archived_at = ? WHERE id = 'task-refresh'`, base); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO task_session_messages
		(id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('message-refresh', 'task-refresh', 'turn-refresh', 'agent', 'completed later', 'message', ?)`, base); err != nil {
		t.Fatal(err)
	}
	fresh, err := service.GetHistoryRun(ctx, "ws-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != RunStatusArchived || fresh.SourceStatus != RunStatusTaskCreated {
		t.Fatalf("refreshed statuses = source %q observed %q; want archived projection", fresh.SourceStatus, fresh.Status)
	}
	if fresh.Summary != "completed later" {
		t.Fatalf("refreshed summary = %q, want later completion", fresh.Summary)
	}

	list, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil || len(list.Items) != 1 || list.Items[0].Status != fresh.Status || list.Items[0].Summary != fresh.Summary {
		t.Fatalf("list and exact refresh disagree: list=%+v exact=%+v err=%v", list, fresh, err)
	}

	if _, err := service.GetHistoryRun(ctx, "ws-2", run.ID); !errors.Is(err, ErrHistoryRunNotFound) {
		t.Fatalf("foreign exact run error = %v, want ErrHistoryRunNotFound", err)
	}
	if _, err := service.GetHistoryRun(ctx, "ws-1", "missing-run"); !errors.Is(err, ErrHistoryRunNotFound) {
		t.Fatalf("missing exact run error = %v, want ErrHistoryRunNotFound", err)
	}
	if err := store.DeleteRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetHistoryRun(ctx, "ws-1", run.ID); !errors.Is(err, ErrHistoryRunNotFound) {
		t.Fatalf("deleted exact run error = %v, want ErrHistoryRunNotFound", err)
	}
}

func TestAutomationHistoryPreservesRecordedIdentities(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()
	base := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

	a := createHistoryAutomation(t, store, "ws-1", "sweep", base)
	insertTask(t, store, "task-retained", false)
	insertPrimarySession(t, store, "task-retained", "RUNNING")

	emptySession := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-retained",
		TurnID: "turn-recorded", Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`),
	}
	recordedSession := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-retained",
		SessionID: "session-recorded", Status: RunStatusTaskCreated, TriggerData: json.RawMessage(`{}`),
	}
	for _, run := range []*AutomationRun{emptySession, recordedSession} {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(store, nil, nil)

	assertIdentities := func(stage string) {
		t.Helper()
		list, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
		if err != nil {
			t.Fatalf("%s list: %v", stage, err)
		}
		byID := map[string]HistoryRun{}
		for _, item := range list.Items {
			byID[item.ID] = item
		}
		empty := byID[emptySession.ID]
		if empty.SessionID != "" || empty.TurnID != "turn-recorded" {
			t.Fatalf("%s list invented a session or lost the turn: %+v", stage, empty)
		}
		if recorded := byID[recordedSession.ID]; recorded.SessionID != "session-recorded" {
			t.Fatalf("%s list changed a recorded session: %+v", stage, recorded)
		}
		exactEmpty, err := service.GetHistoryRun(ctx, "ws-1", emptySession.ID)
		if err != nil || exactEmpty.SessionID != "" || exactEmpty.TurnID != "turn-recorded" {
			t.Fatalf("%s exact invented a session or lost the turn: %+v, err=%v", stage, exactEmpty, err)
		}
		exactRecorded, err := service.GetHistoryRun(ctx, "ws-1", recordedSession.ID)
		if err != nil || exactRecorded.SessionID != "session-recorded" {
			t.Fatalf("%s exact changed a recorded session: %+v, err=%v", stage, exactRecorded, err)
		}
	}

	assertIdentities("initial")
	if _, err := store.db.Exec(`UPDATE task_sessions SET is_primary = 0 WHERE task_id = 'task-retained'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO task_sessions (id, task_id, is_primary, state)
		VALUES ('replacement-session', 'task-retained', 1, 'RUNNING')`); err != nil {
		t.Fatal(err)
	}
	assertIdentities("replaced primary session")

	if _, err := store.db.Exec(`DELETE FROM tasks WHERE id = 'task-retained'`); err != nil {
		t.Fatal(err)
	}
	list, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Items {
		if item.TaskID != "task-retained" {
			t.Fatalf("deleted task identity was lost from the list: %+v", item)
		}
		if item.Status != RunStatusCancelled || item.SourceStatus != RunStatusTaskCreated {
			t.Fatalf("deleted task status projection changed: %+v", item)
		}
	}
	exact, err := service.GetHistoryRun(ctx, "ws-1", emptySession.ID)
	if err != nil || exact.TaskID != "task-retained" || exact.Status != RunStatusCancelled {
		t.Fatalf("deleted task identity was lost from exact refresh: %+v, err=%v", exact, err)
	}
}

func TestAutomationHistoryExcludesLargePrivatePayloads(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	a := createHistoryAutomation(t, store, "ws-1", "sweep", base)
	large := strings.Repeat("x", 2<<20)
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeWebhook, Status: RunStatusSucceeded,
		DedupKey: "dedup-key", RetryClaimToken: "claim-token",
		TriggerData:               json.RawMessage(`{"body":"` + large + `"}`),
		RetryResolvedPrompt:       large,
		RetryPolicySnapshot:       large,
		RetryTriggerSnapshot:      large,
		RetryLaunchConfigSnapshot: large,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil, nil)

	assertNarrow := func(item HistoryRun) {
		t.Helper()
		if item.Status != RunStatusSucceeded || item.SourceStatus != RunStatusSucceeded {
			t.Fatalf("status projection changed: %+v", item)
		}
	}
	list, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("large-payload list = %+v, err=%v", list, err)
	}
	assertNarrow(list.Items[0])
	exact, err := service.GetHistoryRun(ctx, "ws-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertNarrow(*exact)

	rows, overflow, err := store.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil || overflow || len(rows) != 1 {
		t.Fatalf("store history rows = %+v, overflow=%v, err=%v", rows, overflow, err)
	}
	row := rows[0]
	if row.TriggerDataJSON != "" || row.RetryResolvedPrompt != "" || row.RetryPolicySnapshot != "" ||
		row.RetryTriggerSnapshot != "" || row.RetryLaunchConfigSnapshot != "" ||
		row.DedupKey != "" || row.RetryClaimToken != "" {
		t.Fatalf("history projection materialized private payload fields: %+v", row.AutomationRun)
	}
	exactRow, err := store.GetHistoryRun(ctx, "ws-1", run.ID)
	if err != nil || exactRow.TriggerDataJSON != "" || exactRow.RetryResolvedPrompt != "" {
		t.Fatalf("exact history projection materialized private payloads: %+v, err=%v", exactRow, err)
	}
}

func TestAutomationHistoryFallsBackToTaskSummaryForLegacyRun(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()
	base := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	a := createHistoryAutomation(t, store, "ws-1", "sweep", base)
	legacy := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-legacy",
		Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`),
	}
	bound := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled, TaskID: "task-legacy",
		TurnID: "missing-turn", Status: RunStatusSucceeded, TriggerData: json.RawMessage(`{}`),
	}
	for _, run := range []*AutomationRun{legacy, bound} {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`INSERT INTO task_session_messages
		(id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('message-legacy', 'task-legacy', 'turn-old', 'agent', 'legacy task result', 'message', ?)`, base); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil, nil)
	list, err := service.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: "ws-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]HistoryRun{}
	for _, item := range list.Items {
		byID[item.ID] = item
	}
	if byID[legacy.ID].Summary != "legacy task result" {
		t.Fatalf("legacy run summary = %q, want task-level fallback", byID[legacy.ID].Summary)
	}
	if byID[bound.ID].Summary != "" {
		t.Fatalf("turn-bound run must not use the task-level fallback: %q", byID[bound.ID].Summary)
	}
	exact, err := service.GetHistoryRun(ctx, "ws-1", legacy.ID)
	if err != nil || exact.Summary != "legacy task result" {
		t.Fatalf("exact legacy run summary = %q, err=%v", exact.Summary, err)
	}
}
