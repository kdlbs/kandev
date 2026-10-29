package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/plugins"
)

func newAutomationHistoryAdapterTest(t *testing.T) (*automation.Service, *sqlx.DB, automationHistoryAdapter) {
	t.Helper()
	database, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := automation.NewStore(database, database)
	if err != nil {
		t.Fatal(err)
	}
	service := automation.NewService(store, nil, nil)
	return service, database, automationHistoryAdapter{service: service}
}

func TestPluginAutomationHistoryAdapter(t *testing.T) {
	service, database, adapter := newAutomationHistoryAdapterTest(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	definition := &automation.Automation{
		WorkspaceID: "ws-1", Name: "nightly", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true,
	}
	if err := service.Store().CreateAutomation(ctx, definition); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE automations SET created_at = ?, updated_at = ? WHERE id = ?`,
		base, base, definition.ID); err != nil {
		t.Fatal(err)
	}
	scheduled := &automation.AutomationTrigger{
		AutomationID: definition.ID, Type: automation.TriggerTypeScheduled,
		Config: json.RawMessage(`{"cron_expression":"0 9 * * *","timezone":"Europe/Berlin"}`), Enabled: true,
	}
	if err := service.Store().CreateTrigger(ctx, scheduled); err != nil {
		t.Fatal(err)
	}
	webhook := &automation.AutomationTrigger{
		AutomationID: definition.ID, Type: automation.TriggerTypeWebhook,
		Config: json.RawMessage(`{"dedup_key":"body.id","filters":[{"path":"body.secret","op":"eq","values":["super-secret-value"]}]}`), Enabled: true,
	}
	if err := service.Store().CreateTrigger(ctx, webhook); err != nil {
		t.Fatal(err)
	}

	definitions, err := adapter.ListAutomationHistoryDefinitions(ctx, "ws-1", "", nil, nil, 10)
	if err != nil || len(definitions.Items) != 1 {
		t.Fatalf("definitions = %+v, err=%v", definitions, err)
	}
	mapped := definitions.Items[0]
	if len(mapped.Triggers) != 2 {
		t.Fatalf("mapped triggers = %+v", mapped.Triggers)
	}
	if !mapped.Triggers[0].ScheduleAvailable || mapped.Triggers[0].Timezone != "Europe/Berlin" {
		t.Fatalf("scheduled projection = %+v", mapped.Triggers[0])
	}
	if mapped.Triggers[1].ScheduleAvailable || strings.Contains(mapped.ResourceVersion, "super-secret-value") {
		t.Fatalf("webhook projection leaked configuration: %+v", mapped.Triggers[1])
	}
	if mapped.ResourceVersion == "" {
		t.Fatal("definition resource version is empty")
	}
	repeat, err := adapter.ListAutomationHistoryDefinitions(ctx, "ws-1", "", nil, nil, 10)
	if err != nil || repeat.Items[0].ResourceVersion != mapped.ResourceVersion {
		t.Fatalf("definition version is not stable: %q != %q, err=%v",
			repeat.Items[0].ResourceVersion, mapped.ResourceVersion, err)
	}

	if _, err := database.Exec(`CREATE TABLE task_session_messages (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '',
		author_type TEXT NOT NULL DEFAULT 'user', content TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT 'message', created_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	run := &automation.AutomationRun{
		AutomationID: definition.ID, TriggerType: automation.TriggerTypeScheduled,
		Status: automation.RunStatusTaskCreated, TaskID: "task-1", SessionID: "session-1", TurnID: "turn-1",
		Summary: "api_key=secret-value completed", ErrorMessage: "password=hunter2 failed",
		TriggerData: json.RawMessage(`{"body":"raw-webhook-secret"}`),
	}
	if err := service.Store().CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE automation_runs SET created_at = ?, error_message = ? WHERE id = ?`,
		base, "password=hunter2 failed", run.ID); err != nil {
		t.Fatal(err)
	}
	systemHeavyContent := "<kandev-system>" + strings.Repeat("hidden context ", 40) +
		"private instruction</kandev-system> api_key=secret-value visible completion"
	if _, err := database.Exec(`INSERT INTO task_session_messages
		(id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('message-1', 'task-1', 'turn-1', 'agent', ?, 'message', ?)`, systemHeavyContent, base); err != nil {
		t.Fatal(err)
	}
	second := &automation.AutomationRun{
		AutomationID: definition.ID, TriggerType: automation.TriggerTypeScheduled,
		Status: automation.RunStatusSucceeded, TriggerData: json.RawMessage(`{}`),
	}
	if err := service.Store().CreateRun(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE automation_runs SET created_at = ? WHERE id = ?`, base.Add(time.Minute), second.ID); err != nil {
		t.Fatal(err)
	}

	runs, err := adapter.ListAutomationHistoryRuns(ctx, "ws-1", "", nil, nil, 10)
	if err != nil || len(runs.Items) != 2 {
		t.Fatalf("runs = %+v, err=%v", runs, err)
	}
	if runs.Items[0].ID != second.ID || runs.Items[1].ID != run.ID {
		t.Fatalf("run order = %+v", runs.Items)
	}
	observed := runs.Items[1]
	if !strings.Contains(observed.Summary, "visible completion") ||
		strings.Contains(observed.Summary, "secret-value") ||
		strings.Contains(observed.Summary, "hidden context") ||
		strings.Contains(observed.Summary, "private instruction") {
		t.Fatalf("summary was not sanitized after stripping system content: %q", observed.Summary)
	}
	if strings.Contains(observed.ErrorExcerpt, "hunter2") {
		t.Fatalf("error excerpt leaked a credential: %q", observed.ErrorExcerpt)
	}
	if observed.SourceStatus != string(automation.RunStatusTaskCreated) || observed.Status != string(automation.RunStatusTaskCreated) {
		t.Fatalf("run statuses = source %q observed %q", observed.SourceStatus, observed.Status)
	}
	if observed.ResourceVersion == "" {
		t.Fatal("run resource version is empty")
	}
	refreshed, err := adapter.GetAutomationHistoryRun(ctx, "ws-1", run.ID)
	if err != nil || refreshed.ResourceVersion != observed.ResourceVersion {
		t.Fatalf("exact refresh version = %q, want %q, err=%v",
			refreshed.ResourceVersion, observed.ResourceVersion, err)
	}
	if _, err := database.Exec(`UPDATE automation_runs SET status = 'failed', error_message = 'later failure' WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	changed, err := adapter.GetAutomationHistoryRun(ctx, "ws-1", run.ID)
	if err != nil || changed.Status != string(automation.RunStatusFailed) || changed.ResourceVersion == observed.ResourceVersion {
		t.Fatalf("changed exact run = %+v, err=%v", changed, err)
	}

	limited, err := adapter.ListAutomationHistoryRuns(ctx, "ws-1", "", nil, nil, 1)
	if err != nil || len(limited.Items) != 1 || !limited.Overflow {
		t.Fatalf("bounded runs = %+v, err=%v", limited, err)
	}
	if _, err := adapter.ListAutomationHistoryRuns(ctx, "ws-2", "", nil, nil, 10); err != nil {
		t.Fatalf("foreign workspace empty query should succeed: %v", err)
	}
	if _, err := adapter.ListAutomationHistoryRuns(ctx, "ws-1", "missing-automation", nil, nil, 10); !errors.Is(err, plugins.ErrAutomationHistoryNotFound) {
		t.Fatalf("foreign automation filter error = %v, want ErrAutomationHistoryNotFound", err)
	}
	if _, err := adapter.GetAutomationHistoryRun(ctx, "ws-2", run.ID); !errors.Is(err, plugins.ErrAutomationHistoryNotFound) {
		t.Fatalf("foreign run error = %v, want ErrAutomationHistoryNotFound", err)
	}
}

func TestPluginAutomationHistoryTruncatedSystemBlockFailsClosed(t *testing.T) {
	service, database, adapter := newAutomationHistoryAdapterTest(t)
	ctx := context.Background()
	definition := &automation.Automation{
		WorkspaceID: "ws-1", Name: "nightly", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true,
	}
	if err := service.Store().CreateAutomation(ctx, definition); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE task_session_messages (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '',
		author_type TEXT NOT NULL DEFAULT 'user', content TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT 'message', created_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	run := &automation.AutomationRun{
		AutomationID: definition.ID, TriggerType: automation.TriggerTypeScheduled,
		Status: automation.RunStatusSucceeded, TaskID: "task-leak", TurnID: "turn-leak",
		TriggerData: json.RawMessage(`{}`),
	}
	if err := service.Store().CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	unterminated := "<kandev-system>" + strings.Repeat("hidden context ", 500) + "leaked hidden tail"
	if _, err := database.Exec(`INSERT INTO task_session_messages
		(id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('message-leak', 'task-leak', 'turn-leak', 'agent', ?, 'message', ?)`,
		unterminated, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	observed, err := adapter.GetAutomationHistoryRun(ctx, "ws-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Summary != "" {
		t.Fatalf("an unterminated system block must fail closed, got %q", observed.Summary)
	}
}

func TestPluginAutomationHistoryMasksTruncatedCredentialTail(t *testing.T) {
	service, database, adapter := newAutomationHistoryAdapterTest(t)
	ctx := context.Background()
	definition := &automation.Automation{
		WorkspaceID: "ws-1", Name: "nightly", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true,
	}
	if err := service.Store().CreateAutomation(ctx, definition); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE task_session_messages (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '',
		author_type TEXT NOT NULL DEFAULT 'user', content TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT 'message', created_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	run := &automation.AutomationRun{
		AutomationID: definition.ID, TriggerType: automation.TriggerTypeScheduled,
		Status: automation.RunStatusSucceeded, TaskID: "task-secret", TurnID: "turn-secret",
		TriggerData: json.RawMessage(`{}`),
	}
	if err := service.Store().CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	partialToken := strings.Repeat("a", 4072) + " ghp_" + strings.Repeat("b", 19)
	if _, err := database.Exec(`INSERT INTO task_session_messages
		(id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('message-secret', 'task-secret', 'turn-secret', 'agent', ?, 'message', ?)`,
		partialToken, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	observed, err := adapter.GetAutomationHistoryRun(ctx, "ws-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(observed.Summary, "ghp_b") || strings.Contains(observed.Summary, strings.Repeat("b", 19)) {
		t.Fatalf("truncated credential tail leaked: %q", observed.Summary)
	}
	if !strings.Contains(observed.Summary, "ghp_[redacted]") {
		t.Fatalf("truncated credential tail was not masked: %q", observed.Summary)
	}
}
