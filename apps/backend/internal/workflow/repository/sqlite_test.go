package repository

import (
	"context"
	"database/sql"
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

func TestWorkflowStepWIPFields_ReplayMigrationDefaultsExistingRows(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
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
