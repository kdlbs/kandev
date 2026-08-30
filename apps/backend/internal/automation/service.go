package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// ErrTaskNotFound is the sentinel that run-cleanup paths check to distinguish
// "the task is already gone — fine, drop the run row anyway" from a real
// upstream failure. TaskDeleter implementations should wrap this when the
// task domain reports a missing row, so this package can recognize the case
// via errors.Is without importing the task repository package (see
// backendapp's taskDeleterAdapter for the production wiring).
var ErrTaskNotFound = errors.New("automation: task not found for cleanup")

// ErrRepositoryNotInWorkspace is returned when a submitted repository_ids
// entry doesn't resolve to a repository belonging to the automation's
// workspace (including a nonexistent repository ID).
var ErrRepositoryNotInWorkspace = errors.New("automation: repository does not belong to this workspace")

// ErrDuplicateRepositoryID is returned when repository_ids contains the
// same ID more than once.
var ErrDuplicateRepositoryID = errors.New("automation: duplicate repository_ids entry")

// ErrRepositoryLookupUnavailable is returned when repository_ids is
// non-empty but no RepositoryLookup has been wired via
// SetRepositoryLookup. Fails closed rather than skipping validation,
// because this check guards cross-workspace repository access.
var ErrRepositoryLookupUnavailable = errors.New("automation: repository validation is not available")

// ErrAutomationNotFound covers both a missing automation and one in a workspace
// the caller cannot reach, so authorization never leaks which of the two it is.
var ErrAutomationNotFound = errors.New("automation: not found")

// ErrAgentProfileNotFound is returned when a create or update names an agent
// profile ID that does not resolve to a live profile row.
//
// Deleting a profile disables the automations bound to it before the row goes
// away, which only ever covered bindings that were valid to begin with. A
// request naming a profile that never existed — or one deleted long enough ago
// that nothing remembers it — sailed straight past that, and the result is the
// exact shape the delete ordering was built to prevent: an enabled automation
// pointed at a profile that isn't there, failing quietly on a schedule.
var ErrAgentProfileNotFound = errors.New("automation: agent profile not found")

var ErrInvalidContinuationPolicy = errors.New("automation: invalid continuation policy")

// ErrAutomationRunNotDispatchable means the run was stopped or otherwise
// settled before its agent turn could be started. The event handler must not
// launch work for this run.
var ErrAutomationRunNotDispatchable = errors.New("automation: run is not dispatchable")

// ErrAutomationRunNotLive is returned by the orchestrator when an exact run
// binding is no longer the live turn. The service maps that outcome to the
// same not-found result as a missing or terminal run, so a stale stop request
// cannot affect a newer turn in a shared session.
var ErrAutomationRunNotLive = errors.New("automation: run is not live")

// RunStopper cancels one exact task/session/turn binding. The bool is false
// when the binding is already terminal or stale; that is not an internal
// failure and must not cancel a successor turn.
type RunStopper interface {
	StopAutomationRun(ctx context.Context, taskID, sessionID, turnID string) (bool, error)
}

// RunLivenessChecker answers whether an exact bound turn is still live or
// blocked. Errors fail closed and leave the run open for a later retry.
type RunLivenessChecker interface {
	AutomationRunLive(ctx context.Context, taskID, sessionID, turnID string) (bool, error)
}

// RunDispatch is the exact identity returned after an agent turn is accepted.
// The service binds it while holding the automation run lock, so stopping or
// deleting an automation cannot race the task/session/turn admission.
type RunDispatch struct {
	TaskID    string
	SessionID string
	TurnID    string
}

// RunDispatcher serializes accepted-turn dispatch with exact-run stop and
// run-history deletion.
type RunDispatcher interface {
	DispatchRun(
		ctx context.Context,
		runID string,
		action ThreadAction,
		reason string,
		dispatch func() (RunDispatch, error),
	) error
}

func validateContinuationSettings(policy ContinuationPolicy, maxRuns int) error {
	if policy == "" {
		policy = ContinuationPolicyNewTask
	}
	if policy != ContinuationPolicyNewTask && policy != ContinuationPolicyReuseThread {
		return fmt.Errorf("%w: %q", ErrInvalidContinuationPolicy, policy)
	}
	if policy == ContinuationPolicyReuseThread && maxRuns != 1 {
		return fmt.Errorf("reuse_thread requires max_concurrent_runs = 1")
	}
	return nil
}

// TaskDeleter deletes a task and cleans up its resources.
// Satisfied by *taskservice.Service; injected to avoid a cyclic import.
// Implementations should return errors wrapping ErrTaskNotFound when the
// task is already gone.
type TaskDeleter interface {
	DeleteTask(ctx context.Context, id string) error
}

// WorkflowLocator resolves the workspace a workflow belongs to. Satisfied by
// the task service; injected to avoid a cyclic import, like TaskDeleter.
//
// Without it the server accepts any non-empty workflow id, so a request naming
// another workspace's workflow is stored verbatim. The editor no longer offers
// those, but a UI filter is not an authorization boundary.
type WorkflowLocator interface {
	WorkflowWorkspaceID(ctx context.Context, workflowID string) (string, error)
}

// WorkflowStepLocator verifies that a workflow step belongs to a workflow in
// the same workspace. It is separate from WorkflowLocator because the task and
// workflow services own these two records in different repositories.
type WorkflowStepLocator interface {
	WorkflowStepBelongs(ctx context.Context, workspaceID, workflowID, stepID string) (bool, error)
}

// AgentProfileLookup answers whether an agent profile ID resolves to a live
// (not soft-deleted) profile row. Satisfied by an adapter over the agent
// settings store and injected rather than imported, for the same reason
// TaskDeleter and WorkflowLocator are: the agent settings controller already
// reaches into this package to disable automations before a profile delete, so
// importing it back here would close the cycle.
//
// Existence only, deliberately. Agent profiles are either workspace-scoped or
// global (an empty workspace_id), and an automation's binding has never been
// checked against either, so answering the narrower "does this row exist" is
// what closes the defect without retroactively invalidating bindings that are
// merely cross-workspace.
//
// The (bool, error) split is load-bearing: a definitive "no such profile" is
// what this rejects, a driver failure is not, and collapsing the two would let
// one flaky read reject a perfectly good binding.
type AgentProfileLookup interface {
	AgentProfileExists(ctx context.Context, profileID string) (bool, error)
}

// Service coordinates automation operations.
type Service struct {
	store       *Store
	eventBus    bus.EventBus
	logger      *logger.Logger
	taskDeleter TaskDeleter // optional; nil-safe
	runStopper  RunStopper  // optional; wired by the orchestrator composition
	runLiveness RunLivenessChecker
	// workflowLocator gates workflow ownership. Optional: when nil (isolated
	// tests) ownership is not enforced.
	workflowLocator WorkflowLocator
	// workflowStepLocator gates the relationship between a workflow and its
	// selected step. Optional for isolated tests, but wired in production.
	workflowStepLocator WorkflowStepLocator

	// repoLookup validates repository_ids on create/update — every ID must
	// resolve to a repository belonging to the automation's workspace. Nil
	// = validation skipped (not yet wired at startup, or an isolated test).
	repoLookup RepositoryLookup

	// taskOriginLookup resolves a task's workspace and whether it is a hidden
	// automation run. It is used by the merged-PR subscriber and by cleanup so
	// visible automation-created tasks are never treated as disposable runs.
	// Nil = not wired; the github_pr_merged trigger type then never fires.
	taskOriginLookup TaskOriginLookup

	// agentProfileLookup validates agent_profile_id on create/update. Nil =
	// validation skipped, like workflowLocator above and unlike repoLookup —
	// see validateAgentProfileID for why this one does not fail closed.
	agentProfileLookup AgentProfileLookup

	// authorizeWorkspace gates automation access by workspace ownership
	// (opt-in auth). Nil = unscoped (internal schedulers/pollers, auth
	// disabled). Set via SetWorkspaceAuthorizer.
	authorizeWorkspace func(ctx context.Context, workspaceID string) error

	// exportAgentProfileLookup, exportExecutorProfileLookup,
	// exportWorkflowLookup, exportWorkflowStepLookup, and
	// exportRepositoryLookup resolve an automation's descriptor references
	// for the AC-29 YAML export. Unlike the validation-only lookups above,
	// nil is not "skip enforcement" — resolveDescriptors fails closed with
	// ErrExportLookupUnavailable for any reference it cannot resolve because
	// the matching lookup was never wired.
	exportAgentProfileLookup    ExportAgentProfileLookup
	exportExecutorProfileLookup ExportExecutorProfileLookup
	exportWorkflowLookup        ExportWorkflowLookup
	exportWorkflowStepLookup    ExportWorkflowStepLookup
	exportRepositoryLookup      ExportRepositoryLookup

	// exportWorkspaceLookup answers AC-44 step 2's workspace-existence check.
	// Nil is a construction error, not "skip enforcement": the endpoint
	// cannot honour AC-35 without it, so collectExportAutomations fails
	// closed with ErrExportWorkspaceLookupUnavailable rather than falling
	// through to a 200 for a workspace that may never have existed.
	exportWorkspaceLookup ExportWorkspaceLookup

	// runLocks serializes run creation (RecordRun, the concurrency-cap skip
	// insert) against DeleteAllRuns per automation ID. Without this, a run
	// created between DeleteAllRuns' task-id snapshot and its final row
	// purge would have its row deleted without its task ever reaching the
	// TaskDeleter — orphaning the task. Entries are never removed: growth is
	// bounded by the number of distinct automation IDs (~160 B per entry).
	runLocks sync.Map // automationID (string) -> *sync.Mutex
}

// NewService creates a new automation service.
func NewService(store *Store, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		store:    store,
		eventBus: eventBus,
		logger:   log,
	}
}

// Store returns the underlying store (for scheduler/poller access).
func (s *Service) Store() *Store {
	return s.store
}

// SetTaskDeleter wires the task deletion handler for run cleanup.
// Optional: when nil, run deletion skips task teardown.
func (s *Service) SetTaskDeleter(d TaskDeleter) {
	s.taskDeleter = d
}

// SetRunStopper wires exact automation-turn cancellation. It is kept as a
// narrow interface so the automation package does not depend on the
// orchestrator implementation.
func (s *Service) SetRunStopper(stopper RunStopper) {
	s.runStopper = stopper
}

// SetRunLivenessChecker wires restart reconciliation for exact bound turns.
func (s *Service) SetRunLivenessChecker(checker RunLivenessChecker) {
	s.runLiveness = checker
}

// SetWorkflowLocator wires the workflow ownership check.
func (s *Service) SetWorkflowLocator(l WorkflowLocator) {
	s.workflowLocator = l
}

// SetWorkflowStepLocator wires the workflow-step relationship check.
func (s *Service) SetWorkflowStepLocator(l WorkflowStepLocator) {
	s.workflowStepLocator = l
}

// authorizeWorkflowOwnership rejects a workflow belonging to a workspace other
// than the one the automation is being saved into.
func (s *Service) authorizeWorkflowOwnership(ctx context.Context, workspaceID, workflowID string) error {
	if s.workflowLocator == nil || workflowID == "" {
		return nil
	}
	owner, err := s.workflowLocator.WorkflowWorkspaceID(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("resolve workflow workspace: %w", err)
	}
	if owner != workspaceID {
		return fmt.Errorf("workflow does not belong to this workspace")
	}
	return nil
}

// authorizeWorkflowStepOwnership rejects a step that is not part of the
// selected workflow. A step without a workflow is never a valid binding.
func (s *Service) authorizeWorkflowStepOwnership(ctx context.Context, workspaceID, workflowID, stepID string) error {
	if stepID == "" {
		return nil
	}
	if workflowID == "" {
		return fmt.Errorf("workflow step requires a workflow")
	}
	if s.workflowStepLocator == nil {
		return nil
	}
	belongs, err := s.workflowStepLocator.WorkflowStepBelongs(ctx, workspaceID, workflowID, stepID)
	if err != nil {
		return fmt.Errorf("resolve workflow step: %w", err)
	}
	if !belongs {
		return fmt.Errorf("workflow step does not belong to this workflow")
	}
	return nil
}

// SetAgentProfileLookup wires the agent-profile existence check applied to
// agent_profile_id on create/update.
func (s *Service) SetAgentProfileLookup(l AgentProfileLookup) {
	s.agentProfileLookup = l
}

// validateAgentProfileID rejects a binding to an agent profile that is not
// there. Without it the only enforcement is executor.PrepareSession at launch
// time — long after the automation has been persisted, scheduled and fired —
// so the automation looks configured on screen and dies on every firing.
//
// Two skips, for different reasons:
//
//   - No lookup wired: skip, matching workflowLocator rather than repoLookup's
//     fail-closed. repoLookup guards cross-workspace repository access, so
//     "unconfigured" must not mean "unchecked" there. This check is referential
//     integrity over an ID that grants nothing, and it applies to nearly every
//     automation rather than only those carrying a repository list — so failing
//     closed would turn one missing wire into "no automation can be saved at
//     all", in a subsystem whose startup is explicitly non-fatal.
//
//   - Empty profileID: accepted, because it is a *different* defect and this is
//     not the change that should fix it. An empty ID is not an inherit-the-
//     default path — nothing on the firing path substitutes a workspace default,
//     and the launch fails with ErrNoAgentProfileID — but the editor's save
//     button still allows it and most of this package's tests construct
//     automations without one. Rejecting it here would refuse data the UI is
//     currently able to produce, which is a product decision, not a defect fix.
func (s *Service) validateAgentProfileID(ctx context.Context, profileID string) error {
	if s.agentProfileLookup == nil || profileID == "" {
		return nil
	}
	exists, err := s.agentProfileLookup.AgentProfileExists(ctx, profileID)
	if err != nil {
		// Surfaced, not swallowed into "missing": a driver failure must not be
		// reported to the user as a profile that does not exist.
		return fmt.Errorf("resolve agent profile %q: %w", profileID, err)
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrAgentProfileNotFound, profileID)
	}
	return nil
}

// SetWorkspaceAuthorizer wires the per-user workspace-access check (opt-in
// auth). The authorizer must return nil for contexts without a request
// identity (internal callers).
func (s *Service) SetWorkspaceAuthorizer(fn func(ctx context.Context, workspaceID string) error) {
	s.authorizeWorkspace = fn
}

// SetTaskOriginLookup wires the task workspace/origin resolver used by the
// github_pr_merged trigger subscriber and run cleanup. Must be called before
// Start, following the SetWorkflowLocator precedent.
func (s *Service) SetTaskOriginLookup(l TaskOriginLookup) {
	s.taskOriginLookup = l
}

// TaskOriginLookup returns the wired lookup (may be nil).
func (s *Service) TaskOriginLookup() TaskOriginLookup {
	return s.taskOriginLookup
}

// SetRepositoryLookup wires the repository ownership validator for
// repository bindings on create/update. This is a security control (prevents a
// crafted request attaching another workspace's repository), so an unset
// lookup fails closed for any non-empty repository list rather than silently
// skipping validation.
func (s *Service) SetRepositoryLookup(lookup RepositoryLookup) {
	s.repoLookup = lookup
}

// SetExportAgentProfileLookup wires the agent profile resolver the YAML
// export uses to build exportAgentProfile descriptors.
func (s *Service) SetExportAgentProfileLookup(l ExportAgentProfileLookup) {
	s.exportAgentProfileLookup = l
}

// SetExportExecutorProfileLookup wires the executor profile resolver the
// YAML export uses to build exportExecutorProfile descriptors.
func (s *Service) SetExportExecutorProfileLookup(l ExportExecutorProfileLookup) {
	s.exportExecutorProfileLookup = l
}

// SetExportWorkflowLookup wires the workflow resolver the YAML export uses
// to build exportWorkflow descriptors.
func (s *Service) SetExportWorkflowLookup(l ExportWorkflowLookup) {
	s.exportWorkflowLookup = l
}

// SetExportWorkflowStepLookup wires the workflow step resolver the YAML
// export uses to populate exportWorkflow.Step.
func (s *Service) SetExportWorkflowStepLookup(l ExportWorkflowStepLookup) {
	s.exportWorkflowStepLookup = l
}

// SetExportRepositoryLookup wires the repository resolver the YAML export
// uses to resolve repository_ids to names.
func (s *Service) SetExportRepositoryLookup(l ExportRepositoryLookup) {
	s.exportRepositoryLookup = l
}

// SetExportWorkspaceLookup wires the workspace-existence check the YAML
// export uses for AC-44 step 2.
func (s *Service) SetExportWorkspaceLookup(l ExportWorkspaceLookup) {
	s.exportWorkspaceLookup = l
}

func (s *Service) authorizeWs(ctx context.Context, workspaceID string) error {
	if s.authorizeWorkspace == nil {
		return nil
	}
	return s.authorizeWorkspace(ctx, workspaceID)
}

// authorizeAutomation loads an automation and authorizes its workspace,
// returning ErrAutomationNotFound (via the store's not-found) for both a
// missing automation and a foreign one — no existence leak.
func (s *Service) authorizeAutomation(ctx context.Context, id string) error {
	if s.authorizeWorkspace == nil {
		return nil
	}
	a, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return err
	}
	// GetAutomation reports a missing row as (nil, nil), so a stale id — a
	// bookmarked page for a deleted automation, a client retrying after a
	// delete — reaches here with nothing to authorize. Dereferencing it panics
	// the backend on an ordinary not-found.
	if a == nil {
		return ErrAutomationNotFound
	}
	return s.authorizeWorkspace(ctx, a.WorkspaceID)
}

// --- Automation CRUD ---

// CreateAutomation creates an automation with its initial triggers.
func (s *Service) CreateAutomation(ctx context.Context, req *CreateAutomationRequest) (*Automation, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if err := s.authorizeWs(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}

	maxRuns := req.MaxConcurrentRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}
	continuationPolicy := req.ContinuationPolicy
	if continuationPolicy == "" {
		continuationPolicy = ContinuationPolicyNewTask
	}
	if err := validateContinuationSettings(continuationPolicy, maxRuns); err != nil {
		return nil, err
	}

	taskMode := req.TaskMode
	if taskMode == "" {
		taskMode = TaskModeAutomationRun
	}
	repositories, err := s.resolveAutomationRepositories(ctx, req.WorkspaceID, req.Repositories, req.RepositoryIDs)
	if err != nil {
		return nil, err
	}
	repositoryMode := RepositoryModeNone
	if len(repositories) > 0 {
		repositoryMode = RepositoryModeSelected
	}
	if req.RepositoryMode != "" && req.RepositoryMode != repositoryMode {
		return nil, fmt.Errorf("%w: %q", ErrInvalidRepositoryMode, req.RepositoryMode)
	}
	repositoryIDs := repositoryIDs(repositories)
	if err := validateAutomationTarget(taskMode, repositoryMode, req.WorkflowID, repositoryIDs); err != nil {
		return nil, err
	}
	// Hidden automation runs may omit a workflow. Visible normal tasks require
	// one, and when a workflow is supplied its ownership and optional starting
	// step are still checked.
	if err := s.authorizeWorkflowOwnership(ctx, req.WorkspaceID, req.WorkflowID); err != nil {
		return nil, err
	}
	if err := s.authorizeWorkflowStepOwnership(ctx, req.WorkspaceID, req.WorkflowID, req.WorkflowStepID); err != nil {
		return nil, err
	}
	a := &Automation{
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowID:         req.WorkflowID,
		WorkflowStepID:     req.WorkflowStepID,
		AgentProfileID:     req.AgentProfileID,
		ExecutorProfileID:  req.ExecutorProfileID,
		TaskMode:           taskMode,
		RepositoryMode:     repositoryMode,
		Repositories:       repositories,
		RepositoryIDs:      repositoryIDs,
		Prompt:             req.Prompt,
		TaskTitleTemplate:  req.TaskTitleTemplate,
		Enabled:            true,
		MaxConcurrentRuns:  maxRuns,
		ContinuationPolicy: continuationPolicy,
	}
	if err := s.validateAgentProfileID(ctx, req.AgentProfileID); err != nil {
		return nil, err
	}
	if err := s.store.CreateAutomation(ctx, a); err != nil {
		return nil, fmt.Errorf("create automation: %w", err)
	}

	// Create initial triggers. The cron check is the same one AddTrigger and
	// UpdateTrigger apply: without it an expression the scheduler cannot parse
	// is accepted at creation and rejected on the first edit, and in between the
	// automation simply never fires with nothing on screen to say why.
	for _, ts := range req.Triggers {
		if err := validateScheduledConfig(ts.Type, ts.Config); err != nil {
			return nil, err
		}
		t := &AutomationTrigger{
			AutomationID: a.ID,
			Type:         ts.Type,
			Config:       ts.Config,
			Enabled:      ts.Enabled,
		}
		if err := s.store.CreateTrigger(ctx, t); err != nil {
			s.logger.Error("failed to create trigger during automation creation",
				zap.String("automation_id", a.ID),
				zap.String("type", string(ts.Type)),
				zap.Error(err))
		}
	}

	return s.store.GetAutomation(ctx, a.ID)
}

// GetAutomation retrieves an automation by ID.
func (s *Service) GetAutomation(ctx context.Context, id string) (*Automation, error) {
	if err := s.authorizeAutomation(ctx, id); err != nil {
		return nil, err
	}
	return s.store.GetAutomation(ctx, id)
}

// ListAutomations returns all automations for a workspace.
func (s *Service) ListAutomations(ctx context.Context, workspaceID string) ([]*Automation, error) {
	if err := s.authorizeWs(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListAutomations(ctx, workspaceID)
}

// UpdateAutomation applies partial updates.
func (s *Service) UpdateAutomation(ctx context.Context, id string, req *UpdateAutomationRequest) (*Automation, error) {
	if err := s.authorizeAutomation(ctx, id); err != nil {
		return nil, err
	}
	// Checked here rather than inside authorizeUpdatedReferences because,
	// unlike the workflow and repository checks there, profile existence is not
	// asked relative to the automation's workspace and so needs no stored row
	// to answer. Rebinding is the path that matters most: an automation is
	// edited far more often than it is created, and a profile that has since
	// been deleted is still offered by any stale editor tab.
	if req.AgentProfileID != nil {
		if err := s.validateAgentProfileID(ctx, *req.AgentProfileID); err != nil {
			return nil, err
		}
	}
	if err := s.authorizeUpdatedReferences(ctx, id, req); err != nil {
		return nil, err
	}
	existing, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrAutomationNotFound
	}
	taskMode := existing.TaskMode
	if taskMode == "" {
		taskMode = TaskModeAutomationRun
	}
	if req.TaskMode != nil {
		taskMode = *req.TaskMode
	}
	repositories := existing.Repositories
	if req.Repositories != nil || req.RepositoryIDs != nil {
		repositories, err = s.resolveAutomationRepositories(
			ctx, existing.WorkspaceID, req.Repositories, req.RepositoryIDs,
		)
		if err != nil {
			return nil, err
		}
	}
	if req.RepositoryMode != nil {
		switch *req.RepositoryMode {
		case RepositoryModeNone:
			repositories = nil
		case RepositoryModeWorkspaceDefault:
			return nil, fmt.Errorf("%w: %q", ErrInvalidRepositoryMode, *req.RepositoryMode)
		}
	}
	repositoryMode := RepositoryModeNone
	if len(repositories) > 0 {
		repositoryMode = RepositoryModeSelected
	}
	if req.RepositoryMode != nil && *req.RepositoryMode != repositoryMode {
		return nil, fmt.Errorf("%w: %q", ErrInvalidRepositoryMode, *req.RepositoryMode)
	}
	workflowID := existing.WorkflowID
	if req.WorkflowID != nil {
		workflowID = *req.WorkflowID
	}
	repositoryIDs := repositoryIDs(repositories)
	if err := validateAutomationTarget(taskMode, repositoryMode, workflowID, repositoryIDs); err != nil {
		return nil, err
	}
	policy := existing.ContinuationPolicy
	if policy == "" {
		policy = ContinuationPolicyNewTask
	}
	maxRuns := existing.MaxConcurrentRuns
	if req.ContinuationPolicy != nil {
		policy = *req.ContinuationPolicy
	}
	if req.MaxConcurrentRuns != nil {
		maxRuns = *req.MaxConcurrentRuns
	}
	storeReq := req
	if policy == ContinuationPolicyReuseThread && maxRuns <= 0 {
		maxRuns = 1
		normalized := 1
		clone := *req
		clone.MaxConcurrentRuns = &normalized
		storeReq = &clone
	}
	if err := validateContinuationSettings(policy, maxRuns); err != nil {
		return nil, err
	}
	if req.Repositories != nil || req.RepositoryIDs != nil || req.RepositoryMode != nil {
		clone := *storeReq
		clone.Repositories = repositories
		clone.RepositoryIDs = nil
		clone.RepositoryMode = &repositoryMode
		storeReq = &clone
	}
	if err := s.store.UpdateAutomation(ctx, id, storeReq); err != nil {
		return nil, err
	}
	return s.store.GetAutomation(ctx, id)
}

// authorizeUpdatedReferences checks the fields of req that name something the
// automation's workspace must own — its repositories, workflow, and workflow
// step. All three need the stored automation to learn that workspace, so it is
// loaded once here rather than by each check.
func (s *Service) authorizeUpdatedReferences(ctx context.Context, id string, req *UpdateAutomationRequest) error {
	if req.Repositories == nil && req.RepositoryIDs == nil && req.WorkflowID == nil && req.WorkflowStepID == nil {
		return nil
	}
	existing, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("automation not found: %s", id)
	}
	if req.Repositories != nil || req.RepositoryIDs != nil {
		if _, err := s.resolveAutomationRepositories(
			ctx, existing.WorkspaceID, req.Repositories, req.RepositoryIDs,
		); err != nil {
			return err
		}
	}
	if req.WorkflowID == nil && req.WorkflowStepID == nil {
		return nil
	}
	workflowID := existing.WorkflowID
	if req.WorkflowID != nil {
		workflowID = *req.WorkflowID
	}
	if err := s.authorizeWorkflowOwnership(ctx, existing.WorkspaceID, workflowID); err != nil {
		return err
	}
	stepID := existing.WorkflowStepID
	if req.WorkflowStepID != nil {
		stepID = *req.WorkflowStepID
	}
	return s.authorizeWorkflowStepOwnership(ctx, existing.WorkspaceID, workflowID, stepID)
}

func (s *Service) resolveAutomationRepositories(
	ctx context.Context,
	workspaceID string,
	repositories []AutomationRepository,
	legacyIDs []string,
) ([]AutomationRepository, error) {
	if repositories == nil {
		repositories = repositoriesFromIDs(legacyIDs)
	}
	if len(repositories) == 0 {
		return []AutomationRepository{}, nil
	}
	if s.repoLookup == nil {
		return nil, ErrRepositoryLookupUnavailable
	}
	result := make([]AutomationRepository, 0, len(repositories))
	seen := make(map[string]bool, len(repositories))
	for _, repository := range repositories {
		if seen[repository.RepositoryID] {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateRepositoryID, repository.RepositoryID)
		}
		seen[repository.RepositoryID] = true
		repoWorkspaceID, defaultBranch, ok := s.repoLookup.GetRepository(ctx, repository.RepositoryID)
		if !ok || repoWorkspaceID != workspaceID {
			return nil, fmt.Errorf("%w: %s", ErrRepositoryNotInWorkspace, repository.RepositoryID)
		}
		if strings.TrimSpace(repository.BaseBranch) == "" {
			repository.BaseBranch = defaultBranch
		}
		if strings.TrimSpace(repository.BaseBranch) == "" {
			return nil, fmt.Errorf("%w: repository %s requires a base branch", ErrInvalidRepositoryMode, repository.RepositoryID)
		}
		result = append(result, repository)
	}
	return result, nil
}

// DeleteAutomation removes an automation.
func (s *Service) DeleteAutomation(ctx context.Context, id string) error {
	if err := s.authorizeAutomation(ctx, id); err != nil {
		return err
	}
	unlock := s.automationRunLock(id)
	defer unlock()
	if err := s.stopOpenAutomationRuns(ctx, id); err != nil {
		return err
	}
	cleanupTaskIDs, err := s.hiddenAutomationTaskIDs(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.store.DeleteAutomationWithCleanup(ctx, id, cleanupTaskIDs); err != nil {
		return err
	}
	_ = s.ReconcileCleanupJobs(ctx)
	return nil
}

func (s *Service) hiddenAutomationTaskIDs(ctx context.Context, automationID string) ([]string, error) {
	taskIDs, err := s.store.ListAutomationTaskIDs(ctx, automationID)
	if err != nil {
		return nil, err
	}
	if s.taskOriginLookup == nil {
		return taskIDs, nil
	}
	hidden := make([]string, 0, len(taskIDs))
	seen := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		_, isAutomationRun, ok := s.taskOriginLookup.TaskWorkspaceAndAutomationOrigin(ctx, taskID)
		if ok && isAutomationRun {
			hidden = append(hidden, taskID)
		}
	}
	return hidden, nil
}

// DeleteAutomationsByWorkspace applies the same ownership and cleanup path as
// an individual automation deletion. Workspace reset must not bypass hidden
// task capture and leave reusable worktrees orphaned.
func (s *Service) DeleteAutomationsByWorkspace(ctx context.Context, workspaceID string) (int, error) {
	automations, err := s.store.ListAutomations(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, a := range automations {
		if a == nil {
			continue
		}
		if err := s.DeleteAutomation(ctx, a.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// stopOpenAutomationRuns quiesces live bound turns before their automation
// references are removed. An admitted row without a binding has no runtime
// to stop and is safely removed by the deletion transaction.
func (s *Service) stopOpenAutomationRuns(ctx context.Context, automationID string) error {
	runs, err := s.store.ListOpenRuns(ctx, automationID)
	if err != nil {
		return fmt.Errorf("list open automation runs: %w", err)
	}
	for _, run := range runs {
		if run == nil || run.TaskID == "" || run.SessionID == "" || run.TurnID == "" || s.runStopper == nil {
			continue
		}
		stopped, stopErr := s.runStopper.StopAutomationRun(ctx, run.TaskID, run.SessionID, run.TurnID)
		if stopErr != nil {
			return fmt.Errorf("stop automation run %s: %w", run.ID, stopErr)
		}
		if stopped {
			if markErr := s.store.MarkRunTerminal(ctx, run.ID, run.SessionID, run.TurnID, RunStatusFailed, "automation deleted"); markErr != nil {
				return fmt.Errorf("mark automation run %s failed: %w", run.ID, markErr)
			}
		}
	}
	return nil
}

// ReconcileOpenRuns settles rows left open by a process stop. Admission rows
// without a binding are never guessed into a task and fail immediately. Bound
// rows are settled only when the orchestrator confirms that their exact turn
// is no longer live or blocked.
func (s *Service) ReconcileOpenRuns(ctx context.Context) error {
	runs, err := s.store.ListAllOpenRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run == nil {
			continue
		}
		if run.TaskID == "" || run.SessionID == "" || run.TurnID == "" {
			if err := s.store.MarkRunTerminal(ctx, run.ID, "", "", RunStatusFailed, "backend stopped before the automation turn was bound"); err != nil {
				s.logger.Warn("failed to reconcile unbound automation run", zap.String("run_id", run.ID), zap.Error(err))
			}
			continue
		}
		if s.runLiveness == nil {
			continue
		}
		live, checkErr := s.runLiveness.AutomationRunLive(ctx, run.TaskID, run.SessionID, run.TurnID)
		if checkErr != nil {
			s.logger.Warn("failed to inspect automation run liveness", zap.String("run_id", run.ID), zap.Error(checkErr))
			continue
		}
		if !live {
			if err := s.store.MarkRunTerminal(ctx, run.ID, run.SessionID, run.TurnID, RunStatusFailed, "automation turn was stale after backend recovery"); err != nil {
				s.logger.Warn("failed to reconcile stale automation run", zap.String("run_id", run.ID), zap.Error(err))
			}
		}
	}
	return nil
}

// ReconcileCleanupJobs retries hidden-task cleanup left after automation
// deletion. A missing task is already clean; every other failure remains
// durable with a safe error for the next startup/reconciliation pass.
func (s *Service) ReconcileCleanupJobs(ctx context.Context) error {
	jobs, err := s.store.ListCleanupJobs(ctx)
	if err != nil {
		return err
	}
	if s.taskDeleter == nil {
		return nil
	}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		delErr := s.taskDeleter.DeleteTask(ctx, job.TaskID)
		if delErr == nil || errors.Is(delErr, ErrTaskNotFound) {
			if err := s.store.DeleteCleanupJob(ctx, job.TaskID); err != nil {
				s.logger.Warn("failed to remove completed automation cleanup job", zap.String("task_id", job.TaskID), zap.Error(err))
			}
			continue
		}
		if err := s.store.UpdateCleanupJobError(ctx, job.TaskID, delErr.Error()); err != nil {
			s.logger.Warn("failed to persist automation cleanup error", zap.String("task_id", job.TaskID), zap.Error(err))
		}
	}
	return nil
}

// StopRun cancels one open automation run. The stored binding is authoritative;
// callers never provide task, session, or turn identities themselves.
func (s *Service) StopRun(ctx context.Context, automationID, runID string) (*AutomationRun, error) {
	if err := s.authorizeAutomation(ctx, automationID); err != nil {
		return nil, err
	}
	unlock := s.automationRunLock(automationID)
	defer unlock()

	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	if run == nil || run.AutomationID != automationID ||
		(run.Status != RunStatusTriggered && run.Status != RunStatusTaskCreated) {
		return nil, ErrAutomationNotFound
	}
	if run.TaskID != "" && run.SessionID != "" && run.TurnID != "" {
		if s.runStopper == nil {
			return nil, errors.New("automation run stopper is not configured")
		}
		stopped, stopErr := s.runStopper.StopAutomationRun(ctx, run.TaskID, run.SessionID, run.TurnID)
		if stopErr != nil {
			return nil, stopErr
		}
		if !stopped {
			return nil, ErrAutomationNotFound
		}
	}
	if err := s.store.MarkRunTerminal(ctx, run.ID, run.SessionID, run.TurnID, RunStatusFailed, "stopped by user"); err != nil {
		return nil, err
	}
	run.Status = RunStatusFailed
	run.ErrorMessage = "stopped by user"
	return run, nil
}

// EnableAutomation sets enabled = true.
func (s *Service) EnableAutomation(ctx context.Context, id string) error {
	if err := s.authorizeAutomation(ctx, id); err != nil {
		return err
	}
	enabled := true
	return s.store.UpdateAutomation(ctx, id, &UpdateAutomationRequest{Enabled: &enabled})
}

// DisableAutomation sets enabled = false.
func (s *Service) DisableAutomation(ctx context.Context, id string) error {
	if err := s.authorizeAutomation(ctx, id); err != nil {
		return err
	}
	enabled := false
	return s.store.UpdateAutomation(ctx, id, &UpdateAutomationRequest{Enabled: &enabled})
}

// --- Trigger CRUD ---

// validateScheduledConfig rejects a cron expression the scheduler could never
// run. Without it the editor's regex is the only gate, and it is both too
// permissive (accepting "60 * * * *", "*/0 * * * *", reversed ranges like
// "10-5 * * * *") and too strict (rejecting named fields such as MON or JAN
// that robfig/cron accepts). Either way the user saves a schedule that then
// silently never fires. Parsing with the scheduler's own parser is the only
// definition of valid that matters here.
func validateScheduledConfig(triggerType TriggerType, raw json.RawMessage) error {
	if triggerType != TriggerTypeScheduled || len(raw) == 0 {
		return nil
	}
	var cfg ScheduledTriggerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("invalid scheduled trigger config: %w", err)
	}
	// An empty expression means "not scheduled yet", not invalid — the editor
	// writes it while the user is still choosing.
	if strings.TrimSpace(cfg.CronExpression) == "" {
		return nil
	}
	if _, err := nextCronFire(cfg.CronExpression, cfg.Timezone, time.Now().UTC()); err != nil {
		return fmt.Errorf("invalid schedule %q: %w", cfg.CronExpression, err)
	}
	return nil
}

// AddTrigger adds a trigger to an automation.
func (s *Service) AddTrigger(ctx context.Context, req *AddTriggerRequest) (*AutomationTrigger, error) {
	if req.AutomationID == "" {
		return nil, fmt.Errorf("automation_id is required")
	}
	if err := s.authorizeAutomation(ctx, req.AutomationID); err != nil {
		return nil, err
	}
	if err := validateScheduledConfig(req.Type, req.Config); err != nil {
		return nil, err
	}
	t := &AutomationTrigger{
		AutomationID: req.AutomationID,
		Type:         req.Type,
		Config:       req.Config,
		Enabled:      req.Enabled,
	}
	if err := s.store.CreateTrigger(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTrigger updates a trigger.
func (s *Service) UpdateTrigger(ctx context.Context, id string, req *UpdateTriggerRequest) error {
	if err := s.authorizeTrigger(ctx, id); err != nil {
		return err
	}
	existing, err := s.store.GetTrigger(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("trigger not found: %s", id)
	}
	if req.Config != nil {
		if err := validateScheduledConfig(existing.Type, *req.Config); err != nil {
			return err
		}
	}
	return s.store.UpdateTrigger(ctx, id, req)
}

// DeleteTrigger removes a trigger.
func (s *Service) DeleteTrigger(ctx context.Context, id string) error {
	if err := s.authorizeTrigger(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteTrigger(ctx, id)
}

// authorizeTrigger resolves a trigger's automation and authorizes its
// workspace.
func (s *Service) authorizeTrigger(ctx context.Context, triggerID string) error {
	if s.authorizeWorkspace == nil {
		return nil
	}
	automationID, err := s.store.GetTriggerAutomationID(ctx, triggerID)
	if err != nil {
		return err
	}
	return s.authorizeAutomation(ctx, automationID)
}

// --- Run queries ---

// ListRuns returns recent runs for an automation.
func (s *Service) ListRuns(ctx context.Context, automationID string, limit int) ([]*AutomationRun, error) {
	if err := s.authorizeAutomation(ctx, automationID); err != nil {
		return nil, err
	}
	return s.store.ListRuns(ctx, automationID, limit)
}

// ListWorkspaceRuns returns recent runs across every automation in the
// workspace. Authorization is on the workspace itself rather than
// per-automation (as ListRuns does), because the workspace is the whole
// scope of the query — every row it can return already belongs to it.
func (s *Service) ListWorkspaceRuns(ctx context.Context, workspaceID string, limit int) ([]*WorkspaceAutomationRun, error) {
	if err := s.authorizeWs(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListWorkspaceRuns(ctx, workspaceID, limit)
}

// ListAutomationSummaries returns one health summary per automation in the
// workspace. Authorized on the workspace for the same reason ListWorkspaceRuns
// is: every row it can return already belongs to that workspace.
func (s *Service) ListAutomationSummaries(ctx context.Context, workspaceID string) ([]*AutomationSummary, error) {
	if err := s.authorizeWs(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.ListAutomationSummaries(ctx, workspaceID)
}

// GetAutomationSummary returns one automation's summary, authorized on the
// automation itself. The detail page reads it for the same reason the list
// reads the workspace-wide set: its own run window is capped, so an open run
// older than that window would leave the page claiming nothing is in flight.
func (s *Service) GetAutomationSummary(ctx context.Context, automationID string) (*AutomationSummary, error) {
	if err := s.authorizeAutomation(ctx, automationID); err != nil {
		return nil, err
	}
	return s.store.GetAutomationSummary(ctx, automationID)
}

// GetRun returns a single run by ID, or nil if not found.
func (s *Service) GetRun(ctx context.Context, id string) (*AutomationRun, error) {
	run, err := s.store.GetRun(ctx, id)
	if err != nil || run == nil {
		return run, err
	}
	if err := s.authorizeAutomation(ctx, run.AutomationID); err != nil {
		return nil, err
	}
	return run, nil
}

// automationRunLock returns an unlock func for the per-automation mutex that
// serializes run creation (createRunLocked) against DeleteAllRuns.
func (s *Service) automationRunLock(automationID string) func() {
	v, _ := s.runLocks.LoadOrStore(automationID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// DispatchRun serializes the fallible agent dispatch with exact-run stop and
// deletion. The callback is invoked only while the admitted run is still
// open; its exact task/session/turn identity is bound before the lock is
// released, so a stop can never settle a different firing.
func (s *Service) DispatchRun(
	ctx context.Context,
	runID string,
	action ThreadAction,
	reason string,
	dispatch func() (RunDispatch, error),
) error {
	if runID == "" || dispatch == nil {
		return ErrAutomationRunNotDispatchable
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return ErrAutomationRunNotDispatchable
	}
	unlock := s.automationRunLock(run.AutomationID)
	defer unlock()

	run, err = s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil || run.Status != RunStatusTriggered {
		return ErrAutomationRunNotDispatchable
	}

	dispatchResult, err := dispatch()
	if err != nil {
		return s.markDispatchFailed(ctx, runID, err)
	}
	if dispatchResult.TaskID == "" || dispatchResult.SessionID == "" || dispatchResult.TurnID == "" {
		return s.markDispatchFailed(ctx, runID, errors.New("automation dispatch returned no exact identity"))
	}
	if err := s.store.BindRun(ctx, runID, dispatchResult.TaskID, dispatchResult.SessionID, dispatchResult.TurnID, action, reason); err != nil {
		return s.markDispatchFailed(ctx, runID, err)
	}
	return nil
}

func (s *Service) markDispatchFailed(ctx context.Context, runID string, dispatchErr error) error {
	if err := s.store.MarkRunTerminal(ctx, runID, "", "", RunStatusFailed, dispatchErr.Error()); err != nil {
		return fmt.Errorf("%w (mark run failed: %v)", dispatchErr, err)
	}
	return dispatchErr
}

// lockRun returns the per-automation lock for a persisted run. Binding uses
// the same lock as DispatchRun and StopRun, while the store's status guard
// remains the final protection against a row deleted between the lookup and
// the update.
func (s *Service) lockRun(ctx context.Context, runID string) (func(), error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, ErrAutomationRunNotDispatchable
	}
	return s.automationRunLock(run.AutomationID), nil
}

// createRunLocked persists a run row while holding the per-automation lock
// that DeleteAllRuns also acquires. Without this, a run created between
// DeleteAllRuns' task-id snapshot and its final row purge would be deleted
// without its task ever reaching the TaskDeleter, orphaning the task.
func (s *Service) createRunLocked(ctx context.Context, run *AutomationRun) error {
	defer s.automationRunLock(run.AutomationID)()
	return s.store.CreateRun(ctx, run)
}

// DeleteRun removes a single run and its associated task (if any).
// Task deletion is best-effort: a not-found error is silently ignored so
// stale/orphaned run rows are always removable by the user.
func (s *Service) DeleteRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	if run == nil {
		return s.store.DeleteRun(ctx, runID)
	}
	// Authorized here rather than in the WS handler, like every other exported
	// operation on this service. A destructive call that depends on its caller
	// remembering to check is one new caller away from not being checked.
	if err := s.authorizeAutomation(ctx, run.AutomationID); err != nil {
		return err
	}
	unlock := s.automationRunLock(run.AutomationID)
	defer unlock()
	if err := s.deleteRunTaskIfUnreferenced(ctx, run); err != nil {
		return err
	}
	return s.store.DeleteRun(ctx, runID)
}

func (s *Service) deleteRunTaskIfUnreferenced(ctx context.Context, run *AutomationRun) error {
	if run.TaskID == "" || s.taskDeleter == nil {
		return nil
	}
	if s.taskOriginLookup != nil {
		_, isAutomationRun, ok := s.taskOriginLookup.TaskWorkspaceAndAutomationOrigin(ctx, run.TaskID)
		if ok && !isAutomationRun {
			return nil
		}
	}
	referenced, err := s.runTaskHasReferences(ctx, run)
	if err != nil {
		return err
	}
	if referenced {
		return nil
	}
	if err := s.taskDeleter.DeleteTask(ctx, run.TaskID); err != nil {
		if !errors.Is(err, ErrTaskNotFound) {
			return fmt.Errorf("delete task: %w", err)
		}
		s.logger.Debug("run task already gone, continuing delete",
			zap.String("run_id", run.ID), zap.String("task_id", run.TaskID))
	}
	return nil
}

func (s *Service) runTaskHasReferences(ctx context.Context, run *AutomationRun) (bool, error) {
	otherRun, err := s.store.IsTaskReferencedByRun(ctx, run.AutomationID, run.ID, run.TaskID)
	if err != nil {
		return false, fmt.Errorf("check run task references: %w", err)
	}
	if otherRun {
		return true, nil
	}
	continuation, err := s.store.IsContinuationTask(ctx, run.AutomationID, run.TaskID)
	if err != nil {
		return false, fmt.Errorf("check continuation task reference: %w", err)
	}
	if continuation {
		return true, nil
	}
	foreign, err := s.store.IsTaskReferencedByOtherAutomation(ctx, run.AutomationID, run.TaskID)
	if err != nil {
		return false, fmt.Errorf("check foreign task references: %w", err)
	}
	return foreign, nil
}

// DeleteAllRuns removes every run for an automation, deleting each associated
// hidden task first. Visible automation-created tasks belong to the normal
// task lifecycle and remain available after their run history is cleared.
// Task deletion is best-effort: not-found errors are ignored.
func (s *Service) DeleteAllRuns(ctx context.Context, automationID string) error {
	if err := s.authorizeAutomation(ctx, automationID); err != nil {
		return err
	}
	unlock := s.automationRunLock(automationID)
	defer unlock()
	taskIDs, err := s.store.ListRunTaskIDs(ctx, automationID)
	if err != nil {
		return fmt.Errorf("list run task ids: %w", err)
	}
	a, err := s.store.GetAutomation(ctx, automationID)
	if err != nil {
		return err
	}
	if a != nil && a.ContinuationTaskID != "" {
		taskIDs = append(taskIDs, a.ContinuationTaskID)
	}
	seen := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		if s.taskOriginLookup != nil {
			_, isAutomationRun, ok := s.taskOriginLookup.TaskWorkspaceAndAutomationOrigin(ctx, taskID)
			if ok && !isAutomationRun {
				continue
			}
		}
		foreign, refErr := s.store.IsTaskReferencedByOtherAutomation(ctx, automationID, taskID)
		if refErr != nil {
			return fmt.Errorf("check task %s references: %w", taskID, refErr)
		}
		if foreign || s.taskDeleter == nil {
			continue
		}
		if delErr := s.taskDeleter.DeleteTask(ctx, taskID); delErr != nil {
			if !errors.Is(delErr, ErrTaskNotFound) {
				return fmt.Errorf("delete task %s: %w", taskID, delErr)
			}
			s.logger.Debug("run task already gone, skipping",
				zap.String("automation_id", automationID),
				zap.String("task_id", taskID))
		}
	}
	return s.store.ClearContinuationAndDeleteAllRuns(ctx, automationID)
}

// --- Trigger firing ---

// FireResult reports what FireTrigger actually did. A skip is not an error:
// the trigger was evaluated and deliberately not run. Callers that report back
// to a human must distinguish the two, otherwise a deliberate skip is
// indistinguishable from a fire that happened.
type FireResult struct {
	Skipped bool
	// RunID identifies the admitted run when the trigger was accepted.
	RunID string
	// Reason is human-readable and set only when Skipped.
	Reason string
}

// RenderRunDisplayTitle resolves the title snapshot stored with a firing. It
// is evaluated before admission so later edits to the automation cannot change
// the title shown for an already-admitted run.
func RenderRunDisplayTitle(a *Automation, triggerType TriggerType, triggerData json.RawMessage) string {
	if a == nil {
		return ""
	}
	if a.TaskTitleTemplate != "" {
		if title := InterpolatePrompt(a.TaskTitleTemplate, triggerType, triggerData); title != "" {
			return taskservice.TruncateTaskTitle(title)
		}
	}
	if info := GetTriggerTypeInfo(triggerType); info != nil && info.DefaultTaskTitle != "" {
		if title := InterpolatePrompt(info.DefaultTaskTitle, triggerType, triggerData); title != "" {
			return taskservice.TruncateTaskTitle(title)
		}
	}
	return taskservice.TruncateTaskTitle(fmt.Sprintf("[Auto] %s", a.Name))
}

// FireTrigger publishes an AutomationTriggered event for the given trigger.
// The orchestrator handles task creation in response.
func (s *Service) FireTrigger(ctx context.Context, automationID, triggerID string, triggerType TriggerType, triggerData json.RawMessage, dedupKey string) (FireResult, error) {
	// Admission decisions live in one place so every caller — scheduler,
	// webhook, and the manual Run button — gets the same answer about whether a
	// fire actually happened.
	a, loadErr := s.store.GetAutomation(ctx, automationID)
	if loadErr != nil {
		return FireResult{}, fmt.Errorf("load automation: %w", loadErr)
	}
	if a == nil {
		return FireResult{Skipped: true, Reason: "automation no longer exists"}, nil
	}
	// The UI offers Run on a disabled automation. The orchestrator discards the
	// event downstream, so without this the caller is told a run started that
	// never could — and last_triggered_at moves for a run that never ran.
	if !a.Enabled {
		return FireResult{Skipped: true, Reason: "automation is disabled"}, nil
	}
	// Serialize deduplication, the concurrency admission check, and the run
	// insert. Otherwise two scheduler/webhook callers can both observe a free
	// slot and publish two fires, or DeleteAllRuns can remove a row after its
	// task snapshot but before the row is inserted.
	admittedRun, capReason, duplicate, admissionErr := s.admitTrigger(
		ctx, a, triggerID, triggerType, triggerData, dedupKey,
	)
	if admissionErr != nil {
		return FireResult{}, admissionErr
	}
	if duplicate {
		return FireResult{Skipped: true, Reason: "this trigger has already fired"}, nil
	}

	// Record that the trigger was evaluated now that the cap check itself
	// has succeeded. Scheduled triggers use this to pace themselves against
	// their cron interval (CronScheduler.shouldFire): if this were updated
	// before the cap check (or despite the cap check returning an error), an
	// infrastructural failure in the check would look like a completed
	// evaluation and suppress the next attempt until the full cron interval
	// elapses, instead of retrying on the next scheduler tick. And if it
	// were only updated on an actual fire, a trigger stuck behind
	// max_concurrent_runs would look "overdue" again on every subsequent
	// tick and get re-evaluated — and re-skipped — far more often than its
	// configured schedule.
	now := time.Now().UTC()
	if updateErr := s.store.UpdateTriggerEvaluatedAt(ctx, triggerID, now); updateErr != nil {
		s.logger.Warn("failed to update last_evaluated_at",
			zap.String("trigger_id", triggerID), zap.Error(updateErr))
	}
	if capReason != "" {
		return FireResult{Skipped: true, Reason: capReason}, nil
	}

	evt := &AutomationTriggeredEvent{
		AutomationID: automationID,
		RunID:        admittedRun.ID,
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		TriggerData:  triggerData,
		DedupKey:     dedupKey,
	}

	event := bus.NewEvent(events.AutomationTriggered, "automation_service", evt)
	if err := s.eventBus.Publish(ctx, events.AutomationTriggered, event); err != nil {
		if markErr := s.store.MarkRunTerminal(ctx, admittedRun.ID, "", "", RunStatusFailed, err.Error()); markErr != nil {
			return FireResult{}, fmt.Errorf("publish automation triggered: %w; mark admitted run failed: %v", err, markErr)
		}
		return FireResult{}, fmt.Errorf("publish automation triggered: %w", err)
	}
	if updateErr := s.store.UpdateLastTriggered(ctx, automationID, now); updateErr != nil {
		s.logger.Warn("failed to update last_triggered_at",
			zap.String("automation_id", automationID), zap.Error(updateErr))
	}

	s.logger.Info("automation trigger fired",
		zap.String("automation_id", automationID),
		zap.String("trigger_id", triggerID),
		zap.String("type", string(triggerType)))
	return FireResult{RunID: admittedRun.ID}, nil
}

func (s *Service) admitTrigger(
	ctx context.Context,
	a *Automation,
	triggerID string,
	triggerType TriggerType,
	triggerData json.RawMessage,
	dedupKey string,
) (*AutomationRun, string, bool, error) {
	unlock := s.automationRunLock(a.ID)
	defer unlock()
	return s.admitTriggerLocked(ctx, a, triggerID, triggerType, triggerData, dedupKey)
}

func (s *Service) admitTriggerLocked(
	ctx context.Context,
	a *Automation,
	triggerID string,
	triggerType TriggerType,
	triggerData json.RawMessage,
	dedupKey string,
) (*AutomationRun, string, bool, error) {
	if dedupKey != "" {
		exists, err := s.store.HasRunWithDedupKey(ctx, a.ID, dedupKey)
		if err != nil {
			return nil, "", false, fmt.Errorf("check dedup: %w", err)
		}
		if exists {
			s.logger.Debug("skipping duplicate trigger",
				zap.String("automation_id", a.ID), zap.String("dedup_key", dedupKey))
			return nil, "", true, nil
		}
	}

	capReason, active, full, err := s.automationCapacity(ctx, a)
	if err != nil {
		return nil, "", false, err
	}
	if full {
		s.recordSkippedTrigger(ctx, a, triggerID, triggerType, triggerData, dedupKey, capReason, active)
		return nil, capReason, false, nil
	}

	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		Status:       RunStatusTriggered,
		DedupKey:     dedupKey,
		TriggerData:  triggerData,
		DisplayTitle: RenderRunDisplayTitle(a, triggerType, triggerData),
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, "", false, fmt.Errorf("record admitted run: %w", err)
	}
	return run, "", false, nil
}

func (s *Service) automationCapacity(ctx context.Context, a *Automation) (string, int, bool, error) {
	if a.MaxConcurrentRuns <= 0 {
		return "", 0, false, nil
	}
	active, err := s.store.CountActiveRuns(ctx, a.ID)
	if err != nil {
		return "", 0, false, fmt.Errorf("count active runs: %w", err)
	}
	if active < a.MaxConcurrentRuns {
		return "", active, false, nil
	}
	return fmt.Sprintf("max_concurrent_runs=%d reached", a.MaxConcurrentRuns), active, true, nil
}

func (s *Service) recordSkippedTrigger(
	ctx context.Context,
	a *Automation,
	triggerID string,
	triggerType TriggerType,
	triggerData json.RawMessage,
	dedupKey, reason string,
	active int,
) {
	skipDedupKey := dedupKey
	if triggerType == TriggerTypeGitHubPRMerged {
		skipDedupKey = ""
	}
	skipRun := &AutomationRun{
		AutomationID: a.ID,
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		Status:       RunStatusSkipped,
		DedupKey:     skipDedupKey,
		TriggerData:  triggerData,
		ErrorMessage: reason,
		DisplayTitle: RenderRunDisplayTitle(a, triggerType, triggerData),
	}
	if err := s.store.CreateRun(ctx, skipRun); err != nil {
		s.logger.Warn("failed to record skipped run", zap.Error(err))
	}
	s.logger.Info("automation trigger skipped: concurrency cap reached",
		zap.String("automation_id", a.ID), zap.Int("active", active), zap.Int("max", a.MaxConcurrentRuns))
}

// RecordRun records a trigger run outcome.
func (s *Service) RecordRun(ctx context.Context, run *AutomationRun) error {
	if run == nil {
		return fmt.Errorf("automation run is required")
	}
	unlock := s.automationRunLock(run.AutomationID)
	defer unlock()
	if run.TriggerDataJSON == "" {
		run.TriggerDataJSON = string(run.TriggerData)
	}
	adopted, err := s.store.AdoptTriggeredRun(ctx, run)
	if err != nil {
		return err
	}
	if adopted {
		return nil
	}
	return s.store.CreateRun(ctx, run)
}

func (s *Service) BindRunTask(ctx context.Context, runID, taskID string) error {
	unlock, err := s.lockRun(ctx, runID)
	if err != nil {
		return err
	}
	defer unlock()
	return s.store.BindRunTask(ctx, runID, taskID)
}

func (s *Service) SetContinuationTaskID(ctx context.Context, automationID, taskID string) error {
	return s.store.SetContinuationTaskID(ctx, automationID, taskID)
}

func (s *Service) BindRun(ctx context.Context, runID, taskID, sessionID, turnID string, action ThreadAction, reason string) error {
	unlock, err := s.lockRun(ctx, runID)
	if err != nil {
		return err
	}
	defer unlock()
	return s.store.BindRun(ctx, runID, taskID, sessionID, turnID, action, reason)
}

func (s *Service) MarkRunTerminal(ctx context.Context, runID, sessionID, turnID string, status RunStatus, errMsg string) error {
	return s.store.MarkRunTerminal(ctx, runID, sessionID, turnID, status, errMsg)
}

func (s *Service) MarkRunTerminalByBinding(ctx context.Context, taskID, sessionID, turnID string, status RunStatus, errMsg string) error {
	return s.store.MarkRunTerminalByBinding(ctx, taskID, sessionID, turnID, status, errMsg)
}

// MarkRunFailedByTaskID transitions a still-pending run (task_created) into
// the failed state. Used when something downstream of task creation aborts
// the run, e.g. a permission prompt an automation run can't answer.
func (s *Service) MarkRunFailedByTaskID(ctx context.Context, taskID, errMsg string) error {
	return s.store.MarkRunFailedByTaskID(ctx, taskID, errMsg)
}

// MarkRunSucceededByTaskID transitions a still-pending run (task_created)
// into the succeeded state when the launched agent finishes cleanly.
func (s *Service) MarkRunSucceededByTaskID(ctx context.Context, taskID string) error {
	return s.store.MarkRunSucceededByTaskID(ctx, taskID)
}

// PrunableRunTaskIDs names the tasks whose workspaces the caller may reclaim
// now that finalizedTaskID's run has reached a terminal status. See the store
// method for what counts as prunable.
//
// Unauthorized on purpose, like RecordRun and MarkRun{Failed,Succeeded}ByTaskID
// beside it: the only caller is the orchestrator's own run finalization, which
// runs on a background goroutine with no user in its context. An authorized
// call there would fail for every automation rather than for none.
func (s *Service) PrunableRunTaskIDs(ctx context.Context, finalizedTaskID string, keep int) ([]string, error) {
	return s.store.PrunableRunTaskIDs(ctx, finalizedTaskID, keep)
}

// RunWorkspaceInUse reports whether an agent is holding the given task's
// workspace right now. Unauthorized for the same reason as PrunableRunTaskIDs:
// its only caller is the orchestrator's own sweep, re-checking the answer that
// PrunableRunTaskIDs gave it before it deletes anything.
func (s *Service) RunWorkspaceInUse(ctx context.Context, taskID string) (bool, error) {
	return s.store.RunWorkspaceInUse(ctx, taskID)
}

// GetWebhookSecret returns the webhook secret for an automation.
func (s *Service) GetWebhookSecret(ctx context.Context, id string) (string, error) {
	a, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return "", err
	}
	if a == nil {
		return "", fmt.Errorf("automation not found: %s", id)
	}
	if err := s.authorizeWs(ctx, a.WorkspaceID); err != nil {
		return "", err
	}
	return a.WebhookSecret, nil
}
