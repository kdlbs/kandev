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
		task_mode TEXT NOT NULL DEFAULT 'automation_run',
		repository_mode TEXT NOT NULL DEFAULT 'none',
		repository_id TEXT NOT NULL DEFAULT '',
		prompt TEXT DEFAULT '',
		task_title_template TEXT DEFAULT '',
		-- execution_mode is retained so existing rows need no migration. The
		-- task/run choice is withdrawn: no firing path consults it. The one
		-- surviving reader is the migration notice, which derives a single
		-- boolean from it (see automationColumns). New rows are written with
		-- an explicit '' — the 'task' DEFAULT below only ever describes rows
		-- that predate the withdrawal.
		execution_mode TEXT NOT NULL DEFAULT 'task',
		enabled BOOLEAN DEFAULT 1,
		max_concurrent_runs INTEGER DEFAULT 1,
		continuation_policy TEXT NOT NULL DEFAULT 'new_task',
		continuation_task_id TEXT DEFAULT '',
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
		session_id TEXT DEFAULT '',
		turn_id TEXT DEFAULT '',
		thread_action TEXT DEFAULT '',
		thread_reason TEXT DEFAULT '',
		display_title TEXT DEFAULT '',
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
		base_branch TEXT NOT NULL DEFAULT '',
		position INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		UNIQUE(automation_id, repository_id)
	);

	CREATE INDEX IF NOT EXISTS idx_automation_repositories_automation ON automation_repositories(automation_id);

	CREATE TABLE IF NOT EXISTS automation_task_cleanup_jobs (
		task_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		automation_id TEXT NOT NULL,
		last_error TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_automation_task_cleanup_jobs_due
		ON automation_task_cleanup_jobs(updated_at, created_at);
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
//
// migrateExecutionModeSQL still runs because the notice derivation in
// automationColumns needs the column to exist on every DB it queries,
// including one initialised before the column was ever added.
const (
	migrateTaskTitleSQL          = `ALTER TABLE automations ADD COLUMN task_title_template TEXT DEFAULT ''`
	migrateExecutionModeSQL      = `ALTER TABLE automations ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'task'`
	migrateRepositoryIDSQL       = `ALTER TABLE automations ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`
	migrateContinuationPolicySQL = `ALTER TABLE automations ADD COLUMN continuation_policy TEXT NOT NULL DEFAULT 'new_task'`
	migrateContinuationTaskSQL   = `ALTER TABLE automations ADD COLUMN continuation_task_id TEXT DEFAULT ''`
	migrateTaskModeSQL           = `ALTER TABLE automations ADD COLUMN task_mode TEXT NOT NULL DEFAULT 'automation_run'`
	migrateRepositoryModeSQL     = `ALTER TABLE automations ADD COLUMN repository_mode TEXT NOT NULL DEFAULT 'none'`
	migrateRepositoryBranchSQL   = `ALTER TABLE automation_repositories ADD COLUMN base_branch TEXT NOT NULL DEFAULT ''`
	migrateRunSessionSQL         = `ALTER TABLE automation_runs ADD COLUMN session_id TEXT DEFAULT ''`
	migrateRunTurnSQL            = `ALTER TABLE automation_runs ADD COLUMN turn_id TEXT DEFAULT ''`
	migrateRunThreadActionSQL    = `ALTER TABLE automation_runs ADD COLUMN thread_action TEXT DEFAULT ''`
	migrateRunThreadReasonSQL    = `ALTER TABLE automation_runs ADD COLUMN thread_reason TEXT DEFAULT ''`
	migrateRunDisplayTitleSQL    = `ALTER TABLE automation_runs ADD COLUMN display_title TEXT DEFAULT ''`
)

// automationColumns is the explicit column list for every query that scans a
// full Automation row. Spelled out rather than `SELECT *` because the table
// carries columns the Automation struct does not: the legacy repository_id
// (superseded by automation_repositories — see the comment above
// migrateRepositoryIDSQL) and the withdrawn execution_mode, neither of which
// sqlx could map. sqlx runs in safe mode, so a returned column with no struct
// destination is a runtime error, not a compile-time one.
//
// execution_mode is read here, and only here, as the comparison
// `execution_mode = 'task'` aliased to legacy_board_card. The raw value is
// deliberately never selected: what the reader needs is the one bit "did this
// automation used to put a card on the board", for a migration notice that
// closes once. Projecting the mode itself would hand every future caller a
// mode to branch on, and the whole point of withdrawing it is that no firing
// path has one. See docs/specs/office/requirements/automations-settings.md § Migration.
const automationColumns = `id, workspace_id, name, description, workflow_id, workflow_step_id,
	agent_profile_id, executor_profile_id, task_mode, repository_mode, prompt, task_title_template,
	 enabled, max_concurrent_runs, continuation_policy, continuation_task_id, webhook_secret,
	 last_triggered_at, created_at, updated_at,
	execution_mode = 'task' AS legacy_board_card`

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	s.db.Exec(migrateTaskTitleSQL)          //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateExecutionModeSQL)      //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRepositoryIDSQL)       //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateContinuationPolicySQL) //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateContinuationTaskSQL)   //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateTaskModeSQL)           //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRepositoryModeSQL)     //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRepositoryBranchSQL)   //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRunSessionSQL)         //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRunTurnSQL)            //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRunThreadActionSQL)    //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRunThreadReasonSQL)    //nolint:errcheck // duplicate-column on existing DBs
	s.db.Exec(migrateRunDisplayTitleSQL)    //nolint:errcheck // duplicate-column on existing DBs
	if err := s.backfillLegacyRepositoryIDs(); err != nil {
		return err
	}
	return s.backfillRepositoryModes()
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

// backfillRepositoryModes derives the mode from the explicit list. Empty
// legacy workspace-default rows become repository-free; Kandev no longer
// attaches the first workspace repository implicitly.
func (s *Store) backfillRepositoryModes() error {
	if _, err := s.db.Exec(`
		UPDATE automations
		SET repository_mode = 'selected'
		WHERE repository_mode = 'workspace_default'
		  AND EXISTS (
			SELECT 1 FROM automation_repositories
			WHERE automation_repositories.automation_id = automations.id
		  )`); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE automations SET repository_mode = 'none' WHERE repository_mode = 'workspace_default'`)
	return err
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
	if a.ContinuationPolicy == "" {
		a.ContinuationPolicy = ContinuationPolicyNewTask
	}
	if a.TaskMode == "" {
		a.TaskMode = TaskModeAutomationRun
	}
	normalizeAutomationRepositories(a)
	if a.RepositoryMode == "" {
		if len(a.Repositories) > 0 {
			a.RepositoryMode = RepositoryModeSelected
		} else {
			a.RepositoryMode = RepositoryModeNone
		}
	}
	if err := validateAutomationTarget(a.TaskMode, a.RepositoryMode, a.WorkflowID, a.RepositoryIDs); err != nil {
		return err
	}
	if err := validateContinuationSettings(a.ContinuationPolicy, a.MaxConcurrentRuns); err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// execution_mode is written as the empty string rather than left to the
	// column's DEFAULT. Nothing reads the mode to decide anything — but the
	// DEFAULT is 'task', which is exactly the value the migration notice
	// treats as "this automation used to put a card on the board". Letting
	// the DEFAULT fill it would make every automation created from here on
	// indistinguishable from a pre-upgrade one, and the one-time notice would
	// never stop being true. Empty means "no mode was ever chosen", which is
	// the honest record for a row created after the choice was withdrawn.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO automations (id, workspace_id, name, description, workflow_id, workflow_step_id,
			agent_profile_id, executor_profile_id,
			task_mode, repository_mode, prompt, task_title_template, execution_mode,
			enabled, max_concurrent_runs, continuation_policy, continuation_task_id,
			webhook_secret, last_triggered_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.WorkspaceID, a.Name, a.Description, a.WorkflowID, a.WorkflowStepID,
		a.AgentProfileID, a.ExecutorProfileID,
		string(a.TaskMode), string(a.RepositoryMode),
		a.Prompt, a.TaskTitleTemplate,
		a.Enabled, a.MaxConcurrentRuns, a.ContinuationPolicy, a.ContinuationTaskID,
		a.WebhookSecret, a.LastTriggeredAt, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return err
	}
	if err := insertAutomationRepositories(ctx, tx, a.ID, a.Repositories); err != nil {
		return err
	}
	return tx.Commit()
}

// insertAutomationRepositories inserts one automation_repositories row per
// ID, preserving slice order via the position column. No-op for an empty
// slice. Shared by CreateAutomation and UpdateAutomation's replace path.
func insertAutomationRepositories(ctx context.Context, tx *sqlx.Tx, automationID string, repositories []AutomationRepository) error {
	now := time.Now().UTC()
	for i, repository := range repositories {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO automation_repositories (id, automation_id, repository_id, base_branch, position, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), automationID, repository.RepositoryID, repository.BaseBranch, i, now)
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
	repositories, err := s.listRepositoriesForAutomations(ctx, []string{id})
	if err != nil {
		return nil, fmt.Errorf("hydrate repository_ids: %w", err)
	}
	a.Repositories = repositories[id]
	a.RepositoryIDs = repositoryIDs(a.Repositories)
	return &a, nil
}

// listRepositoryIDsForAutomations batch-loads ordered repository_ids for
// several automations at once, mirroring listTriggersForAutomations.
func (s *Store) listRepositoriesForAutomations(ctx context.Context, automationIDs []string) (map[string][]AutomationRepository, error) {
	if len(automationIDs) == 0 {
		return make(map[string][]AutomationRepository), nil
	}
	query, args, err := sqlx.In(
		`SELECT automation_id, repository_id, base_branch FROM automation_repositories
		WHERE automation_id IN (?) ORDER BY automation_id, position`, automationIDs)
	if err != nil {
		return nil, err
	}
	query = s.ro.Rebind(query)
	type row struct {
		AutomationID string `db:"automation_id"`
		RepositoryID string `db:"repository_id"`
		BaseBranch   string `db:"base_branch"`
	}
	var rows []row
	if err := s.ro.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[string][]AutomationRepository, len(automationIDs))
	for _, id := range automationIDs {
		result[id] = []AutomationRepository{}
	}
	for _, r := range rows {
		result[r.AutomationID] = append(result[r.AutomationID], AutomationRepository{
			RepositoryID: r.RepositoryID,
			BaseBranch:   r.BaseBranch,
		})
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
	repositoriesByAutomation, err := s.listRepositoriesForAutomations(ctx, ids)
	if err != nil {
		return fmt.Errorf("hydrate repository_ids: %w", err)
	}
	for _, a := range automations {
		a.Triggers = triggersByAutomation[a.ID]
		a.Repositories = repositoriesByAutomation[a.ID]
		a.RepositoryIDs = repositoryIDs(a.Repositories)
	}
	return nil
}

// UpdateAutomation applies partial updates to an automation. When repository
// bindings or legacy repository IDs are non-nil, it atomically replaces the
// automation_repositories rows. An explicit non-selected repository mode also
// clears the rows when a partial client omits repository data.
func (s *Store) UpdateAutomation(ctx context.Context, id string, req *UpdateAutomationRequest) error {
	a, err := s.GetAutomation(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("automation not found: %s", id)
	}
	applyAutomationUpdate(a, req)
	if a.ContinuationPolicy == "" {
		a.ContinuationPolicy = ContinuationPolicyNewTask
	}
	if a.TaskMode == "" {
		a.TaskMode = TaskModeAutomationRun
	}
	normalizeAutomationRepositories(a)
	if a.RepositoryMode == "" {
		if len(a.Repositories) > 0 {
			a.RepositoryMode = RepositoryModeSelected
		} else {
			a.RepositoryMode = RepositoryModeNone
		}
	}
	if err := validateAutomationTarget(a.TaskMode, a.RepositoryMode, a.WorkflowID, a.RepositoryIDs); err != nil {
		return err
	}
	if a.ContinuationPolicy == ContinuationPolicyReuseThread && a.MaxConcurrentRuns <= 0 {
		a.MaxConcurrentRuns = 1
	}
	if err := validateContinuationSettings(a.ContinuationPolicy, a.MaxConcurrentRuns); err != nil {
		return err
	}
	a.UpdatedAt = time.Now().UTC()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		UPDATE automations SET name = ?, description = ?, workflow_id = ?, workflow_step_id = ?,
			agent_profile_id = ?, executor_profile_id = ?,
			task_mode = ?, repository_mode = ?, prompt = ?, task_title_template = ?,
			enabled = ?, max_concurrent_runs = ?, continuation_policy = ?, updated_at = ?
		WHERE id = ?`,
		a.Name, a.Description, a.WorkflowID, a.WorkflowStepID,
		a.AgentProfileID, a.ExecutorProfileID,
		string(a.TaskMode), string(a.RepositoryMode),
		a.Prompt, a.TaskTitleTemplate,
		a.Enabled, a.MaxConcurrentRuns, a.ContinuationPolicy, a.UpdatedAt, id)
	if err != nil {
		return err
	}
	if req.Repositories != nil || req.RepositoryIDs != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM automation_repositories WHERE automation_id = ?`, id); err != nil {
			return fmt.Errorf("clear automation_repositories: %w", err)
		}
		if err := insertAutomationRepositories(ctx, tx, id, a.Repositories); err != nil {
			return err
		}
	} else if req.RepositoryMode != nil && *req.RepositoryMode != RepositoryModeSelected {
		if _, err := tx.ExecContext(ctx, `DELETE FROM automation_repositories WHERE automation_id = ?`, id); err != nil {
			return fmt.Errorf("clear automation_repositories for repository mode: %w", err)
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
	if req.TaskMode != nil {
		a.TaskMode = *req.TaskMode
	}
	if req.RepositoryMode != nil {
		a.RepositoryMode = *req.RepositoryMode
		if a.RepositoryMode != RepositoryModeSelected && req.RepositoryIDs == nil && req.Repositories == nil {
			a.Repositories = nil
			a.RepositoryIDs = nil
		}
	}
	if req.Repositories != nil {
		a.Repositories = append([]AutomationRepository(nil), req.Repositories...)
		a.RepositoryIDs = repositoryIDs(a.Repositories)
		if req.RepositoryMode == nil {
			if len(a.Repositories) > 0 {
				a.RepositoryMode = RepositoryModeSelected
			} else {
				a.RepositoryMode = RepositoryModeNone
			}
		}
	}
	if req.RepositoryIDs != nil {
		a.RepositoryIDs = append([]string(nil), req.RepositoryIDs...)
		a.Repositories = repositoriesFromIDs(req.RepositoryIDs)
		if req.RepositoryMode == nil {
			if len(req.RepositoryIDs) > 0 {
				a.RepositoryMode = RepositoryModeSelected
			} else {
				a.RepositoryMode = RepositoryModeNone
			}
		}
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
	if req.ContinuationPolicy != nil {
		a.ContinuationPolicy = *req.ContinuationPolicy
	}
}

func normalizeAutomationRepositories(a *Automation) {
	if len(a.Repositories) == 0 && len(a.RepositoryIDs) > 0 {
		a.Repositories = repositoriesFromIDs(a.RepositoryIDs)
	}
	a.RepositoryIDs = repositoryIDs(a.Repositories)
}

func repositoriesFromIDs(ids []string) []AutomationRepository {
	result := make([]AutomationRepository, 0, len(ids))
	for _, id := range ids {
		result = append(result, AutomationRepository{RepositoryID: id})
	}
	return result
}

func repositoryIDs(repositories []AutomationRepository) []string {
	result := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		result = append(result, repository.RepositoryID)
	}
	return result
}

// DeleteAutomation removes an automation and its triggers/runs (CASCADE).
func (s *Store) DeleteAutomation(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM automations WHERE id = ?`, id)
	return err
}

// AutomationTaskCleanupJob is durable ownership for a hidden task whose
// automation references have already been removed. The job deliberately has
// no foreign key to the deleted automation: it must survive that deletion.
type AutomationTaskCleanupJob struct {
	TaskID       string    `db:"task_id" json:"task_id"`
	WorkspaceID  string    `db:"workspace_id" json:"workspace_id"`
	AutomationID string    `db:"automation_id" json:"automation_id"`
	LastError    string    `db:"last_error" json:"last_error,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// ListOpenRuns returns all admitted runs that still occupy an automation
// concurrency slot. It is intentionally unbounded and server-only: cleanup
// and startup reconciliation must not miss an old open run because a UI list
// limit was applied.
func (s *Store) ListOpenRuns(ctx context.Context, automationID string) ([]*AutomationRun, error) {
	var runs []*AutomationRun
	err := s.ro.SelectContext(ctx, &runs, `
		SELECT * FROM automation_runs
		WHERE automation_id = ? AND status IN (?, ?)
		ORDER BY created_at ASC, id ASC`,
		automationID, string(RunStatusTriggered), string(RunStatusTaskCreated))
	for _, run := range runs {
		run.TriggerData = json.RawMessage(run.TriggerDataJSON)
	}
	return runs, err
}

// ListAllOpenRuns is the startup-reconciliation view. It includes disabled
// automations because disabling a rule must not make an already-admitted run
// disappear from recovery.
func (s *Store) ListAllOpenRuns(ctx context.Context) ([]*AutomationRun, error) {
	var runs []*AutomationRun
	err := s.ro.SelectContext(ctx, &runs, `
		SELECT * FROM automation_runs
		WHERE status IN (?, ?)
		ORDER BY automation_id, created_at ASC, id ASC`,
		string(RunStatusTriggered), string(RunStatusTaskCreated))
	for _, run := range runs {
		run.TriggerData = json.RawMessage(run.TriggerDataJSON)
	}
	return runs, err
}

// DeleteAutomationWithCleanup captures every task owned only by the
// automation, inserts one durable cleanup job per task, and removes the
// automation and its cascading references in one transaction. The caller must
// stop live sessions before invoking this method.
type automationCleanupOwner struct {
	WorkspaceID        string `db:"workspace_id"`
	ContinuationTaskID string `db:"continuation_task_id"`
}

func (s *Store) DeleteAutomationWithCleanup(ctx context.Context, id string, cleanupTaskIDs ...[]string) ([]string, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	owner, found, err := loadAutomationCleanupOwner(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	var taskIDs []string
	if len(cleanupTaskIDs) > 0 {
		taskIDs = cleanupTaskIDs[0]
	} else {
		taskIDs, err = listAutomationCleanupTaskIDs(ctx, tx, id, owner.ContinuationTaskID)
	}
	if err != nil {
		return nil, err
	}
	jobs, err := insertAutomationCleanupJobs(ctx, tx, id, owner, taskIDs)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM automations WHERE id = ?`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return jobs, nil
}

// ListAutomationTaskIDs returns the distinct task IDs currently owned by an
// automation's runs plus its continuation pointer. The service uses the task
// origin to keep visible normal tasks out of hidden-task cleanup jobs.
func (s *Store) ListAutomationTaskIDs(ctx context.Context, id string) ([]string, error) {
	var taskIDs []string
	if err := s.ro.SelectContext(ctx, &taskIDs,
		`SELECT DISTINCT task_id FROM automation_runs WHERE automation_id = ? AND task_id != ''`, id); err != nil {
		return nil, err
	}
	var continuationTaskID string
	if err := s.ro.GetContext(ctx, &continuationTaskID,
		`SELECT continuation_task_id FROM automations WHERE id = ?`, id); err == nil && continuationTaskID != "" {
		taskIDs = append(taskIDs, continuationTaskID)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return taskIDs, nil
}

func loadAutomationCleanupOwner(ctx context.Context, tx *sqlx.Tx, id string) (automationCleanupOwner, bool, error) {
	var owner automationCleanupOwner
	err := tx.GetContext(ctx, &owner,
		`SELECT workspace_id, continuation_task_id FROM automations WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return owner, false, nil
	}
	return owner, err == nil, err
}

func listAutomationCleanupTaskIDs(ctx context.Context, tx *sqlx.Tx, id, continuationTaskID string) ([]string, error) {
	var taskIDs []string
	if err := tx.SelectContext(ctx, &taskIDs,
		`SELECT DISTINCT task_id FROM automation_runs WHERE automation_id = ? AND task_id != ''`, id); err != nil {
		return nil, err
	}
	if continuationTaskID != "" {
		taskIDs = append(taskIDs, continuationTaskID)
	}
	return taskIDs, nil
}

func insertAutomationCleanupJobs(
	ctx context.Context,
	tx *sqlx.Tx,
	automationID string,
	owner automationCleanupOwner,
	taskIDs []string,
) ([]string, error) {
	seen := make(map[string]struct{}, len(taskIDs))
	jobs := make([]string, 0, len(taskIDs))
	now := time.Now().UTC()
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		owned, err := automationCleanupTaskIsUnreferenced(ctx, tx, automationID, taskID)
		if err != nil {
			return nil, err
		}
		if !owned {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO automation_task_cleanup_jobs
				(task_id, workspace_id, automation_id, last_error, created_at, updated_at)
			VALUES (?, ?, ?, '', ?, ?)
			ON CONFLICT(task_id) DO UPDATE SET
				workspace_id = excluded.workspace_id,
				automation_id = excluded.automation_id,
				updated_at = excluded.updated_at`,
			taskID, owner.WorkspaceID, automationID, now, now); err != nil {
			return nil, err
		}
		jobs = append(jobs, taskID)
	}
	return jobs, nil
}

func automationCleanupTaskIsUnreferenced(ctx context.Context, tx *sqlx.Tx, automationID, taskID string) (bool, error) {
	var refs int
	if err := tx.GetContext(ctx, &refs, `
		SELECT COUNT(*) FROM automation_runs
		WHERE task_id = ? AND automation_id != ?`, taskID, automationID); err != nil {
		return false, err
	}
	if refs > 0 {
		return false, nil
	}
	var pointers int
	if err := tx.GetContext(ctx, &pointers, `
		SELECT COUNT(*) FROM automations
		WHERE continuation_task_id = ? AND id != ?`, taskID, automationID); err != nil {
		return false, err
	}
	return pointers == 0, nil
}

// IsTaskReferencedByOtherAutomation reports whether a task is still owned by
// another run or continuation pointer. The caller holds the relevant
// automation admission lock; this protects run/delete-all races within one
// automation while the query protects cross-automation sharing.
func (s *Store) IsTaskReferencedByOtherAutomation(ctx context.Context, automationID, taskID string) (bool, error) {
	if taskID == "" {
		return false, nil
	}
	var refs int
	err := s.ro.GetContext(ctx, &refs, `
		SELECT COUNT(*) FROM automation_runs
		WHERE task_id = ? AND automation_id != ?`, taskID, automationID)
	if err != nil {
		return false, err
	}
	if refs > 0 {
		return true, nil
	}
	var pointers int
	err = s.ro.GetContext(ctx, &pointers, `
		SELECT COUNT(*) FROM automations
		WHERE continuation_task_id = ? AND id != ?`, taskID, automationID)
	return pointers > 0, err
}

// IsTaskReferencedByRun reports whether another run in the same automation
// owns the task. It is used when deleting one run from a shared thread.
func (s *Store) IsTaskReferencedByRun(ctx context.Context, automationID, runID, taskID string) (bool, error) {
	if taskID == "" {
		return false, nil
	}
	var count int
	err := s.ro.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM automation_runs
		WHERE automation_id = ? AND id != ? AND task_id = ?`, automationID, runID, taskID)
	return count > 0, err
}

// IsContinuationTask reports whether the automation currently protects the
// task from run-row deletion.
func (s *Store) IsContinuationTask(ctx context.Context, automationID, taskID string) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM automations WHERE id = ? AND continuation_task_id = ?`, automationID, taskID)
	return count > 0, err
}

func (s *Store) ListCleanupJobs(ctx context.Context) ([]*AutomationTaskCleanupJob, error) {
	var jobs []*AutomationTaskCleanupJob
	err := s.ro.SelectContext(ctx, &jobs,
		`SELECT * FROM automation_task_cleanup_jobs ORDER BY created_at ASC, task_id ASC`)
	return jobs, err
}

func (s *Store) DeleteCleanupJob(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM automation_task_cleanup_jobs WHERE task_id = ?`, taskID)
	return err
}

func (s *Store) UpdateCleanupJobError(ctx context.Context, taskID, message string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE automation_task_cleanup_jobs SET last_error = ?, updated_at = ? WHERE task_id = ?`,
		message, time.Now().UTC(), taskID)
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
		ORDER BY t.created_at ASC, t.id ASC`, string(triggerType))
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
			dedup_key, trigger_data, error_message, session_id, turn_id, thread_action, thread_reason,
			display_title, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.AutomationID, r.TriggerID, r.TriggerType, r.TaskID, r.Status,
		r.DedupKey, r.TriggerDataJSON, r.ErrorMessage, r.SessionID, r.TurnID,
		r.ThreadAction, r.ThreadReason, r.DisplayTitle, r.CreatedAt)
	return err
}

// AdoptTriggeredRun upgrades the admission row for callers that still report
// an outcome through RecordRun without carrying the new run ID. It is keyed by
// the dedup key and only touches an unbound triggered row, so it cannot rewrite
// a different firing or a terminal history row.
func (s *Store) AdoptTriggeredRun(ctx context.Context, r *AutomationRun) (bool, error) {
	if r == nil || r.DedupKey == "" {
		return false, nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE automation_runs SET task_id = ?, status = ?, trigger_data = ?, error_message = ?,
			session_id = ?, turn_id = ?, thread_action = ?, thread_reason = ?, display_title = ?
		WHERE id = (
			SELECT id FROM automation_runs
			WHERE automation_id = ? AND dedup_key = ? AND status = ?
			ORDER BY created_at DESC, id DESC LIMIT 1
		) AND status = ?`,
		r.TaskID, r.Status, r.TriggerDataJSON, r.ErrorMessage, r.SessionID, r.TurnID,
		r.ThreadAction, r.ThreadReason, r.DisplayTitle,
		r.AutomationID, r.DedupKey, string(RunStatusTriggered), string(RunStatusTriggered))
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// BindRunTask records the task created for an admitted run while leaving the
// run in triggered state. This closes the task ownership gap before the agent
// session is ready, so a failed launch can still settle the exact run.
func (s *Store) BindRunTask(ctx context.Context, runID, taskID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE automation_runs SET task_id = ?
		WHERE id = ? AND status = ?`, taskID, runID, string(RunStatusTriggered))
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return fmt.Errorf("automation run %s is not an admitted triggered run", runID)
	}
	return nil
}

func (s *Store) SetContinuationTaskID(ctx context.Context, automationID, taskID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE automations SET continuation_task_id = ?, updated_at = ? WHERE id = ?`,
		taskID, time.Now().UTC(), automationID)
	return err
}

// BindRun attaches the exact session and turn after dispatch and makes the run
// task_created. The identity is kept on the row so shared-task completion can
// never settle a different firing.
func (s *Store) BindRun(ctx context.Context, runID, taskID, sessionID, turnID string, action ThreadAction, reason string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE automation_runs
		SET task_id = ?, session_id = ?, turn_id = ?, thread_action = ?, thread_reason = ?, status = ?
		WHERE id = ? AND status IN (?, ?)`,
		taskID, sessionID, turnID, action, reason, string(RunStatusTaskCreated),
		runID, string(RunStatusTriggered), string(RunStatusTaskCreated))
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return fmt.Errorf("automation run %s is not bindable", runID)
	}
	return nil
}

// MarkRunTerminal settles one exact run. Empty session/turn arguments are
// allowed only for a pre-dispatch failure; once both are known they must match
// the persisted binding.
func (s *Store) MarkRunTerminal(ctx context.Context, runID, sessionID, turnID string, status RunStatus, errMsg string) error {
	query := `UPDATE automation_runs SET status = ?, error_message = ? WHERE id = ? AND status IN (?, ?)`
	args := []any{string(status), errMsg, runID, string(RunStatusTriggered), string(RunStatusTaskCreated)}
	if sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	if turnID != "" {
		query += ` AND turn_id = ?`
		args = append(args, turnID)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return fmt.Errorf("automation run %s does not match the requested terminal binding", runID)
	}
	return nil
}

// MarkRunTerminalByBinding settles a run using the exact task/session/turn
// triple observed by lifecycle completion. It is used by shared tasks where
// several runs intentionally point at one task and session.
func (s *Store) MarkRunTerminalByBinding(ctx context.Context, taskID, sessionID, turnID string, status RunStatus, errMsg string) error {
	if taskID == "" || sessionID == "" || turnID == "" {
		return fmt.Errorf("exact automation run binding is required")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE automation_runs SET status = ?, error_message = ?
		WHERE task_id = ? AND session_id = ? AND turn_id = ? AND status = ?`,
		string(status), errMsg, taskID, sessionID, turnID, string(RunStatusTaskCreated))
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return fmt.Errorf("no open automation run matches task/session/turn binding")
	}
	return nil
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
	if limit > maxRunsLimit {
		limit = maxRunsLimit
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
	if err := s.hydrateRunSummaries(ctx, runs); err != nil {
		return nil, err
	}
	return runs, nil
}

// maxRunsLimit caps the per-automation run history for the same reason the
// workspace feed is capped: a client should not be able to pull unbounded
// historical transcript metadata in one request.
const maxRunsLimit = 200

// runSummaryRow is the newest agent message for one exact turn.
type runSummaryRow struct {
	TurnID  string         `db:"turn_id"`
	Summary sql.NullString `db:"summary"`
}

// hydrateRunSummaries replaces the task-level fallback summary with the
// message from the exact turn bound to a run. Reused tasks can contain several
// turns, so looking only at task_id can show a later firing's result for an
// earlier run. Runs without a turn binding retain the legacy task-level query.
// Bound runs are hydrated in one set-based query because these lists are also
// used by live polling and can contain hundreds of runs.
func (s *Store) hydrateRunSummaries(ctx context.Context, runs []*AutomationRun) error {
	turnIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		if run.TurnID != "" {
			turnIDs = append(turnIDs, run.TurnID)
		}
	}
	if len(turnIDs) == 0 {
		return nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(turnIDs)), ",")
	args := make([]any, len(turnIDs))
	for i, turnID := range turnIDs {
		args[i] = turnID
	}
	var summaries []runSummaryRow
	err := s.ro.SelectContext(ctx, &summaries, `
		SELECT m.turn_id, substr(m.content, 1, 280) AS summary
		FROM task_session_messages m
		WHERE m.turn_id IN (`+placeholders+`)
		  AND m.author_type = 'agent' AND m.type = 'message'
		  AND NOT EXISTS (
			SELECT 1 FROM task_session_messages newer
			WHERE newer.turn_id = m.turn_id
			  AND newer.author_type = 'agent' AND newer.type = 'message'
			  AND (newer.created_at > m.created_at OR
				(newer.created_at = m.created_at AND newer.id > m.id))
		  )`, args...)
	if db.IsMissingTableError(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, run := range runs {
		if run.TurnID != "" {
			run.Summary = ""
		}
	}
	byTurn := make(map[string]string, len(summaries))
	for _, summary := range summaries {
		if summary.Summary.Valid {
			byTurn[summary.TurnID] = summary.Summary.String
		}
	}
	for _, run := range runs {
		if summary, ok := byTurn[run.TurnID]; ok {
			run.Summary = summary
		}
	}
	return nil
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
		ar.dedup_key, ar.trigger_data, ar.error_message,
		ar.session_id, ar.turn_id, ar.thread_action, ar.thread_reason, ar.display_title,
		ar.created_at,
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
		COALESCE(NULLIF(ar.session_id, ''), (
			SELECT ts.id FROM task_sessions ts
			WHERE ts.task_id = ar.task_id AND ts.is_primary = 1
			LIMIT 1
		), ar.session_id, '') AS session_id`

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
	if err := s.hydrateWorkspaceRunSummaries(ctx, runs); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *Store) hydrateWorkspaceRunSummaries(ctx context.Context, runs []*WorkspaceAutomationRun) error {
	plain := make([]*AutomationRun, len(runs))
	for i := range runs {
		plain[i] = &runs[i].AutomationRun
	}
	return s.hydrateRunSummaries(ctx, plain)
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
	boundRuns := make([]*AutomationRun, len(rows))
	for i := range rows {
		boundRuns[i] = &rows[i].AutomationRun
	}
	if err := s.hydrateRunSummaries(ctx, boundRuns); err != nil {
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
			WHERE aro.automation_id = ar.automation_id AND aro.status IN (?, ?)
		) AS open_runs
		FROM automation_runs ar
		JOIN automations a ON a.id = ar.automation_id
		WHERE `+scope+` AND `+latestRunPerAutomationSQL,
		string(RunStatusTriggered), string(RunStatusTaskCreated), arg)
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

const openRunPredicateSQL = `(
		ar.status = ?
		OR (ar.status = ? AND t.id IS NOT NULL AND t.archived_at IS NULL
			AND NOT EXISTS (
			SELECT 1 FROM task_sessions ts
			WHERE ts.task_id = ar.task_id AND ts.is_primary = 1 AND ts.state = ?
			)
		))`

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
	return []any{
		string(RunStatusTriggered), string(RunStatusTaskCreated),
		string(taskmodels.TaskSessionStateCancelled),
	}
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
		`SELECT COUNT(*) FROM automation_runs WHERE automation_id = ? AND status IN (?, ?)`,
		automationID, string(RunStatusTriggered), string(RunStatusTaskCreated))
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
		`SELECT DISTINCT task_id FROM automation_runs WHERE automation_id = ? AND task_id != ''`,
		automationID)
	return ids, err
}

// DeleteAllRuns removes every run row for an automation.
func (s *Store) DeleteAllRuns(ctx context.Context, automationID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM automation_runs WHERE automation_id = ?`, automationID)
	return err
}

// ClearContinuationAndDeleteAllRuns atomically releases the reusable task
// pointer with the run rows it protects. The service performs task cleanup
// before calling this method, while holding the automation admission lock.
func (s *Store) ClearContinuationAndDeleteAllRuns(ctx context.Context, automationID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`UPDATE automations SET continuation_task_id = '', updated_at = ? WHERE id = ?`, time.Now().UTC(), automationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM automation_runs WHERE automation_id = ?`, automationID); err != nil {
		return err
	}
	return tx.Commit()
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
