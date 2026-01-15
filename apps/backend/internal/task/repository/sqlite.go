package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// SQLiteRepository provides SQLite-based task storage operations
type SQLiteRepository struct {
	db *sql.DB
}

// Ensure SQLiteRepository implements Repository interface
var _ Repository = (*SQLiteRepository)(nil)

// NewSQLiteRepository creates a new SQLite repository
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

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)

	repo := &SQLiteRepository{db: db}

	// Initialize schema
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

// initSchema creates the database tables if they don't exist
func (r *SQLiteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		owner_id TEXT DEFAULT '',
		default_executor_id TEXT DEFAULT '',
		default_environment_id TEXT DEFAULT '',
		default_agent_profile_id TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS executors (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		is_system INTEGER NOT NULL DEFAULT 0,
		config TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS environments (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		kind TEXT NOT NULL,
		is_system INTEGER NOT NULL DEFAULT 0,
		worktree_root TEXT DEFAULT '',
		image_tag TEXT DEFAULT '',
		dockerfile TEXT DEFAULT '',
		build_config TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS boards (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS columns (
		id TEXT PRIMARY KEY,
		board_id TEXT NOT NULL,
		name TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		state TEXT DEFAULT 'TODO',
		color TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		board_id TEXT NOT NULL,
		column_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		state TEXT DEFAULT 'TODO',
		priority INTEGER DEFAULT 0,
		repository_id TEXT DEFAULT '',
		base_branch TEXT DEFAULT '',
		assigned_to TEXT DEFAULT '',
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
		FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS repositories (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'local',
		local_path TEXT DEFAULT '',
		provider TEXT DEFAULT '',
		provider_repo_id TEXT DEFAULT '',
		provider_owner TEXT DEFAULT '',
		provider_name TEXT DEFAULT '',
		default_branch TEXT DEFAULT '',
		setup_script TEXT DEFAULT '',
		cleanup_script TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS repository_scripts (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		name TEXT NOT NULL,
		command TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_board_id ON tasks(board_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_column_id ON tasks(column_id);
	CREATE INDEX IF NOT EXISTS idx_columns_board_id ON columns(board_id);
	CREATE INDEX IF NOT EXISTS idx_repositories_workspace_id ON repositories(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_repository_scripts_repo_id ON repository_scripts(repository_id);

	CREATE TABLE IF NOT EXISTS task_messages (
		id TEXT PRIMARY KEY,
		agent_session_id TEXT NOT NULL,
		task_id TEXT DEFAULT '',
		author_type TEXT NOT NULL DEFAULT 'user',
		author_id TEXT DEFAULT '',
		content TEXT NOT NULL,
		requests_input INTEGER DEFAULT 0,
		type TEXT NOT NULL DEFAULT 'message',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON task_messages(agent_session_id);
	CREATE INDEX IF NOT EXISTS idx_messages_created_at ON task_messages(created_at);
	CREATE INDEX IF NOT EXISTS idx_messages_session_created ON task_messages(agent_session_id, created_at);

	CREATE TABLE IF NOT EXISTS agent_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_instance_id TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL,
		executor_id TEXT DEFAULT '',
		environment_id TEXT DEFAULT '',
		repository_id TEXT DEFAULT '',
		base_branch TEXT DEFAULT '',
		worktree_id TEXT DEFAULT '',
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		agent_profile_snapshot TEXT DEFAULT '{}',
		executor_snapshot TEXT DEFAULT '{}',
		environment_snapshot TEXT DEFAULT '{}',
		repository_snapshot TEXT DEFAULT '{}',
		state TEXT NOT NULL DEFAULT 'CREATED',
		progress INTEGER DEFAULT 0,
		error_message TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_agent_sessions_task_id ON agent_sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_agent_sessions_state ON agent_sessions(state);
	CREATE INDEX IF NOT EXISTS idx_agent_sessions_task_state ON agent_sessions(task_id, state);
	`

	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	if err := r.ensureColumn("boards", "workspace_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("tasks", "workspace_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("workspaces", "default_executor_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("workspaces", "default_environment_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("workspaces", "default_agent_profile_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("columns", "color", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("environments", "is_system", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureColumn("repositories", "deleted_at", "DATETIME"); err != nil {
		return err
	}
	if err := r.ensureColumn("executors", "deleted_at", "DATETIME"); err != nil {
		return err
	}
	if err := r.ensureColumn("environments", "deleted_at", "DATETIME"); err != nil {
		return err
	}

	// Ensure new message columns exist for existing databases
	if err := r.ensureColumn("task_messages", "type", "TEXT NOT NULL DEFAULT 'message'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_messages", "metadata", "TEXT DEFAULT '{}'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_messages", "agent_session_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Ensure container_id column exists for existing databases
	if err := r.ensureColumn("agent_sessions", "container_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("agent_sessions", "executor_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("agent_sessions", "environment_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("agent_sessions", "state", "TEXT NOT NULL DEFAULT 'CREATED'"); err != nil {
		return err
	}

	if err := r.ensureDefaultWorkspace(); err != nil {
		return err
	}

	if err := r.ensureDefaultExecutorsAndEnvironments(); err != nil {
		return err
	}

	if err := r.ensureWorkspaceIndexes(); err != nil {
		return err
	}

	return nil
}

func (r *SQLiteRepository) ensureColumn(table, column, definition string) error {
	exists, err := r.columnExists(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func (r *SQLiteRepository) columnExists(table, column string) (bool, error) {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (r *SQLiteRepository) ensureDefaultWorkspace() error {
	ctx := context.Background()

	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM workspaces").Scan(&count); err != nil {
		return err
	}

	if count == 0 {
		var boardCount int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM boards").Scan(&boardCount); err != nil {
			return err
		}
		var taskCount int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM tasks").Scan(&taskCount); err != nil {
			return err
		}
		defaultID := uuid.New().String()
		now := time.Now().UTC()
		workspaceName := "Default Workspace"
		workspaceDescription := "Default workspace"
		if boardCount > 0 || taskCount > 0 {
			workspaceName = "Migrated Workspace"
			workspaceDescription = ""
		}
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO workspaces (
				id,
				name,
				description,
				owner_id,
				default_executor_id,
				default_environment_id,
				default_agent_profile_id,
				created_at,
				updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, defaultID, workspaceName, workspaceDescription, "", nil, nil, nil, now, now); err != nil {
			return err
		}

		if boardCount == 0 && taskCount == 0 {
			boardID := uuid.New().String()
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO boards (id, workspace_id, name, description, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, boardID, defaultID, "Dev", "Default development board", now, now); err != nil {
				return err
			}

			type columnSeed struct {
				name     string
				position int
				state    string
				color    string
			}
			columns := []columnSeed{
				{name: "Todo", position: 0, state: string(v1.TaskStateTODO), color: "bg-neutral-400"},
				{name: "In Progress", position: 1, state: string(v1.TaskStateInProgress), color: "bg-blue-500"},
				{name: "Review", position: 2, state: string(v1.TaskStateReview), color: "bg-yellow-500"},
				{name: "Done", position: 3, state: string(v1.TaskStateCompleted), color: "bg-green-500"},
			}
			for _, column := range columns {
				if _, err := r.db.ExecContext(ctx, `
					INSERT INTO columns (id, board_id, name, position, state, color, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				`, uuid.New().String(), boardID, column.name, column.position, column.state, column.color, now, now); err != nil {
					return err
				}
			}
		}
	}

	var defaultWorkspaceID string
	if err := r.db.QueryRowContext(ctx, "SELECT id FROM workspaces ORDER BY created_at LIMIT 1").Scan(&defaultWorkspaceID); err != nil {
		return err
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE boards SET workspace_id = ? WHERE workspace_id = '' OR workspace_id IS NULL
	`, defaultWorkspaceID); err != nil {
		return err
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE tasks
		SET workspace_id = (
			SELECT workspace_id FROM boards WHERE boards.id = tasks.board_id
		)
		WHERE workspace_id = '' OR workspace_id IS NULL
	`); err != nil {
		return err
	}

	return nil
}

func (r *SQLiteRepository) ensureDefaultExecutorsAndEnvironments() error {
	ctx := context.Background()

	var executorCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM executors").Scan(&executorCount); err != nil {
		return err
	}

	if executorCount == 0 {
		now := time.Now().UTC()
		executors := []struct {
			id       string
			name     string
			execType models.ExecutorType
			status   models.ExecutorStatus
			isSystem bool
			config   map[string]string
		}{
			{
				id:       models.ExecutorIDLocalPC,
				name:     "Local PC",
				execType: models.ExecutorTypeLocalPC,
				status:   models.ExecutorStatusActive,
				isSystem: true,
				config:   map[string]string{},
			},
			{
				id:       models.ExecutorIDLocalDocker,
				name:     "Local Docker",
				execType: models.ExecutorTypeLocalDocker,
				status:   models.ExecutorStatusActive,
				isSystem: false,
				config: map[string]string{
					"docker_host": "unix:///var/run/docker.sock",
				},
			},
			{
				id:       models.ExecutorIDRemoteDocker,
				name:     "Remote Docker",
				execType: models.ExecutorTypeRemoteDocker,
				status:   models.ExecutorStatusDisabled,
				isSystem: false,
				config:   map[string]string{},
			},
		}

		for _, executor := range executors {
			configJSON, err := json.Marshal(executor.config)
			if err != nil {
				return fmt.Errorf("failed to serialize executor config: %w", err)
			}
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO executors (id, name, type, status, is_system, config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, executor.id, executor.name, executor.execType, executor.status, boolToInt(executor.isSystem), string(configJSON), now, now); err != nil {
				return err
			}
		}
	} else {
		if _, err := r.db.ExecContext(ctx, `
			UPDATE executors SET is_system = 1 WHERE id = ?
		`, models.ExecutorIDLocalPC); err != nil {
			return err
		}
	}

	var envCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM environments").Scan(&envCount); err != nil {
		return err
	}
	if envCount == 0 {
		now := time.Now().UTC()
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, models.EnvironmentIDLocal, "Local", models.EnvironmentKindLocalPC, boolToInt(true), "~/kandev", "", "", "{}", now, now); err != nil {
			return err
		}
	} else {
		var localCount int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM environments WHERE id = ?", models.EnvironmentIDLocal).Scan(&localCount); err != nil {
			return err
		}
		if localCount == 0 {
			now := time.Now().UTC()
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, models.EnvironmentIDLocal, "Local", models.EnvironmentKindLocalPC, boolToInt(true), "~/kandev", "", "", "{}", now, now); err != nil {
				return err
			}
		}
		if _, err := r.db.ExecContext(ctx, `
			UPDATE environments
			SET is_system = 1,
				image_tag = '',
				dockerfile = '',
				build_config = '{}'
			WHERE id = ?
		`, models.EnvironmentIDLocal); err != nil {
			return err
		}
		if _, err := r.db.ExecContext(ctx, `
			UPDATE environments
			SET worktree_root = ?
			WHERE id = ? AND (worktree_root IS NULL OR worktree_root = '')
		`, "~/kandev", models.EnvironmentIDLocal); err != nil {
			return err
		}
	}

	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (r *SQLiteRepository) ensureWorkspaceIndexes() error {
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)`); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_boards_workspace_id ON boards(workspace_id)`); err != nil {
		return err
	}
	return nil
}

func (r *SQLiteRepository) backfillTaskMappings() error {
	ctx := context.Background()

	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO task_boards (task_id, board_id)
		SELECT id, board_id FROM tasks WHERE board_id != ''
	`)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO task_columns (task_id, board_id, column_id, position)
		SELECT id, board_id, column_id, position
		FROM tasks
		WHERE board_id != '' AND column_id != ''
	`)
	return err
}

// Close closes the database connection
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// DB returns the underlying sql.DB instance for shared access
func (r *SQLiteRepository) DB() *sql.DB {
	return r.db
}

// Workspace operations

// CreateWorkspace creates a new workspace
func (r *SQLiteRepository) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	if workspace.ID == "" {
		workspace.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO workspaces (
			id,
			name,
			description,
			owner_id,
			default_executor_id,
			default_environment_id,
			default_agent_profile_id,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, workspace.ID, workspace.Name, workspace.Description, workspace.OwnerID, workspace.DefaultExecutorID, workspace.DefaultEnvironmentID, workspace.DefaultAgentProfileID, workspace.CreatedAt, workspace.UpdatedAt)

	return err
}

// GetWorkspace retrieves a workspace by ID
func (r *SQLiteRepository) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	workspace := &models.Workspace{}
	var defaultExecutorID sql.NullString
	var defaultEnvironmentID sql.NullString
	var defaultAgentProfileID sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, owner_id, default_executor_id, default_environment_id, default_agent_profile_id, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, id).Scan(
		&workspace.ID,
		&workspace.Name,
		&workspace.Description,
		&workspace.OwnerID,
		&defaultExecutorID,
		&defaultEnvironmentID,
		&defaultAgentProfileID,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
	)
	if defaultExecutorID.Valid && defaultExecutorID.String != "" {
		workspace.DefaultExecutorID = &defaultExecutorID.String
	}
	if defaultEnvironmentID.Valid && defaultEnvironmentID.String != "" {
		workspace.DefaultEnvironmentID = &defaultEnvironmentID.String
	}
	if defaultAgentProfileID.Valid && defaultAgentProfileID.String != "" {
		workspace.DefaultAgentProfileID = &defaultAgentProfileID.String
	}

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return workspace, err
}

// UpdateWorkspace updates an existing workspace
func (r *SQLiteRepository) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	workspace.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE workspaces
		SET name = ?,
			description = ?,
			default_executor_id = ?,
			default_environment_id = ?,
			default_agent_profile_id = ?,
			updated_at = ?
		WHERE id = ?
	`, workspace.Name, workspace.Description, workspace.DefaultExecutorID, workspace.DefaultEnvironmentID, workspace.DefaultAgentProfileID, workspace.UpdatedAt, workspace.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workspace not found: %s", workspace.ID)
	}
	return nil
}

// DeleteWorkspace deletes a workspace by ID
func (r *SQLiteRepository) DeleteWorkspace(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workspace not found: %s", id)
	}
	return nil
}

// ListWorkspaces returns all workspaces
func (r *SQLiteRepository) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, owner_id, default_executor_id, default_environment_id, default_agent_profile_id, created_at, updated_at
		FROM workspaces ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Workspace
	for rows.Next() {
		workspace := &models.Workspace{}
		var defaultExecutorID sql.NullString
		var defaultEnvironmentID sql.NullString
		var defaultAgentProfileID sql.NullString
		if err := rows.Scan(
			&workspace.ID,
			&workspace.Name,
			&workspace.Description,
			&workspace.OwnerID,
			&defaultExecutorID,
			&defaultEnvironmentID,
			&defaultAgentProfileID,
			&workspace.CreatedAt,
			&workspace.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if defaultExecutorID.Valid && defaultExecutorID.String != "" {
			workspace.DefaultExecutorID = &defaultExecutorID.String
		}
		if defaultEnvironmentID.Valid && defaultEnvironmentID.String != "" {
			workspace.DefaultEnvironmentID = &defaultEnvironmentID.String
		}
		if defaultAgentProfileID.Valid && defaultAgentProfileID.String != "" {
			workspace.DefaultAgentProfileID = &defaultAgentProfileID.String
		}
		result = append(result, workspace)
	}
	return result, rows.Err()
}

// Task operations

// CreateTask creates a new task
func (r *SQLiteRepository) CreateTask(ctx context.Context, task *models.Task) error {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, board_id, column_id, title, description, state, priority, repository_id, base_branch, assigned_to, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.WorkspaceID, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.RepositoryID, task.BaseBranch, task.AssignedTo, task.Position, string(metadata), task.CreatedAt, task.UpdatedAt)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// GetTask retrieves a task by ID
func (r *SQLiteRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task := &models.Task{}
	var metadata string
	var repositoryID, baseBranch, assignedTo sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, repository_id, base_branch, assigned_to, position, metadata, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.WorkspaceID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &repositoryID, &baseBranch, &assignedTo, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	task.RepositoryID = repositoryID.String
	task.BaseBranch = baseBranch.String
	task.AssignedTo = assignedTo.String
	_ = json.Unmarshal([]byte(metadata), &task.Metadata)
	return task, nil
}

// UpdateTask updates an existing task
func (r *SQLiteRepository) UpdateTask(ctx context.Context, task *models.Task) error {
	task.UpdatedAt = time.Now().UTC()

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET workspace_id = ?, board_id = ?, column_id = ?, title = ?, description = ?, state = ?, priority = ?, repository_id = ?, base_branch = ?, assigned_to = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, task.WorkspaceID, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.RepositoryID, task.BaseBranch, task.AssignedTo, task.Position, string(metadata), task.UpdatedAt, task.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", task.ID)
	}
	return nil
}

// DeleteTask deletes a task by ID
func (r *SQLiteRepository) DeleteTask(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ListTasks returns all tasks for a board
func (r *SQLiteRepository) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, repository_id, base_branch, assigned_to, position, metadata, created_at, updated_at
		FROM tasks
		WHERE board_id = ?
		ORDER BY position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

// ListTasksByColumn returns all tasks in a column
func (r *SQLiteRepository) ListTasksByColumn(ctx context.Context, columnID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, repository_id, base_branch, assigned_to, position, metadata, created_at, updated_at
		FROM tasks
		WHERE column_id = ? ORDER BY position
	`, columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

func (r *SQLiteRepository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		var repositoryID, baseBranch, assignedTo sql.NullString
		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.BoardID,
			&task.ColumnID,
			&task.Title,
			&task.Description,
			&task.State,
			&task.Priority,
			&repositoryID,
			&baseBranch,
			&assignedTo,
			&task.Position,
			&metadata,
			&task.CreatedAt,
			&task.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		task.RepositoryID = repositoryID.String
		task.BaseBranch = baseBranch.String
		task.AssignedTo = assignedTo.String
		_ = json.Unmarshal([]byte(metadata), &task.Metadata)
		result = append(result, task)
	}
	return result, rows.Err()
}

// UpdateTaskState updates the state of a task
func (r *SQLiteRepository) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
	result, err := r.db.ExecContext(ctx, `UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`, state, time.Now().UTC(), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// AddTaskToBoard adds a task to a board with placement
func (r *SQLiteRepository) AddTaskToBoard(ctx context.Context, taskID, boardID, columnID string, position int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET board_id = ?, column_id = ?, position = ?, updated_at = ? WHERE id = ?
	`, boardID, columnID, position, time.Now().UTC(), taskID)
	return err
}

// RemoveTaskFromBoard removes a task from a board
func (r *SQLiteRepository) RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET board_id = '', column_id = '', position = 0, updated_at = ? WHERE id = ? AND board_id = ?
	`, time.Now().UTC(), taskID, boardID)
	return err
}

func (r *SQLiteRepository) addTaskToBoardTx(ctx context.Context, tx *sql.Tx, taskID, boardID, columnID string, position int) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE tasks SET board_id = ?, column_id = ?, position = ?, updated_at = ? WHERE id = ?
	`, boardID, columnID, position, time.Now().UTC(), taskID)
	return err
}

// Board operations

// CreateBoard creates a new board
func (r *SQLiteRepository) CreateBoard(ctx context.Context, board *models.Board) error {
	if board.ID == "" {
		board.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	board.CreatedAt = now
	board.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO boards (id, workspace_id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, board.ID, board.WorkspaceID, board.Name, board.Description, board.CreatedAt, board.UpdatedAt)

	return err
}

// GetBoard retrieves a board by ID
func (r *SQLiteRepository) GetBoard(ctx context.Context, id string) (*models.Board, error) {
	board := &models.Board{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, description, created_at, updated_at
		FROM boards WHERE id = ?
	`, id).Scan(&board.ID, &board.WorkspaceID, &board.Name, &board.Description, &board.CreatedAt, &board.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("board not found: %s", id)
	}
	return board, err
}

// UpdateBoard updates an existing board
func (r *SQLiteRepository) UpdateBoard(ctx context.Context, board *models.Board) error {
	board.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE boards SET name = ?, description = ?, updated_at = ? WHERE id = ?
	`, board.Name, board.Description, board.UpdatedAt, board.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("board not found: %s", board.ID)
	}
	return nil
}

// DeleteBoard deletes a board by ID
func (r *SQLiteRepository) DeleteBoard(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM boards WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("board not found: %s", id)
	}
	return nil
}

// ListBoards returns all boards
func (r *SQLiteRepository) ListBoards(ctx context.Context, workspaceID string) ([]*models.Board, error) {
	query := `
		SELECT id, workspace_id, name, description, created_at, updated_at FROM boards
	`
	var args []interface{}
	if workspaceID != "" {
		query += " WHERE workspace_id = ?"
		args = append(args, workspaceID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Board
	for rows.Next() {
		board := &models.Board{}
		err := rows.Scan(&board.ID, &board.WorkspaceID, &board.Name, &board.Description, &board.CreatedAt, &board.UpdatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, board)
	}
	return result, rows.Err()
}

// Column operations

// CreateColumn creates a new column
func (r *SQLiteRepository) CreateColumn(ctx context.Context, column *models.Column) error {
	if column.ID == "" {
		column.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	column.CreatedAt = now
	column.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO columns (id, board_id, name, position, state, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, column.ID, column.BoardID, column.Name, column.Position, column.State, column.Color, column.CreatedAt, column.UpdatedAt)

	return err
}

// GetColumn retrieves a column by ID
func (r *SQLiteRepository) GetColumn(ctx context.Context, id string) (*models.Column, error) {
	column := &models.Column{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, board_id, name, position, state, color, created_at, updated_at
		FROM columns WHERE id = ?
	`, id).Scan(&column.ID, &column.BoardID, &column.Name, &column.Position, &column.State, &column.Color, &column.CreatedAt, &column.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("column not found: %s", id)
	}
	return column, err
}

// GetColumnByState retrieves a column by board ID and state
func (r *SQLiteRepository) GetColumnByState(ctx context.Context, boardID string, state v1.TaskState) (*models.Column, error) {
	column := &models.Column{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, board_id, name, position, state, color, created_at, updated_at
		FROM columns WHERE board_id = ? AND state = ?
	`, boardID, state).Scan(&column.ID, &column.BoardID, &column.Name, &column.Position, &column.State, &column.Color, &column.CreatedAt, &column.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("column not found for board %s with state %s", boardID, state)
	}
	return column, err
}

// UpdateColumn updates an existing column
func (r *SQLiteRepository) UpdateColumn(ctx context.Context, column *models.Column) error {
	column.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE columns SET name = ?, position = ?, state = ?, color = ?, updated_at = ? WHERE id = ?
	`, column.Name, column.Position, column.State, column.Color, column.UpdatedAt, column.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("column not found: %s", column.ID)
	}
	return nil
}

// DeleteColumn deletes a column by ID
func (r *SQLiteRepository) DeleteColumn(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM columns WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("column not found: %s", id)
	}
	return nil
}

// ListColumns returns all columns for a board
func (r *SQLiteRepository) ListColumns(ctx context.Context, boardID string) ([]*models.Column, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, board_id, name, position, state, color, created_at, updated_at
		FROM columns WHERE board_id = ? ORDER BY position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Column
	for rows.Next() {
		column := &models.Column{}
		err := rows.Scan(&column.ID, &column.BoardID, &column.Name, &column.Position, &column.State, &column.Color, &column.CreatedAt, &column.UpdatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, column)
	}
	return result, rows.Err()
}

// Message operations

// CreateMessage creates a new message
func (r *SQLiteRepository) CreateMessage(ctx context.Context, message *models.Message) error {
	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorUser
	}

	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}

	messageType := string(message.Type)
	if messageType == "" {
		messageType = string(models.MessageTypeMessage)
	}

	metadataJSON := "{}"
	if message.Metadata != nil {
		metadataBytes, err := json.Marshal(message.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize message metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_messages (id, agent_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, message.ID, message.AgentSessionID, message.TaskID, message.AuthorType, message.AuthorID, message.Content, requestsInput, messageType, metadataJSON, message.CreatedAt)

	return err
}

// GetMessage retrieves a message by ID
func (r *SQLiteRepository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, agent_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_messages WHERE id = ?
	`, id).Scan(&message.ID, &message.AgentSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)

	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize message metadata: %w", err)
		}
	}

	return message, nil
}

// ListMessages returns all messages for a session ordered by creation time.
func (r *SQLiteRepository) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, agent_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_messages WHERE agent_session_id = ? ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var requestsInput int
		var messageType string
		var metadataJSON string
		err := rows.Scan(&message.ID, &message.AgentSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
		if err != nil {
			return nil, err
		}
		message.RequestsInput = requestsInput == 1
		message.Type = models.MessageType(messageType)

		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize message metadata: %w", err)
			}
		}

		result = append(result, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListMessagesPaginated returns messages for a session ordered by creation time with pagination.
func (r *SQLiteRepository) ListMessagesPaginated(ctx context.Context, sessionID string, opts ListMessagesOptions) ([]*models.Message, bool, error) {
	limit := opts.Limit
	if limit < 0 {
		limit = 0
	}

	sortDir := "ASC"
	if strings.EqualFold(opts.Sort, "desc") {
		sortDir = "DESC"
	}

	var cursor *models.Message
	if opts.Before != "" {
		var err error
		cursor, err = r.GetMessage(ctx, opts.Before)
		if err != nil {
			return nil, false, err
		}
		if cursor.AgentSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.Before)
		}
	}
	if opts.After != "" {
		var err error
		cursor, err = r.GetMessage(ctx, opts.After)
		if err != nil {
			return nil, false, err
		}
		if cursor.AgentSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.After)
		}
	}

	query := `
		SELECT id, agent_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_messages WHERE agent_session_id = ?`
	args := []interface{}{sessionID}
	if cursor != nil {
		if opts.Before != "" {
			query += " AND (created_at < ? OR (created_at = ? AND id < ?))"
		} else if opts.After != "" {
			query += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		}
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	query += fmt.Sprintf(" ORDER BY created_at %s, id %s", sortDir, sortDir)
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit+1)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var result []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var requestsInput int
		var messageType string
		var metadataJSON string
		err := rows.Scan(&message.ID, &message.AgentSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
		if err != nil {
			return nil, false, err
		}
		message.RequestsInput = requestsInput == 1
		message.Type = models.MessageType(messageType)

		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
				return nil, false, fmt.Errorf("failed to deserialize message metadata: %w", err)
			}
		}

		result = append(result, message)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := false
	if limit > 0 && len(result) > limit {
		hasMore = true
		result = result[:limit]
	}
	return result, hasMore, nil
}

// DeleteMessage deletes a message by ID
func (r *SQLiteRepository) DeleteMessage(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_messages WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", id)
	}
	return nil
}

// GetMessageByToolCallID retrieves a message by session ID and tool_call_id in metadata
func (r *SQLiteRepository) GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, agent_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_messages WHERE agent_session_id = ? AND json_extract(metadata, '$.tool_call_id') = ?
	`, sessionID, toolCallID).Scan(&message.ID, &message.AgentSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)
	if metadataJSON != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &message.Metadata)
	}
	return message, nil
}

// UpdateMessage updates an existing message
func (r *SQLiteRepository) UpdateMessage(ctx context.Context, message *models.Message) error {
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE task_messages SET content = ?, requests_input = ?, type = ?, metadata = ?
		WHERE id = ?
	`, message.Content, requestsInput, string(message.Type), string(metadataJSON), message.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", message.ID)
	}
	return nil
}

// Repository operations

// CreateRepository creates a new repository
func (r *SQLiteRepository) CreateRepository(ctx context.Context, repository *models.Repository) error {
	if repository.ID == "" {
		repository.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repository.CreatedAt = now
	repository.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO repositories (
			id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
			provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, repository.ID, repository.WorkspaceID, repository.Name, repository.SourceType, repository.LocalPath, repository.Provider,
		repository.ProviderRepoID, repository.ProviderOwner, repository.ProviderName, repository.DefaultBranch,
		repository.SetupScript, repository.CleanupScript, repository.CreatedAt, repository.UpdatedAt, repository.DeletedAt)

	return err
}

// GetRepository retrieves a repository by ID
func (r *SQLiteRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	repository := &models.Repository{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
		       provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at, deleted_at
		FROM repositories WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderOwner, &repository.ProviderName,
		&repository.DefaultBranch, &repository.SetupScript, &repository.CleanupScript, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("repository not found: %s", id)
	}
	return repository, err
}

// UpdateRepository updates an existing repository
func (r *SQLiteRepository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	repository.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE repositories SET
			name = ?, source_type = ?, local_path = ?, provider = ?, provider_repo_id = ?, provider_owner = ?,
			provider_name = ?, default_branch = ?, setup_script = ?, cleanup_script = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, repository.Name, repository.SourceType, repository.LocalPath, repository.Provider, repository.ProviderRepoID,
		repository.ProviderOwner, repository.ProviderName, repository.DefaultBranch, repository.SetupScript, repository.CleanupScript,
		repository.UpdatedAt, repository.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository not found: %s", repository.ID)
	}
	return nil
}

// DeleteRepository deletes a repository by ID
func (r *SQLiteRepository) DeleteRepository(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE repositories SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository not found: %s", id)
	}
	return nil
}

// ListRepositories returns all repositories for a workspace
func (r *SQLiteRepository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
		       provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at, deleted_at
		FROM repositories WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Repository
	for rows.Next() {
		repository := &models.Repository{}
		err := rows.Scan(
			&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
			&repository.Provider, &repository.ProviderRepoID, &repository.ProviderOwner, &repository.ProviderName,
			&repository.DefaultBranch, &repository.SetupScript, &repository.CleanupScript, &repository.CreatedAt, &repository.UpdatedAt, &repository.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, repository)
	}
	return result, rows.Err()
}

// Repository script operations

// CreateRepositoryScript creates a new repository script
func (r *SQLiteRepository) CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	if script.ID == "" {
		script.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	script.CreatedAt = now
	script.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO repository_scripts (id, repository_id, name, command, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, script.ID, script.RepositoryID, script.Name, script.Command, script.Position, script.CreatedAt, script.UpdatedAt)

	return err
}

// GetRepositoryScript retrieves a repository script by ID
func (r *SQLiteRepository) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	script := &models.RepositoryScript{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts WHERE id = ?
	`, id).Scan(&script.ID, &script.RepositoryID, &script.Name, &script.Command, &script.Position, &script.CreatedAt, &script.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("repository script not found: %s", id)
	}
	return script, err
}

// UpdateRepositoryScript updates an existing repository script
func (r *SQLiteRepository) UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	script.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE repository_scripts SET name = ?, command = ?, position = ?, updated_at = ? WHERE id = ?
	`, script.Name, script.Command, script.Position, script.UpdatedAt, script.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository script not found: %s", script.ID)
	}
	return nil
}

// DeleteRepositoryScript deletes a repository script by ID
func (r *SQLiteRepository) DeleteRepositoryScript(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM repository_scripts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repository script not found: %s", id)
	}
	return nil
}

// ListRepositoryScripts returns all scripts for a repository
func (r *SQLiteRepository) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts WHERE repository_id = ? ORDER BY position
	`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.RepositoryScript
	for rows.Next() {
		script := &models.RepositoryScript{}
		err := rows.Scan(&script.ID, &script.RepositoryID, &script.Name, &script.Command, &script.Position, &script.CreatedAt, &script.UpdatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, script)
	}
	return result, rows.Err()
}

// Agent Session operations

// CreateAgentSession creates a new agent session
func (r *SQLiteRepository) CreateAgentSession(ctx context.Context, session *models.AgentSession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	session.StartedAt = now
	session.UpdatedAt = now
	if session.State == "" {
		session.State = models.AgentSessionStateCreated
	}

	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize agent session metadata: %w", err)
	}
	agentProfileSnapshotJSON, err := json.Marshal(session.AgentProfileSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize agent profile snapshot: %w", err)
	}
	executorSnapshotJSON, err := json.Marshal(session.ExecutorSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize executor snapshot: %w", err)
	}
	environmentSnapshotJSON, err := json.Marshal(session.EnvironmentSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize environment snapshot: %w", err)
	}
	repositorySnapshotJSON, err := json.Marshal(session.RepositorySnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize repository snapshot: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO agent_sessions (
			id, task_id, agent_instance_id, container_id, agent_profile_id, executor_id, environment_id,
			repository_id, base_branch, worktree_id, worktree_path, worktree_branch,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, progress, error_message, metadata, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.TaskID, session.AgentInstanceID, session.ContainerID, session.AgentProfileID,
		session.ExecutorID, session.EnvironmentID, session.RepositoryID, session.BaseBranch, session.WorktreeID, session.WorktreePath, session.WorktreeBranch,
		string(agentProfileSnapshotJSON), string(executorSnapshotJSON), string(environmentSnapshotJSON), string(repositorySnapshotJSON),
		string(session.State), session.Progress, session.ErrorMessage, string(metadataJSON),
		session.StartedAt, session.CompletedAt, session.UpdatedAt)

	return err
}

// GetAgentSession retrieves an agent session by ID
func (r *SQLiteRepository) GetAgentSession(ctx context.Context, id string) (*models.AgentSession, error) {
	session := &models.AgentSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_instance_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch, worktree_id, worktree_path, worktree_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions WHERE id = ?
	`, id).Scan(
		&session.ID, &session.TaskID, &session.AgentInstanceID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch, &session.WorktreeID, &session.WorktreePath, &session.WorktreeBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.AgentSessionState(state)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}
	if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
		}
	}
	if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
		}
	}
	if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
		}
	}
	if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
		}
	}

	return session, nil
}

// GetAgentSessionByTaskID retrieves the most recent agent session for a task
func (r *SQLiteRepository) GetAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error) {
	session := &models.AgentSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_instance_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch, worktree_id, worktree_path, worktree_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions WHERE task_id = ? ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentInstanceID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch, &session.WorktreeID, &session.WorktreePath, &session.WorktreeBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.AgentSessionState(state)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}
	if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
		}
	}
	if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
		}
	}
	if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
		}
	}
	if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
		}
	}

	return session, nil
}

// GetActiveAgentSessionByTaskID retrieves the active (running/waiting) agent session for a task
func (r *SQLiteRepository) GetActiveAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error) {
	session := &models.AgentSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_instance_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch, worktree_id, worktree_path, worktree_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions
		WHERE task_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentInstanceID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch, &session.WorktreeID, &session.WorktreePath, &session.WorktreeBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active agent session for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.AgentSessionState(state)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}
	if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
		}
	}
	if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
		}
	}
	if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
		}
	}
	if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
		if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
			return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
		}
	}

	return session, nil
}

// UpdateAgentSession updates an existing agent session
func (r *SQLiteRepository) UpdateAgentSession(ctx context.Context, session *models.AgentSession) error {
	session.UpdatedAt = time.Now().UTC()

	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize agent session metadata: %w", err)
	}
	agentProfileSnapshotJSON, err := json.Marshal(session.AgentProfileSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize agent profile snapshot: %w", err)
	}
	executorSnapshotJSON, err := json.Marshal(session.ExecutorSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize executor snapshot: %w", err)
	}
	environmentSnapshotJSON, err := json.Marshal(session.EnvironmentSnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize environment snapshot: %w", err)
	}
	repositorySnapshotJSON, err := json.Marshal(session.RepositorySnapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize repository snapshot: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_sessions SET
			agent_instance_id = ?, container_id = ?, agent_profile_id = ?, executor_id = ?, environment_id = ?,
			repository_id = ?, base_branch = ?, worktree_id = ?, worktree_path = ?, worktree_branch = ?,
			agent_profile_snapshot = ?, executor_snapshot = ?, environment_snapshot = ?, repository_snapshot = ?,
			state = ?, progress = ?, error_message = ?, metadata = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, session.AgentInstanceID, session.ContainerID, session.AgentProfileID, session.ExecutorID, session.EnvironmentID,
		session.RepositoryID, session.BaseBranch, session.WorktreeID, session.WorktreePath, session.WorktreeBranch,
		string(agentProfileSnapshotJSON), string(executorSnapshotJSON), string(environmentSnapshotJSON), string(repositorySnapshotJSON),
		string(session.State), session.Progress, session.ErrorMessage, string(metadataJSON), session.CompletedAt,
		session.UpdatedAt, session.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", session.ID)
	}
	return nil
}

// UpdateAgentSessionState updates just the state and error message of an agent session
func (r *SQLiteRepository) UpdateAgentSessionState(ctx context.Context, id string, status models.AgentSessionState, errorMessage string) error {
	now := time.Now().UTC()

	var completedAt *time.Time
	if status == models.AgentSessionStateCompleted || status == models.AgentSessionStateFailed || status == models.AgentSessionStateCancelled {
		completedAt = &now
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_sessions SET state = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?
	`, string(status), errorMessage, completedAt, now, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// ListAgentSessions returns all agent sessions for a task
func (r *SQLiteRepository) ListAgentSessions(ctx context.Context, taskID string) ([]*models.AgentSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_instance_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch, worktree_id, worktree_path, worktree_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions WHERE task_id = ? ORDER BY started_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanAgentSessions(rows)
}

// ListActiveAgentSessions returns all active agent sessions across all tasks
func (r *SQLiteRepository) ListActiveAgentSessions(ctx context.Context) ([]*models.AgentSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_instance_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch, worktree_id, worktree_path, worktree_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions WHERE state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT') ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanAgentSessions(rows)
}

func (r *SQLiteRepository) HasActiveAgentSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM agent_sessions
		WHERE agent_profile_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, agentProfileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *SQLiteRepository) HasActiveAgentSessionsByExecutor(ctx context.Context, executorID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM agent_sessions
		WHERE executor_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, executorID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *SQLiteRepository) HasActiveAgentSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM agent_sessions
		WHERE environment_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, environmentID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *SQLiteRepository) HasActiveAgentSessionsByRepository(ctx context.Context, repositoryID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1
		FROM agent_sessions s
		INNER JOIN tasks t ON t.id = s.task_id
		WHERE s.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
			AND t.repository_id = ?
		LIMIT 1
	`, repositoryID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// scanAgentSessions is a helper to scan multiple agent session rows
func (r *SQLiteRepository) scanAgentSessions(rows *sql.Rows) ([]*models.AgentSession, error) {
	var result []*models.AgentSession
	for rows.Next() {
		session := &models.AgentSession{}
		var state string
		var metadataJSON string
		var agentProfileSnapshotJSON string
		var executorSnapshotJSON string
		var environmentSnapshotJSON string
		var repositorySnapshotJSON string
		var completedAt sql.NullTime

		err := rows.Scan(
			&session.ID, &session.TaskID, &session.AgentInstanceID, &session.ContainerID, &session.AgentProfileID,
			&session.ExecutorID, &session.EnvironmentID,
			&session.RepositoryID, &session.BaseBranch, &session.WorktreeID, &session.WorktreePath, &session.WorktreeBranch,
			&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
			&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.State = models.AgentSessionState(state)
		if completedAt.Valid {
			session.CompletedAt = &completedAt.Time
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
			}
		}
		if agentProfileSnapshotJSON != "" && agentProfileSnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(agentProfileSnapshotJSON), &session.AgentProfileSnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize agent profile snapshot: %w", err)
			}
		}
		if executorSnapshotJSON != "" && executorSnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(executorSnapshotJSON), &session.ExecutorSnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize executor snapshot: %w", err)
			}
		}
		if environmentSnapshotJSON != "" && environmentSnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(environmentSnapshotJSON), &session.EnvironmentSnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize environment snapshot: %w", err)
			}
		}
		if repositorySnapshotJSON != "" && repositorySnapshotJSON != "{}" {
			if err := json.Unmarshal([]byte(repositorySnapshotJSON), &session.RepositorySnapshot); err != nil {
				return nil, fmt.Errorf("failed to deserialize repository snapshot: %w", err)
			}
		}

		result = append(result, session)
	}
	return result, rows.Err()
}

// DeleteAgentSession deletes an agent session by ID
func (r *SQLiteRepository) DeleteAgentSession(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM agent_sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// Executor operations

func (r *SQLiteRepository) CreateExecutor(ctx context.Context, executor *models.Executor) error {
	if executor.ID == "" {
		executor.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	executor.CreatedAt = now
	executor.UpdatedAt = now

	configJSON, err := json.Marshal(executor.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize executor config: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO executors (id, name, type, status, is_system, config, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, executor.ID, executor.Name, executor.Type, executor.Status, boolToInt(executor.IsSystem), string(configJSON), executor.CreatedAt, executor.UpdatedAt, executor.DeletedAt)
	return err
}

func (r *SQLiteRepository) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	executor := &models.Executor{}
	var configJSON string
	var isSystem int

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, type, status, is_system, config, created_at, updated_at, deleted_at
		FROM executors WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(
		&executor.ID, &executor.Name, &executor.Type, &executor.Status,
		&isSystem, &configJSON, &executor.CreatedAt, &executor.UpdatedAt, &executor.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("executor not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	executor.IsSystem = isSystem == 1
	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &executor.Config); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor config: %w", err)
		}
	}
	return executor, nil
}

func (r *SQLiteRepository) UpdateExecutor(ctx context.Context, executor *models.Executor) error {
	executor.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(executor.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize executor config: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE executors SET name = ?, type = ?, status = ?, is_system = ?, config = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, executor.Name, executor.Type, executor.Status, boolToInt(executor.IsSystem), string(configJSON), executor.UpdatedAt, executor.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor not found: %s", executor.ID)
	}
	return nil
}

func (r *SQLiteRepository) DeleteExecutor(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE executors SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor not found: %s", id)
	}
	return nil
}

func (r *SQLiteRepository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, type, status, is_system, config, created_at, updated_at, deleted_at
		FROM executors WHERE deleted_at IS NULL ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Executor
	for rows.Next() {
		executor := &models.Executor{}
		var configJSON string
		var isSystem int
		if err := rows.Scan(
			&executor.ID, &executor.Name, &executor.Type, &executor.Status,
			&isSystem, &configJSON, &executor.CreatedAt, &executor.UpdatedAt, &executor.DeletedAt,
		); err != nil {
			return nil, err
		}
		executor.IsSystem = isSystem == 1
		if configJSON != "" && configJSON != "{}" {
			if err := json.Unmarshal([]byte(configJSON), &executor.Config); err != nil {
				return nil, fmt.Errorf("failed to deserialize executor config: %w", err)
			}
		}
		result = append(result, executor)
	}
	return result, rows.Err()
}

// Environment operations

func (r *SQLiteRepository) CreateEnvironment(ctx context.Context, environment *models.Environment) error {
	if environment.ID == "" {
		environment.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	environment.CreatedAt = now
	environment.UpdatedAt = now

	buildConfigJSON, err := json.Marshal(environment.BuildConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize environment build config: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO environments (id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, environment.ID, environment.Name, environment.Kind, boolToInt(environment.IsSystem), environment.WorktreeRoot, environment.ImageTag, environment.Dockerfile, string(buildConfigJSON), environment.CreatedAt, environment.UpdatedAt, environment.DeletedAt)
	return err
}

func (r *SQLiteRepository) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	environment := &models.Environment{}
	var buildConfigJSON string
	var isSystem int

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at, deleted_at
		FROM environments WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(
		&environment.ID, &environment.Name, &environment.Kind, &isSystem, &environment.WorktreeRoot,
		&environment.ImageTag, &environment.Dockerfile, &buildConfigJSON,
		&environment.CreatedAt, &environment.UpdatedAt, &environment.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	environment.IsSystem = isSystem == 1
	if buildConfigJSON != "" && buildConfigJSON != "{}" {
		if err := json.Unmarshal([]byte(buildConfigJSON), &environment.BuildConfig); err != nil {
			return nil, fmt.Errorf("failed to deserialize environment build config: %w", err)
		}
	}
	return environment, nil
}

func (r *SQLiteRepository) UpdateEnvironment(ctx context.Context, environment *models.Environment) error {
	environment.UpdatedAt = time.Now().UTC()

	buildConfigJSON, err := json.Marshal(environment.BuildConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize environment build config: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE environments SET name = ?, kind = ?, is_system = ?, worktree_root = ?, image_tag = ?, dockerfile = ?, build_config = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, environment.Name, environment.Kind, boolToInt(environment.IsSystem), environment.WorktreeRoot, environment.ImageTag, environment.Dockerfile, string(buildConfigJSON), environment.UpdatedAt, environment.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("environment not found: %s", environment.ID)
	}
	return nil
}

func (r *SQLiteRepository) DeleteEnvironment(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		UPDATE environments SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("environment not found: %s", id)
	}
	return nil
}

func (r *SQLiteRepository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at, deleted_at
		FROM environments WHERE deleted_at IS NULL ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Environment
	for rows.Next() {
		environment := &models.Environment{}
		var buildConfigJSON string
		var isSystem int
		if err := rows.Scan(
			&environment.ID, &environment.Name, &environment.Kind, &isSystem, &environment.WorktreeRoot,
			&environment.ImageTag, &environment.Dockerfile, &buildConfigJSON,
			&environment.CreatedAt, &environment.UpdatedAt, &environment.DeletedAt,
		); err != nil {
			return nil, err
		}
		environment.IsSystem = isSystem == 1
		if buildConfigJSON != "" && buildConfigJSON != "{}" {
			if err := json.Unmarshal([]byte(buildConfigJSON), &environment.BuildConfig); err != nil {
				return nil, fmt.Errorf("failed to deserialize environment build config: %w", err)
			}
		}
		result = append(result, environment)
	}
	return result, rows.Err()
}
