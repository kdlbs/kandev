package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

func TestCreateAutomation_PersistsRepositoryIDsInOrder(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Multi-repo automation",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		Enabled:           true,
		MaxConcurrentRuns: 1,
		RepositoryIDs:     []string{"repo-b", "repo-a"},
	}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected automation, got nil")
	}
	if len(got.RepositoryIDs) != 2 || got.RepositoryIDs[0] != "repo-b" || got.RepositoryIDs[1] != "repo-a" {
		t.Fatalf("expected repository_ids [repo-b repo-a] in order, got %v", got.RepositoryIDs)
	}
}

func TestCreateAutomation_EmptyRepositoryIDs(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "No repo", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RepositoryIDs) != 0 {
		t.Fatalf("expected no repository_ids, got %v", got.RepositoryIDs)
	}
	if got.RepositoryIDs == nil {
		t.Fatal("expected repository_ids to be a non-nil empty slice, got nil (would encode as JSON null)")
	}
}

func TestUpdateAutomation_ReplacesRepositoryIDs(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID: "ws-1", Name: "Original", WorkflowID: "wf-1", WorkflowStepID: "s-1",
		Enabled: true, RepositoryIDs: []string{"repo-a"},
	}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	newRepos := []string{"repo-x", "repo-y", "repo-z"}
	if err := store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{RepositoryIDs: newRepos}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RepositoryIDs) != 3 || got.RepositoryIDs[0] != "repo-x" || got.RepositoryIDs[2] != "repo-z" {
		t.Fatalf("expected repository_ids [repo-x repo-y repo-z], got %v", got.RepositoryIDs)
	}

	// Explicit empty slice clears the list.
	if err := store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{RepositoryIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RepositoryIDs) != 0 {
		t.Fatalf("expected repository_ids cleared, got %v", got.RepositoryIDs)
	}
}

func TestUpdateAutomation_NilRepositoryIDsLeavesUnchanged(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID: "ws-1", Name: "Original", WorkflowID: "wf-1", WorkflowStepID: "s-1",
		Enabled: true, RepositoryIDs: []string{"repo-a"},
	}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	newName := "Renamed"
	if err := store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Name: &newName}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RepositoryIDs) != 1 || got.RepositoryIDs[0] != "repo-a" {
		t.Fatalf("expected repository_ids unchanged [repo-a], got %v", got.RepositoryIDs)
	}
}

func TestListAutomations_BatchHydratesRepositoryIDs(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a1 := &Automation{WorkspaceID: "ws-1", Name: "A1", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true, RepositoryIDs: []string{"repo-1", "repo-2"}}
	a2 := &Automation{WorkspaceID: "ws-1", Name: "A2", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a1); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAutomation(ctx, a2); err != nil {
		t.Fatal(err)
	}

	items, err := store.ListAutomations(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*Automation{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if got := byID[a1.ID].RepositoryIDs; len(got) != 2 || got[0] != "repo-1" || got[1] != "repo-2" {
		t.Fatalf("expected a1 repository_ids [repo-1 repo-2], got %v", got)
	}
	if got := byID[a2.ID].RepositoryIDs; len(got) != 0 {
		t.Fatalf("expected a2 repository_ids empty, got %v", got)
	}
	if byID[a2.ID].RepositoryIDs == nil {
		t.Fatal("expected a2 repository_ids to be a non-nil empty slice, got nil (would encode as JSON null)")
	}
}

// TestInitSchema_BackfillsLegacyRepositoryID seeds a DB shaped like a
// pre-automation_repositories install (automations.repository_id populated,
// no automation_repositories table) and asserts NewStore backfills exactly
// one automation_repositories row per non-empty legacy value, and that
// running it again is a no-op (no duplicate rows).
func TestInitSchema_BackfillsLegacyRepositoryID(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Pre-automation_repositories schema: automations already has
	// repository_id (as ALTER'd by an earlier version) but no join table.
	preSchema := `
		CREATE TABLE automations (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL,
			description TEXT DEFAULT '', workflow_id TEXT NOT NULL, workflow_step_id TEXT NOT NULL,
			agent_profile_id TEXT NOT NULL, executor_profile_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '', prompt TEXT DEFAULT '',
			task_title_template TEXT DEFAULT '', execution_mode TEXT NOT NULL DEFAULT 'task',
			enabled BOOLEAN DEFAULT 1, max_concurrent_runs INTEGER DEFAULT 1,
			webhook_secret TEXT DEFAULT '', last_triggered_at DATETIME,
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		);
		CREATE TABLE automation_triggers (
			id TEXT PRIMARY KEY, automation_id TEXT NOT NULL, type TEXT NOT NULL,
			config TEXT NOT NULL DEFAULT '{}', enabled BOOLEAN DEFAULT 1,
			last_evaluated_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		);
		CREATE TABLE automation_runs (
			id TEXT PRIMARY KEY, automation_id TEXT NOT NULL, trigger_id TEXT NOT NULL,
			trigger_type TEXT NOT NULL, task_id TEXT DEFAULT '', status TEXT NOT NULL,
			dedup_key TEXT DEFAULT '', trigger_data TEXT NOT NULL DEFAULT '{}',
			error_message TEXT DEFAULT '', created_at DATETIME NOT NULL
		);
	`
	if _, err := db.Exec(preSchema); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(
		`INSERT INTO automations (id, workspace_id, name, workflow_id, workflow_step_id,
			agent_profile_id, executor_profile_id, repository_id, created_at, updated_at)
		VALUES ('a1', 'ws-1', 'Legacy', 'wf-1', 's-1', '', '', 'repo-legacy', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.GetAutomation(context.Background(), "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RepositoryIDs) != 1 || got.RepositoryIDs[0] != "repo-legacy" {
		t.Fatalf("expected backfilled repository_ids [repo-legacy], got %v", got.RepositoryIDs)
	}

	// Re-running initSchema (as NewStore does on every boot) must not
	// duplicate the backfilled row.
	if _, err := NewStore(db, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM automation_repositories WHERE automation_id = 'a1'`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 backfilled row after repeated init, got %d", count)
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

func TestDeleteRun(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		TaskID:       "task-abc",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	// GetRun finds it.
	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("expected run, got nil")
		return
	}
	if got.TaskID != "task-abc" {
		t.Errorf("expected task_id 'task-abc', got %q", got.TaskID)
	}

	// DeleteRun removes it.
	if err := store.DeleteRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil after delete, got run")
	}
}

func TestMarkRunFailedByTaskID_PreservesMergedPRDedupKey(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-failed", Name: "Merged cleanup", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	const dedupKey = "pr_merged:task-1:acme/api#7"
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeGitHubPRMerged,
		TaskID: "task-1", Status: RunStatusTaskCreated, DedupKey: dedupKey,
		TriggerData: json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunFailedByTaskID(ctx, "task-1", "agent failed"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RunStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.DedupKey != dedupKey {
		t.Fatalf("dedup key = %q, want %q", got.DedupKey, dedupKey)
	}
}

func TestDeleteAllRuns(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "B", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusSkipped,
			TaskID:       "task-" + string(rune('0'+i)),
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	taskIDs, err := store.ListRunTaskIDs(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(taskIDs) != 3 {
		t.Fatalf("expected 3 task IDs, got %d", len(taskIDs))
	}

	if err := store.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListRuns(ctx, a.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs after delete, got %d", len(runs))
	}
}

// createTasksTable adds a minimal shadow of the task repository's `tasks`
// and `task_sessions` tables (id/archived_at, and task_id/is_primary/state
// respectively) to the store's DB. The automation package never owns
// these tables — apps/backend/internal/task/repository/sqlite is the
// canonical owner — so only tests that exercise the CountActiveRuns/
// ListRuns task-state join create them; production always has the real
// tables already migrated by the task repository before automation
// triggers can fire.

// Only enabled automations count as a reason you cannot delete a profile: a
// disabled one is not going to fire, so naming it would be noise in the
// confirmation dialog.
func TestListEnabledByAgentProfile_OnlyEnabledAndOnlyThatProfile(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	for _, spec := range []struct {
		name     string
		profile  string
		enabled  bool
		expected bool
	}{
		{"nightly sweep", "profile-a", true, true},
		{"paused sweep", "profile-a", false, false},
		{"someone else's", "profile-b", true, false},
	} {
		a := &Automation{
			WorkspaceID: "ws-1", Name: spec.name,
			AgentProfileID: spec.profile, Enabled: spec.enabled,
		}
		if err := store.CreateAutomation(ctx, a); err != nil {
			t.Fatalf("CreateAutomation(%s): %v", spec.name, err)
		}
	}

	got, err := store.ListEnabledByAgentProfile(ctx, "profile-a")
	if err != nil {
		t.Fatalf("ListEnabledByAgentProfile: %v", err)
	}
	if len(got) != 1 || got[0].Name != "nightly sweep" {
		t.Fatalf("expected only the enabled automation for profile-a, got %+v", got)
	}
	if got[0].WorkspaceID != "ws-1" {
		t.Errorf("workspace should travel with the reference, got %q", got[0].WorkspaceID)
	}
}

func TestListEnabledByAgentProfile_EmptyProfileMatchesNothing(t *testing.T) {
	store := setupTestStore(t)
	// An unset agent_profile_id would otherwise match every automation that
	// also has none, which is the opposite of "these block your deletion".
	got, err := store.ListEnabledByAgentProfile(context.Background(), "")
	if err != nil {
		t.Fatalf("ListEnabledByAgentProfile: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no matches for an empty profile id, got %+v", got)
	}
}
