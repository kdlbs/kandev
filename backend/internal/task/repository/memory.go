package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MemoryRepository provides in-memory task storage operations
type MemoryRepository struct {
	tasks   map[string]*models.Task
	boards  map[string]*models.Board
	columns map[string]*models.Column
	mu      sync.RWMutex
}

// Ensure MemoryRepository implements Repository interface
var _ Repository = (*MemoryRepository)(nil)

// NewMemoryRepository creates a new in-memory task repository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		tasks:   make(map[string]*models.Task),
		boards:  make(map[string]*models.Board),
		columns: make(map[string]*models.Column),
	}
}

// Close is a no-op for in-memory repository
func (r *MemoryRepository) Close() error {
	return nil
}

// Task operations

// CreateTask creates a new task
func (r *MemoryRepository) CreateTask(ctx context.Context, task *models.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	r.tasks[task.ID] = task
	return nil
}

// GetTask retrieves a task by ID
func (r *MemoryRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return task, nil
}

// UpdateTask updates an existing task
func (r *MemoryRepository) UpdateTask(ctx context.Context, task *models.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[task.ID]; !ok {
		return fmt.Errorf("task not found: %s", task.ID)
	}
	task.UpdatedAt = time.Now().UTC()
	r.tasks[task.ID] = task
	return nil
}

// DeleteTask deletes a task by ID
func (r *MemoryRepository) DeleteTask(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[id]; !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	delete(r.tasks, id)
	return nil
}

// ListTasks returns all tasks for a board
func (r *MemoryRepository) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Task
	for _, task := range r.tasks {
		if task.BoardID == boardID {
			result = append(result, task)
		}
	}
	return result, nil
}

// ListTasksByColumn returns all tasks in a column
func (r *MemoryRepository) ListTasksByColumn(ctx context.Context, columnID string) ([]*models.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Task
	for _, task := range r.tasks {
		if task.ColumnID == columnID {
			result = append(result, task)
		}
	}
	return result, nil
}

// UpdateTaskState updates the state of a task
func (r *MemoryRepository) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	task.State = state
	task.UpdatedAt = time.Now().UTC()
	return nil
}

// Board operations

// CreateBoard creates a new board
func (r *MemoryRepository) CreateBoard(ctx context.Context, board *models.Board) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if board.ID == "" {
		board.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	board.CreatedAt = now
	board.UpdatedAt = now

	r.boards[board.ID] = board
	return nil
}

// GetBoard retrieves a board by ID
func (r *MemoryRepository) GetBoard(ctx context.Context, id string) (*models.Board, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	board, ok := r.boards[id]
	if !ok {
		return nil, fmt.Errorf("board not found: %s", id)
	}
	return board, nil
}

// UpdateBoard updates an existing board
func (r *MemoryRepository) UpdateBoard(ctx context.Context, board *models.Board) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.boards[board.ID]; !ok {
		return fmt.Errorf("board not found: %s", board.ID)
	}
	board.UpdatedAt = time.Now().UTC()
	r.boards[board.ID] = board
	return nil
}

// DeleteBoard deletes a board by ID
func (r *MemoryRepository) DeleteBoard(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.boards[id]; !ok {
		return fmt.Errorf("board not found: %s", id)
	}
	delete(r.boards, id)
	return nil
}

// ListBoards returns all boards
func (r *MemoryRepository) ListBoards(ctx context.Context) ([]*models.Board, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Board, 0, len(r.boards))
	for _, board := range r.boards {
		result = append(result, board)
	}
	return result, nil
}

// Column operations

// CreateColumn creates a new column
func (r *MemoryRepository) CreateColumn(ctx context.Context, column *models.Column) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if column.ID == "" {
		column.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	column.CreatedAt = now
	column.UpdatedAt = now

	r.columns[column.ID] = column
	return nil
}

// GetColumn retrieves a column by ID
func (r *MemoryRepository) GetColumn(ctx context.Context, id string) (*models.Column, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	column, ok := r.columns[id]
	if !ok {
		return nil, fmt.Errorf("column not found: %s", id)
	}
	return column, nil
}

// UpdateColumn updates an existing column
func (r *MemoryRepository) UpdateColumn(ctx context.Context, column *models.Column) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.columns[column.ID]; !ok {
		return fmt.Errorf("column not found: %s", column.ID)
	}
	column.UpdatedAt = time.Now().UTC()
	r.columns[column.ID] = column
	return nil
}

// DeleteColumn deletes a column by ID
func (r *MemoryRepository) DeleteColumn(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.columns[id]; !ok {
		return fmt.Errorf("column not found: %s", id)
	}
	delete(r.columns, id)
	return nil
}

// ListColumns returns all columns for a board
func (r *MemoryRepository) ListColumns(ctx context.Context, boardID string) ([]*models.Column, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Column
	for _, column := range r.columns {
		if column.BoardID == boardID {
			result = append(result, column)
		}
	}
	return result, nil
}

