package sentry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/integrations/workspacescope"
)

// Store persists workspace-scoped Sentry instances. Each instance's secret
// token is delegated to the shared encrypted secret store and not stored here.
type Store struct {
	db                  *sqlx.DB
	ro                  *sqlx.DB
	migratedToWorkspace string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("sentry schema init: %w", err)
	}
	return s, nil
}

// sentryConfigsColumns is the id-keyed multi-instance sentry_configs column
// list, shared by createTablesSQL and the workspace→instance rebuild so the two
// can never drift.
const sentryConfigsColumns = `
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		auth_method TEXT NOT NULL,
		url TEXT NOT NULL DEFAULT 'https://sentry.io',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(workspace_id, name)`

// sentryWatchesColumns is the sentry_issue_watches column list including the
// nullable sentry_instance_id foreign key. Shared by createTablesSQL and the
// instance-column rebuild.
const sentryWatchesColumns = `
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		-- Bound Sentry instance. NULL = legacy unbound watch (created before a
		-- workspace had an instance, or migrated from the single-config model);
		-- the poller resolves an unbound watch to the workspace's sole instance
		-- at poll time. ON DELETE RESTRICT so an in-use instance cannot be
		-- deleted out from under a watch.
		sentry_instance_id TEXT,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		-- Optional repository binding. Empty = unbound (repo-less task, the
		-- historical behaviour). When set, watcher-created tasks launch in an
		-- isolated worktree of this repo cut from base_branch.
		repository_id TEXT NOT NULL DEFAULT '',
		base_branch TEXT NOT NULL DEFAULT '',
		filter_json TEXT NOT NULL DEFAULT '{}',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		max_inflight_tasks INTEGER DEFAULT 5,
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY(sentry_instance_id) REFERENCES sentry_configs(id) ON DELETE RESTRICT`

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS sentry_configs (` + sentryConfigsColumns + `
	);
	CREATE INDEX IF NOT EXISTS idx_sentry_configs_workspace
		ON sentry_configs(workspace_id);

	CREATE TABLE IF NOT EXISTS sentry_issue_watches (` + sentryWatchesColumns + `
	);
	CREATE INDEX IF NOT EXISTS idx_sentry_issue_watches_workspace
		ON sentry_issue_watches(workspace_id);

	CREATE TABLE IF NOT EXISTS sentry_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		issue_short_id TEXT NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, issue_short_id),
		FOREIGN KEY(issue_watch_id) REFERENCES sentry_issue_watches(id) ON DELETE CASCADE
	);
`

// singletonID is the synthetic primary key used by the legacy install-wide
// sentry_configs table.
const singletonID = "singleton"

// MigratedFromWorkspace is kept for parity with Jira/Linear provider
// migrations. It returns the workspace_id that received a legacy singleton row,
// so the provider secret migration can rekey the token from the singleton
// secret key to the workspace key before it is rekeyed again per instance.
func (s *Store) MigratedFromWorkspace() string {
	return s.migratedToWorkspace
}

// initSchema brings any database shape up to the id-keyed multi-instance model
// and applies the additive column migrations. Ordering matters:
//  1. legacy singleton → workspace-keyed (PR #1572's migration, kept verbatim);
//  2. workspace-keyed → id-keyed instances (this change);
//  3. createTablesSQL builds any absent table (fresh install, or the empty
//     sentry_configs a watches-only migration needs to reference);
//  4. additive ALTER-ADD column migrations reach the full pre-FK watches shape;
//  5. watches table rebuild adds the nullable sentry_instance_id FK + backfill.
//
// Steps 2 and 5 are each guarded on shape detection so a crash between them
// re-runs safely.
func (s *Store) initSchema() error {
	if err := s.migrateLegacySingletonTable(); err != nil {
		return err
	}
	if err := s.migrateConfigsTableToInstances(); err != nil {
		return err
	}
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := s.addConfigURLColumn(); err != nil {
		return err
	}
	if err := s.addMaxInflightTasksColumn(); err != nil {
		return err
	}
	if err := s.addIssueWatchLastErrorColumns(); err != nil {
		return err
	}
	if err := s.addIssueWatchRepositoryColumns(); err != nil {
		return err
	}
	if err := s.migrateWatchesAddInstanceColumn(); err != nil {
		return err
	}
	// The instance index is created here rather than in createTablesSQL: on a
	// pre-#1572 database the watches table exists without the sentry_instance_id
	// column until the rebuild above adds it, so indexing that column earlier
	// would fail. Idempotent for fresh installs (column already present).
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sentry_issue_watches_instance
		ON sentry_issue_watches(sentry_instance_id)`)
	return err
}

func (s *Store) migrateLegacySingletonTable() error {
	cols, err := s.tableColumns("sentry_configs")
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

	selectCols := "auth_method"
	if _, ok := cols["url"]; ok {
		selectCols += ", url"
	} else {
		selectCols += ", 'https://sentry.io' AS url"
	}
	if healthColumnsPresent(cols) {
		selectCols += ", last_checked_at, last_ok, last_error"
	} else {
		selectCols += ", NULL AS last_checked_at, 0 AS last_ok, '' AS last_error"
	}
	selectCols += ", created_at, updated_at"
	var authMethod, rawURL, lastError sql.NullString
	var lastCheckedAt sql.NullTime
	var lastOk sql.NullInt64
	var createdAt, updatedAt sql.NullTime
	row := tx.QueryRow(`SELECT `+selectCols+` FROM sentry_configs WHERE id = ? LIMIT 1`, singletonID)
	switch err := row.Scan(&authMethod, &rawURL, &lastCheckedAt, &lastOk, &lastError, &createdAt, &updatedAt); {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`DROP TABLE sentry_configs`); err != nil {
			return err
		}
		return tx.Commit()
	case err != nil:
		return err
	}
	if _, err := tx.Exec(`DROP TABLE sentry_configs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE sentry_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT 'https://sentry.io',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO sentry_configs (workspace_id, auth_method, url,
			last_checked_at, last_ok, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		targetWorkspace, authMethod.String, rawURL.String,
		nullableTime(lastCheckedAt), lastOk.Int64, lastError.String,
		nullableTime(createdAt), nullableTime(updatedAt)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.migratedToWorkspace = targetWorkspace
	return nil
}

// legacyWorkspaceConfig is one row of the pre-instance workspace-keyed
// sentry_configs table, read before the id-keyed rebuild.
type legacyWorkspaceConfig struct {
	workspaceID   string
	authMethod    string
	url           string
	lastCheckedAt sql.NullTime
	lastOk        int64
	lastError     string
	createdAt     sql.NullTime
	updatedAt     sql.NullTime
}

// migrateConfigsTableToInstances rebuilds a workspace-keyed sentry_configs
// table (PR #1572's one-config-per-workspace shape: workspace_id PRIMARY KEY,
// no id column) into the id-keyed multi-instance shape, creating exactly one
// instance per existing workspace row (id=uuid, name derived from the URL host
// with per-workspace suffixing on collision). Idempotent: a table that already
// has an id column (fresh install or a completed prior run) is left untouched,
// so a crash between this step and the watch rebuild re-runs safely.
func (s *Store) migrateConfigsTableToInstances() error {
	cols, err := s.tableColumns("sentry_configs")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil // fresh install — createTablesSQL builds the id-keyed table.
	}
	if _, hasID := cols["id"]; hasID {
		return nil // already id-keyed.
	}
	if _, hasWorkspace := cols["workspace_id"]; !hasWorkspace {
		return nil // unexpected shape; the legacy-singleton path owns it.
	}
	legacy, err := s.readLegacyWorkspaceConfigs(cols)
	if err != nil {
		return err
	}
	usedNames := make(map[string]map[string]struct{})
	ids := make([]string, len(legacy))
	names := make([]string, len(legacy))
	for i, r := range legacy {
		if usedNames[r.workspaceID] == nil {
			usedNames[r.workspaceID] = make(map[string]struct{})
		}
		ids[i] = uuid.New().String()
		names[i] = uniqueInstanceName(usedNames[r.workspaceID], hostFromURL(r.url))
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DROP TABLE sentry_configs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE sentry_configs (` + sentryConfigsColumns + `)`); err != nil {
		return err
	}
	for i, r := range legacy {
		if _, err := tx.Exec(`
			INSERT INTO sentry_configs (id, workspace_id, name, auth_method, url,
				last_checked_at, last_ok, last_error, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ids[i], r.workspaceID, names[i], r.authMethod, r.url,
			nullableTime(r.lastCheckedAt), r.lastOk, r.lastError,
			nullableTime(r.createdAt), nullableTime(r.updatedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// readLegacyWorkspaceConfigs reads every row of the workspace-keyed
// sentry_configs table, defaulting url / health columns for databases predating
// them so the rebuild always has a complete row.
func (s *Store) readLegacyWorkspaceConfigs(cols map[string]struct{}) ([]legacyWorkspaceConfig, error) {
	selectCols := "workspace_id, auth_method"
	if _, ok := cols["url"]; ok {
		selectCols += ", url"
	} else {
		selectCols += ", 'https://sentry.io' AS url"
	}
	if healthColumnsPresent(cols) {
		selectCols += ", last_checked_at, last_ok, last_error"
	} else {
		selectCols += ", NULL AS last_checked_at, 0 AS last_ok, '' AS last_error"
	}
	selectCols += ", created_at, updated_at"
	rows, err := s.db.Query(`SELECT ` + selectCols + ` FROM sentry_configs ORDER BY created_at, workspace_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []legacyWorkspaceConfig
	for rows.Next() {
		var r legacyWorkspaceConfig
		if err := rows.Scan(&r.workspaceID, &r.authMethod, &r.url,
			&r.lastCheckedAt, &r.lastOk, &r.lastError, &r.createdAt, &r.updatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// migrateWatchesAddInstanceColumn rebuilds sentry_issue_watches to add the
// nullable sentry_instance_id foreign key. SQLite cannot ALTER-ADD a column
// with a FOREIGN KEY clause, so the table is rebuilt: create new, copy rows,
// drop old, rename. Each watch is backfilled to its workspace's sole instance
// (NULL when the workspace has no instance). The rebuild runs on a dedicated
// connection with foreign_keys temporarily off so DROP TABLE does not
// cascade-delete the sentry_issue_watch_tasks children (the PRAGMA is a no-op
// inside a transaction and per-connection, hence the dedicated conn), then
// restores enforcement. Idempotent: the migration is skipped only when the
// column already has the exact nullable-FK target shape, so a crash
// mid-migration (or an older wrong-shaped column) always re-runs safely.
func (s *Store) migrateWatchesAddInstanceColumn() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil // fresh install — createTablesSQL builds the column + FK.
	}
	if _, ok := cols["sentry_instance_id"]; ok {
		correct, err := s.watchesInstanceColumnCorrect()
		if err != nil {
			return err
		}
		if correct {
			return nil // already the nullable-FK shape.
		}
		// Column exists but predates the FK/nullable schema (an experimental
		// pre-release build stored it as NOT NULL DEFAULT '' with no foreign
		// key). Fall through to rebuild it into the correct shape.
	}
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	// Restore enforcement on the way out regardless of outcome.
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`) }()
	return rebuildWatchesWithInstanceColumn(ctx, conn)
}

// watchesInstanceColumnCorrect reports whether sentry_issue_watches
// .sentry_instance_id already has the target shape: a NULLABLE column backed by
// a foreign key to sentry_configs. Older experimental builds added it as
// NOT NULL DEFAULT ” without a foreign key; those must be rebuilt, not skipped.
func (s *Store) watchesInstanceColumnCorrect() (bool, error) {
	nullable, err := s.columnIsNullable("sentry_issue_watches", "sentry_instance_id")
	if err != nil {
		return false, err
	}
	if !nullable {
		return false, nil
	}
	return s.hasForeignKey("sentry_issue_watches", "sentry_instance_id", "sentry_configs", "id", "RESTRICT")
}

// columnIsNullable reports whether a column exists and permits NULL (PRAGMA
// table_info notnull flag == 0).
func (s *Store) columnIsNullable(table, column string) (bool, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
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
			return false, err
		}
		if name == column {
			return notnull == 0, nil
		}
	}
	return false, rows.Err()
}

// hasForeignKey reports whether `table` has a foreign key from `column` to
// `refTable(wantRefColumn)` with the given ON DELETE rule (PRAGMA
// foreign_key_list; SQLite reports on_delete uppercased, e.g. "RESTRICT").
func (s *Store) hasForeignKey(table, column, refTable, wantRefColumn, wantOnDelete string) (bool, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA foreign_key_list(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			id       int
			seq      int
			refTbl   string
			from     string
			to       sql.NullString
			onUpdate string
			onDelete string
			match    string
		)
		if err := rows.Scan(&id, &seq, &refTbl, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return false, err
		}
		if from == column && refTbl == refTable && to.String == wantRefColumn && onDelete == wantOnDelete {
			return true, nil
		}
	}
	return false, rows.Err()
}

// rebuildWatchesWithInstanceColumn performs the create/copy/drop/rename table
// rebuild inside a single transaction on conn (which already has foreign_keys
// disabled). Split out to keep migrateWatchesAddInstanceColumn within lint
// limits.
func rebuildWatchesWithInstanceColumn(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE sentry_issue_watches_new (`+sentryWatchesColumns+`)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sentry_issue_watches_new (
			id, workspace_id, sentry_instance_id, workflow_id, workflow_step_id,
			repository_id, base_branch, filter_json, agent_profile_id,
			executor_profile_id, prompt, enabled, poll_interval_seconds,
			max_inflight_tasks, last_polled_at, last_error, last_error_at,
			created_at, updated_at)
		SELECT
			w.id, w.workspace_id,
			(SELECT c.id FROM sentry_configs c WHERE c.workspace_id = w.workspace_id
			 ORDER BY c.created_at, c.id LIMIT 1),
			w.workflow_id, w.workflow_step_id, w.repository_id, w.base_branch,
			w.filter_json, w.agent_profile_id, w.executor_profile_id, w.prompt,
			w.enabled, w.poll_interval_seconds, w.max_inflight_tasks,
			w.last_polled_at, w.last_error, w.last_error_at, w.created_at, w.updated_at
		FROM sentry_issue_watches w`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE sentry_issue_watches`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE sentry_issue_watches_new RENAME TO sentry_issue_watches`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_sentry_issue_watches_workspace ON sentry_issue_watches(workspace_id)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_sentry_issue_watches_instance ON sentry_issue_watches(sentry_instance_id)`); err != nil {
		return err
	}
	return tx.Commit()
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

func healthColumnsPresent(cols map[string]struct{}) bool {
	for _, name := range []string{"last_checked_at", "last_ok", "last_error"} {
		if _, ok := cols[name]; !ok {
			return false
		}
	}
	return true
}

func nullableTime(t sql.NullTime) interface{} {
	if !t.Valid {
		return nil
	}
	return t.Time
}

// defaultInstanceName is the fallback instance name when a URL yields no host.
const defaultInstanceName = "Sentry"

// hostFromURL extracts the host of a Sentry instance URL for use as a default
// instance name. Falls back to "Sentry" for a blank/unparseable URL.
func hostFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return defaultInstanceName
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return defaultInstanceName
	}
	return u.Host
}

// uniqueInstanceName returns a name unique within the supplied set, suffixing
// " (2)", " (3)", … on collision. The chosen name is recorded in used so the
// next call for the same workspace avoids it.
func uniqueInstanceName(used map[string]struct{}, base string) string {
	if base == "" {
		base = defaultInstanceName
	}
	name := base
	for i := 2; ; i++ {
		if _, taken := used[name]; !taken {
			used[name] = struct{}{}
			return name
		}
		name = fmt.Sprintf("%s (%d)", base, i)
	}
}

// addIssueWatchRepositoryColumns brings older databases up to the current
// schema by appending repository_id / base_branch to sentry_issue_watches when
// missing. Both backfill to ” (unbound), so existing repo-less watches keep
// their behaviour. Idempotent — column lookup before each ALTER avoids the
// "duplicate column name" error.
func (s *Store) addIssueWatchRepositoryColumns() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["repository_id"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add repository_id column: %w", err)
		}
	}
	if _, ok := cols["base_branch"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN base_branch TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add base_branch column: %w", err)
		}
	}
	return nil
}

// addConfigURLColumn adds the url column to a workspace-keyed sentry_configs
// table when missing (a pre-self-hosted database). Retained for the upgrade
// lineage; the id-keyed table always declares url, so this is a no-op there.
func (s *Store) addConfigURLColumn() error {
	cols, err := s.tableColumns("sentry_configs")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["url"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE sentry_configs ADD COLUMN url TEXT NOT NULL DEFAULT 'https://sentry.io'`); err != nil {
		return fmt.Errorf("add url column: %w", err)
	}
	return nil
}

// addMaxInflightTasksColumn adds the max_inflight_tasks column to
// sentry_issue_watches when missing, backfilling existing rows to the default
// (5). Idempotent.
func (s *Store) addMaxInflightTasksColumn() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["max_inflight_tasks"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN max_inflight_tasks INTEGER DEFAULT 5`); err != nil {
		return fmt.Errorf("add max_inflight_tasks column: %w", err)
	}
	return nil
}

// addIssueWatchLastErrorColumns appends last_error / last_error_at to
// sentry_issue_watches when missing. Idempotent — column lookup before each
// ALTER avoids the "duplicate column name" error.
func (s *Store) addIssueWatchLastErrorColumns() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["last_error"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add last_error column: %w", err)
		}
	}
	if _, ok := cols["last_error_at"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN last_error_at DATETIME`); err != nil {
			return fmt.Errorf("add last_error_at column: %w", err)
		}
	}
	return nil
}

// tableColumns returns the set of column names for a table via PRAGMA
// table_info, used by the lightweight ADD COLUMN migrations and the shape
// detection guarding the table rebuilds above.
func (s *Store) tableColumns(table string) (map[string]struct{}, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
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

const selectInstanceColumns = `id, workspace_id, name, auth_method, url,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// ListInstances returns every Sentry instance in a workspace, ordered by name.
func (s *Store) ListInstances(ctx context.Context, workspaceID string) ([]*SentryConfig, error) {
	var rows []SentryConfig
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+selectInstanceColumns+` FROM sentry_configs WHERE workspace_id = ? ORDER BY name, id`,
		workspaceID); err != nil {
		return nil, err
	}
	return instancePtrs(rows), nil
}

// ListAllInstances returns every instance across all workspaces. Used by the
// auth-health poller and the provider secret rekey.
func (s *Store) ListAllInstances(ctx context.Context) ([]*SentryConfig, error) {
	var rows []SentryConfig
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+selectInstanceColumns+` FROM sentry_configs ORDER BY workspace_id, name, id`); err != nil {
		return nil, err
	}
	return instancePtrs(rows), nil
}

// GetInstance returns an instance by ID, or nil when no row matches. Ownership
// is checked by the service against the request's workspace.
func (s *Store) GetInstance(ctx context.Context, id string) (*SentryConfig, error) {
	var cfg SentryConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectInstanceColumns+` FROM sentry_configs WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// CreateInstance inserts a new instance row. ID and timestamps are assigned
// here when unset. A duplicate (workspace_id, name) surfaces as
// ErrDuplicateInstanceName.
func (s *Store) CreateInstance(ctx context.Context, cfg *SentryConfig) error {
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sentry_configs (id, workspace_id, name, auth_method, url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cfg.ID, cfg.WorkspaceID, cfg.Name, cfg.AuthMethod, cfg.URL, cfg.CreatedAt, cfg.UpdatedAt)
	if isUniqueViolation(err) {
		return ErrDuplicateInstanceName
	}
	return err
}

// UpdateInstance overwrites name/auth_method/url of an existing instance. The
// last_* health columns are owned by the poller. Returns ErrInstanceNotFound
// when no row matches and ErrDuplicateInstanceName on a name collision.
func (s *Store) UpdateInstance(ctx context.Context, cfg *SentryConfig) error {
	cfg.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE sentry_configs SET name = ?, auth_method = ?, url = ?, updated_at = ?
		WHERE id = ?`,
		cfg.Name, cfg.AuthMethod, cfg.URL, cfg.UpdatedAt, cfg.ID)
	if isUniqueViolation(err) {
		return ErrDuplicateInstanceName
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

// DeleteInstance removes an instance row. A watch still referencing it trips
// the ON DELETE RESTRICT foreign key, surfaced as ErrInstanceInUse (the service
// fills the watch count). Returns ErrInstanceNotFound when no row matches.
func (s *Store) DeleteInstance(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sentry_configs WHERE id = ?`, id)
	if isForeignKeyViolation(err) {
		return ErrInstanceInUse{}
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

// CountWatchesForInstance counts every watch (enabled and disabled) bound to an
// instance, used to reject deletion of an in-use instance with a friendly count.
func (s *Store) CountWatchesForInstance(ctx context.Context, instanceID string) (int, error) {
	var n int
	if err := s.ro.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM sentry_issue_watches WHERE sentry_instance_id = ?`, instanceID); err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateAuthHealthForInstance records the result of a credential probe on one
// instance row.
func (s *Store) UpdateAuthHealthForInstance(ctx context.Context, id string, ok bool, errMsg string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sentry_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE id = ?`,
		checkedAt, ok, errMsg, id)
	return err
}

// HasConfig reports whether any instance exists. Used by the auth-health poller
// to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	if err := s.ro.GetContext(ctx, &present, `SELECT COUNT(*) FROM sentry_configs`); err != nil {
		return false, err
	}
	return present > 0, nil
}

func instancePtrs(rows []SentryConfig) []*SentryConfig {
	out := make([]*SentryConfig, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func isForeignKeyViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
