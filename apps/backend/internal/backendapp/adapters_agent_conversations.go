package backendapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/state"
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

// agentConversationProfileAdapter wraps the agent settings repository to
// satisfy the narrow agentConversationProfileRepo interface Ensure uses to
// validate a plugin-configured agent profile before creating (or repairing)
// any hidden task/session. GetAgentProfile already filters out soft-deleted
// rows (deleted_at IS NULL), so sql.ErrNoRows covers both "never existed"
// and "deleted".
type agentConversationProfileAdapter struct {
	repo interface {
		GetAgentProfile(ctx context.Context, id string) (*agentsettingsmodels.AgentProfile, error)
	}
}

func (a agentConversationProfileAdapter) GetProfile(ctx context.Context, profileID string) (taskservice.AgentConversationProfileInfo, bool, error) {
	profile, err := a.repo.GetAgentProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return taskservice.AgentConversationProfileInfo{}, false, nil
		}
		return taskservice.AgentConversationProfileInfo{}, false, err
	}
	if profile == nil {
		return taskservice.AgentConversationProfileInfo{}, false, nil
	}
	return taskservice.AgentConversationProfileInfo{Enabled: profile.Enabled}, true, nil
}

// agentConversationStateStoreAdapter wraps the plugin_state SQLite store
// (internal/plugins/state.Store — already durable and shared across backend
// instances via the DB) to satisfy the narrow agentConversationStateRepo
// interface. Claim is atomic at the SQL level (INSERT ... ON CONFLICT DO
// NOTHING against a UNIQUE index), unlike a process-local map: it survives a
// backend restart and stays correct under concurrent callers, including
// separate backend processes sharing the same database.
type agentConversationStateStoreAdapter struct {
	store *state.Store
}

func (a agentConversationStateStoreAdapter) Claim(ctx context.Context, pluginID, scope, scopeID, key string, value json.RawMessage) (bool, error) {
	return a.store.Claim(ctx, pluginID, scope, scopeID, key, value)
}

// agentConversationDispatcherAdapter adapts pluginsTaskMessengerAdapter's
// idempotent delivery primitive to the taskservice.AgentConversationService
// dispatcher seam, so a scheduled coordinator wake reaches the same real
// orchestrator-backed delivery path as the plugin Host's SendMessage RPC —
// launching a never-started session or prompting/resuming an idle one —
// instead of only persisting a message row nothing ever executes.
type agentConversationDispatcherAdapter struct {
	messenger pluginsTaskMessengerAdapter
}

func (a agentConversationDispatcherAdapter) Deliver(ctx context.Context, taskID string, session *taskmodels.TaskSession, text, source, idempotencyID string) (string, error) {
	return a.messenger.StartOrPromptIdempotent(ctx, taskID, session, text, source, idempotencyID)
}

// NewAgentConversationService creates the managed conversation service wired
// to the shared repository, agent settings repository, plugin state store,
// and event bus. Called during boot after the task service, repository, and
// event bus are all available. Its runtime dispatcher is wired later, once
// the orchestrator exists — see SetAgentConversationsDispatcher.
func NewAgentConversationService(
	taskRepo interface {
		ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*taskmodels.Task, int, error)
		CreateTask(ctx context.Context, task *taskmodels.Task) error
		DeleteTask(ctx context.Context, taskID string) error
		GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
		CreateTaskSession(ctx context.Context, session *taskmodels.TaskSession) error
	},
	profileRepo interface {
		GetAgentProfile(ctx context.Context, id string) (*agentsettingsmodels.AgentProfile, error)
	},
	stateStore *state.Store,
	eventBus bus.EventBus,
) *taskservice.AgentConversationService {
	return taskservice.NewAgentConversationService(
		agentConversationTaskAdapter{repo: taskRepo},
		agentConversationSessionAdapter{repo: taskRepo},
		agentConversationProfileAdapter{repo: profileRepo},
		agentConversationStateStoreAdapter{store: stateStore},
		eventBus,
	)
}

// SetAgentConversationsDispatcher wires the live orchestrator-backed
// dispatcher onto an already-constructed AgentConversationService. Deferred
// to main.go's post-orchestrator wiring block (alongside SetWriteDeps) for
// the same boot-ordering reason: the orchestrator is constructed after
// StartActivePlugins spawns boot-active plugins, so it does not exist yet
// when NewAgentConversationService runs during service construction.
func SetAgentConversationsDispatcher(svc *taskservice.AgentConversationService, tasks messengerTaskSvc, orch messengerOrch, log *logger.Logger) {
	svc.SetDispatcher(agentConversationDispatcherAdapter{
		messenger: pluginsTaskMessengerAdapter{tasks: tasks, orch: orch, log: log},
	})
}
