package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/db/dialect"
)

type sqliteRepository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	ownsDB bool
}

var _ Repository = (*sqliteRepository)(nil)

func newSQLiteRepositoryWithDB(writer, reader *sqlx.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(writer, reader, false)
}

func newSQLiteRepository(writer, reader *sqlx.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: writer, ro: reader, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := writer.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func (r *sqliteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		workspace_id TEXT DEFAULT NULL,
		supports_mcp INTEGER NOT NULL DEFAULT 0,
		mcp_config_path TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS agent_profiles (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		name TEXT NOT NULL,
		agent_display_name TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		mode TEXT DEFAULT NULL,
		migrated_from TEXT DEFAULT NULL,
		auto_approve INTEGER NOT NULL DEFAULT 0,
		dangerously_skip_permissions INTEGER NOT NULL DEFAULT 0,
		allow_indexing INTEGER NOT NULL DEFAULT 1,
		cli_passthrough INTEGER NOT NULL DEFAULT 0,
		user_modified INTEGER NOT NULL DEFAULT 0,
		plan TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS agent_profile_mcp_configs (
		profile_id TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 0,
		servers_json TEXT NOT NULL DEFAULT '{}',
		meta_json TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (profile_id) REFERENCES agent_profiles(id) ON DELETE CASCADE
	);

	DROP INDEX IF EXISTS idx_agents_name;
	CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_name ON agents(name);
	CREATE INDEX IF NOT EXISTS idx_agent_profiles_agent_id ON agent_profiles(agent_id);
	`
	_, err := r.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add tui_config column (idempotent — ignore error if already exists)
	_, _ = r.db.Exec(`ALTER TABLE agents ADD COLUMN tui_config TEXT DEFAULT NULL`)

	// Migration: add mode and migrated_from columns on agent_profiles (idempotent)
	_, _ = r.db.Exec(`ALTER TABLE agent_profiles ADD COLUMN mode TEXT DEFAULT NULL`)
	_, _ = r.db.Exec(`ALTER TABLE agent_profiles ADD COLUMN migrated_from TEXT DEFAULT NULL`)

	// Migration: drop CHECK(model != '') constraint from agent_profiles.
	//
	// The ACP-first model means models and modes are populated from the host
	// utility probe cache at boot. An empty model is valid — it means "use the
	// agent's default". SQLite does not support ALTER COLUMN or DROP CONSTRAINT,
	// so we must recreate the table. This is idempotent: we check whether the
	// old CHECK constraint still exists before doing anything.
	if err := r.migrateDropModelCheckConstraint(); err != nil {
		return fmt.Errorf("failed to migrate agent_profiles model constraint: %w", err)
	}

	return nil
}

// migrateDropModelCheckConstraint recreates agent_profiles without the legacy
// non-empty-model CHECK constraint. Existing databases created before the
// ACP-first migration carry this constraint, which prevents empty model
// values. New databases (created by the CREATE TABLE IF NOT EXISTS above)
// never have it.
//
// The migration is idempotent: it inspects sqlite_master for the CHECK keyword
// and only proceeds when the constraint is present.
func (r *sqliteRepository) migrateDropModelCheckConstraint() error {
	var tableDDL string
	err := r.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='agent_profiles'`,
	).Scan(&tableDDL)
	if errors.Is(err, sql.ErrNoRows) {
		// Table doesn't exist yet (fresh DB, CREATE TABLE IF NOT EXISTS
		// hasn't run or was a no-op) — nothing to migrate.
		return nil
	}
	if err != nil {
		return fmt.Errorf("query agent_profiles DDL: %w", err)
	}

	// Only migrate if the old model CHECK constraint is still present.
	// Use a targeted match to avoid false-positives from unrelated future
	// CHECK constraints on the same table.
	if !strings.Contains(tableDDL, "CHECK(model") {
		return nil
	}

	return r.recreateAgentProfilesWithoutModelCheck()
}

// recreateAgentProfilesWithoutModelCheck performs the actual SQLite table
// recreation: copy data into a new table without the CHECK constraint, drop
// the old table, rename the new one. Wrapped in a transaction so a crash
// mid-migration doesn't leave the DB without the agent_profiles table.
func (r *sqliteRepository) recreateAgentProfilesWithoutModelCheck() error {
	// Disable FK enforcement during the recreation: the DB is opened with
	// _foreign_keys=on, and agent_profile_mcp_configs references
	// agent_profiles(id). This matches the pattern in task/repository.
	if _, err := r.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign keys for migration: %w", err)
	}
	defer func() { _, _ = r.db.Exec(`PRAGMA foreign_keys=ON`) }()

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const columns = `id, agent_id, name, agent_display_name, model, mode, migrated_from,
		auto_approve, dangerously_skip_permissions, allow_indexing,
		cli_passthrough, user_modified, plan, created_at, updated_at, deleted_at`

	if _, err := tx.Exec(`CREATE TABLE agent_profiles_new (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		name TEXT NOT NULL,
		agent_display_name TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		mode TEXT DEFAULT NULL,
		migrated_from TEXT DEFAULT NULL,
		auto_approve INTEGER NOT NULL DEFAULT 0,
		dangerously_skip_permissions INTEGER NOT NULL DEFAULT 0,
		allow_indexing INTEGER NOT NULL DEFAULT 1,
		cli_passthrough INTEGER NOT NULL DEFAULT 0,
		user_modified INTEGER NOT NULL DEFAULT 0,
		plan TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		deleted_at TIMESTAMP,
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("create new table: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO agent_profiles_new (` + columns + `) SELECT ` + columns + ` FROM agent_profiles`,
	); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	if _, err := tx.Exec(`DROP TABLE agent_profiles`); err != nil {
		return fmt.Errorf("drop old table: %w", err)
	}

	if _, err := tx.Exec(`ALTER TABLE agent_profiles_new RENAME TO agent_profiles`); err != nil {
		return fmt.Errorf("rename new table: %w", err)
	}

	if _, err := tx.Exec(
		`CREATE INDEX IF NOT EXISTS idx_agent_profiles_agent_id ON agent_profiles(agent_id)`,
	); err != nil {
		return fmt.Errorf("recreate index: %w", err)
	}

	return tx.Commit()
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) CreateAgent(ctx context.Context, agent *models.Agent) error {
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	agent.CreatedAt = now
	agent.UpdatedAt = now
	var tuiConfigJSON *string
	if agent.TUIConfig != nil {
		data, err := json.Marshal(agent.TUIConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal tui_config: %w", err)
		}
		s := string(data)
		tuiConfigJSON = &s
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agents (id, name, workspace_id, supports_mcp, mcp_config_path, tui_config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), agent.ID, agent.Name, agent.WorkspaceID, dialect.BoolToInt(agent.SupportsMCP), agent.MCPConfigPath, tuiConfigJSON, agent.CreatedAt, agent.UpdatedAt)
	return err
}

func (r *sqliteRepository) GetAgent(ctx context.Context, id string) (*models.Agent, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, tui_config, created_at, updated_at
		FROM agents WHERE id = ?
	`), id)
	return scanAgent(row)
}

func (r *sqliteRepository) GetAgentByName(ctx context.Context, name string) (*models.Agent, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, tui_config, created_at, updated_at
		FROM agents WHERE name = ?
	`), name)
	return scanAgent(row)
}

func (r *sqliteRepository) UpdateAgent(ctx context.Context, agent *models.Agent) error {
	agent.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agents SET workspace_id = ?, supports_mcp = ?, mcp_config_path = ?, updated_at = ?
		WHERE id = ?
	`), agent.WorkspaceID, dialect.BoolToInt(agent.SupportsMCP), agent.MCPConfigPath, agent.UpdatedAt, agent.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", agent.ID)
	}
	return nil
}

func (r *sqliteRepository) DeleteAgent(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM agents WHERE id = ?`), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", id)
	}
	return nil
}

func (r *sqliteRepository) ListAgents(ctx context.Context) ([]*models.Agent, error) {
	return r.listAgentsWhere(ctx, "1=1")
}

func (r *sqliteRepository) GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*models.AgentProfileMcpConfig, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT profile_id, enabled, servers_json, meta_json, created_at, updated_at
		FROM agent_profile_mcp_configs
		WHERE profile_id = ?
	`), profileID)

	var config models.AgentProfileMcpConfig
	var enabled int
	var serversJSON string
	var metaJSON string
	if err := row.Scan(&config.ProfileID, &enabled, &serversJSON, &metaJSON, &config.CreatedAt, &config.UpdatedAt); err != nil {
		return nil, err
	}
	config.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(serversJSON), &config.Servers); err != nil {
		return nil, fmt.Errorf("failed to parse MCP servers JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(metaJSON), &config.Meta); err != nil {
		return nil, fmt.Errorf("failed to parse MCP meta JSON: %w", err)
	}
	return &config, nil
}

func (r *sqliteRepository) UpsertAgentProfileMcpConfig(ctx context.Context, config *models.AgentProfileMcpConfig) error {
	if config.ProfileID == "" {
		return fmt.Errorf("profile ID is required")
	}
	if config.Servers == nil {
		config.Servers = map[string]interface{}{}
	}
	if config.Meta == nil {
		config.Meta = map[string]interface{}{}
	}
	now := time.Now().UTC()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now

	serversJSON, err := json.Marshal(config.Servers)
	if err != nil {
		return fmt.Errorf("failed to serialize MCP servers: %w", err)
	}
	metaJSON, err := json.Marshal(config.Meta)
	if err != nil {
		return fmt.Errorf("failed to serialize MCP meta: %w", err)
	}

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_profile_mcp_configs (profile_id, enabled, servers_json, meta_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id) DO UPDATE SET
			enabled = excluded.enabled,
			servers_json = excluded.servers_json,
			meta_json = excluded.meta_json,
			updated_at = excluded.updated_at
	`), config.ProfileID, dialect.BoolToInt(config.Enabled), string(serversJSON), string(metaJSON), config.CreatedAt, config.UpdatedAt)
	return err
}

func (r *sqliteRepository) CreateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, mode, migrated_from, auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, user_modified, plan, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)
	`), profile.ID, profile.AgentID, profile.Name, profile.AgentDisplayName, profile.Model,
		nullableString(profile.Mode), nullableString(profile.MigratedFrom),
		dialect.BoolToInt(profile.AutoApprove),
		dialect.BoolToInt(profile.DangerouslySkipPermissions), dialect.BoolToInt(profile.AllowIndexing), dialect.BoolToInt(profile.CLIPassthrough), dialect.BoolToInt(profile.UserModified), profile.CreatedAt, profile.UpdatedAt, profile.DeletedAt)
	return err
}

// nullableString converts an empty string to sql.NullString zero-value so the
// column is written as NULL rather than "". Keeps nullable columns clean.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func (r *sqliteRepository) UpdateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	profile.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles
		SET name = ?, agent_display_name = ?, model = ?, mode = ?, migrated_from = ?, auto_approve = ?, dangerously_skip_permissions = ?, allow_indexing = ?, cli_passthrough = ?, user_modified = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`), profile.Name, profile.AgentDisplayName, profile.Model,
		nullableString(profile.Mode), nullableString(profile.MigratedFrom),
		dialect.BoolToInt(profile.AutoApprove),
		dialect.BoolToInt(profile.DangerouslySkipPermissions), dialect.BoolToInt(profile.AllowIndexing), dialect.BoolToInt(profile.CLIPassthrough), dialect.BoolToInt(profile.UserModified), profile.UpdatedAt, profile.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent profile not found: %s", profile.ID)
	}
	return nil
}

func (r *sqliteRepository) DeleteAgentProfile(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_profiles SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`), now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent profile not found: %s", id)
	}
	return nil
}

func (r *sqliteRepository) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, agent_id, name, agent_display_name, model, mode, migrated_from, auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, user_modified, plan, created_at, updated_at, deleted_at
		FROM agent_profiles WHERE id = ? AND deleted_at IS NULL
	`), id)
	return scanAgentProfile(row)
}

func (r *sqliteRepository) ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, agent_id, name, agent_display_name, model, mode, migrated_from, auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, user_modified, plan, created_at, updated_at, deleted_at
		FROM agent_profiles WHERE agent_id = ? AND deleted_at IS NULL ORDER BY created_at DESC
	`), agentID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var result []*models.AgentProfile
	for rows.Next() {
		profile, err := scanAgentProfile(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, profile)
	}
	return result, rows.Err()
}

func (r *sqliteRepository) ListTUIAgents(ctx context.Context) ([]*models.Agent, error) {
	return r.listAgentsWhere(ctx, "tui_config IS NOT NULL")
}

func (r *sqliteRepository) listAgentsWhere(ctx context.Context, where string) ([]*models.Agent, error) {
	rows, err := r.ro.QueryContext(ctx,
		`SELECT id, name, workspace_id, supports_mcp, mcp_config_path, tui_config, created_at, updated_at
		FROM agents WHERE `+where+` ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var result []*models.Agent
	for rows.Next() {
		agent, scanErr := scanAgent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, agent)
	}
	return result, rows.Err()
}

func scanAgent(scanner interface {
	Scan(dest ...any) error
}) (*models.Agent, error) {
	agent := &models.Agent{}
	var supportsMCP int
	var workspaceID sql.NullString
	var tuiConfigRaw sql.NullString
	if err := scanner.Scan(
		&agent.ID,
		&agent.Name,
		&workspaceID,
		&supportsMCP,
		&agent.MCPConfigPath,
		&tuiConfigRaw,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if workspaceID.Valid {
		agent.WorkspaceID = &workspaceID.String
	}
	agent.SupportsMCP = supportsMCP == 1
	if tuiConfigRaw.Valid {
		var cfg models.TUIConfigJSON
		if err := json.Unmarshal([]byte(tuiConfigRaw.String), &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse tui_config: %w", err)
		}
		agent.TUIConfig = &cfg
	}
	return agent, nil
}

func scanAgentProfile(scanner interface {
	Scan(dest ...any) error
}) (*models.AgentProfile, error) {
	profile := &models.AgentProfile{}
	var mode sql.NullString
	var migratedFrom sql.NullString
	var autoApprove int
	var skipPermissions int
	var allowIndexing int
	var cliPassthrough int
	var userModified int
	var plan string // unused, kept for backwards compatibility
	if err := scanner.Scan(
		&profile.ID,
		&profile.AgentID,
		&profile.Name,
		&profile.AgentDisplayName,
		&profile.Model,
		&mode,
		&migratedFrom,
		&autoApprove,
		&skipPermissions,
		&allowIndexing,
		&cliPassthrough,
		&userModified,
		&plan,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&profile.DeletedAt,
	); err != nil {
		return nil, err
	}
	if mode.Valid {
		profile.Mode = mode.String
	}
	if migratedFrom.Valid {
		profile.MigratedFrom = migratedFrom.String
	}
	profile.AutoApprove = autoApprove == 1
	profile.DangerouslySkipPermissions = skipPermissions == 1
	profile.AllowIndexing = allowIndexing == 1
	profile.CLIPassthrough = cliPassthrough == 1
	profile.UserModified = userModified == 1
	return profile, nil
}
