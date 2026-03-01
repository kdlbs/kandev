package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/utility/models"
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
	// New simplified schema for utility agents
	schema := `
		CREATE TABLE IF NOT EXISTS utility_agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			prompt TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT 'claude-code',
			model TEXT NOT NULL DEFAULT '',
			builtin INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);

		CREATE TABLE IF NOT EXISTS utility_agent_calls (
			id TEXT PRIMARY KEY,
			utility_id TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			resolved_prompt TEXT NOT NULL DEFAULT '',
			response TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			response_tokens INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			FOREIGN KEY (utility_id) REFERENCES utility_agents(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_utility_calls_utility_id ON utility_agent_calls(utility_id);
		CREATE INDEX IF NOT EXISTS idx_utility_calls_session_id ON utility_agent_calls(session_id);
		CREATE INDEX IF NOT EXISTS idx_utility_calls_created_at ON utility_agent_calls(created_at);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	// Add enabled column if it doesn't exist (migration for existing DBs)
	_, _ = r.db.Exec(`ALTER TABLE utility_agents ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`)

	// Seed built-in agents
	if err := r.seedBuiltinAgents(); err != nil {
		return fmt.Errorf("failed to seed built-in agents: %w", err)
	}

	// Migration: fix template placeholders from {{.Key}} to {{Key}} format
	if err := r.migrateTemplatePlaceholders(); err != nil {
		return fmt.Errorf("failed to migrate template placeholders: %w", err)
	}

	return nil
}

// migrateTemplatePlaceholders fixes old {{.Key}} format to {{Key}} format in builtin prompts.
func (r *sqliteRepository) migrateTemplatePlaceholders() error {
	placeholders := []string{
		"GitDiff", "CommitLog", "ChangedFiles", "DiffSummary", "BranchName", "BaseBranch",
		"TaskTitle", "TaskDescription", "SessionID", "WorkspacePath", "UserPrompt",
	}
	for _, p := range placeholders {
		old := "{{." + p + "}}"
		new := "{{" + p + "}}"
		_, _ = r.db.Exec(r.db.Rebind(`UPDATE utility_agents SET prompt = REPLACE(prompt, ?, ?) WHERE builtin = 1`), old, new)
	}
	return nil
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	var errs []error
	if err := r.db.Close(); err != nil {
		errs = append(errs, err)
	}
	if r.ro != r.db {
		if err := r.ro.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (r *sqliteRepository) ListAgents(ctx context.Context) ([]*models.UtilityAgent, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, description, prompt, agent_id, model, builtin, enabled, created_at, updated_at
		FROM utility_agents
		ORDER BY builtin DESC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanAgentRows(rows)
}

func (r *sqliteRepository) scanAgentRows(rows *sql.Rows) ([]*models.UtilityAgent, error) {
	var agents []*models.UtilityAgent
	for rows.Next() {
		agent := &models.UtilityAgent{}
		var builtinInt, enabledInt int
		if err := rows.Scan(
			&agent.ID, &agent.Name, &agent.Description, &agent.Prompt,
			&agent.AgentID, &agent.Model, &builtinInt, &enabledInt,
			&agent.CreatedAt, &agent.UpdatedAt,
		); err != nil {
			return nil, err
		}
		agent.Builtin = builtinInt == 1
		agent.Enabled = enabledInt == 1
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

func (r *sqliteRepository) GetAgentByID(ctx context.Context, id string) (*models.UtilityAgent, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, description, prompt, agent_id, model, builtin, enabled, created_at, updated_at
		FROM utility_agents WHERE id = ?
	`), id)
	return r.scanAgentRow(row)
}

func (r *sqliteRepository) GetAgentByName(ctx context.Context, name string) (*models.UtilityAgent, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, description, prompt, agent_id, model, builtin, enabled, created_at, updated_at
		FROM utility_agents WHERE name = ?
	`), name)
	return r.scanAgentRow(row)
}

func (r *sqliteRepository) scanAgentRow(row *sql.Row) (*models.UtilityAgent, error) {
	agent := &models.UtilityAgent{}
	var builtinInt, enabledInt int
	if err := row.Scan(
		&agent.ID, &agent.Name, &agent.Description, &agent.Prompt,
		&agent.AgentID, &agent.Model, &builtinInt, &enabledInt,
		&agent.CreatedAt, &agent.UpdatedAt,
	); err != nil {
		return nil, err
	}
	agent.Builtin = builtinInt == 1
	agent.Enabled = enabledInt == 1
	return agent, nil
}

func (r *sqliteRepository) CreateAgent(ctx context.Context, agent *models.UtilityAgent) error {
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}
	agent.Name = strings.TrimSpace(agent.Name)
	agent.Description = strings.TrimSpace(agent.Description)
	agent.Prompt = strings.TrimSpace(agent.Prompt)
	if agent.CreatedAt.IsZero() {
		agent.CreatedAt = time.Now().UTC()
	}
	agent.UpdatedAt = time.Now().UTC()

	builtinInt := 0
	if agent.Builtin {
		builtinInt = 1
	}
	enabledInt := 0
	if agent.Enabled {
		enabledInt = 1
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO utility_agents (id, name, description, prompt, agent_id, model, builtin, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), agent.ID, agent.Name, agent.Description, agent.Prompt, agent.AgentID, agent.Model,
		builtinInt, enabledInt, agent.CreatedAt, agent.UpdatedAt)
	return err
}

func (r *sqliteRepository) UpdateAgent(ctx context.Context, agent *models.UtilityAgent) error {
	if agent == nil {
		return errors.New("agent is nil")
	}
	agent.Name = strings.TrimSpace(agent.Name)
	agent.Description = strings.TrimSpace(agent.Description)
	agent.Prompt = strings.TrimSpace(agent.Prompt)
	agent.UpdatedAt = time.Now().UTC()

	enabledInt := 0
	if agent.Enabled {
		enabledInt = 1
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE utility_agents
		SET name = ?, description = ?, prompt = ?, agent_id = ?, model = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`), agent.Name, agent.Description, agent.Prompt, agent.AgentID, agent.Model, enabledInt, agent.UpdatedAt, agent.ID)
	return err
}

func (r *sqliteRepository) DeleteAgent(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM utility_agents WHERE id = ? AND builtin = 0`), id)
	return err
}

func (r *sqliteRepository) ListCalls(ctx context.Context, utilityID string, limit int) ([]*models.UtilityAgentCall, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, utility_id, session_id, resolved_prompt, response, model, prompt_tokens, response_tokens, duration_ms, status, error_message, created_at, completed_at
		FROM utility_agent_calls
		WHERE utility_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`), utilityID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanCallRows(rows)
}

func (r *sqliteRepository) scanCallRows(rows *sql.Rows) ([]*models.UtilityAgentCall, error) {
	var calls []*models.UtilityAgentCall
	for rows.Next() {
		call := &models.UtilityAgentCall{}
		if err := rows.Scan(
			&call.ID, &call.UtilityID, &call.SessionID, &call.ResolvedPrompt, &call.Response,
			&call.Model, &call.PromptTokens, &call.ResponseTokens, &call.DurationMs,
			&call.Status, &call.ErrorMessage, &call.CreatedAt, &call.CompletedAt,
		); err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, rows.Err()
}

func (r *sqliteRepository) GetCallByID(ctx context.Context, id string) (*models.UtilityAgentCall, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, utility_id, session_id, resolved_prompt, response, model, prompt_tokens, response_tokens, duration_ms, status, error_message, created_at, completed_at
		FROM utility_agent_calls WHERE id = ?
	`), id)
	call := &models.UtilityAgentCall{}
	if err := row.Scan(
		&call.ID, &call.UtilityID, &call.SessionID, &call.ResolvedPrompt, &call.Response,
		&call.Model, &call.PromptTokens, &call.ResponseTokens, &call.DurationMs,
		&call.Status, &call.ErrorMessage, &call.CreatedAt, &call.CompletedAt,
	); err != nil {
		return nil, err
	}
	return call, nil
}

func (r *sqliteRepository) CreateCall(ctx context.Context, call *models.UtilityAgentCall) error {
	if call.ID == "" {
		call.ID = uuid.New().String()
	}
	if call.CreatedAt.IsZero() {
		call.CreatedAt = time.Now().UTC()
	}
	if call.Status == "" {
		call.Status = "pending"
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO utility_agent_calls (id, utility_id, session_id, resolved_prompt, response, model, prompt_tokens, response_tokens, duration_ms, status, error_message, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), call.ID, call.UtilityID, call.SessionID, call.ResolvedPrompt, call.Response,
		call.Model, call.PromptTokens, call.ResponseTokens, call.DurationMs,
		call.Status, call.ErrorMessage, call.CreatedAt, call.CompletedAt)
	return err
}

func (r *sqliteRepository) UpdateCall(ctx context.Context, call *models.UtilityAgentCall) error {
	if call == nil {
		return errors.New("call is nil")
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE utility_agent_calls
		SET response = ?, model = ?, prompt_tokens = ?, response_tokens = ?, duration_ms = ?, status = ?, error_message = ?, completed_at = ?
		WHERE id = ?
	`), call.Response, call.Model, call.PromptTokens, call.ResponseTokens, call.DurationMs,
		call.Status, call.ErrorMessage, call.CompletedAt, call.ID)
	return err
}
