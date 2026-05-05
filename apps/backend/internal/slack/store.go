package slack

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store persists the install-wide Slack configuration. Secret values (xoxc
// token, d cookie) live in the shared encrypted secrets store, not here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB

	// migratedFromWorkspace records the workspace_id of the row that was
	// promoted into the singleton during initSchema. Provider reads this to
	// migrate the per-workspace secrets to the new global keys. Empty when
	// no migration ran.
	migratedFromWorkspace string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("slack schema init: %w", err)
	}
	return s, nil
}

// MigratedFromWorkspace returns the workspace_id of the row promoted to the
// singleton during the per-workspace → singleton schema migration, or "" when
// no migration ran.
func (s *Store) MigratedFromWorkspace() string {
	return s.migratedFromWorkspace
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS slack_configs (
		id TEXT PRIMARY KEY CHECK(id = 'singleton'),
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

// singletonID is the synthetic primary key of the (only) row in slack_configs.
const singletonID = "singleton"

func (s *Store) initSchema() error {
	if err := s.migrateLegacyPerWorkspaceTable(); err != nil {
		return err
	}
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	return nil
}

// migrateLegacyPerWorkspaceTable detects the pre-singleton schema (where
// slack_configs was keyed by workspace_id) and rewrites it into the singleton
// shape. Picks the most-recently-updated row and records the source
// workspace_id so the provider can migrate the secrets.
func (s *Store) migrateLegacyPerWorkspaceTable() error {
	cols, err := s.tableColumns()
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, hasWorkspace := cols["workspace_id"]; !hasWorkspace {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Build the SELECT against only columns present in this database — the
	// per-workspace shape evolved across patches (utility_agent_id,
	// poll_interval_seconds, the slack_team/slack_user/last_seen trio, and
	// the auth-health trio were all added incrementally), so a deployment
	// upgrading from any earlier intermediate would crash on a SELECT that
	// assumed every column. Mirrors the Linear/Jira fix.
	selectCols := "workspace_id, auth_method"
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

	var sourceWorkspace, authMethod, commandPrefix, utilityAgentID sql.NullString
	var pollIntervalSeconds sql.NullInt64
	var slackTeamID, slackUserID, lastSeenTS, lastError sql.NullString
	var lastCheckedAt sql.NullTime
	var lastOk sql.NullInt64
	var createdAt, updatedAt sql.NullTime
	row := tx.QueryRow(`SELECT ` + selectCols + ` FROM slack_configs ORDER BY updated_at DESC, workspace_id DESC LIMIT 1`)
	switch err := row.Scan(&sourceWorkspace, &authMethod,
		&commandPrefix, &utilityAgentID, &pollIntervalSeconds,
		&slackTeamID, &slackUserID, &lastSeenTS,
		&lastCheckedAt, &lastOk, &lastError,
		&createdAt, &updatedAt); {
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
	pollSeconds := int(pollIntervalSeconds.Int64)
	if pollSeconds == 0 {
		pollSeconds = DefaultPollIntervalSeconds
	}
	if _, err := tx.Exec(`
		INSERT INTO slack_configs (
			id, auth_method, command_prefix, utility_agent_id, poll_interval_seconds,
			slack_team_id, slack_user_id, last_seen_ts,
			last_checked_at, last_ok, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		singletonID, authMethod.String, commandPrefix.String, utilityAgentID.String, pollSeconds,
		slackTeamID.String, slackUserID.String, lastSeenTS.String,
		nullableTime(lastCheckedAt), lastOk.Int64, lastError.String,
		nullableTime(createdAt), nullableTime(updatedAt)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.migratedFromWorkspace = sourceWorkspace.String
	return nil
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

const selectConfigColumns = `auth_method, command_prefix, utility_agent_id,
		poll_interval_seconds,
		slack_team_id, slack_user_id, last_seen_ts,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the singleton Slack config, or nil when no row exists.
func (s *Store) GetConfig(ctx context.Context) (*SlackConfig, error) {
	var cfg SlackConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM slack_configs WHERE id = ?`, singletonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the singleton row. Health columns
// (last_*) and watermark columns (last_seen_ts, slack_team_id, slack_user_id)
// are owned by the poller / trigger and aren't touched here.
func (s *Store) UpsertConfig(ctx context.Context, cfg *SlackConfig) error {
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO slack_configs (
			id, auth_method,
			command_prefix, utility_agent_id, poll_interval_seconds,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			auth_method = excluded.auth_method,
			command_prefix = excluded.command_prefix,
			utility_agent_id = excluded.utility_agent_id,
			poll_interval_seconds = excluded.poll_interval_seconds,
			updated_at = excluded.updated_at`,
		singletonID, cfg.AuthMethod,
		cfg.CommandPrefix, cfg.UtilityAgentID, cfg.PollIntervalSeconds,
		cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the singleton row. Secrets must be cleared separately.
func (s *Store) DeleteConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM slack_configs WHERE id = ?`, singletonID)
	return err
}

// HasConfig reports whether the singleton row exists. Used by the auth-health
// poller to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	err := s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM slack_configs WHERE id = ?`, singletonID)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// UpdateAuthHealth records the result of a credential probe and (when
// supplied) the user/team identifiers captured during the same probe so the
// trigger can scope its searches without an extra round-trip.
func (s *Store) UpdateAuthHealth(ctx context.Context, ok bool, errMsg, teamID, userID string, checkedAt time.Time) error {
	if teamID != "" && userID != "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE slack_configs
			SET last_checked_at = ?, last_ok = ?, last_error = ?,
				slack_team_id = ?, slack_user_id = ?
			WHERE id = ?`,
			checkedAt, ok, errMsg, teamID, userID, singletonID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE slack_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE id = ?`,
		checkedAt, ok, errMsg, singletonID)
	return err
}

// UpdateLastSeenTS advances the trigger's watermark.
func (s *Store) UpdateLastSeenTS(ctx context.Context, ts string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE slack_configs SET last_seen_ts = ? WHERE id = ?`,
		ts, singletonID)
	return err
}
