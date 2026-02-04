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

	// Create workflow_steps table
	stepsSchema := `
	CREATE TABLE IF NOT EXISTS workflow_steps (
		id TEXT PRIMARY KEY,
		board_id TEXT NOT NULL,
		name TEXT NOT NULL,
		step_type TEXT NOT NULL,
		position INTEGER NOT NULL,
		color TEXT,
		auto_start_agent INTEGER DEFAULT 0,
		plan_mode INTEGER DEFAULT 0,
		require_approval INTEGER DEFAULT 0,
		prompt_prefix TEXT,
		prompt_suffix TEXT,
		on_complete_step_id TEXT,
		on_approval_step_id TEXT,
		allow_manual_move INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_steps_board ON workflow_steps(board_id);
	`
	if _, err := r.db.Exec(stepsSchema); err != nil {
		return fmt.Errorf("failed to create workflow_steps table: %w", err)
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

	// Seed default workflow steps for boards that don't have any
	if err := r.seedDefaultWorkflowSteps(); err != nil {
		return fmt.Errorf("failed to seed default workflow steps: %w", err)
	}

	return nil
}

// seedDefaultWorkflowSteps creates default workflow steps for boards that don't have any.
// Uses the simple template as the default workflow.
func (r *Repository) seedDefaultWorkflowSteps() error {
	// Find boards without workflow steps
	rows, err := r.db.Query(`
		SELECT b.id FROM boards b
		LEFT JOIN workflow_steps ws ON ws.board_id = b.id
		WHERE ws.id IS NULL
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var boardIDs []string
	for rows.Next() {
		var boardID string
		if err := rows.Scan(&boardID); err != nil {
			return err
		}
		boardIDs = append(boardIDs, boardID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Use the simple template for default workflow steps
	now := time.Now()
	simpleTemplate := r.getSimpleTemplate(now)

	for _, boardID := range boardIDs {
		// Map template step IDs to generated UUIDs for linking
		idMap := make(map[string]string)
		for _, stepDef := range simpleTemplate.Steps {
			idMap[stepDef.ID] = uuid.New().String()
		}

		for _, stepDef := range simpleTemplate.Steps {
			stepID := idMap[stepDef.ID]

			// Map OnCompleteStepID if set
			var onCompleteStepID *string
			if stepDef.OnCompleteStepID != "" {
				if mappedID, ok := idMap[stepDef.OnCompleteStepID]; ok {
					onCompleteStepID = &mappedID
				}
			}

			if _, err := r.db.Exec(`
				INSERT INTO workflow_steps (
					id, board_id, name, step_type, position, color,
					auto_start_agent, plan_mode, require_approval,
					prompt_prefix, prompt_suffix, on_complete_step_id,
					allow_manual_move, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				stepID, boardID, stepDef.Name, string(stepDef.StepType), stepDef.Position, stepDef.Color,
				stepDef.AutoStartAgent, stepDef.PlanMode, stepDef.RequireApproval,
				stepDef.PromptPrefix, stepDef.PromptSuffix, onCompleteStepID,
				stepDef.AllowManualMove, now, now,
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
		// Check if template already exists
		var exists bool
		err := r.db.QueryRow("SELECT 1 FROM workflow_templates WHERE id = ?", tmpl.ID).Scan(&exists)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if exists {
			continue
		}

		stepsJSON, err := json.Marshal(tmpl.Steps)
		if err != nil {
			return fmt.Errorf("failed to marshal steps for template %s: %w", tmpl.ID, err)
		}

		_, err = r.db.Exec(`
			INSERT INTO workflow_templates (id, name, description, is_system, steps, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, tmpl.ID, tmpl.Name, tmpl.Description, sqlite.BoolToInt(tmpl.IsSystem), string(stepsJSON), tmpl.CreatedAt, tmpl.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to insert template %s: %w", tmpl.ID, err)
		}
	}

	return nil
}

// getSystemTemplates returns the predefined system workflow templates.
func (r *Repository) getSystemTemplates() []*models.WorkflowTemplate {
	now := time.Now().UTC()
	return []*models.WorkflowTemplate{
		r.getSimpleTemplate(now),
		r.getStandardTemplate(now),
		r.getArchitectureTemplate(now),
	}
}

// getStandardTemplate returns the standard 4-step workflow template with planning phase.
// Steps: Todo → Plan → Implementation → Done
func (r *Repository) getStandardTemplate(now time.Time) *models.WorkflowTemplate {
	return &models.WorkflowTemplate{
		ID:          "standard",
		Name:        "Plan & Build",
		Description: "Plan first, then build: Todo → Plan → Implementation → Done",
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []models.StepDefinition{
			{
				ID:              "todo",
				Name:            "Todo",
				StepType:        models.StepTypeBacklog,
				Position:        0,
				Color:           "bg-neutral-400",
				TaskState:       "TODO",
				AutoStartAgent:  false,
				AllowManualMove: true,
			},
			{
				ID:               "plan",
				Name:             "Plan",
				StepType:         models.StepTypePlanning,
				Position:         1,
				Color:            "bg-purple-500",
				TaskState:        "IN_PROGRESS",
				AutoStartAgent:   true,
				PlanMode:         true,
				RequireApproval:  true,
				PromptPrefix:     "[PLANNING PHASE]\nAnalyze this task and create a detailed implementation plan.\nDo NOT make any code changes yet - only analyze and plan.\n\nBefore creating the plan, ask the user clarifying questions if anything is unclear or ambiguous about the requirements. Use the ask_user_question_kandev tool to get answers before proceeding.\n\nCreate a plan that includes:\n1. Understanding of the requirements\n2. Files that need to be modified or created\n3. Step-by-step implementation approach\n4. Potential risks or considerations\n\nWhen including diagrams in your plan (architecture, sequence, flowcharts, etc.), always use mermaid syntax in code blocks.\n\nIMPORTANT: Save your plan using the create_task_plan_kandev MCP tool with the task_id provided in the session context.\nAfter saving the plan, STOP and wait for user review. The user will review your plan in the UI and may edit it.\nDo not create any other files during this phase - only use the MCP tool to save the plan.",
				OnCompleteStepID: "implementation",
				AllowManualMove:  true,
			},
			{
				ID:               "implementation",
				Name:             "Implementation",
				StepType:         models.StepTypeImplementation,
				Position:         2,
				Color:            "bg-blue-500",
				TaskState:        "IN_PROGRESS",
				AutoStartAgent:   true,
				RequireApproval:  true,
				PromptPrefix:     "[IMPLEMENTATION PHASE]\nBefore starting implementation, retrieve the task plan using get_task_plan_kandev with the task_id from the session context.\nReview the plan carefully, including any edits the user may have made.\nAcknowledge the plan and any user modifications before proceeding.\n\nThen implement the task following the plan step-by-step.\nYou can update the plan during implementation using update_task_plan_kandev if needed.",
				OnCompleteStepID: "done",
				AllowManualMove:  true,
			},
			{
				ID:              "done",
				Name:            "Done",
				StepType:        models.StepTypeDone,
				Position:        3,
				Color:           "bg-green-500",
				TaskState:       "COMPLETED",
				AutoStartAgent:  false,
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
		Description: "Design and review: Ideas → Planning → Review → Approved",
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []models.StepDefinition{
			{
				ID:              "backlog",
				Name:            "Ideas",
				StepType:        models.StepTypeBacklog,
				Position:        0,
				Color:           "bg-neutral-400",
				TaskState:       "TODO",
				AutoStartAgent:  false,
				AllowManualMove: true,
			},
			{
				ID:               "planning",
				Name:             "Planning",
				StepType:         models.StepTypePlanning,
				Position:         1,
				Color:            "bg-purple-500",
				TaskState:        "IN_PROGRESS",
				AutoStartAgent:   true,
				PlanMode:         true,
				PromptPrefix:     "[ARCHITECTURE PHASE]\nAnalyze and design the architecture for this task.\n\nBefore creating the design, ask the user clarifying questions if anything is unclear or ambiguous about the requirements. Use the ask_user_question_kandev tool to get answers before proceeding.\n\nWhen including diagrams in your design (architecture, sequence, flowcharts, etc.), always use mermaid syntax in code blocks.\n\nIMPORTANT: Save your design using the create_task_plan_kandev MCP tool with the task_id provided in the session context.\nAfter saving the plan, STOP and wait for user review. The user will review your design in the UI and may edit it.\nDo not create any other files during this phase - only use the MCP tool to save the plan.",
				OnCompleteStepID: "review",
				AllowManualMove:  true,
			},
			{
				ID:               "review",
				Name:             "Review",
				StepType:         models.StepTypeReview,
				Position:         2,
				Color:            "bg-yellow-500",
				TaskState:        "REVIEW",
				AutoStartAgent:   false,
				RequireApproval:  true,
				OnApprovalStepID: "done",
				AllowManualMove:  true,
			},
			{
				ID:              "done",
				Name:            "Approved",
				StepType:        models.StepTypeDone,
				Position:        3,
				Color:           "bg-green-500",
				TaskState:       "COMPLETED",
				AutoStartAgent:  false,
				AllowManualMove: true,
			},
		},
	}
}

// getSimpleTemplate returns the simple workflow template.
func (r *Repository) getSimpleTemplate(now time.Time) *models.WorkflowTemplate {
	return &models.WorkflowTemplate{
		ID:          "simple",
		Name:        "Simple",
		Description: "Quick start: Backlog → In Progress → Review → Done",
		IsSystem:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps: []models.StepDefinition{
			{
				ID:              "backlog",
				Name:            "Backlog",
				StepType:        models.StepTypeBacklog,
				Position:        0,
				Color:           "bg-neutral-400",
				TaskState:       "TODO",
				AutoStartAgent:  false,
				AllowManualMove: true,
			},
			{
				ID:               "in-progress",
				Name:             "In Progress",
				StepType:         models.StepTypeImplementation,
				Position:         1,
				Color:            "bg-blue-500",
				TaskState:        "IN_PROGRESS",
				AutoStartAgent:   true,
				OnCompleteStepID: "review",
				AllowManualMove:  true,
			},
			{
				ID:              "review",
				Name:             "Review",
				StepType:         models.StepTypeReview,
				Position:         2,
				Color:            "bg-yellow-500",
				TaskState:        "REVIEW",
				AutoStartAgent:  false,
				RequireApproval: false,
				AllowManualMove:  true,
			},
			{
				ID:              "done",
				Name:            "Done",
				StepType:        models.StepTypeDone,
				Position:        3,
				Color:           "bg-green-500",
				TaskState:       "COMPLETED",
				AutoStartAgent:  false,
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

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO workflow_steps (
			id, board_id, name, step_type, position, color,
			auto_start_agent, plan_mode, require_approval,
			prompt_prefix, prompt_suffix, on_complete_step_id, on_approval_step_id,
			allow_manual_move, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, step.ID, step.BoardID, step.Name, step.StepType, step.Position, step.Color,
		sqlite.BoolToInt(step.AutoStartAgent), sqlite.BoolToInt(step.PlanMode), sqlite.BoolToInt(step.RequireApproval),
		step.PromptPrefix, step.PromptSuffix, step.OnCompleteStepID, step.OnApprovalStepID,
		sqlite.BoolToInt(step.AllowManualMove), step.CreatedAt, step.UpdatedAt)

	return err
}

// GetStep retrieves a workflow step by ID.
func (r *Repository) GetStep(ctx context.Context, id string) (*models.WorkflowStep, error) {
	step := &models.WorkflowStep{}
	var autoStartAgent, planMode, requireApproval, allowManualMove int
	var onCompleteStepID, onApprovalStepID, promptPrefix, promptSuffix, color sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, board_id, name, step_type, position, color,
			auto_start_agent, plan_mode, require_approval,
			prompt_prefix, prompt_suffix, on_complete_step_id, on_approval_step_id,
			allow_manual_move, created_at, updated_at
		FROM workflow_steps WHERE id = ?
	`, id).Scan(&step.ID, &step.BoardID, &step.Name, &step.StepType, &step.Position, &color,
		&autoStartAgent, &planMode, &requireApproval,
		&promptPrefix, &promptSuffix, &onCompleteStepID, &onApprovalStepID,
		&allowManualMove, &step.CreatedAt, &step.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow step not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	step.AutoStartAgent = autoStartAgent == 1
	step.PlanMode = planMode == 1
	step.RequireApproval = requireApproval == 1
	step.AllowManualMove = allowManualMove == 1
	if color.Valid {
		step.Color = color.String
	}
	if promptPrefix.Valid {
		step.PromptPrefix = promptPrefix.String
	}
	if promptSuffix.Valid {
		step.PromptSuffix = promptSuffix.String
	}
	if onCompleteStepID.Valid {
		step.OnCompleteStepID = &onCompleteStepID.String
	}
	if onApprovalStepID.Valid {
		step.OnApprovalStepID = &onApprovalStepID.String
	}

	return step, nil
}

// UpdateStep updates an existing workflow step.
func (r *Repository) UpdateStep(ctx context.Context, step *models.WorkflowStep) error {
	step.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE workflow_steps SET
			name = ?, step_type = ?, position = ?, color = ?,
			auto_start_agent = ?, plan_mode = ?, require_approval = ?,
			prompt_prefix = ?, prompt_suffix = ?, on_complete_step_id = ?, on_approval_step_id = ?,
			allow_manual_move = ?, updated_at = ?
		WHERE id = ?
	`, step.Name, step.StepType, step.Position, step.Color,
		sqlite.BoolToInt(step.AutoStartAgent), sqlite.BoolToInt(step.PlanMode), sqlite.BoolToInt(step.RequireApproval),
		step.PromptPrefix, step.PromptSuffix, step.OnCompleteStepID, step.OnApprovalStepID,
		sqlite.BoolToInt(step.AllowManualMove), step.UpdatedAt, step.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow step not found: %s", step.ID)
	}
	return nil
}

// ClearStepReferences clears any OnCompleteStepID or OnApprovalStepID references
// to the given step ID within the specified board. This should be called before
// deleting a step to prevent dangling references.
func (r *Repository) ClearStepReferences(ctx context.Context, boardID, stepID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE workflow_steps
		SET on_complete_step_id = NULL, updated_at = ?
		WHERE board_id = ? AND on_complete_step_id = ?
	`, time.Now().UTC(), boardID, stepID)
	if err != nil {
		return fmt.Errorf("failed to clear on_complete_step_id references: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE workflow_steps
		SET on_approval_step_id = NULL, updated_at = ?
		WHERE board_id = ? AND on_approval_step_id = ?
	`, time.Now().UTC(), boardID, stepID)
	if err != nil {
		return fmt.Errorf("failed to clear on_approval_step_id references: %w", err)
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

// ListStepsByBoard returns all workflow steps for a board.
func (r *Repository) ListStepsByBoard(ctx context.Context, boardID string) ([]*models.WorkflowStep, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, board_id, name, step_type, position, color,
			auto_start_agent, plan_mode, require_approval,
			prompt_prefix, prompt_suffix, on_complete_step_id, on_approval_step_id,
			allow_manual_move, created_at, updated_at
		FROM workflow_steps WHERE board_id = ? ORDER BY position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanSteps(rows)
}

// GetStepByBoardAndType retrieves a workflow step by board ID and step type.
func (r *Repository) GetStepByBoardAndType(ctx context.Context, boardID string, stepType models.StepType) (*models.WorkflowStep, error) {
	step := &models.WorkflowStep{}
	var autoStartAgent, planMode, requireApproval, allowManualMove int
	var onCompleteStepID, onApprovalStepID, promptPrefix, promptSuffix, color sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, board_id, name, step_type, position, color,
			auto_start_agent, plan_mode, require_approval,
			prompt_prefix, prompt_suffix, on_complete_step_id, on_approval_step_id,
			allow_manual_move, created_at, updated_at
		FROM workflow_steps WHERE board_id = ? AND step_type = ?
	`, boardID, stepType).Scan(&step.ID, &step.BoardID, &step.Name, &step.StepType, &step.Position, &color,
		&autoStartAgent, &planMode, &requireApproval,
		&promptPrefix, &promptSuffix, &onCompleteStepID, &onApprovalStepID,
		&allowManualMove, &step.CreatedAt, &step.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow step not found for board %s with type %s", boardID, stepType)
	}
	if err != nil {
		return nil, err
	}

	step.AutoStartAgent = autoStartAgent == 1
	step.PlanMode = planMode == 1
	step.RequireApproval = requireApproval == 1
	step.AllowManualMove = allowManualMove == 1
	if color.Valid {
		step.Color = color.String
	}
	if promptPrefix.Valid {
		step.PromptPrefix = promptPrefix.String
	}
	if promptSuffix.Valid {
		step.PromptSuffix = promptSuffix.String
	}
	if onCompleteStepID.Valid {
		step.OnCompleteStepID = &onCompleteStepID.String
	}
	if onApprovalStepID.Valid {
		step.OnApprovalStepID = &onApprovalStepID.String
	}

	return step, nil
}

// scanSteps is a helper to scan multiple workflow step rows.
func (r *Repository) scanSteps(rows *sql.Rows) ([]*models.WorkflowStep, error) {
	var result []*models.WorkflowStep
	for rows.Next() {
		step := &models.WorkflowStep{}
		var autoStartAgent, planMode, requireApproval, allowManualMove int
		var onCompleteStepID, onApprovalStepID, promptPrefix, promptSuffix, color sql.NullString

		err := rows.Scan(&step.ID, &step.BoardID, &step.Name, &step.StepType, &step.Position, &color,
			&autoStartAgent, &planMode, &requireApproval,
			&promptPrefix, &promptSuffix, &onCompleteStepID, &onApprovalStepID,
			&allowManualMove, &step.CreatedAt, &step.UpdatedAt)
		if err != nil {
			return nil, err
		}

		step.AutoStartAgent = autoStartAgent == 1
		step.PlanMode = planMode == 1
		step.RequireApproval = requireApproval == 1
		step.AllowManualMove = allowManualMove == 1
		if color.Valid {
			step.Color = color.String
		}
		if promptPrefix.Valid {
			step.PromptPrefix = promptPrefix.String
		}
		if promptSuffix.Valid {
			step.PromptSuffix = promptSuffix.String
		}
		if onCompleteStepID.Valid {
			step.OnCompleteStepID = &onCompleteStepID.String
		}
		if onApprovalStepID.Valid {
			step.OnApprovalStepID = &onApprovalStepID.String
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
