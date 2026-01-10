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
	CREATE TABLE IF NOT EXISTS boards (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		owner_id TEXT DEFAULT '',
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
		board_id TEXT NOT NULL,
		column_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		state TEXT DEFAULT 'TODO',
		priority INTEGER DEFAULT 0,
		agent_type TEXT DEFAULT '',
		assigned_to TEXT DEFAULT '',
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
		FOREIGN KEY (column_id) REFERENCES columns(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_board_id ON tasks(board_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_column_id ON tasks(column_id);
	CREATE INDEX IF NOT EXISTS idx_columns_board_id ON columns(board_id);
	`

	_, err := r.db.Exec(schema)
	return err
}

// Close closes the database connection
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
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

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO tasks (id, board_id, column_id, title, description, state, priority, agent_type, assigned_to, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.AgentType, task.AssignedTo, task.Position, string(metadata), task.CreatedAt, task.UpdatedAt)

	return err
}

// GetTask retrieves a task by ID
func (r *SQLiteRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task := &models.Task{}
	var metadata string

	err := r.db.QueryRowContext(ctx, `
		SELECT id, board_id, column_id, title, description, state, priority, agent_type, assigned_to, position, metadata, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(&task.ID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &task.AgentType, &task.AssignedTo, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)

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
		UPDATE tasks SET board_id = ?, column_id = ?, title = ?, description = ?, state = ?, priority = ?, agent_type = ?, assigned_to = ?, position = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, task.BoardID, task.ColumnID, task.Title, task.Description, task.State, task.Priority, task.AgentType, task.AssignedTo, task.Position, string(metadata), task.UpdatedAt, task.ID)
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
		SELECT id, board_id, column_id, title, description, state, priority, agent_type, assigned_to, position, metadata, created_at, updated_at
		FROM tasks WHERE board_id = ? ORDER BY position
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
		SELECT id, board_id, column_id, title, description, state, priority, agent_type, assigned_to, position, metadata, created_at, updated_at
		FROM tasks WHERE column_id = ? ORDER BY position
	`, columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTasks(rows)
}

// scanTasks scans multiple task rows
func (r *SQLiteRepository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		err := rows.Scan(&task.ID, &task.BoardID, &task.ColumnID, &task.Title, &task.Description, &task.State, &task.Priority, &task.AgentType, &task.AssignedTo, &task.Position, &metadata, &task.CreatedAt, &task.UpdatedAt)
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
		INSERT INTO boards (id, name, description, owner_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, board.ID, board.Name, board.Description, board.OwnerID, board.CreatedAt, board.UpdatedAt)

	return err
}

// GetBoard retrieves a board by ID
func (r *SQLiteRepository) GetBoard(ctx context.Context, id string) (*models.Board, error) {
	board := &models.Board{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, owner_id, created_at, updated_at
		FROM boards WHERE id = ?
	`, id).Scan(&board.ID, &board.Name, &board.Description, &board.OwnerID, &board.CreatedAt, &board.UpdatedAt)

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
func (r *SQLiteRepository) ListBoards(ctx context.Context) ([]*models.Board, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, owner_id, created_at, updated_at FROM boards ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Board
	for rows.Next() {
		board := &models.Board{}
		err := rows.Scan(&board.ID, &board.Name, &board.Description, &board.OwnerID, &board.CreatedAt, &board.UpdatedAt)
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
