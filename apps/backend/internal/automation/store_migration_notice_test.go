package automation

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// seedPreWithdrawalAutomation writes an automation row the way an install
// running the old code wrote one — with an execution_mode the user chose.
// Going through raw SQL is the point: CreateAutomation no longer offers that
// choice, so the only honest way to produce a pre-upgrade row is to write one.
func seedPreWithdrawalAutomation(t *testing.T, store *Store, id, workspaceID, executionMode string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := store.db.Exec(
		`INSERT INTO automations (id, workspace_id, name, description, workflow_id, workflow_step_id,
			agent_profile_id, executor_profile_id, prompt, task_title_template, execution_mode,
			enabled, max_concurrent_runs, webhook_secret, created_at, updated_at)
		VALUES (?, ?, ?, '', 'wf-1', 'step-1', 'agent-1', 'exec-1', 'report', '', ?, 1, 1, 'secret', ?, ?)`,
		id, workspaceID, id, executionMode, now, now)
	if err != nil {
		t.Fatalf("seed %s automation %s: %v", executionMode, id, err)
	}
}

// An automation stored in `task` mode is the one whose board cards stop
// appearing after the upgrade, and `task` was the DEFAULT — so this is the
// common case, not an exotic one. The list read path is where the automations
// UI learns it has something to say.
func TestListAutomations_FlagsOnlyAutomationsStoredInTaskExecutionMode(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	seedPreWithdrawalAutomation(t, store, "a-task", "ws-1", "task")
	seedPreWithdrawalAutomation(t, store, "a-run", "ws-1", "run")

	automations, err := store.ListAutomations(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	if len(automations) != 2 {
		t.Fatalf("expected both seeded automations, got %d", len(automations))
	}
	flags := map[string]bool{}
	for _, a := range automations {
		flags[a.ID] = a.LegacyBoardCard
	}
	if !flags["a-task"] {
		t.Error("an automation stored in task mode used to produce a board card and must be flagged")
	}
	if flags["a-run"] {
		t.Error("an automation stored in run mode never produced a board card; nothing changed for it")
	}
}

// The same derivation has to hold on the single-automation read, or the
// detail surface would disagree with the list it was opened from.
func TestGetAutomation_FlagsAutomationStoredInTaskExecutionMode(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	seedPreWithdrawalAutomation(t, store, "a-task", "ws-1", "task")

	got, err := store.GetAutomation(ctx, "a-task")
	if err != nil {
		t.Fatalf("GetAutomation: %v", err)
	}
	if got == nil {
		t.Fatal("expected the seeded automation back")
	}
	if !got.LegacyBoardCard {
		t.Error("expected the stored task mode to be reported on the single-automation read too")
	}
}

// The notice must close. execution_mode's column DEFAULT is 'task', so an
// automation created after the withdrawal would look pre-upgrade forever if
// the insert left the DEFAULT to fill it — the "one-time" notice would come
// back for every automation the user ever creates.
func TestCreateAutomation_NewAutomationIsNotFlaggedAsLegacy(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "created after the withdrawal",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		Enabled:           true,
		MaxConcurrentRuns: 1,
	}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}

	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAutomation: %v", err)
	}
	if got == nil {
		t.Fatal("expected the created automation back")
	}
	if got.LegacyBoardCard {
		t.Error("an automation created after the withdrawal never had a board card to lose")
	}
}

// A DB initialised before execution_mode existed at all still has to answer
// the derivation — the ALTER that adds the column runs on every boot for
// exactly this reason, and the column's DEFAULT correctly reports those rows
// as pre-upgrade ones.
func TestInitSchema_DerivesLegacyFlagOnADBPredatingExecutionMode(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	preSchema := `
		CREATE TABLE automations (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL,
			description TEXT DEFAULT '', workflow_id TEXT NOT NULL, workflow_step_id TEXT NOT NULL,
			agent_profile_id TEXT NOT NULL, executor_profile_id TEXT NOT NULL,
			prompt TEXT DEFAULT '', enabled BOOLEAN DEFAULT 1, max_concurrent_runs INTEGER DEFAULT 1,
			webhook_secret TEXT DEFAULT '', last_triggered_at DATETIME,
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		);
	`
	if _, err := db.Exec(preSchema); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(
		`INSERT INTO automations (id, workspace_id, name, workflow_id, workflow_step_id,
			agent_profile_id, executor_profile_id, created_at, updated_at)
		VALUES ('a-ancient', 'ws-1', 'Ancient', 'wf-1', 's-1', '', '', ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("NewStore on a pre-execution_mode DB: %v", err)
	}

	got, err := store.GetAutomation(context.Background(), "a-ancient")
	if err != nil {
		t.Fatalf("GetAutomation: %v", err)
	}
	if got == nil {
		t.Fatal("expected the ancient automation back")
	}
	if !got.LegacyBoardCard {
		t.Error("an automation that predates execution_mode ran in the task default and lost its card too")
	}
}
