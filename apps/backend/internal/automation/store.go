package automation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Store provides SQLite persistence for automations.
type Store struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewStore creates a new automation store and initializes the schema.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("automation schema init: %w", err)
	}
	return s, nil
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS automations (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL,
		executor_profile_id TEXT NOT NULL,
		prompt TEXT DEFAULT '',
		enabled BOOLEAN DEFAULT 1,
		max_concurrent_runs INTEGER DEFAULT 1,
		webhook_secret TEXT DEFAULT '',
		last_triggered_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS automation_triggers (
		id TEXT PRIMARY KEY,
		automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		enabled BOOLEAN DEFAULT 1,
		last_evaluated_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_automation_triggers_automation ON automation_triggers(automation_id);

	CREATE TABLE IF NOT EXISTS automation_runs (
		id TEXT PRIMARY KEY,
		automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
		trigger_id TEXT NOT NULL,
		trigger_type TEXT NOT NULL,
		task_id TEXT DEFAULT '',
		status TEXT NOT NULL,
		dedup_key TEXT DEFAULT '',
		trigger_data TEXT NOT NULL DEFAULT '{}',
		error_message TEXT DEFAULT '',
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_automation_runs_automation ON automation_runs(automation_id);
	CREATE INDEX IF NOT EXISTS idx_automation_runs_dedup ON automation_runs(automation_id, dedup_key);
`

const migrateTaskTitleSQL = `
	ALTER TABLE automations ADD COLUMN task_title_template TEXT DEFAULT '';
`

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	// Idempotent migration: add task_title_template column if missing.
	s.db.Exec(migrateTaskTitleSQL) //nolint:errcheck // column may already exist
	return nil
}

// --- Automation CRUD ---

// CreateAutomation persists a new automation.
func (s *Store) CreateAutomation(ctx context.Context, a *Automation) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.WebhookSecret == "" {
		a.WebhookSecret = generateSecret()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO automations (id, workspace_id, name, description, workflow_id, workflow_step_id,
			agent_profile_id, executor_profile_id, prompt, task_title_template, enabled, max_concurrent_runs,
			webhook_secret, last_triggered_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.WorkspaceID, a.Name, a.Description, a.WorkflowID, a.WorkflowStepID,
		a.AgentProfileID, a.ExecutorProfileID, a.Prompt, a.TaskTitleTemplate, a.Enabled, a.MaxConcurrentRuns,
		a.WebhookSecret, a.LastTriggeredAt, a.CreatedAt, a.UpdatedAt)
	return err
}

// GetAutomation returns an automation by ID with its triggers hydrated.
func (s *Store) GetAutomation(ctx context.Context, id string) (*Automation, error) {
	var a Automation
	err := s.ro.GetContext(ctx, &a, `SELECT * FROM automations WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	triggers, err := s.ListTriggers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("hydrate triggers: %w", err)
	}
	a.Triggers = triggers
	return &a, nil
}

// ListAutomations returns all automations for a workspace with triggers hydrated.
func (s *Store) ListAutomations(ctx context.Context, workspaceID string) ([]*Automation, error) {
	var automations []*Automation
	err := s.ro.SelectContext(ctx, &automations,
		`SELECT * FROM automations WHERE workspace_id = ? ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(automations) == 0 {
		return automations, nil
	}
	// Batch-load triggers for all automations.
	ids := make([]string, len(automations))
	for i, a := range automations {
		ids[i] = a.ID
	}
	triggersByAutomation, err := s.listTriggersForAutomations(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("hydrate triggers: %w", err)
	}
	for _, a := range automations {
		a.Triggers = triggersByAutomation[a.ID]
	}
	return automations, nil
}

// ListAllEnabled returns all enabled automations (across workspaces).
func (s *Store) ListAllEnabled(ctx context.Context) ([]*Automation, error) {
	var automations []*Automation
	err := s.ro.SelectContext(ctx, &automations,
		`SELECT * FROM automations WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	if len(automations) == 0 {
		return automations, nil
	}
	ids := make([]string, len(automations))
	for i, a := range automations {
		ids[i] = a.ID
	}
	triggersByAutomation, err := s.listTriggersForAutomations(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("hydrate triggers: %w", err)
	}
	for _, a := range automations {
		a.Triggers = triggersByAutomation[a.ID]
	}
	return automations, nil
}

// UpdateAutomation applies partial updates to an automation.
func (s *Store) UpdateAutomation(ctx context.Context, id string, req *UpdateAutomationRequest) error {
	a, err := s.GetAutomation(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("automation not found: %s", id)
	}
	if req.Name != nil {
		a.Name = *req.Name
	}
	if req.Description != nil {
		a.Description = *req.Description
	}
	if req.WorkflowID != nil {
		a.WorkflowID = *req.WorkflowID
	}
	if req.WorkflowStepID != nil {
		a.WorkflowStepID = *req.WorkflowStepID
	}
	if req.AgentProfileID != nil {
		a.AgentProfileID = *req.AgentProfileID
	}
	if req.ExecutorProfileID != nil {
		a.ExecutorProfileID = *req.ExecutorProfileID
	}
	if req.Prompt != nil {
		a.Prompt = *req.Prompt
	}
	if req.Enabled != nil {
		a.Enabled = *req.Enabled
	}
	if req.MaxConcurrentRuns != nil {
		a.MaxConcurrentRuns = *req.MaxConcurrentRuns
	}
	if req.TaskTitleTemplate != nil {
		a.TaskTitleTemplate = *req.TaskTitleTemplate
	}
	a.UpdatedAt = time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		UPDATE automations SET name = ?, description = ?, workflow_id = ?, workflow_step_id = ?,
			agent_profile_id = ?, executor_profile_id = ?, prompt = ?, task_title_template = ?,
			enabled = ?, max_concurrent_runs = ?, updated_at = ?
		WHERE id = ?`,
		a.Name, a.Description, a.WorkflowID, a.WorkflowStepID,
		a.AgentProfileID, a.ExecutorProfileID, a.Prompt, a.TaskTitleTemplate,
		a.Enabled, a.MaxConcurrentRuns, a.UpdatedAt, id)
	return err
}

// DeleteAutomation removes an automation and its triggers/runs (CASCADE).
func (s *Store) DeleteAutomation(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM automations WHERE id = ?`, id)
	return err
}

// UpdateLastTriggered updates the last_triggered_at timestamp.
func (s *Store) UpdateLastTriggered(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE automations SET last_triggered_at = ?, updated_at = ? WHERE id = ?`,
		t, time.Now().UTC(), id)
	return err
}

// --- Trigger CRUD ---

// CreateTrigger adds a trigger to an automation.
func (s *Store) CreateTrigger(ctx context.Context, t *AutomationTrigger) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	t.ConfigJSON = string(t.Config)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO automation_triggers (id, automation_id, type, config, enabled, last_evaluated_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.AutomationID, t.Type, t.ConfigJSON, t.Enabled, t.LastEvaluatedAt, t.CreatedAt, t.UpdatedAt)
	return err
}

// ListTriggers returns all triggers for an automation.
func (s *Store) ListTriggers(ctx context.Context, automationID string) ([]AutomationTrigger, error) {
	var triggers []AutomationTrigger
	err := s.ro.SelectContext(ctx, &triggers,
		`SELECT * FROM automation_triggers WHERE automation_id = ? ORDER BY created_at`, automationID)
	hydrateTriggers(triggers)
	return triggers, err
}

// hydrateTriggers converts the ConfigJSON string field to the Config json.RawMessage.
func hydrateTriggers(triggers []AutomationTrigger) {
	for i := range triggers {
		triggers[i].Config = json.RawMessage(triggers[i].ConfigJSON)
	}
}

func (s *Store) listTriggersForAutomations(ctx context.Context, automationIDs []string) (map[string][]AutomationTrigger, error) {
	if len(automationIDs) == 0 {
		return make(map[string][]AutomationTrigger), nil
	}
	query, args, err := sqlx.In(
		`SELECT * FROM automation_triggers WHERE automation_id IN (?) ORDER BY created_at`, automationIDs)
	if err != nil {
		return nil, err
	}
	query = s.ro.Rebind(query)
	var triggers []AutomationTrigger
	if err := s.ro.SelectContext(ctx, &triggers, query, args...); err != nil {
		return nil, err
	}
	hydrateTriggers(triggers)
	result := make(map[string][]AutomationTrigger, len(automationIDs))
	for i := range triggers {
		result[triggers[i].AutomationID] = append(result[triggers[i].AutomationID], triggers[i])
	}
	return result, nil
}

// UpdateTrigger applies partial updates to a trigger.
func (s *Store) UpdateTrigger(ctx context.Context, id string, req *UpdateTriggerRequest) error {
	var t AutomationTrigger
	err := s.ro.GetContext(ctx, &t, `SELECT * FROM automation_triggers WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("trigger not found: %s", id)
	}
	if err != nil {
		return err
	}
	if req.Config != nil {
		t.ConfigJSON = string(*req.Config)
	}
	if req.Enabled != nil {
		t.Enabled = *req.Enabled
	}
	t.UpdatedAt = time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`UPDATE automation_triggers SET config = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		t.ConfigJSON, t.Enabled, t.UpdatedAt, id)
	return err
}

// DeleteTrigger removes a trigger.
func (s *Store) DeleteTrigger(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM automation_triggers WHERE id = ?`, id)
	return err
}

// UpdateTriggerEvaluatedAt sets the last_evaluated_at timestamp.
func (s *Store) UpdateTriggerEvaluatedAt(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_triggers SET last_evaluated_at = ?, updated_at = ? WHERE id = ?`,
		t, time.Now().UTC(), id)
	return err
}

// ListEnabledTriggersByType returns enabled triggers of a specific type (across all enabled automations).
func (s *Store) ListEnabledTriggersByType(ctx context.Context, triggerType TriggerType) ([]AutomationTrigger, error) {
	var triggers []AutomationTrigger
	err := s.ro.SelectContext(ctx, &triggers, `
		SELECT t.* FROM automation_triggers t
		JOIN automations a ON a.id = t.automation_id
		WHERE t.type = ? AND t.enabled = 1 AND a.enabled = 1
		ORDER BY t.created_at`, string(triggerType))
	hydrateTriggers(triggers)
	return triggers, err
}

// --- Run operations ---

// CreateRun records a trigger firing.
func (s *Store) CreateRun(ctx context.Context, r *AutomationRun) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	r.CreatedAt = time.Now().UTC()
	r.TriggerDataJSON = string(r.TriggerData)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO automation_runs (id, automation_id, trigger_id, trigger_type, task_id, status,
			dedup_key, trigger_data, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.AutomationID, r.TriggerID, r.TriggerType, r.TaskID, r.Status,
		r.DedupKey, r.TriggerDataJSON, r.ErrorMessage, r.CreatedAt)
	return err
}

// ListRuns returns recent runs for an automation.
func (s *Store) ListRuns(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error) {
	if limit <= 0 {
		limit = 50
	}
	var runs []*AutomationRun
	err := s.ro.SelectContext(ctx, &runs,
		`SELECT * FROM automation_runs WHERE automation_id = ? ORDER BY created_at DESC LIMIT ?`,
		automationID, limit)
	for _, r := range runs {
		r.TriggerData = json.RawMessage(r.TriggerDataJSON)
	}
	return runs, err
}

// HasRunWithDedupKey checks if a run with the given dedup key already exists.
func (s *Store) HasRunWithDedupKey(ctx context.Context, automationID, dedupKey string) (bool, error) {
	if dedupKey == "" {
		return false, nil
	}
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM automation_runs WHERE automation_id = ? AND dedup_key = ?`,
		automationID, dedupKey)
	return count > 0, err
}

// CountActiveRuns returns the number of runs with task_created status for an automation.
func (s *Store) CountActiveRuns(ctx context.Context, automationID string) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM automation_runs WHERE automation_id = ? AND status = ?`,
		automationID, string(RunStatusTaskCreated))
	return count, err
}

// DeleteAutomationsByWorkspace removes all automations (and their triggers/runs) for a workspace.
// Used by e2e reset.
func (s *Store) DeleteAutomationsByWorkspace(ctx context.Context, workspaceID string) (int, error) {
	// Get automation IDs first for cascade cleanup.
	var ids []string
	if err := s.ro.SelectContext(ctx, &ids,
		`SELECT id FROM automations WHERE workspace_id = ?`, workspaceID); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	for _, id := range ids {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM automation_triggers WHERE automation_id = ?`, id)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM automation_runs WHERE automation_id = ?`, id)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM automations WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// generateSecret creates a random hex string for webhook authentication.
func generateSecret() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return uuid.New().String()
	}
	return hex.EncodeToString(b)
}
