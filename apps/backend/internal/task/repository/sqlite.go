package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
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

	CREATE TABLE IF NOT EXISTS task_agent_execution_logs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_instance_id TEXT NOT NULL DEFAULT '',
		log_level TEXT NOT NULL DEFAULT 'info',
		message_type TEXT NOT NULL,
		message TEXT NOT NULL DEFAULT '',
		metadata TEXT DEFAULT '{}',
		timestamp DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_execution_logs_task_id ON task_agent_execution_logs(task_id);
	CREATE INDEX IF NOT EXISTS idx_execution_logs_timestamp ON task_agent_execution_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_execution_logs_task_timestamp ON task_agent_execution_logs(task_id, timestamp);
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
		defaultID := uuid.New().String()
		now := time.Now().UTC()
		if _, err := r.db.ExecContext(ctx, `
			INSERT INTO workspaces (id, name, description, owner_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, defaultID, "Default Workspace", "", "", now, now); err != nil {
			return err
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

	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, board_id, column_id, title, description, state, priority, agent_type, repository_url, branch, assigned_to, position, metadata, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.WorkspaceID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &task.AgentType, &task.RepositoryURL, &task.Branch, &task.AssignedTo, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)

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
		err := rows.Scan(&task.ID, &task.WorkspaceID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &task.AgentType, &task.RepositoryURL, &task.Branch, &task.AssignedTo, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)
		if err != nil {
			return nil, err
		}
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
		err := rows.Scan(
			&task.ID,
			&task.WorkspaceID,
			&task.Title,
			&task.Description,
			&task.State,
			&task.Priority,
			&task.AgentType,
			&task.RepositoryURL,
			&task.Branch,
			&task.AssignedTo,
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
		INSERT INTO columns (id, board_id, name, position, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, column.ID, column.BoardID, column.Name, column.Position, column.State, column.CreatedAt, column.UpdatedAt)

	return err
}

// GetColumn retrieves a column by ID
func (r *SQLiteRepository) GetColumn(ctx context.Context, id string) (*models.Column, error) {
	column := &models.Column{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, board_id, name, position, state, created_at, updated_at
		FROM columns WHERE id = ?
	`, id).Scan(&column.ID, &column.BoardID, &column.Name, &column.Position, &column.State, &column.CreatedAt, &column.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("column not found: %s", id)
	}
	return column, err
}

// UpdateColumn updates an existing column
func (r *SQLiteRepository) UpdateColumn(ctx context.Context, column *models.Column) error {
	column.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, `
		UPDATE columns SET name = ?, position = ?, state = ?, updated_at = ? WHERE id = ?
	`, column.Name, column.Position, column.State, column.UpdatedAt, column.ID)
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
		SELECT id, board_id, name, position, state, created_at, updated_at
		FROM columns WHERE board_id = ? ORDER BY position
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Column
	for rows.Next() {
		column := &models.Column{}
		err := rows.Scan(&column.ID, &column.BoardID, &column.Name, &column.Position, &column.State, &column.CreatedAt, &column.UpdatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, column)
	}
	return result, rows.Err()
}
