package automation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
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
		repository_id TEXT NOT NULL DEFAULT '',
		prompt TEXT DEFAULT '',
		task_title_template TEXT DEFAULT '',
		-- execution_mode is retained so existing rows need no migration; the
		-- task/run execution-mode choice is withdrawn and nothing reads it.
		execution_mode TEXT NOT NULL DEFAULT 'task',
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
	CREATE INDEX IF NOT EXISTS idx_automation_runs_created_at ON automation_runs(created_at DESC);
	-- The summary query asks each automation for its newest run, ordered by
	-- (created_at, id) — the ordering every run query uses. Without the
	-- composite the per-automation lookup sorts that automation's whole run
	-- history, once per candidate row, which is quadratic in a workspace's run
	-- count. /automations runs this on load and every poll while anything is
	-- running, so the sort shows up as seconds of latency, not milliseconds.
	CREATE INDEX IF NOT EXISTS idx_automation_runs_automation_created ON automation_runs(automation_id, created_at DESC, id DESC);

	CREATE TABLE IF NOT EXISTS automation_repositories (
		id TEXT PRIMARY KEY,
		automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
		repository_id TEXT NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		UNIQUE(automation_id, repository_id)
	);

	CREATE INDEX IF NOT EXISTS idx_automation_repositories_automation ON automation_repositories(automation_id);
`

// In-branch column additions. The canonical CREATE TABLE covers fresh
// installs; these ALTERs cover DBs already initialised from an earlier
// commit on this branch (the original PR #406 schema). SQLite returns a
// duplicate-column error when the column already exists, which we swallow.
//
// automations.repository_id is retained as a legacy, write-once column: it
// is never read or written by current code (repository selection now lives
// in automation_repositories), but dropping a column referenced by two
// FK-child tables (automation_triggers, automation_runs) under
// foreign_keys=on would require table-recreate migration infrastructure
// this package doesn't have yet. Every query that scans a full Automation
// row uses the explicit automationColumns list, which omits it, so its
// continued presence in the table is inert.
const (
	migrateTaskTitleSQL     = `ALTER TABLE automations ADD COLUMN task_title_template TEXT DEFAULT ''`
	migrateExecutionModeSQL = `ALTER TABLE automations ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'task'`
	migrateRepositoryIDSQL  = `ALTER TABLE automations ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`
)

// automationColumns is the explicit column list for every query that scans a
// full Automation row. Spelled out rather than `SELECT *` because the table
// carries two columns the Automation struct does not: the legacy repository_id
// (superseded by automation_repositories — see the comment above
// migrateRepositoryIDSQL) and the withdrawn execution_mode. sqlx would fail to
// map either.
const automationColumns = `id, workspace_id, name, description, workflow_id, workflow_step_id,
	agent_profile_id, executor_profile_id, prompt, task_title_template,
	enabled, max_concurrent_runs, webhook_secret, last_triggered_at, created_at, updated_at`

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	s.db.Exec(migrateTaskTitleSQL)     //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateExecutionModeSQL) //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRepositoryIDSQL)  //nolint:errcheck // duplicate-column on existing DBs
	return s.backfillLegacyRepositoryIDs()
}

// backfillLegacyRepositoryIDs copies every non-empty legacy
// automations.repository_id value into automation_repositories (position 0)
// the first time a DB upgrades to this schema. Idempotent: the
// UNIQUE(automation_id, repository_id) constraint plus INSERT OR IGNORE
// means re-running this on an already-migrated DB inserts nothing new.
func (s *Store) backfillLegacyRepositoryIDs() error {
	type legacyRow struct {
		ID           string    `db:"id"`
		RepositoryID string    `db:"repository_id"`
		CreatedAt    time.Time `db:"created_at"`
	}
	var rows []legacyRow
	err := s.db.Select(&rows,
		`SELECT id, repository_id, created_at FROM automations WHERE repository_id != ''`)
	if err != nil {
		return fmt.Errorf("select legacy repository_id rows: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin legacy repository_id backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, row := range rows {
		_, err := tx.Exec(
			`INSERT OR IGNORE INTO automation_repositories (id, automation_id, repository_id, position, created_at)
			VALUES (?, ?, ?, 0, ?)`,
			uuid.New().String(), row.ID, row.RepositoryID, row.CreatedAt)
		if err != nil {
			return fmt.Errorf("backfill automation_repositories for %s: %w", row.ID, err)
		}
		// Clear the legacy column in the same transaction as the insert.
		// Without this, a later UpdateAutomation that removes this exact
		// repository from automation_repositories would resurrect it on
		// the next boot's backfill pass (INSERT OR IGNORE only blocks
		// exact re-insertion, not re-addition after deletion).
		if _, err := tx.Exec(`UPDATE automations SET repository_id = '' WHERE id = ?`, row.ID); err != nil {
			return fmt.Errorf("clear legacy repository_id for %s: %w", row.ID, err)
		}
	}
	return tx.Commit()
}

// --- Automation CRUD ---

// CreateAutomation persists a new automation and its repository_ids.
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
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// execution_mode is omitted deliberately — the column's NOT NULL DEFAULT
	// fills it and no code path reads it back.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO automations (id, workspace_id, name, description, workflow_id, workflow_step_id,
			agent_profile_id, executor_profile_id,
			prompt, task_title_template,
			enabled, max_concurrent_runs,
			webhook_secret, last_triggered_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.WorkspaceID, a.Name, a.Description, a.WorkflowID, a.WorkflowStepID,
		a.AgentProfileID, a.ExecutorProfileID,
		a.Prompt, a.TaskTitleTemplate,
		a.Enabled, a.MaxConcurrentRuns,
		a.WebhookSecret, a.LastTriggeredAt, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return err
	}
	if err := insertAutomationRepositories(ctx, tx, a.ID, a.RepositoryIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// insertAutomationRepositories inserts one automation_repositories row per
// ID, preserving slice order via the position column. No-op for an empty
// slice. Shared by CreateAutomation and UpdateAutomation's replace path.
func insertAutomationRepositories(ctx context.Context, tx *sqlx.Tx, automationID string, repositoryIDs []string) error {
	now := time.Now().UTC()
	for i, repositoryID := range repositoryIDs {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO automation_repositories (id, automation_id, repository_id, position, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			uuid.New().String(), automationID, repositoryID, i, now)
		if err != nil {
			return fmt.Errorf("insert automation_repositories: %w", err)
		}
	}
	return nil
}

// GetAutomation returns an automation by ID with its triggers and
// repository_ids hydrated.
func (s *Store) GetAutomation(ctx context.Context, id string) (*Automation, error) {
	var a Automation
	err := s.ro.GetContext(ctx, &a, `SELECT `+automationColumns+` FROM automations WHERE id = ?`, id)
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
	repoIDs, err := s.listRepositoryIDsForAutomations(ctx, []string{id})
	if err != nil {
		return nil, fmt.Errorf("hydrate repository_ids: %w", err)
	}
	a.RepositoryIDs = repoIDs[id]
	return &a, nil
}

// listRepositoryIDsForAutomations batch-loads ordered repository_ids for
// several automations at once, mirroring listTriggersForAutomations.
func (s *Store) listRepositoryIDsForAutomations(ctx context.Context, automationIDs []string) (map[string][]string, error) {
	if len(automationIDs) == 0 {
		return make(map[string][]string), nil
	}
	query, args, err := sqlx.In(
		`SELECT automation_id, repository_id FROM automation_repositories
		WHERE automation_id IN (?) ORDER BY automation_id, position`, automationIDs)
	if err != nil {
		return nil, err
	}
	query = s.ro.Rebind(query)
	type row struct {
		AutomationID string `db:"automation_id"`
		RepositoryID string `db:"repository_id"`
	}
	var rows []row
	if err := s.ro.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(automationIDs))
	for _, id := range automationIDs {
		result[id] = []string{}
	}
	for _, r := range rows {
		result[r.AutomationID] = append(result[r.AutomationID], r.RepositoryID)
	}
	return result, nil
}

// AgentProfileBinding names one automation bound to an agent profile.
type AgentProfileBinding struct {
	ID          string `db:"id"`
	Name        string `db:"name"`
	WorkspaceID string `db:"workspace_id"`
}

// ListEnabledByAgentProfile returns the enabled automations that would launch
// against the given agent profile.
//
// Only enabled ones: a disabled automation is not going to fire, so naming it
// as a reason you cannot delete a profile is noise. Triggers and repositories
// are deliberately not hydrated — the caller wants identity, not the object.
func (s *Store) ListEnabledByAgentProfile(ctx context.Context, agentProfileID string) ([]AgentProfileBinding, error) {
	if agentProfileID == "" {
		return nil, nil
	}
	var rows []AgentProfileBinding
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT id, name, workspace_id FROM automations
		WHERE agent_profile_id = ? AND enabled = 1
		ORDER BY name
	`), agentProfileID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// DisableByAgentProfile turns off every enabled automation bound to the given
// agent profile, returning the ones it disabled so the caller can report.
//
// Used when a profile is force-deleted: an automation left enabled would keep
// firing on its schedule into a profile that no longer exists. Disabling rather
// than deleting keeps the automation recoverable — the user picks a new profile
// and toggles it back on.
func (s *Store) DisableByAgentProfile(ctx context.Context, agentProfileID string) ([]AgentProfileBinding, error) {
	if agentProfileID == "" {
		return nil, nil
	}
	bindings, err := s.ListEnabledByAgentProfile(ctx, agentProfileID)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automations SET enabled = 0, updated_at = ?
		WHERE agent_profile_id = ? AND enabled = 1
	`), time.Now().UTC(), agentProfileID)
	if err != nil {
		return nil, fmt.Errorf("disable automations for agent profile: %w", err)
	}
	return bindings, nil
}

// ListAutomations returns all automations for a workspace with triggers and
// repository_ids hydrated.
func (s *Store) ListAutomations(ctx context.Context, workspaceID string) ([]*Automation, error) {
	var automations []*Automation
	err := s.ro.SelectContext(ctx, &automations,
		`SELECT `+automationColumns+` FROM automations WHERE workspace_id = ? ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	return automations, s.hydrateAutomations(ctx, automations)
}

// ListAllEnabled returns all enabled automations (across workspaces).
func (s *Store) ListAllEnabled(ctx context.Context) ([]*Automation, error) {
	var automations []*Automation
	err := s.ro.SelectContext(ctx, &automations,
		`SELECT `+automationColumns+` FROM automations WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return automations, s.hydrateAutomations(ctx, automations)
}

// hydrateAutomations batch-loads triggers and repository_ids onto an
// already-fetched automations slice. Shared by ListAutomations/ListAllEnabled.
func (s *Store) hydrateAutomations(ctx context.Context, automations []*Automation) error {
	if len(automations) == 0 {
		return nil
	}
	ids := make([]string, len(automations))
	for i, a := range automations {
		ids[i] = a.ID
	}
	triggersByAutomation, err := s.listTriggersForAutomations(ctx, ids)
	if err != nil {
		return fmt.Errorf("hydrate triggers: %w", err)
	}
	repoIDsByAutomation, err := s.listRepositoryIDsForAutomations(ctx, ids)
	if err != nil {
		return fmt.Errorf("hydrate repository_ids: %w", err)
	}
	for _, a := range automations {
		a.Triggers = triggersByAutomation[a.ID]
		a.RepositoryIDs = repoIDsByAutomation[a.ID]
	}
	return nil
}

// UpdateAutomation applies partial updates to an automation. When
// req.RepositoryIDs is non-nil, it atomically replaces the automation's
// automation_repositories rows (nil means "leave unchanged"; an explicit
// empty slice clears the list).
func (s *Store) UpdateAutomation(ctx context.Context, id string, req *UpdateAutomationRequest) error {
	a, err := s.GetAutomation(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("automation not found: %s", id)
	}
	applyAutomationUpdate(a, req)
	a.UpdatedAt = time.Now().UTC()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		UPDATE automations SET name = ?, description = ?, workflow_id = ?, workflow_step_id = ?,
			agent_profile_id = ?, executor_profile_id = ?,
			prompt = ?, task_title_template = ?,
			enabled = ?, max_concurrent_runs = ?, updated_at = ?
		WHERE id = ?`,
		a.Name, a.Description, a.WorkflowID, a.WorkflowStepID,
		a.AgentProfileID, a.ExecutorProfileID,
		a.Prompt, a.TaskTitleTemplate,
		a.Enabled, a.MaxConcurrentRuns, a.UpdatedAt, id)
	if err != nil {
		return err
	}
	if req.RepositoryIDs != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM automation_repositories WHERE automation_id = ?`, id); err != nil {
			return fmt.Errorf("clear automation_repositories: %w", err)
		}
		if err := insertAutomationRepositories(ctx, tx, id, req.RepositoryIDs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func applyAutomationUpdate(a *Automation, req *UpdateAutomationRequest) {
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

// GetTriggerAutomationID resolves the automation a trigger belongs to. Used by
// the auth layer to authorize trigger mutations by workspace ownership.
func (s *Store) GetTriggerAutomationID(ctx context.Context, triggerID string) (string, error) {
	var automationID string
	err := s.ro.GetContext(ctx, &automationID,
		`SELECT automation_id FROM automation_triggers WHERE id = ?`, triggerID)
	return automationID, err
}

// GetTrigger returns a single trigger, or nil when it no longer exists.
func (s *Store) GetTrigger(ctx context.Context, id string) (*AutomationTrigger, error) {
	var triggers []AutomationTrigger
	if err := s.ro.SelectContext(ctx, &triggers,
		`SELECT * FROM automation_triggers WHERE id = ?`, id); err != nil {
		return nil, err
	}
	if len(triggers) == 0 {
		return nil, nil
	}
	hydrateTriggers(triggers)
	return &triggers[0], nil
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

// MarkRunFailedByTaskID flips the most recent task_created run for a task
// into the failed state. Used when a downstream condition (e.g. a permission
// prompt an automation run can't answer) makes the run effectively
// dead. No-op if no matching run is found.
func (s *Store) MarkRunFailedByTaskID(ctx context.Context, taskID, errMsg string) error {
	return s.updateRunTerminalStatus(ctx, taskID, RunStatusFailed, errMsg)
}

// MarkRunSucceededByTaskID flips the most recent task_created run for a task
// into the succeeded state. Used when an automation-launched agent completes
// without error.
func (s *Store) MarkRunSucceededByTaskID(ctx context.Context, taskID string) error {
	return s.updateRunTerminalStatus(ctx, taskID, RunStatusSucceeded, "")
}

// updateRunTerminalStatus is the shared implementation behind MarkRun{Failed,Succeeded}ByTaskID.
func (s *Store) updateRunTerminalStatus(ctx context.Context, taskID string, status RunStatus, errMsg string) error {
	if taskID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE automation_runs SET status = ?, error_message = ?
		WHERE id = (
			SELECT id FROM automation_runs
			WHERE task_id = ? AND status = ?
			ORDER BY created_at DESC LIMIT 1
		)`,
		string(status), errMsg, taskID, string(RunStatusTaskCreated))
	return err
}

// ListRuns returns recent runs for an automation. A task_created run whose
// generated task has been archived is reported as archived; one whose task
// is gone entirely or explicitly cancelled is reported as cancelled — see
// listRunsWithTaskState for the full precedence — without touching runs
// that already reached a real terminal status. Falls back to the raw
// stored status when the tasks table isn't present (isolated
// automation-only tests; production always has it, migrated by the task
// repository before automation triggers can fire).
func (s *Store) ListRuns(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error) {
	if limit <= 0 {
		limit = 50
	}
	runs, err := s.listRunsWithTaskState(ctx, automationID, limit)
	if db.IsMissingTableError(err) {
		runs, err = s.listRunsRaw(ctx, automationID, limit)
	}
	if err != nil {
		return nil, err
	}
	for _, r := range runs {
		r.TriggerData = json.RawMessage(r.TriggerDataJSON)
	}
	return runs, nil
}

// runTaskStateColumnsSQL is the read-time projection of a run, shared
// verbatim by the per-automation and workspace-wide list queries. It is a
// const rather than duplicated SQL because the status derivation below is
// the app's definition of what a run's status *means* to a reader: two
// copies would drift, and the same run would then report one status on the
// automation's settings page and another in the workspace feed. Assumes
// the caller aliases automation_runs as ar and LEFT JOINs tasks as t, and
// binds runTaskStateArgs() ahead of its own WHERE/LIMIT parameters.
//
// Assumes a task_created run always carries a non-empty ar.task_id: the
// sole production write path (orchestrator's recordSuccessRun) sets TaskID
// and Status together in the same INSERT. If that's ever violated, the
// LEFT JOIN never matches an empty task_id against a real task row, so the
// run falls into the "no live task" branch below and displays as cancelled
// rather than its raw stored status — reachable today only through the e2e
// run-seeding endpoint, never in production.
//
// Three read-time overrides of a still-open task_created run, in priority
// order: (1) the task row is gone entirely — deleted, outcome
// unrecoverable — shown as cancelled; (2) the task was archived
// (archived_at set, via the UI or by the agent itself, e.g. an "archive
// this task" instruction) — shown as archived, which takes precedence over
// the session check below; (3) the task's current (is_primary) session is
// CANCELLED — set only when an agent run was manually stopped
// (coordinator/MCP stop_task, or the UI Stop button) — a deliberate
// cancellation, shown as cancelled. Task.state is deliberately NOT
// consulted here: stopping an agent leaves the task itself at whatever
// state the stop caller chose (e.g. REVIEW) and only ever marks the
// *session* CANCELLED — see orchestrator.handleAgentStopped's "we do NOT
// update task state here" note. Filtering on is_primary picks the task's
// current session, so a resumed-and-completed task isn't misclassified by
// a stale cancelled session left over from an earlier stop. EXISTS (rather
// than a LEFT JOIN) keeps the query correct even if the "at most one
// is_primary=1 row per task" invariant is ever violated — a join would fan
// out and duplicate the run.
const runTaskStateColumnsSQL = `
		ar.id, ar.automation_id, ar.trigger_id, ar.trigger_type,
		-- A run keeps its task_id after the task row is deleted, which reads as a
		-- transcript that can still be opened. Report no task rather than a link
		-- that dead-ends; the derived cancelled status below already says why.
		CASE WHEN t.id IS NULL THEN '' ELSE ar.task_id END AS task_id,
		CASE
			WHEN ar.status = ? AND t.id IS NULL THEN ?
			WHEN ar.status = ? AND t.archived_at IS NOT NULL THEN ?
			WHEN ar.status = ? AND EXISTS (
				SELECT 1 FROM task_sessions ts
				WHERE ts.task_id = ar.task_id AND ts.is_primary = 1 AND ts.state = ?
			) THEN ?
			ELSE ar.status
		END AS status,
		ar.dedup_key, ar.trigger_data, ar.error_message, ar.created_at,
		COALESCE((
			SELECT substr(m.content, 1, 280) FROM task_session_messages m
			WHERE m.task_id = ar.task_id
				AND m.author_type = 'agent'
				AND m.type = 'message'
			ORDER BY m.created_at DESC, m.id DESC LIMIT 1
		), '') AS summary,
		-- The run's conversation. The detail view reads the transcript in place
		-- rather than sending the reader to the task page, and the chat panel is
		-- driven by a session id, not a task id — resolving it here keeps that
		-- one query instead of one per run on the client.
		COALESCE((
			SELECT ts.id FROM task_sessions ts
			WHERE ts.task_id = ar.task_id AND ts.is_primary = 1
			LIMIT 1
		), '') AS session_id`

// runTaskStateArgs binds the placeholders in runTaskStateColumnsSQL, in
// order. Kept next to the SQL so a new WHEN can't be added without the
// matching argument being obvious.
func runTaskStateArgs() []any {
	return []any{
		string(RunStatusTaskCreated), string(RunStatusCancelled),
		string(RunStatusTaskCreated), string(RunStatusArchived),
		string(RunStatusTaskCreated), string(taskmodels.TaskSessionStateCancelled), string(RunStatusCancelled),
	}
}

func (s *Store) listRunsWithTaskState(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error) {
	var runs []*AutomationRun
	err := s.ro.SelectContext(ctx, &runs, `
		SELECT`+runTaskStateColumnsSQL+`
		FROM automation_runs ar
		LEFT JOIN tasks t ON t.id = ar.task_id
		WHERE ar.automation_id = ?
		ORDER BY `+runOrderSQL("ar")+` LIMIT ?`,
		append(runTaskStateArgs(), automationID, limit)...)
	return runs, err
}

func (s *Store) listRunsRaw(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error) {
	var runs []*AutomationRun
	err := s.ro.SelectContext(ctx, &runs,
		`SELECT * FROM automation_runs WHERE automation_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`,
		automationID, limit)
	return runs, err
}

// maxWorkspaceRunsLimit caps the workspace-wide feed. Unlike ListRuns, which
// is scoped to one automation the user is already looking at, this query
// spans every automation in the workspace — an uncapped limit would let a
// single client pull the entire run history over the socket.
const maxWorkspaceRunsLimit = 200

// ListWorkspaceRuns returns recent runs across every automation in a
// workspace, newest first, each attributed to its automation. Status
// derivation and summary are identical to ListRuns — same
// runTaskStateColumnsSQL — so a run reads the same way in the workspace
// feed as it does on its own automation's page. Falls back to raw stored
// status when the tasks table isn't present, for the same reason ListRuns
// does (isolated automation-only tests; production always has it).
func (s *Store) ListWorkspaceRuns(ctx context.Context, workspaceID string, limit int) ([]*WorkspaceAutomationRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > maxWorkspaceRunsLimit {
		limit = maxWorkspaceRunsLimit
	}
	runs, err := s.listWorkspaceRunsWithTaskState(ctx, workspaceID, limit)
	if db.IsMissingTableError(err) {
		runs, err = s.listWorkspaceRunsRaw(ctx, workspaceID, limit)
	}
	if err != nil {
		return nil, err
	}
	for _, r := range runs {
		r.TriggerData = json.RawMessage(r.TriggerDataJSON)
	}
	return runs, nil
}

func (s *Store) listWorkspaceRunsWithTaskState(ctx context.Context, workspaceID string, limit int) ([]*WorkspaceAutomationRun, error) {
	// The automations join is an INNER join, not a LEFT one: a run whose
	// automation is gone can't be attributed to anything the reader can
	// open, and the ON DELETE CASCADE means it shouldn't exist anyway.
	var runs []*WorkspaceAutomationRun
	err := s.ro.SelectContext(ctx, &runs, `
		SELECT`+runTaskStateColumnsSQL+`,
			a.name AS automation_name
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		LEFT JOIN tasks t ON t.id = ar.task_id
		WHERE a.workspace_id = ?
		ORDER BY `+runOrderSQL("ar")+` LIMIT ?`,
		append(runTaskStateArgs(), workspaceID, limit)...)
	return runs, err
}

// ListAutomationSummaries returns one row per automation in a workspace that
// has ever run: its newest run and how many of its runs are still open.
//
// The runs list used to derive both by scanning the workspace feed, which is
// capped. Past the cap an automation's newest run falls out of the window and
// its row reports "No runs yet" and idle — the two things a health indicator
// must never get wrong — and the open count backing "won't fire: still
// running" silently drops to zero. Answering per automation makes the row's
// claims independent of how noisy its neighbours are.
func (s *Store) ListAutomationSummaries(ctx context.Context, workspaceID string) ([]*AutomationSummary, error) {
	return s.listSummaries(ctx, "a.workspace_id = ?", workspaceID)
}

// GetAutomationSummary returns one automation's summary, or nil if it has never
// run. The detail page needs the same authoritative open count the list uses:
// its own run window is capped too, so an open run older than the window would
// otherwise leave the page reporting that nothing is in flight.
func (s *Store) GetAutomationSummary(ctx context.Context, automationID string) (*AutomationSummary, error) {
	summaries, err := s.listSummaries(ctx, "ar.automation_id = ?", automationID)
	if err != nil || len(summaries) == 0 {
		return nil, err
	}
	return summaries[0], nil
}

// listSummaries answers both facts in ONE statement.
//
// Two queries would be two snapshots: a run created between them reads as
// `last_run = task_created` with `open_runs = 0`, so the row renders idle and
// the client never starts polling — permanently stale until a manual refresh.
// The open count is a correlated subquery over the same automation, which also
// means an automation with an open run always has a latest run, so no second
// pass is needed to invent rows for counts without runs.
func (s *Store) listSummaries(ctx context.Context, scope string, arg any) ([]*AutomationSummary, error) {
	rows, err := s.selectSummaries(ctx, scope, arg)
	if db.IsMissingTableError(err) {
		rows, err = s.selectSummariesRaw(ctx, scope, arg)
	}
	if err != nil {
		return nil, err
	}
	summaries := make([]*AutomationSummary, 0, len(rows))
	for _, row := range rows {
		run := row.AutomationRun
		run.TriggerData = json.RawMessage(run.TriggerDataJSON)
		summaries = append(summaries, &AutomationSummary{
			AutomationID: run.AutomationID,
			OpenRuns:     row.OpenRuns,
			LastRun:      &run,
		})
	}
	return summaries, nil
}

// summaryRow is the wire shape of the single summary query: a run plus its
// automation's open count.
type summaryRow struct {
	AutomationRun
	OpenRuns int `db:"open_runs"`
}

// openRunsSubquerySQL counts an automation's outstanding runs alongside its
// latest one. Aliased aro/tro so it can nest inside a query that already uses
// ar/t, and it binds openRunArgs() like every other user of the predicate.
var openRunsSubquerySQL = `(
			SELECT COUNT(*) FROM automation_runs aro
			LEFT JOIN tasks tro ON tro.id = aro.task_id
			WHERE aro.automation_id = ar.automation_id AND ` + openRunPredicateAliased("aro", "tro") + `
		) AS open_runs`

// latestRunPerAutomationSQL picks each automation's newest run by the same
// ordering every other run query uses (created_at then id), so "the newest run"
// means the row that leads the feed rather than an arbitrary tie-break.
var latestRunPerAutomationSQL = `ar.id = (
			SELECT ar2.id FROM automation_runs ar2
			WHERE ar2.automation_id = ar.automation_id
			ORDER BY ` + runOrderSQL("ar2") + ` LIMIT 1
		)`

func (s *Store) selectSummaries(ctx context.Context, scope string, arg any) ([]summaryRow, error) {
	var rows []summaryRow
	args := runTaskStateArgs()
	args = append(args, openRunArgs()...)
	args = append(args, arg)
	err := s.ro.SelectContext(ctx, &rows, `
		SELECT`+runTaskStateColumnsSQL+`,
			`+openRunsSubquerySQL+`
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		LEFT JOIN tasks t ON t.id = ar.task_id
		WHERE `+scope+` AND `+latestRunPerAutomationSQL,
		args...)
	return rows, err
}

// selectSummariesRaw is the no-tasks-table fallback the run lists also carry
// (isolated automation-only tests; production always has the table). Without
// tasks there is nothing to derive from, so the stored status stands and the
// open count is a plain status match.
func (s *Store) selectSummariesRaw(ctx context.Context, scope string, arg any) ([]summaryRow, error) {
	var rows []summaryRow
	err := s.ro.SelectContext(ctx, &rows, `
		SELECT ar.*, (
			SELECT COUNT(*) FROM automation_runs aro
			WHERE aro.automation_id = ar.automation_id AND aro.status = ?
		) AS open_runs
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		WHERE `+scope+` AND `+latestRunPerAutomationSQL,
		string(RunStatusTaskCreated), arg)
	return rows, err
}

func (s *Store) listWorkspaceRunsRaw(ctx context.Context, workspaceID string, limit int) ([]*WorkspaceAutomationRun, error) {
	var runs []*WorkspaceAutomationRun
	err := s.ro.SelectContext(ctx, &runs, `
		SELECT ar.*, a.name AS automation_name
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		WHERE a.workspace_id = ?
		ORDER BY `+runOrderSQL("ar")+` LIMIT ?`,
		workspaceID, limit)
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

// CountActiveRuns returns the number of runs with task_created status for
// an automation whose generated task is still open. A task_created run
// whose task was archived, deleted, or explicitly cancelled no longer
// represents outstanding work — the user (or agent) closed it out some
// other way — so it must not keep counting against max_concurrent_runs
// forever. Falls back to a plain count when the tasks table isn't present
// (isolated automation-only tests; production always has it).
func (s *Store) CountActiveRuns(ctx context.Context, automationID string) (int, error) {
	count, err := s.countActiveRunsWithTaskState(ctx, automationID)
	if db.IsMissingTableError(err) {
		return s.countActiveRunsRaw(ctx, automationID)
	}
	return count, err
}

// openRunPredicateSQL is what "this run is still outstanding" means, shared by
// the concurrency-cap count and the per-automation summary the runs list reads.
// One definition, for the same reason runTaskStateColumnsSQL is one: the cap
// deciding a run is open while the list shows the automation idle is a
// contradiction the user cannot resolve from the screen. Assumes automation_runs
// aliased ar with tasks LEFT JOINed as t, and binds two arguments —
// RunStatusTaskCreated and the cancelled session state.
//
// Same non-empty-task_id assumption as listRunsWithTaskState: an empty
// ar.task_id never matches a real task row, so such a run falls out of the
// open set instead of erroring. See listRunsWithTaskState for why the current
// (is_primary) session's state, not the task's own state, is the cancellation
// signal, and why NOT EXISTS rather than a LEFT JOIN.
const openRunPredicateSQL = `ar.status = ?
		AND t.id IS NOT NULL AND t.archived_at IS NULL
		AND NOT EXISTS (
			SELECT 1 FROM task_sessions ts
			WHERE ts.task_id = ar.task_id AND ts.is_primary = 1 AND ts.state = ?
		)`

// openRunPredicateAliased is the same predicate under different table aliases,
// for the summary query where it nests inside a statement already using ar/t.
// Derived from openRunPredicateSQL rather than written twice so the definition
// of "open" stays in one place.
func openRunPredicateAliased(runAlias, taskAlias string) string {
	replaced := strings.ReplaceAll(openRunPredicateSQL, "ar.", runAlias+".")
	return strings.ReplaceAll(replaced, "t.", taskAlias+".")
}

// runOrderSQL is how every run query orders: newest first, with the id breaking
// ties. Without the tie-break two runs written in the same second can order one
// way in the feed and the other way in the summary, so the list's "last said"
// would disagree with the entry that leads the automation's own activity.
func runOrderSQL(alias string) string {
	return alias + ".created_at DESC, " + alias + ".id DESC"
}

// openRunArgs binds openRunPredicateSQL's placeholders, in order.
func openRunArgs() []any {
	return []any{string(RunStatusTaskCreated), string(taskmodels.TaskSessionStateCancelled)}
}

func (s *Store) countActiveRunsWithTaskState(ctx context.Context, automationID string) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM automation_runs ar
		LEFT JOIN tasks t ON t.id = ar.task_id
		WHERE ar.automation_id = ? AND `+openRunPredicateSQL,
		append([]any{automationID}, openRunArgs()...)...)
	return count, err
}

func (s *Store) countActiveRunsRaw(ctx context.Context, automationID string) (int, error) {
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM automation_runs WHERE automation_id = ? AND status = ?`,
		automationID, string(RunStatusTaskCreated))
	return count, err
}

// GetRun returns a single run by ID, or nil if not found.
func (s *Store) GetRun(ctx context.Context, id string) (*AutomationRun, error) {
	var r AutomationRun
	err := s.ro.GetContext(ctx, &r,
		`SELECT * FROM automation_runs WHERE id = ?`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	r.TriggerData = json.RawMessage(r.TriggerDataJSON)
	return &r, nil
}

// DeleteRun removes a single run row.
func (s *Store) DeleteRun(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM automation_runs WHERE id = ?`, id)
	return err
}

// ListRunTaskIDs returns all non-empty task_id values for an automation's runs.
// Used by DeleteAllRuns so the service can clean up tasks before purging rows.
func (s *Store) ListRunTaskIDs(ctx context.Context, automationID string) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids,
		`SELECT task_id FROM automation_runs WHERE automation_id = ? AND task_id != ''`,
		automationID)
	return ids, err
}

// DeleteAllRuns removes every run row for an automation.
func (s *Store) DeleteAllRuns(ctx context.Context, automationID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM automation_runs WHERE automation_id = ?`, automationID)
	return err
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
