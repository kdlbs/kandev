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
	messages       map[string]*models.Message
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
		messages:       make(map[string]*models.Message),
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

// AddTaskToBoard adds a task to a board with placement
func (r *MemoryRepository) AddTaskToBoard(ctx context.Context, taskID, boardID, columnID string, position int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	task.BoardID = boardID
	task.ColumnID = columnID
	task.Position = position
	task.UpdatedAt = time.Now().UTC()
	r.tasks[taskID] = task
	r.addTaskPlacement(taskID, boardID, columnID, position)
	return nil
}

// RemoveTaskFromBoard removes a task from a board
func (r *MemoryRepository) RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, ok := r.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if task.BoardID == boardID {
		task.BoardID = ""
		task.ColumnID = ""
		task.Position = 0
		task.UpdatedAt = time.Now().UTC()
		r.tasks[taskID] = task
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
	if !ok || repository.DeletedAt != nil {
		return nil, fmt.Errorf("repository not found: %s", id)
	}
	return repository, nil
}

func (r *MemoryRepository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.repositories[repository.ID]; !ok || existing.DeletedAt != nil {
		return fmt.Errorf("repository not found: %s", repository.ID)
	}
	repository.UpdatedAt = time.Now().UTC()
	r.repositories[repository.ID] = repository
	return nil
}

func (r *MemoryRepository) DeleteRepository(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	repository, ok := r.repositories[id]
	if !ok || repository.DeletedAt != nil {
		return fmt.Errorf("repository not found: %s", id)
	}
	now := time.Now().UTC()
	repository.DeletedAt = &now
	repository.UpdatedAt = now
	r.repositories[id] = repository
	return nil
}

func (r *MemoryRepository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Repository, 0)
	for _, repository := range r.repositories {
		if repository.WorkspaceID == workspaceID && repository.DeletedAt == nil {
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

func isSessionActive(status models.AgentSessionState) bool {
	return status == models.AgentSessionStateCreated ||
		status == models.AgentSessionStateRunning ||
		status == models.AgentSessionStateWaitingForInput ||
		status == models.AgentSessionStateStarting
}

// Message operations

// CreateMessage creates a new message
func (r *MemoryRepository) CreateMessage(ctx context.Context, message *models.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorUser
	}
	if message.Type == "" {
		message.Type = models.MessageTypeMessage
	}

	r.messages[message.ID] = message
	return nil
}

// GetMessage retrieves a message by ID
func (r *MemoryRepository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	message, ok := r.messages[id]
	if !ok {
		return nil, fmt.Errorf("message not found: %s", id)
	}
	return message, nil
}

// GetMessageByToolCallID retrieves a message by session ID and tool_call_id in metadata.
func (r *MemoryRepository) GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, message := range r.messages {
		if message.AgentSessionID != sessionID || message.Metadata == nil {
			continue
		}
		if value, ok := message.Metadata["tool_call_id"]; ok {
			if toolCallIDValue, ok := value.(string); ok && toolCallIDValue == toolCallID {
				return message, nil
			}
		}
	}
	return nil, fmt.Errorf("message not found: %s", toolCallID)
}

// UpdateMessage updates an existing message.
func (r *MemoryRepository) UpdateMessage(ctx context.Context, message *models.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.messages[message.ID]
	if !ok {
		return fmt.Errorf("message not found: %s", message.ID)
	}
	message.CreatedAt = existing.CreatedAt
	r.messages[message.ID] = message
	return nil
}

// ListMessages returns all messages for a session ordered by creation time.
func (r *MemoryRepository) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Message
	for _, message := range r.messages {
		if message.AgentSessionID == sessionID {
			result = append(result, message)
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

// ListMessagesPaginated returns messages for a session ordered by creation time with pagination.
func (r *MemoryRepository) ListMessagesPaginated(ctx context.Context, sessionID string, opts ListMessagesOptions) ([]*models.Message, bool, error) {
	limit := opts.Limit
	if limit < 0 {
		limit = 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var cursor *models.Message
	if opts.Before != "" {
		message, ok := r.messages[opts.Before]
		if !ok {
			return nil, false, fmt.Errorf("message not found: %s", opts.Before)
		}
		if message.AgentSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.Before)
		}
		cursor = message
	}
	if opts.After != "" {
		message, ok := r.messages[opts.After]
		if !ok {
			return nil, false, fmt.Errorf("message not found: %s", opts.After)
		}
		if message.AgentSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.After)
		}
		cursor = message
	}

	var result []*models.Message
	for _, message := range r.messages {
		if message.AgentSessionID == sessionID {
			result = append(result, message)
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
		for _, message := range result {
			if opts.Before != "" {
				if message.CreatedAt.Before(cursor.CreatedAt) || (message.CreatedAt.Equal(cursor.CreatedAt) && message.ID < cursor.ID) {
					filtered = append(filtered, message)
				}
			} else if opts.After != "" {
				if message.CreatedAt.After(cursor.CreatedAt) || (message.CreatedAt.Equal(cursor.CreatedAt) && message.ID > cursor.ID) {
					filtered = append(filtered, message)
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

// DeleteMessage deletes a message by ID
func (r *MemoryRepository) DeleteMessage(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.messages[id]; !ok {
		return fmt.Errorf("message not found: %s", id)
	}
	delete(r.messages, id)
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
	if session.State == "" {
		session.State = models.AgentSessionStateCreated
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
		if session.TaskID == taskID && (session.State == models.AgentSessionStateCreated || session.State == models.AgentSessionStateStarting || session.State == models.AgentSessionStateRunning || session.State == models.AgentSessionStateWaitingForInput) {
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

// UpdateAgentSessionState updates the status of an agent session
func (r *MemoryRepository) UpdateAgentSessionState(ctx context.Context, id string, status models.AgentSessionState, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.agentSessions[id]
	if !ok {
		return fmt.Errorf("agent session not found: %s", id)
	}
	session.State = status
	session.ErrorMessage = errorMessage
	session.UpdatedAt = time.Now().UTC()
	if status == models.AgentSessionStateCompleted || status == models.AgentSessionStateFailed || status == models.AgentSessionStateCancelled {
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
		if session.State == models.AgentSessionStateCreated || session.State == models.AgentSessionStateStarting || session.State == models.AgentSessionStateRunning || session.State == models.AgentSessionStateWaitingForInput {
			result = append(result, session)
		}
	}
	return result, nil
}

func (r *MemoryRepository) HasActiveAgentSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, session := range r.agentSessions {
		if session.AgentProfileID == agentProfileID && isSessionActive(session.State) {
			return true, nil
		}
	}
	return false, nil
}

func (r *MemoryRepository) HasActiveAgentSessionsByExecutor(ctx context.Context, executorID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, session := range r.agentSessions {
		if session.ExecutorID == executorID && isSessionActive(session.State) {
			return true, nil
		}
	}
	return false, nil
}

func (r *MemoryRepository) HasActiveAgentSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, session := range r.agentSessions {
		if session.EnvironmentID == environmentID && isSessionActive(session.State) {
			return true, nil
		}
	}
	return false, nil
}

func (r *MemoryRepository) HasActiveAgentSessionsByRepository(ctx context.Context, repositoryID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, session := range r.agentSessions {
		if !isSessionActive(session.State) {
			continue
		}
		task, ok := r.tasks[session.TaskID]
		if !ok {
			continue
		}
		if task.RepositoryID == repositoryID {
			return true, nil
		}
	}
	return false, nil
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
	if !ok || executor.DeletedAt != nil {
		return nil, fmt.Errorf("executor not found: %s", id)
	}
	return executor, nil
}

func (r *MemoryRepository) UpdateExecutor(ctx context.Context, executor *models.Executor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.executors[executor.ID]; !ok || existing.DeletedAt != nil {
		return fmt.Errorf("executor not found: %s", executor.ID)
	}
	executor.UpdatedAt = time.Now().UTC()
	r.executors[executor.ID] = executor
	return nil
}

func (r *MemoryRepository) DeleteExecutor(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	executor, ok := r.executors[id]
	if !ok || executor.DeletedAt != nil {
		return fmt.Errorf("executor not found: %s", id)
	}
	now := time.Now().UTC()
	executor.DeletedAt = &now
	executor.UpdatedAt = now
	r.executors[id] = executor
	return nil
}

func (r *MemoryRepository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Executor, 0, len(r.executors))
	for _, executor := range r.executors {
		if executor.DeletedAt == nil {
			result = append(result, executor)
		}
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
	if !ok || environment.DeletedAt != nil {
		return nil, fmt.Errorf("environment not found: %s", id)
	}
	return environment, nil
}

func (r *MemoryRepository) UpdateEnvironment(ctx context.Context, environment *models.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.environments[environment.ID]; !ok || existing.DeletedAt != nil {
		return fmt.Errorf("environment not found: %s", environment.ID)
	}
	environment.UpdatedAt = time.Now().UTC()
	r.environments[environment.ID] = environment
	return nil
}

func (r *MemoryRepository) DeleteEnvironment(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	environment, ok := r.environments[id]
	if !ok || environment.DeletedAt != nil {
		return fmt.Errorf("environment not found: %s", id)
	}
	now := time.Now().UTC()
	environment.DeletedAt = &now
	environment.UpdatedAt = now
	r.environments[id] = environment
	return nil
}

func (r *MemoryRepository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Environment, 0, len(r.environments))
	for _, environment := range r.environments {
		if environment.DeletedAt == nil {
			result = append(result, environment)
		}
	}
	return result, nil
}
