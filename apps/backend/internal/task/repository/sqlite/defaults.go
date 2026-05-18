package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// Built-in workflow template IDs used by office onboarding to materialise
// the Phase 6 (task-model-unification) workflows. The IDs are present in
// the embedded YAML at config/workflows/.
const (
	templateIDOfficeDefault = "office-default"
	templateIDRoutine       = "routine"
)

// SystemWorkflowTemplateIDs lists workflow template IDs whose tasks are
// treated as "system tasks" — created and maintained by kandev itself
// (today: routine-fired tasks). The Office Tasks UI hides these by
// default; a developer toggle re-includes them. Keep this in sync as
// new system workflows land.
var SystemWorkflowTemplateIDs = []string{
	templateIDRoutine,
}

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

// EnsureOfficeWorkflow makes sure the workspace has an office workflow and
// that workspaces.office_workflow_id points at it. The system workflow is
// the YAML-templated "office-default" one — the legacy 7-step hardcoded
// workflow has been retired. Idempotent.
func (r *Repository) EnsureOfficeWorkflow(ctx context.Context, workspaceID string) (string, error) {
	workflowID, err := r.EnsureOfficeDefaultWorkflow(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if err := r.stampWorkspaceOfficeWorkflow(ctx, workspaceID, workflowID); err != nil {
		return "", err
	}
	return workflowID, nil
}

// stampWorkspaceOfficeWorkflow sets workspaces.office_workflow_id to
// workflowID when the column is empty. Existing non-empty values are
// preserved so manual overrides aren't clobbered.
func (r *Repository) stampWorkspaceOfficeWorkflow(ctx context.Context, workspaceID, workflowID string) error {
	var existing string
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(
		`SELECT COALESCE(office_workflow_id, '') FROM workspaces WHERE id = ?`,
	), workspaceID).Scan(&existing); err != nil {
		return fmt.Errorf("query workspace office workflow: %w", err)
	}
	if existing != "" {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(
		`UPDATE workspaces SET office_workflow_id = ? WHERE id = ?`,
	), workflowID, workspaceID); err != nil {
		return fmt.Errorf("update workspace office_workflow_id: %w", err)
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

// EnsureOfficeDefaultWorkflow materialises the built-in Office Default
// workflow (Backlog -> Work -> Review -> Approval -> Done) for a workspace.
// Idempotent: returns the existing workflow id when one already exists,
// keyed off (workspace_id, workflow_template_id="office-default").
//
// Phase 6 (ADR-0004) — onboarding calls this on first workspace setup so
// office tasks have a workflow with the new event-driven triggers wired.
func (r *Repository) EnsureOfficeDefaultWorkflow(ctx context.Context, workspaceID string) (string, error) {
	return r.ensureBuiltinWorkflow(ctx, workspaceID, templateIDOfficeDefault, "Office Default", "System workflow with primary work, multi-agent review, and multi-agent approval")
}

// EnsureRoutineWorkflow materialises the built-in Routine workflow
// (single auto-completing step) for a workspace. Idempotent.
//
// PR 3 of office-heartbeat-rework — heavy routine fires create a fresh
// task in this workflow; auto_start_agent kicks off the agent on the
// start step and on_turn_complete moves the task to Done.
func (r *Repository) EnsureRoutineWorkflow(ctx context.Context, workspaceID string) (string, error) {
	return r.ensureBuiltinWorkflow(ctx, workspaceID, templateIDRoutine, "Routine", "System workflow for routine-fired tasks")
}

// ensureBuiltinWorkflow creates a workflow + its steps from an embedded
// YAML template if no workflow with that template id exists in the workspace.
func (r *Repository) ensureBuiltinWorkflow(ctx context.Context, workspaceID, templateID, name, description string) (string, error) {
	if workspaceID == "" {
		return "", fmt.Errorf("workspace_id is required")
	}
	existing, err := r.findWorkflowByTemplate(ctx, workspaceID, templateID)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}
	tmpl, err := loadBuiltinTemplate(templateID)
	if err != nil {
		return "", err
	}
	return r.createWorkflowFromTemplate(ctx, workspaceID, name, description, tmpl)
}

// findWorkflowByTemplate returns the workflow id in workspaceID with the
// given workflow_template_id, or empty string when none exists.
func (r *Repository) findWorkflowByTemplate(ctx context.Context, workspaceID, templateID string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id FROM workflows
		WHERE workspace_id = ? AND workflow_template_id = ?
		LIMIT 1
	`), workspaceID, templateID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query workflow by template: %w", err)
	}
	return id, nil
}

// loadBuiltinTemplate reads a single embedded workflow YAML by id.
func loadBuiltinTemplate(templateID string) (*wfmodels.WorkflowTemplate, error) {
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load embedded templates: %w", err)
	}
	for _, t := range templates {
		if t.ID == templateID {
			return t, nil
		}
	}
	return nil, fmt.Errorf("builtin workflow template not found: %s", templateID)
}

// createWorkflowFromTemplate inserts a workflow row and all its steps from a
// loaded template. Step IDs from the template are remapped to fresh UUIDs
// and any move_to_step references updated to match.
func (r *Repository) createWorkflowFromTemplate(
	ctx context.Context, workspaceID, name, description string, tmpl *wfmodels.WorkflowTemplate,
) (string, error) {
	now := time.Now().UTC()
	workflowID := uuid.New().String()

	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflows (id, workspace_id, name, description, workflow_template_id, is_system, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, 999, ?, ?)
	`), workflowID, workspaceID, name, description, tmpl.ID, now, now); err != nil {
		return "", fmt.Errorf("insert builtin workflow %s: %w", tmpl.ID, err)
	}

	idMap := make(map[string]string, len(tmpl.Steps))
	for _, def := range tmpl.Steps {
		idMap[def.ID] = uuid.New().String()
	}
	for _, def := range tmpl.Steps {
		if err := r.insertTemplateStep(ctx, workflowID, def, idMap, now); err != nil {
			return "", err
		}
	}
	return workflowID, nil
}

// insertTemplateStep inserts a workflow step generated from a StepDefinition.
func (r *Repository) insertTemplateStep(
	ctx context.Context, workflowID string, def wfmodels.StepDefinition,
	idMap map[string]string, now time.Time,
) error {
	events := wfmodels.RemapStepEvents(def.Events, idMap)
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("marshal events for step %s: %w", def.Name, err)
	}
	stage := def.StageType
	if stage == "" {
		stage = wfmodels.StageTypeCustom
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, color, prompt, events,
			allow_manual_move, is_start_step, show_in_command_panel,
			auto_archive_after_hours, agent_profile_id, stage_type,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		idMap[def.ID], workflowID, def.Name, def.Position, def.Color, def.Prompt,
		string(eventsJSON), dialect.BoolToInt(def.AllowManualMove),
		dialect.BoolToInt(def.IsStartStep), dialect.BoolToInt(def.ShowInCommandPanel),
		def.AutoArchiveAfterHours, def.AgentProfileID, string(stage),
		now, now,
	)
	if err != nil {
		return fmt.Errorf("insert builtin step %s: %w", def.Name, err)
	}
	return nil
}
