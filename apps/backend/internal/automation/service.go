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
	// workflowLocator gates workflow ownership. Optional: when nil (isolated
	// tests) ownership is not enforced.
	workflowLocator WorkflowLocator

	// repoLookup validates repository_ids on create/update — every ID must
	// resolve to a repository belonging to the automation's workspace. Nil
	// = validation skipped (not yet wired at startup, or an isolated test).
	repoLookup RepositoryLookup

	// taskOriginLookup resolves a task's workspace and whether it is an
	// automation run, used by the merged-PR subscriber's state machine.
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

// SetWorkflowLocator wires the workflow ownership check.
func (s *Service) SetWorkflowLocator(l WorkflowLocator) {
	s.workflowLocator = l
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
// github_pr_merged trigger subscriber. Must be called before Start, following
// the SetWorkflowLocator precedent.
func (s *Service) SetTaskOriginLookup(l TaskOriginLookup) {
	s.taskOriginLookup = l
}

// TaskOriginLookup returns the wired lookup (may be nil).
func (s *Service) TaskOriginLookup() TaskOriginLookup {
	return s.taskOriginLookup
}

// SetRepositoryLookup wires the repository ownership validator for
// repository_ids on create/update. This is a security control (prevents a
// crafted request attaching another workspace's repository), so an unset
// lookup fails closed for any non-empty repository_ids list rather than
// silently skipping validation — see validateRepositoryIDs.
func (s *Service) SetRepositoryLookup(lookup RepositoryLookup) {
	s.repoLookup = lookup
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

	// Workflow + step are optional for every automation: no automation run is
	// placed on a board, so no automation needs a starting column. Ownership
	// is still enforced when one is supplied.
	if err := s.authorizeWorkflowOwnership(ctx, req.WorkspaceID, req.WorkflowID); err != nil {
		return nil, err
	}
	a := &Automation{
		WorkspaceID:       req.WorkspaceID,
		Name:              req.Name,
		Description:       req.Description,
		WorkflowID:        req.WorkflowID,
		WorkflowStepID:    req.WorkflowStepID,
		AgentProfileID:    req.AgentProfileID,
		ExecutorProfileID: req.ExecutorProfileID,
		RepositoryIDs:     req.RepositoryIDs,
		Prompt:            req.Prompt,
		TaskTitleTemplate: req.TaskTitleTemplate,
		Enabled:           true,
		MaxConcurrentRuns: maxRuns,
	}
	if err := s.validateRepositoryIDs(ctx, req.WorkspaceID, req.RepositoryIDs); err != nil {
		return nil, err
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
	if err := s.store.UpdateAutomation(ctx, id, req); err != nil {
		return nil, err
	}
	return s.store.GetAutomation(ctx, id)
}

// authorizeUpdatedReferences checks the fields of req that name something the
// automation's workspace must own — its repositories and its workflow. Both
// need the stored automation to learn that workspace, so it is loaded once
// here rather than by each check.
func (s *Service) authorizeUpdatedReferences(ctx context.Context, id string, req *UpdateAutomationRequest) error {
	if req.RepositoryIDs == nil && req.WorkflowID == nil {
		return nil
	}
	existing, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("automation not found: %s", id)
	}
	if req.RepositoryIDs != nil {
		if err := s.validateRepositoryIDs(ctx, existing.WorkspaceID, req.RepositoryIDs); err != nil {
			return err
		}
	}
	if req.WorkflowID == nil {
		return nil
	}
	return s.authorizeWorkflowOwnership(ctx, existing.WorkspaceID, *req.WorkflowID)
}

// validateRepositoryIDs rejects a duplicate entry, or any ID that isn't a
// repository belonging to workspaceID. An empty list always passes. A
// non-empty list with no RepositoryLookup wired fails closed
// (ErrRepositoryLookupUnavailable) rather than silently skipping the check —
// this validates cross-workspace access, so "unconfigured" must not mean
// "unchecked".
func (s *Service) validateRepositoryIDs(ctx context.Context, workspaceID string, repositoryIDs []string) error {
	if len(repositoryIDs) == 0 {
		return nil
	}
	if s.repoLookup == nil {
		return ErrRepositoryLookupUnavailable
	}
	seen := make(map[string]bool, len(repositoryIDs))
	for _, id := range repositoryIDs {
		if seen[id] {
			return fmt.Errorf("%w: %s", ErrDuplicateRepositoryID, id)
		}
		seen[id] = true
		repoWorkspaceID, _, ok := s.repoLookup.GetRepository(ctx, id)
		if !ok || repoWorkspaceID != workspaceID {
			return fmt.Errorf("%w: %s", ErrRepositoryNotInWorkspace, id)
		}
	}
	return nil
}

// DeleteAutomation removes an automation.
func (s *Service) DeleteAutomation(ctx context.Context, id string) error {
	if err := s.authorizeAutomation(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteAutomation(ctx, id)
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
	if req.Config != nil {
		existing, err := s.store.GetTrigger(ctx, id)
		if err != nil {
			return err
		}
		if existing != nil {
			if err := validateScheduledConfig(existing.Type, *req.Config); err != nil {
				return err
			}
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
	// Authorized here rather than in the WS handler, like every other exported
	// operation on this service. A destructive call that depends on its caller
	// remembering to check is one new caller away from not being checked.
	if run != nil {
		if authErr := s.authorizeAutomation(ctx, run.AutomationID); authErr != nil {
			return authErr
		}
	}
	if run != nil && run.TaskID != "" && s.taskDeleter != nil {
		if delErr := s.taskDeleter.DeleteTask(ctx, run.TaskID); delErr != nil {
			if !errors.Is(delErr, ErrTaskNotFound) {
				return fmt.Errorf("delete task: %w", delErr)
			}
			s.logger.Debug("run task already gone, continuing delete",
				zap.String("run_id", runID),
				zap.String("task_id", run.TaskID))
		}
	}
	return s.store.DeleteRun(ctx, runID)
}

// DeleteAllRuns removes every run for an automation, deleting each associated
// task first. Task deletion is best-effort: not-found errors are ignored.
func (s *Service) DeleteAllRuns(ctx context.Context, automationID string) error {
	if err := s.authorizeAutomation(ctx, automationID); err != nil {
		return err
	}
	defer s.automationRunLock(automationID)()
	if s.taskDeleter != nil {
		taskIDs, err := s.store.ListRunTaskIDs(ctx, automationID)
		if err != nil {
			return fmt.Errorf("list run task ids: %w", err)
		}
		for _, taskID := range taskIDs {
			if delErr := s.taskDeleter.DeleteTask(ctx, taskID); delErr != nil {
				if !errors.Is(delErr, ErrTaskNotFound) {
					return fmt.Errorf("delete task %s: %w", taskID, delErr)
				}
				s.logger.Debug("run task already gone, skipping",
					zap.String("automation_id", automationID),
					zap.String("task_id", taskID))
			}
		}
	}
	return s.store.DeleteAllRuns(ctx, automationID)
}

// --- Trigger firing ---

// FireResult reports what FireTrigger actually did. A skip is not an error:
// the trigger was evaluated and deliberately not run. Callers that report back
// to a human must distinguish the two, otherwise a deliberate skip is
// indistinguishable from a fire that happened.
type FireResult struct {
	Skipped bool
	// Reason is human-readable and set only when Skipped.
	Reason string
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

	// Check dedup.
	if dedupKey != "" {
		exists, err := s.store.HasRunWithDedupKey(ctx, automationID, dedupKey)
		if err != nil {
			return FireResult{}, fmt.Errorf("check dedup: %w", err)
		}
		if exists {
			s.logger.Debug("skipping duplicate trigger",
				zap.String("automation_id", automationID),
				zap.String("dedup_key", dedupKey))
			return FireResult{Skipped: true, Reason: "this trigger has already fired"}, nil
		}
	}

	// Enforce max_concurrent_runs: a run is "active" while still in
	// task_created (succeeded/failed/skipped don't count). If at the cap,
	// record a skipped run so the user can see the cap kicked in.
	capReason, capErr := s.maybeSkipForConcurrencyCap(ctx, a, triggerID, triggerType, triggerData, dedupKey)
	if capErr != nil {
		return FireResult{}, capErr
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
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		TriggerData:  triggerData,
		DedupKey:     dedupKey,
	}

	if updateErr := s.store.UpdateLastTriggered(ctx, automationID, now); updateErr != nil {
		s.logger.Warn("failed to update last_triggered_at",
			zap.String("automation_id", automationID), zap.Error(updateErr))
	}

	event := bus.NewEvent(events.AutomationTriggered, "automation_service", evt)
	if err := s.eventBus.Publish(ctx, events.AutomationTriggered, event); err != nil {
		return FireResult{}, fmt.Errorf("publish automation triggered: %w", err)
	}

	s.logger.Info("automation trigger fired",
		zap.String("automation_id", automationID),
		zap.String("trigger_id", triggerID),
		zap.String("type", string(triggerType)))
	return FireResult{}, nil
}

// RecordRun records a trigger run outcome.
func (s *Service) RecordRun(ctx context.Context, run *AutomationRun) error {
	return s.createRunLocked(ctx, run)
}

// maybeSkipForConcurrencyCap enforces max_concurrent_runs. Returns the
// human-readable skip reason, empty when the trigger may proceed. When
// skipped, a "skipped" run row is persisted so the user can see the cap
// kicked in.
func (s *Service) maybeSkipForConcurrencyCap(ctx context.Context, a *Automation, triggerID string, triggerType TriggerType, triggerData json.RawMessage, dedupKey string) (string, error) {
	if a.MaxConcurrentRuns <= 0 {
		return "", nil
	}
	automationID := a.ID
	active, err := s.store.CountActiveRuns(ctx, automationID)
	if err != nil {
		return "", fmt.Errorf("count active runs: %w", err)
	}
	if active < a.MaxConcurrentRuns {
		return "", nil
	}
	reason := fmt.Sprintf("max_concurrent_runs=%d reached", a.MaxConcurrentRuns)
	// github_pr_merged cap-skip rows must NOT consume the dedup key so that a
	// later event for the same PR can retry. Write an empty key for this type.
	skipDedupKey := dedupKey
	if triggerType == TriggerTypeGitHubPRMerged {
		skipDedupKey = ""
	}
	skipRun := &AutomationRun{
		AutomationID: automationID,
		TriggerID:    triggerID,
		TriggerType:  triggerType,
		Status:       RunStatusSkipped,
		DedupKey:     skipDedupKey,
		TriggerData:  triggerData,
		ErrorMessage: reason,
	}
	if recErr := s.createRunLocked(ctx, skipRun); recErr != nil {
		s.logger.Warn("failed to record skipped run", zap.Error(recErr))
	}
	s.logger.Info("automation trigger skipped: concurrency cap reached",
		zap.String("automation_id", automationID),
		zap.Int("active", active),
		zap.Int("max", a.MaxConcurrentRuns))
	return reason, nil
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
