package backendapp

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// agentConversationTaskAdapter wraps the shared repository to satisfy the
// narrow agentConversationTaskRepo interface. The shared repository
// (repos.Task) implements repository.TaskRepository, which has the
// exact method signatures we need.
type agentConversationTaskAdapter struct {
	repo interface {
		ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*taskmodels.Task, int, error)
		CreateTask(ctx context.Context, task *taskmodels.Task) error
		DeleteTask(ctx context.Context, taskID string) error
	}
}

func (a agentConversationTaskAdapter) ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*taskmodels.Task, int, error) {
	return a.repo.ListTasksByWorkspace(ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig)
}

func (a agentConversationTaskAdapter) CreateTask(ctx context.Context, task *taskmodels.Task) error {
	return a.repo.CreateTask(ctx, task)
}

func (a agentConversationTaskAdapter) DeleteTask(ctx context.Context, taskID string) error {
	return a.repo.DeleteTask(ctx, taskID)
}

// agentConversationSessionAdapter wraps the shared repository to satisfy the
// narrow agentConversationSessionRepo interface.
type agentConversationSessionAdapter struct {
	repo interface {
		GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
		CreateTaskSession(ctx context.Context, session *taskmodels.TaskSession) error
	}
}

func (a agentConversationSessionAdapter) GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskSession, error) {
	return a.repo.GetPrimarySessionByTaskID(ctx, taskID)
}

func (a agentConversationSessionAdapter) CreateTaskSession(ctx context.Context, session *taskmodels.TaskSession) error {
	return a.repo.CreateTaskSession(ctx, session)
}

// agentConversationMessageAdapter wraps the shared repository to satisfy the
// narrow agentConversationMessageRepo interface.
type agentConversationMessageAdapter struct {
	repo interface {
		CreateMessage(ctx context.Context, msg *taskmodels.Message) error
	}
}

func (a agentConversationMessageAdapter) CreateMessage(ctx context.Context, msg *taskmodels.Message) error {
	return a.repo.CreateMessage(ctx, msg)
}

// agentConversationStateAdapter provides an in-memory store for occurrence-key
// deduplication. In production, this is a best-effort cache that restarts with
// the process; the Host's occurrence-claim mechanism itself handles RESTART
// recovery through the dispatch-request flow (an occurrence key is known only
// to the scheduler that generated it, and a fresh process generates fresh
// keys, so the old claims are effectively GC'd at restart).
type agentConversationStateAdapter struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newAgentConversationStateAdapter() *agentConversationStateAdapter {
	return &agentConversationStateAdapter{data: make(map[string][]byte)}
}

func (a *agentConversationStateAdapter) Get(_ context.Context, scope, scopeID, key string) ([]byte, bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	k := scope + "/" + scopeID + "/" + key
	v, ok := a.data[k]
	return v, ok, nil
}

func (a *agentConversationStateAdapter) Set(_ context.Context, scope, scopeID, key string, value []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	k := scope + "/" + scopeID + "/" + key
	a.data[k] = value
	return nil
}

// NewAgentConversationService creates the managed conversation service wired
// to the shared repository and event bus. Called during boot after the
// task service, repository, and event bus are all available.
func NewAgentConversationService(
	taskRepo interface {
		ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*taskmodels.Task, int, error)
		CreateTask(ctx context.Context, task *taskmodels.Task) error
		DeleteTask(ctx context.Context, taskID string) error
		GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
		CreateTaskSession(ctx context.Context, session *taskmodels.TaskSession) error
		CreateMessage(ctx context.Context, msg *taskmodels.Message) error
	},
	eventBus bus.EventBus,
) *taskservice.AgentConversationService {
	return taskservice.NewAgentConversationService(
		agentConversationTaskAdapter{repo: taskRepo},
		agentConversationSessionAdapter{repo: taskRepo},
		agentConversationMessageAdapter{repo: taskRepo},
		newAgentConversationStateAdapter(),
		eventBus,
	)
}
