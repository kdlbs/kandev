package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
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
		agent_type TEXT DEFAULT '',
		repository_url TEXT DEFAULT '',
		branch TEXT DEFAULT '',
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

	CREATE TABLE IF NOT EXISTS task_boards (
		task_id TEXT NOT NULL,
		board_id TEXT NOT NULL,
		PRIMARY KEY (task_id, board_id),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS task_columns (
		task_id TEXT NOT NULL,
		board_id TEXT NOT NULL,
		column_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		PRIMARY KEY (task_id, board_id),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
		FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_board_id ON tasks(board_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_column_id ON tasks(column_id);
	CREATE INDEX IF NOT EXISTS idx_columns_board_id ON columns(board_id);
	CREATE INDEX IF NOT EXISTS idx_task_boards_board_id ON task_boards(board_id);
	CREATE INDEX IF NOT EXISTS idx_task_columns_board_id ON task_columns(board_id);
	CREATE INDEX IF NOT EXISTS idx_task_columns_column_id ON task_columns(column_id);
	CREATE INDEX IF NOT EXISTS idx_repositories_workspace_id ON repositories(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_repository_scripts_repo_id ON repository_scripts(repository_id);

	CREATE TABLE IF NOT EXISTS task_comments (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		author_type TEXT NOT NULL DEFAULT 'user',
		author_id TEXT DEFAULT '',
		content TEXT NOT NULL,
		requests_input INTEGER DEFAULT 0,
		acp_session_id TEXT DEFAULT '',
		type TEXT NOT NULL DEFAULT 'message',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_comments_task_id ON task_comments(task_id);
	CREATE INDEX IF NOT EXISTS idx_comments_created_at ON task_comments(created_at);
	CREATE INDEX IF NOT EXISTS idx_comments_task_created ON task_comments(task_id, created_at);

	CREATE TABLE IF NOT EXISTS agent_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_instance_id TEXT NOT NULL DEFAULT '',
		agent_type TEXT NOT NULL,
		acp_session_id TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		progress INTEGER DEFAULT 0,
		error_message TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_agent_sessions_task_id ON agent_sessions(task_id);
	CREATE INDEX IF NOT EXISTS idx_agent_sessions_status ON agent_sessions(status);
	CREATE INDEX IF NOT EXISTS idx_agent_sessions_task_status ON agent_sessions(task_id, status);
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
	if err := r.ensureColumn("columns", "color", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Ensure new comment columns exist for existing databases
	if err := r.ensureColumn("task_comments", "type", "TEXT NOT NULL DEFAULT 'message'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_comments", "metadata", "TEXT DEFAULT '{}'"); err != nil {
		return err
	}
	if err := r.ensureColumn("task_comments", "agent_session_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}

	if err := r.ensureDefaultWorkspace(); err != nil {
		return err
	}

	if err := r.backfillTaskMappings(); err != nil {
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
			INSERT INTO workspaces (id, name, description, owner_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, defaultID, workspaceName, workspaceDescription, "", now, now); err != nil {
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
		INSERT INTO workspaces (id, name, description, owner_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, workspace.ID, workspace.Name, workspace.Description, workspace.OwnerID, workspace.CreatedAt, workspace.UpdatedAt)

	return err
}

// GetWorkspace retrieves a workspace by ID
func (r *SQLiteRepository) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	workspace := &models.Workspace{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, owner_id, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Description, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return workspace, err
}

// UpdateWorkspace updates an existing workspace
func (r *SQLiteRepository) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	workspace.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE workspaces SET name = ?, description = ?, updated_at = ? WHERE id = ?
	`, workspace.Name, workspace.Description, workspace.UpdatedAt, workspace.ID)
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
		SELECT id, name, description, owner_id, created_at, updated_at FROM workspaces ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Workspace
	for rows.Next() {
		workspace := &models.Workspace{}
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Description, &workspace.OwnerID, &workspace.CreatedAt, &workspace.UpdatedAt); err != nil {
			return nil, err
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
		INSERT INTO tasks (id, workspace_id, board_id, column_id, title, description, state, priority, agent_type, repository_url, branch, assigned_to, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.WorkspaceID, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.AgentType, task.RepositoryURL, task.Branch, task.AssignedTo, task.Position, string(metadata), task.CreatedAt, task.UpdatedAt)
	if err != nil {
		tx.Rollback()
		return err
	}

	if task.BoardID != "" {
		if err := r.addTaskToBoardTx(ctx, tx, task.ID, task.BoardID, task.ColumnID, task.Position); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// GetTask retrieves a task by ID
func (r *SQLiteRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task := &models.Task{}
	var metadata string
	var agentType, repositoryURL, branch, assignedTo sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, agent_type, repository_url, branch, assigned_to, position, metadata, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.WorkspaceID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &agentType, &repositoryURL, &branch, &assignedTo, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	task.AgentType = agentType.String
	task.RepositoryURL = repositoryURL.String
	task.Branch = branch.String
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
		UPDATE tasks SET workspace_id = ?, board_id = ?, column_id = ?, title = ?, description = ?, state = ?, priority = ?, agent_type = ?, repository_url = ?, branch = ?, assigned_to = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, task.WorkspaceID, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.AgentType, task.RepositoryURL, task.Branch, task.AssignedTo, task.Position, string(metadata), task.UpdatedAt, task.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", task.ID)
	}
	if task.BoardID != "" {
		if err := r.AddTaskToBoard(ctx, task.ID, task.BoardID, task.ColumnID, task.Position); err != nil {
			return err
		}
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
		SELECT t.id, t.workspace_id, t.title, t.description, t.state, t.priority, t.agent_type, t.repository_url, t.branch, t.assigned_to, t.metadata, t.created_at, t.updated_at,
		       tc.board_id, tc.column_id, tc.position
		FROM tasks t
		INNER JOIN task_columns tc ON tc.task_id = t.id
		WHERE tc.board_id = ?
		ORDER BY tc.position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTasksWithPlacement(rows)
}

// ListTasksByColumn returns all tasks in a column
func (r *SQLiteRepository) ListTasksByColumn(ctx context.Context, columnID string) ([]*models.Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.workspace_id, t.title, t.description, t.state, t.priority, t.agent_type, t.repository_url, t.branch, t.assigned_to, t.metadata, t.created_at, t.updated_at,
		       tc.board_id, tc.column_id, tc.position
		FROM tasks t
		INNER JOIN task_columns tc ON tc.task_id = t.id
		WHERE tc.column_id = ? ORDER BY tc.position
	`, columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTasksWithPlacement(rows)
}

// scanTasks scans multiple task rows
func (r *SQLiteRepository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		var agentType, repositoryURL, branch, assignedTo sql.NullString
		err := rows.Scan(&task.ID, &task.WorkspaceID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &agentType, &repositoryURL, &branch, &assignedTo, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)
		if err != nil {
			return nil, err
		}
		task.AgentType = agentType.String
		task.RepositoryURL = repositoryURL.String
		task.Branch = branch.String
		task.AssignedTo = assignedTo.String
		_ = json.Unmarshal([]byte(metadata), &task.Metadata)
		result = append(result, task)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) scanTasksWithPlacement(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		var agentType, repositoryURL, branch, assignedTo sql.NullString
		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.Title,
			&task.Description,
			&task.State,
			&task.Priority,
			&agentType,
			&repositoryURL,
			&branch,
			&assignedTo,
			&metadata,
			&task.CreatedAt,
			&task.UpdatedAt,
			&task.BoardID,
			&task.ColumnID,
			&task.Position,
		)
		if err != nil {
			return nil, err
		}
		task.AgentType = agentType.String
		task.RepositoryURL = repositoryURL.String
		task.Branch = branch.String
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
		INSERT OR IGNORE INTO task_boards (task_id, board_id) VALUES (?, ?)
	`, taskID, boardID)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO task_columns (task_id, board_id, column_id, position)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(task_id, board_id) DO UPDATE SET column_id = excluded.column_id, position = excluded.position
	`, taskID, boardID, columnID, position)
	return err
}

// RemoveTaskFromBoard removes a task from a board
func (r *SQLiteRepository) RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM task_columns WHERE task_id = ? AND board_id = ?`, taskID, boardID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_boards WHERE task_id = ? AND board_id = ?`, taskID, boardID)
	return err
}

func (r *SQLiteRepository) addTaskToBoardTx(ctx context.Context, tx *sql.Tx, taskID, boardID, columnID string, position int) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO task_boards (task_id, board_id) VALUES (?, ?)
	`, taskID, boardID); err != nil {
		return err
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_columns (task_id, board_id, column_id, position)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(task_id, board_id) DO UPDATE SET column_id = excluded.column_id, position = excluded.position
	`, taskID, boardID, columnID, position)
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

// Comment operations

// CreateComment creates a new comment
func (r *SQLiteRepository) CreateComment(ctx context.Context, comment *models.Comment) error {
	if comment.ID == "" {
		comment.ID = uuid.New().String()
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now().UTC()
	}
	if comment.AuthorType == "" {
		comment.AuthorType = models.CommentAuthorUser
	}

	requestsInput := 0
	if comment.RequestsInput {
		requestsInput = 1
	}

	// Default type to "message" if empty
	commentType := string(comment.Type)
	if commentType == "" {
		commentType = string(models.CommentTypeMessage)
	}

	// Serialize metadata to JSON
	metadataJSON := "{}"
	if comment.Metadata != nil {
		metadataBytes, err := json.Marshal(comment.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize comment metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_comments (id, task_id, author_type, author_id, content, requests_input, acp_session_id, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, comment.ID, comment.TaskID, comment.AuthorType, comment.AuthorID, comment.Content, requestsInput, comment.ACPSessionID, commentType, metadataJSON, comment.CreatedAt)

	return err
}

// GetComment retrieves a comment by ID
func (r *SQLiteRepository) GetComment(ctx context.Context, id string) (*models.Comment, error) {
	comment := &models.Comment{}
	var requestsInput int
	var commentType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, author_type, author_id, content, requests_input, acp_session_id, type, metadata, created_at
		FROM task_comments WHERE id = ?
	`, id).Scan(&comment.ID, &comment.TaskID, &comment.AuthorType, &comment.AuthorID, &comment.Content, &requestsInput, &comment.ACPSessionID, &commentType, &metadataJSON, &comment.CreatedAt)
	if err != nil {
		return nil, err
	}
	comment.RequestsInput = requestsInput == 1
	comment.Type = models.CommentType(commentType)

	// Deserialize metadata from JSON
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &comment.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize comment metadata: %w", err)
		}
	}

	return comment, nil
}

// ListComments returns all comments for a task ordered by creation time
func (r *SQLiteRepository) ListComments(ctx context.Context, taskID string) ([]*models.Comment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, author_type, author_id, content, requests_input, acp_session_id, type, metadata, created_at
		FROM task_comments WHERE task_id = ? ORDER BY created_at ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Comment
	for rows.Next() {
		comment := &models.Comment{}
		var requestsInput int
		var commentType string
		var metadataJSON string
		err := rows.Scan(&comment.ID, &comment.TaskID, &comment.AuthorType, &comment.AuthorID, &comment.Content, &requestsInput, &comment.ACPSessionID, &commentType, &metadataJSON, &comment.CreatedAt)
		if err != nil {
			return nil, err
		}
		comment.RequestsInput = requestsInput == 1
		comment.Type = models.CommentType(commentType)

		// Deserialize metadata from JSON
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &comment.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize comment metadata: %w", err)
			}
		}

		result = append(result, comment)
	}
	return result, rows.Err()
}

// DeleteComment deletes a comment by ID
func (r *SQLiteRepository) DeleteComment(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_comments WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("comment not found: %s", id)
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
			provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, repository.ID, repository.WorkspaceID, repository.Name, repository.SourceType, repository.LocalPath, repository.Provider,
		repository.ProviderRepoID, repository.ProviderOwner, repository.ProviderName, repository.DefaultBranch,
		repository.SetupScript, repository.CleanupScript, repository.CreatedAt, repository.UpdatedAt)

	return err
}

// GetRepository retrieves a repository by ID
func (r *SQLiteRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	repository := &models.Repository{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, source_type, local_path, provider, provider_repo_id, provider_owner,
		       provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at
		FROM repositories WHERE id = ?
	`, id).Scan(
		&repository.ID, &repository.WorkspaceID, &repository.Name, &repository.SourceType, &repository.LocalPath,
		&repository.Provider, &repository.ProviderRepoID, &repository.ProviderOwner, &repository.ProviderName,
		&repository.DefaultBranch, &repository.SetupScript, &repository.CleanupScript, &repository.CreatedAt, &repository.UpdatedAt,
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
		WHERE id = ?
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
	result, err := r.db.ExecContext(ctx, `DELETE FROM repositories WHERE id = ?`, id)
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
		       provider_name, default_branch, setup_script, cleanup_script, created_at, updated_at
		FROM repositories WHERE workspace_id = ? ORDER BY created_at DESC
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
			&repository.DefaultBranch, &repository.SetupScript, &repository.CleanupScript, &repository.CreatedAt, &repository.UpdatedAt,
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

	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize agent session metadata: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO agent_sessions (
			id, task_id, agent_instance_id, agent_type, acp_session_id, status, progress,
			error_message, metadata, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.TaskID, session.AgentInstanceID, session.AgentType, session.ACPSessionID,
		string(session.Status), session.Progress, session.ErrorMessage, string(metadataJSON),
		session.StartedAt, session.CompletedAt, session.UpdatedAt)

	return err
}

// GetAgentSession retrieves an agent session by ID
func (r *SQLiteRepository) GetAgentSession(ctx context.Context, id string) (*models.AgentSession, error) {
	session := &models.AgentSession{}
	var status string
	var metadataJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_instance_id, agent_type, acp_session_id, status, progress,
		       error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions WHERE id = ?
	`, id).Scan(
		&session.ID, &session.TaskID, &session.AgentInstanceID, &session.AgentType,
		&session.ACPSessionID, &status, &session.Progress, &session.ErrorMessage,
		&metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	session.Status = models.AgentSessionStatus(status)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}

	return session, nil
}

// GetAgentSessionByTaskID retrieves the most recent agent session for a task
func (r *SQLiteRepository) GetAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error) {
	session := &models.AgentSession{}
	var status string
	var metadataJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_instance_id, agent_type, acp_session_id, status, progress,
		       error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions WHERE task_id = ? ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentInstanceID, &session.AgentType,
		&session.ACPSessionID, &status, &session.Progress, &session.ErrorMessage,
		&metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.Status = models.AgentSessionStatus(status)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
		}
	}

	return session, nil
}

// GetActiveAgentSessionByTaskID retrieves the active (running/waiting) agent session for a task
func (r *SQLiteRepository) GetActiveAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error) {
	session := &models.AgentSession{}
	var status string
	var metadataJSON string
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_id, agent_instance_id, agent_type, acp_session_id, status, progress,
		       error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions
		WHERE task_id = ? AND status IN ('pending', 'running', 'waiting')
		ORDER BY started_at DESC LIMIT 1
	`, taskID).Scan(
		&session.ID, &session.TaskID, &session.AgentInstanceID, &session.AgentType,
		&session.ACPSessionID, &status, &session.Progress, &session.ErrorMessage,
		&metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active agent session for task: %s", taskID)
	}
	if err != nil {
		return nil, err
	}

	session.Status = models.AgentSessionStatus(status)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
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

	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_sessions SET
			agent_instance_id = ?, agent_type = ?, acp_session_id = ?, status = ?, progress = ?,
			error_message = ?, metadata = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, session.AgentInstanceID, session.AgentType, session.ACPSessionID, string(session.Status),
		session.Progress, session.ErrorMessage, string(metadataJSON), session.CompletedAt,
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

// UpdateAgentSessionStatus updates just the status and error message of an agent session
func (r *SQLiteRepository) UpdateAgentSessionStatus(ctx context.Context, id string, status models.AgentSessionStatus, errorMessage string) error {
	now := time.Now().UTC()

	var completedAt *time.Time
	if status == models.AgentSessionStatusCompleted || status == models.AgentSessionStatusFailed || status == models.AgentSessionStatusStopped {
		completedAt = &now
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE agent_sessions SET status = ?, error_message = ?, completed_at = ?, updated_at = ? WHERE id = ?
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
		SELECT id, task_id, agent_instance_id, agent_type, acp_session_id, status, progress,
		       error_message, metadata, started_at, completed_at, updated_at
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
		SELECT id, task_id, agent_instance_id, agent_type, acp_session_id, status, progress,
		       error_message, metadata, started_at, completed_at, updated_at
		FROM agent_sessions WHERE status IN ('pending', 'running', 'waiting') ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanAgentSessions(rows)
}

// scanAgentSessions is a helper to scan multiple agent session rows
func (r *SQLiteRepository) scanAgentSessions(rows *sql.Rows) ([]*models.AgentSession, error) {
	var result []*models.AgentSession
	for rows.Next() {
		session := &models.AgentSession{}
		var status string
		var metadataJSON string
		var completedAt sql.NullTime

		err := rows.Scan(
			&session.ID, &session.TaskID, &session.AgentInstanceID, &session.AgentType,
			&session.ACPSessionID, &status, &session.Progress, &session.ErrorMessage,
			&metadataJSON, &session.StartedAt, &completedAt, &session.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		session.Status = models.AgentSessionStatus(status)
		if completedAt.Valid {
			session.CompletedAt = &completedAt.Time
		}
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize agent session metadata: %w", err)
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