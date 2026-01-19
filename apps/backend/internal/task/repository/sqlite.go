package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// sqliteRepository provides SQLite-based task storage operations.
type sqliteRepository struct {
	db     *sql.DB
	ownsDB bool
}

// Ensure sqliteRepository implements Repository interface
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

// initSchema creates the database tables if they don't exist
func (r *sqliteRepository) initSchema() error {
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
		resumable INTEGER NOT NULL DEFAULT 1,
		config TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS executors_running (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL UNIQUE,
		task_id TEXT NOT NULL,
		executor_id TEXT NOT NULL,
		runtime TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'starting',
		resumable INTEGER NOT NULL DEFAULT 0,
		resume_token TEXT DEFAULT '',
		agent_execution_id TEXT DEFAULT '',
		container_id TEXT DEFAULT '',
		agentctl_url TEXT DEFAULT '',
		agentctl_port INTEGER DEFAULT 0,
		pid INTEGER DEFAULT 0,
		worktree_id TEXT DEFAULT '',
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		last_seen_at DATETIME,
		error_message TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
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
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
		FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS task_repositories (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		base_branch TEXT DEFAULT '',
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
		UNIQUE(task_id, repository_id)
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
	CREATE INDEX IF NOT EXISTS idx_task_repositories_task_id ON task_repositories(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_repositories_repository_id ON task_repositories(repository_id);
	CREATE INDEX IF NOT EXISTS idx_columns_board_id ON columns(board_id);
	CREATE INDEX IF NOT EXISTS idx_repositories_workspace_id ON repositories(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_repository_scripts_repo_id ON repository_scripts(repository_id);

	CREATE TABLE IF NOT EXISTS task_session_messages (
		id TEXT PRIMARY KEY,
		task_session_id TEXT NOT NULL,
		task_id TEXT DEFAULT '',
		author_type TEXT NOT NULL DEFAULT 'user',
		author_id TEXT DEFAULT '',
		content TEXT NOT NULL,
		requests_input INTEGER DEFAULT 0,
		type TEXT NOT NULL DEFAULT 'message',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON task_session_messages(task_session_id);
	CREATE INDEX IF NOT EXISTS idx_messages_created_at ON task_session_messages(created_at);
	CREATE INDEX IF NOT EXISTS idx_messages_session_created ON task_session_messages(task_session_id, created_at);

	CREATE TABLE IF NOT EXISTS task_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_execution_id TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL,
		executor_id TEXT DEFAULT '',
		environment_id TEXT DEFAULT '',
		repository_id TEXT DEFAULT '',
		base_branch TEXT DEFAULT '',
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

	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_id ON task_sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_state ON task_sessions(state);
	CREATE INDEX IF NOT EXISTS idx_task_sessions_task_state ON task_sessions(task_id, state);

	CREATE TABLE IF NOT EXISTS task_session_worktrees (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		worktree_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		merged_at DATETIME,
		deleted_at DATETIME,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE,
		UNIQUE(session_id, worktree_id)
	);

	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_session_id ON task_session_worktrees(session_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_worktree_id ON task_session_worktrees(worktree_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_repository_id ON task_session_worktrees(repository_id);
	CREATE INDEX IF NOT EXISTS idx_task_session_worktrees_status ON task_session_worktrees(status);
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
	if err := r.ensureColumn("executors", "resumable", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := r.ensureColumn("environments", "deleted_at", "DATETIME"); err != nil {
		return err
	}

	// Ensure new message columns exist for existing databases
	if err := r.ensureColumn("task_session_messages", "type", "TEXT NOT NULL DEFAULT 'message'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_session_messages", "metadata", "TEXT DEFAULT '{}'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_session_messages", "task_session_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Ensure container_id column exists for existing databases
	if err := r.ensureColumn("task_sessions", "container_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_sessions", "executor_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_sessions", "environment_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_sessions", "state", "TEXT NOT NULL DEFAULT 'CREATED'"); err != nil {
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

func (r *sqliteRepository) ensureColumn(table, column, definition string) error {
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

func (r *sqliteRepository) columnExists(table, column string) (bool, error) {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

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

func (r *sqliteRepository) ensureDefaultWorkspace() error {
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

func (r *sqliteRepository) ensureDefaultExecutorsAndEnvironments() error {
	ctx := context.Background()

	var executorCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM executors").Scan(&executorCount); err != nil {
		return err
	}

	if executorCount == 0 {
		now := time.Now().UTC()
		executors := []struct {
			id        string
			name      string
			execType  models.ExecutorType
			status    models.ExecutorStatus
			isSystem  bool
			resumable bool
			config    map[string]string
		}{
			{
				id:        models.ExecutorIDLocalPC,
				name:      "Local PC",
				execType:  models.ExecutorTypeLocalPC,
				status:    models.ExecutorStatusActive,
				isSystem:  true,
				resumable: true,
				config:    map[string]string{},
			},
			{
				id:        models.ExecutorIDLocalDocker,
				name:      "Local Docker",
				execType:  models.ExecutorTypeLocalDocker,
				status:    models.ExecutorStatusActive,
				isSystem:  false,
				resumable: true,
				config: map[string]string{
					"docker_host": "unix:///var/run/docker.sock",
				},
			},
			{
				id:        models.ExecutorIDRemoteDocker,
				name:      "Remote Docker",
				execType:  models.ExecutorTypeRemoteDocker,
				status:    models.ExecutorStatusDisabled,
				isSystem:  false,
				resumable: true,
				config:    map[string]string{},
			},
		}

		for _, executor := range executors {
			configJSON, err := json.Marshal(executor.config)
			if err != nil {
				return fmt.Errorf("failed to serialize executor config: %w", err)
			}
			if _, err := r.db.ExecContext(ctx, `
				INSERT INTO executors (id, name, type, status, is_system, resumable, config, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, executor.id, executor.name, executor.execType, executor.status, boolToInt(executor.isSystem), boolToInt(executor.resumable), string(configJSON), now, now); err != nil {
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

func (r *sqliteRepository) ensureWorkspaceIndexes() error {
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)`); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_boards_workspace_id ON boards(workspace_id)`); err != nil {
		return err
	}
	return nil
}

// Close closes the database connection
func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

// DB returns the underlying sql.DB instance for shared access
func (r *sqliteRepository) DB() *sql.DB {
	return r.db
}

// Workspace operations

// CreateWorkspace creates a new workspace
func (r *sqliteRepository) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
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
func (r *sqliteRepository) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
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
func (r *sqliteRepository) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
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
func (r *sqliteRepository) DeleteWorkspace(ctx context.Context, id string) error {
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
func (r *sqliteRepository) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, owner_id, default_executor_id, default_environment_id, default_agent_profile_id, created_at, updated_at
		FROM workspaces ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
func (r *sqliteRepository) CreateTask(ctx context.Context, task *models.Task) error {
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
		INSERT INTO tasks (id, workspace_id, board_id, column_id, title, description, state, priority, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.WorkspaceID, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.CreatedAt, task.UpdatedAt)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("failed to rollback task insert: %w", rollbackErr)
		}
		return err
	}

	return tx.Commit()
}

// GetTask retrieves a task by ID
func (r *sqliteRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task := &models.Task{}
	var metadata string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.WorkspaceID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(metadata), &task.Metadata)
	return task, nil
}

// UpdateTask updates an existing task
func (r *sqliteRepository) UpdateTask(ctx context.Context, task *models.Task) error {
	task.UpdatedAt = time.Now().UTC()

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET workspace_id = ?, board_id = ?, column_id = ?, title = ?, description = ?, state = ?, priority = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, task.WorkspaceID, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.UpdatedAt, task.ID)
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
func (r *sqliteRepository) DeleteTask(ctx context.Context, id string) error {
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
func (r *sqliteRepository) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks
		WHERE board_id = ?
		ORDER BY position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// ListTasksByColumn returns all tasks in a column
func (r *sqliteRepository) ListTasksByColumn(ctx context.Context, columnID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, position, metadata, created_at, updated_at
		FROM tasks
		WHERE column_id = ? ORDER BY position
	`, columnID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

func (r *sqliteRepository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.BoardID,
			&task.ColumnID,
			&task.Title,
			&task.Description,
			&task.State,
			&task.Priority,
			&task.Position,
			&metadata,
			&task.CreatedAt,
			&task.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metadata), &task.Metadata)
		result = append(result, task)
	}
	return result, rows.Err()
}

// UpdateTaskState updates the state of a task
func (r *sqliteRepository) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
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

// TaskRepository operations

func (r *sqliteRepository) CreateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	if taskRepo.ID == "" {
		taskRepo.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	taskRepo.CreatedAt = now
	taskRepo.UpdatedAt = now

	metadataJSON, err := json.Marshal(taskRepo.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO task_repositories (
			id, task_id, repository_id, base_branch, position, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, taskRepo.ID, taskRepo.TaskID, taskRepo.RepositoryID, taskRepo.BaseBranch, taskRepo.Position, string(metadataJSON), taskRepo.CreatedAt, taskRepo.UpdatedAt)
	return err
}

func (r *sqliteRepository) GetTaskRepository(ctx context.Context, id string) (*models.TaskRepository, error) {
	taskRepo := &models.TaskRepository{}
	var metadataJSON string

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, repository_id, base_branch, position, metadata, created_at, updated_at
		FROM task_repositories WHERE id = ?
	`, id).Scan(
		&taskRepo.ID,
		&taskRepo.TaskID,
		&taskRepo.RepositoryID,
		&taskRepo.BaseBranch,
		&taskRepo.Position,
		&metadataJSON,
		&taskRepo.CreatedAt,
		&taskRepo.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task repository not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &taskRepo.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize task repository metadata: %w", err)
		}
	}
	return taskRepo, nil
}

func (r *sqliteRepository) ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, repository_id, base_branch, position, metadata, created_at, updated_at
		FROM task_repositories
		WHERE task_id = ?
		ORDER BY position ASC, created_at ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.TaskRepository
	for rows.Next() {
		taskRepo := &models.TaskRepository{}
		var metadataJSON string
		if err := rows.Scan(
			&taskRepo.ID,
			&taskRepo.TaskID,
			&taskRepo.RepositoryID,
			&taskRepo.BaseBranch,
			&taskRepo.Position,
			&metadataJSON,
			&taskRepo.CreatedAt,
			&taskRepo.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &taskRepo.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize task repository metadata: %w", err)
			}
		}
		result = append(result, taskRepo)
	}
	return result, rows.Err()
}

func (r *sqliteRepository) UpdateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	taskRepo.UpdatedAt = time.Now().UTC()

	metadataJSON, err := json.Marshal(taskRepo.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE task_repositories SET
			task_id = ?, repository_id = ?, base_branch = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, taskRepo.TaskID, taskRepo.RepositoryID, taskRepo.BaseBranch, taskRepo.Position, string(metadataJSON), taskRepo.UpdatedAt, taskRepo.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task repository not found: %s", taskRepo.ID)
	}
	return nil
}

func (r *sqliteRepository) DeleteTaskRepository(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_repositories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task repository not found: %s", id)
	}
	return nil
}

func (r *sqliteRepository) DeleteTaskRepositoriesByTask(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_repositories WHERE task_id = ?`, taskID)
	return err
}

func (r *sqliteRepository) GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error) {
	repos, err := r.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, nil
	}
	return repos[0], nil
}

// AddTaskToBoard adds a task to a board with placement
func (r *sqliteRepository) AddTaskToBoard(ctx context.Context, taskID, boardID, columnID string, position int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET board_id = ?, column_id = ?, position = ?, updated_at = ? WHERE id = ?
	`, boardID, columnID, position, time.Now().UTC(), taskID)
	return err
}

// RemoveTaskFromBoard removes a task from a board
func (r *sqliteRepository) RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tasks SET board_id = '', column_id = '', position = 0, updated_at = ? WHERE id = ? AND board_id = ?
	`, time.Now().UTC(), taskID, boardID)
	return err
}

// Board operations

// CreateBoard creates a new board
func (r *sqliteRepository) CreateBoard(ctx context.Context, board *models.Board) error {
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
func (r *sqliteRepository) GetBoard(ctx context.Context, id string) (*models.Board, error) {
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
func (r *sqliteRepository) UpdateBoard(ctx context.Context, board *models.Board) error {
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
func (r *sqliteRepository) DeleteBoard(ctx context.Context, id string) error {
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
func (r *sqliteRepository) ListBoards(ctx context.Context, workspaceID string) ([]*models.Board, error) {
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
	defer func() { _ = rows.Close() }()

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
func (r *sqliteRepository) CreateColumn(ctx context.Context, column *models.Column) error {
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
func (r *sqliteRepository) GetColumn(ctx context.Context, id string) (*models.Column, error) {
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
func (r *sqliteRepository) GetColumnByState(ctx context.Context, boardID string, state v1.TaskState) (*models.Column, error) {
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
func (r *sqliteRepository) UpdateColumn(ctx context.Context, column *models.Column) error {
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
func (r *sqliteRepository) DeleteColumn(ctx context.Context, id string) error {
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
func (r *sqliteRepository) ListColumns(ctx context.Context, boardID string) ([]*models.Column, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, board_id, name, position, state, color, created_at, updated_at
		FROM columns WHERE board_id = ? ORDER BY position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
func (r *sqliteRepository) CreateMessage(ctx context.Context, message *models.Message) error {
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
		INSERT INTO task_session_messages (id, task_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, message.ID, message.TaskSessionID, message.TaskID, message.AuthorType, message.AuthorID, message.Content, requestsInput, messageType, metadataJSON, message.CreatedAt)

	return err
}

// GetMessage retrieves a message by ID
func (r *sqliteRepository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE id = ?
	`, id).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
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
func (r *sqliteRepository) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var requestsInput int
		var messageType string
		var metadataJSON string
		err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
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
func (r *sqliteRepository) ListMessagesPaginated(ctx context.Context, sessionID string, opts ListMessagesOptions) ([]*models.Message, bool, error) {
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
		if cursor.TaskSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.Before)
		}
	}
	if opts.After != "" {
		var err error
		cursor, err = r.GetMessage(ctx, opts.After)
		if err != nil {
			return nil, false, err
		}
		if cursor.TaskSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.After)
		}
	}

	query := `
		SELECT id, task_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ?`
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
	defer func() { _ = rows.Close() }()

	var result []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var requestsInput int
		var messageType string
		var metadataJSON string
		err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
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
func (r *sqliteRepository) DeleteMessage(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_session_messages WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", id)
	}
	return nil
}

// GetMessageByToolCallID retrieves a tool_call message by session ID and tool_call_id in metadata
func (r *sqliteRepository) GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? AND type = 'tool_call' AND json_extract(metadata, '$.tool_call_id') = ?
	`, sessionID, toolCallID).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID,
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

// GetMessageByPendingID retrieves a message by session ID and pending_id in metadata
func (r *sqliteRepository) GetMessageByPendingID(ctx context.Context, sessionID, pendingID string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? AND json_extract(metadata, '$.pending_id') = ?
	`, sessionID, pendingID).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.AuthorType, &message.AuthorID,
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
func (r *sqliteRepository) UpdateMessage(ctx context.Context, message *models.Message) error {
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE task_session_messages SET content = ?, requests_input = ?, type = ?, metadata = ?
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
func (r *sqliteRepository) CreateRepository(ctx context.Context, repository *models.Repository) error {
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
func (r *sqliteRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
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
func (r *sqliteRepository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
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
func (r *sqliteRepository) DeleteRepository(ctx context.Context, id string) error {
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
func (r *sqliteRepository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
		       provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at, deleted_at
		FROM repositories WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
func (r *sqliteRepository) CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
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
func (r *sqliteRepository) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
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
func (r *sqliteRepository) UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
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
func (r *sqliteRepository) DeleteRepositoryScript(ctx context.Context, id string) error {
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
func (r *sqliteRepository) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, repository_id, name, command, position, created_at, updated_at
		FROM repository_scripts WHERE repository_id = ? ORDER BY position
	`, repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
func (r *sqliteRepository) CreateTaskSession(ctx context.Context, session *models.TaskSession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	session.StartedAt = now
	session.UpdatedAt = now
	if session.State == "" {
		session.State = models.TaskSessionStateCreated
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
		INSERT INTO task_sessions (
			id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
			repository_id, base_branch,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, progress, error_message, metadata, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.TaskID, session.AgentExecutionID, session.ContainerID, session.AgentProfileID,
		session.ExecutorID, session.EnvironmentID, session.RepositoryID, session.BaseBranch,
		string(agentProfileSnapshotJSON), string(executorSnapshotJSON), string(environmentSnapshotJSON), string(repositorySnapshotJSON),
		string(session.State), session.Progress, session.ErrorMessage, string(metadataJSON),
		session.StartedAt, session.CompletedAt, session.UpdatedAt)

	return err
}

// GetAgentSession retrieves an agent session by ID
func (r *sqliteRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE id = ?
	`, id).Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
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

	worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session worktrees: %w", err)
	}
	session.Worktrees = worktrees

	return session, nil
}

// GetAgentSessionByTaskID retrieves the most recent agent session for a task
func (r *sqliteRepository) GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE task_id = ? ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
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

	worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session worktrees: %w", err)
	}
	session.Worktrees = worktrees

	return session, nil
}

// GetActiveAgentSessionByTaskID retrieves the active (running/waiting) agent session for a task
func (r *sqliteRepository) GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	session := &models.TaskSession{}
	var state string
	var metadataJSON string
	var agentProfileSnapshotJSON string
	var executorSnapshotJSON string
	var environmentSnapshotJSON string
	var repositorySnapshotJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions
		WHERE task_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
		&session.ExecutorID, &session.EnvironmentID,
		&session.RepositoryID, &session.BaseBranch,
		&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
		&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active agent session for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.State = models.TaskSessionState(state)
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

	worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session worktrees: %w", err)
	}
	session.Worktrees = worktrees

	return session, nil
}

// UpdateAgentSession updates an existing agent session
func (r *sqliteRepository) UpdateTaskSession(ctx context.Context, session *models.TaskSession) error {
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
		UPDATE task_sessions SET
			agent_execution_id = ?, container_id = ?, agent_profile_id = ?, executor_id = ?, environment_id = ?,
			repository_id = ?, base_branch = ?,
			agent_profile_snapshot = ?, executor_snapshot = ?, environment_snapshot = ?, repository_snapshot = ?,
			state = ?, progress = ?, error_message = ?, metadata = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, session.AgentExecutionID, session.ContainerID, session.AgentProfileID, session.ExecutorID, session.EnvironmentID,
		session.RepositoryID, session.BaseBranch,
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
func (r *sqliteRepository) UpdateTaskSessionState(ctx context.Context, id string, status models.TaskSessionState, errorMessage string) error {
	now := time.Now().UTC()

	var completedAt *time.Time
	if status == models.TaskSessionStateCompleted || status == models.TaskSessionStateFailed || status == models.TaskSessionStateCancelled {
		completedAt = &now
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE task_sessions SET state = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?
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

// ListTaskSessions returns all agent sessions for a task
func (r *sqliteRepository) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE task_id = ? ORDER BY started_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load session worktrees: %w", err)
		}
		session.Worktrees = worktrees
	}
	return sessions, nil
}

// ListActiveTaskSessions returns all active agent sessions across all tasks
func (r *sqliteRepository) ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT') ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load session worktrees: %w", err)
		}
		session.Worktrees = worktrees
	}
	return sessions, nil
}

// ListActiveTaskSessionsByTaskID returns all active agent sessions for a specific task
func (r *sqliteRepository) ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, environment_id,
		       repository_id, base_branch,
		       agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
		       state, progress, error_message, metadata, started_at, completed_at, updated_at
		FROM task_sessions WHERE task_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT') ORDER BY started_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sessions, err := r.scanTaskSessions(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		worktrees, err := r.ListTaskSessionWorktrees(ctx, session.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load session worktrees: %w", err)
		}
		session.Worktrees = worktrees
	}
	return sessions, nil
}

func (r *sqliteRepository) HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM task_sessions
		WHERE agent_profile_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, agentProfileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *sqliteRepository) HasActiveTaskSessionsByExecutor(ctx context.Context, executorID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM task_sessions
		WHERE executor_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, executorID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *sqliteRepository) HasActiveTaskSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM task_sessions
		WHERE environment_id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		LIMIT 1
	`, environmentID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *sqliteRepository) HasActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1
		FROM task_sessions s
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
func (r *sqliteRepository) scanTaskSessions(ctx context.Context, rows *sql.Rows) ([]*models.TaskSession, error) {
	var result []*models.TaskSession
	for rows.Next() {
		session := &models.TaskSession{}
		var state string
		var metadataJSON string
		var agentProfileSnapshotJSON string
		var executorSnapshotJSON string
		var environmentSnapshotJSON string
		var repositorySnapshotJSON string
		var completedAt sql.NullTime

		err := rows.Scan(
			&session.ID, &session.TaskID, &session.AgentExecutionID, &session.ContainerID, &session.AgentProfileID,
			&session.ExecutorID, &session.EnvironmentID,
			&session.RepositoryID, &session.BaseBranch,
			&agentProfileSnapshotJSON, &executorSnapshotJSON, &environmentSnapshotJSON, &repositorySnapshotJSON,
			&state, &session.Progress, &session.ErrorMessage, &metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.State = models.TaskSessionState(state)
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
func (r *sqliteRepository) DeleteTaskSession(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", id)
	}
	return nil
}

// Task Session Worktree operations

func (r *sqliteRepository) CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error {
	if sessionWorktree.ID == "" {
		sessionWorktree.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	sessionWorktree.CreatedAt = now
	updatedAt := now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_session_worktrees (
			id, session_id, worktree_id, repository_id, position,
			worktree_path, worktree_branch, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, worktree_id) DO UPDATE SET
			repository_id = excluded.repository_id,
			position = excluded.position,
			worktree_path = excluded.worktree_path,
			worktree_branch = excluded.worktree_branch,
			updated_at = excluded.updated_at
	`,
		sessionWorktree.ID,
		sessionWorktree.SessionID,
		sessionWorktree.WorktreeID,
		sessionWorktree.RepositoryID,
		sessionWorktree.Position,
		sessionWorktree.WorktreePath,
		sessionWorktree.WorktreeBranch,
		sessionWorktree.CreatedAt,
		updatedAt,
	)
	return err
}

func (r *sqliteRepository) ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			tsw.id, tsw.session_id, tsw.worktree_id, tsw.repository_id, tsw.position,
			tsw.worktree_path, tsw.worktree_branch, tsw.created_at
		FROM task_session_worktrees tsw
		WHERE tsw.session_id = ?
		ORDER BY tsw.position ASC, tsw.created_at ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var worktrees []*models.TaskSessionWorktree
	for rows.Next() {
		var wt models.TaskSessionWorktree
		err := rows.Scan(
			&wt.ID,
			&wt.SessionID,
			&wt.WorktreeID,
			&wt.RepositoryID,
			&wt.Position,
			&wt.WorktreePath,
			&wt.WorktreeBranch,
			&wt.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		worktrees = append(worktrees, &wt)
	}
	return worktrees, rows.Err()
}

func (r *sqliteRepository) DeleteTaskSessionWorktree(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_session_worktrees WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task session worktree not found: %s", id)
	}
	return nil
}

func (r *sqliteRepository) DeleteTaskSessionWorktreesBySession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_session_worktrees WHERE session_id = ?`, sessionID)
	return err
}

// Executor operations

func (r *sqliteRepository) CreateExecutor(ctx context.Context, executor *models.Executor) error {
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
		INSERT INTO executors (id, name, type, status, is_system, resumable, config, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, executor.ID, executor.Name, executor.Type, executor.Status, boolToInt(executor.IsSystem), boolToInt(executor.Resumable), string(configJSON), executor.CreatedAt, executor.UpdatedAt, executor.DeletedAt)
	return err
}

func (r *sqliteRepository) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	executor := &models.Executor{}
	var configJSON string
	var isSystem int
	var resumable int

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, type, status, is_system, resumable, config, created_at, updated_at, deleted_at
		FROM executors WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(
		&executor.ID, &executor.Name, &executor.Type, &executor.Status,
		&isSystem, &resumable, &configJSON, &executor.CreatedAt, &executor.UpdatedAt, &executor.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("executor not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	executor.IsSystem = isSystem == 1
	executor.Resumable = resumable == 1
	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &executor.Config); err != nil {
			return nil, fmt.Errorf("failed to deserialize executor config: %w", err)
		}
	}
	return executor, nil
}

func (r *sqliteRepository) UpdateExecutor(ctx context.Context, executor *models.Executor) error {
	executor.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(executor.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize executor config: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE executors SET name = ?, type = ?, status = ?, is_system = ?, resumable = ?, config = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, executor.Name, executor.Type, executor.Status, boolToInt(executor.IsSystem), boolToInt(executor.Resumable), string(configJSON), executor.UpdatedAt, executor.ID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor not found: %s", executor.ID)
	}
	return nil
}

func (r *sqliteRepository) DeleteExecutor(ctx context.Context, id string) error {
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

func (r *sqliteRepository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, type, status, is_system, resumable, config, created_at, updated_at, deleted_at
		FROM executors WHERE deleted_at IS NULL ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Executor
	for rows.Next() {
		executor := &models.Executor{}
		var configJSON string
		var isSystem int
		var resumable int
		if err := rows.Scan(
			&executor.ID, &executor.Name, &executor.Type, &executor.Status,
			&isSystem, &resumable, &configJSON, &executor.CreatedAt, &executor.UpdatedAt, &executor.DeletedAt,
		); err != nil {
			return nil, err
		}
		executor.IsSystem = isSystem == 1
		executor.Resumable = resumable == 1
		if configJSON != "" && configJSON != "{}" {
			if err := json.Unmarshal([]byte(configJSON), &executor.Config); err != nil {
				return nil, fmt.Errorf("failed to deserialize executor config: %w", err)
			}
		}
		result = append(result, executor)
	}
	return result, rows.Err()
}

func (r *sqliteRepository) UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error {
	if running == nil {
		return fmt.Errorf("executor running is nil")
	}
	if running.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if running.ID == "" {
		running.ID = running.SessionID
	}
	now := time.Now().UTC()
	if running.CreatedAt.IsZero() {
		running.CreatedAt = now
	}
	running.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO executors_running (
			id, session_id, task_id, executor_id, runtime, status, resumable, resume_token,
			agent_execution_id, container_id, agentctl_url, agentctl_port, pid,
			worktree_id, worktree_path, worktree_branch, last_seen_at, error_message,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			id = excluded.id,
			task_id = excluded.task_id,
			executor_id = excluded.executor_id,
			runtime = excluded.runtime,
			status = excluded.status,
			resumable = excluded.resumable,
			resume_token = excluded.resume_token,
			agent_execution_id = excluded.agent_execution_id,
			container_id = excluded.container_id,
			agentctl_url = excluded.agentctl_url,
			agentctl_port = excluded.agentctl_port,
			pid = excluded.pid,
			worktree_id = excluded.worktree_id,
			worktree_path = excluded.worktree_path,
			worktree_branch = excluded.worktree_branch,
			last_seen_at = excluded.last_seen_at,
			error_message = excluded.error_message,
			updated_at = excluded.updated_at
	`,
		running.ID,
		running.SessionID,
		running.TaskID,
		running.ExecutorID,
		running.Runtime,
		running.Status,
		boolToInt(running.Resumable),
		running.ResumeToken,
		running.AgentExecutionID,
		running.ContainerID,
		running.AgentctlURL,
		running.AgentctlPort,
		running.PID,
		running.WorktreeID,
		running.WorktreePath,
		running.WorktreeBranch,
		running.LastSeenAt,
		running.ErrorMessage,
		running.CreatedAt,
		running.UpdatedAt,
	)
	return err
}

func (r *sqliteRepository) ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, task_id, executor_id, runtime, status, resumable, resume_token,
			agent_execution_id, container_id, agentctl_url, agentctl_port,
			worktree_id, worktree_path, worktree_branch, created_at, updated_at
		FROM executors_running
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*models.ExecutorRunning
	for rows.Next() {
		running := &models.ExecutorRunning{}
		if scanErr := rows.Scan(
			&running.ID,
			&running.SessionID,
			&running.TaskID,
			&running.ExecutorID,
			&running.Runtime,
			&running.Status,
			&running.Resumable,
			&running.ResumeToken,
			&running.AgentExecutionID,
			&running.ContainerID,
			&running.AgentctlURL,
			&running.AgentctlPort,
			&running.WorktreeID,
			&running.WorktreePath,
			&running.WorktreeBranch,
			&running.CreatedAt,
			&running.UpdatedAt,
		); scanErr != nil {
			return nil, scanErr
		}
		results = append(results, running)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *sqliteRepository) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	running := &models.ExecutorRunning{}
	var resumable int
	var lastSeen sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, session_id, task_id, executor_id, runtime, status, resumable, resume_token,
		       agent_execution_id, container_id, agentctl_url, agentctl_port, pid,
		       worktree_id, worktree_path, worktree_branch, last_seen_at, error_message,
		       created_at, updated_at
		FROM executors_running
		WHERE session_id = ?
	`, sessionID).Scan(
		&running.ID,
		&running.SessionID,
		&running.TaskID,
		&running.ExecutorID,
		&running.Runtime,
		&running.Status,
		&resumable,
		&running.ResumeToken,
		&running.AgentExecutionID,
		&running.ContainerID,
		&running.AgentctlURL,
		&running.AgentctlPort,
		&running.PID,
		&running.WorktreeID,
		&running.WorktreePath,
		&running.WorktreeBranch,
		&lastSeen,
		&running.ErrorMessage,
		&running.CreatedAt,
		&running.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("executor running not found for session: %s", sessionID)
	}
	if err != nil {
		return nil, err
	}
	running.Resumable = resumable == 1
	if lastSeen.Valid {
		running.LastSeenAt = &lastSeen.Time
	}
	return running, nil
}

func (r *sqliteRepository) DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM executors_running WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("executor running not found for session: %s", sessionID)
	}
	return nil
}

// Environment operations

func (r *sqliteRepository) CreateEnvironment(ctx context.Context, environment *models.Environment) error {
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

func (r *sqliteRepository) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
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

func (r *sqliteRepository) UpdateEnvironment(ctx context.Context, environment *models.Environment) error {
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

func (r *sqliteRepository) DeleteEnvironment(ctx context.Context, id string) error {
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

func (r *sqliteRepository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, kind, is_system, worktree_root, image_tag, dockerfile, build_config, created_at, updated_at, deleted_at
		FROM environments WHERE deleted_at IS NULL ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
