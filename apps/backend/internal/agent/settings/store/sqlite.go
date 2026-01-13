package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

type SQLiteRepository struct {
	db *sql.DB
}

var _ Repository = (*SQLiteRepository)(nil)

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	normalizedPath := normalizeSQLitePath(dbPath)
	if err := ensureSQLiteDir(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to prepare database path: %w", err)
	}
	if err := ensureSQLiteFile(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to create database file: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_mode=rwc", normalizedPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	repo := &SQLiteRepository{db: db}
	if err := repo.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func ensureSQLiteDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func ensureSQLiteFile(dbPath string) error {
	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func normalizeSQLitePath(dbPath string) string {
	if dbPath == "" {
		return dbPath
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return dbPath
	}
	return abs
}

func (r *SQLiteRepository) initSchema() error {
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
		model TEXT NOT NULL,
		auto_approve INTEGER NOT NULL DEFAULT 0,
		dangerously_skip_permissions INTEGER NOT NULL DEFAULT 0,
		plan TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
	);

	DROP INDEX IF EXISTS idx_agents_name;
	CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_name ON agents(name);
	CREATE INDEX IF NOT EXISTS idx_agent_profiles_agent_id ON agent_profiles(agent_id);
	`
	_, err := r.db.Exec(schema)
	return err
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepository) CreateAgent(ctx context.Context, agent *models.Agent) error {
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	agent.CreatedAt = now
	agent.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agents (id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, agent.ID, agent.Name, agent.WorkspaceID, boolToInt(agent.SupportsMCP), agent.MCPConfigPath, agent.CreatedAt, agent.UpdatedAt)
	return err
}

func (r *SQLiteRepository) GetAgent(ctx context.Context, id string) (*models.Agent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at
		FROM agents WHERE id = ?
	`, id)
	return scanAgent(row)
}

func (r *SQLiteRepository) GetAgentByName(ctx context.Context, name string) (*models.Agent, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at
		FROM agents WHERE name = ?
	`, name)
	return scanAgent(row)
}

func (r *SQLiteRepository) UpdateAgent(ctx context.Context, agent *models.Agent) error {
	agent.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE agents SET workspace_id = ?, supports_mcp = ?, mcp_config_path = ?, updated_at = ?
		WHERE id = ?
	`, agent.WorkspaceID, boolToInt(agent.SupportsMCP), agent.MCPConfigPath, agent.UpdatedAt, agent.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", agent.ID)
	}
	return nil
}

func (r *SQLiteRepository) DeleteAgent(ctx context.Context, id string) error {
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

func (r *SQLiteRepository) ListAgents(ctx context.Context) ([]*models.Agent, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, workspace_id, supports_mcp, mcp_config_path, created_at, updated_at
		FROM agents ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

func (r *SQLiteRepository) CreateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, model, auto_approve, dangerously_skip_permissions, plan, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, profile.ID, profile.AgentID, profile.Name, profile.Model, boolToInt(profile.AutoApprove),
		boolToInt(profile.DangerouslySkipPermissions), profile.Plan, profile.CreatedAt, profile.UpdatedAt)
	return err
}

func (r *SQLiteRepository) UpdateAgentProfile(ctx context.Context, profile *models.AgentProfile) error {
	profile.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_profiles
		SET name = ?, model = ?, auto_approve = ?, dangerously_skip_permissions = ?, plan = ?, updated_at = ?
		WHERE id = ?
	`, profile.Name, profile.Model, boolToInt(profile.AutoApprove),
		boolToInt(profile.DangerouslySkipPermissions), profile.Plan, profile.UpdatedAt, profile.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent profile not found: %s", profile.ID)
	}
	return nil
}

func (r *SQLiteRepository) DeleteAgentProfile(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM agent_profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent profile not found: %s", id)
	}
	return nil
}

func (r *SQLiteRepository) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, agent_id, name, model, auto_approve, dangerously_skip_permissions, plan, created_at, updated_at
		FROM agent_profiles WHERE id = ?
	`, id)
	return scanAgentProfile(row)
}

func (r *SQLiteRepository) ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, agent_id, name, model, auto_approve, dangerously_skip_permissions, plan, created_at, updated_at
		FROM agent_profiles WHERE agent_id = ? ORDER BY created_at DESC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
	if err := scanner.Scan(
		&profile.ID,
		&profile.AgentID,
		&profile.Name,
		&profile.Model,
		&autoApprove,
		&skipPermissions,
		&profile.Plan,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	); err != nil {
		return nil, err
	}
	profile.AutoApprove = autoApprove == 1
	profile.DangerouslySkipPermissions = skipPermissions == 1
	return profile, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
