//revive:disable:file-length-limit // Legacy task operations remain cohesive; workspace-source additions live in focused files.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
)

// defaultPriority is the default value for the task priority column.
// Used when a caller omits priority so the DB CHECK constraint is satisfied.
const defaultPriority = "medium"

const (
	providerAzureDevOps = "azure_devops"
	providerGitHub      = "github"
	providerGitLab      = "gitlab"
	protocolHTTP        = "http"
	protocolHTTPS       = "https"
)

const defaultKandevTaskWorktreePathSegment = "/.kandev/tasks/"

const (
	titleNotPendingReason = "title_not_pending"
	titleNotOwnerReason   = "title_not_owner"
)

// ErrSubtaskDepthExceeded is returned when a caller tries to create a
// subtask of a kanban subtask (nesting depth > 1). Office task trees are
// intentionally exempt.
var ErrSubtaskDepthExceeded = fmt.Errorf("cannot create a subtask of a subtask — maximum nesting depth is 1 for kanban tasks. Create a sibling task under the same parent or a top-level task instead")

// ErrInvalidTaskWorkflow identifies task creation requests whose explicit
// workflow or workflow step relationship is inconsistent.
var ErrInvalidTaskWorkflow = errors.New("invalid task workflow")

// ErrTaskAlreadyArchived is returned by ArchiveTask when the target task
// already has archived_at set. Sentinel so cascade callers (e.g.
// DeleteWorkflow) can treat a concurrent archive as a no-op instead of
// aborting the whole operation.
var ErrTaskAlreadyArchived = errors.New("task is already archived")

// ErrAutoTitlePromptRequired is returned when auto-title creation has neither
// a prompt nor a usable provisional title.
var ErrAutoTitlePromptRequired = errors.New("description or title is required when auto_title is enabled")

// ErrAutoTitleUnsupportedForOffice is returned when auto-title generation is
// requested for an Office task. Office agents use a restricted MCP surface
// that does not expose the one-shot title tool.
var ErrAutoTitleUnsupportedForOffice = errors.New("auto_title is not supported for Office tasks")

type pendingTaskTitleSetter interface {
	SetTaskTitleIfPending(ctx context.Context, taskID, sessionID, title string) (bool, error)
}

type taskStopTarget struct {
	sessionID   string
	executionID string
	// terminal records the session state observed when the cleanup snapshot was
	// taken. It is retained for snapshot compatibility, but it must never
	// suppress a non-not-found runtime stop failure.
	terminal bool
}

type taskEnvironmentCleanup struct {
	env              *models.TaskEnvironment
	deleteRow        bool
	preserveBranches bool
}

type taskEnvironmentSessionUsageChecker interface {
	HasActiveTaskSessionsByTaskEnvironmentExcludingTask(ctx context.Context, taskEnvironmentID, taskID string) (bool, error)
}

type taskEnvironmentSessionBorrowerFinder interface {
	FindActiveTaskSessionTaskIDByTaskEnvironmentExcludingTask(ctx context.Context, taskEnvironmentID, taskID string) (string, error)
}

type taskEnvironmentOwnerTransferer interface {
	TransferTaskEnvironmentToTask(ctx context.Context, envID, taskID string) error
}

type workflowStepCapacityTaskCreator interface {
	CreateTaskIfWorkflowStepHasCapacity(ctx context.Context, task *models.Task, targetStepID string, limit int) error
}

type workflowStepAdmissionTaskCreator interface {
	CreateTaskWithWorkflowStepAdmission(ctx context.Context, task *models.Task, targetStepID string, targetLimit int, feederStepID string, feederLimit int) error
}

// Task operations

// isOfficeRequest returns true if the request should create an office task.
func isOfficeRequest(req *CreateTaskRequest) bool {
	return req.ProjectID != "" ||
		req.Origin == models.TaskOriginAgentCreated ||
		req.Origin == models.TaskOriginRoutine ||
		req.Origin == models.TaskOriginOnboarding
}

// CreateTaskOutcome distinguishes why Service.CreateTask returned the task it
// did, per the create-idempotency contract
// (docs/specs/tasks/requirements/external-id-idempotency.md). Only meaningful when
// the request carried an external_id; a request without one always reports
// CreateTaskOutcomeCreated. The fourth contract outcome, CreatedIdentityLost,
// is not produced here — it is decided by the handler during settlement,
// after this call returns (see the spec's "Settlement" section).
type CreateTaskOutcome int

const (
	// CreateTaskOutcomeCreated means no task held the identity (or the
	// request carried none): this call created the returned task.
	CreateTaskOutcomeCreated CreateTaskOutcome = iota
	// CreateTaskOutcomeFoundSettled means a task already held the identity
	// and its creation had finished. No side effects occurred; the caller
	// MUST skip all post-create work.
	CreateTaskOutcomeFoundSettled
	// CreateTaskOutcomeFoundUnsettled means a task already held the identity
	// but its creation had not finished at observation time — it may still
	// be running. No side effects occurred; the caller MUST skip all
	// post-create work and MUST NOT release the identity and create again.
	CreateTaskOutcomeFoundUnsettled
)

// CreateTaskResult is Service.CreateTask's return value.
type CreateTaskResult struct {
	Task    *models.Task
	Outcome CreateTaskOutcome
}

// foundOutcomeFor classifies an existing task holding an external_id as
// settled or unsettled, per the state machine in the spec's "State machine"
// section: external_id_settled_at IS NULL means the claiming create had not
// finished its required synchronous work at observation time.
func foundOutcomeFor(task *models.Task) CreateTaskOutcome {
	if task.ExternalIDSettledAt != nil {
		return CreateTaskOutcomeFoundSettled
	}
	return CreateTaskOutcomeFoundUnsettled
}

// CreateTask creates a new task and publishes a task.created event, or —
// when the request carries an external_id already held by a task — returns
// that task instead, per the create-idempotency contract
// (docs/specs/tasks/requirements/external-id-idempotency.md). WorkflowID is required
// for non-ephemeral kanban tasks. Office tasks (project_id set, or origin is
// agent_created/routine) auto-resolve to the workspace's office workflow.
// Ephemeral tasks (quick chat, config chat) must NOT have a workflow.
//
// The create sequence below is normative — step 3 (the identity lookup) MUST
// precede identifier allocation, WIP admission, and every write, or a dedupe
// hit burns a task_sequence number or fails with a capacity error instead of
// returning the existing task:
//
//  1. authorize workspace
//  2. validate + normalize external_id
//  3. lookup by (workspace_id, external_id) — found → return Found, stop
//  4. required create-time validation
//  5. identifier allocation, WIP admission, task-row insert
//     — unique violation, or any other pre-insert failure, re-reads by
//     external_id before surfacing; a hit still returns Found
//  6. required synchronous post-create work (repositories, blockers, event)
//
// Settlement (step 7) and asynchronous dispatch (step 8) are NOT done here:
// required synchronous work continues in the REST/MCP handlers after this
// call returns, so settlement is their responsibility (see the spec's
// "Settlement call site" section).
func (s *Service) CreateTask(ctx context.Context, req *CreateTaskRequest) (CreateTaskResult, error) {
	if err := s.authorizeWorkspaceID(ctx, req.WorkspaceID); err != nil {
		return CreateTaskResult{}, err
	}

	externalID, err := NormalizeExternalID(req.ExternalID)
	if err != nil {
		return CreateTaskResult{}, err
	}
	req.ExternalID = externalID

	if found, result, err := s.findTaskByExternalIDIfPresent(ctx, req.WorkspaceID, externalID); found {
		return result, err
	}

	prepared, err := s.prepareTaskForCreation(ctx, req, externalID)
	if err != nil {
		if found, ok := s.recoverFoundTaskAfterInsertFailure(ctx, req.WorkspaceID, externalID); ok {
			return found, nil
		}
		return CreateTaskResult{}, err
	}

	if err := s.createTaskWithCapacity(ctx, prepared.task); err != nil {
		if found, ok := s.recoverFoundTaskAfterInsertFailure(ctx, req.WorkspaceID, externalID); ok {
			return found, nil
		}
		s.logger.Error("failed to create task", zap.Error(err))
		return CreateTaskResult{}, err
	}

	return s.finalizeCreatedTask(ctx, prepared, req)
}

// preparedTask carries everything CreateTask resolved BEFORE inserting the
// task row: the row itself, and the fully-resolved task-repository rows that
// only need a TaskID stamped once the insert succeeds.
//
// Resolving the repositories here rather than in finalizeCreatedTask is the
// whole point: a reference that does not resolve must fail while there is
// still no task row to strand.
type preparedTask struct {
	task         *models.Task
	repositories []*models.TaskRepository
}

// prepareTaskForCreation runs create-sequence steps 4-5's non-write half:
// required validation, workflow/step resolution, and office identifier
// assignment, producing the in-memory task CreateTask is about to insert.
func (s *Service) prepareTaskForCreation(ctx context.Context, req *CreateTaskRequest, externalID string) (*preparedTask, error) {
	// Subtasks created without an explicit project inherit the parent's, so
	// office cost events (which copy tasks.project_id verbatim) attribute to
	// the same project as the rest of the tree instead of leaking. Runs first
	// so every isOfficeRequest check below — including prepareAutoTitle's —
	// classifies office-ness from the same, final req.ProjectID: an inherited
	// project must reach the same auto_title rejection an explicit one does,
	// not silently create an office task carrying agent_title_pending.
	if err := s.inheritParentProject(ctx, req); err != nil {
		return nil, err
	}

	if err := prepareAutoTitle(req); err != nil {
		return nil, err
	}
	if err := s.validateCreateTaskRequest(req); err != nil {
		return nil, err
	}
	if err := s.validateSubtaskDepth(ctx, req); err != nil {
		return nil, err
	}

	// Subtasks created without explicit repositories inherit the parent's, so
	// an inherit_parent subtask resolves a repo at launch and can reuse the
	// parent's worktree (the UI omits repositories expecting this). Mirrors the
	// MCP create_task path so UI- and agent-created subtasks behave identically.
	if err := s.inheritParentRepositories(ctx, req); err != nil {
		return nil, err
	}
	if err := s.preflightRepositorySelections(ctx, req); err != nil {
		return nil, err
	}
	if err := s.validateTaskRepositoryPolicies(ctx, req.WorkspaceID, req.Repositories); err != nil {
		return nil, err
	}

	// For office tasks, resolve workflow from workspace
	if isOfficeRequest(req) && req.WorkflowID == "" {
		if err := s.resolveOfficeWorkflow(ctx, req); err != nil {
			return nil, err
		}
	}
	if err := s.validateTaskWorkflow(ctx, req); err != nil {
		return nil, err
	}
	if err := s.prepareContributionDestination(ctx, req); err != nil {
		return nil, err
	}

	workflowStepID := s.resolveWorkflowStep(ctx, req)
	task := s.buildTask(ctx, req, workflowStepID)
	task.ExternalID = externalID

	// Auto-assign identifier for office tasks
	if isOfficeRequest(req) {
		if err := s.assignIdentifier(ctx, task); err != nil {
			return nil, err
		}
	}

	return s.resolveTaskCreationReferences(ctx, req, task)
}

// resolveTaskCreationReferences resolves every remaining caller-supplied
// reference, and runs LAST in prepareTaskForCreation: the earlier steps
// rewrite req.Repositories in place (preflightRepositorySelections and
// prepareContributionDestination both do), so resolution has to see the final
// list. It still runs before the insert, which is the point — anything that
// can reject the request on bad input has now run with nothing yet written.
//
// One consequence is deliberate. A github_url or local_path input resolves
// through find-or-create, so a repository row may now be created before the
// task insert rather than after it, and a create that then fails admission
// (WIP limit, capacity) leaves that row behind. It is NOT rolled back, and the
// asymmetry is the reason: a stranded task row duplicates a card and carries
// deferred_launch, while a repository row is an idempotent workspace-level
// entity that the caller's retry resolves straight back onto. Deleting it
// would be the more dangerous choice — find-or-create means a concurrent task
// can bind to the same row in between, and rolling back would then delete a
// repository somebody else is using.
func (s *Service) resolveTaskCreationReferences(
	ctx context.Context, req *CreateTaskRequest, task *models.Task,
) (*preparedTask, error) {
	if err := s.validateBlockerReferences(ctx, req); err != nil {
		return nil, err
	}
	repositories, err := s.resolveTaskRepositoryRows(ctx, req.WorkspaceID, req.Repositories)
	if err != nil {
		return nil, err
	}
	return &preparedTask{task: task, repositories: repositories}, nil
}

func (s *Service) prepareContributionDestination(ctx context.Context, req *CreateTaskRequest) error {
	if s.contributionDestinationPreparer == nil || req.WorkflowID == "" {
		return nil
	}
	workflow, err := s.workflows.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return fmt.Errorf("load workflow for contribution destination: %w", err)
	}
	if workflow == nil || workflow.WorkflowTemplateID == nil || *workflow.WorkflowTemplateID == "" {
		return nil
	}
	repositories, err := s.loadContributionDestinationRepositories(ctx, req)
	if err != nil {
		return err
	}
	return s.contributionDestinationPreparer.PrepareContributionDestination(ctx, req, workflow, repositories)
}

func (s *Service) loadContributionDestinationRepositories(
	ctx context.Context,
	req *CreateTaskRequest,
) ([]*models.Repository, error) {
	if len(req.Repositories) == 0 || s.repoEntities == nil {
		return nil, nil
	}
	repositories := make([]*models.Repository, len(req.Repositories))
	for index, input := range req.Repositories {
		if input.RepositoryID == "" {
			continue
		}
		repository, err := s.repoEntities.GetRepository(ctx, input.RepositoryID)
		if err != nil {
			return nil, classifyRepositoryResolutionError(index, input.RepositoryID, err)
		}
		if repository == nil || repository.WorkspaceID != req.WorkspaceID {
			return nil, unknownRepositoryReferenceError(index, input.RepositoryID)
		}
		repositories[index] = repository
	}
	return repositories, nil
}

// finalizeCreatedTask runs create-sequence step 6, the required synchronous
// post-create work, after task has been inserted: blocker relationships,
// task repositories, the created-event publish, and the feeder-pull refresh.
func (s *Service) finalizeCreatedTask(ctx context.Context, prepared *preparedTask, req *CreateTaskRequest) (CreateTaskResult, error) {
	task := prepared.task
	// Create blocker relationships if specified. Every id here was validated
	// pre-insert, so a failure now means the blocker was deleted underneath
	// us — rare, but it must not leave the half-built task behind.
	for _, blockerID := range req.BlockedBy {
		if err := s.AddBlocker(ctx, task.ID, blockerID); err != nil {
			return CreateTaskResult{}, s.rollbackPartialTask(ctx, task.ID, fmt.Errorf("add blocker %s: %w", blockerID, err))
		}
	}

	if err := s.persistTaskRepositoryRows(ctx, task.ID, prepared.repositories); err != nil {
		return CreateTaskResult{}, s.rollbackPartialTask(ctx, task.ID, err)
	}

	// Load repositories into task for response
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	s.publishTaskEvent(ctx, events.TaskCreated, task, nil)
	s.pullTasksFromNewFeederWork(ctx, task.WorkflowID, task.WorkflowStepID)
	if refreshed, err := s.tasks.GetTask(ctx, task.ID); err != nil {
		s.logger.Warn("failed to refresh task after feeder pull", zap.String("task_id", task.ID), zap.Error(err))
	} else if refreshed != nil {
		refreshed.Repositories = task.Repositories
		task = refreshed
	}
	s.logger.Info("task created", zap.String("task_id", task.ID), zap.String("title", task.Title))

	return CreateTaskResult{Task: task, Outcome: CreateTaskOutcomeCreated}, nil
}

// findTaskByExternalIDIfPresent is CreateTask's step-3 lookup: when the
// request carries an external_id already held by a task, the caller MUST
// return that task (found=true) instead of continuing the create. found is
// false only when there is nothing to short-circuit on — no external_id, or
// a genuine lookup miss — in which case the caller continues normally.
func (s *Service) findTaskByExternalIDIfPresent(ctx context.Context, workspaceID, externalID string) (found bool, result CreateTaskResult, err error) {
	if externalID == "" {
		return false, CreateTaskResult{}, nil
	}
	task, lookupErr := s.tasks.GetTaskByExternalID(ctx, workspaceID, externalID)
	switch {
	case lookupErr == nil:
		s.hydrateTaskRelations(ctx, task)
		return true, CreateTaskResult{Task: task, Outcome: foundOutcomeFor(task)}, nil
	case !errors.Is(lookupErr, taskrepo.ErrTaskNotFound):
		return true, CreateTaskResult{}, lookupErr
	default:
		return false, CreateTaskResult{}, nil
	}
}

// recoverFoundTaskAfterInsertFailure is the TOCTOU backstop for step 3, and
// the admission-preemption guard: after a step-3 miss, ANY pre-insert
// failure — a unique-index loss on uniq_tasks_external_id, WIP/capacity
// admission rejecting the insert outright, or a step-4/5 failure inside
// prepareTaskForCreation (validation, workflow resolution, or identifier
// allocation) — must re-read by external_id before the original error
// surfaces. This is the spec's "any pre-insert failure — capacity,
// admission, or otherwise" wording taken literally: it is not scoped to
// admission/capacity failures alone. A hit means another caller won the
// race; ok=true tells the caller to return it as Found rather than a
// failure. ok=false (no external_id, or the re-read also finds nothing)
// means the original error must surface — this also covers a bare task-id
// primary-key collision, unrelated to external_id, falling through
// correctly, since no task holds this external_id either way.
func (s *Service) recoverFoundTaskAfterInsertFailure(ctx context.Context, workspaceID, externalID string) (CreateTaskResult, bool) {
	if externalID == "" {
		return CreateTaskResult{}, false
	}
	found, lookupErr := s.tasks.GetTaskByExternalID(ctx, workspaceID, externalID)
	if lookupErr != nil {
		return CreateTaskResult{}, false
	}
	s.hydrateTaskRelations(ctx, found)
	return CreateTaskResult{Task: found, Outcome: foundOutcomeFor(found)}, true
}

func deriveProvisionalTaskTitle(description string) (string, error) {
	words := strings.Fields(description)
	if len(words) == 0 {
		return "", ErrAutoTitlePromptRequired
	}
	if len(words) > 6 {
		words = words[:6]
	}
	title := strings.Join(words, " ")
	title = TruncateTaskTitle(title)
	return title, nil
}

func prepareAutoTitle(req *CreateTaskRequest) error {
	if !req.AutoTitle {
		return nil
	}
	if isOfficeRequest(req) {
		return ErrAutoTitleUnsupportedForOffice
	}
	if isConfigTaskMetadata(req.Metadata) {
		return nil
	}
	if req.IsEphemeral && strings.TrimSpace(req.Description) == "" {
		if strings.TrimSpace(req.Title) == "" {
			return ErrAutoTitlePromptRequired
		}
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata[models.MetaKeyAgentTitlePending] = true
		return nil
	}
	title, err := deriveProvisionalTaskTitle(req.Description)
	if err != nil {
		return err
	}
	req.Title = title
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata[models.MetaKeyAgentTitlePending] = true
	return nil
}

func (s *Service) pullTasksFromNewFeederWork(ctx context.Context, workflowID, feederStepID string) {
	stepLister, ok := s.workflowStepGetter.(workflowStepLister)
	if !ok || workflowID == "" || feederStepID == "" {
		return
	}
	steps, err := stepLister.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		s.logger.Warn("failed to list workflow steps after feeder task creation",
			zap.String("workflow_id", workflowID), zap.Error(err))
		return
	}
	for _, step := range steps {
		if step != nil && step.PullFromStepID == feederStepID {
			s.pullNextTaskOnVacate(ctx, step.ID, "")
		}
	}
}

// ReconcileFeederPulls wakes steps that pull from feederStepID. The
// orchestrator calls this after an admitted manual move's lifecycle barrier
// completes so selection and promotion rules stay owned by this service.
func (s *Service) ReconcileFeederPulls(ctx context.Context, workflowID, feederStepID string) {
	s.pullTasksFromNewFeederWork(ctx, workflowID, feederStepID)
}

func (s *Service) createTaskWithCapacity(ctx context.Context, task *models.Task) error {
	if task.IsEphemeral || task.WorkflowStepID == "" {
		return s.tasks.CreateTask(ctx, task)
	}
	if s.workflowStepGetter == nil {
		return s.tasks.CreateTask(ctx, task)
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil {
		return fmt.Errorf("load workflow step %s for task creation: %w", task.WorkflowStepID, err)
	}
	if step == nil {
		return fmt.Errorf("%w: workflow step not found: %s", ErrInvalidTaskWorkflow, task.WorkflowStepID)
	}
	if step.WIPLimit <= 0 {
		return s.tasks.CreateTask(ctx, task)
	}
	if admissionCreator, ok := s.tasks.(workflowStepAdmissionTaskCreator); ok {
		feederStepID, feederLimit, err := s.resolveAdmissionFeeder(ctx, step)
		if err != nil {
			return err
		}
		return admissionCreator.CreateTaskWithWorkflowStepAdmission(ctx, task, step.ID, step.WIPLimit, feederStepID, feederLimit)
	}
	creator, ok := s.tasks.(workflowStepCapacityTaskCreator)
	if !ok {
		return fmt.Errorf("workflow step %s has WIP limit %d but capacity-aware task persistence is unavailable", step.ID, step.WIPLimit)
	}
	return creator.CreateTaskIfWorkflowStepHasCapacity(ctx, task, step.ID, step.WIPLimit)
}

func (s *Service) resolveAdmissionFeeder(ctx context.Context, step *wfmodels.WorkflowStep) (string, int, error) {
	if step.PullFromStepID == "" {
		return "", 0, nil
	}
	if s.workflowStepGetter == nil {
		return "", 0, fmt.Errorf("workflow step %s configures feeder %s but workflow step lookup is unavailable", step.ID, step.PullFromStepID)
	}
	feeder, err := s.workflowStepGetter.GetStep(ctx, step.PullFromStepID)
	if err != nil {
		return "", 0, fmt.Errorf("load feeder workflow step %s: %w", step.PullFromStepID, err)
	}
	if feeder == nil || feeder.WorkflowID != step.WorkflowID {
		return "", 0, fmt.Errorf("workflow step %s has invalid feeder %s", step.ID, step.PullFromStepID)
	}
	return feeder.ID, feeder.WIPLimit, nil
}

// inheritParentRepositories fills req.Repositories from the parent task when a
// subtask is created without explicit repositories. This applies to any
// repo-less subtask (not only inherit_parent ones), matching the MCP
// create_task path (mcp/handlers.inheritedRepoInputs) so UI- and agent-created
// subtasks behave identically — the UI's new_workspace mode always sends repos,
// so in practice only inherit_parent reaches here empty. RepositoryID and
// BaseBranch carry over; CheckoutBranch is dropped on purpose because two
// worktrees can't share a working branch, so the subtask branches off the same
// base as the parent.
//
// A lookup failure is returned rather than swallowed: a subtask silently
// created with no repositories can't establish a worktree, which would
// reintroduce the exact fresh-worktree bug this inheritance is meant to fix —
// failing fast surfaces the problem at creation time instead.
func (s *Service) inheritParentRepositories(ctx context.Context, req *CreateTaskRequest) error {
	if req.ParentID == "" || len(req.Repositories) > 0 {
		return nil
	}
	parentRepos, err := s.taskRepos.ListTaskRepositories(ctx, req.ParentID)
	if err != nil {
		return fmt.Errorf("list parent repositories for subtask inheritance: %w", err)
	}
	inherited := make([]TaskRepositoryInput, 0, len(parentRepos))
	for _, r := range parentRepos {
		if r == nil || r.RepositoryID == "" {
			continue
		}
		inherited = append(inherited, TaskRepositoryInput{
			RepositoryID: r.RepositoryID,
			BaseBranch:   r.BaseBranch,
		})
	}
	if len(inherited) > 0 {
		req.Repositories = inherited
	}
	return nil
}

// inheritParentProject fills req.ProjectID from the parent task when a
// subtask is created without an explicit project. Office cost events copy
// tasks.project_id verbatim (see office/service/event_subscribers.go
// projectIDForTask) with no ancestry walk or fallback, so a subtask left
// projectless never rolls up to its tree's budget even though its parent
// has one.
//
// CreateTask authorizes only req.WorkspaceID, never the parent, and the MCP
// create-task path deliberately allows an explicit req.WorkspaceID that
// differs from the parent's (TestHandleCreateTask_SubtaskHonorsExplicitWorkspaceAndWorkflow) —
// so a workspace mismatch cannot be rejected outright without breaking that
// flow. Inheritance is skipped instead: a caller authorized only for
// workspace A that passes a parent from workspace B gets a projectless
// subtask in A, never B's project silently attributed to A. This mirrors
// the pre-fix behavior for every subtask (blank project) rather than
// introducing a new failure mode.
func (s *Service) inheritParentProject(ctx context.Context, req *CreateTaskRequest) error {
	if req.ParentID == "" || req.ProjectID != "" {
		return nil
	}
	parent, err := s.tasks.GetTask(ctx, req.ParentID)
	if err != nil {
		return fmt.Errorf("get parent task for project inheritance: %w", err)
	}
	if parent.WorkspaceID != req.WorkspaceID {
		return nil
	}
	req.ProjectID = parent.ProjectID
	return nil
}

// validateCreateTaskRequest validates constraints for task creation.
func (s *Service) validateCreateTaskRequest(req *CreateTaskRequest) error {
	if err := validateTaskTitle(req.Title); err != nil {
		return err
	}
	isOffice := isOfficeRequest(req)
	// Automation runs never land on a board, so they need no workflow — the
	// trigger is the start signal, not a column. They are still ordinary,
	// persistent tasks; only their origin keeps them out of board reads.
	isAutomationRun := req.Origin == models.TaskOriginAutomationRun
	if !req.IsEphemeral && !isOffice && !isAutomationRun && req.WorkflowID == "" {
		return fmt.Errorf("workflow_id is required for non-ephemeral tasks")
	}
	if req.IsEphemeral && req.WorkflowID != "" {
		return fmt.Errorf("workflow_id must be empty for ephemeral tasks")
	}
	if err := validateTaskRepositoryBranches(req.Repositories); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateTaskWorkflow(ctx context.Context, req *CreateTaskRequest) error {
	if req.WorkflowID == "" {
		if req.WorkflowStepID != "" {
			return fmt.Errorf("%w: workflow_step_id requires workflow_id", ErrInvalidTaskWorkflow)
		}
		return nil
	}
	workflow, err := s.workflows.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return err
	}
	if workflow == nil {
		return fmt.Errorf("%w: workflow not found", ErrInvalidTaskWorkflow)
	}
	if workflow.WorkspaceID != req.WorkspaceID {
		return repoerrors.ErrWorkspaceNotFound
	}
	if req.WorkflowStepID == "" {
		return nil
	}
	if s.workflowStepGetter == nil {
		return fmt.Errorf("%w: workflow step validation unavailable", ErrInvalidTaskWorkflow)
	}
	step, err := s.workflowStepGetter.GetStep(ctx, req.WorkflowStepID)
	if err != nil {
		return fmt.Errorf("%w: workflow_step_id: %w", ErrInvalidTaskWorkflow, err)
	}
	if step == nil || step.WorkflowID != req.WorkflowID {
		return fmt.Errorf("%w: workflow step not found: %s", ErrInvalidTaskWorkflow, req.WorkflowStepID)
	}
	return nil
}

// validateSubtaskDepth prevents nesting deeper than one level for kanban
// (non-office) tasks. Office task trees intentionally allow arbitrary depth.
func (s *Service) validateSubtaskDepth(ctx context.Context, req *CreateTaskRequest) error {
	if req.ParentID == "" {
		return nil
	}
	parent, err := s.tasks.GetTask(ctx, req.ParentID)
	if err != nil {
		return fmt.Errorf("invalid parent_id: %w", err)
	}
	if parent.ParentID != "" && !parent.IsFromOffice {
		return ErrSubtaskDepthExceeded
	}
	return nil
}

// resolveOfficeWorkflow sets WorkflowID on the request from the workspace's office workflow.
func (s *Service) resolveOfficeWorkflow(ctx context.Context, req *CreateTaskRequest) error {
	_, orchWorkflowID, err := s.tasks.GetWorkspaceTaskPrefix(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get office workflow for workspace: %w", err)
	}
	if orchWorkflowID == "" {
		return fmt.Errorf("workspace %s has no office workflow configured", req.WorkspaceID)
	}
	req.WorkflowID = orchWorkflowID
	return nil
}

// resolveWorkflowStep resolves the starting workflow step for a new task.
//
// Three destinations, picked by what the caller is asking for:
//
//   - plan mode → the first step by position. Planning happens before the work,
//     so the task belongs at the head of the board even when a later step is
//     marked as the start step.
//   - starting an agent now → the first step that runs agents
//     (on_enter: auto_start_agent). A task that is about to run does not belong
//     in a parking column that was never configured to run anything.
//   - everything else → the workflow's start step (is_start_step).
//
// The middle case is the one that is easy to get wrong: `is_start_step` and
// `auto_start_agent` are separate settings, and routing an agent start through
// the start step silently made the two synonymous. It went unnoticed because
// every built-in template puts both on the same step.
func (s *Service) resolveWorkflowStep(ctx context.Context, req *CreateTaskRequest) string {
	workflowStepID := req.WorkflowStepID
	if workflowStepID == "" && req.WorkflowID != "" && s.startStepResolver != nil {
		var resolvedID string
		var err error
		switch {
		case req.PlanMode:
			resolvedID, err = s.startStepResolver.ResolveFirstStep(ctx, req.WorkflowID)
		case req.StartAgent:
			resolvedID, err = s.startStepResolver.ResolveAutoStartStep(ctx, req.WorkflowID)
		default:
			resolvedID, err = s.startStepResolver.ResolveStartStep(ctx, req.WorkflowID)
		}
		if err != nil {
			s.logger.Warn("failed to resolve start step, using empty",
				zap.String("workflow_id", req.WorkflowID),
				zap.Error(err))
		} else {
			workflowStepID = resolvedID
		}
	}
	return workflowStepID
}

// buildTask constructs a Task model from the CreateTaskRequest.
func (s *Service) buildTask(ctx context.Context, req *CreateTaskRequest, workflowStepID string) *models.Task {
	state := v1.TaskStateCreated
	if req.State != nil {
		state = *req.State
	}
	origin := req.Origin
	if origin == "" {
		origin = models.TaskOriginManual
	}
	labels := req.Labels
	if labels == "" {
		labels = "[]"
	}
	priority := req.Priority
	if priority == "" {
		// Office tasks have a TEXT priority column with a CHECK constraint
		// against the canonical four-value enum; default empty to defaultPriority
		// so callers (e.g. onboarding) can omit it.
		priority = defaultPriority
	}
	metadata := cloneTaskMetadata(req.Metadata)
	delete(metadata, models.MetaKeyDeferredLaunch)
	if req.DeferredLaunch != nil {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		launch := cloneTaskMetadata(req.DeferredLaunch)
		delete(launch, models.DeferredLaunchUserIDKey)
		delete(launch, models.DeferredLaunchRecordRecentUseKey)
		if req.RecordAgentProfileRecentUse {
			if identity, ok := authn.IdentityFromContext(ctx); ok && identity.UserID != "" {
				launch[models.DeferredLaunchUserIDKey] = identity.UserID
				launch[models.DeferredLaunchRecordRecentUseKey] = true
			}
		}
		if ResolveStartWhenUnblocked(req) {
			// Mark the intent as a dependency-chain step. The record is the same
			// one WIP overflow persists — reused so "launch exactly once" and
			// restart survival are inherited — and the flag is what lets
			// dependency resolution recognise its own intents.
			launch[models.DeferredLaunchStartWhenUnblockedKey] = true
		}
		metadata[models.MetaKeyDeferredLaunch] = launch
	}
	if wsPath := strings.TrimSpace(req.WorkspacePath); wsPath != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata[models.MetaKeyWorkspacePath] = wsPath
	}
	return &models.Task{
		ID:                     uuid.New().String(),
		WorkspaceID:            req.WorkspaceID,
		WorkflowID:             req.WorkflowID,
		WorkflowStepID:         workflowStepID,
		Title:                  req.Title,
		Description:            req.Description,
		State:                  state,
		Priority:               priority,
		Position:               req.Position,
		Metadata:               metadata,
		IsEphemeral:            req.IsEphemeral,
		ParentID:               req.ParentID,
		Autopilot:              req.Autopilot,
		AssigneeAgentProfileID: req.AssigneeAgentProfileID,
		Origin:                 origin,
		ProjectID:              req.ProjectID,
		Labels:                 labels,
	}
}

// assignIdentifier generates a sequential identifier (e.g. "KAN-1") for the task.
func (s *Service) assignIdentifier(ctx context.Context, task *models.Task) error {
	prefix, _, err := s.tasks.GetWorkspaceTaskPrefix(ctx, task.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to get task prefix: %w", err)
	}
	seq, err := s.tasks.IncrementTaskSequence(ctx, task.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to increment task sequence: %w", err)
	}
	task.Identifier = fmt.Sprintf("%s-%d", prefix, seq)
	return nil
}

// createTaskRepositories creates task-repository associations, resolving local paths to repository IDs.
func (s *Service) createTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	rows, err := s.resolveTaskRepositoryRows(ctx, workspaceID, repositories)
	if err != nil {
		return err
	}
	return s.persistTaskRepositoryRows(ctx, taskID, rows)
}

// persistTaskRepositoryRows writes already-resolved task-repository rows,
// stamping them with the owning task. Split from resolution so CreateTask can
// resolve BEFORE inserting the task row: every reference failure that used to
// surface here — after the insert, leaving an orphan task — is now raised by
// resolveTaskRepositoryRows while there is still nothing to orphan.
func (s *Service) persistTaskRepositoryRows(ctx context.Context, taskID string, rows []*models.TaskRepository) error {
	for _, taskRepo := range rows {
		taskRepo.TaskID = taskID
		if err := s.taskRepos.CreateTaskRepository(ctx, taskRepo); err != nil {
			s.logger.Error("failed to create task repository", zap.Error(err))
			return err
		}
	}
	return nil
}

// resolveTaskRepositoryRows resolves every repository input to a complete
// TaskRepository row WITHOUT persisting anything and without needing a task to
// exist yet: TaskID is left empty for persistTaskRepositoryRows to stamp.
//
// This half performs all reference resolution, policy lookup, and duplicate
// detection, so it is the half that can fail on caller-supplied data. Keeping
// it free of writes is what lets CreateTask run it pre-insert.
func (s *Service) resolveTaskRepositoryRows(
	ctx context.Context, workspaceID string, repositories []TaskRepositoryInput,
) ([]*models.TaskRepository, error) {
	repoByPath, err := s.repositoriesByLocalPath(ctx, workspaceID, repositories)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(repositories))
	rows := make([]*models.TaskRepository, 0, len(repositories))
	for i, repoInput := range repositories {
		row, err := s.resolveTaskRepositoryRow(ctx, workspaceID, i, repoInput, repoByPath)
		if err != nil {
			return nil, err
		}
		if err := s.claimRepositoryBranchSlot(ctx, seen, row, repoInput); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// repositoriesByLocalPath indexes the workspace's repositories by local path,
// and only when some input actually needs the lookup.
func (s *Service) repositoriesByLocalPath(
	ctx context.Context, workspaceID string, repositories []TaskRepositoryInput,
) (map[string]*models.Repository, error) {
	needed := false
	for _, repoInput := range repositories {
		if repoInput.RepositoryID == "" && repoInput.LocalPath != "" {
			needed = true
			break
		}
	}
	if !needed {
		return nil, nil
	}
	repos, err := s.repoEntities.ListRepositories(ctx, workspaceID)
	if err != nil {
		s.logger.Error("failed to list repositories", zap.Error(err))
		return nil, err
	}
	repoByPath := make(map[string]*models.Repository, len(repos))
	for _, repo := range repos {
		if repo.LocalPath == "" {
			continue
		}
		repoByPath[repo.LocalPath] = repo
	}
	return repoByPath, nil
}

// resolveTaskRepositoryRow resolves a single input into a complete, unsaved
// TaskRepository row.
func (s *Service) resolveTaskRepositoryRow(
	ctx context.Context, workspaceID string, index int,
	repoInput TaskRepositoryInput, repoByPath map[string]*models.Repository,
) (*models.TaskRepository, error) {
	repoInput, err := normalizeContributionBindings(repoInput)
	if err != nil {
		return nil, err
	}
	repositoryID, baseBranch, _, err := s.resolveRepoInput(ctx, workspaceID, repoInput, repoByPath)
	if err != nil {
		if repoInput.RepositoryID != "" {
			return nil, classifyRepositoryResolutionError(index, repoInput.RepositoryID, err)
		}
		return nil, err
	}
	if repositoryID == "" {
		return nil, fmt.Errorf("repository_id is required")
	}
	policy, err := s.resolveTaskRepositoryPolicy(ctx, repositoryID, repoInput)
	if err != nil {
		return nil, err
	}
	baseBranch, err = applyBranchPolicyBaseBranch(baseBranch, repoInput, policy)
	if err != nil {
		return nil, err
	}
	metadata, err := buildTaskRepositoryMetadata(repoInput)
	if err != nil {
		return nil, err
	}
	row := &models.TaskRepository{
		RepositoryID:   repositoryID,
		BaseBranch:     baseBranch,
		CheckoutBranch: repoInput.CheckoutBranch,
		Position:       index,
		Metadata:       metadata,
	}
	if policy != nil {
		row.BranchPolicyID = policy.ID
		row.BranchPolicyName = policy.Name
		row.BranchPolicyBaseBranch = policy.BaseBranch
		row.BranchPolicyBranchTemplate = policy.BranchTemplate
		row.BranchPolicyPullRequestTarget = policy.PullRequestTarget
	}
	return row, nil
}

// normalizeContributionBindings validates the remote-contribution and
// contribution-destination bindings and fills in the branches they imply.
func normalizeContributionBindings(repoInput TaskRepositoryInput) (TaskRepositoryInput, error) {
	if repoInput.RemoteContribution != nil {
		if err := repoInput.RemoteContribution.Validate(); err != nil {
			return repoInput, fmt.Errorf("invalid remote contribution: %w", err)
		}
		if repoInput.CheckoutBranch == "" {
			repoInput.CheckoutBranch = repoInput.RemoteContribution.HeadBranch
		}
		if repoInput.BaseBranch == "" {
			repoInput.BaseBranch = repoInput.RemoteContribution.BaseBranch
		}
		if repoInput.CheckoutBranch != repoInput.RemoteContribution.HeadBranch ||
			repoInput.BaseBranch != repoInput.RemoteContribution.BaseBranch {
			return repoInput, fmt.Errorf("remote contribution branches do not match the resolved binding")
		}
	}
	if repoInput.ContributionDestination != nil {
		if err := repoInput.ContributionDestination.Validate(); err != nil {
			return repoInput, fmt.Errorf("invalid contribution destination: %w", err)
		}
	}
	return repoInput, nil
}

// applyBranchPolicyBaseBranch reconciles the resolved base branch with the
// repository's branch policy.
func applyBranchPolicyBaseBranch(
	baseBranch string, repoInput TaskRepositoryInput, policy *models.RepositoryBranchPolicy,
) (string, error) {
	if policy == nil {
		return baseBranch, nil
	}
	if repoInput.RemoteContribution != nil {
		if policy.BaseBranch != repoInput.RemoteContribution.BaseBranch {
			return "", fmt.Errorf("remote contribution base branch %q does not match branch policy base branch %q",
				repoInput.RemoteContribution.BaseBranch, policy.BaseBranch)
		}
		return baseBranch, nil
	}
	if repoInput.PreserveBaseBranch {
		return baseBranch, nil
	}
	return policy.BaseBranch, nil
}

// buildTaskRepositoryMetadata assembles the row's metadata blob.
func buildTaskRepositoryMetadata(repoInput TaskRepositoryInput) (map[string]interface{}, error) {
	metadata := make(map[string]interface{})
	if prNum := resolvePRNumber(repoInput); prNum > 0 {
		metadata["pr_number"] = prNum
	}
	if repoInput.RemoteContribution != nil {
		if err := models.PutRemoteContribution(metadata, repoInput.RemoteContribution); err != nil {
			return nil, fmt.Errorf("persist remote contribution: %w", err)
		}
	}
	if repoInput.ContributionDestination != nil {
		if err := models.PutContributionDestination(metadata, repoInput.ContributionDestination); err != nil {
			return nil, fmt.Errorf("persist contribution destination: %w", err)
		}
	}
	return metadata, nil
}

// claimRepositoryBranchSlot enforces multi-branch uniqueness. The same
// repository may appear multiple times in a task on different branches.
// Identity is (repository_id, base_branch, checkout_branch) — base_branch
// matters because the worktree executor anchors the branch there while
// checkout_branch stays empty, and the local-executor flow puts the branch in
// checkout_branch with base_branch anchored to default_branch. Both shapes
// must dedup; matching DB key is UNIQUE(task_id, repository_id, base_branch,
// checkout_branch).
func (s *Service) claimRepositoryBranchSlot(
	ctx context.Context, seen map[string]bool, row *models.TaskRepository, repoInput TaskRepositoryInput,
) error {
	dedupKey := row.RepositoryID + "\x00" + row.BaseBranch + "\x00" + row.CheckoutBranch
	if !seen[dedupKey] {
		seen[dedupKey] = true
		return nil
	}
	label := s.repoDisplayLabel(ctx, repoInput, row.RepositoryID)
	branchLabel := row.CheckoutBranch
	if branchLabel == "" {
		branchLabel = row.BaseBranch
	}
	if branchLabel == "" {
		return fmt.Errorf("repository %q is listed more than once for this task", label)
	}
	return fmt.Errorf("repository %q on branch %q is listed more than once for this task", label, branchLabel)
}

// repoDisplayLabel returns a human-readable label for a repository to surface
// in the duplicate-repository error. It prefers owner/name parsed from the
// input's GitHub URL, then the resolved repo entity's owner/name (or bare
// name), and finally falls back to the repositoryID so the message is never
// empty. Best-effort: lookup failures degrade to the next fallback.
func (s *Service) repoDisplayLabel(ctx context.Context, repoInput TaskRepositoryInput, repositoryID string) string {
	if remoteURL := effectiveRemoteURL(repoInput); remoteURL != "" {
		if _, owner, name, _, err := parseRemoteRepositoryURL(remoteURL, repoInput.Provider); err == nil {
			return owner + "/" + name
		}
	}
	if repo, err := s.repoEntities.GetRepository(ctx, repositoryID); err == nil && repo != nil {
		if repo.ProviderOwner != "" && repo.ProviderName != "" {
			return repo.ProviderOwner + "/" + repo.ProviderName
		}
		if repo.Name != "" {
			return repo.Name
		}
	}
	return repositoryID
}

// ResolveRepositoryRef resolves a single TaskRepositoryInput to a
// (repositoryID, baseBranch) pair within the given workspace, creating the
// repository if necessary. Mirrors the resolution used during task creation
// (`createTaskRepositories`), but builds the local-path lookup map on demand
// so callers that only resolve one input (e.g. add_branch) don't need to
// thread the map themselves.
//
// Accepts inputs identified by RepositoryID, GitHubURL, or LocalPath. Returns
// an empty repositoryID with no error when none of those are set, letting
// callers decide whether to fall back to other defaults.
func (s *Service) ResolveRepositoryRef(ctx context.Context, workspaceID string, repoInput TaskRepositoryInput) (repositoryID, baseBranch string, created bool, err error) {
	var repoByPath map[string]*models.Repository
	if repoInput.RepositoryID == "" && repoInput.LocalPath != "" {
		repos, listErr := s.repoEntities.ListRepositories(ctx, workspaceID)
		if listErr != nil {
			return "", "", false, listErr
		}
		repoByPath = make(map[string]*models.Repository, len(repos))
		for _, repo := range repos {
			if repo.LocalPath == "" {
				continue
			}
			repoByPath[repo.LocalPath] = repo
		}
	}
	return s.resolveRepoInput(ctx, workspaceID, repoInput, repoByPath)
}

// resolveRepoInput resolves a RepositoryInput to a repositoryID and baseBranch,
// creating the repository if it doesn't exist yet. Returns created=true only
// when this call inserted a new Repository row (GitHub-URL miss → CreateRepository
// or LocalPath miss → CreateRepository); callers that want to roll back a fresh
// row on a later failure key off this flag.
func (s *Service) resolveRepoInput(ctx context.Context, workspaceID string, repoInput TaskRepositoryInput, repoByPath map[string]*models.Repository) (repositoryID, baseBranch string, created bool, err error) {
	repositoryID = repoInput.RepositoryID
	baseBranch = repoInput.BaseBranch
	if repositoryID != "" {
		return s.resolveRepoInputID(ctx, workspaceID, repositoryID, baseBranch)
	}

	// Only the plugin Host Tasks.Create path can set this internal marker.
	// REST, WebSocket, and MCP callers must go through the built-in resolver;
	// they cannot assert ownership of a plugin descriptor in request data.
	if repoInput.TrustedProviderDescriptor {
		return s.resolveTrustedRemoteRepository(ctx, workspaceID, repoInput, baseBranch)
	}

	if effectiveRemoteURL(repoInput) != "" {
		return s.resolveRepoInputRemote(ctx, workspaceID, repoInput, baseBranch)
	}

	if repoInput.LocalPath == "" {
		return repositoryID, baseBranch, false, nil
	}
	return s.resolveRepoInputLocal(ctx, workspaceID, repoInput, repoByPath, baseBranch)
}

func (s *Service) resolveRepoInputID(ctx context.Context, workspaceID, repositoryID, baseBranch string) (string, string, bool, error) {
	// Verify the repository belongs to the target workspace. Without this
	// check, an agent that knows a repository UUID from another workspace
	// could associate it with a task in this workspace via the MCP tool's
	// repository_id fast path (the github_url and local_path branches both
	// scope through FindOrCreateRepository, which is workspace-bound).
	repo, lookupErr := s.repoEntities.GetRepository(ctx, repositoryID)
	if lookupErr != nil {
		return "", "", false, fmt.Errorf("looking up repository %q: %w", repositoryID, lookupErr)
	}
	if repo == nil || repo.WorkspaceID != workspaceID {
		// Wrapped so transports classify this as a caller-supplied reference
		// problem (VALIDATION) rather than an internal failure; the message
		// itself is unchanged.
		return "", "", false, fmt.Errorf("%w: repository %q does not belong to workspace %q", ErrTaskReferenceNotFound, repositoryID, workspaceID)
	}
	replacementID, replacementCreated, replacementErr := s.safeRepositoryIDForTaskWorktree(ctx, workspaceID, repo)
	if replacementErr != nil {
		return "", "", false, replacementErr
	}
	if replacementID != "" {
		return replacementID, baseBranch, replacementCreated, nil
	}
	return repositoryID, baseBranch, false, nil
}

func (s *Service) safeRepositoryIDForTaskWorktree(ctx context.Context, workspaceID string, repo *models.Repository) (string, bool, error) {
	if !s.isKandevTaskWorktreeRepository(repo) {
		return "", false, nil
	}
	if repo.Provider == "" || repo.ProviderOwner == "" || repo.ProviderName == "" {
		return "", false, fmt.Errorf("repository %q points at a Kandev task worktree; use the source repository or GitHub URL", repo.ID)
	}
	existing, err := s.findSafeReplacementRepository(ctx, workspaceID, repo)
	if err != nil {
		return "", false, err
	}
	if existing != nil {
		return existing.ID, false, nil
	}
	created, createErr := s.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:    workspaceID,
		Name:           repo.ProviderOwner + "/" + repo.ProviderName,
		SourceType:     sourceTypeProvider,
		Provider:       repo.Provider,
		ProviderRepoID: repo.ProviderRepoID,
		ProviderHost:   repo.ProviderHost,
		ProviderScope:  repo.ProviderScope,
		ProviderOwner:  repo.ProviderOwner,
		ProviderName:   repo.ProviderName,
		DefaultBranch:  repo.DefaultBranch,
	})
	if createErr != nil {
		return "", false, fmt.Errorf("create provider repository for task worktree %q: %w", repo.ID, createErr)
	}
	return created.ID, true, nil
}

func (s *Service) replaceTaskWorktreeRepositoryMatch(ctx context.Context, workspaceID string, repo *models.Repository) (*models.Repository, bool, error) {
	replacementID, replacementCreated, err := s.safeRepositoryIDForTaskWorktree(ctx, workspaceID, repo)
	if err != nil {
		return nil, false, err
	}
	if replacementID == "" {
		return repo, false, nil
	}
	replacement, lookupErr := s.repoEntities.GetRepository(ctx, replacementID)
	if lookupErr != nil {
		return nil, false, fmt.Errorf("looking up repository %q: %w", replacementID, lookupErr)
	}
	if replacement == nil {
		return nil, false, fmt.Errorf("replacement repository %q no longer exists", replacementID)
	}
	return replacement, replacementCreated, nil
}

// findSafeReplacementRepository prefers an existing safe local clone over a
// provider row so private/offline repositories can reuse the user's checkout.
func (s *Service) findSafeReplacementRepository(ctx context.Context, workspaceID string, repo *models.Repository) (*models.Repository, error) {
	repos, err := s.repoEntities.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list repositories for task worktree replacement: %w", err)
	}
	var localClone *models.Repository
	var providerRepo *models.Repository
	for _, candidate := range repos {
		if candidate == nil || candidate.ID == repo.ID {
			continue
		}
		if !sameProviderIdentity(repo, candidate) {
			continue
		}
		if s.isKandevTaskWorktreeRepository(candidate) {
			continue
		}
		if candidate.SourceType == sourceTypeLocal && candidate.LocalPath != "" {
			if localClone == nil {
				localClone = candidate
			}
			continue
		}
		if candidate.SourceType == sourceTypeProvider && providerRepo == nil {
			providerRepo = candidate
		}
	}
	if localClone != nil {
		return localClone, nil
	}
	if providerRepo != nil {
		return providerRepo, nil
	}
	return nil, nil
}

func sameProviderIdentity(left, right *models.Repository) bool {
	return left.Provider == right.Provider &&
		normalizeProviderHost(left.Provider, left.ProviderHost) ==
			normalizeProviderHost(right.Provider, right.ProviderHost) &&
		left.ProviderOwner == right.ProviderOwner &&
		left.ProviderName == right.ProviderName
}

func (s *Service) isKandevTaskWorktreeRepository(repo *models.Repository) bool {
	return repo != nil && isKandevTaskWorktreePath(repo.LocalPath, s.discoveryConfig.TaskWorktreeRoots)
}

func isKandevTaskWorktreePath(path string, taskWorktreeRoots []string) bool {
	normalized := normalizeTaskWorktreePath(path)
	if normalized == "" {
		return false
	}
	for _, root := range taskWorktreeRoots {
		if pathAtOrInsideRoot(normalized, normalizeTaskWorktreePath(root)) {
			return true
		}
	}
	return strings.Contains(normalized, defaultKandevTaskWorktreePathSegment) ||
		strings.HasSuffix(normalized, strings.TrimSuffix(defaultKandevTaskWorktreePathSegment, "/"))
}

func normalizeTaskWorktreePath(path string) string {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if normalized == "." || normalized == "" {
		return ""
	}
	return normalized
}

func pathAtOrInsideRoot(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	if root != "/" {
		root = strings.TrimRight(root, "/")
	}
	return path == root || strings.HasPrefix(path, root+"/")
}

// resolveRepoInputLocal handles the LocalPath branch of resolveRepoInput.
// Looks the path up in the workspace snapshot first; on miss, delegates to
// FindOrCreateRepositoryByLocalPath, which re-checks the canonical path
// against the database immediately before inserting (see repoResolveMu) so a
// second resolver racing this one onto the same on-disk repo reuses its row
// instead of creating a sibling duplicate. Extracted to keep resolveRepoInput
// inside the cyclomatic-complexity budget.
func (s *Service) resolveRepoInputLocal(
	ctx context.Context, workspaceID string, repoInput TaskRepositoryInput,
	repoByPath map[string]*models.Repository, baseBranch string,
) (string, string, bool, error) {
	lookupPath := repoInput.LocalPath
	canonicalPath, probedBranch, pathErr := resolveExplicitLocalRepositoryPath(repoInput.LocalPath)
	if pathErr == nil {
		lookupPath = canonicalPath
	}
	repo := repoByPath[lookupPath]
	created := false
	if repo == nil {
		if isKandevTaskWorktreePath(lookupPath, s.discoveryConfig.TaskWorktreeRoots) {
			return "", "", false, fmt.Errorf("local path %q points at a Kandev task worktree; use the source repository or GitHub URL", repoInput.LocalPath)
		}
		name := strings.TrimSpace(repoInput.Name)
		if name == "" {
			name = filepath.Base(repoInput.LocalPath)
		}
		// Resolve default_branch by probing the repo on disk so it's anchored
		// to the integration branch (origin/HEAD or main/master) rather than
		// whatever feature branch the user happens to have checked out. The
		// frontend's `default_branch` hint wins when set; otherwise we probe
		// directly. Falling back to repoInput.BaseBranch is wrong because in
		// the local-executor flow that field carries the user's working
		// branch, which would permanently pin repositories.default_branch to
		// a feature branch and break every downstream merge-base lookup.
		defaultBranch := repoInput.DefaultBranch
		if defaultBranch == "" && pathErr == nil {
			// A manually supplied path is an explicit read-only probe. Canonical
			// repository validation protects the filesystem read; discovery roots
			// only constrain automatic scans.
			if probedBranch != "" {
				defaultBranch = probedBranch
			}
		}
		// identityPath is left empty when canonicalization failed: there is no
		// reliable identity to dedupe against in that case, so
		// FindOrCreateRepositoryByLocalPath always creates — same as prior
		// behavior for that edge case.
		var identityPath string
		if pathErr == nil {
			identityPath = canonicalPath
		}
		resolved, wasCreated, resolveErr := s.FindOrCreateRepositoryByLocalPath(ctx, workspaceID, identityPath, &CreateRepositoryRequest{
			WorkspaceID:   workspaceID,
			Name:          name,
			SourceType:    "local",
			LocalPath:     repoInput.LocalPath,
			DefaultBranch: defaultBranch,
		})
		if resolveErr != nil {
			return "", "", false, resolveErr
		}
		repo = resolved
		if repoByPath != nil {
			repoByPath[repoInput.LocalPath] = repo
			repoByPath[repo.LocalPath] = repo
		}
		created = wasCreated
	} else {
		replacement, replacementCreated, replaceErr := s.replaceTaskWorktreeRepositoryMatch(ctx, workspaceID, repo)
		if replaceErr != nil {
			return "", "", false, replaceErr
		}
		repo = replacement
		created = replacementCreated
	}
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repo.ID, baseBranch, created, nil
}

// resolveRepoInputGitHub handles the GitHub-URL branch of resolveRepoInput:
// parse owner/name, optionally probe the provider for default_branch, then
// FindOrCreateRepository. Extracted so resolveRepoInput stays under the
// cognitive-complexity budget after adding the probe-skip and probe-error
// arms.
func (s *Service) resolveRepoInputRemote(
	ctx context.Context, workspaceID string, repoInput TaskRepositoryInput, baseBranch string,
) (string, string, bool, error) {
	provider, owner, name, canonicalURL, parseErr := parseRemoteRepositoryURL(
		effectiveRemoteURL(repoInput), repoInput.Provider,
	)
	if parseErr != nil {
		return "", "", false, parseErr
	}
	if repoInput.Provider != "" && !strings.EqualFold(repoInput.Provider, provider) {
		return "", "", false, fmt.Errorf("remote_url provider %q does not match provider %q", provider, repoInput.Provider)
	}
	owner, metadataErr := validateRemoteRepositoryMetadata(repoInput, provider, owner, name)
	if metadataErr != nil {
		return "", "", false, metadataErr
	}
	defaultBranch := firstNonEmpty(repoInput.DefaultBranch, repoInput.BaseBranch)
	if defaultBranch == "" && repoInput.ResolveProviderDefaults && s.providerProber != nil && provider == providerGitHub {
		defaultBranch = s.probeProviderDefaultBranchIfMissing(ctx, workspaceID, provider, owner, name)
	}
	providerHost := remoteProviderHost(provider, canonicalURL)
	if repoInput.ProviderHost != "" && !strings.EqualFold(strings.TrimRight(repoInput.ProviderHost, "/"), providerHost) {
		return "", "", false, fmt.Errorf("remote_url provider host %q does not match %q", providerHost, repoInput.ProviderHost)
	}
	if provider == providerGitLab && providerHost != "https://gitlab.com" && !repoInput.TrustedRemote {
		return "", "", false, fmt.Errorf("untrusted GitLab origin %q", providerHost)
	}
	repo, repoCreated, createErr := s.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:    workspaceID,
		Provider:       provider,
		ProviderRepoID: repoInput.ProviderRepoID,
		ProviderHost:   providerHost,
		ProviderOwner:  owner,
		ProviderName:   name,
		RemoteURL:      canonicalURL,
		DefaultBranch:  defaultBranch,
	})
	if createErr != nil {
		return "", "", false, createErr
	}
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repo.ID, baseBranch, repoCreated, nil
}

// resolveTrustedRemoteRepository persists a complete provider descriptor
// supplied by an authorized plugin host. Unlike the built-in URL parser, this
// path never guesses a provider or rebuilds CloneURL, preserving context paths
// and allowing future provider IDs without host-specific branches.
func (s *Service) resolveTrustedRemoteRepository(
	ctx context.Context, workspaceID string, input TaskRepositoryInput, baseBranch string,
) (string, string, bool, error) {
	if err := validateTrustedRemoteRepository(input); err != nil {
		return "", "", false, err
	}
	repo, created, err := s.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:    workspaceID,
		Provider:       input.Provider,
		ProviderHost:   input.ProviderHost,
		ProviderScope:  input.ProviderScope,
		ProviderRepoID: input.ProviderRepoID,
		ProviderOwner:  input.ProviderOwner,
		ProviderName:   input.ProviderName,
		RemoteURL:      strings.TrimSpace(input.RemoteURL),
		DefaultBranch:  firstNonEmpty(input.DefaultBranch, input.BaseBranch),
	})
	if err != nil {
		return "", "", false, err
	}
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	return repo.ID, baseBranch, created, nil
}

func validateTrustedRemoteRepository(input TaskRepositoryInput) error {
	if strings.TrimSpace(input.Provider) == "" || strings.TrimSpace(input.ProviderHost) == "" ||
		strings.TrimSpace(input.ProviderRepoID) == "" || strings.TrimSpace(input.ProviderOwner) == "" ||
		strings.TrimSpace(input.ProviderName) == "" || strings.TrimSpace(input.RemoteURL) == "" {
		return errors.New("complete trusted remote repository descriptor is required")
	}
	if normalizeProviderHost(input.Provider, input.ProviderHost) == "" {
		return errors.New("trusted remote repository provider_host must be an http or https origin without credentials")
	}
	if _, err := validateProviderScope(input.ProviderScope); err != nil {
		return errors.New("trusted remote repository provider_scope is invalid")
	}
	if err := repoclone.ValidateHTTPSCloneOrigin(input.RemoteURL, input.ProviderHost); err != nil {
		return fmt.Errorf("trusted remote repository clone origin: %w", err)
	}
	parsed, _, err := normalizeRemoteRepositoryURL(input.RemoteURL)
	if err != nil {
		return err
	}
	if _, hasPassword := parsed.User.Password(); hasPassword ||
		((parsed.Scheme == protocolHTTP || parsed.Scheme == protocolHTTPS) && parsed.User != nil) {
		return errors.New("trusted remote repository remote_url must be credential-free")
	}
	return nil
}

func validateRemoteRepositoryMetadata(
	input TaskRepositoryInput, provider, owner, name string,
) (string, error) {
	if input.ProviderOwner != "" && provider != providerAzureDevOps && input.ProviderOwner != owner {
		return "", fmt.Errorf("remote_url owner %q does not match provider_owner %q", owner, input.ProviderOwner)
	}
	if input.ProviderName != "" && input.ProviderName != name {
		return "", fmt.Errorf("remote_url repository %q does not match provider_name %q", name, input.ProviderName)
	}
	if input.ProviderOwner != "" {
		return input.ProviderOwner, nil
	}
	return owner, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func effectiveRemoteURL(input TaskRepositoryInput) string {
	if strings.TrimSpace(input.RemoteURL) != "" {
		return strings.TrimSpace(input.RemoteURL)
	}
	return strings.TrimSpace(input.GitHubURL)
}

func parseRemoteRepositoryURL(raw, providerHint string) (provider, owner, name, canonical string, err error) {
	parsed, sshStyle, parseErr := normalizeRemoteRepositoryURL(raw)
	if parseErr != nil {
		return "", "", "", "", parseErr
	}
	host := strings.ToLower(parsed.Hostname())
	originHost := strings.ToLower(parsed.Host)
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	switch host {
	case "github.com", "www.github.com":
		return parseGitHubRemote(host, parts, sshStyle)
	case "gitlab.com", "www.gitlab.com":
		return parseGitLabRemote(parsed.Scheme, strings.TrimPrefix(originHost, "www."), parts, sshStyle)
	case "dev.azure.com":
		return parseAzureHTTPSRemote(parts)
	case "ssh.dev.azure.com":
		return parseAzureSSHRemote(parts)
	default:
		if strings.EqualFold(providerHint, providerGitLab) &&
			(parsed.Scheme == protocolHTTP || parsed.Scheme == protocolHTTPS) {
			return parseGitLabRemote(parsed.Scheme, originHost, parts, false)
		}
		return "", "", "", "", fmt.Errorf("unsupported remote repository host: %s", host)
	}
}

func normalizeRemoteRepositoryURL(raw string) (*url.URL, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, fmt.Errorf("empty remote repository URL")
	}
	sshStyle := strings.HasPrefix(raw, "git@") && strings.Contains(raw, ":")
	if sshStyle {
		hostAndPath := strings.TrimPrefix(raw, "git@")
		separator := strings.Index(hostAndPath, ":")
		raw = "ssh://git@" + hostAndPath[:separator] + "/" + hostAndPath[separator+1:]
	} else if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, parseErr := url.Parse(raw)
	if parseErr != nil || parsed.Host == "" {
		return nil, false, fmt.Errorf("invalid remote repository URL")
	}
	return parsed, sshStyle, nil
}

func parseGitHubRemote(host string, parts []string, sshStyle bool) (string, string, string, string, error) {
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", fmt.Errorf("invalid GitHub repository URL: expected github.com/owner/repo")
	}
	owner, name := parts[0], parts[1]
	canonical := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	if sshStyle {
		canonical = fmt.Sprintf("git@%s:%s/%s.git", host, owner, name)
	}
	return providerGitHub, owner, name, canonical, nil
}

func parseGitLabRemote(scheme, host string, parts []string, sshStyle bool) (string, string, string, string, error) {
	for index, part := range parts {
		if part == "-" {
			parts = parts[:index]
			break
		}
	}
	if len(parts) < 2 {
		return "", "", "", "", fmt.Errorf("invalid GitLab repository URL: expected gitlab.com/namespace/repo")
	}
	owner, name := strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1]
	if owner == "" || name == "" {
		return "", "", "", "", fmt.Errorf("remote repository owner and name are required")
	}
	if scheme != protocolHTTP && scheme != protocolHTTPS {
		scheme = protocolHTTPS
	}
	canonical := fmt.Sprintf("%s://%s/%s/%s.git", scheme, host, owner, name)
	if sshStyle {
		canonical = fmt.Sprintf("git@%s:%s/%s.git", host, owner, name)
	}
	return providerGitLab, owner, name, canonical, nil
}

func remoteProviderHost(provider, remoteURL string) string {
	if provider == providerGitHub {
		return githubProviderHost
	}
	if !strings.EqualFold(provider, providerGitLab) {
		return ""
	}
	origin, _ := ParseGitRemoteIdentity(remoteURL)
	if strings.HasPrefix(origin, protocolHTTP+"://") || strings.HasPrefix(origin, protocolHTTPS+"://") {
		return origin
	}
	if sshHost := strings.TrimPrefix(origin, "ssh://"); sshHost != origin && sshHost != "" {
		return protocolHTTPS + "://" + sshHost
	}
	return ""
}

func parseAzureHTTPSRemote(parts []string) (string, string, string, string, error) {
	if len(parts) != 4 || parts[2] != "_git" || parts[0] == "" || parts[1] == "" || parts[3] == "" {
		return "", "", "", "", fmt.Errorf("invalid Azure DevOps repository URL: expected dev.azure.com/org/project/_git/repo")
	}
	owner, name := parts[1], parts[3]
	canonical := fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", parts[0], owner, name)
	return providerAzureDevOps, owner, name, canonical, nil
}

func parseAzureSSHRemote(parts []string) (string, string, string, string, error) {
	if len(parts) != 4 || parts[0] != "v3" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return "", "", "", "", fmt.Errorf("invalid Azure DevOps SSH URL: expected git@ssh.dev.azure.com:v3/org/project/repo")
	}
	owner, name := parts[2], parts[3]
	canonical := fmt.Sprintf("git@ssh.dev.azure.com:v3/%s/%s/%s", parts[1], owner, name)
	return providerAzureDevOps, owner, name, canonical, nil
}

// probeProviderDefaultBranchIfMissing returns a default_branch resolved via
// the provider prober, but only when the workspace doesn't already hold the
// repo with a non-empty default_branch (the existing value wins downstream,
// so the remote round-trip would be pure waste). A DB lookup error skips
// the probe entirely — FindOrCreateRepository will hit the same DB and
// surface the real cause; we log the lookup failure for observability.
// Probe errors fall through to "" so the AddBranchToTask gate surfaces an
// actionable validation rejection rather than a silent orphan.
func (s *Service) probeProviderDefaultBranchIfMissing(
	ctx context.Context, workspaceID, provider, owner, name string,
) string {
	existing, lookupErr := s.repoEntities.GetRepositoryByProviderIdentity(ctx, models.ProviderRepositoryIdentity{
		WorkspaceID: workspaceID, Provider: provider, Host: githubProviderHost, Owner: owner, Name: name,
	})
	if lookupErr != nil {
		s.logger.Warn("resolveRepoInput: failed to look up existing repo before probe",
			zap.String("provider", provider),
			zap.String("owner", owner),
			zap.String("name", name),
			zap.Error(lookupErr))
		return ""
	}
	if existing != nil && existing.DefaultBranch != "" {
		return ""
	}
	probed, probeErr := s.providerProber.ProbeDefaultBranch(ctx, provider, owner, name)
	if probeErr != nil {
		return ""
	}
	return probed
}

// resolvePRNumber returns the GitHub PR number for a repository input. Prefers
// the explicit PRNumber field; falls back to parsing a /pull/<N> path out of
// GitHubURL when present. Returns 0 when no PR is identified.
//
// The PR number is needed at worktree-creation time so fork PRs (whose head
// branch only exists on the contributor's fork) can be materialized via the
// refs/pull/<N>/head refspec on the base repo instead of a branch-name fetch
// that would 404 against origin.
func resolvePRNumber(input TaskRepositoryInput) int {
	if input.PRNumber > 0 {
		return input.PRNumber
	}
	rawURL := effectiveRemoteURL(input)
	idx := strings.Index(rawURL, "/pull/")
	if idx < 0 {
		return 0
	}
	numStr := rawURL[idx+len("/pull/"):]
	if i := strings.IndexAny(numStr, "/?#"); i >= 0 {
		numStr = numStr[:i]
	}
	n, err := strconv.Atoi(numStr)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ReplaceTaskRepositories deletes all existing task-repository associations
// and recreates them. Exported for callers that mutate repository inputs
// (e.g. the fresh-branch flow rewriting BaseBranch) after CreateTask has
// already persisted the original set.
func (s *Service) ReplaceTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	return s.replaceTaskRepositories(ctx, taskID, workspaceID, repositories)
}

// replaceTaskRepositories deletes all existing task-repository associations and recreates them.
func (s *Service) replaceTaskRepositories(ctx context.Context, taskID, workspaceID string, repositories []TaskRepositoryInput) error {
	existing, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to load existing task repositories", zap.Error(err))
		return err
	}
	preserveTaskRepositoryPolicySnapshots(repositories, existing)
	if err := s.validateTaskRepositoryPolicies(ctx, workspaceID, repositories); err != nil {
		return err
	}
	if err := s.taskRepos.DeleteTaskRepositoriesByTask(ctx, taskID); err != nil {
		s.logger.Error("failed to delete task repositories", zap.Error(err))
		return err
	}
	return s.createTaskRepositories(ctx, taskID, workspaceID, repositories)
}

// GetTask retrieves a task by ID and populates repositories
func (s *Service) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if err := s.authorizeTaskID(ctx, id); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	s.hydrateTaskRelations(ctx, task)
	return task, nil
}

// hydrateTaskRelations populates the relations every task read is expected
// to carry — repositories and workspace folders — that a raw
// repository-layer task struct does not include. Callers that bypass GetTask
// (the create sequence's step-3 lookup, GetTaskByExternalID, and the
// settlement zero-row survivor re-read) must call this explicitly, or a
// retry/lookup silently returns a task missing fields a fresh GetTask would
// have populated.
func (s *Service) hydrateTaskRelations(ctx context.Context, task *models.Task) {
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}
	s.hydrateTaskWorkspaceFolders(ctx, task)
}

// UpdateTask updates an existing task and publishes a task.updated event
func (s *Service) UpdateTask(ctx context.Context, id string, req *UpdateTaskRequest) (*models.Task, error) {
	if err := s.authorizeTaskID(ctx, id); err != nil {
		return nil, err
	}
	if req.Title != nil {
		if err := validateTaskTitle(*req.Title); err != nil {
			return nil, err
		}
	}
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Repositories != nil {
		if err := s.preflightRepositoryInputs(ctx, task.WorkspaceID, req.Repositories); err != nil {
			return nil, err
		}
	}
	oldWorkflowStepID := task.WorkflowStepID
	var oldState *v1.TaskState
	stateChanged := false

	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.State != nil && task.State != *req.State {
		current := task.State
		oldState = &current
		task.State = *req.State
		stateChanged = true
	}
	if req.WorkflowStepID != nil {
		task.WorkflowStepID = *req.WorkflowStepID
	}
	if req.Position != nil {
		task.Position = *req.Position
	}
	if req.Metadata != nil {
		task.Metadata = protectedTaskMetadataUpdate(task.Metadata, req.Metadata)
	}
	if req.Title != nil {
		task.Title = *req.Title
		if task.Metadata != nil {
			delete(task.Metadata, models.MetaKeyAgentTitlePending)
			delete(task.Metadata, models.MetaKeyAgentTitleOwnerSessionID)
		}
	}
	parentCleared := false
	if req.ParentID != nil && *req.ParentID != task.ParentID {
		if err := s.resolveParentID(ctx, task, *req.ParentID); err != nil {
			return nil, err
		}
		parentCleared = *req.ParentID == ""
		task.ParentID = *req.ParentID
		// Re-parenting (or un-nesting) an inherit_parent subtask keeps its
		// materialized workspace as shared_group instead of silently
		// inheriting a different parent's workspace — the same composite
		// semantics as detach-then-nest.
		normalizeWorkspaceModeAfterReparent(task)
	}
	task.UpdatedAt = time.Now().UTC()

	updateCtx := ctx
	if req.WorkflowStepID != nil {
		actorKind, actorID := steptelemetry.HumanOrSystemActor(ctx)
		updateCtx = steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
			Trigger: steptelemetry.TriggerTaskUpdate, ActorKind: actorKind, ActorID: actorID,
		})
	}
	if err := s.tasks.UpdateTask(updateCtx, task); err != nil {
		s.logger.Error("failed to update task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}
	// UpdateTask may have applied a conditional title/metadata patch because
	// this snapshot was stale. Publish and return the row that actually won so
	// callers never receive the provisional title or pending marker again.
	task = s.reloadTaskAfterMutation(ctx, id, task, "update")
	if req.WorkflowStepID != nil && oldWorkflowStepID != task.WorkflowStepID {
		sessionID := ""
		if session := s.resolvePrimaryOrActiveSession(ctx, id); session != nil {
			sessionID = session.ID
		}
		// Generic task updates are a mutation boundary used by plugins and
		// MCP. Record them as system-originated unless a dedicated move API
		// supplied stronger provenance.
		s.recordManualStepTransition(ctx, sessionID, oldWorkflowStepID, task.WorkflowStepID, wfmodels.StepTransitionTriggerTaskUpdate, wfmodels.StepTransitionActorSystem)
	}

	// Update task repositories if provided
	if req.Repositories != nil {
		if err := s.replaceTaskRepositories(ctx, task.ID, task.WorkspaceID, req.Repositories); err != nil {
			return nil, err
		}
	}

	// Load repositories into task for response
	repos, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Error("failed to list task repositories", zap.Error(err))
	} else {
		task.Repositories = repos
	}

	if stateChanged && oldState != nil {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, oldState)
	}
	if parentCleared {
		// Explicitly signal the un-nest with parent_id: nil so clients can
		// distinguish "parent removed" from "parent unchanged" — matching the
		// detach path's event contract. publishTaskEvent otherwise omits an
		// empty parent_id entirely.
		s.publishTaskEventWithExtra(ctx, events.TaskUpdated, task, nil, map[string]interface{}{parentIDEventField: nil})
	} else {
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}
	s.logger.Info("task updated", zap.String("task_id", task.ID))

	return task, nil
}

func (s *Service) reloadTaskAfterMutation(ctx context.Context, id string, fallback *models.Task, operation string) *models.Task {
	current, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		s.logger.Warn("failed to reload task after mutation",
			zap.String("task_id", id), zap.String("operation", operation), zap.Error(err))
		return fallback
	}
	if current != nil {
		return current
	}
	return fallback
}

// SetPendingAgentTitle replaces a pending provisional title exactly once.
// Only the atomically claimed owner session may resolve it. A missing pending
// marker is an idempotent no-op so a human rename or an earlier agent call
// always wins a late request.
func (s *Service) SetPendingAgentTitle(ctx context.Context, id, sessionID, title string) (*models.Task, bool, string, error) {
	if err := s.authorizeTaskID(ctx, id); err != nil {
		return nil, false, "", err
	}
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, false, "", err
	}
	if !models.IsAgentTitlePending(task.Metadata) {
		return task, false, titleNotPendingReason, nil
	}
	if !models.IsAgentTitleOwner(task.Metadata, sessionID) {
		return task, false, titleNotOwnerReason, nil
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, false, "", errors.New("title is required")
	}
	if err := validateTaskTitle(title); err != nil {
		return nil, false, "", err
	}
	if setter, ok := s.tasks.(pendingTaskTitleSetter); ok {
		accepted, err := setter.SetTaskTitleIfPending(ctx, id, sessionID, title)
		if err != nil {
			s.logger.Error("failed to set pending agent title", zap.String("task_id", id), zap.Error(err))
			return nil, false, "", err
		}
		if !accepted {
			current, getErr := s.tasks.GetTask(ctx, id)
			if getErr != nil {
				return nil, false, "", getErr
			}
			if !models.IsAgentTitlePending(current.Metadata) {
				return current, false, titleNotPendingReason, nil
			}
			return current, false, titleNotOwnerReason, nil
		}
		// Reload the winning row so the response/event includes any ordinary
		// task update that raced the conditional title write.
		task = s.reloadTaskAfterMutation(ctx, id, task, "set pending agent title")
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
		return task, true, "", nil
	}
	task.Title = title
	delete(task.Metadata, models.MetaKeyAgentTitlePending)
	delete(task.Metadata, models.MetaKeyAgentTitleOwnerSessionID)
	task.UpdatedAt = time.Now().UTC()
	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to set pending agent title", zap.String("task_id", id), zap.Error(err))
		return nil, false, "", err
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	return task, true, "", nil
}

// parentChainWalkLimit bounds the ancestor walk in resolveParentID so a
// corrupted parent chain can never spin forever. Real subtask trees are
// nowhere near this depth.
const parentChainWalkLimit = 1000

// parentIDEventField is the task-event payload key carrying a task's parent.
// Emitting it explicitly (as nil) on un-nest lets clients tell "parent
// removed" apart from "parent unchanged".
const parentIDEventField = "parent_id"

// ErrInvalidParent wraps every rejection from resolveParentID so HTTP/WS
// handlers can classify a bad re-parent request as a client error (400)
// rather than an internal error (500).
var ErrInvalidParent = errors.New("invalid parent")

// normalizeWorkspaceModeAfterReparent applies the detach operation's
// workspace policy to an explicit parent change: an inherit_parent subtask
// whose hierarchy is being changed must not silently start inheriting a
// different parent's workspace, so its mode becomes shared_group (its
// materialized workspace and group membership are unchanged). Other modes
// pass through.
func normalizeWorkspaceModeAfterReparent(task *models.Task) {
	workspace, ok := task.Metadata["workspace"].(map[string]interface{})
	if !ok {
		return
	}
	if workspace["mode"] == workspaceModeInheritParent {
		workspace["mode"] = workspaceModeSharedGroup
	}
}

// resolveParentID validates a proposed parent assignment for task. An empty
// parentID (un-nest) is always allowed. A non-empty parentID must reference a
// different, existing, non-archived task in the same workspace, and must not
// introduce a cycle (nesting a task under one of its own descendants).
func (s *Service) resolveParentID(ctx context.Context, task *models.Task, parentID string) error {
	if parentID == "" {
		return nil
	}
	if parentID == task.ID {
		return fmt.Errorf("%w: a task cannot be its own parent", ErrInvalidParent)
	}
	parent, err := s.tasks.GetTask(ctx, parentID)
	if err != nil {
		return fmt.Errorf("%w: parent task not found: %s", ErrInvalidParent, parentID)
	}
	if parent.WorkspaceID != task.WorkspaceID {
		return fmt.Errorf("%w: parent task must belong to the same workspace", ErrInvalidParent)
	}
	if parent.ArchivedAt != nil {
		return fmt.Errorf("%w: parent task is archived", ErrInvalidParent)
	}
	// Cycle detection runs before the depth guard so a self-referential
	// re-parent reports the more specific "cycle" error rather than a depth
	// violation.
	if err := s.checkParentCycle(ctx, task, parent); err != nil {
		return err
	}
	return s.validateReparentDepth(ctx, task, parent)
}

// checkParentCycle walks up the parent's ancestor chain. Reaching task.ID means
// the new edge would close a cycle (task -> ... -> parent -> task).
func (s *Service) checkParentCycle(ctx context.Context, task, parent *models.Task) error {
	current := parent
	for i := 0; i < parentChainWalkLimit; i++ {
		if current.ID == task.ID {
			return fmt.Errorf("%w: nesting would create a cycle", ErrInvalidParent)
		}
		if current.ParentID == "" {
			return nil
		}
		ancestor, err := s.tasks.GetTask(ctx, current.ParentID)
		if err != nil {
			// Broken chain — treat the missing ancestor as a root so we don't
			// block a legitimate re-parent on inconsistent data.
			return nil
		}
		current = ancestor
	}
	return fmt.Errorf("%w: parent chain too deep", ErrInvalidParent)
}

// validateReparentDepth enforces the one-level subtask limit for kanban
// (non-office) tasks on the re-parent path, mirroring validateSubtaskDepth on
// the create path. Office task trees intentionally allow arbitrary depth, so
// the guard is skipped when either endpoint is an Office task. The returned
// error wraps both ErrInvalidParent (so handlers map it to HTTP 400) and
// ErrSubtaskDepthExceeded (so callers can still classify the depth violation).
func (s *Service) validateReparentDepth(ctx context.Context, task, parent *models.Task) error {
	if task.IsFromOffice || parent.IsFromOffice {
		return nil
	}
	// Nesting under a task that is itself a subtask would create a grandchild.
	if parent.ParentID != "" {
		return fmt.Errorf("%w: %w", ErrInvalidParent, ErrSubtaskDepthExceeded)
	}
	// Moving a task that already has children would push those children to
	// depth 2 under the new parent.
	children, err := s.tasks.ListChildren(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("%w: failed to check existing subtasks: %v", ErrInvalidParent, err)
	}
	if len(children) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidParent, ErrSubtaskDepthExceeded)
	}
	return nil
}

type taskMessageRollbackRepository interface {
	RestoreTaskMessageRollbackIfSessionState(
		ctx context.Context,
		task *models.Task,
		sessionID string,
		expectedSessionState models.TaskSessionState,
	) (bool, error)
}

// RestoreTaskMessageRollback restores message_task's task-state/workflow-step
// snapshot only while ownerSessionID still has expectedSessionState. It is a
// narrow compensation API: the repository predicate and both task-field
// writes share one SQL statement, so coordinator cancellation cannot be
// overwritten between a state check and the rollback write.
func (s *Service) RestoreTaskMessageRollback(
	ctx context.Context,
	taskID, ownerSessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
	workflowStepID string,
) (*models.Task, bool, error) {
	repo, ok := s.tasks.(taskMessageRollbackRepository)
	if !ok {
		return nil, false, errors.New("task repository does not support guarded message rollback")
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	oldState := task.State
	restoredTask := *task
	restoredTask.State = state
	restoredTask.WorkflowStepID = workflowStepID
	// The rollback's trigger is always unarchive_restore, but its actor
	// prefers an attribution already on ctx over the ActorSystem default —
	// the MCP message-dispatch rollback path (handlers.go's
	// handleMessageTask) knows the causal sender session and sets one
	// before calling here, mirroring the sqlite repository's
	// hardcodedTriggerAttribution prefer-preset-else-fallback pattern.
	rollbackAttribution := steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerUnarchiveRestore, ActorKind: steptelemetry.ActorSystem,
	}
	if preset := steptelemetry.FromContext(ctx); preset.ActorKind != steptelemetry.ActorUnknown {
		rollbackAttribution.ActorKind = preset.ActorKind
		rollbackAttribution.ActorID = preset.ActorID
		rollbackAttribution.SessionID = preset.SessionID
	}
	rollbackCtx := steptelemetry.WithAttribution(ctx, rollbackAttribution)
	updated, err := repo.RestoreTaskMessageRollbackIfSessionState(
		rollbackCtx,
		&restoredTask,
		ownerSessionID,
		expectedSessionState,
	)
	if err != nil || !updated {
		return task, updated, err
	}

	if restoredTask.State != oldState {
		s.publishTaskEvent(ctx, events.TaskStateChanged, &restoredTask, &oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, &restoredTask, nil)
	return &restoredTask, true, nil
}

// ArchiveTask archives a task by setting its archived_at timestamp.
// The task remains in the DB but is excluded from active board views.
// Active agent sessions are stopped and worktrees cleaned up in background.
func (s *Service) ArchiveTask(ctx context.Context, id string) error {
	start := time.Now()

	if err := s.authorizeTaskID(ctx, id); err != nil {
		return err
	}
	// 1. Get task and verify it exists
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return err
	}

	if task.ArchivedAt != nil {
		return fmt.Errorf("%w: %s", ErrTaskAlreadyArchived, id)
	}

	// 2. Gather data needed for cleanup BEFORE archive
	var stopTargets []taskStopTarget
	activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, id)
	if err != nil {
		return fmt.Errorf("list active task sessions for archive: %w", err)
	}
	if s.executionStopper != nil {
		stopTargets, err = s.buildStopTargets(ctx, id, activeSessions)
		if err != nil {
			return fmt.Errorf("list runtime cleanup inventory: %w", err)
		}
	}

	// 2b. Capture git archive snapshot for active sessions BEFORE stopping agents
	// Use a bounded timeout to prevent blocking the archive operation if agentctl is stuck.
	if s.gitArchiveCapture != nil && len(activeSessions) > 0 {
		for _, sess := range activeSessions {
			if sess == nil || sess.ID == "" {
				continue
			}
			snapCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := s.gitArchiveCapture.CaptureArchiveSnapshot(snapCtx, sess.ID)
			cancel()
			if err != nil {
				s.logger.Warn("failed to capture git archive snapshot",
					zap.String("task_id", id),
					zap.String("session_id", sess.ID),
					zap.Error(err))
			}
		}
	}

	sessions, err := s.sessions.ListTaskSessions(ctx, id)
	if err != nil {
		return fmt.Errorf("list task sessions for archive: %w", err)
	}

	worktrees, err := s.gatherWorktreesForDelete(ctx, id)
	if err != nil {
		return fmt.Errorf("list worktrees for archive: %w", err)
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, id)
	if err != nil {
		return fmt.Errorf("lookup task environment for archive: %w", err)
	}
	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: false, preserveBranches: true}
	cleanupJob, err := s.persistTaskResourceCleanup(
		ctx, id, models.TaskResourceCleanupTriggerArchive, "",
		sessions, worktrees, stopTargets, envCleanup, true,
	)
	if err != nil {
		return err
	}

	// 3. Set archived_at in DB
	if err := s.tasks.ArchiveTask(ctx, id); err != nil {
		s.resolveTaskResourceCleanupAfterMutationError(ctx, cleanupJob)
		return err
	}

	// Register the exact inventory before CANCELLED becomes visible. A launch
	// persistence loser can then distinguish these owned executions from one
	// that raced in after the snapshot and must clean itself up.
	s.registerTaskRuntimeStopOwners(stopTargets, true)

	// archived_at has committed above: the rest of this function only
	// reports on that already-durable state (cancelling sessions, re-reading
	// the task, publishing task.updated) or kicks off cleanup that already
	// runs detached from ctx. A client disconnecting right after the write
	// above must not stop any of that from finishing and reaching
	// event-driven clients that don't share the archiving caller's
	// connection.
	finalizeCtx := context.WithoutCancel(ctx)

	// 3b. Finalize active sessions in the DB and publish their cancellation
	// events. See finalizeCancelledSessions for the detailed rationale.
	s.finalizeCancelledSessions(finalizeCtx, id, activeSessions)

	// 4. Re-read task for updated archived_at field
	task, err = s.tasks.GetTask(finalizeCtx, id)
	if err != nil {
		return err
	}

	// 5. Publish task.updated event so frontend removes from board
	s.publishTaskEvent(finalizeCtx, events.TaskUpdated, task, nil)
	s.pullNextTaskOnVacate(finalizeCtx, task.WorkflowStepID, task.ID)
	s.logger.Info("task archived",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Background: Stop agents and cleanup worktrees
	if cleanupJob != nil {
		if err := s.StartPreparedTaskResourceCleanup(finalizeCtx, cleanupJob.OperationID); err != nil {
			s.logger.Warn("start committed archive resource cleanup",
				zap.String("job_id", cleanupJob.ID), zap.String("task_id", id), zap.Error(err))
		}
	} else if len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 || taskEnv != nil {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup, false,
			"task archived", "failed to stop session on task archive", "task archive cleanup completed")
	}

	return nil
}

// finalizeCancelledSessions finalizes an archived task's active sessions in
// the DB and publishes a session.state_changed event for each one actually
// cancelled. The async cleanup that follows tears down the agent processes;
// this records the terminal session state, which process teardown does not
// persist on its own. The DB write alone is invisible to any client cache
// (e.g. an Office task list's "is running" indicator) that's kept fresh
// exclusively by that event, and would otherwise show a live spinner
// forever after archive.
//
// CancelActiveTaskSessionsByTaskID is bounded by its own internal 10s
// timeout, so a single attempt can time out under SQLite writer contention
// alone — with archived_at already committed by the caller, that would
// silently leave this task's sessions stuck in an active DB state forever,
// since nothing else ever re-attempts the cancellation and no
// session.state_changed event is delivered to clear a client's running
// indicator. Retrying a small, fixed number of times — each attempt getting
// its own fresh 10s budget from the repository — meaningfully shrinks that
// window without making it unbounded. This is deliberately NOT a background
// reconciliation/sweep system, just closing the immediate race; if every
// attempt still fails, ArchiveTask has already committed archived_at, so
// this remains best-effort cleanup and is not a reason to fail the archive.
func (s *Service) finalizeCancelledSessions(ctx context.Context, taskID string, activeSessions []*models.TaskSession) {
	const maxCancelAttempts = 3
	const cancelRetryBackoff = 250 * time.Millisecond

	var cancelledSessions []*models.TaskSession
	var cancelErr error
	for attempt := 1; attempt <= maxCancelAttempts; attempt++ {
		cancelledSessions, cancelErr = s.sessions.CancelActiveTaskSessionsByTaskID(ctx, taskID, models.SessionArchiveCancelReason)
		if cancelErr == nil {
			break
		}
		if attempt < maxCancelAttempts {
			time.Sleep(cancelRetryBackoff)
		}
	}
	if cancelErr != nil {
		s.logger.Error("failed to reap active sessions on archive after retries",
			zap.String("task_id", taskID),
			zap.Int("attempts", maxCancelAttempts),
			zap.Error(cancelErr))
		return
	}
	if len(cancelledSessions) == 0 {
		return
	}
	s.logger.Info("reaped active sessions on archive",
		zap.String("task_id", taskID),
		zap.Int("count", len(cancelledSessions)))
	// Detach from ctx via WithoutCancel: the DB write above already
	// committed on a detached context, so a client disconnect here must
	// not also suppress the event publish below — event-driven clients
	// need session.state_changed regardless of whether the archiving
	// caller is still connected.
	// Deliberately left unbounded at the batch level: clarification expiry and
	// publishSessionsCancelled give each session their own independent timeout,
	// so one slow write or synchronous subscriber cannot starve later sessions.
	detachedCtx := context.WithoutCancel(ctx)
	if s.clarificationCanceller != nil {
		for _, session := range cancelledSessions {
			if session == nil || session.ID == "" {
				continue
			}
			expireCtx, cancelExpire := context.WithTimeout(detachedCtx, taskPublicationTimeout)
			_, err := s.clarificationCanceller.ExpireSessionAndNotify(expireCtx, session.ID)
			cancelExpire()
			if err != nil {
				s.logger.Error("failed to expire clarification after archive cancellation; response claims remain quarantined",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.Error(err))
			}
		}
	}
	s.publishSessionsCancelled(detachedCtx, taskID, activeSessions, cancelledSessions, models.SessionArchiveCancelReason)
}

func (s *Service) registerTaskRuntimeStopOwners(stopTargets []taskStopTarget, force bool) {
	if s.executionStopper == nil {
		return
	}
	for _, target := range stopTargets {
		if target.sessionID == "" || target.executionID == "" {
			continue
		}
		s.executionStopper.RegisterExecutionStopOwner(
			target.sessionID,
			target.executionID,
			force,
		)
	}
}

// DeleteTask deletes a task and publishes a task.deleted event.
// For fast UI response, the DB delete and event publish happen synchronously,
// while agent stopping and worktree cleanup happen asynchronously.
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	return s.deleteTaskWithReason(ctx, id, "")
}

// DeleteTaskWithReason behaves like DeleteTask but attaches a machine-readable
// reason (e.g. "pr_approved_by_user") to the task.deleted event so the frontend
// can explain why a focused task vanished.
func (s *Service) DeleteTaskWithReason(ctx context.Context, id, reason string) error {
	return s.deleteTaskWithReason(ctx, id, reason)
}

func (s *Service) deleteTaskWithReason(ctx context.Context, id, reason string) error {
	if err := s.authorizeTaskID(ctx, id); err != nil {
		return err
	}
	_, err := s.deleteTaskWithReasonAndDBDelete(ctx, id, reason, models.TaskResourceCleanupTriggerDelete, func(ctx context.Context, id string) (bool, error) {
		if err := s.tasks.DeleteTask(ctx, id); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func (s *Service) deleteExpiredQuickChatTask(ctx context.Context, id string, cutoff time.Time) (bool, error) {
	deleted, err := s.deleteTaskWithReasonAndDBDelete(ctx, id, "", models.TaskResourceCleanupTriggerQuickChatExpire, func(ctx context.Context, id string) (bool, error) {
		return s.tasks.DeleteExpiredQuickChatTask(ctx, id, cutoff)
	})
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return false, nil
	}
	return deleted, err
}

func (s *Service) deleteTaskWithReasonAndDBDelete(
	ctx context.Context,
	id string,
	reason string,
	trigger models.TaskResourceCleanupTrigger,
	deleteFromDB func(context.Context, string) (bool, error),
) (bool, error) {
	start := time.Now()

	// 1. Get task (sync, fast)
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return false, err
	}

	// 2. Gather data needed for cleanup BEFORE delete (sync, fast)
	sessions, err := s.sessions.ListTaskSessions(ctx, id)
	if err != nil {
		return false, fmt.Errorf("list task sessions for delete: %w", err)
	}

	worktrees, err := s.gatherWorktreesForDelete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("list worktrees for delete: %w", err)
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, id)
	if err != nil {
		return false, fmt.Errorf("lookup task environment for delete: %w", err)
	}
	stopTargets, err := s.deleteTaskStopTargets(ctx, id)
	if err != nil {
		return false, err
	}
	if preserved, err := s.preserveTaskEnvironmentForActiveBorrower(ctx, id, taskEnv); err != nil {
		return false, err
	} else if preserved {
		s.logger.Info("transferred borrowed task environment before task delete",
			zap.String("task_id", id),
			zap.String("env_id", taskEnvironmentID(taskEnv)),
			zap.String("new_owner_task_id", taskEnv.TaskID))
	}

	envCleanup := taskEnvironmentCleanup{env: taskEnv, deleteRow: false}
	cleanupJob, err := s.persistTaskResourceCleanup(
		ctx, id, trigger, "", sessions, worktrees, stopTargets, envCleanup, true,
	)
	if err != nil {
		return false, err
	}

	// 4. Delete from DB (sync, fast)
	deleted, err := deleteFromDB(ctx, id)
	if err != nil {
		s.resolveTaskResourceCleanupAfterMutationError(ctx, cleanupJob)
		s.logger.Error("failed to delete task", zap.String("task_id", id), zap.Error(err))
		return false, err
	}
	if !deleted {
		s.resolveTaskResourceCleanupAfterMutationError(ctx, cleanupJob)
		return false, nil
	}
	if s.attachmentSvc != nil {
		if err := s.attachmentSvc.DeleteByTask(context.WithoutCancel(ctx), id); err != nil {
			s.logger.Warn("failed to remove task attachment bytes",
				zap.String("task_id", id), zap.Error(err))
		}
	}
	// Remove dependency edges in both directions. task_blockers predates the
	// tasks foreign key so nothing cascades, and a left-over edge would keep a
	// dependent blocked forever on a task that no longer exists. Dependents are
	// refreshed but deliberately not started: deletion is not success.
	s.deleteDependencyEdgesForTask(context.WithoutCancel(ctx), id)

	// 5. Publish event (sync, fast) - frontend removes task immediately
	var extra map[string]interface{}
	if reason != "" {
		extra = map[string]interface{}{"reason": reason}
	}
	s.publishTaskEventWithExtra(ctx, events.TaskDeleted, task, nil, extra)
	s.pullNextTaskOnVacate(ctx, task.WorkflowStepID, task.ID)
	s.forgetTaskActivity(id)
	s.logger.Info("task deleted",
		zap.String("task_id", id),
		zap.Duration("duration", time.Since(start)))

	// 6. Stop agents and cleanup worktrees in the background. Carry the
	//    envCleanup struct so the task environment row is reset alongside
	//    the worktrees (an extra task.taskEnv != nil branch keeps the
	//    cleanup running when only the env needs reclaiming).
	hasCleanup := len(stopTargets) > 0 || s.worktreeCleanup != nil || len(sessions) > 0 || task.IsEphemeral || taskEnv != nil
	if cleanupJob != nil {
		if err := s.StartPreparedTaskResourceCleanup(ctx, cleanupJob.OperationID); err != nil {
			s.logger.Warn("start committed delete resource cleanup",
				zap.String("job_id", cleanupJob.ID), zap.String("task_id", id), zap.Error(err))
		}
	} else if hasCleanup {
		s.runAsyncTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup, true,
			"task deleted", "failed to stop session on task delete", "task cleanup completed")
	}

	return true, nil
}

func (s *Service) deleteTaskStopTargets(ctx context.Context, id string) ([]taskStopTarget, error) {
	// Must query before delete since DB records will be gone.
	if s.executionStopper == nil {
		return nil, nil
	}
	activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list active sessions for delete: %w", err)
	}
	stopTargets, err := s.buildStopTargets(ctx, id, activeSessions)
	if err != nil {
		return nil, fmt.Errorf("list runtime cleanup inventory: %w", err)
	}
	return stopTargets, nil
}

// CleanupTaskResources tears down a task's runtime resources (container,
// sandbox, worktree, executor_running rows, quick-chat dir, task_environment
// row) AFTER the task row has been archived or deleted by another path.
//
// Used by HandoffService.ArchiveTaskTree / DeleteTaskTree, which bypass
// Service.ArchiveTask / Service.DeleteTask and therefore miss the runtime
// teardown those wrappers run via runAsyncTaskCleanup. Without this call the
// agent gets stopped but its container/sandbox leaks indefinitely.
//
// The caller may already have cancelled active runs separately (cascade does
// this via runCanceller before invoking us), but terminal sessions can still
// have executors_running rows. We still derive runtime stop targets from
// executors_running here so cascade cleanup does not drop durable handles.
// deleteEnvRow controls whether the task_environment row is removed (true
// for delete cascade, false for archive — archive preserves the row). Runtime
// inventory failures abort cleanup so durable stop handles remain retryable.
func (s *Service) CleanupTaskResources(ctx context.Context, taskID string, deleteEnvRow bool) {
	// Capture the lifecycle disposition before borrower ownership transfer can
	// change whether the environment row itself is deleted.
	preserveBranches := !deleteEnvRow
	taskDeleted := deleteEnvRow
	if deleteEnvRow && s.attachmentSvc != nil {
		if err := s.attachmentSvc.DeleteByTask(context.WithoutCancel(ctx), taskID); err != nil {
			s.logger.Warn("failed to remove task attachment bytes during resource cleanup",
				zap.String("task_id", taskID), zap.Error(err))
		}
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list sessions for cascade cleanup",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	var stopTargets []taskStopTarget
	if s.executionStopper != nil {
		stopTargets, err = s.buildStopTargets(ctx, taskID, sessions)
		if err != nil {
			s.logger.Warn("skipping cascade cleanup because runtime inventory failed",
				zap.String("task_id", taskID),
				zap.Error(err))
			return
		}
	}
	worktrees, err := s.gatherWorktreesForDelete(ctx, taskID)
	if err != nil {
		s.logger.Warn("skipping cascade cleanup because worktree inventory failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, taskID)
	if err != nil {
		s.logger.Warn("skipping cascade cleanup because task environment inventory failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if deleteEnvRow {
		preserved, err := s.preserveTaskEnvironmentForActiveBorrower(ctx, taskID, taskEnv)
		if err != nil {
			s.logger.Warn("skipping cascade cleanup because task environment could not be preserved for borrower",
				zap.String("task_id", taskID),
				zap.String("env_id", taskEnvironmentID(taskEnv)),
				zap.Error(err))
			return
		}
		if preserved {
			deleteEnvRow = false
			s.logger.Info("transferred borrowed task environment before cascade delete",
				zap.String("task_id", taskID),
				zap.String("env_id", taskEnvironmentID(taskEnv)),
				zap.String("new_owner_task_id", taskEnv.TaskID))
		}
	}
	envCleanup := taskEnvironmentCleanup{
		env: taskEnv, deleteRow: deleteEnvRow, preserveBranches: preserveBranches,
	}
	if len(sessions) == 0 && len(worktrees) == 0 && len(stopTargets) == 0 && taskEnv == nil {
		return
	}
	reason := "cascade archive"
	if deleteEnvRow {
		reason = "cascade delete"
	}
	s.runAsyncTaskCleanup(taskID, sessions, worktrees, stopTargets, envCleanup, taskDeleted,
		reason, "failed to stop session on cascade cleanup", "cascade cleanup completed")
}

// gatherWorktreesForDelete collects worktrees for a task before it is deleted.
// For legacy WorktreeCleanup implementations that do not implement WorktreeProvider,
// it triggers cleanup immediately and returns nil.
func (s *Service) gatherWorktreesForDelete(ctx context.Context, taskID string) ([]*worktree.Worktree, error) {
	if s.worktreeCleanup == nil {
		return nil, nil
	}
	provider, ok := s.worktreeCleanup.(WorktreeProvider)
	if !ok {
		// Durable cleanup must persist its intent before invoking a destructive
		// legacy callback. Non-durable test/legacy wiring keeps the old behavior.
		if s.resourceCleanups == nil {
			if err := s.worktreeCleanup.OnTaskDeleted(ctx, taskID); err != nil {
				s.logger.Warn("failed to cleanup worktree on task deletion",
					zap.String("task_id", taskID), zap.Error(err))
			}
		}
		return nil, nil
	}
	worktrees, err := provider.GetAllByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return worktrees, nil
}

func (s *Service) gatherTaskEnvironmentForCleanup(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	if s.taskEnvironments == nil {
		return nil, nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (s *Service) runAsyncTaskCleanup(
	id string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	taskDeleted bool,
	stopReason, stopFailMsg, cleanupMsg string,
) {
	go s.runTaskCleanup(id, sessions, worktrees, stopTargets, envCleanup, taskDeleted, stopReason, stopFailMsg, cleanupMsg)
}

func (s *Service) runTaskCleanup(
	id string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	taskDeleted bool,
	stopReason, stopFailMsg, cleanupMsg string,
) {
	cleanupStart := time.Now()
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	refreshedTargets, err := s.refreshTaskRuntimeStopTargets(cleanupCtx, id, stopTargets)
	if err != nil {
		s.logger.Warn(cleanupMsg+" deferred because runtime inventory refresh failed",
			zap.String("task_id", id),
			zap.Error(err))
		s.signalCleanupDoneForTest()
		return
	}
	stopTargets = refreshedTargets
	s.registerTaskRuntimeStopOwners(stopTargets, true)

	stopOutcome := s.stopTaskRuntimeTargetsWithTaskDeleted(cleanupCtx, id, stopTargets, stopReason, stopFailMsg, taskDeleted, true)

	cleanupErrors := s.performTaskCleanup(cleanupCtx, id, sessions, worktrees, stopTargets, envCleanup,
		taskCleanupPreserveRows(stopOutcome))

	if len(cleanupErrors) > 0 {
		s.logger.Warn(cleanupMsg+" with errors",
			zap.String("task_id", id),
			zap.Int("error_count", len(cleanupErrors)),
			zap.Duration("duration", time.Since(cleanupStart)))
	} else {
		s.logger.Info(cleanupMsg,
			zap.String("task_id", id),
			zap.Duration("duration", time.Since(cleanupStart)))
	}
	s.signalCleanupDoneForTest()
}

// isCleanableSessionState reports whether a session has no running agent process
// and stop failures are therefore expected. Unlike the orchestrator's
// isTerminalSessionState (which excludes IDLE), this helper is used only during
// task cleanup to decide whether a stop failure should block environment teardown.
// IDLE is included because an idle session has already released its execution slot
// and will return ErrExecutionNotFound just like CANCELLED/COMPLETED/FAILED.
func isCleanableSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCancelled,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateIdle:
		return true
	}
	return false
}

func (s *Service) buildStopTargets(ctx context.Context, taskID string, activeSessions []*models.TaskSession) ([]taskStopTarget, error) {
	targets := make([]taskStopTarget, 0, len(activeSessions))
	seen := make(map[string]struct{})
	// Index session states so executor_running rows can be marked terminal.
	sessionStates := make(map[string]models.TaskSessionState, len(activeSessions))
	for _, sess := range activeSessions {
		if sess != nil {
			sessionStates[sess.ID] = sess.State
		}
	}
	if s.executors != nil {
		runningRows, err := s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
		if err != nil {
			return nil, err
		}
		for _, running := range runningRows {
			if running == nil || running.SessionID == "" {
				continue
			}
			target := taskStopTarget{
				sessionID:   running.SessionID,
				executionID: strings.TrimSpace(running.AgentExecutionID),
				terminal:    isCleanableSessionState(sessionStates[running.SessionID]),
			}
			targets = append(targets, target)
			seen[target.sessionID] = struct{}{}
		}
	}
	for _, sess := range activeSessions {
		if sess == nil || sess.ID == "" {
			continue
		}
		if _, ok := seen[sess.ID]; ok {
			continue
		}
		// Sessions without an executor_running row that are already in a terminal
		// state have no running process; skip creating a stop target.
		if isCleanableSessionState(sess.State) {
			continue
		}
		target := taskStopTarget{
			sessionID:   sess.ID,
			executionID: strings.TrimSpace(sess.AgentExecutionID),
		}
		if target.executionID == "" && s.executors != nil {
			running, err := s.executors.GetExecutorRunningBySessionID(ctx, sess.ID)
			if err == nil && running != nil {
				target.executionID = strings.TrimSpace(running.AgentExecutionID)
			}
		}
		targets = append(targets, target)
	}
	s.logger.Debug("prepared task cleanup stop targets",
		zap.String("task_id", taskID),
		zap.Int("count", len(targets)))
	return targets, nil
}

// refreshTaskRuntimeStopTargets merges the durable/pre-mutation snapshot with
// the live runtime inventory immediately before teardown. A launch may rotate
// an execution after the snapshot but before archive/delete becomes visible;
// keeping every exact ID makes cleanup safe across that race and worker
// restarts. A session-only fallback is retained only when no exact ID exists
// for that session; otherwise it would observe absence after the exact stop and
// keep a durable cleanup retrying forever.
func (s *Service) refreshTaskRuntimeStopTargets(
	ctx context.Context,
	taskID string,
	snapshot []taskStopTarget,
) ([]taskStopTarget, error) {
	if s.executionStopper == nil || s.executors == nil {
		return snapshot, nil
	}
	runningRows, err := s.executors.ListExecutorsRunningByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return mergeTaskStopTargets(taskStopTargetsFromRunningRows(runningRows), snapshot), nil
}

func taskStopTargetsFromRunningRows(runningRows []*models.ExecutorRunning) []taskStopTarget {
	targets := make([]taskStopTarget, 0, len(runningRows))
	for _, running := range runningRows {
		if running == nil {
			continue
		}
		targets = append(targets, taskStopTarget{
			sessionID:   strings.TrimSpace(running.SessionID),
			executionID: strings.TrimSpace(running.AgentExecutionID),
		})
	}
	return targets
}

func mergeTaskStopTargets(live, snapshot []taskStopTarget) []taskStopTarget {
	sessionsWithExactTarget := exactTaskStopTargetSessions(live, snapshot)
	targets := make([]taskStopTarget, 0, len(live)+len(snapshot))
	seen := make(map[string]int, len(live)+len(snapshot))
	appendTarget := func(target taskStopTarget) {
		target.sessionID = strings.TrimSpace(target.sessionID)
		target.executionID = strings.TrimSpace(target.executionID)
		if target.sessionID == "" {
			return
		}
		if target.executionID == "" {
			if _, hasExact := sessionsWithExactTarget[target.sessionID]; hasExact {
				return
			}
		}
		key := target.sessionID + "\x00" + target.executionID
		if index, exists := seen[key]; exists {
			targets[index].terminal = targets[index].terminal || target.terminal
			return
		}
		seen[key] = len(targets)
		targets = append(targets, target)
	}
	for _, target := range live {
		appendTarget(target)
	}
	for _, target := range snapshot {
		appendTarget(target)
	}
	return targets
}

func exactTaskStopTargetSessions(targetSets ...[]taskStopTarget) map[string]struct{} {
	exact := make(map[string]struct{})
	for _, targets := range targetSets {
		for _, target := range targets {
			sessionID := strings.TrimSpace(target.sessionID)
			if sessionID != "" && strings.TrimSpace(target.executionID) != "" {
				exact[sessionID] = struct{}{}
			}
		}
	}
	return exact
}

// taskRuntimeStopOutcome separates the two independent effects a stop pass has on
// downstream cleanup. failed drives durable-job retry accounting AND row
// preservation (a still-uncertain stop must be retried and its row kept).
// preserve keeps a row (and its worktree) from being torn down WITHOUT counting
// as a retryable failure — used for a confirmed-dead but resume-safe row that was
// repaired in place under the resume-safety invariant.
type taskRuntimeStopOutcome struct {
	failed   map[string]struct{}
	preserve map[string]struct{}
}

// taskCleanupPreserveRows is the set of sessions whose executor row and worktree
// must survive this cleanup pass: every failed (retryable) stop plus every
// resume-safe row repaired in place. performTaskCleanup treats membership as
// "do not delete this row/worktree".
func taskCleanupPreserveRows(outcome taskRuntimeStopOutcome) map[string]struct{} {
	if len(outcome.preserve) == 0 {
		return outcome.failed
	}
	preserve := make(map[string]struct{}, len(outcome.failed)+len(outcome.preserve))
	for id := range outcome.failed {
		preserve[id] = struct{}{}
	}
	for id := range outcome.preserve {
		preserve[id] = struct{}{}
	}
	return preserve
}

func (s *Service) stopTaskRuntimeTargets(ctx context.Context, taskID string, stopTargets []taskStopTarget, stopReason, stopFailMsg string) taskRuntimeStopOutcome {
	return s.stopTaskRuntimeTargetsWithTaskDeleted(ctx, taskID, stopTargets, stopReason, stopFailMsg, false)
}

func (s *Service) stopTaskRuntimeTargetsWithTaskDeleted(
	ctx context.Context,
	taskID string,
	stopTargets []taskStopTarget,
	stopReason, stopFailMsg string,
	taskDeleted bool,
	waitForStop ...bool,
) taskRuntimeStopOutcome {
	outcome := taskRuntimeStopOutcome{
		failed:   make(map[string]struct{}),
		preserve: make(map[string]struct{}),
	}
	if s.executionStopper == nil || len(stopTargets) == 0 {
		return outcome
	}
	wait := len(waitForStop) > 0 && waitForStop[0]
	for _, target := range stopTargets {
		if context.Cause(ctx) != nil {
			return outcome
		}
		s.stopTaskRuntimeTarget(ctx, taskID, target, stopReason, stopFailMsg, taskDeleted, wait, &outcome)
	}
	return outcome
}

func (s *Service) stopTaskRuntimeTarget(
	ctx context.Context,
	taskID string,
	target taskStopTarget,
	stopReason, stopFailMsg string,
	taskDeleted, waitForStop bool,
	outcome *taskRuntimeStopOutcome,
) {
	if target.executionID != "" {
		s.stopTaskRuntimeExecution(ctx, taskID, target, stopReason, stopFailMsg, outcome)
		return
	}
	s.stopTaskRuntimeSession(ctx, taskID, target, stopReason, stopFailMsg, taskDeleted, waitForStop, outcome)
}

func (s *Service) stopTaskRuntimeExecution(
	ctx context.Context,
	taskID string,
	target taskStopTarget,
	stopReason, stopFailMsg string,
	outcome *taskRuntimeStopOutcome,
) {
	err := s.executionStopper.StopExecution(ctx, target.executionID, stopReason, true)
	if err == nil || runtimeStopAlreadyComplete(err) {
		return
	}
	outcome.failed[target.sessionID] = struct{}{}
	s.logger.Warn(stopFailMsg,
		zap.String("task_id", taskID),
		zap.String("session_id", target.sessionID),
		zap.String("execution_id", target.executionID),
		zap.Error(err))
}

func (s *Service) stopTaskRuntimeSession(
	ctx context.Context,
	taskID string,
	target taskStopTarget,
	stopReason, stopFailMsg string,
	taskDeleted bool,
	waitForStop bool,
	outcome *taskRuntimeStopOutcome,
) {
	var err error
	if waitForStop {
		if synchronous, ok := s.executionStopper.(synchronousTaskExecutionStopper); ok {
			err = synchronous.StopSessionSynchronously(ctx, target.sessionID, stopReason, true)
		} else {
			err = s.executionStopper.StopSession(ctx, target.sessionID, stopReason, true)
		}
	} else {
		err = s.executionStopper.StopSession(ctx, target.sessionID, stopReason, true)
	}
	if err == nil {
		return
	}
	// A session-level not-found is retryable by default (the execution may
	// simply not be registered yet). But when the owned row is a
	// confirmed-dead LOCAL runtime, the runtime really is gone — treat the
	// stop as already complete so the durable cleanup job stops retrying a
	// runtime that will never come back.
	if runtimeStopAlreadyComplete(err) {
		if taskDeleted && s.stopDeletedSessionRuntime(ctx, taskID, target, stopReason) {
			return
		}
		if running := s.confirmedDeadLocalRow(ctx, target.sessionID); running != nil {
			s.reconcileConfirmedDeadRow(ctx, taskID, target.sessionID, running, outcome)
			return
		}
	}
	outcome.failed[target.sessionID] = struct{}{}
	s.logger.Warn(stopFailMsg,
		zap.String("task_id", taskID),
		zap.String("session_id", target.sessionID),
		zap.Error(err))
}

// stopDeletedSessionRuntime handles a session stop that raced with task deletion.
// A missing session is complete only when no executor row remains. If a late row
// has an exact execution ID, stop that execution before cleanup removes the row.
// When the row has no execution ID, return false so the caller can apply the
// confirmed-dead local probe before marking the session for retry.
func (s *Service) stopDeletedSessionRuntime(
	ctx context.Context,
	taskID string,
	target taskStopTarget,
	stopReason string,
) bool {
	if s.executors == nil || s.executionStopper == nil {
		return false
	}
	running, err := s.executors.GetExecutorRunningBySessionID(ctx, target.sessionID)
	if err != nil {
		if errors.Is(err, models.ErrExecutorRunningNotFound) {
			s.logger.Debug("deleted task session has no registered runtime",
				zap.String("task_id", taskID),
				zap.String("session_id", target.sessionID))
			return true
		}
		s.logger.Warn("failed to inspect deleted task session runtime",
			zap.String("task_id", taskID),
			zap.String("session_id", target.sessionID),
			zap.Error(err))
		return false
	}
	if running == nil {
		return true
	}
	executionID := strings.TrimSpace(running.AgentExecutionID)
	if executionID == "" {
		return false
	}

	s.executionStopper.RegisterExecutionStopOwner(target.sessionID, executionID, true)
	if err := s.executionStopper.StopExecution(ctx, executionID, stopReason, true); err != nil {
		if runtimeStopAlreadyComplete(err) {
			s.logger.Debug("late deleted task execution was already stopped",
				zap.String("task_id", taskID),
				zap.String("session_id", target.sessionID),
				zap.String("execution_id", executionID),
				zap.Error(err))
			return true
		}
		s.logger.Warn("failed to stop late deleted task execution",
			zap.String("task_id", taskID),
			zap.String("session_id", target.sessionID),
			zap.String("execution_id", executionID),
			zap.Error(err))
		return false
	}
	return true
}

// reconcileConfirmedDeadRow applies the resume-safety deletion invariant to a
// confirmed-dead local row whose stop reported not-found. A row holding a
// resume_token or backing a resumable session is repaired in place (never
// deleted) and marked preserve-without-retry; a row that is neither is left for
// the normal cleanup path to prune. Neither outcome is a failed stop, so the
// durable cleanup job does not retry solely because the runtime was absent.
func (s *Service) reconcileConfirmedDeadRow(
	ctx context.Context,
	taskID, sessionID string,
	running *models.ExecutorRunning,
	outcome *taskRuntimeStopOutcome,
) {
	if models.RowMustBePreserved(running, s.sessionStateForCleanup(ctx, sessionID)) {
		outcome.preserve[sessionID] = struct{}{}
		if err := s.executors.RepairExecutorRunningDead(ctx, sessionID); err != nil {
			s.logger.Warn("failed to repair confirmed-dead resume-safe runtime row",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		s.logger.Info("session runtime confirmed dead and resume-safe; repairing row in place",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return
	}
	s.logger.Info("session runtime confirmed dead and already gone; treating stop as complete",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
}

// sessionStateForCleanup best-effort reads a session's state for the
// resume-safety check. A missing session is treated as an empty (non-resumable)
// state so a row's preservation then hinges on its resume_token alone.
func (s *Service) sessionStateForCleanup(ctx context.Context, sessionID string) models.TaskSessionState {
	if s.sessions == nil {
		return ""
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return ""
	}
	return session.State
}

func runtimeStopAlreadyComplete(err error) bool {
	return errors.Is(err, runtimeapi.ErrNotFound)
}

// confirmedDeadLocalRow returns the executors_running row backing sessionID when
// it is a confirmed-dead LOCAL runtime — a local process handle that no longer
// exists on this host — and nil otherwise. It returns nil when no prober is
// wired, the row is missing/unreadable, or the runtime-aware liveness is
// anything other than Dead, so an alive, unknown, or remote row is never treated
// as absent.
func (s *Service) confirmedDeadLocalRow(ctx context.Context, sessionID string) *models.ExecutorRunning {
	if s.rowLivenessProber == nil || s.executors == nil {
		return nil
	}
	running, err := s.executors.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil {
		return nil
	}
	if s.rowLivenessProber.RowLiveness(running) != models.ProcessLivenessDead {
		return nil
	}
	return running
}

// performTaskCleanup handles post-deletion cleanup operations.
// Handles worktree cleanup, executor_running records, and quick-chat workspace directories.
// Agent stopping is handled separately in the DeleteTask background goroutine.
// Returns a slice of errors encountered (empty if all succeeded).
func (s *Service) performTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	preserveExecutorRows map[string]struct{},
) []error {
	var errs []error
	if cause := context.Cause(ctx); cause != nil {
		return []error{cause}
	}
	hasPreservedRuntimes := len(preserveExecutorRows) > 0

	if hasPreservedRuntimes {
		s.logger.Warn("skipping shared environment cleanup after failed runtime stop",
			zap.String("task_id", taskID),
			zap.Int("preserved_runtime_count", len(preserveExecutorRows)))
	}
	errs = append(errs, s.cleanupDestructiveTaskResources(ctx, taskID, sessions, worktrees, envCleanup, preserveExecutorRows)...)
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}

	sessionIDs := cleanupSessionIDs(sessions, stopTargets)
	for _, sessionID := range sessionIDs {
		if _, preserve := preserveExecutorRows[sessionID]; preserve {
			s.logger.Warn("preserving executor runtime row after failed stop",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID))
			continue
		}
		if cause := context.Cause(ctx); cause != nil {
			return append(errs, cause)
		}
		if err := s.executors.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
			s.logger.Debug("failed to delete executor runtime for session",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			// Don't add to errs - this is a debug-level issue
		}
	}

	// Cleanup quick-chat workspace directories for all tasks (not just ephemeral).
	// Non-ephemeral office tasks also get quick-chat dirs allocated by manager_launch.go;
	// both cases must be cleaned up to avoid a disk leak.
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}
	errs = append(errs, s.cleanupQuickChatDirs(ctx, taskID, sessionIDs, preserveExecutorRows)...)

	return errs
}

func (s *Service) signalCleanupDoneForTest() {
	if s.cleanupDoneForTest == nil {
		return
	}
	select {
	case s.cleanupDoneForTest <- struct{}{}:
	default:
	}
}

func cleanupSessionIDs(sessions []*models.TaskSession, stopTargets []taskStopTarget) []string {
	sessionIDs := make([]string, 0, len(sessions)+len(stopTargets))
	seen := make(map[string]struct{})
	for _, session := range sessions {
		if session == nil {
			continue
		}
		sessionIDs = appendUniqueSessionID(sessionIDs, seen, session.ID)
	}
	for _, target := range stopTargets {
		sessionIDs = appendUniqueSessionID(sessionIDs, seen, target.sessionID)
	}
	return sessionIDs
}

func appendUniqueSessionID(sessionIDs []string, seen map[string]struct{}, sessionID string) []string {
	if sessionID == "" {
		return sessionIDs
	}
	if _, ok := seen[sessionID]; ok {
		return sessionIDs
	}
	seen[sessionID] = struct{}{}
	return append(sessionIDs, sessionID)
}

func (s *Service) cleanupQuickChatDirs(
	ctx context.Context,
	taskID string,
	sessionIDs []string,
	preserveExecutorRows map[string]struct{},
) []error {
	if s.quickChatDir == "" {
		return nil
	}
	var errs []error
	for _, sessionID := range sessionIDs {
		if _, preserve := preserveExecutorRows[sessionID]; preserve {
			continue
		}
		sessionDir := filepath.Join(s.quickChatDir, sessionID)
		if _, statErr := os.Stat(sessionDir); statErr != nil {
			// Directory does not exist — nothing to remove.
			continue
		}
		if cause := context.Cause(ctx); cause != nil {
			return append(errs, cause)
		}
		if err := os.RemoveAll(sessionDir); err != nil {
			s.logger.Warn("failed to cleanup quick-chat workspace directory",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("path", sessionDir),
				zap.Error(err))
			errs = append(errs, fmt.Errorf("cleanup quick-chat dir %s: %w", sessionID, err))
			continue
		}
		s.logger.Debug("cleaned up quick-chat workspace directory",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("path", sessionDir))
	}
	return errs
}

func (s *Service) cleanupDestructiveTaskResources(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	envCleanup taskEnvironmentCleanup,
	preserveExecutorRows map[string]struct{},
) []error {
	var errs []error
	if cause := context.Cause(ctx); cause != nil {
		return []error{cause}
	}
	skipOwnedEnvironment, err := s.hasActiveOtherTaskSessionsForEnvironment(ctx, taskID, envCleanup.env)
	if err != nil {
		s.logger.Warn("skipping task environment cleanup after shared-environment ownership check failed",
			zap.String("task_id", taskID),
			zap.String("env_id", taskEnvironmentID(envCleanup.env)),
			zap.Error(err))
		errs = append(errs, fmt.Errorf("check task environment ownership %s: %w", taskEnvironmentID(envCleanup.env), err))
		skipOwnedEnvironment = true
	}
	if skipOwnedEnvironment {
		s.logger.Info("skipping task environment cleanup while another task still uses it",
			zap.String("task_id", taskID),
			zap.String("env_id", taskEnvironmentID(envCleanup.env)))
	}
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}
	if len(preserveExecutorRows) == 0 && !skipOwnedEnvironment {
		errs = append(errs, s.cleanupTaskEnvironment(ctx, taskID, envCleanup)...)
		if cause := context.Cause(ctx); cause != nil {
			return append(errs, cause)
		}
	}
	originalWorktreeCount := len(worktrees)
	worktrees = s.filterOwnedWorktreesForTaskCleanup(ctx, taskID, sessions, worktrees, envCleanup.env, skipOwnedEnvironment)
	worktrees = cleanupEligibleWorktrees(worktrees, envCleanup.env, preserveExecutorRows)
	var referenceErrs []error
	worktrees, referenceErrs = s.filterSharedWorktreesForTaskCleanup(ctx, taskID, sessions, worktrees)
	errs = append(errs, referenceErrs...)
	if len(worktrees) == 0 {
		if originalWorktreeCount > 0 {
			s.logger.Debug("no task worktrees eligible for cleanup",
				zap.String("task_id", taskID),
				zap.Int("input_count", originalWorktreeCount),
				zap.Int("preserved_runtime_count", len(preserveExecutorRows)))
		}
		return errs
	}
	if cause := context.Cause(ctx); cause != nil {
		return append(errs, cause)
	}
	var cleanupErr error
	if envCleanup.preserveBranches {
		cleaner, ok := s.worktreeCleanup.(WorktreeArchiveBatchCleaner)
		if !ok {
			return append(errs, errors.New("worktree cleaner cannot preserve branches during archive cleanup"))
		}
		cleanupErr = cleaner.CleanupWorktreesPreservingBranches(ctx, worktrees)
	} else if cleaner, ok := s.worktreeCleanup.(WorktreeBatchCleaner); ok {
		cleanupErr = cleaner.CleanupWorktrees(ctx, worktrees)
	}
	if cleanupErr != nil {
		s.logger.Warn("failed to cleanup task worktrees",
			zap.String("task_id", taskID),
			zap.Error(cleanupErr))
		errs = append(errs, fmt.Errorf("cleanup worktrees: %w", cleanupErr))
	}
	return errs
}

func (s *Service) filterSharedWorktreesForTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
) ([]*worktree.Worktree, []error) {
	guard, ok := s.worktreeCleanup.(worktreeReferenceGuard)
	if !ok || len(worktrees) == 0 {
		return worktrees, nil
	}
	excludedSessionIDs := taskSessionIDs(sessions)
	filtered := worktrees[:0]
	var errs []error
	for _, wt := range worktrees {
		count, err := guard.CountActiveWorktreeReferences(ctx, wt.ID, excludedSessionIDs)
		if err != nil {
			errs = append(errs, fmt.Errorf("count active references for worktree %s: %w", wt.ID, err))
			continue
		}
		if count == 0 {
			filtered = append(filtered, wt)
			continue
		}
		// A worktree borrowed by another task is protected by the single
		// environment-repository row it shares. There is no per-session
		// reference to release: marking the row deleted would destroy the
		// borrower's workspace. The environment ownership transfer handled
		// earlier in the flow re-homes the row to the borrower.
		s.logger.Info("preserving worktree still referenced by another active task",
			zap.String("task_id", taskID),
			zap.String("worktree_id", wt.ID),
			zap.Int("active_references", count))
	}
	return filtered, errs
}

func taskSessionIDs(sessions []*models.TaskSession) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session != nil && session.ID != "" {
			ids = append(ids, session.ID)
		}
	}
	return ids
}

func taskEnvironmentID(env *models.TaskEnvironment) string {
	if env == nil {
		return ""
	}
	return env.ID
}

func (s *Service) hasActiveOtherTaskSessionsForEnvironment(ctx context.Context, taskID string, env *models.TaskEnvironment) (bool, error) {
	if env == nil || env.ID == "" || s.sessions == nil {
		return false, nil
	}
	checker, ok := s.sessions.(taskEnvironmentSessionUsageChecker)
	if !ok {
		return false, nil
	}
	return checker.HasActiveTaskSessionsByTaskEnvironmentExcludingTask(ctx, env.ID, taskID)
}

func (s *Service) preserveTaskEnvironmentForActiveBorrower(ctx context.Context, taskID string, env *models.TaskEnvironment) (bool, error) {
	if env == nil || env.ID == "" || s.sessions == nil {
		return false, nil
	}
	finder, ok := s.sessions.(taskEnvironmentSessionBorrowerFinder)
	if !ok {
		return false, nil
	}
	borrowerTaskID, err := finder.FindActiveTaskSessionTaskIDByTaskEnvironmentExcludingTask(ctx, env.ID, taskID)
	if err != nil {
		return false, fmt.Errorf("find task environment borrower %s: %w", env.ID, err)
	}
	if borrowerTaskID == "" {
		return false, nil
	}
	ownerTransfer, ok := s.taskEnvironments.(taskEnvironmentOwnerTransferer)
	if !ok {
		return false, fmt.Errorf("task environment repository cannot transfer borrowed environment %s", env.ID)
	}
	if err := ownerTransfer.TransferTaskEnvironmentToTask(ctx, env.ID, borrowerTaskID); err != nil {
		return false, fmt.Errorf("transfer task environment %s to %s: %w", env.ID, borrowerTaskID, err)
	}
	env.TaskID = borrowerTaskID
	return true, nil
}

func (s *Service) filterOwnedWorktreesForTaskCleanup(
	ctx context.Context,
	taskID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	ownedEnv *models.TaskEnvironment,
	skipOwnedEnvironment bool,
) []*worktree.Worktree {
	if len(worktrees) == 0 {
		return worktrees
	}
	bySession := make(map[string]*models.TaskSession, len(sessions))
	for _, sess := range sessions {
		if sess != nil && sess.ID != "" {
			bySession[sess.ID] = sess
		}
	}
	envCache := map[string]*models.TaskEnvironment{}
	filtered := worktrees[:0]
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		sess, sessionLoaded := bySession[wt.SessionID]
		if s.taskOwnsSessionWorktree(ctx, taskID, wt.SessionID, sess, sessionLoaded, ownedEnv, skipOwnedEnvironment, envCache) {
			filtered = append(filtered, wt)
		}
	}
	return filtered
}

func (s *Service) taskOwnsSessionWorktree(
	ctx context.Context,
	taskID string,
	sessionID string,
	session *models.TaskSession,
	sessionLoaded bool,
	ownedEnv *models.TaskEnvironment,
	skipOwnedEnvironment bool,
	envCache map[string]*models.TaskEnvironment,
) bool {
	if session == nil {
		if sessionID == "" {
			return true
		}
		if !sessionLoaded {
			s.logger.Warn("skipping task worktree cleanup because session ownership cannot be checked",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID))
		}
		return false
	}
	if session.TaskEnvironmentID == "" {
		return true
	}
	if ownedEnv != nil && session.TaskEnvironmentID == ownedEnv.ID {
		return !skipOwnedEnvironment
	}
	if s.taskEnvironments == nil {
		s.logger.Warn("skipping task worktree cleanup because task environment ownership cannot be checked",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("task_environment_id", session.TaskEnvironmentID))
		return false
	}
	env, ok := envCache[session.TaskEnvironmentID]
	if !ok {
		var err error
		env, err = s.taskEnvironments.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
		if err != nil {
			s.logger.Warn("skipping task worktree cleanup because task environment lookup failed",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.String("task_environment_id", session.TaskEnvironmentID),
				zap.Error(err))
			return false
		}
		envCache[session.TaskEnvironmentID] = env
	}
	if env == nil || env.TaskID != taskID {
		return false
	}
	return true
}

func cleanupEligibleWorktrees(worktrees []*worktree.Worktree, env *models.TaskEnvironment, preserveExecutorRows map[string]struct{}) []*worktree.Worktree {
	if len(preserveExecutorRows) == 0 || len(worktrees) == 0 {
		return worktrees
	}
	filtered := worktrees[:0]
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if _, preserve := preserveExecutorRows[wt.SessionID]; preserve {
			continue
		}
		filtered = append(filtered, wt)
	}
	return filtered
}

func (s *Service) cleanupTaskEnvironment(
	ctx context.Context,
	taskID string,
	cleanup taskEnvironmentCleanup,
) []error {
	if cleanup.env == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return []error{cause}
	}
	if err := s.teardownEnvironmentResources(ctx, cleanup.env); err != nil {
		s.logger.Warn("failed to teardown task environment during task cleanup",
			zap.String("task_id", taskID),
			zap.String("env_id", cleanup.env.ID),
			zap.Error(err))
		return []error{fmt.Errorf("teardown task environment %s: %w", cleanup.env.ID, err)}
	}
	if cleanup.deleteRow {
		if cause := context.Cause(ctx); cause != nil {
			return []error{cause}
		}
		if err := s.taskEnvironments.DeleteTaskEnvironment(ctx, cleanup.env.ID); err != nil &&
			!errors.Is(err, taskrepo.ErrTaskEnvironmentNotFound) {
			s.logger.Warn("failed to delete task environment row during task cleanup",
				zap.String("task_id", taskID),
				zap.String("env_id", cleanup.env.ID),
				zap.Error(err))
			return []error{fmt.Errorf("delete task environment row %s: %w", cleanup.env.ID, err)}
		}
	}
	return nil
}

// ListTasks returns all tasks for a workflow
func (s *Service) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	if err := s.authorizeWorkflowID(ctx, workflowID); err != nil {
		return nil, err
	}
	workflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if workflow == nil {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	tasks, err := s.tasks.ListTasks(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	tasks = filterTasksByWorkspace(tasks, workflow.WorkspaceID)

	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}
	s.hydrateTaskWorkspaceFoldersBatch(ctx, tasks)

	return tasks, nil
}

func filterTasksByWorkspace(tasks []*models.Task, workspaceID string) []*models.Task {
	filtered := tasks[:0]
	for _, task := range tasks {
		if task != nil && task.WorkspaceID == workspaceID {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

type workspaceArchiveModeLister interface {
	ListTasksByWorkspaceWithArchiveMode(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived bool) ([]*models.Task, int, error)
}

func listTasksByWorkspaceWithArchiveMode(repo taskrepo.TaskRepository, ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived bool) ([]*models.Task, int, error) {
	if lister, ok := repo.(workspaceArchiveModeLister); ok {
		return lister.ListTasksByWorkspaceWithArchiveMode(ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived)
	}
	if onlyArchived {
		return nil, 0, fmt.Errorf("archived-only workspace task listing is unavailable")
	}
	return repo.ListTasksByWorkspace(ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig)
}

// ListTasksByWorkspace returns paginated tasks for a workspace with task repositories loaded.
// If query is non-empty, filters by task title, description, repository name, or repository path.
// workflowID and repositoryID, when non-empty, further restrict results to that workflow/repository.
func (s *Service) ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error) {
	return s.ListTasksByWorkspaceWithArchiveMode(ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, false)
}

// ListTasksByWorkspaceWithArchiveMode is the additive workspace-list contract
// used by the sidebar archive view. onlyArchived takes precedence over
// includeArchived when both are true.
func (s *Service) ListTasksByWorkspaceWithArchiveMode(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, sort string, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived bool) ([]*models.Task, int, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, 0, err
	}
	tasks, total, err := listTasksByWorkspaceWithArchiveMode(s.tasks, ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived)
	if err != nil {
		return nil, 0, err
	}

	tasks, total = s.augmentWithPRMatches(ctx, tasks, total, prSearchOptions{
		workspaceID:      workspaceID,
		workflowID:       workflowID,
		repositoryID:     repositoryID,
		query:            query,
		page:             page,
		pageSize:         pageSize,
		includeArchived:  includeArchived,
		onlyArchived:     onlyArchived,
		includeEphemeral: includeEphemeral,
		onlyEphemeral:    onlyEphemeral,
		excludeConfig:    excludeConfig,
	})

	if err := s.loadTaskRepositoriesBatch(ctx, tasks); err != nil {
		s.logger.Error("failed to batch-load task repositories", zap.Error(err))
	}
	s.hydrateTaskWorkspaceFoldersBatch(ctx, tasks)

	return tasks, total, nil
}

type prSearchOptions struct {
	workspaceID      string
	workflowID       string
	repositoryID     string
	query            string
	page             int
	pageSize         int
	includeArchived  bool
	onlyArchived     bool
	includeEphemeral bool
	onlyEphemeral    bool
	excludeConfig    bool
}

// parsePRQuery extracts a positive PR number from a search query, accepting an
// optional leading '#'. Returns (0, false) when the query is not a PR number.
func parsePRQuery(query string) (int, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(query), "#")
	if trimmed == "" {
		return 0, false
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// isConfigTask reports whether a task is a config-mode task (mirrors the SQL
// `json_extract(metadata, '$.config_mode') IS NOT 1` filter). JSON-decoded
// numbers arrive as float64, so accept both numeric 1 and bool true.
func isConfigTask(task *models.Task) bool {
	return isConfigTaskMetadata(task.Metadata)
}

func isConfigTaskMetadata(metadata map[string]interface{}) bool {
	switch v := metadata["config_mode"].(type) {
	case float64:
		return v == 1
	case int:
		return v == 1
	case bool:
		return v
	default:
		return false
	}
}

// augmentWithPRMatches surfaces tasks associated with a PR number when the
// search query looks like one. PR matches are prepended (most relevant) and
// deduped against the existing results; `total` grows by the net-new count.
// Best-effort: a missing resolver or a lookup error leaves results unchanged.
//
// Augmentation only applies to the first page of an unscoped search. It is
// skipped for page > 1 (the prepend+truncate only makes sense against page 1,
// otherwise a PR match would re-appear on every page and push out a real
// result) and when a workflow or repository filter is set (a PR-matched task
// isn't guaranteed to satisfy those filters, and the only caller that searches
// by PR number — the Cmd+K command panel — sets neither).
func (s *Service) augmentWithPRMatches(ctx context.Context, tasks []*models.Task, total int, opts prSearchOptions) ([]*models.Task, int) {
	if opts.page > 1 || opts.workflowID != "" || opts.repositoryID != "" {
		return tasks, total
	}
	prNum, ok := parsePRQuery(opts.query)
	if !ok || s.prTaskResolver == nil {
		return tasks, total
	}
	ids, err := s.prTaskResolver.FindTaskIDsByPRNumber(ctx, opts.workspaceID, prNum)
	if err != nil {
		s.logger.Warn("PR-number task lookup failed", zap.Int("pr_number", prNum), zap.Error(err))
		return tasks, total
	}

	matched := s.fetchPRMatchedTasks(ctx, ids, tasks, opts)
	if len(matched) == 0 {
		return tasks, total
	}

	merged := make([]*models.Task, 0, len(matched)+len(tasks))
	merged = append(merged, matched...)
	merged = append(merged, tasks...)
	total += len(matched)
	if len(merged) > opts.pageSize {
		merged = merged[:opts.pageSize]
	}
	return merged, total
}

// fetchPRMatchedTasks batch-loads the resolver's task IDs that aren't already in
// `existing`, applies the same visibility filters as the repository search, and
// returns the survivors in resolver order. The resolver returns distinct IDs,
// so excluding the already-present ones is enough to keep the result deduped.
func (s *Service) fetchPRMatchedTasks(ctx context.Context, ids []string, existing []*models.Task, opts prSearchOptions) []*models.Task {
	seen := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		seen[t.ID] = struct{}{}
	}
	var fetchIDs []string
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			fetchIDs = append(fetchIDs, id)
		}
	}
	if len(fetchIDs) == 0 {
		return nil
	}
	fetched, err := s.tasks.GetTasksByIDs(ctx, fetchIDs)
	if err != nil {
		s.logger.Warn("PR-match task fetch failed", zap.Error(err))
		return nil
	}
	byID := make(map[string]*models.Task, len(fetched))
	for _, t := range fetched {
		byID[t.ID] = t
	}
	var matched []*models.Task
	for _, id := range fetchIDs {
		task := byID[id]
		if task == nil || s.prMatchFilteredOut(task, opts) {
			continue
		}
		matched = append(matched, task)
	}
	return matched
}

// prMatchFilteredOut applies the same visibility filters the repository search
// uses, so a PR-matched task respects includeArchived / ephemeral / config flags.
func (s *Service) prMatchFilteredOut(task *models.Task, opts prSearchOptions) bool {
	if opts.onlyArchived {
		if task.ArchivedAt == nil {
			return true
		}
	} else if !opts.includeArchived && task.ArchivedAt != nil {
		return true
	}
	if opts.onlyEphemeral && !task.IsEphemeral {
		return true
	}
	if !opts.includeEphemeral && !opts.onlyEphemeral && task.IsEphemeral {
		return true
	}
	if opts.excludeConfig && isConfigTask(task) {
		return true
	}
	return false
}

// loadTaskRepositoriesBatch loads repositories for multiple tasks in a single query.
func (s *Service) loadTaskRepositoriesBatch(ctx context.Context, tasks []*models.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	repoMap, err := s.taskRepos.ListTaskRepositoriesByTaskIDs(ctx, taskIDs)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		task.Repositories = repoMap[task.ID]
	}
	return nil
}
