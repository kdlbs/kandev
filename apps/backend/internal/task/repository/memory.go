package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	repositories   map[string]*models.Repository
	repoScripts    map[string]*models.RepositoryScript
	agentSessions  map[string]*models.AgentSession
	executors      map[string]*models.Executor
	environments   map[string]*models.Environment
	taskBoards     map[string]map[string]struct{}
	taskPlacements map[string]map[string]*taskPlacement
	mu             sync.RWMutex
}

// Ensure MemoryRepository implements Repository interface
var _ Repository = (*MemoryRepository)(nil)

// NewMemoryRepository creates a new in-memory task repository
func NewMemoryRepository() *MemoryRepository {
	repo := &MemoryRepository{
		workspaces:     make(map[string]*models.Workspace),
		tasks:          make(map[string]*models.Task),
		boards:         make(map[string]*models.Board),
		columns:        make(map[string]*models.Column),
		comments:       make(map[string]*models.Comment),
		repositories:   make(map[string]*models.Repository),
		repoScripts:    make(map[string]*models.RepositoryScript),
		agentSessions:  make(map[string]*models.AgentSession),
		executors:      make(map[string]*models.Executor),
		environments:   make(map[string]*models.Environment),
		taskBoards:     make(map[string]map[string]struct{}),
		taskPlacements: make(map[string]map[string]*taskPlacement),
	}
	repo.seedDefaults()
	return repo
}

func (r *MemoryRepository) seedDefaults() {
	now := time.Now().UTC()
	r.executors[models.ExecutorIDLocalPC] = &models.Executor{
		ID:        models.ExecutorIDLocalPC,
		Name:      "Local PC",
		Type:      models.ExecutorTypeLocalPC,
		Status:    models.ExecutorStatusActive,
		IsSystem:  true,
		Config:    map[string]string{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID:        models.ExecutorIDLocalDocker,
		Name:      "Local Docker",
		Type:      models.ExecutorTypeLocalDocker,
		Status:    models.ExecutorStatusActive,
		IsSystem:  false,
		Config:    map[string]string{"docker_host": "unix:///var/run/docker.sock"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.executors[models.ExecutorIDRemoteDocker] = &models.Executor{
		ID:        models.ExecutorIDRemoteDocker,
		Name:      "Remote Docker",
		Type:      models.ExecutorTypeRemoteDocker,
		Status:    models.ExecutorStatusDisabled,
		IsSystem:  false,
		Config:    map[string]string{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.environments[models.EnvironmentIDLocal] = &models.Environment{
		ID:           models.EnvironmentIDLocal,
		Name:         "Local",
		Kind:         models.EnvironmentKindLocalPC,
		WorktreeRoot: "~/kandev",
		CreatedAt:    now,
		UpdatedAt:    now,
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

// GetColumnByState retrieves a column by board ID and state
func (r *MemoryRepository) GetColumnByState(ctx context.Context, boardID string, state v1.TaskState) (*models.Column, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, column := range r.columns {
		if column.BoardID == boardID && column.State == state {
			return column, nil
		}
	}
	return nil, fmt.Errorf("column not found for board %s with state %s", boardID, state)
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

// Repository operations

func (r *MemoryRepository) CreateRepository(ctx context.Context, repository *models.Repository) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if repository.ID == "" {
		repository.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	repository.CreatedAt = now
	repository.UpdatedAt = now
	r.repositories[repository.ID] = repository
	return nil
}

func (r *MemoryRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	repository, ok := r.repositories[id]
	if !ok {
		return nil, fmt.Errorf("repository not found: %s", id)
	}
	return repository, nil
}

func (r *MemoryRepository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.repositories[repository.ID]; !ok {
		return fmt.Errorf("repository not found: %s", repository.ID)
	}
	repository.UpdatedAt = time.Now().UTC()
	r.repositories[repository.ID] = repository
	return nil
}

func (r *MemoryRepository) DeleteRepository(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.repositories[id]; !ok {
		return fmt.Errorf("repository not found: %s", id)
	}
	delete(r.repositories, id)
	for scriptID, script := range r.repoScripts {
		if script.RepositoryID == id {
			delete(r.repoScripts, scriptID)
		}
	}
	return nil
}

func (r *MemoryRepository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Repository, 0)
	for _, repository := range r.repositories {
		if repository.WorkspaceID == workspaceID {
			result = append(result, repository)
		}
	}
	return result, nil
}

// Repository script operations

func (r *MemoryRepository) CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if script.ID == "" {
		script.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	script.CreatedAt = now
	script.UpdatedAt = now
	r.repoScripts[script.ID] = script
	return nil
}

func (r *MemoryRepository) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	script, ok := r.repoScripts[id]
	if !ok {
		return nil, fmt.Errorf("repository script not found: %s", id)
	}
	return script, nil
}

func (r *MemoryRepository) UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.repoScripts[script.ID]; !ok {
		return fmt.Errorf("repository script not found: %s", script.ID)
	}
	script.UpdatedAt = time.Now().UTC()
	r.repoScripts[script.ID] = script
	return nil
}

func (r *MemoryRepository) DeleteRepositoryScript(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.repoScripts[id]; !ok {
		return fmt.Errorf("repository script not found: %s", id)
	}
	delete(r.repoScripts, id)
	return nil
}

func (r *MemoryRepository) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.RepositoryScript, 0)
	for _, script := range r.repoScripts {
		if script.RepositoryID == repositoryID {
			result = append(result, script)
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
	if comment.Type == "" {
		comment.Type = models.CommentTypeMessage
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

// GetCommentByToolCallID retrieves a comment by task ID and tool_call_id in metadata.
func (r *MemoryRepository) GetCommentByToolCallID(ctx context.Context, taskID, toolCallID string) (*models.Comment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, comment := range r.comments {
		if comment.TaskID != taskID || comment.Metadata == nil {
			continue
		}
		if value, ok := comment.Metadata["tool_call_id"]; ok {
			if toolCallIDValue, ok := value.(string); ok && toolCallIDValue == toolCallID {
				return comment, nil
			}
		}
	}
	return nil, fmt.Errorf("comment not found: %s", toolCallID)
}

// UpdateComment updates an existing comment.
func (r *MemoryRepository) UpdateComment(ctx context.Context, comment *models.Comment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.comments[comment.ID]
	if !ok {
		return fmt.Errorf("comment not found: %s", comment.ID)
	}
	comment.CreatedAt = existing.CreatedAt
	r.comments[comment.ID] = comment
	return nil
}

// ListComments returns all comments for a task ordered by creation time.
func (r *MemoryRepository) ListComments(ctx context.Context, taskID string) ([]*models.Comment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Comment
	for _, comment := range r.comments {
		if comment.TaskID == taskID {
			result = append(result, comment)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// ListCommentsPaginated returns comments for a task ordered by creation time with pagination.
func (r *MemoryRepository) ListCommentsPaginated(ctx context.Context, taskID string, opts ListCommentsOptions) ([]*models.Comment, bool, error) {
	limit := opts.Limit
	if limit < 0 {
		limit = 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var cursor *models.Comment
	if opts.Before != "" {
		comment, ok := r.comments[opts.Before]
		if !ok {
			return nil, false, fmt.Errorf("comment not found: %s", opts.Before)
		}
		if comment.TaskID != taskID {
			return nil, false, fmt.Errorf("comment cursor not found: %s", opts.Before)
		}
		cursor = comment
	}
	if opts.After != "" {
		comment, ok := r.comments[opts.After]
		if !ok {
			return nil, false, fmt.Errorf("comment not found: %s", opts.After)
		}
		if comment.TaskID != taskID {
			return nil, false, fmt.Errorf("comment cursor not found: %s", opts.After)
		}
		cursor = comment
	}

	var result []*models.Comment
	for _, comment := range r.comments {
		if comment.TaskID == taskID {
			result = append(result, comment)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	if cursor != nil {
		filtered := result[:0]
		for _, comment := range result {
			if opts.Before != "" {
				if comment.CreatedAt.Before(cursor.CreatedAt) || (comment.CreatedAt.Equal(cursor.CreatedAt) && comment.ID < cursor.ID) {
					filtered = append(filtered, comment)
				}
			} else if opts.After != "" {
				if comment.CreatedAt.After(cursor.CreatedAt) || (comment.CreatedAt.Equal(cursor.CreatedAt) && comment.ID > cursor.ID) {
					filtered = append(filtered, comment)
				}
			}
		}
		result = filtered
	}

	if strings.EqualFold(opts.Sort, "desc") {
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
	}

	hasMore := false
	if limit > 0 && len(result) > limit {
		hasMore = true
		result = result[:limit]
	}
	return result, hasMore, nil
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

// AgentSession operations

// CreateAgentSession creates a new agent session
func (r *MemoryRepository) CreateAgentSession(ctx context.Context, session *models.AgentSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if session.StartedAt.IsZero() {
		session.StartedAt = now
	}
	session.UpdatedAt = now

	r.agentSessions[session.ID] = session
	return nil
}

// GetAgentSession retrieves an agent session by ID
func (r *MemoryRepository) GetAgentSession(ctx context.Context, id string) (*models.AgentSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.agentSessions[id]
	if !ok {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	return session, nil
}

// GetAgentSessionByTaskID retrieves the most recent agent session for a task
func (r *MemoryRepository) GetAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest *models.AgentSession
	for _, session := range r.agentSessions {
		if session.TaskID == taskID {
			if latest == nil || session.StartedAt.After(latest.StartedAt) {
				latest = session
			}
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("no agent session found for task: %s", taskID)
	}
	return latest, nil
}

// GetActiveAgentSessionByTaskID retrieves the active agent session for a task
func (r *MemoryRepository) GetActiveAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, session := range r.agentSessions {
		if session.TaskID == taskID && (session.Status == models.AgentSessionStatusRunning || session.Status == models.AgentSessionStatusWaiting || session.Status == models.AgentSessionStatusPending) {
			return session, nil
		}
	}
	return nil, fmt.Errorf("no active agent session found for task: %s", taskID)
}

// UpdateAgentSession updates an existing agent session
func (r *MemoryRepository) UpdateAgentSession(ctx context.Context, session *models.AgentSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.agentSessions[session.ID]; !ok {
		return fmt.Errorf("agent session not found: %s", session.ID)
	}
	session.UpdatedAt = time.Now().UTC()
	r.agentSessions[session.ID] = session
	return nil
}

// UpdateAgentSessionStatus updates the status of an agent session
func (r *MemoryRepository) UpdateAgentSessionStatus(ctx context.Context, id string, status models.AgentSessionStatus, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.agentSessions[id]
	if !ok {
		return fmt.Errorf("agent session not found: %s", id)
	}
	session.Status = status
	session.ErrorMessage = errorMessage
	session.UpdatedAt = time.Now().UTC()
	if status == models.AgentSessionStatusCompleted || status == models.AgentSessionStatusFailed || status == models.AgentSessionStatusStopped {
		now := time.Now().UTC()
		session.CompletedAt = &now
	}
	return nil
}

// ListAgentSessions returns all agent sessions for a task
func (r *MemoryRepository) ListAgentSessions(ctx context.Context, taskID string) ([]*models.AgentSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.AgentSession
	for _, session := range r.agentSessions {
		if session.TaskID == taskID {
			result = append(result, session)
		}
	}
	return result, nil
}

// ListActiveAgentSessions returns all active agent sessions
func (r *MemoryRepository) ListActiveAgentSessions(ctx context.Context) ([]*models.AgentSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.AgentSession
	for _, session := range r.agentSessions {
		if session.Status == models.AgentSessionStatusRunning || session.Status == models.AgentSessionStatusWaiting || session.Status == models.AgentSessionStatusPending {
			result = append(result, session)
		}
	}
	return result, nil
}

// DeleteAgentSession deletes an agent session by ID
func (r *MemoryRepository) DeleteAgentSession(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.agentSessions[id]; !ok {
		return fmt.Errorf("agent session not found: %s", id)
	}
	delete(r.agentSessions, id)
	return nil
}

// Executor operations

func (r *MemoryRepository) CreateExecutor(ctx context.Context, executor *models.Executor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if executor.ID == "" {
		executor.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	executor.CreatedAt = now
	executor.UpdatedAt = now

	r.executors[executor.ID] = executor
	return nil
}

func (r *MemoryRepository) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	executor, ok := r.executors[id]
	if !ok {
		return nil, fmt.Errorf("executor not found: %s", id)
	}
	return executor, nil
}

func (r *MemoryRepository) UpdateExecutor(ctx context.Context, executor *models.Executor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.executors[executor.ID]; !ok {
		return fmt.Errorf("executor not found: %s", executor.ID)
	}
	executor.UpdatedAt = time.Now().UTC()
	r.executors[executor.ID] = executor
	return nil
}

func (r *MemoryRepository) DeleteExecutor(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.executors[id]; !ok {
		return fmt.Errorf("executor not found: %s", id)
	}
	delete(r.executors, id)
	return nil
}

func (r *MemoryRepository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Executor, 0, len(r.executors))
	for _, executor := range r.executors {
		result = append(result, executor)
	}
	return result, nil
}

// Environment operations

func (r *MemoryRepository) CreateEnvironment(ctx context.Context, environment *models.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if environment.ID == "" {
		environment.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	environment.CreatedAt = now
	environment.UpdatedAt = now
	r.environments[environment.ID] = environment
	return nil
}

func (r *MemoryRepository) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	environment, ok := r.environments[id]
	if !ok {
		return nil, fmt.Errorf("environment not found: %s", id)
	}
	return environment, nil
}

func (r *MemoryRepository) UpdateEnvironment(ctx context.Context, environment *models.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.environments[environment.ID]; !ok {
		return fmt.Errorf("environment not found: %s", environment.ID)
	}
	environment.UpdatedAt = time.Now().UTC()
	r.environments[environment.ID] = environment
	return nil
}

func (r *MemoryRepository) DeleteEnvironment(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.environments[id]; !ok {
		return fmt.Errorf("environment not found: %s", id)
	}
	delete(r.environments, id)
	return nil
}

func (r *MemoryRepository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Environment, 0, len(r.environments))
	for _, environment := range r.environments {
		result = append(result, environment)
	}
	return result, nil
}
