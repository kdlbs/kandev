package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/sqlite"
)

type sqliteRepository struct {
	db     *sql.DB
	ownsDB bool
}

var _ Repository = (*sqliteRepository)(nil)

func newSQLiteRepositoryWithDB(dbConn *sql.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(dbConn, false)
}

func newSQLiteRepository(dbConn *sql.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: dbConn, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := dbConn.Close(); closeErr != nil {
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
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS agent_profiles (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		name TEXT NOT NULL,
		agent_display_name TEXT NOT NULL,
		model TEXT NOT NULL CHECK(model != ''),
		auto_approve INTEGER NOT NULL DEFAULT 0,
		dangerously_skip_permissions INTEGER NOT NULL DEFAULT 0,
		allow_indexing INTEGER NOT NULL DEFAULT 1,
		plan TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME,
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS agent_profile_mcp_configs (
		profile_id TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 0,
		servers_json TEXT NOT NULL DEFAULT '{}',
		meta_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
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
	if err := sqlite.EnsureColumn(r.db, "agent_profiles", "deleted_at", "DATETIME"); err != nil {
		return err
	}
	if err := sqlite.EnsureColumn(r.db, "agent_profiles", "agent_display_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := sqlite.EnsureColumn(r.db, "agent_profiles", "cli_passthrough", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return nil
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
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agents (id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, agent.ID, agent.Name, agent.WorkspaceID, sqlite.BoolToInt(agent.SupportsMCP), agent.MCPConfigPath, agent.CreatedAt, agent.UpdatedAt)
	return err
}

func (r *sqliteRepository) GetAgent(ctx context.Context, id string) (*models.Agent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at
		FROM agents WHERE id = ?
	`, id)
	return scanAgent(row)
}

func (r *sqliteRepository) GetAgentByName(ctx context.Context, name string) (*models.Agent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at
		FROM agents WHERE name = ?
	`, name)
	return scanAgent(row)
}

func (r *sqliteRepository) UpdateAgent(ctx context.Context, agent *models.Agent) error {
	agent.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE agents SET workspace_id = ?, supports_mcp = ?, mcp_config_path = ?, updated_at = ?
		WHERE id = ?
	`, agent.WorkspaceID, sqlite.BoolToInt(agent.SupportsMCP), agent.MCPConfigPath, agent.UpdatedAt, agent.ID)
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
	result, err := r.db.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
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
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at
		FROM agents ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var result []*models.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, agent)
	}
	return result, rows.Err()
}

func (r *sqliteRepository) GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*models.AgentProfileMcpConfig, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT profile_id, enabled, servers_json, meta_json, created_at, updated_at
		FROM agent_profile_mcp_configs
		WHERE profile_id = ?
	`, profileID)

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

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO agent_profile_mcp_configs (profile_id, enabled, servers_json, meta_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id) DO UPDATE SET
			enabled = excluded.enabled,
			servers_json = excluded.servers_json,
			meta_json = excluded.meta_json,
			updated_at = excluded.updated_at
	`, config.ProfileID, sqlite.BoolToInt(config.Enabled), string(serversJSON), string(metaJSON), config.CreatedAt, config.UpdatedAt)
	return err
}

func (r *sqliteRepository) CreateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, model, auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, plan, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, profile.ID, profile.AgentID, profile.Name, profile.AgentDisplayName, profile.Model, sqlite.BoolToInt(profile.AutoApprove),
		sqlite.BoolToInt(profile.DangerouslySkipPermissions), sqlite.BoolToInt(profile.AllowIndexing), sqlite.BoolToInt(profile.CLIPassthrough), profile.Plan, profile.CreatedAt, profile.UpdatedAt, profile.DeletedAt)
	return err
}

func (r *sqliteRepository) UpdateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	profile.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_profiles
		SET name = ?, agent_display_name = ?, model = ?, auto_approve = ?, dangerously_skip_permissions = ?, allow_indexing = ?, cli_passthrough = ?, plan = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, profile.Name, profile.AgentDisplayName, profile.Model, sqlite.BoolToInt(profile.AutoApprove),
		sqlite.BoolToInt(profile.DangerouslySkipPermissions), sqlite.BoolToInt(profile.AllowIndexing), sqlite.BoolToInt(profile.CLIPassthrough), profile.Plan, profile.UpdatedAt, profile.ID)
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
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_profiles SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
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
	row := r.db.QueryRowContext(ctx, `
		SELECT id, agent_id, name, agent_display_name, model, auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, plan, created_at, updated_at, deleted_at
		FROM agent_profiles WHERE id = ? AND deleted_at IS NULL
	`, id)
	return scanAgentProfile(row)
}

func (r *sqliteRepository) ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, agent_id, name, agent_display_name, model, auto_approve, dangerously_skip_permissions, allow_indexing, cli_passthrough, plan, created_at, updated_at, deleted_at
		FROM agent_profiles WHERE agent_id = ? AND deleted_at IS NULL ORDER BY created_at DESC
	`, agentID)
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

func scanAgent(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.Agent, error) {
	agent := &models.Agent{}
	var supportsMCP int
	var workspaceID sql.NullString
	if err := scanner.Scan(
		&agent.ID,
		&agent.Name,
		&workspaceID,
		&supportsMCP,
		&agent.MCPConfigPath,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if workspaceID.Valid {
		agent.WorkspaceID = &workspaceID.String
	}
	agent.SupportsMCP = supportsMCP == 1
	return agent, nil
}

func scanAgentProfile(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.AgentProfile, error) {
	profile := &models.AgentProfile{}
	var autoApprove int
	var skipPermissions int
	var allowIndexing int
	var cliPassthrough int
	if err := scanner.Scan(
		&profile.ID,
		&profile.AgentID,
		&profile.Name,
		&profile.AgentDisplayName,
		&profile.Model,
		&autoApprove,
		&skipPermissions,
		&allowIndexing,
		&cliPassthrough,
		&profile.Plan,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&profile.DeletedAt,
	); err != nil {
		return nil, err
	}
	profile.AutoApprove = autoApprove == 1
	profile.DangerouslySkipPermissions = skipPermissions == 1
	profile.AllowIndexing = allowIndexing == 1
	profile.CLIPassthrough = cliPassthrough == 1
	return profile, nil
}
