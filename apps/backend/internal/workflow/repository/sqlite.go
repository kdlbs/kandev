// Package repository provides SQLite-based repository implementations for workflow entities.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/sqlite"
	"github.com/kandev/kandev/internal/workflow/models"
)

// Repository provides SQLite-based workflow storage operations.
type Repository struct {
	db *sql.DB
}

// NewWithDB creates a new SQLite repository with an existing database connection.
func NewWithDB(db *sql.DB) (*Repository, error) {
	repo := &Repository{db: db}
	if err := repo.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize workflow schema: %w", err)
	}
	return repo, nil
}

// DB returns the underlying sql.DB instance for shared access.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// initSchema creates the database tables if they don't exist.
func (r *Repository) initSchema() error {
	// Create workflow_templates table
	templatesSchema := `
	CREATE TABLE IF NOT EXISTS workflow_templates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		is_system INTEGER DEFAULT 0,
		steps TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	if _, err := r.db.Exec(templatesSchema); err != nil {
		return fmt.Errorf("failed to create workflow_templates table: %w", err)
	}

	// Create workflow_steps table with new event-driven schema
	stepsSchema := `
	CREATE TABLE IF NOT EXISTS workflow_steps (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		name TEXT NOT NULL,
		position INTEGER NOT NULL,
		color TEXT,
		prompt TEXT,
		events TEXT,
		allow_manual_move INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow ON workflow_steps(workflow_id);
	`
	if _, err := r.db.Exec(stepsSchema); err != nil {
		return fmt.Errorf("failed to create workflow_steps table: %w", err)
	}

	// Migrate old schema to new schema if needed
	if err := r.migrateWorkflowSteps(); err != nil {
		return fmt.Errorf("failed to migrate workflow_steps: %w", err)
	}

	// Add is_start_step column if not present
	if err := r.addIsStartStepColumn(); err != nil {
		return fmt.Errorf("failed to add is_start_step column: %w", err)
	}

	// Create session_step_history table
	historySchema := `
	CREATE TABLE IF NOT EXISTS session_step_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		from_step_id TEXT,
		to_step_id TEXT NOT NULL,
		trigger TEXT NOT NULL,
		actor_id TEXT,
		metadata TEXT,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_session_step_history_session ON session_step_history(session_id);
	`
	if _, err := r.db.Exec(historySchema); err != nil {
		return fmt.Errorf("failed to create session_step_history table: %w", err)
	}

	// Seed system templates
	if err := r.seedSystemTemplates(); err != nil {
		return fmt.Errorf("failed to seed system templates: %w", err)
	}

	// Seed default workflow steps for workflows that don't have any
	if err := r.seedDefaultWorkflowSteps(); err != nil {
		return fmt.Errorf("failed to seed default workflow steps: %w", err)
	}

	return nil
}

// migrateWorkflowSteps detects old schema (has step_type column) and migrates data to new schema.
func (r *Repository) migrateWorkflowSteps() error {
	// Check if old column exists
	var hasStepType bool
	rows, err := r.db.Query("PRAGMA table_info(workflow_steps)")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == "step_type" {
			hasStepType = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !hasStepType {
		return nil // Already on new schema
	}

	// Migrate: read old data, recreate table with new schema, insert converted data
	type oldStep struct {
		ID               string
		WorkflowID       string
		Name             string
		Position         int
		Color            sql.NullString
		AutoStartAgent   int
		PlanMode         int
		RequireApproval  int
		PromptPrefix     sql.NullString
		PromptSuffix     sql.NullString
		OnCompleteStepID sql.NullString
		OnApprovalStepID sql.NullString
		AllowManualMove  int
		CreatedAt        time.Time
		UpdatedAt        time.Time
	}

	oldRows, err := r.db.Query(`
		SELECT id, workflow_id, name, position, color,
			auto_start_agent, plan_mode, require_approval,
			prompt_prefix, prompt_suffix, on_complete_step_id, on_approval_step_id,
			allow_manual_move, created_at, updated_at
		FROM workflow_steps
	`)
	if err != nil {
		return fmt.Errorf("failed to read old workflow_steps: %w", err)
	}
	defer func() { _ = oldRows.Close() }()

	var oldSteps []oldStep
	for oldRows.Next() {
		var s oldStep
		if err := oldRows.Scan(&s.ID, &s.WorkflowID, &s.Name, &s.Position, &s.Color,
			&s.AutoStartAgent, &s.PlanMode, &s.RequireApproval,
			&s.PromptPrefix, &s.PromptSuffix, &s.OnCompleteStepID, &s.OnApprovalStepID,
			&s.AllowManualMove, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return fmt.Errorf("failed to scan old workflow step: %w", err)
		}
		oldSteps = append(oldSteps, s)
	}
	if err := oldRows.Err(); err != nil {
		return err
	}

	// Drop old table and recreate within a transaction for atomicity
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DROP TABLE workflow_steps"); err != nil {
		return fmt.Errorf("failed to drop old workflow_steps table: %w", err)
	}

	newSchema := `
	CREATE TABLE workflow_steps (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		name TEXT NOT NULL,
		position INTEGER NOT NULL,
		color TEXT,
		prompt TEXT,
		events TEXT,
		allow_manual_move INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow ON workflow_steps(workflow_id);
	`
	if _, err := tx.Exec(newSchema); err != nil {
		return fmt.Errorf("failed to recreate workflow_steps table: %w", err)
	}

	// Convert and insert old data
	for _, s := range oldSteps {
		events := convertOldStepToEvents(s.AutoStartAgent == 1, s.PlanMode == 1, s.RequireApproval == 1,
			s.OnCompleteStepID, s.OnApprovalStepID)
		eventsJSON, err := json.Marshal(events)
		if err != nil {
			return fmt.Errorf("failed to marshal events for step %s: %w", s.ID, err)
		}

		// Combine old prompt prefix/suffix into single prompt
		prompt := ""
		if s.PromptPrefix.Valid && s.PromptPrefix.String != "" {
			prompt = s.PromptPrefix.String + "\n\n{{task_prompt}}"
			if s.PromptSuffix.Valid && s.PromptSuffix.String != "" {
				prompt += "\n\n" + s.PromptSuffix.String
			}
		} else if s.PromptSuffix.Valid && s.PromptSuffix.String != "" {
			prompt = "{{task_prompt}}\n\n" + s.PromptSuffix.String
		}

		color := ""
		if s.Color.Valid {
			color = s.Color.String
		}

		if _, err := tx.Exec(`
			INSERT INTO workflow_steps (id, workflow_id, name, position, color, prompt, events, allow_manual_move, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, s.ID, s.WorkflowID, s.Name, s.Position, color, prompt, string(eventsJSON), s.AllowManualMove, s.CreatedAt, s.UpdatedAt); err != nil {
			return fmt.Errorf("failed to insert migrated step %s: %w", s.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	return nil
}

// addIsStartStepColumn adds the is_start_step column to workflow_steps if it doesn't exist.
func (r *Repository) addIsStartStepColumn() error {
	rows, err := r.db.Query("PRAGMA table_info(workflow_steps)")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var hasColumn bool
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == "is_start_step" {
			hasColumn = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	_, err = r.db.Exec("ALTER TABLE workflow_steps ADD COLUMN is_start_step INTEGER DEFAULT 0")
	return err
}

// convertOldStepToEvents converts old boolean/pointer fields to new event-driven model.
func convertOldStepToEvents(autoStart, planMode, requireApproval bool, onCompleteStepID, onApprovalStepID sql.NullString) models.StepEvents {
	events := models.StepEvents{}

	// On Enter actions
	if planMode {
		events.OnEnter = append(events.OnEnter, models.OnEnterAction{Type: models.OnEnterEnablePlanMode})
	}
	if autoStart {
		events.OnEnter = append(events.OnEnter, models.OnEnterAction{Type: models.OnEnterAutoStartAgent})
	}

	// On Turn Complete actions
	if planMode {
		events.OnTurnComplete = append(events.OnTurnComplete, models.OnTurnCompleteAction{Type: models.OnTurnCompleteDisablePlanMode})
	}

	if !requireApproval {
		// If not requiring approval and has on_complete, add move transition
		if onCompleteStepID.Valid && onCompleteStepID.String != "" {
			events.OnTurnComplete = append(events.OnTurnComplete, models.OnTurnCompleteAction{
				Type:   models.OnTurnCompleteMoveToStep,
				Config: map[string]interface{}{"step_id": onCompleteStepID.String},
			})
		}
	} else if onApprovalStepID.Valid && onApprovalStepID.String != "" {
		// Require approval before transitioning to the approval step
		events.OnTurnComplete = append(events.OnTurnComplete, models.OnTurnCompleteAction{
			Type: models.OnTurnCompleteMoveToStep,
			Config: map[string]interface{}{
				"step_id":           onApprovalStepID.String,
				"requires_approval": true,
			},
		})
	}

	return events
}

// seedDefaultWorkflowSteps creates default workflow steps for workflows that don't have any.
// Uses the simple template as the default workflow.
func (r *Repository) seedDefaultWorkflowSteps() error {
	// Find workflows without workflow steps
	rows, err := r.db.Query(`
		SELECT w.id FROM workflows w
		LEFT JOIN workflow_steps ws ON ws.workflow_id = w.id
		WHERE ws.id IS NULL
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var workflowIDs []string
	for rows.Next() {
		var workflowID string
		if err := rows.Scan(&workflowID); err != nil {
			return err
		}
		workflowIDs = append(workflowIDs, workflowID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Use the simple template for default workflow steps
	now := time.Now()
	simpleTemplate := r.getKanbanTemplate(now)

	for _, workflowID := range workflowIDs {
		// Build mapping from template step ID to new UUID
		idMap := make(map[string]string, len(simpleTemplate.Steps))
		for _, stepDef := range simpleTemplate.Steps {
			idMap[stepDef.ID] = uuid.New().String()
		}

		for _, stepDef := range simpleTemplate.Steps {
			events := models.RemapStepEvents(stepDef.Events, idMap)
			eventsJSON, err := json.Marshal(events)
			if err != nil {
				return fmt.Errorf("failed to marshal events: %w", err)
			}

			if _, err := r.db.Exec(`
				INSERT INTO workflow_steps (
					id, workflow_id, name, position, color,
					prompt, events, allow_manual_move, is_start_step, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				idMap[stepDef.ID], workflowID, stepDef.Name, stepDef.Position, stepDef.Color,
				stepDef.Prompt, string(eventsJSON), sqlite.BoolToInt(stepDef.AllowManualMove),
				sqlite.BoolToInt(stepDef.IsStartStep), now, now,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// seedSystemTemplates inserts the default system workflow templates.
func (r *Repository) seedSystemTemplates() error {
	templates := r.getSystemTemplates()

	for _, tmpl := range templates {
		// Always upsert system templates to keep them current
		stepsJSON, err := json.Marshal(tmpl.Steps)
		if err != nil {
			return fmt.Errorf("failed to marshal steps for template %s: %w", tmpl.ID, err)
		}

		_, err = r.db.Exec(`
			INSERT INTO workflow_templates (id, name, description, is_system, steps, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				steps = excluded.steps,
				updated_at = excluded.updated_at
		`, tmpl.ID, tmpl.Name, tmpl.Description, sqlite.BoolToInt(tmpl.IsSystem), string(stepsJSON), tmpl.CreatedAt, tmpl.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to upsert template %s: %w", tmpl.ID, err)
		}
	}

	return nil
}

// getSystemTemplates returns the predefined system workflow templates.
func (r *Repository) getSystemTemplates() []*models.WorkflowTemplate {
	now := time.Now().UTC()
	return []*models.WorkflowTemplate{
		r.getKanbanTemplate(now),
		r.getStandardTemplate(now),
		r.getArchitectureTemplate(now),
		r.getPRReviewTemplate(now),
	}
}

// getStandardTemplate returns the standard 4-step workflow template with planning phase.
// Steps: Todo → Plan → Implementation → Done
func (r *Repository) getStandardTemplate(now time.Time) *models.WorkflowTemplate {
	return &models.WorkflowTemplate{
		ID:          "standard",
		Name:        "Plan & Build",
		Description: "Two-phase workflow: the agent first creates a plan for your review, then implements it. Ideal for tasks that benefit from upfront design.",
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []models.StepDefinition{
			{
				ID:              "todo",
				Name:            "Todo",
				Position:        0,
				Color:           "bg-neutral-400",
				Events:          models.StepEvents{},
				AllowManualMove: true,
			},
			{
				ID:          "plan",
				Name:        "Plan",
				Position:    1,
				Color:       "bg-purple-500",
				IsStartStep: true,
				Prompt:      "[PLANNING PHASE]\nAnalyze this task and create a detailed implementation plan.\nDo NOT make any code changes yet - only analyze and plan.\n\nBefore creating the plan, ask the user clarifying questions if anything is unclear or ambiguous about the requirements. Use the ask_user_question_kandev tool to get answers before proceeding.\n\nCreate a plan that includes:\n1. Understanding of the requirements\n2. Files that need to be modified or created\n3. Step-by-step implementation approach\n4. Potential risks or considerations\n\nWhen including diagrams in your plan (architecture, sequence, flowcharts, etc.), always use mermaid syntax in code blocks.\n\nIMPORTANT: Save your plan using the create_task_plan_kandev MCP tool with the task_id provided in the session context.\nAfter saving the plan, STOP and wait for user review. The user will review your plan in the UI and may edit it.\nDo not create any other files during this phase - only use the MCP tool to save the plan.\n\n{{task_prompt}}",
				Events: models.StepEvents{
					OnEnter: []models.OnEnterAction{
						{Type: models.OnEnterEnablePlanMode},
						{Type: models.OnEnterAutoStartAgent},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:       "implementation",
				Name:     "Implementation",
				Position: 2,
				Color:    "bg-blue-500",
				Prompt:   "[IMPLEMENTATION PHASE]\nBefore starting implementation, retrieve the task plan using get_task_plan_kandev with the task_id from the session context.\nReview the plan carefully, including any edits the user may have made.\nAcknowledge the plan and any user modifications before proceeding.\n\nThen implement the task following the plan step-by-step.\nYou can update the plan during implementation using update_task_plan_kandev if needed.\n\n{{task_prompt}}",
				Events: models.StepEvents{
					OnEnter: []models.OnEnterAction{
						{Type: models.OnEnterAutoStartAgent},
					},
					OnTurnComplete: []models.OnTurnCompleteAction{
						{Type: models.OnTurnCompleteMoveToNext},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:              "done",
				Name:            "Done",
				Position:        3,
				Color:           "bg-green-500",
				Events:          models.StepEvents{},
				AllowManualMove: true,
			},
		},
	}
}

// getArchitectureTemplate returns the architecture workflow template.
func (r *Repository) getArchitectureTemplate(now time.Time) *models.WorkflowTemplate {
	return &models.WorkflowTemplate{
		ID:          "architecture",
		Name:        "Architecture",
		Description: "Focus on architecture and design. The agent creates technical designs for your review before any implementation begins.",
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []models.StepDefinition{
			{
				ID:              "backlog",
				Name:            "Ideas",
				Position:        0,
				Color:           "bg-neutral-400",
				Events:          models.StepEvents{},
				AllowManualMove: true,
			},
			{
				ID:          "planning",
				Name:        "Planning",
				Position:    1,
				Color:       "bg-purple-500",
				IsStartStep: true,
				Prompt:      "[ARCHITECTURE PHASE]\nAnalyze and design the architecture for this task.\n\nBefore creating the design, ask the user clarifying questions if anything is unclear or ambiguous about the requirements. Use the ask_user_question_kandev tool to get answers before proceeding.\n\nWhen including diagrams in your design (architecture, sequence, flowcharts, etc.), always use mermaid syntax in code blocks.\n\nIMPORTANT: Save your design using the create_task_plan_kandev MCP tool with the task_id provided in the session context.\nAfter saving the plan, STOP and wait for user review. The user will review your design in the UI and may edit it.\nDo not create any other files during this phase - only use the MCP tool to save the plan.\n\n{{task_prompt}}",
				Events: models.StepEvents{
					OnEnter: []models.OnEnterAction{
						{Type: models.OnEnterEnablePlanMode},
						{Type: models.OnEnterAutoStartAgent},
					},
					OnTurnComplete: []models.OnTurnCompleteAction{
						{Type: models.OnTurnCompleteDisablePlanMode},
						{Type: models.OnTurnCompleteMoveToNext},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:       "review",
				Name:     "Review",
				Position: 2,
				Color:    "bg-yellow-500",
				Events: models.StepEvents{
					OnEnter: []models.OnEnterAction{
						{Type: models.OnEnterEnablePlanMode},
					},
					OnTurnStart: []models.OnTurnStartAction{
						{Type: models.OnTurnStartMoveToPrevious},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:              "done",
				Name:            "Approved",
				Position:        3,
				Color:           "bg-green-500",
				Events:          models.StepEvents{},
				AllowManualMove: true,
			},
		},
	}
}

// getKanbanTemplate returns the kanban workflow template.
func (r *Repository) getKanbanTemplate(now time.Time) *models.WorkflowTemplate {
	return &models.WorkflowTemplate{
		ID:          "simple",
		Name:        "Kanban",
		Description: "Classic board with automated agent work. Tasks start in In Progress where the agent runs automatically, then move to Review when done.",
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []models.StepDefinition{
			{
				ID:       "backlog",
				Name:     "Backlog",
				Position: 0,
				Color:    "bg-neutral-400",
				Events: models.StepEvents{
					OnTurnStart: []models.OnTurnStartAction{
						{Type: models.OnTurnStartMoveToNext},
					},
					OnTurnComplete: []models.OnTurnCompleteAction{
						{Type: models.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "review"}},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:       "in-progress",
				Name:     "In Progress",
				Position: 1,
				Color:    "bg-blue-500",
				Events: models.StepEvents{
					OnEnter: []models.OnEnterAction{
						{Type: models.OnEnterAutoStartAgent},
					},
					OnTurnComplete: []models.OnTurnCompleteAction{
						{Type: models.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "review"}},
					},
				},
				AllowManualMove: true,
				IsStartStep:     true,
			},
			{
				ID:       "review",
				Name:     "Review",
				Position: 2,
				Color:    "bg-yellow-500",
				Events: models.StepEvents{
					OnTurnStart: []models.OnTurnStartAction{
						{Type: models.OnTurnStartMoveToPrevious},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:       "done",
				Name:     "Done",
				Position: 3,
				Color:    "bg-green-500",
				Events: models.StepEvents{
					OnTurnStart: []models.OnTurnStartAction{
						{Type: models.OnTurnStartMoveToStep, Config: map[string]interface{}{"step_id": "in-progress"}},
					},
				},
				AllowManualMove: true,
			},
		},
	}
}


// getPRReviewTemplate returns the PR review workflow template.
func (r *Repository) getPRReviewTemplate(now time.Time) *models.WorkflowTemplate {
	return &models.WorkflowTemplate{
		ID:          "pr-review",
		Name:        "PR Review",
		Description: "Track pull requests through review. PRs wait in queue, get reviewed by the agent, then marked as done.",
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []models.StepDefinition{
			{
				ID:          "waiting",
				Name:        "Waiting",
				Position:    0,
				Color:       "bg-neutral-400",
				IsStartStep: true,
				Events: models.StepEvents{
					OnTurnStart: []models.OnTurnStartAction{
						{Type: models.OnTurnStartMoveToNext},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:       "review",
				Name:     "Review",
				Position: 1,
				Color:    "bg-yellow-500",
				Prompt:   "Please review the changed files in the current git worktree.\n\nSTEP 1: Determine what to review\n- First, check if there are any uncommitted changes (dirty working directory)\n- If there are uncommitted/staged changes: review those files\n- If the working directory is clean: review the commits that have diverged from the master/main branch\n\nSTEP 2: Review the changes, then output your findings in EXACTLY 4 sections: BUG, IMPROVEMENT, NITPICK, PERFORMANCE.\n\nRules:\n- Each section is OPTIONAL — only include it if you have findings for that category\n- If a section has no findings, omit it entirely\n- Format each finding as: filename:line_number - Description\n- Be specific and reference exact line numbers\n- Keep descriptions concise but actionable\n\n{{task_prompt}}",
				Events: models.StepEvents{
					OnEnter: []models.OnEnterAction{
						{Type: models.OnEnterAutoStartAgent},
					},
					OnTurnComplete: []models.OnTurnCompleteAction{
						{Type: models.OnTurnCompleteMoveToNext},
					},
				},
				AllowManualMove: true,
			},
			{
				ID:       "done",
				Name:     "Done",
				Position: 2,
				Color:    "bg-green-500",
				Events: models.StepEvents{
					OnTurnStart: []models.OnTurnStartAction{
						{Type: models.OnTurnStartMoveToPrevious},
					},
				},
				AllowManualMove: true,
			},
		},
	}
}

// ============================================================================
// WorkflowTemplate CRUD Operations
// ============================================================================

// CreateTemplate creates a new workflow template.
func (r *Repository) CreateTemplate(ctx context.Context, template *models.WorkflowTemplate) error {
	if template.ID == "" {
		template.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	template.CreatedAt = now
	template.UpdatedAt = now

	stepsJSON, err := json.Marshal(template.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal steps: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO workflow_templates (id, name, description, is_system, steps, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, template.ID, template.Name, template.Description, sqlite.BoolToInt(template.IsSystem), string(stepsJSON), template.CreatedAt, template.UpdatedAt)

	return err
}

// GetTemplate retrieves a workflow template by ID.
func (r *Repository) GetTemplate(ctx context.Context, id string) (*models.WorkflowTemplate, error) {
	template := &models.WorkflowTemplate{}
	var stepsJSON string
	var isSystem int

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, is_system, steps, created_at, updated_at
		FROM workflow_templates WHERE id = ?
	`, id).Scan(&template.ID, &template.Name, &template.Description, &isSystem, &stepsJSON, &template.CreatedAt, &template.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow template not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	template.IsSystem = isSystem == 1
	if err := json.Unmarshal([]byte(stepsJSON), &template.Steps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
	}

	return template, nil
}

// UpdateTemplate updates an existing workflow template.
func (r *Repository) UpdateTemplate(ctx context.Context, template *models.WorkflowTemplate) error {
	template.UpdatedAt = time.Now().UTC()

	stepsJSON, err := json.Marshal(template.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal steps: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE workflow_templates SET name = ?, description = ?, steps = ?, updated_at = ?
		WHERE id = ? AND is_system = 0
	`, template.Name, template.Description, string(stepsJSON), template.UpdatedAt, template.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow template not found or is a system template: %s", template.ID)
	}
	return nil
}

// DeleteTemplate deletes a workflow template by ID.
func (r *Repository) DeleteTemplate(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM workflow_templates WHERE id = ? AND is_system = 0`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow template not found or is a system template: %s", id)
	}
	return nil
}

// ListTemplates returns all workflow templates.
func (r *Repository) ListTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, is_system, steps, created_at, updated_at
		FROM workflow_templates
		ORDER BY is_system DESC,
		CASE
			WHEN id = 'simple' THEN 1
			WHEN id = 'standard' THEN 2
			WHEN id = 'architecture' THEN 3
			ELSE 999
		END,
		name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.WorkflowTemplate
	for rows.Next() {
		template := &models.WorkflowTemplate{}
		var stepsJSON string
		var isSystem int

		err := rows.Scan(&template.ID, &template.Name, &template.Description, &isSystem, &stepsJSON, &template.CreatedAt, &template.UpdatedAt)
		if err != nil {
			return nil, err
		}

		template.IsSystem = isSystem == 1
		if err := json.Unmarshal([]byte(stepsJSON), &template.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal steps for template %s: %w", template.ID, err)
		}

		result = append(result, template)
	}
	return result, rows.Err()
}

// GetSystemTemplates returns only system workflow templates.
func (r *Repository) GetSystemTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, is_system, steps, created_at, updated_at
		FROM workflow_templates WHERE is_system = 1 ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.WorkflowTemplate
	for rows.Next() {
		template := &models.WorkflowTemplate{}
		var stepsJSON string
		var isSystem int

		err := rows.Scan(&template.ID, &template.Name, &template.Description, &isSystem, &stepsJSON, &template.CreatedAt, &template.UpdatedAt)
		if err != nil {
			return nil, err
		}

		template.IsSystem = isSystem == 1
		if err := json.Unmarshal([]byte(stepsJSON), &template.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal steps for template %s: %w", template.ID, err)
		}

		result = append(result, template)
	}
	return result, rows.Err()
}

// ============================================================================
// WorkflowStep CRUD Operations
// ============================================================================

// CreateStep creates a new workflow step.
func (r *Repository) CreateStep(ctx context.Context, step *models.WorkflowStep) error {
	if step.ID == "" {
		step.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	step.CreatedAt = now
	step.UpdatedAt = now

	eventsJSON, err := json.Marshal(step.Events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, color,
			prompt, events, allow_manual_move, is_start_step, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, step.ID, step.WorkflowID, step.Name, step.Position, step.Color,
		step.Prompt, string(eventsJSON), sqlite.BoolToInt(step.AllowManualMove),
		sqlite.BoolToInt(step.IsStartStep), step.CreatedAt, step.UpdatedAt)

	return err
}

// scanStep scans a single workflow step row including JSON events parsing.
func (r *Repository) scanStep(row interface {
	Scan(dest ...interface{}) error
}) (*models.WorkflowStep, error) {
	step := &models.WorkflowStep{}
	var allowManualMove, isStartStep int
	var color, prompt, eventsJSON sql.NullString

	err := row.Scan(&step.ID, &step.WorkflowID, &step.Name, &step.Position, &color,
		&prompt, &eventsJSON, &allowManualMove, &isStartStep, &step.CreatedAt, &step.UpdatedAt)

	if err != nil {
		return nil, err
	}

	step.AllowManualMove = allowManualMove == 1
	step.IsStartStep = isStartStep == 1
	if color.Valid {
		step.Color = color.String
	}
	if prompt.Valid {
		step.Prompt = prompt.String
	}
	if eventsJSON.Valid && eventsJSON.String != "" {
		if err := json.Unmarshal([]byte(eventsJSON.String), &step.Events); err != nil {
			return nil, fmt.Errorf("failed to unmarshal events: %w", err)
		}
	}

	return step, nil
}

const stepSelectColumns = `id, workflow_id, name, position, color, prompt, events, allow_manual_move, is_start_step, created_at, updated_at`

// GetStep retrieves a workflow step by ID.
func (r *Repository) GetStep(ctx context.Context, id string) (*models.WorkflowStep, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE id = ?
	`, id)

	step, err := r.scanStep(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow step not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return step, nil
}

// UpdateStep updates an existing workflow step.
func (r *Repository) UpdateStep(ctx context.Context, step *models.WorkflowStep) error {
	step.UpdatedAt = time.Now().UTC()

	eventsJSON, err := json.Marshal(step.Events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE workflow_steps SET
			name = ?, position = ?, color = ?,
			prompt = ?, events = ?,
			allow_manual_move = ?, is_start_step = ?, updated_at = ?
		WHERE id = ?
	`, step.Name, step.Position, step.Color,
		step.Prompt, string(eventsJSON),
		sqlite.BoolToInt(step.AllowManualMove), sqlite.BoolToInt(step.IsStartStep), step.UpdatedAt, step.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow step not found: %s", step.ID)
	}
	return nil
}

// ClearStartStepFlag clears the is_start_step flag for all steps in a workflow except the given step.
func (r *Repository) ClearStartStepFlag(ctx context.Context, workflowID, exceptStepID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE workflow_steps SET is_start_step = 0
		WHERE workflow_id = ? AND id != ?
	`, workflowID, exceptStepID)
	return err
}

// GetStartStep returns the step marked as is_start_step for a workflow.
// Returns nil if no step is marked.
func (r *Repository) GetStartStep(ctx context.Context, workflowID string) (*models.WorkflowStep, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1
		LIMIT 1
	`, workflowID)

	step, err := r.scanStep(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return step, nil
}

// ClearStepReferences clears any move_to_step event config references
// to the given step ID within the specified workflow.
func (r *Repository) ClearStepReferences(ctx context.Context, workflowID, stepID string) error {
	// Load all steps in the workflow and update any that reference the deleted step
	steps, err := r.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}

	for _, step := range steps {
		modified := false
		for i, action := range step.Events.OnTurnComplete {
			if action.Type == models.OnTurnCompleteMoveToStep && action.Config != nil {
				if refID, ok := action.Config["step_id"].(string); ok && refID == stepID {
					// Remove this action
					step.Events.OnTurnComplete = append(step.Events.OnTurnComplete[:i], step.Events.OnTurnComplete[i+1:]...)
					modified = true
					break
				}
			}
		}
		if modified {
			if err := r.UpdateStep(ctx, step); err != nil {
				return fmt.Errorf("failed to clear step reference from step %s: %w", step.ID, err)
			}
		}
	}

	return nil
}

// DeleteStep deletes a workflow step by ID.
func (r *Repository) DeleteStep(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM workflow_steps WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow step not found: %s", id)
	}
	return nil
}

// ListStepsByWorkflow returns all workflow steps for a workflow.
func (r *Repository) ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*models.WorkflowStep, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE workflow_id = ? ORDER BY position
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanSteps(rows)
}

// scanSteps is a helper to scan multiple workflow step rows.
func (r *Repository) scanSteps(rows *sql.Rows) ([]*models.WorkflowStep, error) {
	var result []*models.WorkflowStep
	for rows.Next() {
		step, err := r.scanStep(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, step)
	}
	return result, rows.Err()
}

// ============================================================================
// SessionStepHistory Operations
// ============================================================================

// CreateHistory creates a new session step history entry.
func (r *Repository) CreateHistory(ctx context.Context, history *models.SessionStepHistory) error {
	now := time.Now().UTC()
	history.CreatedAt = now

	var metadataJSON *string
	if history.Metadata != nil {
		data, err := json.Marshal(history.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		s := string(data)
		metadataJSON = &s
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO session_step_history (session_id, from_step_id, to_step_id, trigger, actor_id, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, history.SessionID, history.FromStepID, history.ToStepID, history.Trigger, history.ActorID, metadataJSON, history.CreatedAt)
	if err != nil {
		return err
	}

	// Get the auto-incremented ID
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	history.ID = id

	return nil
}

// ListHistoryBySession returns all step history entries for a session.
func (r *Repository) ListHistoryBySession(ctx context.Context, sessionID string) ([]*models.SessionStepHistory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, from_step_id, to_step_id, trigger, actor_id, metadata, created_at
		FROM session_step_history WHERE session_id = ? ORDER BY created_at
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.SessionStepHistory
	for rows.Next() {
		history := &models.SessionStepHistory{}
		var fromStepID, actorID, metadataJSON sql.NullString

		err := rows.Scan(&history.ID, &history.SessionID, &fromStepID, &history.ToStepID, &history.Trigger, &actorID, &metadataJSON, &history.CreatedAt)
		if err != nil {
			return nil, err
		}

		if fromStepID.Valid {
			history.FromStepID = &fromStepID.String
		}
		if actorID.Valid {
			history.ActorID = &actorID.String
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &history.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		result = append(result, history)
	}
	return result, rows.Err()
}
