package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

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
	StopExecution(ctx context.Context, executionID, reason string, force bool) error
	// RegisterExecutionStopOwner records exact teardown ownership before a
	// terminal session mutation. It never replaces the explicit stop call.
	RegisterExecutionStopOwner(sessionID, executionID string, force bool)
}

// TaskResourceCleanupActivityGate serializes durable cleanup with install-wide maintenance.
type TaskResourceCleanupActivityGate interface {
	AcquireTaskResourceCleanup(context.Context) (TaskResourceCleanupActivityLease, error)
}

type TaskResourceCleanupActivityLease interface {
	Release()
}

// ProviderDefaultBranchProber resolves a provider repo's default branch
// (e.g. "main" / "master") without requiring a local clone. Used by
// AddBranchToTask to satisfy the worktree-create precondition synchronously,
// since add_branch does not trigger the executor-side backfillRepoDefaultBranch
// path. Default implementation (cmd/kandev) shells out to
// `git ls-remote --symref`; tests inject a stub via SetProviderDefaultBranchProber.
//
// Implementations MUST honour ctx cancellation so a slow / hung remote does not
// stall the calling MCP tool. Returns ("", error) on probe failure — callers
// fall through to the explicit "cannot resolve base_branch" rejection rather
// than persisting an empty-default row.
type ProviderDefaultBranchProber interface {
	ProbeDefaultBranch(ctx context.Context, provider, owner, name string) (string, error)
}

// BranchMaterializer creates a worktree on disk and persists a
// task_session_worktrees row for a newly added task_repository row, without
// restarting the agent. Used by AddBranchToTask so MCP-driven "add a branch
// to this task" actually surfaces the new worktree in the UI on the next
// poll, rather than waiting for a session relaunch.
//
// The implementation lives in cmd/kandev (it needs worktree.Manager, the
// session/env repos, and the repository entity layer) — the service layer
// only knows the abstract capability.
// AgentBaseBranchPusher pushes an updated per-repo base-branch map to the
// agentctl instance(s) of any running execution for a task. Used by
// UpdateRepositoryBaseBranch so the changes-panel "Compare against" picker
// updates BaseCommit / Ahead / Behind live, not just at next session start.
// Implementations must be no-op-safe when the task has no running execution.
type AgentBaseBranchPusher interface {
	PushBaseBranchesForTask(ctx context.Context, taskID string, branches map[string]string)
}

type BranchMaterializer interface {
	// MaterializeBranch creates the worktree for a freshly inserted
	// task_repositories row. Best-effort: when no active session exists yet
	// the implementation may choose to no-op and let the next session launch
	// create the worktree via the standard multi-repo prepare path.
	MaterializeBranch(ctx context.Context, taskID, taskRepositoryID string) error
}

// GitArchiveCapture captures git state (commits, cumulative diff) when a task is archived.
// This allows preserving the final git state of a session for historical purposes.
type GitArchiveCapture interface {
	// CaptureArchiveSnapshot captures the git state for a session before archiving.
	// Returns nil if capture is not possible (e.g., agent not running).
	CaptureArchiveSnapshot(ctx context.Context, sessionID string) error
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

// PRTaskResolver resolves which tasks are associated with a GitHub PR number.
// Implemented by the github service; injected so the task service can surface a
// task by its PR number in search without coupling to the github schema.
type PRTaskResolver interface {
	FindTaskIDsByPRNumber(ctx context.Context, workspaceID string, prNumber int) ([]string, error)
}

// StartStepResolver resolves the starting step for a workflow.
type StartStepResolver interface {
	ResolveStartStep(ctx context.Context, workflowID string) (string, error)
	ResolveFirstStep(ctx context.Context, workflowID string) (string, error)
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
	Workspaces       repository.WorkspaceRepository
	Tasks            repository.TaskRepository
	TaskRepos        repository.TaskRepoRepository
	Workflows        repository.WorkflowRepository
	Messages         repository.MessageRepository
	Turns            repository.TurnRepository
	Sessions         repository.SessionRepository
	GitSnapshots     repository.GitSnapshotRepository
	RepoEntities     repository.RepositoryEntityRepository
	Executors        repository.ExecutorRepository
	Environments     repository.EnvironmentRepository
	TaskEnvironments repository.TaskEnvironmentRepository
	Reviews          repository.ReviewRepository
	ResourceCleanups repository.TaskResourceCleanupRepository
}

// Service provides task business logic
type Service struct {
	workspaces            repository.WorkspaceRepository
	tasks                 repository.TaskRepository
	taskRepos             repository.TaskRepoRepository
	workflows             repository.WorkflowRepository
	messages              repository.MessageRepository
	turns                 repository.TurnRepository
	sessions              repository.SessionRepository
	gitSnapshots          repository.GitSnapshotRepository
	repoEntities          repository.RepositoryEntityRepository
	executors             repository.ExecutorRepository
	environments          repository.EnvironmentRepository
	taskEnvironments      repository.TaskEnvironmentRepository
	reviews               repository.ReviewRepository
	resourceCleanups      repository.TaskResourceCleanupRepository
	eventBus              bus.EventBus
	logger                *logger.Logger
	discoveryConfig       RepositoryDiscoveryConfig
	worktreeCleanup       WorktreeCleanup
	executionStopper      TaskExecutionStopper
	cleanupActivity       TaskResourceCleanupActivityGate
	branchMaterializer    BranchMaterializer
	providerProber        ProviderDefaultBranchProber
	gitArchiveCapture     GitArchiveCapture
	workflowStepCreator   WorkflowStepCreator
	workflowStepGetter    WorkflowStepGetter
	startStepResolver     StartStepResolver
	prTaskResolver        PRTaskResolver
	quickChatDir          string // Directory for quick-chat workspaces (e.g., ~/.kandev/quick-chat)
	branchFetcher         *branchFetcher
	envDestroyer          EnvironmentDestroyer
	sessionRunningChecker SessionRunningChecker
	remoteBranchLister    RemoteBranchLister
	repoCloneLocation     RepoCloneLocation
	blockers              BlockerRepository
	comments              CommentRepository
	baseBranchPusher      AgentBaseBranchPusher
	runtimeOverridesMu    sync.Mutex
	// cleanupDoneForTest lets unit tests wait for async cleanup; nil in production.
	cleanupDoneForTest  chan struct{}
	cleanupWorkerMu     sync.Mutex
	cleanupWorkerCancel context.CancelFunc
	cleanupWorkerWG     sync.WaitGroup
	cleanupWorkerWake   chan struct{}
	cleanupRunsMu       sync.Mutex
	cleanupRuns         map[*taskResourceCleanupRun]struct{}
}

// NewService creates a new task service
func NewService(repos Repos, eventBus bus.EventBus, log *logger.Logger, discoveryConfig RepositoryDiscoveryConfig) *Service {
	return &Service{
		workspaces:       repos.Workspaces,
		tasks:            repos.Tasks,
		taskRepos:        repos.TaskRepos,
		workflows:        repos.Workflows,
		messages:         repos.Messages,
		turns:            repos.Turns,
		sessions:         repos.Sessions,
		gitSnapshots:     repos.GitSnapshots,
		repoEntities:     repos.RepoEntities,
		executors:        repos.Executors,
		environments:     repos.Environments,
		taskEnvironments: repos.TaskEnvironments,
		reviews:          repos.Reviews,
		resourceCleanups: repos.ResourceCleanups,
		eventBus:         eventBus,
		logger:           log,
		discoveryConfig:  discoveryConfig,
		branchFetcher:    newBranchFetcher(log.Zap()),
	}
}

// SetWorktreeCleanup sets the worktree cleanup handler for task deletion.
func (s *Service) SetWorktreeCleanup(cleanup WorktreeCleanup) {
	s.worktreeCleanup = cleanup
}

func (s *Service) setCleanupDoneForTestHook(ch chan struct{}) {
	s.cleanupDoneForTest = ch
}

// SetBranchMaterializer wires the mid-session worktree materializer for
// AddBranchToTask. Optional — when unset, MCP add_branch only inserts the
// task_repositories row and the worktree appears on next session launch.
func (s *Service) SetBranchMaterializer(m BranchMaterializer) {
	s.branchMaterializer = m
}

// SetAgentBaseBranchPusher wires the live-update push for
// UpdateRepositoryBaseBranch. Optional — when unset, the persisted DB value
// is the source of truth and the new base branch takes effect at next
// session launch.
func (s *Service) SetAgentBaseBranchPusher(p AgentBaseBranchPusher) {
	s.baseBranchPusher = p
}

// SetProviderDefaultBranchProber wires the synchronous default-branch probe
// used by AddBranchToTask's GitHub-URL resolution. Optional — when unset,
// add_branch with a provider URL and no base_branch falls through to the
// "cannot resolve base_branch" rejection instead of persisting an empty row.
func (s *Service) SetProviderDefaultBranchProber(p ProviderDefaultBranchProber) {
	s.providerProber = p
}

// SetExecutionStopper wires the task execution stopper (orchestrator).
func (s *Service) SetExecutionStopper(stopper TaskExecutionStopper) {
	s.executionStopper = stopper
}

func (s *Service) SetTaskResourceCleanupActivityGate(gate TaskResourceCleanupActivityGate) {
	s.cleanupActivity = gate
}

// SetGitArchiveCapture wires the git archive capture handler.
func (s *Service) SetGitArchiveCapture(capture GitArchiveCapture) {
	s.gitArchiveCapture = capture
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

// SetPRTaskResolver wires the GitHub PR→task resolver for PR-number search.
// Optional — when unset, search by PR number is a no-op.
func (s *Service) SetPRTaskResolver(resolver PRTaskResolver) {
	s.prTaskResolver = resolver
}

// SetQuickChatDir sets the directory for quick-chat workspaces.
// When set, task cleanup deletes the session directory under this path for all tasks.
func (s *Service) SetQuickChatDir(dir string) {
	s.quickChatDir = dir
}

// RemoteBranchLister fetches branches from a provider's remote (e.g. GitHub
// API) without needing a local clone. Used by ListBranches so a repo that is
// registered as remote ("Remote" badge in the UI) can serve branches before
// or even without the orchestrator finishing its clone.
type RemoteBranchLister interface {
	ListRepoBranches(ctx context.Context, owner, repo string) ([]Branch, error)
}

// SetRemoteBranchLister wires the remote branch source. Currently only GitHub
// is plumbed; other providers can be added by extending the adapter.
func (s *Service) SetRemoteBranchLister(lister RemoteBranchLister) {
	s.remoteBranchLister = lister
}

// RepoCloneLocation reports the base path the orchestrator clones repos into
// (e.g. ~/.kandev/repos or KANDEV_REPOCLONE_BASEPATH). Listing local branches
// for a cloned repo requires that path to be allow-listed by
// discoveryRoots(); without this hook clones to a custom basepath silently
// fall outside the allow-list and branch listing returns no results.
type RepoCloneLocation interface {
	ExpandedBasePath() (string, error)
}

// SetRepoCloneLocation wires the cloner so its base path is treated as an
// implicit discovery root.
func (s *Service) SetRepoCloneLocation(loc RepoCloneLocation) {
	s.repoCloneLocation = loc
}
