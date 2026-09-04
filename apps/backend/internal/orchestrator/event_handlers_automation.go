package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.uber.org/zap"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

const automationDefaultBaseBranch = "main"

// AutomationService is the interface the orchestrator uses for automation operations.
type AutomationService interface {
	GetAutomation(ctx context.Context, id string) (*automation.Automation, error)
	RecordRun(ctx context.Context, run *automation.AutomationRun) error
	MarkRunFailedByTaskID(ctx context.Context, taskID, errMsg string) error
	MarkRunSucceededByTaskID(ctx context.Context, taskID string) error
}

// automationRunBinding is the exact-run extension implemented by the
// automation service. It stays separate from AutomationService so existing
// integration stubs that exercise legacy events remain source-compatible.
type automationRunBinding interface {
	GetRun(ctx context.Context, id string) (*automation.AutomationRun, error)
	BindRunTask(ctx context.Context, runID, taskID string) error
	BindRun(ctx context.Context, runID, taskID, sessionID, turnID string, action automation.ThreadAction, reason string) error
	MarkRunTerminal(ctx context.Context, runID, sessionID, turnID string, status automation.RunStatus, errMsg string) error
	MarkRunTerminalByBinding(ctx context.Context, taskID, sessionID, turnID string, status automation.RunStatus, errMsg string) error
}

type automationRunDispatcher interface {
	DispatchRun(
		ctx context.Context,
		runID string,
		action automation.ThreadAction,
		reason string,
		dispatch func() (automation.RunDispatch, error),
	) error
}

type automationContinuationState interface {
	SetContinuationTaskID(ctx context.Context, automationID, taskID string) error
}

// StopAutomationRun cancels only the currently active turn identified by the
// exact task/session/turn triple. A false result means the binding is stale or
// already terminal and no successor turn was touched.
func (s *Service) StopAutomationRun(ctx context.Context, taskID, sessionID, turnID string) (bool, error) {
	if taskID == "" || sessionID == "" || turnID == "" || s.turnService == nil || s.executor == nil {
		return false, nil
	}
	valid, err := s.automationTurnMatches(ctx, taskID, sessionID, turnID, true)
	if err != nil {
		return false, err
	}
	if !valid {
		return false, nil
	}
	return s.stopTaskSessionForCoordinator(ctx, taskID, sessionID)
}

// AutomationRunLive is the conservative liveness check used during startup
// recovery. A session that is still starting/running, an exact open turn, or a
// live permission blocker remains open. Missing runtime state is not enough to
// keep a parked/stale binding open forever.
func (s *Service) AutomationRunLive(ctx context.Context, taskID, sessionID, turnID string) (bool, error) {
	live, err := s.automationRunLive(ctx, taskID, sessionID, turnID)
	if automationRunExecutionGone(err) {
		return false, nil
	}
	return live, err
}

func (s *Service) automationRunLive(ctx context.Context, taskID, sessionID, turnID string) (bool, error) {
	if taskID == "" || sessionID == "" || turnID == "" || s.turnService == nil {
		return false, nil
	}
	session, valid, err := s.loadAutomationCoordinatorSession(ctx, taskID, sessionID, false)
	if err != nil {
		return false, err
	}
	if !valid {
		return false, nil
	}
	active, err := s.coordinatorActiveTurn(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if active != nil && active.ID == turnID {
		return true, nil
	}
	if isCoordinatorLiveSessionState(session.State) {
		return true, nil
	}
	return s.hasPendingCoordinatorPermissions(ctx, sessionID)
}

func automationRunExecutionGone(err error) bool {
	return runtimeapi.IsNotFound(err) ||
		errors.Is(err, executor.ErrExecutionNotFound) ||
		errors.Is(err, sql.ErrNoRows) ||
		errors.Is(err, taskrepo.ErrTaskNotFound) ||
		errors.Is(err, models.ErrTaskSessionNotFound)
}

func (s *Service) automationTurnMatches(ctx context.Context, taskID, sessionID, turnID string, requireStoppable bool) (bool, error) {
	session, valid, err := s.loadAutomationCoordinatorSession(ctx, taskID, sessionID, requireStoppable)
	if err != nil || !valid {
		return valid, err
	}
	active, err := s.coordinatorActiveTurn(ctx, session.ID)
	if err != nil {
		return false, err
	}
	return active != nil && active.ID == turnID, nil
}

func (s *Service) loadAutomationCoordinatorSession(ctx context.Context, taskID, sessionID string, requireStoppable bool) (*models.TaskSession, bool, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	if task == nil || !isAutomationTaskOrigin(task.Origin) {
		return nil, false, nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if session == nil || session.TaskID != taskID {
		return nil, false, nil
	}
	if requireStoppable && !isCoordinatorStoppableSessionState(session.State) {
		return nil, false, nil
	}
	return session, true, nil
}

func isAutomationTaskOrigin(origin string) bool {
	return origin == models.TaskOriginAutomationRun || origin == models.TaskOriginAutomationTask
}

func (s *Service) coordinatorActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	active, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if isNoActiveTurnError(err) {
		return nil, nil
	}
	return active, err
}

func isCoordinatorLiveSessionState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateStarting || state == models.TaskSessionStateRunning
}

func (s *Service) hasPendingCoordinatorPermissions(ctx context.Context, sessionID string) (bool, error) {
	if s.executor == nil {
		return false, nil
	}
	permissions, err := s.executor.ListPendingPermissions(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return len(permissions) > 0, nil
}

// taskCleaner deletes a task a firing has abandoned.
//
// Deliberately one method wide, and asserted at the call site rather than added
// to repoStore: that interface is implemented by several mocks, and none of
// them should have to grow a method for a path they never take.
type taskCleaner interface {
	DeleteTask(ctx context.Context, id string) error
}

// automationRunRetention names the runs whose workspaces have aged out, and
// answers whether one of them has since gone live. Kept off AutomationService
// and asserted at the call site for the same reason taskCleaner is kept off
// repoStore: several test stubs implement that interface and none of them
// should grow a method for a path they never take.
//
// Both halves live on one interface on purpose. Selection and the pre-removal
// re-check are two readings of the same question a moment apart, and a service
// that could answer only the first would hand the sweep a list it has no way
// to re-validate — the assertion failing (and nothing being reclaimed) is the
// safe outcome, so it must be all or nothing.
type automationRunRetention interface {
	PrunableRunTaskIDs(ctx context.Context, finalizedTaskID string, keep int) ([]string, error)
	RunWorkspaceInUse(ctx context.Context, taskID string) (bool, error)
}

// automationWorktreeReaper is the slice of worktree.Manager that retention
// uses. Narrowed to an interface so the sweep can be tested without a real
// git checkout on disk, and so the orchestrator states exactly which two
// capabilities it depends on.
type automationWorktreeReaper interface {
	GetAllByTaskID(ctx context.Context, taskID string) ([]*worktree.Worktree, error)
	RemoveByID(ctx context.Context, worktreeID string, removeBranch bool) error
}

// SetAutomationService sets the automation service for handling automation triggers.
func (s *Service) SetAutomationService(svc AutomationService) {
	s.automationService = svc
}

// SetWorktreeManager wires the worktree manager used to reclaim the workspaces
// of automation runs that have aged out of the retention window.
//
// Takes the concrete manager rather than the interface so a nil manager stays
// nil here: assigning a typed-nil pointer into an interface field produces a
// non-nil interface, and every nil check downstream would then pass and panic.
func (s *Service) SetWorktreeManager(mgr *worktree.Manager) {
	if mgr == nil {
		return
	}
	s.worktreeReaper = mgr
	s.taskLaunchRecoveryWorktree = mgr
}

// subscribeAutomationEvents subscribes to automation-related events on the event bus.
func (s *Service) subscribeAutomationEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.AutomationTriggered, s.handleAutomationTriggered); err != nil {
		s.logger.Error("failed to subscribe to automation.triggered events", zap.Error(err))
	}
}

// handleAutomationTriggered creates a task when an automation trigger fires.
func (s *Service) handleAutomationTriggered(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*automation.AutomationTriggeredEvent)
	if !ok {
		return nil
	}

	s.logger.Info("automation trigger received",
		zap.String("automation_id", evt.AutomationID),
		zap.String("trigger_type", string(evt.TriggerType)))

	if s.automationService == nil || s.reviewTaskCreator == nil {
		s.logger.Warn("automation service or task creator not configured")
		return nil
	}

	go s.createAutomationTask(context.Background(), evt)
	return nil
}

func (s *Service) createAutomationTask(ctx context.Context, evt *automation.AutomationTriggeredEvent) {
	a, err := s.automationService.GetAutomation(ctx, evt.AutomationID)
	if err != nil || a == nil {
		s.logger.Error("failed to load automation for trigger",
			zap.String("automation_id", evt.AutomationID), zap.Error(err))
		s.recordFailedRun(ctx, evt, "automation not found")
		return
	}
	if !a.Enabled {
		s.logger.Debug("automation disabled, skipping",
			zap.String("automation_id", evt.AutomationID))
		s.recordFailedRun(ctx, evt, "automation is disabled")
		return
	}

	// Interpolate prompt with trigger data.
	prompt := automation.InterpolatePrompt(a.Prompt, evt.TriggerType, evt.TriggerData)
	if prompt == "" {
		prompt = fmt.Sprintf("Automation '%s' triggered by %s", a.Name, evt.TriggerType)
	}

	title := s.resolveAutomationTaskTitle(a, evt)
	if binding, ok := s.automationService.(automationRunBinding); ok && evt.RunID != "" {
		if run, runErr := binding.GetRun(ctx, evt.RunID); runErr == nil && run != nil && run.DisplayTitle != "" {
			title = run.DisplayTitle
		}
	}
	metadata := map[string]interface{}{
		"automation_id":                 a.ID,
		"automation_name":               a.Name,
		"trigger_id":                    evt.TriggerID,
		"trigger_type":                  string(evt.TriggerType),
		models.MetaKeyAgentProfileID:    a.AgentProfileID,
		models.MetaKeyExecutorProfileID: a.ExecutorProfileID,
	}
	if evt.TriggerType == automation.TriggerTypeGitHubPRMerged {
		var triggerData struct {
			TaskID string `json:"task_id"`
		}
		_ = json.Unmarshal(evt.TriggerData, &triggerData)
		metadata[models.MetaKeyAutomationTargetTaskID] = triggerData.TaskID
	}

	task, continuationSession, action, reason, taskErr := s.prepareAutomationTask(
		ctx, a, evt, title, prompt, metadata,
	)
	if taskErr != nil {
		s.recordFailedRun(ctx, evt, taskErr.Error())
		return
	}

	// The run row is the record that this firing happened and carries its exact
	// concurrency accounting. Hidden tasks are reachable only through that row;
	// visible normal tasks also remain available through ordinary task lists.
	if err := s.recordSuccessRun(ctx, evt, task.ID); err != nil {
		s.logger.Error("failed to record automation run; abandoning the firing",
			zap.String("automation_id", a.ID),
			zap.String("task_id", task.ID),
			zap.Error(err))
		if action != automation.ThreadActionResumed {
			s.deleteAbandonedTask(ctx, a.ID, task.ID)
		}
		return
	}

	// Associate PR with task for github_pr triggers (same as PR Watcher).
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		if repositories := s.resolveAutomationRepository(ctx, a, evt); len(repositories) > 0 {
			s.associateAutomationPR(ctx, task.ID, repositories[0].RepositoryID, evt.TriggerData)
		}
	}

	s.logger.Info("created automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID),
		zap.String("trigger_type", string(evt.TriggerType)))

	if continuationSession != nil {
		s.dispatchAutomationContinuation(ctx, a, task, continuationSession, prompt, metadata, evt.RunID, action, reason)
		return
	}

	// Auto-start unconditionally: nobody opens an automation's task to drag
	// it, so the trigger has to be the start signal. A workflow step's
	// auto_start_agent setting is irrelevant here because the automation
	// trigger is the start signal for both hidden runs and visible normal tasks.
	s.autoStartAutomationTaskForRun(ctx, a, task, task.WorkflowStepID, evt.RunID, action, reason)
}

func (s *Service) prepareAutomationTask(
	ctx context.Context,
	a *automation.Automation,
	evt *automation.AutomationTriggeredEvent,
	title, prompt string,
	metadata map[string]interface{},
) (*models.Task, *models.TaskSession, automation.ThreadAction, string, error) {
	action := automation.ThreadActionCreated
	reason := "new task created for automation run"
	taskMode := a.TaskMode
	if taskMode == "" {
		taskMode = automation.TaskModeAutomationRun
	}
	repositoryMode := a.RepositoryMode
	if repositoryMode == "" {
		if len(a.Repositories) > 0 || len(a.RepositoryIDs) > 0 {
			repositoryMode = automation.RepositoryModeSelected
		} else {
			repositoryMode = automation.RepositoryModeNone
		}
	}
	metadata[models.MetaKeyAutomationTaskMode] = string(taskMode)
	metadata[models.MetaKeyAutomationRepositoryMode] = string(repositoryMode)
	taskOrigin := models.TaskOriginAutomationRun
	if taskMode == automation.TaskModeNormalTask {
		taskOrigin = models.TaskOriginAutomationTask
	}
	var task *models.Task
	var continuationSession *models.TaskSession
	if a.ContinuationPolicy == automation.ContinuationPolicyReuseThread {
		task, continuationSession, reason = s.findAutomationContinuation(ctx, a, evt)
		if task != nil {
			action = automation.ThreadActionResumed
		} else if a.ContinuationTaskID != "" {
			action = automation.ThreadActionReplaced
		}
	}
	if task != nil {
		return task, continuationSession, action, reason, nil
	}

	repositories := s.resolveAutomationRepository(ctx, a, evt)
	task, err := s.reviewTaskCreator.CreateReviewTask(ctx, &ReviewTaskRequest{
		WorkspaceID:    a.WorkspaceID,
		WorkflowID:     a.WorkflowID,
		WorkflowStepID: a.WorkflowStepID,
		Title:          title,
		Description:    prompt,
		Repositories:   repositories,
		Metadata:       metadata,
		Origin:         taskOrigin,
	})
	if err != nil {
		return nil, nil, action, reason, fmt.Errorf("create automation task: %w", err)
	}
	if a.ContinuationPolicy != automation.ContinuationPolicyReuseThread {
		return task, nil, action, reason, nil
	}
	state, ok := s.automationService.(automationContinuationState)
	if !ok {
		s.deleteAbandonedTask(ctx, a.ID, task.ID)
		return nil, nil, action, reason, fmt.Errorf("automation continuation state is unavailable")
	}
	if err := state.SetContinuationTaskID(ctx, a.ID, task.ID); err != nil {
		s.deleteAbandonedTask(ctx, a.ID, task.ID)
		return nil, nil, action, reason, err
	}
	if action == automation.ThreadActionReplaced {
		reason = "previous continuation was unavailable; created a replacement task"
	}
	return task, nil, action, reason, nil
}

// automationContinuationMetadataSnapshot records the values that a firing
// replaces. It lets a failed dispatch restore the previous authorization
// target instead of leaving a stopped firing's target on the shared task.
type automationContinuationMetadataSnapshot struct {
	values  map[string]interface{}
	present map[string]bool
}

// refreshAutomationContinuationMetadata is kept as the small direct helper
// used by tests and legacy callers. The run-aware path uses the snapshot form
// so it can restore the task when dispatch or exact binding fails.
func (s *Service) refreshAutomationContinuationMetadata(ctx context.Context, task *models.Task, metadata map[string]interface{}) error {
	_, err := s.refreshAutomationContinuationMetadataWithSnapshot(ctx, task, metadata)
	return err
}

// refreshAutomationContinuationMetadataWithSnapshot merges this firing's
// freshly-computed metadata onto a resumed continuation task and persists it.
// The writes happen only after DispatchRun has admitted the firing and holds
// its per-automation lock. This prevents a stopped or deleted run from
// changing the shared task's archive authorization.
//
// Each key is patched independently through the concurrent-key-safe
// SetTaskMetadataKey primitive rather than a full-row UpdateTask. A full-row
// write built from an in-memory snapshot could silently resurrect a key that
// another lifecycle operation just removed. Deferred-launch ownership is
// server-managed and is skipped here.
func (s *Service) refreshAutomationContinuationMetadataWithSnapshot(
	ctx context.Context,
	task *models.Task,
	metadata map[string]interface{},
) (*automationContinuationMetadataSnapshot, error) {
	keys := orderedAutomationContinuationMetadataKeys(metadata)
	snapshot := &automationContinuationMetadataSnapshot{
		values:  make(map[string]interface{}, len(keys)),
		present: make(map[string]bool, len(keys)),
	}
	for _, key := range keys {
		value, ok := task.Metadata[key]
		snapshot.values[key] = value
		snapshot.present[key] = ok
	}

	written := make([]string, 0, len(keys))
	for _, key := range keys {
		if err := s.repo.SetTaskMetadataKey(ctx, task.ID, key, metadata[key]); err != nil {
			s.logger.Error("failed to refresh automation continuation task metadata",
				zap.String("task_id", task.ID), zap.String("metadata_key", key), zap.Error(err))
			if restoreErr := s.restoreAutomationContinuationMetadata(ctx, task, snapshot, written); restoreErr != nil {
				s.logger.Warn("failed to restore automation continuation task metadata after refresh failure",
					zap.String("task_id", task.ID), zap.Error(restoreErr))
			}
			return nil, fmt.Errorf("refresh automation continuation metadata key %q: %w", key, err)
		}
		written = append(written, key)
	}
	if len(written) == 0 {
		return snapshot, nil
	}
	if err := s.reloadAutomationContinuationTask(ctx, task); err != nil {
		if restoreErr := s.restoreAutomationContinuationMetadata(ctx, task, snapshot, written); restoreErr != nil {
			s.logger.Warn("failed to restore automation continuation task metadata after reload failure",
				zap.String("task_id", task.ID), zap.Error(restoreErr))
		}
		return nil, fmt.Errorf("reload refreshed automation continuation task: %w", err)
	}
	s.publishTaskUpdated(ctx, task)
	return snapshot, nil
}

func orderedAutomationContinuationMetadataKeys(metadata map[string]interface{}) []string {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		if key == models.MetaKeyDeferredLaunch || key == models.MetaKeyAutomationTargetTaskID {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if _, ok := metadata[models.MetaKeyAutomationTargetTaskID]; ok {
		keys = append([]string{models.MetaKeyAutomationTargetTaskID}, keys...)
	}
	return keys
}

func (s *Service) restoreAutomationContinuationMetadata(
	ctx context.Context,
	task *models.Task,
	snapshot *automationContinuationMetadataSnapshot,
	keys []string,
) error {
	if snapshot == nil || len(keys) == 0 {
		return nil
	}
	for _, key := range keys {
		var err error
		if snapshot.present[key] {
			err = s.repo.SetTaskMetadataKey(ctx, task.ID, key, snapshot.values[key])
		} else {
			_, err = s.repo.RemoveTaskMetadataKey(ctx, task.ID, key)
		}
		if err != nil {
			return fmt.Errorf("restore automation continuation metadata key %q: %w", key, err)
		}
	}
	if err := s.reloadAutomationContinuationTask(ctx, task); err != nil {
		return err
	}
	s.publishTaskUpdated(ctx, task)
	return nil
}

func (s *Service) reloadAutomationContinuationTask(ctx context.Context, task *models.Task) error {
	refreshed, err := s.repo.GetTask(ctx, task.ID)
	if err != nil {
		return err
	}
	if refreshed == nil {
		return fmt.Errorf("task %s was not found", task.ID)
	}
	*task = *refreshed
	return nil
}

func (s *Service) findAutomationContinuation(ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent) (*models.Task, *models.TaskSession, string) {
	if a.ContinuationTaskID == "" {
		return nil, nil, "no continuation task exists; created the first task"
	}
	task, err := s.repo.GetTask(ctx, a.ContinuationTaskID)
	if err != nil || task == nil {
		return nil, nil, "previous continuation task was not found"
	}
	if reason := s.continuationTaskIncompatibility(ctx, a, evt, task); reason != "" {
		return nil, nil, reason
	}
	session, reason := s.continuationSession(ctx, task.ID)
	if reason != "" {
		return nil, nil, reason
	}
	return task, session, "continued the previous task and session"
}

func (s *Service) continuationTaskIncompatibility(ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent, task *models.Task) string {
	if task.WorkspaceID != a.WorkspaceID || task.Origin != automationTaskOrigin(a) || task.ArchivedAt != nil {
		return "previous continuation task is not compatible"
	}
	if !continuationWorkflowMatches(a, task) {
		return "previous continuation task uses a different workflow"
	}
	if !continuationTaskModeMatches(a, task) {
		return "previous continuation task uses a different target mode"
	}
	if !continuationRepositoryModeMatches(a, task) {
		return "previous continuation task uses a different repository mode"
	}
	if !continuationProfilesMatch(a, task) {
		return "previous continuation task uses different runtime profiles"
	}
	if !s.continuationRepositoriesMatch(ctx, a, evt, task) {
		return "previous continuation task uses different repositories"
	}
	return ""
}

func automationTaskOrigin(a *automation.Automation) string {
	if a.TaskMode == automation.TaskModeNormalTask {
		return models.TaskOriginAutomationTask
	}
	return models.TaskOriginAutomationRun
}

func continuationWorkflowMatches(a *automation.Automation, task *models.Task) bool {
	return task.WorkflowID == a.WorkflowID && (a.WorkflowStepID == "" || task.WorkflowStepID == a.WorkflowStepID)
}

func continuationTaskModeMatches(a *automation.Automation, task *models.Task) bool {
	taskMode := a.TaskMode
	if taskMode == "" {
		taskMode = automation.TaskModeAutomationRun
	}
	previous := metadataString(task.Metadata, models.MetaKeyAutomationTaskMode)
	return previous == "" || previous == string(taskMode)
}

func continuationRepositoryModeMatches(a *automation.Automation, task *models.Task) bool {
	repositoryMode := a.RepositoryMode
	if repositoryMode == "" {
		if len(a.Repositories) > 0 || len(a.RepositoryIDs) > 0 {
			repositoryMode = automation.RepositoryModeSelected
		} else {
			repositoryMode = automation.RepositoryModeNone
		}
	}
	previous := metadataString(task.Metadata, models.MetaKeyAutomationRepositoryMode)
	return previous == "" || previous == string(repositoryMode)
}

func continuationProfilesMatch(a *automation.Automation, task *models.Task) bool {
	return metadataString(task.Metadata, models.MetaKeyAgentProfileID) == a.AgentProfileID &&
		metadataString(task.Metadata, models.MetaKeyExecutorProfileID) == a.ExecutorProfileID
}

func (s *Service) continuationRepositoriesMatch(ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent, task *models.Task) bool {
	expected := s.resolveAutomationRepository(ctx, a, evt)
	if sameAutomationRepositoryIDs(task.Repositories, expected) {
		return true
	}
	store, ok := s.repo.(interface {
		ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	})
	if !ok {
		return false
	}
	links, err := store.ListTaskRepositories(ctx, task.ID)
	return err == nil && sameAutomationRepositoryIDs(links, expected)
}

func (s *Service) continuationSession(ctx context.Context, taskID string) (*models.TaskSession, string) {
	lookup, ok := s.repo.(interface {
		GetTaskSessionByTaskID(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return nil, "task session lookup is unavailable"
	}
	session, err := lookup.GetTaskSessionByTaskID(ctx, taskID)
	if err != nil || session == nil {
		return nil, "previous continuation session was not found"
	}
	if session.State != models.TaskSessionStateWaitingForInput && session.State != models.TaskSessionStateIdle {
		return nil, "previous continuation session is not ready"
	}
	return session, ""
}

func metadataString(metadata map[string]interface{}, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func sameAutomationRepositoryIDs(taskRepos []*models.TaskRepository, expected []ReviewTaskRepository) bool {
	if len(taskRepos) != len(expected) {
		return false
	}
	for i, expectedRepo := range expected {
		if taskRepos[i] == nil || taskRepos[i].RepositoryID != expectedRepo.RepositoryID ||
			taskRepos[i].BaseBranch != expectedRepo.BaseBranch {
			return false
		}
	}
	return true
}

// dispatchAutomationRun hands an admitted run to the service-owned
// dispatcher when available. The dispatcher holds the per-automation lock
// across the provider call and exact binding, so a stop cannot race a launch.
// The bool distinguishes that path from legacy test/integration services that
// do not implement the extension yet.
func (s *Service) dispatchAutomationRun(
	ctx context.Context,
	automationID, taskID, sessionID, runID string,
	action automation.ThreadAction,
	reason, operation string,
	dispatch func() (automation.RunDispatch, error),
	onFailure func(),
) bool {
	if runID == "" {
		return false
	}
	dispatcher, ok := s.automationService.(automationRunDispatcher)
	if !ok {
		return false
	}
	if err := dispatcher.DispatchRun(ctx, runID, action, reason, dispatch); err == nil {
		return true
	} else {
		if onFailure != nil {
			onFailure()
		}
		s.logger.Error("failed to dispatch automation run",
			zap.String("operation", operation), zap.String("automation_id", automationID),
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
	s.cleanupFailedAutomationTask(ctx, automationID, taskID, action)
	return true
}

func (s *Service) cleanupFailedAutomationTask(ctx context.Context, automationID, taskID string, action automation.ThreadAction) {
	if action == automation.ThreadActionResumed {
		return
	}
	s.deleteAbandonedTask(ctx, automationID, taskID)
}

// bindAutomationRun records a dispatch result for legacy callers that do not
// use the service-owned dispatcher. It returns false after a binding failure
// because the provider turn must not continue as an untracked run.
func (s *Service) bindAutomationRun(
	ctx context.Context,
	runID, taskID, sessionID, turnID string,
	action automation.ThreadAction,
	reason, operation string,
) bool {
	if runID == "" {
		return true
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return true
	}
	bindErr := binding.BindRun(ctx, runID, taskID, sessionID, turnID, action, reason)
	if bindErr == nil {
		return true
	}
	s.logger.Error("failed to bind automation run",
		zap.String("operation", operation), zap.String("run_id", runID),
		zap.String("task_id", taskID), zap.String("turn_id", turnID), zap.Error(bindErr))
	if !s.markExactAutomationRunTerminal(ctx, runID, "", "", false, bindErr.Error()) {
		s.markAutomationRunTerminal(ctx, taskID, false, bindErr.Error())
	}
	return false
}

func (s *Service) dispatchAutomationContinuation(ctx context.Context, a *automation.Automation, task *models.Task, session *models.TaskSession, prompt string, metadata map[string]interface{}, runID string, action automation.ThreadAction, reason string) {
	var snapshot *automationContinuationMetadataSnapshot
	restore := func() {
		if snapshot == nil {
			return
		}
		keys := orderedAutomationContinuationMetadataKeys(metadata)
		if err := s.restoreAutomationContinuationMetadata(ctx, task, snapshot, keys); err != nil {
			s.logger.Warn("failed to restore automation continuation task metadata",
				zap.String("task_id", task.ID), zap.Error(err))
		}
		snapshot = nil
	}
	dispatch := func() (automation.RunDispatch, error) {
		var err error
		snapshot, err = s.refreshAutomationContinuationMetadataWithSnapshot(ctx, task, metadata)
		if err != nil {
			return automation.RunDispatch{}, err
		}
		result, err := s.promptAutomationContinuation(ctx, task, session, prompt)
		if err != nil {
			restore()
			return automation.RunDispatch{}, err
		}
		return result, nil
	}
	if s.dispatchAutomationRun(ctx, a.ID, task.ID, session.ID, runID, action, reason, "continuation", dispatch, restore) {
		return
	}

	dispatchResult, err := dispatch()
	if err != nil {
		s.logger.Error("failed to dispatch automation continuation",
			zap.String("automation_id", a.ID), zap.String("task_id", task.ID), zap.String("session_id", session.ID), zap.Error(err))
		if !s.markExactAutomationRunTerminal(ctx, runID, "", "", false, err.Error()) {
			s.markAutomationRunTerminal(ctx, task.ID, false, err.Error())
		}
		return
	}
	if !s.bindAutomationRun(ctx, runID, dispatchResult.TaskID, dispatchResult.SessionID, dispatchResult.TurnID, action, reason, "continuation") {
		restore()
		return
	}
}

func (s *Service) promptAutomationContinuation(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
	prompt string,
) (automation.RunDispatch, error) {
	result, err := s.PromptTask(ctx, task.ID, session.ID, prompt, "", false, nil, true)
	if err != nil {
		return automation.RunDispatch{}, err
	}
	turnID := ""
	if result != nil {
		turnID = result.TurnID
	}
	if turnID == "" && s.turnService != nil {
		if active, turnErr := s.turnService.GetActiveTurn(ctx, session.ID); turnErr == nil && active != nil {
			turnID = active.ID
		}
	}
	if turnID == "" {
		s.cancelAutomationDispatch(ctx, task.ID, session.ID)
		return automation.RunDispatch{}, errors.New("automation continuation dispatch returned no turn identity")
	}
	s.recordAutomationContinuationMessage(ctx, task.ID, session.ID, turnID, prompt)
	return automation.RunDispatch{TaskID: task.ID, SessionID: session.ID, TurnID: turnID}, nil
}

func (s *Service) cancelAutomationDispatch(ctx context.Context, taskID, sessionID string) {
	if s.turnService == nil || s.executor == nil {
		return
	}
	if _, err := s.stopTaskSessionForCoordinator(ctx, taskID, sessionID); err != nil {
		s.logger.Warn("failed to cancel automation dispatch without a turn identity",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
}

func (s *Service) recordAutomationContinuationMessage(ctx context.Context, taskID, sessionID, turnID, prompt string) {
	if s.messageCreator == nil || prompt == "" || turnID == "" {
		return
	}
	meta := NewUserMessageMeta().WithAutoStart(true)
	if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, turnID, meta.ToMap()); err != nil {
		s.logger.Warn("failed to record automation continuation prompt", zap.String("task_id", taskID), zap.String("turn_id", turnID), zap.Error(err))
	}
}

func (s *Service) autoStartAutomationTask(ctx context.Context, a *automation.Automation, task *models.Task, workflowStepID string) {
	s.autoStartAutomationTaskForRun(ctx, a, task, workflowStepID, "", automation.ThreadActionCreated, "")
}

func (s *Service) autoStartAutomationTaskForRun(ctx context.Context, a *automation.Automation, task *models.Task, workflowStepID, runID string, action automation.ThreadAction, reason string) {
	if s.dispatchAutomationRun(ctx, a.ID, task.ID, "", runID, action, reason, "auto-start", func() (automation.RunDispatch, error) {
		return s.startAutomationTask(ctx, a, task, workflowStepID)
	}, nil) {
		return
	}

	dispatch, err := s.startAutomationTask(ctx, a, task, workflowStepID)
	if err != nil {
		s.logger.Error("failed to auto-start automation task",
			zap.String("task_id", task.ID), zap.Error(err))
		// The run row was written before the launch, so a start that never
		// happened would otherwise sit at task_created forever and hold a
		// max_concurrent_runs slot — one failed launch would stop the
		// automation permanently, with no completion event coming to free it.
		if !s.markExactAutomationRunTerminal(ctx, runID, "", "", false, err.Error()) {
			s.markAutomationRunTerminal(ctx, task.ID, false, err.Error())
		}
		return
	}
	if !s.bindAutomationRun(ctx, runID, dispatch.TaskID, dispatch.SessionID, dispatch.TurnID, action, reason, "auto-start") {
		return
	}
	s.logger.Info("auto-started automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID))
}

func (s *Service) startAutomationTask(
	ctx context.Context,
	a *automation.Automation,
	task *models.Task,
	workflowStepID string,
) (automation.RunDispatch, error) {
	execution, err := s.StartTask(
		ctx,
		task.ID,
		a.AgentProfileID,
		"",
		a.ExecutorProfileID,
		"",
		task.Description,
		workflowStepID,
		false,
		true,
		nil,
	)
	if err != nil {
		return automation.RunDispatch{}, err
	}
	if execution == nil || execution.SessionID == "" || execution.TurnID == "" {
		return automation.RunDispatch{}, errors.New("automation task start returned no session or turn identity")
	}
	return automation.RunDispatch{TaskID: task.ID, SessionID: execution.SessionID, TurnID: execution.TurnID}, nil
}

// resolveAutomationRepository determines the repositories for an
// automation-triggered task. For github_pr triggers, it always extracts
// repo info from the trigger data — the PR's own repo is the only sensible
// choice when responding to a PR event, so the automation's RepositoryIDs
// are ignored. For other triggers (scheduled, webhook), it prefers the
// automation's explicit repository/base-branch pairs. An empty selection is
// repository-free and never falls back to workspace ordering.
func (s *Service) resolveAutomationRepository(
	ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent,
) []ReviewTaskRepository {
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		return s.resolveGitHubPRTriggerRepository(ctx, a.WorkspaceID, evt.TriggerData)
	}
	if len(a.Repositories) == 0 && len(a.RepositoryIDs) == 0 {
		return nil
	}
	return s.resolveExplicitRepositories(ctx, configuredAutomationRepositories(a))
}

// resolveExplicitRepositories loads each configured repository and produces one
// ReviewTaskRepository entry per resolvable pair, in order. Canonical bindings
// use their saved base branch; legacy repository-ID-only rows use the repository
// default. A repository that fails to load is skipped (with a warning) rather
// than aborting the whole firing, so one bad repository does not sink the others.
func (s *Service) resolveExplicitRepositories(
	ctx context.Context, repositories []automation.AutomationRepository,
) []ReviewTaskRepository {
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil
	}
	resolved := make([]ReviewTaskRepository, 0, len(repositories))
	for _, configured := range repositories {
		repo, err := store.GetRepository(ctx, configured.RepositoryID)
		if err != nil || repo == nil {
			s.logger.Warn("failed to load explicit automation repository",
				zap.String("repository_id", configured.RepositoryID), zap.Error(err))
			continue
		}
		baseBranch := configured.BaseBranch
		if baseBranch == "" {
			baseBranch = repo.DefaultBranch
		}
		if baseBranch == "" {
			baseBranch = automationDefaultBaseBranch
		}
		resolved = append(resolved, ReviewTaskRepository{
			RepositoryID:   repo.ID,
			BaseBranch:     baseBranch,
			CheckoutBranch: baseBranch,
		})
	}
	return resolved
}

func configuredAutomationRepositories(a *automation.Automation) []automation.AutomationRepository {
	if len(a.Repositories) > 0 {
		return a.Repositories
	}
	result := make([]automation.AutomationRepository, 0, len(a.RepositoryIDs))
	for _, repositoryID := range a.RepositoryIDs {
		result = append(result, automation.AutomationRepository{RepositoryID: repositoryID})
	}
	return result
}

// resolveGitHubPRTriggerRepository extracts repo owner/name from PR trigger data
// and resolves it via the repository resolver.
func (s *Service) resolveGitHubPRTriggerRepository(
	ctx context.Context, workspaceID string, triggerData json.RawMessage,
) []ReviewTaskRepository {
	if s.repositoryResolver == nil {
		return nil
	}
	var data struct {
		Repo       string `json:"repo"`
		HeadBranch string `json:"head_branch"`
		BaseBranch string `json:"base_branch"`
	}
	if err := json.Unmarshal(triggerData, &data); err != nil || data.Repo == "" {
		return nil
	}
	parts := strings.SplitN(data.Repo, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	owner, name := parts[0], parts[1]
	repoID, baseBranch, err := s.repositoryResolver.ResolveForReview(
		ctx, workspaceID, "github", owner, name, data.BaseBranch,
	)
	if err != nil || repoID == "" {
		s.logger.Warn("failed to resolve PR trigger repository",
			zap.String("repo", data.Repo), zap.Error(err))
		return nil
	}
	return []ReviewTaskRepository{{
		RepositoryID:   repoID,
		BaseBranch:     baseBranch,
		CheckoutBranch: data.HeadBranch,
	}}
}

// resolveAutomationTaskTitle builds the task title from the automation's template or falls back to a default.
func (s *Service) resolveAutomationTaskTitle(a *automation.Automation, evt *automation.AutomationTriggeredEvent) string {
	return automation.RenderRunDisplayTitle(a, evt.TriggerType, evt.TriggerData)
}

// associateAutomationPR links a task to a GitHub PR using trigger data.
func (s *Service) associateAutomationPR(ctx context.Context, taskID, repositoryID string, triggerData json.RawMessage) {
	if s.githubService == nil {
		return
	}
	var data struct {
		Number      float64 `json:"number"`
		Title       string  `json:"title"`
		HTMLURL     string  `json:"html_url"`
		AuthorLogin string  `json:"author_login"`
		Repo        string  `json:"repo"`
		HeadBranch  string  `json:"head_branch"`
		BaseBranch  string  `json:"base_branch"`
		Body        string  `json:"body"`
		Draft       bool    `json:"draft"`
		State       string  `json:"state"`
	}
	if err := json.Unmarshal(triggerData, &data); err != nil || data.Repo == "" {
		return
	}
	parts := strings.SplitN(data.Repo, "/", 2)
	if len(parts) != 2 {
		return
	}
	pr := &github.PR{
		Number:      int(data.Number),
		Title:       data.Title,
		HTMLURL:     data.HTMLURL,
		AuthorLogin: data.AuthorLogin,
		RepoOwner:   parts[0],
		RepoName:    parts[1],
		HeadBranch:  data.HeadBranch,
		BaseBranch:  data.BaseBranch,
		Body:        data.Body,
		Draft:       data.Draft,
		State:       data.State,
	}
	if _, err := s.githubService.AssociatePRWithTask(ctx, taskID, repositoryID, pr); err != nil {
		s.logger.Error("failed to associate PR with automation task",
			zap.String("task_id", taskID),
			zap.Int("pr_number", pr.Number),
			zap.Error(err))
	}
}

func (s *Service) recordFailedRun(ctx context.Context, evt *automation.AutomationTriggeredEvent, errMsg string) {
	if s.automationService == nil {
		return
	}
	if s.markExactAutomationRunTerminal(ctx, evt.RunID, "", "", false, errMsg) {
		return
	}
	// github_pr_merged pre-task-failure rows must NOT consume the dedup key so
	// that a later event for the same PR can retry. All recordFailedRun call
	// sites are pre-task-creation, so blanking is always correct for this type.
	failDedupKey := evt.DedupKey
	if evt.TriggerType == automation.TriggerTypeGitHubPRMerged {
		failDedupKey = ""
	}
	run := &automation.AutomationRun{
		AutomationID: evt.AutomationID,
		TriggerID:    evt.TriggerID,
		TriggerType:  evt.TriggerType,
		Status:       automation.RunStatusFailed,
		DedupKey:     failDedupKey,
		TriggerData:  evt.TriggerData,
		ErrorMessage: errMsg,
	}
	if recordErr := s.automationService.RecordRun(ctx, run); recordErr != nil {
		s.logger.Error("failed to record automation run", zap.Error(recordErr))
	}
}

// recordSuccessRun writes the row that makes a firing visible and countable.
// It reports failure rather than logging it: the caller must not launch an
// agent for a run nothing can see.
// deleteAbandonedTask removes a task whose run row was never written.
//
// A failure here is reported rather than swallowed: the task is then genuinely
// orphaned — hidden by its origin, pointed at by no run — and only a human
// reading this line will know it is there.
func (s *Service) deleteAbandonedTask(ctx context.Context, automationID, taskID string) {
	s.clearAbandonedContinuation(ctx, automationID, taskID)
	cleaner, ok := s.repo.(taskCleaner)
	if !ok {
		return
	}
	if err := cleaner.DeleteTask(ctx, taskID); err != nil {
		s.logger.Error("failed to delete the task of an unrecorded automation run",
			zap.String("automation_id", automationID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func (s *Service) clearAbandonedContinuation(ctx context.Context, automationID, taskID string) {
	if s.automationService == nil {
		return
	}
	state, ok := s.automationService.(automationContinuationState)
	if !ok {
		return
	}
	current, err := s.automationService.GetAutomation(ctx, automationID)
	if err != nil || current == nil || current.ContinuationTaskID != taskID {
		return
	}
	if err := state.SetContinuationTaskID(ctx, automationID, ""); err != nil {
		s.logger.Error("failed to clear abandoned automation continuation",
			zap.String("automation_id", automationID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func (s *Service) recordSuccessRun(
	ctx context.Context, evt *automation.AutomationTriggeredEvent, taskID string,
) error {
	if s.automationService == nil {
		return nil
	}
	if evt.RunID != "" {
		binding, ok := s.automationService.(automationRunBinding)
		if !ok {
			return fmt.Errorf("automation service cannot bind admitted run %s", evt.RunID)
		}
		return binding.BindRunTask(ctx, evt.RunID, taskID)
	}
	run := &automation.AutomationRun{
		AutomationID: evt.AutomationID,
		TriggerID:    evt.TriggerID,
		TriggerType:  evt.TriggerType,
		TaskID:       taskID,
		Status:       automation.RunStatusTaskCreated,
		DedupKey:     evt.DedupKey,
		TriggerData:  evt.TriggerData,
	}
	return s.automationService.RecordRun(ctx, run)
}

func (s *Service) markExactAutomationRunTerminal(ctx context.Context, runID, sessionID, turnID string, success bool, errMsg string) bool {
	if runID == "" || s.automationService == nil {
		return false
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return false
	}
	status := automation.RunStatusFailed
	if success {
		status = automation.RunStatusSucceeded
	}
	if err := binding.MarkRunTerminal(ctx, runID, sessionID, turnID, status, errMsg); err != nil {
		s.logger.Warn("failed to update exact automation run terminal status",
			zap.String("run_id", runID), zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
		return false
	}
	return true
}

// handleAutomationTurnComplete closes out an automation run on the stream
// complete event. ACP agents can stay alive after stop_reason=end_turn, so
// waiting for agent.completed would leave the AutomationRun stuck at
// task_created and keep the executor alive.
func (s *Service) handleAutomationTurnComplete(
	ctx context.Context, taskID, sessionID string, session *models.TaskSession, stopReason string, isError bool, errMsg string,
) bool {
	return s.handleAutomationTurnCompleteForTurn(ctx, taskID, sessionID, session, "", stopReason, isError, errMsg)
}

func (s *Service) handleAutomationTurnCompleteForTurn(
	ctx context.Context, taskID, sessionID string, session *models.TaskSession, turnID, stopReason string, isError bool, errMsg string,
) bool {
	if taskID == "" || sessionID == "" {
		return false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return false
	}
	if task.Origin == models.TaskOriginAutomationTask {
		return s.handleVisibleAutomationTurnComplete(ctx, taskID, sessionID, turnID, stopReason, isError, errMsg)
	}
	if task.Origin != models.TaskOriginAutomationRun {
		return false
	}

	cancelled := stopReason == stopReasonCancelled
	success := !isError && stopReason != agentEventError && !cancelled
	if cancelled && errMsg == "" {
		errMsg = stopReasonCancelled
	}
	// A successful run parks in WAITING_FOR_INPUT, which is where the ordinary
	// agent.completed path leaves a finished turn. COMPLETED is terminal and
	// explicitly not resumable ("create a new session instead"), so using it
	// here would make every finished run unrepliable — the opposite of "a run
	// is a thread, not a receipt". Success is recorded on the AutomationRun
	// row; the session state only says whether the conversation can continue.
	nextState := models.TaskSessionStateWaitingForInput
	if cancelled {
		nextState = models.TaskSessionStateCancelled
	} else if !success {
		nextState = models.TaskSessionStateFailed
	}
	s.updateTaskSessionState(ctx, taskID, sessionID, nextState, errMsg, false, session)
	s.markAutomationRunTerminalForTurn(ctx, taskID, sessionID, turnID, success, errMsg)
	s.stopAutomationAgent(ctx, taskID, sessionID, session)
	// The worktree is deliberately left in place: the files a run writes are
	// usually the point of running it, and an agent that ends by asking a
	// question needs a workspace in which to be answered.
	return true
}

func (s *Service) handleVisibleAutomationTurnComplete(
	ctx context.Context, taskID, sessionID, turnID, stopReason string, isError bool, errMsg string,
) bool {
	if turnID != "" {
		s.markAutomationRunTerminalForTurn(
			ctx,
			taskID,
			sessionID,
			turnID,
			!isError && stopReason != agentEventError && stopReason != stopReasonCancelled,
			errMsg,
		)
	}
	return false
}

// finalizeAutomationRun flips an automation run's row from task_created →
// succeeded|failed when its agent terminates, so max_concurrent_runs frees up
// without anyone archiving anything. Keyed on automation origin so both hidden
// coordinator tasks and visible automation-created tasks are settled while
// regular tasks remain untouched.
func (s *Service) finalizeAutomationRun(ctx context.Context, taskID string, success bool, errMsg string) {
	if taskID == "" {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	if !isAutomationTaskOrigin(task.Origin) {
		return
	}
	if task.Origin == models.TaskOriginAutomationTask {
		// Visible automation tasks may share one session across multiple exact
		// runs. The stream completion path settles the current bound turn; the
		// process-exit fallback must never settle a different run by task ID.
		lookup, ok := s.repo.(interface {
			GetTaskSessionByTaskID(context.Context, string) (*models.TaskSession, error)
		})
		if !ok || s.turnService == nil {
			return
		}
		session, sessionErr := lookup.GetTaskSessionByTaskID(ctx, taskID)
		if sessionErr != nil || session == nil {
			return
		}
		turn, turnErr := s.turnService.GetActiveTurn(ctx, session.ID)
		if turnErr != nil || turn == nil {
			return
		}
		s.markAutomationRunTerminalForTurn(ctx, taskID, session.ID, turn.ID, success, errMsg)
		return
	}

	s.markAutomationRunTerminal(ctx, taskID, success, errMsg)
}

func (s *Service) markAutomationRunTerminal(ctx context.Context, taskID string, success bool, errMsg string) {
	if s.automationService != nil {
		var markErr error
		if success {
			markErr = s.automationService.MarkRunSucceededByTaskID(ctx, taskID)
		} else {
			markErr = s.automationService.MarkRunFailedByTaskID(ctx, taskID, errMsg)
		}
		if markErr != nil {
			s.logger.Warn("failed to update automation run terminal status",
				zap.String("task_id", taskID),
				zap.Bool("success", success),
				zap.Error(markErr))
		}
	}
	// Every terminal transition is also the moment one more run enters the
	// retention window and pushes the oldest one out, so this is the only hook
	// that keeps up with the firing rate on its own — no sweeper, no schedule.
	s.pruneAutomationRunWorktrees(ctx, taskID)
}

func (s *Service) markAutomationRunTerminalForTurn(ctx context.Context, taskID, sessionID, turnID string, success bool, errMsg string) {
	if turnID == "" {
		s.markAutomationRunTerminal(ctx, taskID, success, errMsg)
		return
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		s.markAutomationRunTerminal(ctx, taskID, success, errMsg)
		return
	}
	status := automation.RunStatusFailed
	if success {
		status = automation.RunStatusSucceeded
	}
	if err := binding.MarkRunTerminalByBinding(ctx, taskID, sessionID, turnID, status, errMsg); err != nil {
		s.logger.Warn("failed to update automation run by exact turn binding",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
	}
}

// pruneAutomationRunWorktrees reclaims the workspaces of the runs that just
// aged out of this automation's retention window. Run rows, error messages and
// transcripts are untouched: an old run stays readable, it just no longer has a
// checkout to be replied in.
//
// Nothing here can fail the run. The firing already happened and its outcome is
// already recorded; a reclaim that doesn't happen costs disk, while an error
// propagated from here would turn a successful automation into a failed one and
// leave the run row disagreeing with what the agent actually did.
func (s *Service) pruneAutomationRunWorktrees(ctx context.Context, taskID string) {
	if s.worktreeReaper == nil || taskID == "" {
		return
	}
	retention, ok := s.automationService.(automationRunRetention)
	if !ok {
		return
	}
	agedOut, err := retention.PrunableRunTaskIDs(ctx, taskID, automation.DefaultRunWorktreeRetention)
	if err != nil {
		// Warned, not returned. The retry queue drained below holds workspaces
		// already known to have survived a removal, and they are owed
		// regardless of whether this automation's aged-out lookup succeeded —
		// a failed query is no reason to keep sitting on disk nobody wants.
		s.logger.Warn("failed to list automation runs past the worktree retention window",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	for _, agedOutTaskID := range s.automationWorkspaceSweepOrder(agedOut) {
		if task, taskErr := s.repo.GetTask(ctx, agedOutTaskID); taskErr == nil && task != nil && task.Origin == models.TaskOriginAutomationTask {
			continue
		}
		s.reclaimAutomationRunWorkspace(ctx, retention, agedOutTaskID)
	}
}

// reclaimAutomationRunWorkspace removes one aged-out run's worktrees, leaving
// its branch behind: the commits a run produced are usually the reason it was
// worth running, and they cost a ref rather than a checkout. A run's task can
// own several worktrees (one per repository), so all of them go.
func (s *Service) reclaimAutomationRunWorkspace(
	ctx context.Context, retention automationRunRetention, taskID string,
) {
	worktrees, err := s.worktreeReaper.GetAllByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list worktrees of an aged-out automation run",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	for _, wt := range worktrees {
		if wt == nil || !automationWorkspaceNeedsReclaiming(wt) {
			continue
		}
		// Re-asked per worktree rather than once per task, because every
		// removal before this one widened the gap since selection. The whole
		// task is abandoned the moment it looks live: its worktrees are one
		// agent's working set, and reclaiming the rest of them out from under
		// a live turn is the same data loss in a smaller package.
		if s.automationRunWentLive(ctx, retention, taskID) {
			return
		}
		s.removeAgedOutRunWorktree(ctx, taskID, wt)
	}
}

// automationWorkspaceNeedsReclaiming decides whether a worktree row is still
// costing disk, from the disk rather than from the row.
//
// GetAllByTaskID reports deleted rows too, so some skip rule is needed or an
// already-reclaimed checkout would be handed back to git on every firing. The
// tempting rule — skip anything the row calls deleted — trusts a claim the
// worktree manager does not actually stand behind: removeWorktree logs a
// failed directory removal at Warn and then marks the row deleted and returns
// nil regardless. A row saying "deleted" therefore means "somebody tried",
// not "the bytes are gone", and taking it at face value is how a retention
// feature ends up freeing nothing while reporting that it freed everything.
//
// So a deleted row still gets reclaimed if its directory is there, which is
// also what makes such a failure retryable rather than terminal.
func automationWorkspaceNeedsReclaiming(wt *worktree.Worktree) bool {
	if wt.Status != worktree.StatusDeleted {
		return true
	}
	return automationWorkspaceOnDisk(wt.Path)
}

// automationWorkspaceOnDisk reports whether a worktree's directory is still
// present. Lstat, not Stat: a dangling symlink left where a checkout used to
// be is still an entry that has to go, and following the link would call it
// gone. An empty path names nothing, so there is nothing to reclaim.
func automationWorkspaceOnDisk(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Lstat(path)
	return err == nil
}

// automationRunWentLive re-checks, immediately before a removal, whether the
// run has gone back to work since it was selected. See
// automation.Store.RunWorkspaceInUse for why the check cannot live in the
// selection query alone and why the worktree manager's own reference count
// does not cover this case.
//
// A failed check counts as live. The cost of being wrong that way is one
// checkout kept until the next firing; the cost of being wrong the other way
// is a user's agent losing its working tree mid-turn.
func (s *Service) automationRunWentLive(
	ctx context.Context, retention automationRunRetention, taskID string,
) bool {
	inUse, err := retention.RunWorkspaceInUse(ctx, taskID)
	if err != nil {
		s.logger.Warn("could not re-check whether an aged-out automation run went live; keeping its workspace",
			zap.String("task_id", taskID),
			zap.Error(err))
		return true
	}
	if inUse {
		s.logger.Info("kept the workspace of an aged-out automation run that went live before it could be reclaimed",
			zap.String("task_id", taskID))
	}
	return inUse
}

// removeAgedOutRunWorktree removes one worktree and verifies the result before
// claiming anything.
//
// The verification is the point. RemoveByID returning nil is not evidence that
// the checkout is gone — the manager swallows a failed directory removal (see
// automationWorkspaceNeedsReclaiming) — so a sweep that logged success on a nil
// error was reporting reclaimed disk that was still fully occupied, and the run
// then dropped out of the candidate set for good. Stat-ing the path afterwards
// is the only honest confirmation available from this side of the interface.
//
// Neither failure path is fatal to the run: the firing already happened and its
// outcome is already recorded. They are queued instead, so the next
// finalization tries again rather than writing the workspace off.
func (s *Service) removeAgedOutRunWorktree(ctx context.Context, taskID string, wt *worktree.Worktree) {
	if err := s.worktreeReaper.RemoveByID(ctx, wt.ID, false); err != nil {
		if errors.Is(err, worktree.ErrWorktreeNotFound) {
			return
		}
		s.logger.Warn("failed to reclaim the workspace of an aged-out automation run",
			zap.String("task_id", taskID),
			zap.String("worktree_id", wt.ID),
			zap.Error(err))
		s.queueAutomationWorkspaceReclaim(taskID)
		return
	}
	if automationWorkspaceOnDisk(wt.Path) {
		s.logger.Error("workspace of an aged-out automation run survived its removal; no disk was reclaimed",
			zap.String("task_id", taskID),
			zap.String("worktree_id", wt.ID),
			zap.String("path", wt.Path))
		s.queueAutomationWorkspaceReclaim(taskID)
		return
	}
	s.logger.Info("reclaimed the workspace of an aged-out automation run",
		zap.String("task_id", taskID),
		zap.String("worktree_id", wt.ID),
		zap.Int("retention", automation.DefaultRunWorktreeRetention))
}

// maxPendingAutomationWorkspaceReclaims bounds the retry queue. An install
// where dozens of removals are failing has a problem retention cannot fix by
// remembering more of them, and the queue must not become the leak it exists
// to stop.
const maxPendingAutomationWorkspaceReclaims = 64

// queueAutomationWorkspaceReclaim marks a run's workspace as still owed.
//
// Retention's candidate set is "runs that still have a live worktree row", and
// a removal that fails after the manager has already marked the row deleted
// leaves a run that no query will ever offer again — its directory is on disk,
// invisible to the sweep that was supposed to reclaim it. This queue is that
// run's only way back, so the next finalization retries it alongside the runs
// that aged out naturally.
//
// In-memory on purpose: it is a retry hint, not a record. A restart loses it,
// which costs the disk one directory until something else cleans it up — the
// Error log above is what a human is meant to act on in that case.
func (s *Service) queueAutomationWorkspaceReclaim(taskID string) {
	s.unreclaimedWorkspacesMu.Lock()
	defer s.unreclaimedWorkspacesMu.Unlock()
	if _, queued := s.unreclaimedWorkspaces[taskID]; queued {
		return
	}
	if len(s.unreclaimedWorkspaces) >= maxPendingAutomationWorkspaceReclaims {
		s.logger.Warn("dropping an automation workspace from the reclaim retry queue; too many removals are failing",
			zap.String("task_id", taskID),
			zap.Int("queued", len(s.unreclaimedWorkspaces)))
		return
	}
	if s.unreclaimedWorkspaces == nil {
		s.unreclaimedWorkspaces = map[string]struct{}{}
	}
	s.unreclaimedWorkspaces[taskID] = struct{}{}
}

// automationWorkspaceSweepOrder is what this finalization will actually try:
// the runs whose removal previously failed, then the ones that have just aged
// out, with the overlap collapsed so a run in both lists is attempted once.
//
// The retries lead because they are known-wasted disk, whereas an aged-out run
// is merely eligible. Draining unconditionally keeps the queue from outliving
// the problem — a task that fails again re-queues itself from
// removeAgedOutRunWorktree, and one that has since been cleaned up simply
// finds nothing to do.
func (s *Service) automationWorkspaceSweepOrder(agedOut []string) []string {
	s.unreclaimedWorkspacesMu.Lock()
	pending := make([]string, 0, len(s.unreclaimedWorkspaces))
	for taskID := range s.unreclaimedWorkspaces {
		pending = append(pending, taskID)
	}
	clear(s.unreclaimedWorkspaces)
	s.unreclaimedWorkspacesMu.Unlock()
	// Map iteration order is random; the sweep's log and its test both read
	// better when the same failures are retried in the same order every time.
	sort.Strings(pending)

	seen := make(map[string]struct{}, len(pending)+len(agedOut))
	order := make([]string, 0, len(pending)+len(agedOut))
	for _, taskID := range append(pending, agedOut...) {
		if _, duplicate := seen[taskID]; duplicate {
			continue
		}
		seen[taskID] = struct{}{}
		order = append(order, taskID)
	}
	return order
}

func (s *Service) stopAutomationAgent(
	ctx context.Context, taskID, sessionID string, session *models.TaskSession,
) {
	if s.agentManager == nil {
		return
	}
	executionID := ""
	if session != nil {
		executionID = session.AgentExecutionID
	}
	if executionID == "" {
		running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to resolve automation execution for turn-complete stop",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}
		if running != nil {
			executionID = running.AgentExecutionID
		}
	}
	if executionID == "" {
		return
	}
	if err := s.agentManager.StopAgent(ctx, executionID, false); err != nil {
		s.logger.Warn("failed to stop automation agent on turn complete",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("agent_execution_id", executionID),
			zap.Error(err))
	}
}
