package automation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestCreateAndGetAutomation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Test Automation",
		Description:       "Runs on cron",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Prompt:            "Hello {{trigger.type}}",
		Enabled:           true,
		MaxConcurrentRuns: 1,
	}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if a.WebhookSecret == "" {
		t.Fatal("expected non-empty webhook secret")
	}

	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected automation, got nil")
	}
	if got.Name != "Test Automation" {
		t.Errorf("expected name 'Test Automation', got %q", got.Name)
	}
	if !got.Enabled {
		t.Error("expected enabled = true")
	}
}

func TestCreateAndListTriggers(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "*/5 * * * *"})
	t1 := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}
	if err := store.CreateTrigger(ctx, t1); err != nil {
		t.Fatal(err)
	}

	cfg2, _ := json.Marshal(WebhookTriggerConfig{})
	t2 := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeWebhook, Config: cfg2, Enabled: true}
	if err := store.CreateTrigger(ctx, t2); err != nil {
		t.Fatal(err)
	}

	triggers, err := store.ListTriggers(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggers) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(triggers))
	}

	// Verify trigger hydration on GetAutomation.
	got, _ := store.GetAutomation(ctx, a.ID)
	if len(got.Triggers) != 2 {
		t.Fatalf("expected 2 hydrated triggers, got %d", len(got.Triggers))
	}
}

func TestRunDeduplication(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerID:    "t-1",
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		DedupKey:     "scheduled:t-1:12345",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	exists, err := store.HasRunWithDedupKey(ctx, a.ID, "scheduled:t-1:12345")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected dedup key to exist")
	}

	exists, _ = store.HasRunWithDedupKey(ctx, a.ID, "other-key")
	if exists {
		t.Error("expected other key to not exist")
	}
}

func TestListEnabledTriggersByType(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a1 := &Automation{WorkspaceID: "ws-1", Name: "Enabled", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a1); err != nil {
		t.Fatal(err)
	}
	a2 := &Automation{WorkspaceID: "ws-1", Name: "Disabled", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: false}
	if err := store.CreateAutomation(ctx, a2); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "@hourly"})
	if err := store.CreateTrigger(ctx, &AutomationTrigger{AutomationID: a1.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTrigger(ctx, &AutomationTrigger{AutomationID: a2.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	triggers, err := store.ListEnabledTriggersByType(ctx, TriggerTypeScheduled)
	if err != nil {
		t.Fatal(err)
	}
	// Only one — from the enabled automation.
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger from enabled automation, got %d", len(triggers))
	}
}

func TestUpdateAutomation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "Original", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	newName := "Updated"
	enabled := false
	if err := store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Name: &newName, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetAutomation(ctx, a.ID)
	if got.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", got.Name)
	}
	if got.Enabled {
		t.Error("expected enabled = false")
	}
}

func TestDeleteAutomation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "To Delete", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "@hourly"})
	if err := store.CreateTrigger(ctx, &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteAutomation(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetAutomation(ctx, a.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}
