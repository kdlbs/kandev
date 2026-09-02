package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

// WorktreeCleanupIdentityProvider captures immutable checkout identities before
// a durable cleanup snapshot is stored. Implementations that do not provide it
// remain compatible with legacy cleanup wiring, which uses the older live-state
// fallback in the worktree manager.
type WorktreeCleanupIdentityProvider interface {
	CaptureCleanupHeadOIDs(ctx context.Context, worktrees []*worktree.Worktree) (map[string]string, error)
}

// WorktreeBatchCleaner extends WorktreeProvider with batch cleanup.
type WorktreeBatchCleaner interface {
	WorktreeProvider
	// CleanupWorktrees removes multiple worktrees in a single operation.
	CleanupWorktrees(ctx context.Context, worktrees []*worktree.Worktree) error
}

// WorktreeArchiveBatchCleaner removes archived task worktrees without deleting
// the local branches required for later recovery.
type WorktreeArchiveBatchCleaner interface {
	WorktreeBatchCleaner
	CleanupWorktreesPreservingBranches(ctx context.Context, worktrees []*worktree.Worktree) error
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

// synchronousTaskExecutionStopper is an optional cleanup-only extension. The
// normal StopSession contract schedules process teardown asynchronously, but
// destructive resource cleanup must wait until the process exits.
type synchronousTaskExecutionStopper interface {
	StopSessionSynchronously(ctx context.Context, sessionID, reason string, force bool) error
}

// TerminalClarificationCanceller expires durable input requests after a task
// service-owned terminal transition, such as archive cancellation.
type TerminalClarificationCanceller interface {
	ExpireSessionAndNotify(ctx context.Context, sessionID string) (int, error)
}

// TaskLSPLifecycle is the task-owned language-server cleanup/recovery seam.
// Callers provide only a task resolved by the service; session and execution
// identifiers never cross this ownership boundary.
type TaskLSPLifecycle interface {
	CancelTaskOperations(taskID string)
	CleanupTask(ctx context.Context, taskID, reason string) error
	ReconcileTask(ctx context.Context, taskID string) error
	WorkspaceSourcesChanged(ctx context.Context, taskID string) error
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

// AgentComparisonTargetPusher pushes the durable provider-qualified comparison
// target map to every running agentctl execution for a task. Implementations
// must replace the complete projection, including an empty map, so a cleared
// target cannot remain cached in a live workspace.
type AgentComparisonTargetPusher interface {
	PushComparisonTargetsForTask(ctx context.Context, taskID string, targets map[string]models.ComparisonTarget)
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
	ResolveAutoStartStep(ctx context.Context, workflowID string) (string, error)
}

// StepHistoryRecorder persists an ADR 0015 session-step transition audit
// row. Optional — when unset, MoveTaskWithOptions records nothing. Errors
// are logged and swallowed by the caller: the audit trail must never fail
// the move it is recording.
type StepHistoryRecorder interface {
	CreateStepTransition(ctx context.Context, sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actorID *string, metadata map[string]interface{}) error
}

type asyncStepHistoryRecorder interface {
	EnqueueStepTransition(sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actorID *string, metadata map[string]interface{})
}

// ContributionDestinationPreparer is an internal creation-time hook for a
// server-owned publication route. It runs after request/workflow validation
// and external-ID deduplication, but before the task row is inserted.
type ContributionDestinationPreparer interface {
	PrepareContributionDestination(
		ctx context.Context,
		req *CreateTaskRequest,
		workflow *models.Workflow,
		repositories []*models.Repository,
	) error
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
	ErrTaskLSPAdmissionBlocked    = errors.New("task environment lifecycle transition in progress")
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
	RepositorySets    repository.RepositorySetRepository
	BranchPolicies    repository.RepositoryBranchPolicyRepository
	RepositoryCleanup repository.RepositoryCleanupRepository
	Executors         repository.ExecutorRepository
	Environments      repository.EnvironmentRepository
	TaskEnvironments  repository.TaskEnvironmentRepository
	Reviews           repository.ReviewRepository
	ResourceCleanups  repository.TaskResourceCleanupRepository
	StatusSummaries   repository.TaskStatusSummaryRepository
	TaskActivity      repository.TaskActivityRepository
	SubagentContexts  repository.SubagentContextRepository
	Usage             repository.UsageRepository
}

// Service provides task business logic
type Service struct {
	workspaces                      repository.WorkspaceRepository
	tasks                           repository.TaskRepository
	taskRepos                       repository.TaskRepoRepository
	workspaceFolders                repository.TaskWorkspaceFolderRepository
	workflows                       repository.WorkflowRepository
	messages                        repository.MessageRepository
	attachments                     repository.AttachmentRepository
	turns                           repository.TurnRepository
	sessions                        repository.SessionRepository
	gitSnapshots                    repository.GitSnapshotRepository
	repoEntities                    repository.RepositoryEntityRepository
	repositorySets                  repository.RepositorySetRepository
	branchPolicies                  repository.RepositoryBranchPolicyRepository
	repositoryCleanup               repository.RepositoryCleanupRepository
	executors                       repository.ExecutorRepository
	environments                    repository.EnvironmentRepository
	taskEnvironments                repository.TaskEnvironmentRepository
	reviews                         repository.ReviewRepository
	resourceCleanups                repository.TaskResourceCleanupRepository
	statusSummaries                 repository.TaskStatusSummaryRepository
	taskActivity                    repository.TaskActivityRepository
	subagentContexts                repository.SubagentContextRepository
	usage                           repository.UsageRepository
	attachmentSvc                   *AttachmentService
	statusSummaryPRs                TaskStatusSummaryPRReader
	statusSummaryProjector          TaskStatusSummaryEventProjector
	queuedPromptCounter             QueuedPromptCounter
	eventBus                        bus.EventBus
	logger                          *logger.Logger
	discoveryConfig                 RepositoryDiscoveryConfig
	worktreeCleanup                 WorktreeCleanup
	executionStopper                TaskExecutionStopper
	taskLSP                         TaskLSPLifecycle
	taskEnvironmentResetGuard       TaskEnvironmentResetGuard
	clarificationCanceller          TerminalClarificationCanceller
	rowLivenessProber               TaskRowLivenessProber
	contextWindowResetter           func(context.Context, string) error
	cleanupActivity                 TaskResourceCleanupActivityGate
	branchMaterializer              BranchMaterializer
	workspaceSourceMaterializer     WorkspaceSourceMaterializer
	workspaceSourceLocksMu          sync.Mutex
	workspaceSourceLocks            map[string]*sync.Mutex
	taskLSPAdmissionMu              sync.Mutex
	taskLSPAdmissions               map[string]*taskLSPAdmissionGate
	taskEnvLSPAdmissionMu           sync.Mutex
	taskEnvLSPAdmissions            map[string]*sync.RWMutex
	workspaceTaskAdmissionMu        sync.Mutex
	workspaceTaskAdmissions         map[string]*sync.RWMutex
	providerProber                  ProviderDefaultBranchProber
	gitArchiveCapture               GitArchiveCapture
	workflowStepCreator             WorkflowStepCreator
	workspaceBootstrapper           WorkspaceBootstrapper
	workflowStepGetter              WorkflowStepGetter
	startStepResolver               StartStepResolver
	stepHistoryRecorder             StepHistoryRecorder
	contributionDestinationPreparer ContributionDestinationPreparer
	prTaskResolver                  PRTaskResolver
	quickChatDir                    string // Directory for quick-chat workspaces (e.g., ~/.kandev/quick-chat)
	branchFetcher                   *branchFetcher
	envDestroyer                    EnvironmentDestroyer
	runtimeSecretDeleter            TaskEnvironmentRuntimeSecretDeleter
	sshTaskDirReclaimer             SSHTaskDirReclaimer
	sessionRunningChecker           SessionRunningChecker
	remoteBranchLister              RemoteBranchLister
	repositorySelectionResolver     RepositorySelectionResolver
	repoCloneLocation               RepoCloneLocation
	blockers                        BlockerRepository
	// dependencyEdgeMu serializes validate-then-insert for dependency edges so
	// two concurrent adds cannot each pass a cycle walk that predates the
	// other's insert and commit a cycle between them.
	dependencyEdgeMu       sync.Mutex
	comments               CommentRepository
	taskStateActivity      TaskStateActivityLogger
	secretStore            secrets.SecretStore
	workspaceSecretDeleter WorkspaceSecretDeleter
	baseBranchPusher       AgentBaseBranchPusher
	comparisonTargetPusher AgentComparisonTargetPusher
	runtimeOverridesMu     sync.Mutex

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
	cleanupPrepMu       sync.Mutex
	cleanupPreparations map[string]*taskResourceCleanupPreparationLease
	cleanupPrepWG       sync.WaitGroup
	cleanupPrepClosed   bool
	// repoResolveMu serializes the check-then-create sections of
	// FindOrCreateRepository and FindOrCreateRepositoryByLocalPath so two
	// resolvers racing to register the same not-yet-known repository (by
	// provider identity or by canonical local_path) converge on a single row
	// instead of each inserting a duplicate. Covers concurrent requests
	// within this backend process only — this backend is single-process per
	// SQLite database, so that is the complete threat model today.
	repoResolveMu sync.Mutex
	// pendingActionProjectionMu guards the durable-generation logical clock
	// shared by REST snapshots and semantic message events. Revisions are
	// reserved before each repository read so delayed results stay ordered.
	pendingActionProjectionMu       sync.Mutex
	pendingActionProjectionEpoch    string
	pendingActionProjectionSequence uint64
}

// AcquireTaskLSPAdmission holds shared task admission while a language-server
// launch probes or acquires runtime resources. Environment reset and terminal
// task mutations hold the exclusive side through LSP cleanup and durable row
// mutation, so a queued start cannot recreate resources behind teardown.
func (s *Service) AcquireTaskLSPAdmission(
	ctx context.Context,
	taskID string,
) (func(), error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	gate := s.taskLSPAdmissionLock(taskID)
	if !gate.TryRLock() {
		return nil, ErrTaskLSPAdmissionBlocked
	}
	environment, err := s.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil {
		gate.RUnlock()
		return nil, err
	}
	var environmentLock *sync.RWMutex
	if environment != nil && environment.ID != "" {
		environmentLock = s.taskLSPEnvironmentAdmissionLock(environment.ID)
		if !environmentLock.TryRLock() {
			gate.RUnlock()
			return nil, ErrTaskLSPAdmissionBlocked
		}
	}
	if err := context.Cause(ctx); err != nil {
		if environmentLock != nil {
			environmentLock.RUnlock()
		}
		gate.RUnlock()
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			if environmentLock != nil {
				environmentLock.RUnlock()
			}
			gate.RUnlock()
		})
	}, nil
}

func (s *Service) taskLSPAdmissionLock(taskID string) *taskLSPAdmissionGate {
	s.taskLSPAdmissionMu.Lock()
	defer s.taskLSPAdmissionMu.Unlock()
	if s.taskLSPAdmissions == nil {
		s.taskLSPAdmissions = make(map[string]*taskLSPAdmissionGate)
	}
	lock := s.taskLSPAdmissions[taskID]
	if lock == nil {
		lock = &taskLSPAdmissionGate{}
		s.taskLSPAdmissions[taskID] = lock
	}
	return lock
}

func (s *Service) acquireTaskLSPMutation(taskID string) func() {
	gate := s.taskLSPAdmissionLock(taskID)
	return gate.Lock(func() {
		if s.taskLSP != nil {
			s.taskLSP.CancelTaskOperations(taskID)
		}
	})
}

// taskLSPAdmissionGate publishes writer intent before waiting for current
// readers. That closes the gap where a terminal mutation could cancel active
// LSP work yet lose a race to one new admission before the writer blocked it.
type taskLSPAdmissionGate struct {
	stateMu sync.Mutex
	rw      sync.RWMutex
	writers int
}

func (g *taskLSPAdmissionGate) TryRLock() bool {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()
	if g.writers != 0 {
		return false
	}
	g.rw.RLock()
	return true
}

func (g *taskLSPAdmissionGate) RUnlock() {
	g.rw.RUnlock()
}

func (g *taskLSPAdmissionGate) Lock(interrupt func()) func() {
	g.stateMu.Lock()
	g.writers++
	g.stateMu.Unlock()
	if interrupt != nil {
		interrupt()
	}
	g.rw.Lock()
	return func() {
		g.rw.Unlock()
		g.stateMu.Lock()
		g.writers--
		g.stateMu.Unlock()
	}
}

func (s *Service) acquireTaskLSPMutations(taskIDs []string) func() {
	ordered := append([]string(nil), taskIDs...)
	sort.Strings(ordered)
	releases := make([]func(), 0, len(ordered))
	previous := ""
	for _, taskID := range ordered {
		if taskID == "" || taskID == previous {
			continue
		}
		previous = taskID
		releases = append(releases, s.acquireTaskLSPMutation(taskID))
	}
	return func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}
}

func (s *Service) workspaceTaskAdmissionLock(workspaceID string) *sync.RWMutex {
	s.workspaceTaskAdmissionMu.Lock()
	defer s.workspaceTaskAdmissionMu.Unlock()
	if s.workspaceTaskAdmissions == nil {
		s.workspaceTaskAdmissions = make(map[string]*sync.RWMutex)
	}
	lock := s.workspaceTaskAdmissions[workspaceID]
	if lock == nil {
		lock = &sync.RWMutex{}
		s.workspaceTaskAdmissions[workspaceID] = lock
	}
	return lock
}

func (s *Service) acquireWorkspaceTaskCreation(workspaceID string) func() {
	if workspaceID == "" {
		return func() {}
	}
	lock := s.workspaceTaskAdmissionLock(workspaceID)
	lock.RLock()
	return lock.RUnlock
}

func (s *Service) acquireWorkspaceTaskDeletion(workspaceID string) func() {
	if workspaceID == "" {
		return func() {}
	}
	lock := s.workspaceTaskAdmissionLock(workspaceID)
	lock.Lock()
	return lock.Unlock
}

func (s *Service) taskLSPEnvironmentAdmissionLock(environmentID string) *sync.RWMutex {
	s.taskEnvLSPAdmissionMu.Lock()
	defer s.taskEnvLSPAdmissionMu.Unlock()
	if s.taskEnvLSPAdmissions == nil {
		s.taskEnvLSPAdmissions = make(map[string]*sync.RWMutex)
	}
	lock := s.taskEnvLSPAdmissions[environmentID]
	if lock == nil {
		lock = &sync.RWMutex{}
		s.taskEnvLSPAdmissions[environmentID] = lock
	}
	return lock
}

func (s *Service) acquireTaskLSPEnvironmentMutation(environmentID string) func() {
	lock := s.taskLSPEnvironmentAdmissionLock(environmentID)
	lock.Lock()
	return lock.Unlock
}

func (s *Service) acquireTaskLSPEnvironmentMutationForTask(
	ctx context.Context,
	taskID string,
) (func(), error) {
	environment, err := s.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if environment == nil || environment.ID == "" {
		return func() {}, nil
	}
	return s.acquireTaskLSPEnvironmentMutation(environment.ID), nil
}

func (s *Service) acquireTaskLSPEnvironmentMutations(
	ctx context.Context,
	taskIDs []string,
) (func(), error) {
	environmentIDs := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		environment, err := s.GetTaskEnvironmentForTaskLSP(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("resolve task %s physical environment: %w", taskID, err)
		}
		if environment != nil && environment.ID != "" {
			environmentIDs = append(environmentIDs, environment.ID)
		}
	}
	return s.acquireTaskLSPEnvironmentMutationIDs(environmentIDs), nil
}

func (s *Service) acquireTaskLSPEnvironmentMutationIDs(environmentIDs []string) func() {
	unique := make(map[string]struct{}, len(environmentIDs))
	for _, environmentID := range environmentIDs {
		if environmentID != "" {
			unique[environmentID] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(unique))
	for environmentID := range unique {
		ordered = append(ordered, environmentID)
	}
	sort.Strings(ordered)
	releases := make([]func(), 0, len(ordered))
	for _, environmentID := range ordered {
		releases = append(releases, s.acquireTaskLSPEnvironmentMutation(environmentID))
	}
	return func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}
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
		repositorySets:        repos.RepositorySets,
		branchPolicies:        repos.BranchPolicies,
		repositoryCleanup:     repos.RepositoryCleanup,
		executors:             repos.Executors,
		environments:          repos.Environments,
		taskEnvironments:      repos.TaskEnvironments,
		reviews:               repos.Reviews,
		resourceCleanups:      repos.ResourceCleanups,
		statusSummaries:       repos.StatusSummaries,
		taskActivity:          repos.TaskActivity,
		subagentContexts:      repos.SubagentContexts,
		usage:                 repos.Usage,
		eventBus:              eventBus,
		logger:                log,
		discoveryConfig:       discoveryConfig,
		branchFetcher:         newBranchFetcher(log.Zap()),
		lastTaskActivity:      make(map[string]v1.ForegroundActivity),
		lastTaskSubagentCount: make(map[string]int),
		// Focused service tests do not run backend composition. Production
		// replaces this fallback with a database-allocated generation.
		pendingActionProjectionEpoch: "1",
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

// SetAgentComparisonTargetPusher wires the live-update push for provider PR
// and MR reconciliation. Optional: when unset, the persisted attachment
// metadata remains authoritative and is hydrated at the next launch.
func (s *Service) SetAgentComparisonTargetPusher(p AgentComparisonTargetPusher) {
	s.comparisonTargetPusher = p
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

// SetClarificationCanceller wires terminal clarification cleanup for session
// transitions owned by the task service.
func (s *Service) SetClarificationCanceller(canceller TerminalClarificationCanceller) {
	s.clarificationCanceller = canceller
}

// SetTaskLSPLifecycle wires the task-owned language-server controller.
func (s *Service) SetTaskLSPLifecycle(lifecycle TaskLSPLifecycle) {
	s.taskLSP = lifecycle
}

// TaskEnvironmentResetGuard protects physical environments referenced by a
// wider workspace ownership boundary, such as an inherited workspace group.
type TaskEnvironmentResetGuard interface {
	ValidateTaskEnvironmentReset(ctx context.Context, taskID, environmentID string) error
}

// SetTaskEnvironmentResetGuard wires the cross-task workspace owner used by
// ResetTaskEnvironment before any runtime resource is touched.
func (s *Service) SetTaskEnvironmentResetGuard(guard TaskEnvironmentResetGuard) {
	s.taskEnvironmentResetGuard = guard
}

// CleanupTaskLSP exposes the task-owned cleanup hook to cascade composition.
// It accepts only a task ID and semantic reason; runtime/session identifiers
// remain private to the LSP controller.
func (s *Service) CleanupTaskLSP(ctx context.Context, taskID, reason string) error {
	if s.taskLSP == nil {
		return nil
	}
	return s.taskLSP.CleanupTask(ctx, taskID, reason)
}

// StopTaskLSP durably suspends task-owned language servers before task stop
// returns. The environment transition and cleanup share one exclusive
// admission window, so a queued Start cannot recreate a task host after the
// cleanup boundary. A later agent launch marks the environment ready and the
// environment-ready callback reconciles the preserved per-language policy.
func (s *Service) StopTaskLSP(ctx context.Context, taskID, reason string) error {
	releaseMutation := s.acquireTaskLSPMutation(taskID)
	defer releaseMutation()

	environment, err := s.GetTaskEnvironmentForTaskLSP(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task environment before LSP stop: %w", err)
	}
	releaseEnvironmentMutation := func() {}
	if environment != nil && environment.ID != "" {
		releaseEnvironmentMutation = s.acquireTaskLSPEnvironmentMutation(environment.ID)
	}
	defer releaseEnvironmentMutation()
	ownershipTransfer, err := s.prepareTaskEnvironmentForLSPStop(ctx, taskID, environment)
	if err != nil {
		return err
	}
	if s.taskLSP == nil {
		return nil
	}
	if err := s.taskLSP.CleanupTask(ctx, taskID, reason); err != nil {
		return s.rollbackTaskEnvironmentOwnershipAfterFailure(ctx, ownershipTransfer, err)
	}
	return nil
}

func (s *Service) prepareTaskEnvironmentForLSPStop(
	ctx context.Context,
	taskID string,
	environment *models.TaskEnvironment,
) (*workspaceEnvironmentOwnershipTransfer, error) {
	if environment == nil {
		return nil, nil
	}
	current, err := s.taskEnvironments.GetTaskEnvironment(ctx, environment.ID)
	if err != nil {
		return nil, fmt.Errorf("reload task environment before LSP stop: %w", err)
	}
	if current == nil || current.TaskID != taskID {
		return nil, nil
	}
	shared, err := s.hasOtherLiveTasksForEnvironment(ctx, taskID, current)
	if err != nil {
		return nil, fmt.Errorf("check shared task environment before LSP stop: %w", err)
	}
	if shared {
		return s.preserveTaskEnvironmentForLiveBorrower(ctx, taskID, current)
	}
	if current.Status == models.TaskEnvironmentStatusStopped {
		return nil, nil
	}
	current.Status = models.TaskEnvironmentStatusStopped
	if err := s.taskEnvironments.UpdateTaskEnvironment(ctx, current); err != nil {
		return nil, fmt.Errorf("mark task environment stopped before LSP cleanup: %w", err)
	}
	return nil, nil
}

// ReconcileTaskLSP is used by task resume/cascade composition without
// exposing the controller or a runtime execution identifier.
func (s *Service) ReconcileTaskLSP(ctx context.Context, taskID string) error {
	if s.taskLSP == nil {
		return nil
	}
	return s.taskLSP.ReconcileTask(ctx, taskID)
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

// SetStepHistoryRecorder wires the ADR 0015 audit-trail writer for manual
// step transitions (MoveTaskWithOptions). Optional.
func (s *Service) SetStepHistoryRecorder(recorder StepHistoryRecorder) {
	s.stepHistoryRecorder = recorder
}

// SetContributionDestinationPreparer wires the optional server-side
// publication-route preparation used by managed Improve Kandev tasks.
func (s *Service) SetContributionDestinationPreparer(preparer ContributionDestinationPreparer) {
	s.contributionDestinationPreparer = preparer
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

// RemoteBranchSource is the host-verified identity of a provider-backed
// workspace repository. Callers must derive it from persisted repository data.
type RemoteBranchSource struct {
	WorkspaceID          string
	Provider             string
	ProviderHost         string
	ProviderScope        string
	ProviderRepositoryID string
	Owner                string
	Name                 string
	RemoteURL            string
	DefaultBranch        string
}

// RemoteBranchLister fetches branches from a provider's remote API without
// needing a local clone. Used by ListBranches so a repo that is
// registered as remote ("Remote" badge in the UI) can serve branches before
// or even without the orchestrator finishing its clone.
type RemoteBranchLister interface {
	ListRepoBranches(ctx context.Context, source RemoteBranchSource) ([]Branch, error)
}

// SetRemoteBranchLister wires the provider-neutral remote branch source.
func (s *Service) SetRemoteBranchLister(lister RemoteBranchLister) {
	s.remoteBranchLister = lister
}

// SetRepositorySelectionResolver wires server-side inspection for first-use
// plugin repository selections. The resolver is optional for focused callers;
// plugin selections fail closed when it is not wired.
func (s *Service) SetRepositorySelectionResolver(resolver RepositorySelectionResolver) {
	s.repositorySelectionResolver = resolver
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
