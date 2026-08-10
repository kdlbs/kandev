package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// WorktreeCleanup provides worktree cleanup on task deletion.
type WorktreeCleanup interface {
	// OnTaskDeleted is called when a task is deleted to clean up its worktree.
	OnTaskDeleted(ctx context.Context, taskID string) error
}

// WorkspaceSecretDeleter removes secrets owned by a workspace. It is optional
// for isolated task-service users.
type WorkspaceSecretDeleter interface {
	DeleteWorkspaceSecrets(ctx context.Context, workspaceID string) error
}

type transactionalWorkspaceCascade interface {
	DeleteWorkspaceCascadeWithSecretCleanup(
		ctx context.Context,
		id string,
		cleanup func(context.Context, *sqlx.Tx) error,
	) ([]*models.Task, []*models.Workflow, error)
	DeleteWorkspaceCascadeWithNameAndSecretCleanup(
		ctx context.Context,
		id, name string,
		cleanup func(context.Context, *sqlx.Tx) error,
	) ([]*models.Task, []*models.Workflow, error)
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

type worktreeReferenceGuard interface {
	CountActiveWorktreeReferences(ctx context.Context, worktreeID string, excludeSessionIDs []string) (int, error)
	ReleaseWorktreeReference(ctx context.Context, wt *worktree.Worktree) error
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

// TaskRowLivenessProber classifies an executors_running row's backing-process
// liveness in a runtime-aware way (a local process check is never applied to a
// remote/SSH row). It is optional and satisfied by the lifecycle adapter. When
// unwired, cleanup treats every row as Unknown so a not-found stop is never
// mistaken for an absent runtime.
type TaskRowLivenessProber interface {
	RowLiveness(row *models.ExecutorRunning) models.ProcessLiveness
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

// BranchMaterializer creates a worktree on disk and persists the
// task_environment_repos row for a newly added task_repository row, without
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
	MaterializeBranch(ctx context.Context, taskID, taskRepositoryID string) (*BranchMaterializationResult, error)
}

// BranchMaterializationResult describes the live worktree created for a
// branch attachment. A nil result with a nil error means materialization was
// intentionally deferred until the next session launch; callers must check
// for a nil result rather than empty path fields.
type BranchMaterializationResult struct {
	WorktreePath      string
	TaskWorkspacePath string
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

// WorkspaceBootstrapper owns the atomic persistence of a standard Kanban
// workspace and its initial workflow state.
type WorkspaceBootstrapper interface {
	CreateWorkspaceWithKanban(ctx context.Context, workspace *models.Workspace) (*models.Workflow, error)
}

// WorkspaceDefaultsInitializer persists integration defaults after a
// workspace row exists and before its creation event is published.
type WorkspaceDefaultsInitializer interface {
	InitializeWorkspaceDefaults(ctx context.Context, workspaceID string) error
}

// WorkflowStepGetter retrieves workflow step information.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
	// GetNextStepByPosition returns the next step after the given position for a workflow.
	// Returns nil if there is no next step (i.e., current step is the last one).
	GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error)
}

// workflowStepLister is an optional extension used to find WIP steps that
// pull work from a feeder when new work arrives in that feeder.
type workflowStepLister interface {
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error)
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
	ErrWIPLimitExceeded          = wfmodels.ErrWIPLimitExceeded
	ErrInvalidRepositorySettings = errors.New("invalid repository settings")
	ErrInvalidExecutorConfig     = errors.New("invalid executor config")
	// Workspace-source sentinels are the service boundary consumed by the HTTP
	// and MCP adapters. Keep categories stable rather than making callers parse
	// a validation or runtime error string.
	ErrInvalidWorkspaceSource     = errors.New("invalid workspace source")
	ErrWorkspaceSourceConflict    = errors.New("workspace source conflict")
	ErrWorkspaceSourceActive      = errors.New("workspace source task is active")
	ErrUnsupportedWorkspaceSource = errors.New("unsupported workspace source")
	ErrWorkspaceSourceMaterialize = errors.New("workspace source materialization failed")
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
	Workspaces        repository.WorkspaceRepository
	Tasks             repository.TaskRepository
	TaskRepos         repository.TaskRepoRepository
	WorkspaceFolders  repository.TaskWorkspaceFolderRepository
	Workflows         repository.WorkflowRepository
	Messages          repository.MessageRepository
	Attachments       repository.AttachmentRepository
	Turns             repository.TurnRepository
	Sessions          repository.SessionRepository
	GitSnapshots      repository.GitSnapshotRepository
	RepoEntities      repository.RepositoryEntityRepository
	RepositoryCleanup repository.RepositoryCleanupRepository
	Executors         repository.ExecutorRepository
	Environments      repository.EnvironmentRepository
	TaskEnvironments  repository.TaskEnvironmentRepository
	Reviews           repository.ReviewRepository
	ResourceCleanups  repository.TaskResourceCleanupRepository
	StatusSummaries   repository.TaskStatusSummaryRepository
}

// Service provides task business logic
type Service struct {
	workspaces                  repository.WorkspaceRepository
	tasks                       repository.TaskRepository
	taskRepos                   repository.TaskRepoRepository
	workspaceFolders            repository.TaskWorkspaceFolderRepository
	workflows                   repository.WorkflowRepository
	messages                    repository.MessageRepository
	attachments                 repository.AttachmentRepository
	turns                       repository.TurnRepository
	sessions                    repository.SessionRepository
	gitSnapshots                repository.GitSnapshotRepository
	repoEntities                repository.RepositoryEntityRepository
	repositoryCleanup           repository.RepositoryCleanupRepository
	executors                   repository.ExecutorRepository
	environments                repository.EnvironmentRepository
	taskEnvironments            repository.TaskEnvironmentRepository
	reviews                     repository.ReviewRepository
	resourceCleanups            repository.TaskResourceCleanupRepository
	statusSummaries             repository.TaskStatusSummaryRepository
	attachmentSvc               *AttachmentService
	statusSummaryPRs            TaskStatusSummaryPRReader
	queuedPromptCounter         QueuedPromptCounter
	eventBus                    bus.EventBus
	logger                      *logger.Logger
	discoveryConfig             RepositoryDiscoveryConfig
	worktreeCleanup             WorktreeCleanup
	executionStopper            TaskExecutionStopper
	rowLivenessProber           TaskRowLivenessProber
	contextWindowResetter       func(context.Context, string) error
	cleanupActivity             TaskResourceCleanupActivityGate
	branchMaterializer          BranchMaterializer
	workspaceSourceMaterializer WorkspaceSourceMaterializer
	workspaceSourceLocksMu      sync.Mutex
	workspaceSourceLocks        map[string]*sync.Mutex
	providerProber              ProviderDefaultBranchProber
	gitArchiveCapture           GitArchiveCapture
	workflowStepCreator         WorkflowStepCreator
	workspaceBootstrapper       WorkspaceBootstrapper
	workflowStepGetter          WorkflowStepGetter
	startStepResolver           StartStepResolver
	prTaskResolver              PRTaskResolver
	quickChatDir                string // Directory for quick-chat workspaces (e.g., ~/.kandev/quick-chat)
	branchFetcher               *branchFetcher
	envDestroyer                EnvironmentDestroyer
	sessionRunningChecker       SessionRunningChecker
	remoteBranchLister          RemoteBranchLister
	repoCloneLocation           RepoCloneLocation
	blockers                    BlockerRepository
	comments                    CommentRepository
	secretStore                 secrets.SecretStore
	workspaceSecretDeleter      WorkspaceSecretDeleter
	baseBranchPusher            AgentBaseBranchPusher
	runtimeOverridesMu          sync.Mutex

	workspaceSourceProviderRefresher WorkspaceSourceProviderRefresher

	workspaceDefaultsInitializer WorkspaceDefaultsInitializer
	// foregroundActivity resolves the live fine-grained busy substate of a RUNNING
	// session (satisfied by the orchestrator). Used to compute the task-level
	// MOST-ACTIVE-WINS activity aggregate carried on task.updated events. Optional.
	foregroundActivity ForegroundActivityProvider
	// taskActivityMu guards lastTaskActivity, the last task-level activity aggregate
	// emitted per task. It bounds live-propagation task.updated emissions to an
	// actual change of the aggregated three-state value.
	taskActivityMu        sync.Mutex
	lastTaskActivity      map[string]v1.ForegroundActivity
	lastTaskSubagentCount map[string]int
	// taskPublicationMu guards the per-task FIFO dispatchers. It is held only
	// while enqueueing/dequeueing; repository reads and synchronous EventBus
	// delivery always happen after it is released.
	taskPublicationMu sync.Mutex
	taskPublications  map[string]*taskPublicationQueue
	// cleanupDoneForTest lets unit tests wait for async cleanup; nil in production.
	cleanupDoneForTest  chan struct{}
	cleanupWorkerMu     sync.Mutex
	cleanupWorkerCancel context.CancelFunc
	cleanupWorkerWG     sync.WaitGroup
	cleanupWorkerWake   chan struct{}
	cleanupRunsMu       sync.Mutex
	cleanupRuns         map[*taskResourceCleanupRun]struct{}
	// repoResolveMu serializes the check-then-create sections of
	// FindOrCreateRepository and FindOrCreateRepositoryByLocalPath so two
	// resolvers racing to register the same not-yet-known repository (by
	// provider identity or by canonical local_path) converge on a single row
	// instead of each inserting a duplicate. Covers concurrent requests
	// within this backend process only — this backend is single-process per
	// SQLite database, so that is the complete threat model today.
	repoResolveMu sync.Mutex
}

// SetAttachmentService wires the file-backed prompt attachment owner into the
// task service. It is optional for focused unit-test harnesses that never send
// file-backed descriptors.
func (s *Service) SetAttachmentService(attachments *AttachmentService) {
	s.attachmentSvc = attachments
}

// AttachmentService returns the optional file-backed attachment owner wired
// into this task service. It lets route and maintenance composition reuse the
// same storage boundary instead of creating competing service instances.
func (s *Service) AttachmentService() *AttachmentService {
	return s.attachmentSvc
}

// AttachmentRepository returns the attachment registry repository used by the
// task service. It is exposed for composition of the storage maintenance hook.
func (s *Service) AttachmentRepository() repository.AttachmentRepository {
	return s.attachments
}

// SetSecretStore wires metadata-only validation for shared executor profiles.
// Workspace-scoped secret references are rejected before a profile is saved.
func (s *Service) SetSecretStore(secretStore secrets.SecretStore) {
	s.secretStore = secretStore
}

// SetWorkspaceSecretDeleter wires workspace-secret cleanup to workspace
// deletion. The callback runs only after the repository cascade succeeds.
func (s *Service) SetWorkspaceSecretDeleter(deleter WorkspaceSecretDeleter) {
	s.workspaceSecretDeleter = deleter
}

// NewService creates a new task service
func NewService(repos Repos, eventBus bus.EventBus, log *logger.Logger, discoveryConfig RepositoryDiscoveryConfig) *Service {
	return &Service{
		workspaces:            repos.Workspaces,
		tasks:                 repos.Tasks,
		taskRepos:             repos.TaskRepos,
		workspaceFolders:      repos.WorkspaceFolders,
		workflows:             repos.Workflows,
		messages:              repos.Messages,
		attachments:           repos.Attachments,
		turns:                 repos.Turns,
		sessions:              repos.Sessions,
		gitSnapshots:          repos.GitSnapshots,
		repoEntities:          repos.RepoEntities,
		repositoryCleanup:     repos.RepositoryCleanup,
		executors:             repos.Executors,
		environments:          repos.Environments,
		taskEnvironments:      repos.TaskEnvironments,
		reviews:               repos.Reviews,
		resourceCleanups:      repos.ResourceCleanups,
		statusSummaries:       repos.StatusSummaries,
		eventBus:              eventBus,
		logger:                log,
		discoveryConfig:       discoveryConfig,
		branchFetcher:         newBranchFetcher(log.Zap()),
		lastTaskActivity:      make(map[string]v1.ForegroundActivity),
		lastTaskSubagentCount: make(map[string]int),
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

func (s *Service) SetWorkspaceSourceMaterializer(m WorkspaceSourceMaterializer) {
	s.workspaceSourceMaterializer = m
}

// SetWorkspaceSourceProviderRefresher wires the best-effort live MCP provider
// reconciliation used after workspace-source and legacy branch attachments.
func (s *Service) SetWorkspaceSourceProviderRefresher(r WorkspaceSourceProviderRefresher) {
	s.workspaceSourceProviderRefresher = r
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

// SetRowLivenessProber wires the runtime-aware executors_running liveness probe
// (satisfied by the lifecycle adapter). It is optional; when unwired, cleanup
// treats every row as Unknown.
func (s *Service) SetRowLivenessProber(prober TaskRowLivenessProber) {
	s.rowLivenessProber = prober
}

// SetContextWindowResetter wires the guarded context-window reset callback
// owned by the orchestrator. It is optional for isolated task-service users;
// those callers fall back to clearing the session metadata directly.
func (s *Service) SetContextWindowResetter(resetter func(context.Context, string) error) {
	s.contextWindowResetter = resetter
}

func (s *Service) resetContextWindow(ctx context.Context, sessionID string) error {
	if s.contextWindowResetter != nil {
		return s.contextWindowResetter(ctx, sessionID)
	}
	return s.sessions.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyContextWindow, nil)
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

func (s *Service) SetWorkspaceBootstrapper(bootstrapper WorkspaceBootstrapper) {
	s.workspaceBootstrapper = bootstrapper
}

// SetWorkspaceDefaultsInitializer wires the integration initializer used by
// workspace creation. It is optional so the task service remains usable in
// deployments without GitHub.
func (s *Service) SetWorkspaceDefaultsInitializer(initializer WorkspaceDefaultsInitializer) {
	s.workspaceDefaultsInitializer = initializer
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
	ListRepoBranches(ctx context.Context, workspaceID, owner, repo string) ([]Branch, error)
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
