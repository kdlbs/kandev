package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ensureDefaultWorkspace creates a default workspace if none exists
func (r *Repository) ensureDefaultWorkspace() error {
	ctx := context.Background()

	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM workspaces").Scan(&count); err != nil {
		return err
	}

	if count == 0 {
		if err := r.createInitialWorkspace(ctx); err != nil {
			return err
		}
	}

	var defaultWorkspaceID string
	if err := r.db.QueryRowContext(ctx, "SELECT id FROM workspaces ORDER BY created_at LIMIT 1").Scan(&defaultWorkspaceID); err != nil {
		return err
	}

	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflows SET workspace_id = ? WHERE workspace_id = '' OR workspace_id IS NULL
	`), defaultWorkspaceID); err != nil {
		return err
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET workspace_id = (
			SELECT workspace_id FROM workflows WHERE workflows.id = tasks.workflow_id
		)
		WHERE workspace_id = '' OR workspace_id IS NULL
	`); err != nil {
		return err
	}

	return nil
}

// createInitialWorkspace inserts the first workspace and optionally a default workflow.
func (r *Repository) createInitialWorkspace(ctx context.Context) error {
	var workflowCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM workflows").Scan(&workflowCount); err != nil {
		return err
	}
	var taskCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM tasks").Scan(&taskCount); err != nil {
		return err
	}
	defaultID := uuid.New().String()
	now := time.Now().UTC()
	workspaceName := "Default Workspace"
	workspaceDescription := "Default workspace"
	if workflowCount > 0 || taskCount > 0 {
		workspaceName = "Migrated Workspace"
		workspaceDescription = ""
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workspaces (
			id,
			name,
			description,
			owner_id,
			default_executor_id,
			default_environment_id,
			default_agent_profile_id,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), defaultID, workspaceName, workspaceDescription, "", nil, nil, nil, now, now); err != nil {
		return err
	}
	if workflowCount == 0 && taskCount == 0 {
		workflowID := uuid.New().String()
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO workflows (id, workspace_id, name, description, workflow_template_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`), workflowID, defaultID, "Development", "Default development workflow", "simple", now, now); err != nil {
			return err
		}
		// Note: Workflow steps will be created by the workflow repository after it initializes
	}
	return nil
}

// EnsureOrchestrateWorkflow creates or re-reads the system Orchestrate workflow for a workspace.
// It is called by the orchestrate provider after all schema migrations have run.
func (r *Repository) EnsureOrchestrateWorkflow(ctx context.Context, workspaceID string) (string, error) {
	// Check if workspace already has an orchestrate workflow
	var existingID string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(
		`SELECT orchestrate_workflow_id FROM workspaces WHERE id = ?`), workspaceID).Scan(&existingID)
	if err != nil {
		return "", fmt.Errorf("query workspace orchestrate workflow: %w", err)
	}
	if existingID != "" {
		return existingID, nil
	}
	return r.createOrchestrateWorkflow(ctx, workspaceID)
}

// createOrchestrateWorkflow inserts the system Orchestrate workflow and its steps.
func (r *Repository) createOrchestrateWorkflow(ctx context.Context, workspaceID string) (string, error) {
	now := time.Now().UTC()
	workflowID := uuid.New().String()

	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflows (id, workspace_id, name, description, workflow_template_id, is_system, sort_order, created_at, updated_at)
		VALUES (?, ?, 'Orchestrate', 'System workflow for orchestrate tasks', '', 1, 999, ?, ?)
	`), workflowID, workspaceID, now, now); err != nil {
		return "", fmt.Errorf("insert orchestrate workflow: %w", err)
	}

	if err := r.insertOrchestrateSteps(ctx, workflowID, now); err != nil {
		return "", err
	}

	if _, err := r.db.ExecContext(ctx, r.db.Rebind(
		`UPDATE workspaces SET orchestrate_workflow_id = ? WHERE id = ?`), workflowID, workspaceID); err != nil {
		return "", fmt.Errorf("update workspace orchestrate_workflow_id: %w", err)
	}
	return workflowID, nil
}

// insertOrchestrateSteps creates the fixed workflow steps for the Orchestrate workflow.
func (r *Repository) insertOrchestrateSteps(ctx context.Context, workflowID string, now time.Time) error {
	steps := []struct {
		name     string
		position int
		isStart  int
	}{
		{"Backlog", 0, 0},
		{"Todo", 1, 1},
		{"In Progress", 2, 0},
		{"In Review", 3, 0},
		{"Blocked", 4, 0},
		{"Done", 5, 0},
		{"Cancelled", 6, 0},
	}
	for _, step := range steps {
		stepID := uuid.New().String()
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO workflow_steps (id, workflow_id, name, position, color, prompt, events, allow_manual_move, is_start_step, show_in_command_panel, created_at, updated_at)
			VALUES (?, ?, ?, ?, '', '', '{}', 1, ?, 1, ?, ?)
		`), stepID, workflowID, step.name, step.position, step.isStart, now, now); err != nil {
			return fmt.Errorf("insert orchestrate step %s: %w", step.name, err)
		}
	}
	return nil
}

// ensureDefaultExecutorsAndEnvironments creates default executors and environments if none exist
func (r *Repository) ensureDefaultExecutorsAndEnvironments() error {
	ctx := context.Background()
	if err := r.ensureDefaultExecutors(ctx); err != nil {
		return err
	}
	if err := r.ensureDefaultExecutorProfiles(ctx); err != nil {
		return err
	}
	return r.ensureDefaultEnvironment(ctx)
}

func (r *Repository) ensureDefaultExecutors(ctx context.Context) error {
	var executorCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM executors").Scan(&executorCount); err != nil {
		return err
	}
	if executorCount == 0 {
		return r.insertDefaultExecutors(ctx)
	}
	// Ensure system executors have is_system flag set
	for _, systemID := range []string{models.ExecutorIDLocal, models.ExecutorIDWorktree} {
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE executors SET is_system = 1 WHERE id = ?
		`), systemID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) insertDefaultExecutors(ctx context.Context) error {
	now := time.Now().UTC()
	executors := []struct {
		id        string
		name      string
		execType  models.ExecutorType
		status    models.ExecutorStatus
		isSystem  bool
		resumable bool
		config    map[string]string
	}{
		{id: models.ExecutorIDLocal, name: "Local", execType: models.ExecutorTypeLocal, status: models.ExecutorStatusActive, isSystem: true, resumable: true, config: map[string]string{}},
		{id: models.ExecutorIDWorktree, name: "Worktree", execType: models.ExecutorTypeWorktree, status: models.ExecutorStatusActive, isSystem: true, resumable: true, config: map[string]string{}},
		{id: models.ExecutorIDLocalDocker, name: "Local Docker", execType: models.ExecutorTypeLocalDocker, status: models.ExecutorStatusActive, isSystem: false, resumable: true, config: map[string]string{"docker_host": config.DefaultDockerHost()}},
		{id: models.ExecutorIDSprites, name: "Sprites.dev", execType: models.ExecutorTypeSprites, status: models.ExecutorStatusDisabled, isSystem: false, resumable: true, config: map[string]string{}},
	}
	for _, executor := range executors {
		configJSON, err := json.Marshal(executor.config)
		if err != nil {
			return fmt.Errorf("failed to serialize executor config: %w", err)
		}
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO executors (id, name, type, status, is_system, resumable, config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), executor.id, executor.name, executor.execType, executor.status, dialect.BoolToInt(executor.isSystem), dialect.BoolToInt(executor.resumable), string(configJSON), now, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ensureDefaultExecutorProfiles(ctx context.Context) error {
	profileSeeds := []struct {
		executorID string
		name       string
	}{
		{models.ExecutorIDLocal, "Local"},
		{models.ExecutorIDWorktree, "Worktree"},
	}
	for _, seed := range profileSeeds {
		var profileCount int
		if err := r.db.QueryRowContext(ctx, r.db.Rebind(
			"SELECT COUNT(1) FROM executor_profiles WHERE executor_id = ?",
		), seed.executorID).Scan(&profileCount); err != nil {
			return err
		}
		if profileCount == 0 {
			now := time.Now().UTC()
			id := uuid.New().String()
			if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
				INSERT INTO executor_profiles (id, executor_id, name, mcp_policy, config, prepare_script, cleanup_script, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`), id, seed.executorID, seed.name, "", "{}", "", "", now, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) ensureDefaultEnvironment(ctx context.Context) error {
	var envCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM environments").Scan(&envCount); err != nil {
		return err
	}
	if envCount == 0 {
		now := time.Now().UTC()
		_, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), models.EnvironmentIDLocal, "Local", models.EnvironmentKindLocalPC, dialect.BoolToInt(true), "~/kandev", "", "", "{}", now, now)
		return err
	}
	return r.updateDefaultEnvironment(ctx)
}

func (r *Repository) updateDefaultEnvironment(ctx context.Context) error {
	var localCount int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind("SELECT COUNT(1) FROM environments WHERE id = ?"), models.EnvironmentIDLocal).Scan(&localCount); err != nil {
		return err
	}
	if localCount == 0 {
		now := time.Now().UTC()
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), models.EnvironmentIDLocal, "Local", models.EnvironmentKindLocalPC, dialect.BoolToInt(true), "~/kandev", "", "", "{}", now, now); err != nil {
			return err
		}
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE environments SET is_system = 1, image_tag = '', dockerfile = '', build_config = '{}' WHERE id = ?
	`), models.EnvironmentIDLocal); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE environments SET worktree_root = ? WHERE id = ? AND (worktree_root IS NULL OR worktree_root = '')
	`), "~/kandev", models.EnvironmentIDLocal)
	return err
}
