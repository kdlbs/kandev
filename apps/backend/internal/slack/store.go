package slack

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/integrations/workspacescope"
)

// Store persists workspace-scoped Slack configuration. Secret values (xoxc
// token, d cookie) live in the shared encrypted secrets store, not here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB

	defaultWorkspace workspacescope.DefaultResolver

	// migratedToWorkspace records the workspace_id that received a legacy
	// singleton row during initSchema. Provider reads this to migrate the
	// singleton secrets to the workspace-scoped keys. Empty when no migration ran.
	migratedToWorkspace string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("slack schema init: %w", err)
	}
	return s, nil
}

// MigratedFromWorkspace is kept for older provider call sites. It now returns
// the workspace_id that received a legacy singleton row during migration.
func (s *Store) MigratedFromWorkspace() string {
	return s.migratedToWorkspace
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS slack_configs (
		workspace_id TEXT PRIMARY KEY,
		auth_method TEXT NOT NULL,
		command_prefix TEXT NOT NULL DEFAULT '',
		utility_agent_id TEXT NOT NULL DEFAULT '',
		poll_interval_seconds INTEGER NOT NULL DEFAULT 30,
		slack_team_id TEXT NOT NULL DEFAULT '',
		slack_user_id TEXT NOT NULL DEFAULT '',
		last_seen_ts TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
`

// singletonID is the synthetic primary key used by the legacy install-wide
// slack_configs table.
const singletonID = "singleton"

func (s *Store) initSchema() error {
	if err := s.migrateLegacySingletonTable(); err != nil {
		return err
	}
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := s.addConfigColumns(); err != nil {
		return err
	}
	return nil
}

func (s *Store) addConfigColumns() error {
	cols, err := s.tableColumns()
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	for _, col := range []struct {
		name string
		sql  string
	}{
		{"command_prefix", `ALTER TABLE slack_configs ADD COLUMN command_prefix TEXT NOT NULL DEFAULT ''`},
		{"utility_agent_id", `ALTER TABLE slack_configs ADD COLUMN utility_agent_id TEXT NOT NULL DEFAULT ''`},
		{"poll_interval_seconds", `ALTER TABLE slack_configs ADD COLUMN poll_interval_seconds INTEGER NOT NULL DEFAULT 30`},
		{"slack_team_id", `ALTER TABLE slack_configs ADD COLUMN slack_team_id TEXT NOT NULL DEFAULT ''`},
		{"slack_user_id", `ALTER TABLE slack_configs ADD COLUMN slack_user_id TEXT NOT NULL DEFAULT ''`},
		{"last_seen_ts", `ALTER TABLE slack_configs ADD COLUMN last_seen_ts TEXT NOT NULL DEFAULT ''`},
		{"last_checked_at", `ALTER TABLE slack_configs ADD COLUMN last_checked_at DATETIME`},
		{"last_ok", `ALTER TABLE slack_configs ADD COLUMN last_ok INTEGER NOT NULL DEFAULT 0`},
		{"last_error", `ALTER TABLE slack_configs ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`},
	} {
		if _, ok := cols[col.name]; !ok {
			if _, err := s.db.Exec(col.sql); err != nil {
				return fmt.Errorf("add %s column: %w", col.name, err)
			}
		}
	}
	return nil
}

// migrateLegacySingletonTable detects the install-wide singleton schema and
// rewrites it into the workspace-scoped shape. Existing workspace-keyed tables
// are already canonical and are left in place.
func (s *Store) migrateLegacySingletonTable() error {
	cols, err := s.tableColumns()
	if err != nil {
		return err
	}
	if !isLegacySingletonConfig(cols) {
		return nil
	}
	targetWorkspace, err := workspacescope.ResolveMigrationTarget(s.db)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	cfg, err := scanLegacySingletonConfig(tx, cols)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`DROP TABLE slack_configs`); err != nil {
			return err
		}
		return tx.Commit()
	case err != nil:
		return err
	}
	if _, err := tx.Exec(`DROP TABLE slack_configs`); err != nil {
		return err
	}
	if _, err := tx.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := insertMigratedConfig(tx, targetWorkspace, cfg); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.migratedToWorkspace = targetWorkspace
	return nil
}

func isLegacySingletonConfig(cols map[string]struct{}) bool {
	if len(cols) == 0 {
		return false
	}
	if _, hasWorkspace := cols["workspace_id"]; hasWorkspace {
		return false
	}
	_, hasID := cols["id"]
	return hasID
}

type legacySlackConfig struct {
	authMethod           sql.NullString
	commandPrefix        sql.NullString
	utilityAgentID       sql.NullString
	pollIntervalSeconds  sql.NullInt64
	slackTeamID          sql.NullString
	slackUserID          sql.NullString
	lastSeenTS           sql.NullString
	lastCheckedAt        sql.NullTime
	lastOk               sql.NullInt64
	lastError            sql.NullString
	createdAt, updatedAt sql.NullTime
}

func scanLegacySingletonConfig(tx *sql.Tx, cols map[string]struct{}) (*legacySlackConfig, error) {
	// Build the SELECT against only columns present in this database — the
	// per-workspace shape evolved across patches, so upgrades from earlier
	// intermediates need defaults for missing columns.
	selectCols := "auth_method"
	selectCols += pickCol(cols, "command_prefix", "''")
	selectCols += pickCol(cols, "utility_agent_id", "''")
	selectCols += pickCol(cols, "poll_interval_seconds", "30")
	selectCols += pickCol(cols, "slack_team_id", "''")
	selectCols += pickCol(cols, "slack_user_id", "''")
	selectCols += pickCol(cols, "last_seen_ts", "''")
	selectCols += pickCol(cols, "last_checked_at", "NULL")
	selectCols += pickCol(cols, "last_ok", "0")
	selectCols += pickCol(cols, "last_error", "''")
	selectCols += ", created_at, updated_at"

	cfg := &legacySlackConfig{}
	row := tx.QueryRow(`SELECT `+selectCols+` FROM slack_configs WHERE id = ? LIMIT 1`, singletonID)
	err := row.Scan(&cfg.authMethod,
		&cfg.commandPrefix, &cfg.utilityAgentID, &cfg.pollIntervalSeconds,
		&cfg.slackTeamID, &cfg.slackUserID, &cfg.lastSeenTS,
		&cfg.lastCheckedAt, &cfg.lastOk, &cfg.lastError,
		&cfg.createdAt, &cfg.updatedAt)
	return cfg, err
}

func insertMigratedConfig(tx *sql.Tx, workspaceID string, cfg *legacySlackConfig) error {
	pollSeconds := int(cfg.pollIntervalSeconds.Int64)
	if pollSeconds == 0 {
		pollSeconds = DefaultPollIntervalSeconds
	}
	_, err := tx.Exec(`
		INSERT INTO slack_configs (
			workspace_id, auth_method, command_prefix, utility_agent_id, poll_interval_seconds,
			slack_team_id, slack_user_id, last_seen_ts,
			last_checked_at, last_ok, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workspaceID, cfg.authMethod.String, cfg.commandPrefix.String, cfg.utilityAgentID.String, pollSeconds,
		cfg.slackTeamID.String, cfg.slackUserID.String, cfg.lastSeenTS.String,
		nullableTime(cfg.lastCheckedAt), cfg.lastOk.Int64, cfg.lastError.String,
		nullableTime(cfg.createdAt), nullableTime(cfg.updatedAt))
	return err
}

// pickCol returns ", <name>" if the column is present in the legacy schema
// or ", <fallback> AS <name>" otherwise. Lets the migration SELECT defaults
// for columns that didn't exist in earlier intermediate schemas.
func pickCol(cols map[string]struct{}, name, fallback string) string {
	if _, ok := cols[name]; ok {
		return ", " + name
	}
	return ", " + fallback + " AS " + name
}

func nullableTime(t sql.NullTime) interface{} {
	if !t.Valid {
		return nil
	}
	return t.Time
}

// tableColumns returns the column-name set for slack_configs. SQLite doesn't
// support parameterised identifiers in PRAGMA, and the only table this
// migration logic ever inspects is slack_configs, so the table name is
// inlined as a literal.
func (s *Store) tableColumns() (map[string]struct{}, error) {
	rows, err := s.db.Query(`PRAGMA table_info(slack_configs)`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols := make(map[string]struct{})
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = struct{}{}
	}
	return cols, rows.Err()
}

const selectConfigColumns = `workspace_id, auth_method, command_prefix, utility_agent_id,
		poll_interval_seconds,
		slack_team_id, slack_user_id, last_seen_ts,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the default workspace Slack config, or nil when no row
// exists. New code should call GetConfigForWorkspace.
func (s *Store) GetConfig(ctx context.Context) (*SlackConfig, error) {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	return s.GetConfigForWorkspace(ctx, workspaceID)
}

// GetConfigForWorkspace returns the Slack config for a workspace, or nil when
// no row exists.
func (s *Store) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*SlackConfig, error) {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	var cfg SlackConfig
	err = s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM slack_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the default workspace row. Health columns
// (last_*) and watermark columns (last_seen_ts, slack_team_id, slack_user_id)
// are owned by the poller / trigger and aren't touched here.
func (s *Store) UpsertConfig(ctx context.Context, cfg *SlackConfig) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.UpsertConfigForWorkspace(ctx, workspaceID, cfg)
}

// UpsertConfigForWorkspace inserts or updates a workspace row.
func (s *Store) UpsertConfigForWorkspace(ctx context.Context, workspaceID string, cfg *SlackConfig) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO slack_configs (
			workspace_id, auth_method,
			command_prefix, utility_agent_id, poll_interval_seconds,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			auth_method = excluded.auth_method,
			command_prefix = excluded.command_prefix,
			utility_agent_id = excluded.utility_agent_id,
			poll_interval_seconds = excluded.poll_interval_seconds,
			updated_at = excluded.updated_at`,
		workspaceID, cfg.AuthMethod,
		cfg.CommandPrefix, cfg.UtilityAgentID, cfg.PollIntervalSeconds,
		cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the default workspace row. Secrets must be cleared separately.
func (s *Store) DeleteConfig(ctx context.Context) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.DeleteConfigForWorkspace(ctx, workspaceID)
}

// DeleteConfigForWorkspace removes the workspace row. Secrets must be cleared separately.
func (s *Store) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM slack_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// HasConfig reports whether any config row exists. Used by the auth-health
// poller to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	err := s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM slack_configs`)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// ListConfigWorkspaceIDs returns every workspace with a saved Slack config.
func (s *Store) ListConfigWorkspaceIDs(ctx context.Context) ([]string, error) {
	var ids []string
	if err := s.ro.SelectContext(ctx, &ids, `SELECT workspace_id FROM slack_configs ORDER BY workspace_id`); err != nil {
		return nil, err
	}
	return ids, nil
}

// ListConfigs returns every Slack config row ordered by workspace.
func (s *Store) ListConfigs(ctx context.Context) ([]*SlackConfig, error) {
	var configs []*SlackConfig
	if err := s.ro.SelectContext(ctx, &configs, `SELECT `+selectConfigColumns+` FROM slack_configs ORDER BY workspace_id`); err != nil {
		return nil, err
	}
	return configs, nil
}

// UpdateAuthHealth records the result of a credential probe and (when
// supplied) the user/team identifiers captured during the same probe so the
// trigger can scope its searches without an extra round-trip.
func (s *Store) UpdateAuthHealth(ctx context.Context, ok bool, errMsg, teamID, userID string, checkedAt time.Time) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.UpdateAuthHealthForWorkspace(ctx, workspaceID, ok, errMsg, teamID, userID, checkedAt)
}

// UpdateAuthHealthForWorkspace records the result of a credential probe for one workspace.
func (s *Store) UpdateAuthHealthForWorkspace(ctx context.Context, workspaceID string, ok bool, errMsg, teamID, userID string, checkedAt time.Time) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	if teamID != "" && userID != "" {
		_, err = s.db.ExecContext(ctx, `
			UPDATE slack_configs
			SET last_checked_at = ?, last_ok = ?, last_error = ?,
				slack_team_id = ?, slack_user_id = ?
			WHERE workspace_id = ?`,
			checkedAt, ok, errMsg, teamID, userID, workspaceID)
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE slack_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE workspace_id = ?`,
		checkedAt, ok, errMsg, workspaceID)
	return err
}

// UpdateLastSeenTS advances the trigger's watermark.
func (s *Store) UpdateLastSeenTS(ctx context.Context, ts string) error {
	workspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return err
	}
	return s.UpdateLastSeenTSForWorkspace(ctx, workspaceID, ts)
}

// UpdateLastSeenTSForWorkspace advances one workspace trigger watermark.
func (s *Store) UpdateLastSeenTSForWorkspace(ctx context.Context, workspaceID, ts string) error {
	workspaceID, err := s.resolveWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE slack_configs SET last_seen_ts = ? WHERE workspace_id = ?`,
		ts, workspaceID)
	return err
}

func (s *Store) defaultWorkspaceID() (string, error) {
	return s.defaultWorkspace.Resolve(s.db)
}

func (s *Store) resolveWorkspaceID(workspaceID string) (string, error) {
	if workspaceID != "" {
		return workspaceID, nil
	}
	return s.defaultWorkspaceID()
}
