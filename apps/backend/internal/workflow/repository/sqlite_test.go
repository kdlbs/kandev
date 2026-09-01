package repository

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/workflow/models"
)

func setupTestRepo(t *testing.T) *Repository {
	repo, _ := setupTestRepoWithDB(t)
	return repo
}

func setupTestRepoWithDB(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	// Enable FK enforcement explicitly so workflow_step_participants
	// ON DELETE CASCADE behaves as designed in the cascade test.
	if _, err := sqlxDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	// Create workflows table (normally created by task repo)
	_, err = sqlxDB.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		t.Fatalf("failed to create workflows table: %v", err)
	}

	// Create task_sessions table (referenced by session_step_history FK)
	_, err = sqlxDB.Exec(`CREATE TABLE IF NOT EXISTS task_sessions (
		id TEXT PRIMARY KEY
	)`)
	if err != nil {
		t.Fatalf("failed to create task_sessions table: %v", err)
	}

	// Insert a test workflow
	_, err = sqlxDB.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-test', '', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("failed to insert test workflow: %v", err)
	}

	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	return repo, sqlxDB
}

func TestStepAgentProfileID_CreateAndGet(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID:     "wf-test",
		Name:           "Test Step",
		Position:       0,
		Color:          "#000000",
		AgentProfileID: "agent-profile-abc",
	}

	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("failed to create step: %v", err)
	}
	if step.ID == "" {
		t.Fatal("expected step ID to be set")
	}

	retrieved, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if retrieved.AgentProfileID != "agent-profile-abc" {
		t.Errorf("expected agent_profile_id 'agent-profile-abc', got %q", retrieved.AgentProfileID)
	}
}

func TestDeleteStep_ClearsQueuedTaskDestinationAndDeferredLaunch(t *testing.T) {
	repo, db := setupTestRepoWithDB(t)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY, queued_for_step_id TEXT, queued_at TIMESTAMP,
		wip_admitted BOOLEAN, metadata TEXT, updated_at TIMESTAMP
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	step := &models.WorkflowStep{WorkflowID: "wf-test", Name: "Queued", Position: 0}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tasks (id, queued_for_step_id, wip_admitted, metadata) VALUES ('queued', ?, 0, ?)`, step.ID, `{"deferred_launch":{"agent_profile_id":"agent"}}`); err != nil {
		t.Fatalf("insert queued task: %v", err)
	}
	if err := repo.DeleteStep(ctx, step.ID); err != nil {
		t.Fatalf("delete step: %v", err)
	}
	var queuedFor string
	var admitted bool
	var metadata string
	if err := db.QueryRow(`SELECT queued_for_step_id, wip_admitted, metadata FROM tasks WHERE id = 'queued'`).Scan(&queuedFor, &admitted, &metadata); err != nil {
		t.Fatalf("reload queued task: %v", err)
	}
	if queuedFor != "" || !admitted || metadata != `{}` {
		t.Fatalf("cleanup result: queue=%q admitted=%t metadata=%q", queuedFor, admitted, metadata)
	}
}

func TestWorkflowStepWIPFields_CreateUpdateAndGet(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	feeder := &models.WorkflowStep{
		WorkflowID: "wf-test",
		Name:       "Queue",
		Position:   0,
		Color:      "#999999",
	}
	if err := repo.CreateStep(ctx, feeder); err != nil {
		t.Fatalf("failed to create feeder step: %v", err)
	}

	step := &models.WorkflowStep{
		WorkflowID:      "wf-test",
		Name:            "Work",
		Position:        1,
		Color:           "#000000",
		WIPLimit:        2,
		PullFromStepID:  feeder.ID,
		AllowManualMove: true,
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("failed to create step: %v", err)
	}

	retrieved, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if retrieved.WIPLimit != 2 {
		t.Fatalf("WIPLimit = %d, want 2", retrieved.WIPLimit)
	}
	if retrieved.PullFromStepID != feeder.ID {
		t.Fatalf("PullFromStepID = %q, want %q", retrieved.PullFromStepID, feeder.ID)
	}

	retrieved.WIPLimit = 1
	retrieved.PullFromStepID = ""
	if err := repo.UpdateStep(ctx, retrieved); err != nil {
		t.Fatalf("failed to update step: %v", err)
	}
	updated, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("failed to get updated step: %v", err)
	}
	if updated.WIPLimit != 1 {
		t.Fatalf("updated WIPLimit = %d, want 1", updated.WIPLimit)
	}
	if updated.PullFromStepID != "" {
		t.Fatalf("updated PullFromStepID = %q, want empty", updated.PullFromStepID)
	}
}

func TestWorkflowStepCancelTriggersTurnComplete_CreateUpdateAndGet(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := &models.WorkflowStep{WorkflowID: "wf-test", Name: "Cancelable", Position: 0}
	field := reflect.ValueOf(step).Elem().FieldByName("CancelTriggersTurnComplete")
	if !field.IsValid() {
		t.Fatal("WorkflowStep is missing CancelTriggersTurnComplete")
	}
	field.SetBool(true)
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	retrieved, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	retrievedField := reflect.ValueOf(retrieved).Elem().FieldByName("CancelTriggersTurnComplete")
	if !retrievedField.IsValid() {
		t.Fatal("WorkflowStep is missing CancelTriggersTurnComplete after read")
	}
	if !retrievedField.Bool() {
		t.Fatal("cancel trigger was not persisted as true")
	}
	retrievedField.SetBool(false)
	if err := repo.UpdateStep(ctx, retrieved); err != nil {
		t.Fatalf("update step: %v", err)
	}
	updated, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get updated step: %v", err)
	}
	updatedField := reflect.ValueOf(updated).Elem().FieldByName("CancelTriggersTurnComplete")
	if updatedField.Bool() {
		t.Fatal("cancel trigger remained true after explicit false update")
	}
}

func TestWorkflowStepCancelTriggersTurnComplete_ReplayMigrationDefaultsExistingRows(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
			description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY);
		CREATE TABLE workflow_steps (
			id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, name TEXT NOT NULL,
			position INTEGER NOT NULL, color TEXT, prompt TEXT, events TEXT,
			allow_manual_move INTEGER DEFAULT 1, is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1, auto_archive_after_hours INTEGER DEFAULT 0,
			agent_profile_id TEXT DEFAULT '', stage_type TEXT NOT NULL DEFAULT 'custom',
			auto_advance_requires_signal INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		);
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
			VALUES ('wf-replay-cancel', '', 'Replay', datetime('now'), datetime('now'));
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
			VALUES ('legacy-cancel-step', 'wf-replay-cancel', 'Legacy', 0, datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("seed legacy database: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("reopen repo: %v", err)
	}
	var columnCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('workflow_steps') WHERE name = 'cancel_triggers_turn_complete'`).Scan(&columnCount); err != nil {
		t.Fatalf("inspect migrated schema: %v", err)
	}
	if columnCount != 1 {
		t.Fatalf("cancel trigger migration column count = %d, want 1", columnCount)
	}
	step, err := repo.GetStep(context.Background(), "legacy-cancel-step")
	if err != nil {
		t.Fatalf("get legacy step: %v", err)
	}
	field := reflect.ValueOf(step).Elem().FieldByName("CancelTriggersTurnComplete")
	if !field.IsValid() {
		t.Fatal("WorkflowStep is missing CancelTriggersTurnComplete")
	}
	if field.Bool() {
		t.Fatal("legacy row cancel trigger defaulted true; want false")
	}
}

func TestWorkflowStepWIPFields_ReplayMigrationDefaultsExistingRows(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
			description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY);
		CREATE TABLE workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT,
			prompt TEXT,
			events TEXT,
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1,
			auto_archive_after_hours INTEGER DEFAULT 0,
			agent_profile_id TEXT DEFAULT '',
			stage_type TEXT NOT NULL DEFAULT 'custom',
			auto_advance_requires_signal INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		);
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
			VALUES ('wf-test', '', 'Test', datetime('now'), datetime('now'));
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, created_at, updated_at
		) VALUES ('legacy-step', 'wf-test', 'Legacy Step', 0, datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("seed legacy database: %v", err)
	}

	reopened, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("reopen repo: %v", err)
	}
	step, err := reopened.GetStep(context.Background(), "legacy-step")
	if err != nil {
		t.Fatalf("get legacy step: %v", err)
	}
	if step.WIPLimit != 0 {
		t.Fatalf("WIPLimit = %d, want default 0", step.WIPLimit)
	}
	if step.PullFromStepID != "" {
		t.Fatalf("PullFromStepID = %q, want default empty", step.PullFromStepID)
	}
}

func TestClearStepReferencesClearsPullSource(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	feeder := &models.WorkflowStep{
		WorkflowID: "wf-test",
		Name:       "Queue",
		Position:   0,
	}
	if err := repo.CreateStep(ctx, feeder); err != nil {
		t.Fatalf("failed to create feeder: %v", err)
	}
	consumer := &models.WorkflowStep{
		WorkflowID:     "wf-test",
		Name:           "Work",
		Position:       1,
		WIPLimit:       1,
		PullFromStepID: feeder.ID,
	}
	if err := repo.CreateStep(ctx, consumer); err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	if err := repo.ClearStepReferences(ctx, "wf-test", feeder.ID); err != nil {
		t.Fatalf("clear references: %v", err)
	}
	got, err := repo.GetStep(ctx, consumer.ID)
	if err != nil {
		t.Fatalf("get consumer: %v", err)
	}
	if got.PullFromStepID != "" {
		t.Fatalf("PullFromStepID = %q, want empty", got.PullFromStepID)
	}
}

func TestClearStepReferencesClearsAllTransitionTriggers(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	target := &models.WorkflowStep{
		ID:         "target-step",
		WorkflowID: "wf-test",
		Name:       "Target",
		Position:   0,
	}
	if err := repo.CreateStep(ctx, target); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	moveToTarget := func() models.GenericAction {
		return models.GenericAction{
			Type:   models.GenericActionMoveToStep,
			Config: map[string]interface{}{"step_id": target.ID},
		}
	}
	genericActions := []models.GenericAction{moveToTarget(), {Type: models.GenericActionAutoStartAgent}}
	source := &models.WorkflowStep{
		ID:         "source-step",
		WorkflowID: "wf-test",
		Name:       "Source",
		Position:   1,
		Events: models.StepEvents{
			OnTurnStart: []models.OnTurnStartAction{
				{Type: models.OnTurnStartMoveToStep, Config: map[string]interface{}{"step_id": target.ID}},
				{Type: models.OnTurnStartMoveToNext},
			},
			OnTurnComplete: []models.OnTurnCompleteAction{
				{Type: models.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": target.ID}},
				{Type: models.OnTurnCompleteMoveToNext},
			},
			OnComment:           genericActions,
			OnBlockerResolved:   genericActions,
			OnChildrenCompleted: genericActions,
			OnApprovalResolved:  genericActions,
			OnHeartbeat:         genericActions,
			OnBudgetAlert:       genericActions,
			OnAgentError:        genericActions,
		},
	}
	if err := repo.CreateStep(ctx, source); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	if err := repo.ClearStepReferences(ctx, "wf-test", target.ID); err != nil {
		t.Fatalf("clear references: %v", err)
	}

	got, err := repo.GetStep(ctx, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if len(got.Events.OnTurnStart) != 1 || got.Events.OnTurnStart[0].Type != models.OnTurnStartMoveToNext {
		t.Fatalf("OnTurnStart = %#v, want only move_to_next", got.Events.OnTurnStart)
	}
	if len(got.Events.OnTurnComplete) != 1 || got.Events.OnTurnComplete[0].Type != models.OnTurnCompleteMoveToNext {
		t.Fatalf("OnTurnComplete = %#v, want only move_to_next", got.Events.OnTurnComplete)
	}

	for trigger, actions := range map[string][]models.GenericAction{
		"on_comment":            got.Events.OnComment,
		"on_blocker_resolved":   got.Events.OnBlockerResolved,
		"on_children_completed": got.Events.OnChildrenCompleted,
		"on_approval_resolved":  got.Events.OnApprovalResolved,
		"on_heartbeat":          got.Events.OnHeartbeat,
		"on_budget_alert":       got.Events.OnBudgetAlert,
		"on_agent_error":        got.Events.OnAgentError,
	} {
		if len(actions) != 1 || actions[0].Type != models.GenericActionAutoStartAgent {
			t.Errorf("%s = %#v, want only auto_start_agent", trigger, actions)
		}
	}
}

func TestStepAgentProfileID_Update(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID:     "wf-test",
		Name:           "Update Step",
		Position:       0,
		AgentProfileID: "profile-original",
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("failed to create step: %v", err)
	}

	// Update agent_profile_id
	step.AgentProfileID = "profile-updated"
	if err := repo.UpdateStep(ctx, step); err != nil {
		t.Fatalf("failed to update step: %v", err)
	}

	retrieved, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if retrieved.AgentProfileID != "profile-updated" {
		t.Errorf("expected agent_profile_id 'profile-updated', got %q", retrieved.AgentProfileID)
	}

	// Clear agent_profile_id
	step.AgentProfileID = ""
	if err := repo.UpdateStep(ctx, step); err != nil {
		t.Fatalf("failed to update step: %v", err)
	}
	retrieved, _ = repo.GetStep(ctx, step.ID)
	if retrieved.AgentProfileID != "" {
		t.Errorf("expected empty agent_profile_id, got %q", retrieved.AgentProfileID)
	}
}

func TestStepAgentProfileID_ListByWorkflow(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step1 := &models.WorkflowStep{
		WorkflowID:     "wf-test",
		Name:           "Step 1",
		Position:       0,
		AgentProfileID: "profile-a",
	}
	step2 := &models.WorkflowStep{
		WorkflowID:     "wf-test",
		Name:           "Step 2",
		Position:       1,
		AgentProfileID: "profile-b",
	}
	if err := repo.CreateStep(ctx, step1); err != nil {
		t.Fatalf("failed to create step1: %v", err)
	}
	if err := repo.CreateStep(ctx, step2); err != nil {
		t.Fatalf("failed to create step2: %v", err)
	}

	steps, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	if err != nil {
		t.Fatalf("failed to list steps: %v", err)
	}
	// Filter out any seeded steps (migration may have seeded defaults)
	var testSteps []*models.WorkflowStep
	for _, s := range steps {
		if s.AgentProfileID == "profile-a" || s.AgentProfileID == "profile-b" {
			testSteps = append(testSteps, s)
		}
	}
	if len(testSteps) != 2 {
		t.Fatalf("expected 2 steps with agent profiles, got %d", len(testSteps))
	}
	if testSteps[0].AgentProfileID != "profile-a" {
		t.Errorf("expected first step agent_profile_id 'profile-a', got %q", testSteps[0].AgentProfileID)
	}
	if testSteps[1].AgentProfileID != "profile-b" {
		t.Errorf("expected second step agent_profile_id 'profile-b', got %q", testSteps[1].AgentProfileID)
	}
}

func TestStepAgentProfileID_EmptyByDefault(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{
		WorkflowID: "wf-test",
		Name:       "No Profile Step",
		Position:   0,
	}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("failed to create step: %v", err)
	}

	retrieved, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if retrieved.AgentProfileID != "" {
		t.Errorf("expected empty agent_profile_id by default, got %q", retrieved.AgentProfileID)
	}
}

func TestInitSchema_NormalizesDuplicateStartSteps(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	_, err = db.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
			description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY);
		CREATE TABLE workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT,
			prompt TEXT,
			events TEXT,
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1,
			auto_archive_after_hours INTEGER DEFAULT 0,
			agent_profile_id TEXT DEFAULT '',
			stage_type TEXT NOT NULL DEFAULT 'custom',
			auto_advance_requires_signal INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		);
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
			VALUES ('wf-test', '', 'Test', datetime('now'), datetime('now'));
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, is_start_step, created_at, updated_at
		) VALUES
			('latest-position-start', 'wf-test', 'Latest Position Start', 2, 1, datetime('2026-01-01T00:00:00Z'), datetime('2026-01-01T00:00:00Z')),
			('latest-updated-start', 'wf-test', 'Latest Updated Start', 0, 1, datetime('2026-01-01T00:00:00Z'), datetime('2026-01-02T00:00:00Z'));
	`)
	if err != nil {
		t.Fatalf("seed pre-repair database: %v", err)
	}

	reopened, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("reopen repo: %v", err)
	}
	steps, err := reopened.ListStepsByWorkflow(ctx, "wf-test")
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}

	var starts []string
	for _, step := range steps {
		if step.IsStartStep {
			starts = append(starts, step.Name)
		}
	}
	if len(starts) != 1 || starts[0] != "Latest Updated Start" {
		t.Fatalf("expected only Latest Updated Start to remain start, got %v", starts)
	}
}

func TestCreateStep_RollsBackStartDemotionWhenInsertFails(t *testing.T) {
	repo, db := setupTestRepoWithDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`
		DELETE FROM workflow_steps WHERE workflow_id = 'wf-test';
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, is_start_step, created_at, updated_at
		) VALUES
			('old-start', 'wf-test', 'Old Start', 0, 1, datetime('now'), datetime('now')),
			('duplicate-target', 'wf-test', 'Duplicate Target', 1, 0, datetime('now'), datetime('now'));
	`); err != nil {
		t.Fatalf("seed rollback workflow steps: %v", err)
	}

	err := repo.CreateStep(ctx, &models.WorkflowStep{
		ID:          "duplicate-target",
		WorkflowID:  "wf-test",
		Name:        "Duplicate ID Start",
		Position:    99,
		IsStartStep: true,
	})
	if err == nil {
		t.Fatal("expected duplicate ID insert to fail")
	}

	start, err := repo.GetStartStep(ctx, "wf-test")
	if err != nil {
		t.Fatalf("get start step: %v", err)
	}
	if start == nil || start.ID != "old-start" {
		t.Fatalf("expected original start step %q to remain after rollback, got %#v", "old-start", start)
	}
}
