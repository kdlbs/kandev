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
	workspaces     map[string]*models.Workspace
	tasks          map[string]*models.Task
	boards         map[string]*models.Board
	columns        map[string]*models.Column
	comments       map[string]*models.Comment
	taskBoards     map[string]map[string]struct{}
	taskPlacements map[string]map[string]*taskPlacement
	mu             sync.RWMutex
}

// Ensure MemoryRepository implements Repository interface
var _ Repository = (*MemoryRepository)(nil)

// NewMemoryRepository creates a new in-memory task repository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		workspaces:     make(map[string]*models.Workspace),
		tasks:          make(map[string]*models.Task),
		boards:         make(map[string]*models.Board),
		columns:        make(map[string]*models.Column),
		comments:       make(map[string]*models.Comment),
		taskBoards:     make(map[string]map[string]struct{}),
		taskPlacements: make(map[string]map[string]*taskPlacement),
	}
}

type taskPlacement struct {
	boardID  string
	columnID string
	position int
}

// Close is a no-op for in-memory repository
func (r *MemoryRepository) Close() error {
	return nil
}

// Workspace operations

// CreateWorkspace creates a new workspace
func (r *MemoryRepository) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if workspace.ID == "" {
		workspace.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now

	r.workspaces[workspace.ID] = workspace
	return nil
}

// GetWorkspace retrieves a workspace by ID
func (r *MemoryRepository) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workspace, ok := r.workspaces[id]
	if !ok {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return workspace, nil
}

// UpdateWorkspace updates an existing workspace
func (r *MemoryRepository) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.workspaces[workspace.ID]; !ok {
		return fmt.Errorf("workspace not found: %s", workspace.ID)
	}
	workspace.UpdatedAt = time.Now().UTC()
	r.workspaces[workspace.ID] = workspace
	return nil
}

// DeleteWorkspace deletes a workspace by ID
func (r *MemoryRepository) DeleteWorkspace(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.workspaces[id]; !ok {
		return fmt.Errorf("workspace not found: %s", id)
	}
	delete(r.workspaces, id)
	return nil
}

// ListWorkspaces returns all workspaces
func (r *MemoryRepository) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Workspace, 0, len(r.workspaces))
	for _, workspace := range r.workspaces {
		result = append(result, workspace)
	}
	return result, nil
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
	if task.BoardID != "" {
		r.addTaskPlacement(task.ID, task.BoardID, task.ColumnID, task.Position)
	}
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
	if task.BoardID != "" {
		r.addTaskPlacement(task.ID, task.BoardID, task.ColumnID, task.Position)
	}
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
	delete(r.taskBoards, id)
	delete(r.taskPlacements, id)
	return nil
}

// ListTasks returns all tasks for a board
func (r *MemoryRepository) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Task
	for _, task := range r.tasks {
		placement, ok := r.getTaskPlacement(task.ID, boardID)
		if !ok {
			continue
		}
		copy := *task
		copy.BoardID = placement.boardID
		copy.ColumnID = placement.columnID
		copy.Position = placement.position
		result = append(result, &copy)
	}
	return result, nil
}

// ListTasksByColumn returns all tasks in a column
func (r *MemoryRepository) ListTasksByColumn(ctx context.Context, columnID string) ([]*models.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Task
	for _, task := range r.tasks {
		placements, ok := r.taskPlacements[task.ID]
		if !ok {
			continue
		}
		for _, placement := range placements {
			if placement.columnID != columnID {
				continue
			}
			copy := *task
			copy.BoardID = placement.boardID
			copy.ColumnID = placement.columnID
			copy.Position = placement.position
			result = append(result, &copy)
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

// AddTaskToBoard adds a task to a board with placement
func (r *MemoryRepository) AddTaskToBoard(ctx context.Context, taskID, boardID, columnID string, position int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[taskID]; !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	r.addTaskPlacement(taskID, boardID, columnID, position)
	return nil
}

// RemoveTaskFromBoard removes a task from a board
func (r *MemoryRepository) RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[taskID]; !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if placements, ok := r.taskPlacements[taskID]; ok {
		delete(placements, boardID)
	}
	if boards, ok := r.taskBoards[taskID]; ok {
		delete(boards, boardID)
	}
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
func (r *MemoryRepository) ListBoards(ctx context.Context, workspaceID string) ([]*models.Board, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Board, 0, len(r.boards))
	for _, board := range r.boards {
		if workspaceID != "" && board.WorkspaceID != workspaceID {
			continue
		}
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

func (r *MemoryRepository) addTaskPlacement(taskID, boardID, columnID string, position int) {
	if _, ok := r.taskBoards[taskID]; !ok {
		r.taskBoards[taskID] = make(map[string]struct{})
	}
	r.taskBoards[taskID][boardID] = struct{}{}

	if _, ok := r.taskPlacements[taskID]; !ok {
		r.taskPlacements[taskID] = make(map[string]*taskPlacement)
	}
	r.taskPlacements[taskID][boardID] = &taskPlacement{
		boardID:  boardID,
		columnID: columnID,
		position: position,
	}
}

func (r *MemoryRepository) getTaskPlacement(taskID, boardID string) (*taskPlacement, bool) {
	placements, ok := r.taskPlacements[taskID]
	if !ok {
		return nil, false
	}
	placement, ok := placements[boardID]
	return placement, ok
}

// Comment operations

// CreateComment creates a new comment
func (r *MemoryRepository) CreateComment(ctx context.Context, comment *models.Comment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if comment.ID == "" {
		comment.ID = uuid.New().String()
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now().UTC()
	}
	if comment.AuthorType == "" {
		comment.AuthorType = models.CommentAuthorUser
	}

	r.comments[comment.ID] = comment
	return nil
}

// GetComment retrieves a comment by ID
func (r *MemoryRepository) GetComment(ctx context.Context, id string) (*models.Comment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	comment, ok := r.comments[id]
	if !ok {
		return nil, fmt.Errorf("comment not found: %s", id)
	}
	return comment, nil
}

// ListComments returns all comments for a task ordered by creation time
func (r *MemoryRepository) ListComments(ctx context.Context, taskID string) ([]*models.Comment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Comment
	for _, comment := range r.comments {
		if comment.TaskID == taskID {
			result = append(result, comment)
		}
	}
	// Sort by CreatedAt
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].CreatedAt.After(result[j].CreatedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result, nil
}

// DeleteComment deletes a comment by ID
func (r *MemoryRepository) DeleteComment(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.comments[id]; !ok {
		return fmt.Errorf("comment not found: %s", id)
	}
	delete(r.comments, id)
	return nil
}
