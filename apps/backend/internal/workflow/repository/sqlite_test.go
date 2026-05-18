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
	return repo
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
