// Package repository provides SQLite-based repository implementations for workflow entities.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/models"
)

// Repository provides SQLite-based workflow storage operations.
type Repository struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewWithDB creates a new SQLite repository with existing database connections.
func NewWithDB(writer, reader *sqlx.DB) (*Repository, error) {
	repo := &Repository{db: writer, ro: reader}
	if err := repo.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize workflow schema: %w", err)
	}
	return repo, nil
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
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
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
		is_start_step INTEGER DEFAULT 0,
		show_in_command_panel INTEGER DEFAULT 1,
		auto_archive_after_hours INTEGER DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow ON workflow_steps(workflow_id);
	`
	if _, err := r.db.Exec(stepsSchema); err != nil {
		return fmt.Errorf("failed to create workflow_steps table: %w", err)
	}

	// Create session_step_history table (id column differs between SQLite and PostgreSQL)
	var idCol string
	if r.db.DriverName() == "pgx" {
		idCol = "id BIGSERIAL PRIMARY KEY"
	} else {
		idCol = "id INTEGER PRIMARY KEY AUTOINCREMENT"
	}
	historySchema := `
	CREATE TABLE IF NOT EXISTS session_step_history (
		` + idCol + `,
		session_id TEXT NOT NULL,
		from_step_id TEXT,
		to_step_id TEXT NOT NULL,
		trigger TEXT NOT NULL,
		actor_id TEXT,
		metadata TEXT,
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_session_step_history_session ON session_step_history(session_id);
	`
	if _, err := r.db.Exec(historySchema); err != nil {
		return fmt.Errorf("failed to create session_step_history table: %w", err)
	}

	// Add show_in_command_panel column if it doesn't exist (idempotent migration)
	_, _ = r.db.Exec(`ALTER TABLE workflow_steps ADD COLUMN show_in_command_panel INTEGER DEFAULT 1`)

	// Seed system templates
	if err := r.seedSystemTemplates(); err != nil {
		return fmt.Errorf("failed to seed system templates: %w", err)
	}

	// Seed default workflow steps for workflows that don't have any
	if err := r.seedDefaultWorkflowSteps(); err != nil {
		return fmt.Errorf("failed to seed default workflow steps: %w", err)
	}

	// Repair step_id references that were incorrectly saved as template aliases
	// instead of UUIDs. This was caused by a frontend bug (fixed in PR #XXX).
	if err := r.repairBrokenStepIDReferences(); err != nil {
		return fmt.Errorf("failed to repair step_id references: %w", err)
	}

	return nil
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

	// Use the simple (kanban) template for default workflow steps
	now := time.Now()
	simpleTemplate, err := getKanbanTemplate()
	if err != nil {
		return err
	}

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

			if _, err := r.db.Exec(r.db.Rebind(`
				INSERT INTO workflow_steps (
					id, workflow_id, name, position, color,
					prompt, events, allow_manual_move, is_start_step, show_in_command_panel, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`),
				idMap[stepDef.ID], workflowID, stepDef.Name, stepDef.Position, stepDef.Color,
				stepDef.Prompt, string(eventsJSON), dialect.BoolToInt(stepDef.AllowManualMove),
				dialect.BoolToInt(stepDef.IsStartStep), dialect.BoolToInt(stepDef.ShowInCommandPanel), now, now,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// repairBrokenStepIDReferences fixes workflow steps that have template aliases
// (like "review", "in-progress") instead of actual UUIDs in their step_id config.
// This repairs data corrupted by a frontend bug where template events overwrote
// the backend's properly remapped events.
func (r *Repository) repairBrokenStepIDReferences() error {
	byWorkflow, err := r.loadStepsWithStepIDRefs()
	if err != nil {
		return err
	}
	for workflowID, steps := range byWorkflow {
		nameToUUID, err := r.buildStepNameMapping(workflowID)
		if err != nil {
			return err
		}
		if err := r.repairWorkflowSteps(steps, nameToUUID); err != nil {
			return err
		}
	}
	return nil
}

type brokenStepRow struct {
	id, workflowID, events string
}

func (r *Repository) loadStepsWithStepIDRefs() (map[string][]brokenStepRow, error) {
	rows, err := r.db.Query(`
		SELECT ws.id, ws.workflow_id, ws.events
		FROM workflow_steps ws
		WHERE ws.events LIKE '%"step_id"%'
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	byWorkflow := make(map[string][]brokenStepRow)
	for rows.Next() {
		var s brokenStepRow
		if err := rows.Scan(&s.id, &s.workflowID, &s.events); err != nil {
			return nil, err
		}
		byWorkflow[s.workflowID] = append(byWorkflow[s.workflowID], s)
	}
	return byWorkflow, rows.Err()
}

func (r *Repository) buildStepNameMapping(workflowID string) (map[string]string, error) {
	rows, err := r.db.Query(r.db.Rebind(`
		SELECT id, name FROM workflow_steps WHERE workflow_id = ?
	`), workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	nameToUUID := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		nameToUUID[normalizeStepName(name)] = id
	}
	return nameToUUID, rows.Err()
}

func (r *Repository) repairWorkflowSteps(steps []brokenStepRow, nameToUUID map[string]string) error {
	for _, step := range steps {
		if err := r.repairSingleStep(step, nameToUUID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) repairSingleStep(step brokenStepRow, nameToUUID map[string]string) error {
	var events models.StepEvents
	if err := json.Unmarshal([]byte(step.events), &events); err != nil {
		return nil // Skip malformed JSON
	}
	if !repairEventsStepIDs(&events, nameToUUID) {
		return nil // No repairs needed
	}
	data, err := json.Marshal(events)
	if err != nil {
		return nil
	}
	_, err = r.db.Exec(r.db.Rebind(`UPDATE workflow_steps SET events = ? WHERE id = ?`), string(data), step.id)
	if err != nil {
		return fmt.Errorf("repair step %s: %w", step.id, err)
	}
	return nil
}

func repairEventsStepIDs(events *models.StepEvents, nameToUUID map[string]string) bool {
	modified := false
	for i, a := range events.OnTurnStart {
		if a.Type == models.OnTurnStartMoveToStep && a.Config != nil {
			if repairStepIDConfig(a.Config, nameToUUID) {
				events.OnTurnStart[i] = a
				modified = true
			}
		}
	}
	for i, a := range events.OnTurnComplete {
		if a.Type == models.OnTurnCompleteMoveToStep && a.Config != nil {
			if repairStepIDConfig(a.Config, nameToUUID) {
				events.OnTurnComplete[i] = a
				modified = true
			}
		}
	}
	return modified
}

// normalizeStepName converts a step name to a lookup key (lowercase, spaces→hyphens)
func normalizeStepName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// repairStepIDConfig fixes a step_id config value if it's a template alias.
// Returns true if a repair was made.
func repairStepIDConfig(cfg map[string]interface{}, nameToUUID map[string]string) bool {
	stepID, ok := cfg["step_id"].(string)
	if !ok {
		return false
	}
	// Check if it's already a UUID (36 chars with hyphens at expected positions)
	if len(stepID) == 36 && stepID[8] == '-' && stepID[13] == '-' {
		return false
	}
	// Look up the real UUID by normalized name
	if realUUID, found := nameToUUID[normalizeStepName(stepID)]; found {
		cfg["step_id"] = realUUID
		return true
	}
	return false
}

// getKanbanTemplate loads the kanban ("simple") template from embedded YAML.
// Used by migration code to seed default workflow steps.
func getKanbanTemplate() (*models.WorkflowTemplate, error) {
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		return nil, fmt.Errorf("workflows: load embedded templates: %w", err)
	}
	for _, t := range templates {
		if t.ID == "simple" {
			return t, nil
		}
	}
	return nil, fmt.Errorf("workflows: kanban template (id=simple) not found in embedded YAML")
}

// seedSystemTemplates inserts the default system workflow templates.
func (r *Repository) seedSystemTemplates() error {
	templates, err := r.getSystemTemplates()
	if err != nil {
		return err
	}

	for _, tmpl := range templates {
		// Always upsert system templates to keep them current
		stepsJSON, err := json.Marshal(tmpl.Steps)
		if err != nil {
			return fmt.Errorf("failed to marshal steps for template %s: %w", tmpl.ID, err)
		}

		_, err = r.db.Exec(r.db.Rebind(`
			INSERT INTO workflow_templates (id, name, description, is_system, steps, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING
		`), tmpl.ID, tmpl.Name, tmpl.Description, dialect.BoolToInt(tmpl.IsSystem), string(stepsJSON), tmpl.CreatedAt, tmpl.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to upsert template %s: %w", tmpl.ID, err)
		}
	}

	return nil
}

// getSystemTemplates returns the predefined system workflow templates loaded from embedded YAML files.
func (r *Repository) getSystemTemplates() ([]*models.WorkflowTemplate, error) {
	return workflowcfg.LoadTemplates()
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

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_templates (id, name, description, is_system, steps, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), template.ID, template.Name, template.Description, dialect.BoolToInt(template.IsSystem), string(stepsJSON), template.CreatedAt, template.UpdatedAt)

	return err
}

// GetTemplate retrieves a workflow template by ID.
func (r *Repository) GetTemplate(ctx context.Context, id string) (*models.WorkflowTemplate, error) {
	template := &models.WorkflowTemplate{}
	var stepsJSON string
	var isSystem int

	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, description, is_system, steps, created_at, updated_at
		FROM workflow_templates WHERE id = ?
	`), id).Scan(&template.ID, &template.Name, &template.Description, &isSystem, &stepsJSON, &template.CreatedAt, &template.UpdatedAt)

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

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_templates SET name = ?, description = ?, steps = ?, updated_at = ?
		WHERE id = ? AND is_system = 0
	`), template.Name, template.Description, string(stepsJSON), template.UpdatedAt, template.ID)
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
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM workflow_templates WHERE id = ? AND is_system = 0`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workflow template not found or is a system template: %s", id)
	}
	return nil
}

func scanTemplateRows(rows *sql.Rows) ([]*models.WorkflowTemplate, error) {
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

// ListTemplates returns all workflow templates.
func (r *Repository) ListTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	rows, err := r.ro.QueryContext(ctx, `
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
	return scanTemplateRows(rows)
}

// GetSystemTemplates returns only system workflow templates.
func (r *Repository) GetSystemTemplates(ctx context.Context) ([]*models.WorkflowTemplate, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, description, is_system, steps, created_at, updated_at
		FROM workflow_templates WHERE is_system = 1 ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanTemplateRows(rows)
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

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, color,
			prompt, events, allow_manual_move, is_start_step, show_in_command_panel, auto_archive_after_hours, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), step.ID, step.WorkflowID, step.Name, step.Position, step.Color,
		step.Prompt, string(eventsJSON), dialect.BoolToInt(step.AllowManualMove),
		dialect.BoolToInt(step.IsStartStep), dialect.BoolToInt(step.ShowInCommandPanel), step.AutoArchiveAfterHours, step.CreatedAt, step.UpdatedAt)

	return err
}

// scanStep scans a single workflow step row including JSON events parsing.
func (r *Repository) scanStep(row interface {
	Scan(dest ...interface{}) error
}) (*models.WorkflowStep, error) {
	step := &models.WorkflowStep{}
	var allowManualMove, isStartStep, showInCommandPanel int
	var autoArchiveAfterHours sql.NullInt64
	var color, prompt, eventsJSON sql.NullString

	err := row.Scan(&step.ID, &step.WorkflowID, &step.Name, &step.Position, &color,
		&prompt, &eventsJSON, &allowManualMove, &isStartStep, &showInCommandPanel, &autoArchiveAfterHours, &step.CreatedAt, &step.UpdatedAt)

	if err != nil {
		return nil, err
	}

	step.AllowManualMove = allowManualMove == 1
	step.IsStartStep = isStartStep == 1
	step.ShowInCommandPanel = showInCommandPanel == 1
	if autoArchiveAfterHours.Valid {
		step.AutoArchiveAfterHours = int(autoArchiveAfterHours.Int64)
	}
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

const stepSelectColumns = `id, workflow_id, name, position, color, prompt, events, allow_manual_move, is_start_step, show_in_command_panel, auto_archive_after_hours, created_at, updated_at`

// GetStep retrieves a workflow step by ID.
func (r *Repository) GetStep(ctx context.Context, id string) (*models.WorkflowStep, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE id = ?
	`), id)

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

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_steps SET
			name = ?, position = ?, color = ?,
			prompt = ?, events = ?,
			allow_manual_move = ?, is_start_step = ?, show_in_command_panel = ?, auto_archive_after_hours = ?, updated_at = ?
		WHERE id = ?
	`), step.Name, step.Position, step.Color,
		step.Prompt, string(eventsJSON),
		dialect.BoolToInt(step.AllowManualMove), dialect.BoolToInt(step.IsStartStep), dialect.BoolToInt(step.ShowInCommandPanel), step.AutoArchiveAfterHours, step.UpdatedAt, step.ID)
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
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_steps SET is_start_step = 0
		WHERE workflow_id = ? AND id != ?
	`), workflowID, exceptStepID)
	return err
}

// GetStartStep returns the step marked as is_start_step for a workflow.
// Returns nil if no step is marked.
func (r *Repository) GetStartStep(ctx context.Context, workflowID string) (*models.WorkflowStep, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1
		LIMIT 1
	`), workflowID)

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
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM workflow_steps WHERE id = ?`), id)
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
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE workflow_id = ? ORDER BY position
	`), workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanSteps(rows)
}

// ListStepsByWorkspaceID returns all workflow steps for all workflows in a workspace.
func (r *Repository) ListStepsByWorkspaceID(ctx context.Context, workspaceID string) ([]*models.WorkflowStep, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT ws.id, ws.workflow_id, ws.name, ws.position, ws.color, ws.prompt, ws.events,
			ws.allow_manual_move, ws.is_start_step, ws.show_in_command_panel, ws.auto_archive_after_hours, ws.created_at, ws.updated_at
		FROM workflow_steps ws
		JOIN workflows w ON ws.workflow_id = w.id
		WHERE w.workspace_id = ?
		ORDER BY ws.workflow_id, ws.position
	`), workspaceID)
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

	id, err := dialect.InsertReturningID(ctx, r.db,
		`INSERT INTO session_step_history (session_id, from_step_id, to_step_id, trigger, actor_id, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		history.SessionID, history.FromStepID, history.ToStepID, history.Trigger, history.ActorID, metadataJSON, history.CreatedAt)
	if err != nil {
		return err
	}
	history.ID = id

	return nil
}

// ListHistoryBySession returns all step history entries for a session.
func (r *Repository) ListHistoryBySession(ctx context.Context, sessionID string) ([]*models.SessionStepHistory, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, from_step_id, to_step_id, trigger, actor_id, metadata, created_at
		FROM session_step_history WHERE session_id = ? ORDER BY created_at
	`), sessionID)
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
