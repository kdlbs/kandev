package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
)

// WorktreeCleanup provides worktree cleanup on task deletion.
type WorktreeCleanup interface {
	// OnTaskDeleted is called when a task is deleted to clean up its worktree.
	OnTaskDeleted(ctx context.Context, taskID string) error
}

// WorktreeProvider extends WorktreeCleanup with query capabilities.
// Implementations that support this can be type-asserted from WorktreeCleanup.
type WorktreeProvider interface {
	WorktreeCleanup
	// GetAllByTaskID returns all worktrees associated with a task.
	GetAllByTaskID(ctx context.Context, taskID string) ([]*worktree.Worktree, error)
}

// WorktreeBatchCleaner extends WorktreeProvider with batch cleanup.
type WorktreeBatchCleaner interface {
	WorktreeProvider
	// CleanupWorktrees removes multiple worktrees in a single operation.
	CleanupWorktrees(ctx context.Context, worktrees []*worktree.Worktree) error
}

// TaskExecutionStopper stops active task execution (agent session + instance).
type TaskExecutionStopper interface {
	StopTask(ctx context.Context, taskID, reason string, force bool) error
	StopSession(ctx context.Context, sessionID, reason string, force bool) error
}

// WorkflowStepCreator creates workflow steps from a template for a workflow.
type WorkflowStepCreator interface {
	CreateStepsFromTemplate(ctx context.Context, workflowID, templateID string) error
}

// WorkflowStepGetter retrieves workflow step information.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
	// GetNextStepByPosition returns the next step after the given position for a workflow.
	// Returns nil if there is no next step (i.e., current step is the last one).
	GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
}

// StartStepResolver resolves the starting step for a workflow.
type StartStepResolver interface {
	ResolveStartStep(ctx context.Context, workflowID string) (string, error)
}

var (
	ErrActiveTaskSessions        = errors.New("active agent sessions exist")
	ErrInvalidRepositorySettings = errors.New("invalid repository settings")
	ErrInvalidExecutorConfig     = errors.New("invalid executor config")
)

func validateExecutorConfig(config map[string]string) error {
	if config == nil {
		return nil
	}
	policy := strings.TrimSpace(config["mcp_policy"])
	if policy == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(policy), &decoded); err != nil {
		return fmt.Errorf("%w: mcp_policy must be valid JSON", ErrInvalidExecutorConfig)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return fmt.Errorf("%w: mcp_policy must be a JSON object", ErrInvalidExecutorConfig)
	}
	return nil
}

// Repos holds the repository sub-interfaces used by the task service.
type Repos struct {
	Workspaces   repository.WorkspaceRepository
	Tasks        repository.TaskRepository
	TaskRepos    repository.TaskRepoRepository
	Workflows    repository.WorkflowRepository
	Messages     repository.MessageRepository
	Turns        repository.TurnRepository
	Sessions     repository.SessionRepository
	GitSnapshots repository.GitSnapshotRepository
	RepoEntities repository.RepositoryEntityRepository
	Executors    repository.ExecutorRepository
	Environments repository.EnvironmentRepository
	Reviews      repository.ReviewRepository
}

// Service provides task business logic
type Service struct {
	workspaces          repository.WorkspaceRepository
	tasks               repository.TaskRepository
	taskRepos           repository.TaskRepoRepository
	workflows           repository.WorkflowRepository
	messages            repository.MessageRepository
	turns               repository.TurnRepository
	sessions            repository.SessionRepository
	gitSnapshots        repository.GitSnapshotRepository
	repoEntities        repository.RepositoryEntityRepository
	executors           repository.ExecutorRepository
	environments        repository.EnvironmentRepository
	reviews             repository.ReviewRepository
	eventBus            bus.EventBus
	logger              *logger.Logger
	discoveryConfig     RepositoryDiscoveryConfig
	worktreeCleanup     WorktreeCleanup
	executionStopper    TaskExecutionStopper
	workflowStepCreator WorkflowStepCreator
	workflowStepGetter  WorkflowStepGetter
	startStepResolver   StartStepResolver
}

// NewService creates a new task service
func NewService(repos Repos, eventBus bus.EventBus, log *logger.Logger, discoveryConfig RepositoryDiscoveryConfig) *Service {
	return &Service{
		workspaces:      repos.Workspaces,
		tasks:           repos.Tasks,
		taskRepos:       repos.TaskRepos,
		workflows:       repos.Workflows,
		messages:        repos.Messages,
		turns:           repos.Turns,
		sessions:        repos.Sessions,
		gitSnapshots:    repos.GitSnapshots,
		repoEntities:    repos.RepoEntities,
		executors:       repos.Executors,
		environments:    repos.Environments,
		reviews:         repos.Reviews,
		eventBus:        eventBus,
		logger:          log,
		discoveryConfig: discoveryConfig,
	}
}

// SetWorktreeCleanup sets the worktree cleanup handler for task deletion.
func (s *Service) SetWorktreeCleanup(cleanup WorktreeCleanup) {
	s.worktreeCleanup = cleanup
}

// SetExecutionStopper wires the task execution stopper (orchestrator).
func (s *Service) SetExecutionStopper(stopper TaskExecutionStopper) {
	s.executionStopper = stopper
}

// SetWorkflowStepCreator wires the workflow step creator for workflow creation.
func (s *Service) SetWorkflowStepCreator(creator WorkflowStepCreator) {
	s.workflowStepCreator = creator
}

// SetWorkflowStepGetter wires the workflow step getter for MoveTask.
func (s *Service) SetWorkflowStepGetter(getter WorkflowStepGetter) {
	s.workflowStepGetter = getter
}

// SetStartStepResolver wires the start step resolver for CreateTask.
func (s *Service) SetStartStepResolver(resolver StartStepResolver) {
	s.startStepResolver = resolver
}
