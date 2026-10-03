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
	BindRunTask(ctx context.Context, runID, taskID, repositoryReason string) error
	BindRun(ctx context.Context, runID, taskID, sessionID, turnID string, action automation.ThreadAction, reason string) error
	MarkRunTerminal(ctx context.Context, runID, sessionID, turnID string, status automation.RunStatus, errMsg string) error
	MarkRunTerminalByBinding(ctx context.Context, taskID, sessionID, turnID string, status automation.RunStatus, errMsg string) error
}
type automationOpenRunLookup interface {
	ListOpenRunsByTaskID(ctx context.Context, taskID string) ([]*automation.AutomationRun, error)
}

type deferredAutomationRunCloser interface {
	MarkDeferredRunFailedByTaskID(ctx context.Context, taskID, errMsg string) error
}

type automationDispatchReader interface {
	GetAutomationForDispatch(ctx context.Context, id string) (*automation.Automation, error)
}

type managedAutomationRunDispatcher interface {
	DispatchManagedAutomationRun(ctx context.Context, runID string) error
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
type automationRetryFailure interface {
	FinalizeAutomationRetryFailure(ctx context.Context, runID string, generation int64, raw error, phase string) (*automation.AutomationRun, error)
}
type automationRetryBinding interface {
	PromoteClaimedRetry(ctx context.Context, runID, token string, generation int64) error
}
type automationRetryRelease interface {
	ReleaseRetryClaim(ctx context.Context, runID, token string, generation int64) error
}

type automationRetryCapacityDefer interface {
	DeferRetryClaimForCapacity(ctx context.Context, runID, token string, generation int64) error
}
type automationRetryCapacity interface {
	RetryClaimCapacityAvailable(ctx context.Context, runID string) (bool, error)
}
type automationRetrySuccess interface {
	MarkAutomationRetrySucceeded(ctx context.Context, runID string, generation int64) error
}
type automationRetryReceipt interface {
	AcknowledgeRetryEvent(ctx context.Context, eventID, leaseToken, runID string, version int64) error
}
type automationRetryOperation interface {
	BeginRetryTaskOperation(ctx context.Context, runID string, generation int64) (*automation.RetryOperation, error)
	CommitRetryTaskOperation(ctx context.Context, runID string, generation int64, leaseToken, taskID string) error
}

type automationRetryRecoveryOperation interface {
	GetRetryTaskOperation(ctx context.Context, runID string, generation int64) (*automation.RetryOperation, error)
}

type automationRetryContinuationOperation interface {
	CommitRetryContinuationOperation(
		ctx context.Context,
		runID string,
		generation int64,
		leaseToken string,
		dispatch automation.RunDispatch,
	) error
}

type automationRetryAmbiguous interface {
	MarkRetryOperationAmbiguous(
		ctx context.Context,
		runID string,
		generation int64,
		leaseToken string,
		dispatch automation.RunDispatch,
	) error
}

type automationRetryOperationFence interface {
	VerifyRetryTaskOperation(ctx context.Context, runID string, generation int64, leaseToken string) error
}
type automationRetryRunLock interface {
	WithRetryRunLock(ctx context.Context, runID string, fn func(context.Context) error) error
}

const (
	retryOperationCommittedState = "committed"
	retryOperationAmbiguousState = "ambiguous"
)

var errDeterministicCommittedRetryTask = errors.New("deterministic committed retry task failure")

func committedRetryTaskDeterministicError(message string) error {
	return fmt.Errorf("%w: %s", errDeterministicCommittedRetryTask, message)
}

func isDeterministicCommittedRetryTaskError(err error) bool {
	return errors.Is(err, errDeterministicCommittedRetryTask)
}

const retryAutomationRunIDMetadataKey = "automation_run_id"

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

// taskLifecycleDeleter owns destructive task cleanup for an abandoned
// automation firing. It deliberately stays off repoStore so repository
// implementations cannot accidentally perform lifecycle-blind deletion.
type taskLifecycleDeleter interface {
	DeleteTaskWithLifecycle(ctx context.Context, id string) error
}

// automationRunRetention names the runs whose workspaces have aged out, and
// answers whether one of them has since gone live. Kept off AutomationService
// and asserted at the call site so repository stubs need not implement
// retention-only capabilities.
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
	if s.executor != nil {
		s.executor.SetSelectedWorktreeRecoveryAdmission(mgr.AdmitRecovery)
	}
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

func automationExecutionTriggerData(
	evt *automation.AutomationTriggeredEvent,
	snapshot *automation.RetryLaunchConfigSnapshot,
) json.RawMessage {
	if snapshot != nil && len(snapshot.TriggerData) > 0 {
		return snapshot.TriggerData
	}
	if len(evt.SafeTriggerData) > 0 {
		return evt.SafeTriggerData
	}
	return evt.TriggerData
}

// handleAutomationTriggered creates a task when an automation trigger fires.
//
//nolint:nestif // The event path preserves run admission and managed-delivery reconciliation order.
func (s *Service) handleAutomationTriggered(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*automation.AutomationTriggeredEvent)
	if !ok {
		return nil
	}

	s.logger.Info("automation trigger received",
		zap.String("automation_id", evt.AutomationID),
		zap.String("trigger_type", string(evt.TriggerType)))

	if s.automationService == nil {
		s.logger.Warn("automation service not configured")
		return nil
	}
	if reader, ok := s.automationService.(automationDispatchReader); ok {
		a, loadErr := reader.GetAutomationForDispatch(ctx, evt.AutomationID)
		if loadErr != nil {
			s.logger.Warn("failed to inspect automation destination", zap.String("automation_id", evt.AutomationID), zap.Error(loadErr))
			return loadErr
		}
		if a == nil {
			return nil
		}
		if a.TaskMode == automation.TaskModeManagedConversation {
			dispatcher, available := s.automationService.(managedAutomationRunDispatcher)
			if !available {
				return fmt.Errorf("managed conversation automation delivery unavailable")
			}
			go func() {
				if err := dispatcher.DispatchManagedAutomationRun(context.Background(), evt.RunID); err != nil {
					s.logger.Error("managed conversation automation delivery failed",
						zap.String("run_id", evt.RunID), zap.Error(err))
				}
			}()
			return nil
		}
	}
	if s.reviewTaskCreator == nil {
		s.logger.Warn("automation task creator not configured")
		return nil
	}

	if evt.TriggerType == automation.TriggerTypePluginEvent {
		delivery, ok := s.automationService.(interface {
			ClaimPluginWebhookRun(context.Context, string) (bool, error)
			CompletePluginWebhookRun(context.Context, string) error
		})
		if !ok {
			return fmt.Errorf("plugin webhook dispatch unavailable")
		}
		go func() {
			claimed, err := delivery.ClaimPluginWebhookRun(context.Background(), evt.RunID)
			if err != nil {
				s.logger.Error("webhook dispatch claim failed", zap.Error(err))
				return
			}
			if !claimed {
				return
			}
			s.createAutomationTask(context.Background(), evt)
			if err := delivery.CompletePluginWebhookRun(context.Background(), evt.RunID); err != nil {
				s.logger.Error("webhook dispatch completion failed", zap.Error(err))
			}
		}()
		return nil
	}
	go s.createAutomationTask(context.Background(), evt)
	return nil
}

func (s *Service) createAutomationTask(ctx context.Context, evt *automation.AutomationTriggeredEvent) {
	if evt.RunID != "" {
		if locker, ok := s.automationService.(automationRetryRunLock); ok {
			if err := locker.WithRetryRunLock(ctx, evt.RunID, func(lockedCtx context.Context) error {
				s.createAutomationTaskLocked(lockedCtx, evt)
				return nil
			}); err != nil {
				s.logger.Debug("retry automation event was not admitted",
					zap.String("run_id", evt.RunID), zap.Error(err))
			}
			return
		}
	}
	s.createAutomationTaskLocked(ctx, evt)
}

//nolint:gocognit,cyclop,funlen,maintidx // Retry admission, provider effects, and binding share one critical section.
func (s *Service) createAutomationTaskLocked(ctx context.Context, evt *automation.AutomationTriggeredEvent) {
	var retryRun *automation.AutomationRun
	var retrySnapshot *automation.RetryLaunchConfigSnapshot
	var a *automation.Automation
	var retryOperation *automation.RetryOperation
	//nolint:nestif // Claim promotion is a single identity-fenced boundary.
	if binding, ok := s.automationService.(automationRunBinding); ok && evt.RunID != "" {
		candidate, _ := binding.GetRun(ctx, evt.RunID)
		if candidate != nil && candidate.RetryGroupID != "" {
			retryRun = candidate
			if evt.RetryClaimToken != "" {
				retryBinding, retryOK := s.automationService.(automationRetryBinding)
				if !retryOK {
					s.recordFailedRun(ctx, evt, "retry claim unavailable")
					return
				}
				if capacity, capacityOK := s.automationService.(automationRetryCapacity); capacityOK {
					available, capacityErr := capacity.RetryClaimCapacityAvailable(ctx, evt.RunID)
					if capacityErr != nil || !available {
						if deferer, deferOK := s.automationService.(automationRetryCapacityDefer); deferOK {
							_ = deferer.DeferRetryClaimForCapacity(ctx, evt.RunID, evt.RetryClaimToken, evt.RetryGroupGeneration)
						} else if release, releaseOK := s.automationService.(automationRetryRelease); releaseOK {
							_ = release.ReleaseRetryClaim(ctx, evt.RunID, evt.RetryClaimToken, evt.RetryGroupGeneration)
						}
						return
					}
				}
				if err := retryBinding.PromoteClaimedRetry(ctx, evt.RunID, evt.RetryClaimToken, evt.RetryGroupGeneration); err != nil {
					return
				}
			}
			snapshot, snapshotErr := automation.DecodeRetryLaunchConfigSnapshot(
				retryRun.RetryLaunchConfigSnapshot, retryRun.RetryLaunchConfigVersion,
			)
			if snapshotErr != nil {
				s.recordFailedRun(ctx, evt, snapshotErr.Error())
				return
			}
			retrySnapshot = &snapshot
			evt.AutomationID = snapshot.AutomationID
			evt.TriggerID = snapshot.TriggerID
			evt.TriggerType = snapshot.TriggerType
			evt.DedupKey = snapshot.DedupKey
			evt.RetryGroupGeneration = retryRun.RetryGroupGeneration
			a = automationFromRetrySnapshot(snapshot)
		}
	}
	retryInitialTriggerData := retryRun != nil && evt.RetryClaimToken == "" && len(evt.TriggerData) > 0
	initialTriggerData := evt.TriggerData
	evt.TriggerData = automationExecutionTriggerData(evt, retrySnapshot)
	if retryInitialTriggerData {
		evt.TriggerData = initialTriggerData
	}
	retryOperation, operationErr := s.beginRetryTaskOperation(ctx, evt, retryRun)
	if operationErr != nil {
		if !errors.Is(operationErr, automation.ErrRetryGenerationMismatch) &&
			!errors.Is(operationErr, automation.ErrRetryOperationUndispatchable) {
			s.recordFailedRun(ctx, evt, operationErr.Error())
		}
		return
	}
	if retryRun == nil {
		var loadErr error
		a, loadErr = s.automationService.GetAutomation(ctx, evt.AutomationID)
		if loadErr != nil || a == nil {
			s.logger.Error("failed to load automation for trigger",
				zap.String("automation_id", evt.AutomationID), zap.Error(loadErr))
			s.recordFailedRun(ctx, evt, "automation not found")
			return
		}
		if !a.Enabled {
			s.logger.Debug("automation disabled, skipping",
				zap.String("automation_id", evt.AutomationID))
			s.recordFailedRun(ctx, evt, "automation is disabled")
			return
		}
	} else if a == nil {
		s.recordFailedRun(ctx, evt, "retry launch configuration unavailable")
		return
	}
	if evt.RetryAmbiguousRecovery {
		s.recoverAmbiguousRetry(ctx, a, evt, retryOperation)
		return
	}
	// Initial deliveries use the quoted agent interpolation path. Retry
	// attempts use the immutable bounded prompt snapshot.
	prompt := automation.InterpolateAgentPrompt(a.Prompt, evt.TriggerType, evt.TriggerData)
	if retrySnapshot != nil && !retryInitialTriggerData {
		prompt = retrySnapshot.ResolvedPrompt
	} else if retryRun != nil && !retryInitialTriggerData && retryRun.RetryResolvedPrompt != "" {
		prompt = retryRun.RetryResolvedPrompt
	}
	if prompt == "" {
		prompt = fmt.Sprintf("Automation '%s' triggered by %s", a.Name, evt.TriggerType)
	}

	title := s.resolveAutomationTaskTitle(a, evt)
	if retrySnapshot != nil {
		if retryRun.DisplayTitle != "" {
			title = retryRun.DisplayTitle
		} else if retrySnapshot.ResolvedTitle != "" {
			title = retrySnapshot.ResolvedTitle
		}
	} else if binding, ok := s.automationService.(automationRunBinding); ok && evt.RunID != "" {
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
	if evt.RunID != "" {
		metadata[retryAutomationRunIDMetadataKey] = evt.RunID
	}
	if evt.TriggerType == automation.TriggerTypeGitHubPRMerged {
		var triggerData struct {
			TaskID string `json:"task_id"`
		}
		_ = json.Unmarshal(evt.TriggerData, &triggerData)
		metadata[models.MetaKeyAutomationTargetTaskID] = triggerData.TaskID
	}

	var task *models.Task
	var continuationSession *models.TaskSession
	action := automation.ThreadActionCreated
	var reasons automationRunReasons
	var taskErr error
	if retryOperation != nil && retryOperation.State == retryOperationCommittedState {
		task, taskErr = s.adoptCommittedRetryTask(ctx, a, evt, retryOperation)
		reasons.Thread = retryRun.ThreadReason
		reasons.Repository = retryRun.RepositoryReason
	} else {
		task, continuationSession, action, reasons, taskErr = s.prepareAutomationTask(
			ctx, a, evt, title, prompt, metadata, retryOperation,
		)
	}
	if taskErr != nil {
		if retryOperation != nil && retryOperation.State == retryOperationCommittedState {
			if isDeterministicCommittedRetryTaskError(taskErr) {
				s.recordFailedRun(ctx, evt, taskErr.Error())
				return
			}
			s.logger.Error("failed to adopt committed retry task",
				zap.String("automation_id", a.ID),
				zap.String("run_id", evt.RunID),
				zap.String("task_id", retryOperation.ExternalTaskID),
				zap.Error(taskErr))
			return
		}
		s.recordFailedRun(ctx, evt, taskErr.Error())
		return
	}
	if retryOperation != nil && retryOperation.State == retryOperationCommittedState {
		adopted, adoptErr := s.adoptCommittedRetryContinuation(ctx, evt.RunID, task.ID, retryOperation)
		if adoptErr != nil {
			s.logger.Warn("failed to bind committed retry continuation; leaving receipt pending",
				zap.String("automation_id", a.ID),
				zap.String("run_id", evt.RunID),
				zap.String("task_id", task.ID),
				zap.Error(adoptErr))
			return
		}
		if adopted {
			s.acknowledgeRetryEventAfterBinding(ctx, evt)
			return
		}
	}
	if retryRun != nil && retryOperation != nil &&
		retryOperation.State == retryOperationCommittedState &&
		s.retryRunHasExactBinding(ctx, evt.RunID) {
		s.acknowledgeRetryEventAfterBinding(ctx, evt)
		return
	}
	if retryOperation != nil && retryOperation.State != retryOperationCommittedState &&
		continuationSession == nil {
		operationService, operationOK := s.automationService.(automationRetryOperation)
		if !operationOK {
			s.deleteAbandonedTask(ctx, a.ID, task.ID)
			s.recordFailedRun(ctx, evt, "retry operation ledger unavailable")
			return
		}
		if err := operationService.CommitRetryTaskOperation(ctx, evt.RunID,
			evt.RetryGroupGeneration, retryOperation.LeaseToken, task.ID); err != nil {
			s.deleteAbandonedTask(ctx, a.ID, task.ID)
			s.recordFailedRun(ctx, evt, err.Error())
			return
		}
		retryOperation.State = retryOperationCommittedState
		retryOperation.ExternalTaskID = task.ID
	}

	// A committed provider task is durable ownership. If the first binding
	// attempt fails, retry that exact binding and leave the operation intact
	// when reconciliation cannot complete; creating a successor would risk
	// launching a duplicate provider task.
	if err := s.recordSuccessRun(ctx, evt, task.ID, reasons.Repository); err != nil {
		if retryOperation != nil && retryOperation.State == retryOperationCommittedState {
			committedTaskID := retryOperation.ExternalTaskID
			if committedTaskID == "" {
				committedTaskID = task.ID
			}
			if bindErr := s.recordSuccessRun(ctx, evt, committedTaskID, reasons.Repository); bindErr != nil {
				s.logger.Error("failed to reconcile committed retry task binding",
					zap.String("automation_id", a.ID),
					zap.String("task_id", committedTaskID),
					zap.Error(bindErr))
				return
			}
			task.ID = committedTaskID
		} else {
			s.logger.Error("failed to record automation run; abandoning the firing",
				zap.String("automation_id", a.ID),
				zap.String("task_id", task.ID),
				zap.Error(err))
			if action != automation.ThreadActionResumed {
				s.deleteAbandonedTask(ctx, a.ID, task.ID)
			}
			s.recordFailedRun(ctx, evt, err.Error())
			return
		}
	}

	// Associate PR with task for github_pr triggers (same as PR Watcher).
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		if repositories, _ := s.resolveAutomationRepository(ctx, a, evt); len(repositories) > 0 {
			s.associateAutomationPR(ctx, task.ID, repositories[0].RepositoryID, evt.TriggerData)
		}
	}

	s.logger.Info("created automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID),
		zap.String("trigger_type", string(evt.TriggerType)))

	if continuationSession != nil {
		if s.dispatchAutomationContinuation(ctx, a, task, continuationSession, prompt, metadata, evt.RunID, action, reasons.Thread, retryOperation) && retryRun != nil {
			s.acknowledgeRetryEventAfterBinding(ctx, evt)

		}
		return
	}

	// Auto-start unconditionally: nobody opens an automation's task to drag
	// it, so the trigger has to be the start signal. A workflow step's
	// auto_start_agent setting is irrelevant here because the automation
	// trigger is the start signal for both hidden runs and visible normal tasks.
	if s.autoStartAutomationTaskForRun(
		ctx, a, task, task.WorkflowStepID, evt.RunID, action, reasons.Thread,
		retryOperation != nil && retryOperation.State == retryOperationCommittedState,
		retryOperation,
	) && retryRun != nil {
		s.acknowledgeRetryEventAfterBinding(ctx, evt)
	}
}
func automationFromRetrySnapshot(snapshot automation.RetryLaunchConfigSnapshot) *automation.Automation {
	repositoryIDs := make([]string, 0, len(snapshot.Repositories))
	for _, repository := range snapshot.Repositories {
		repositoryIDs = append(repositoryIDs, repository.RepositoryID)
	}
	return &automation.Automation{
		ID:                 snapshot.AutomationID,
		WorkspaceID:        snapshot.WorkspaceID,
		Name:               snapshot.Name,
		WorkflowID:         snapshot.WorkflowID,
		WorkflowStepID:     snapshot.WorkflowStepID,
		AgentProfileID:     snapshot.AgentProfileID,
		ExecutorProfileID:  snapshot.ExecutorProfileID,
		Prompt:             snapshot.Prompt,
		TaskTitleTemplate:  snapshot.TaskTitleTemplate,
		TaskMode:           snapshot.TaskMode,
		RepositoryMode:     snapshot.RepositoryMode,
		Repositories:       append([]automation.AutomationRepository(nil), snapshot.Repositories...),
		RepositoryIDs:      repositoryIDs,
		ContinuationPolicy: snapshot.ContinuationPolicy,
		MaxConcurrentRuns:  snapshot.MaxConcurrentRuns,
		ContinuationTaskID: snapshot.ContinuationTaskID,
		RetryPolicy:        snapshot.RetryPolicy,
		Enabled:            true,
	}
}

// automationRunReasons carries both disposition tokens recorded for a firing
// in one struct rather than as two adjacent bare string returns — two bare
// strings threaded through prepareAutomationTask/createAutomationTask are
// compiler-indistinguishable, so a transposed pair would compile silently
// and corrupt the audit trail.
type automationRunReasons struct {
	Thread     string
	Repository string
}

func (s *Service) recoverAmbiguousRetry(
	ctx context.Context,
	a *automation.Automation,
	evt *automation.AutomationTriggeredEvent,
	retryOperation *automation.RetryOperation,
) {
	if retryOperation == nil || retryOperation.State != retryOperationAmbiguousState {
		s.recordFailedRun(ctx, evt, "ambiguous retry operation is unavailable")
		return
	}
	task, taskErr := s.adoptCommittedRetryTask(ctx, a, evt, retryOperation)
	if taskErr != nil {
		if isDeterministicCommittedRetryTaskError(taskErr) {
			s.recordFailedRun(ctx, evt, taskErr.Error())
		} else {
			s.logger.Warn("failed to resolve ambiguous retry task",
				zap.String("run_id", evt.RunID), zap.Error(taskErr))
		}
		return
	}
	adopted, adoptErr := s.adoptCommittedRetryContinuation(ctx, evt.RunID, task.ID, retryOperation)
	if adoptErr != nil || !adopted {
		s.logger.Warn("failed to bind ambiguous retry continuation",
			zap.String("run_id", evt.RunID), zap.String("task_id", task.ID), zap.Error(adoptErr))
		return
	}
	s.acknowledgeRetryEventAfterBinding(ctx, evt)
}

func (s *Service) adoptCommittedRetryTask(
	ctx context.Context,
	a *automation.Automation,
	evt *automation.AutomationTriggeredEvent,
	operation *automation.RetryOperation,
) (*models.Task, error) {
	if operation == nil || operation.ExternalTaskID == "" {
		return nil, committedRetryTaskDeterministicError("committed retry task identity is missing")
	}
	if s.repo == nil {
		return nil, errors.New("committed retry task lookup unavailable")
	}
	task, err := s.repo.GetTask(ctx, operation.ExternalTaskID)
	if err != nil {
		if automationRunExecutionGone(err) {
			return nil, committedRetryTaskDeterministicError("committed retry task is missing")
		}
		return nil, err
	}
	if task == nil {
		return nil, committedRetryTaskDeterministicError("committed retry task is missing")
	}
	if task.WorkspaceID != a.WorkspaceID {
		return nil, committedRetryTaskDeterministicError("committed retry task belongs to a different workspace")
	}
	allowSharedContinuation := operation.ExternalSessionID != "" && operation.ExternalTurnID != ""
	if err := validateRetryTaskOwnership(task, a, evt, automationTaskOrigin(a), allowSharedContinuation); err != nil {
		return nil, committedRetryTaskDeterministicError(err.Error())
	}
	return task, nil
}

func validateRetryTaskOwnership(
	task *models.Task,
	a *automation.Automation,
	evt *automation.AutomationTriggeredEvent,
	expectedOrigin string,
	allowSharedContinuation bool,
) error {
	if task == nil || a == nil || evt == nil {
		return errors.New("retry task ownership context is incomplete")
	}
	if task.WorkspaceID != a.WorkspaceID {
		return errors.New("retry task belongs to a different workspace")
	}
	if task.Origin != expectedOrigin {
		return errors.New("retry task has an incompatible origin")
	}
	if !allowSharedContinuation && task.ExternalID != evt.RetryExternalID {
		return errors.New("retry task has an incompatible external identity")
	}
	if metadataString(task.Metadata, "automation_id") != a.ID ||
		metadataString(task.Metadata, retryAutomationRunIDMetadataKey) != evt.RunID {
		return errors.New("retry task has an incompatible automation identity")
	}
	return nil
}

func (s *Service) adoptCommittedRetryContinuation(
	ctx context.Context,
	runID string,
	taskID string,
	operation *automation.RetryOperation,
) (bool, error) {
	if operation == nil || (operation.ExternalSessionID == "" && operation.ExternalTurnID == "") {
		return false, nil
	}
	if operation.ExternalSessionID == "" || operation.ExternalTurnID == "" {
		return false, errors.New("committed retry continuation identity is incomplete")
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return false, errors.New("automation run binding unavailable")
	}
	if err := binding.BindRun(ctx, runID, taskID, operation.ExternalSessionID,
		operation.ExternalTurnID, automation.ThreadActionResumed,
		"recovered accepted continuation"); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) beginRetryTaskOperation(ctx context.Context, evt *automation.AutomationTriggeredEvent, retryRun *automation.AutomationRun) (*automation.RetryOperation, error) {
	if retryRun == nil {
		return nil, nil
	}
	if evt.RetryGroupGeneration == 0 {
		evt.RetryGroupGeneration = retryRun.RetryGroupGeneration
	}
	if evt.RetryExternalID == "" {
		evt.RetryExternalID = automation.RetryTaskExternalID(evt.RunID, evt.RetryGroupGeneration)
	}
	operationService, ok := s.automationService.(automationRetryOperation)
	if !ok {
		return nil, errors.New("retry operation ledger unavailable")
	}
	if evt.RetryAmbiguousRecovery {
		recoveryOperation, recoveryOK := s.automationService.(automationRetryRecoveryOperation)
		if !recoveryOK {
			return nil, errors.New("retry recovery operation ledger unavailable")
		}
		return recoveryOperation.GetRetryTaskOperation(ctx, evt.RunID, evt.RetryGroupGeneration)
	}
	return operationService.BeginRetryTaskOperation(ctx, evt.RunID, evt.RetryGroupGeneration)
}
func (s *Service) verifyRetryTaskOperation(ctx context.Context, evt *automation.AutomationTriggeredEvent, operations ...*automation.RetryOperation) error {
	if len(operations) == 0 || operations[0] == nil {
		return nil
	}
	fence, ok := s.automationService.(automationRetryOperationFence)
	if !ok {
		return errors.New("retry operation fence unavailable")
	}
	operation := operations[0]
	return fence.VerifyRetryTaskOperation(ctx, evt.RunID, evt.RetryGroupGeneration, operation.LeaseToken)
}

func (s *Service) prepareAutomationTask(
	ctx context.Context,
	a *automation.Automation,
	evt *automation.AutomationTriggeredEvent,
	title, prompt string,
	metadata map[string]interface{},
	operations ...*automation.RetryOperation,
) (*models.Task, *models.TaskSession, automation.ThreadAction, automationRunReasons, error) {
	action := automation.ThreadActionCreated
	reasons := automationRunReasons{Thread: "new task created for automation run"}
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
		task, continuationSession, reasons.Thread = s.findAutomationContinuation(ctx, a, evt)
		if task != nil {
			action = automation.ThreadActionResumed
		} else if a.ContinuationTaskID != "" {
			action = automation.ThreadActionReplaced
		}
	}
	if task != nil {
		// No repository resolution happens on this path (site 395 below never
		// executes), so the token is set here: the firing's disposition is
		// genuinely "inherited from the continued task", recorded on this new
		// run's own row. Highest-precedence, short-circuiting token.
		reasons.Repository = repositoryContinuationReused
		return task, continuationSession, action, reasons, nil
	}

	repositories, repoReason := s.resolveAutomationRepository(ctx, a, evt)
	reasons.Repository = repoReason
	if err := s.verifyRetryTaskOperation(ctx, evt, operations...); err != nil {
		return nil, nil, action, reasons, err
	}
	task, err := s.reviewTaskCreator.CreateReviewTask(ctx, &ReviewTaskRequest{
		WorkspaceID:    a.WorkspaceID,
		WorkflowID:     a.WorkflowID,
		WorkflowStepID: a.WorkflowStepID,
		Title:          title,
		Description:    prompt,
		Repositories:   repositories,
		Metadata:       metadata,
		Origin:         taskOrigin,
		ExternalID:     evt.RetryExternalID,
	})
	if err != nil {
		return nil, nil, action, reasons, fmt.Errorf("create automation task: %w", err)
	}
	if evt.RunID != "" {
		if err := validateRetryTaskOwnership(task, a, evt, taskOrigin, false); err != nil {
			return nil, nil, action, reasons, err
		}
	}
	if a.ContinuationPolicy != automation.ContinuationPolicyReuseThread {
		return task, nil, action, reasons, nil
	}
	state, ok := s.automationService.(automationContinuationState)
	if !ok {
		s.deleteAbandonedTask(ctx, a.ID, task.ID)
		return nil, nil, action, reasons, fmt.Errorf("automation continuation state is unavailable")
	}
	if err := state.SetContinuationTaskID(ctx, a.ID, task.ID); err != nil {
		s.deleteAbandonedTask(ctx, a.ID, task.ID)
		return nil, nil, action, reasons, err
	}
	if action == automation.ThreadActionReplaced {
		reasons.Thread = "previous continuation was unavailable; created a replacement task"
	}
	return task, nil, action, reasons, nil
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
	expected, _ := s.resolveAutomationRepository(ctx, a, evt)
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
	preserveTaskOnFailure bool,
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
	} else if errors.Is(err, automation.ErrRunDeferred) {
		// The run stays open and owns its task; the ceiling sweep replays the
		// queued start, so the task and the queued record must both remain.
		s.logger.Info("automation run start queued by the session ceiling",
			zap.String("operation", operation), zap.String("automation_id", automationID),
			zap.String("task_id", taskID), zap.String("run_id", runID))
		return true
	} else {
		if onFailure != nil {
			onFailure()
		}
		s.logger.Error("failed to dispatch automation run",
			zap.String("operation", operation), zap.String("automation_id", automationID),
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
	if !preserveTaskOnFailure {
		s.cleanupFailedAutomationTask(ctx, automationID, taskID, action)
	}
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
	if !s.markExactAutomationRunTerminal(ctx, runID, "", "", false, bindErr.Error()) && runID == "" {
		s.markAutomationRunTerminal(ctx, taskID, false, bindErr.Error())
	}
	return false
}

func (s *Service) commitRetryContinuationOperation(
	ctx context.Context,
	runID string,
	operation *automation.RetryOperation,
	dispatch automation.RunDispatch,
) error {
	if operation == nil {
		return nil
	}
	operationService, ok := s.automationService.(automationRetryContinuationOperation)
	if !ok {
		return errors.New("retry continuation operation ledger unavailable")
	}
	if err := operationService.CommitRetryContinuationOperation(
		ctx, runID, operation.GroupGeneration, operation.LeaseToken, dispatch); err != nil {
		return err
	}
	operation.State = retryOperationCommittedState
	operation.ExternalTaskID = dispatch.TaskID
	operation.ExternalSessionID = dispatch.SessionID
	operation.ExternalTurnID = dispatch.TurnID
	operation.LeaseToken = ""
	return nil
}

func (s *Service) markRetryOperationAmbiguous(
	ctx context.Context,
	runID string,
	operation *automation.RetryOperation,
	dispatch automation.RunDispatch,
) error {
	if operation == nil {
		return nil
	}
	operationService, ok := s.automationService.(automationRetryAmbiguous)
	if !ok {
		return errors.New("retry ambiguous operation ledger unavailable")
	}
	if err := operationService.MarkRetryOperationAmbiguous(
		ctx, runID, operation.GroupGeneration, operation.LeaseToken, dispatch,
	); err != nil {
		return err
	}
	operation.State = retryOperationAmbiguousState
	operation.ExternalTaskID = dispatch.TaskID
	operation.ExternalSessionID = dispatch.SessionID
	operation.ExternalTurnID = dispatch.TurnID
	operation.LeaseToken = ""
	return nil
}

func (s *Service) verifyRetryContinuationAdmission(
	ctx context.Context,
	runID string,
	operations ...*automation.RetryOperation,
) error {
	if len(operations) == 0 || operations[0] == nil {
		return nil
	}
	operation := operations[0]
	return s.verifyRetryTaskOperation(ctx, &automation.AutomationTriggeredEvent{
		RunID: runID, RetryGroupGeneration: operation.GroupGeneration,
	}, operation)
}

func (s *Service) dispatchAutomationContinuation(ctx context.Context, a *automation.Automation, task *models.Task, session *models.TaskSession, prompt string, metadata map[string]interface{}, runID string, action automation.ThreadAction, reason string, operations ...*automation.RetryOperation) bool {
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
	restoreIfUncommitted := func() {
		if len(operations) > 0 && operations[0] != nil &&
			operations[0].State == retryOperationCommittedState {
			return
		}
		restore()
	}
	dispatch := func() (automation.RunDispatch, error) {
		var err error
		snapshot, err = s.refreshAutomationContinuationMetadataWithSnapshot(ctx, task, metadata)
		if err != nil {
			return automation.RunDispatch{}, err
		}
		if err := s.verifyRetryContinuationAdmission(ctx, runID, operations...); err != nil {
			return automation.RunDispatch{}, err
		}
		var operation *automation.RetryOperation
		if len(operations) > 0 {
			operation = operations[0]
		}
		result, err := s.promptAutomationContinuation(ctx, task, session, prompt, runID, operation)
		if err != nil {
			restoreIfUncommitted()
			return automation.RunDispatch{}, err
		}
		return result, nil
	}
	if s.dispatchAutomationRun(ctx, a.ID, task.ID, session.ID, runID, action, reason, "continuation", dispatch, restoreIfUncommitted, false) {
		return s.retryRunHasExactBinding(ctx, runID)
	}

	dispatchResult, err := dispatch()
	if err != nil {
		s.logger.Error("failed to dispatch automation continuation",
			zap.String("automation_id", a.ID), zap.String("task_id", task.ID), zap.String("session_id", session.ID), zap.Error(err))
		if !s.markExactAutomationRunTerminal(ctx, runID, "", "", false, err.Error()) && runID == "" {
			s.markAutomationRunTerminal(ctx, task.ID, false, err.Error())
		}
		return false
	}
	if !s.bindAutomationRun(ctx, runID, dispatchResult.TaskID, dispatchResult.SessionID, dispatchResult.TurnID, action, reason, "continuation") {
		restoreIfUncommitted()
		return false
	}
	return s.retryRunHasExactBinding(ctx, runID)
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
func (s *Service) promptAutomationContinuation(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
	prompt string,
	runID string,
	operation *automation.RetryOperation,
) (automation.RunDispatch, error) {
	var acceptanceErr error
	result, err := s.promptTask(ctx, task.ID, session.ID, prompt, "", false, nil, true, launchOriginAutomatic, promptTaskOptions{
		onAccepted: func(turnID string) {
			if operation == nil {
				return
			}
			acceptedDispatch := automation.RunDispatch{
				TaskID: task.ID, SessionID: session.ID, TurnID: turnID,
			}
			commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), promptFailureCleanupTimeout)
			acceptanceErr = s.commitRetryContinuationOperation(commitCtx, runID, operation, acceptedDispatch)
			if acceptanceErr != nil {
				ambiguousErr := s.markRetryOperationAmbiguous(commitCtx, runID, operation, acceptedDispatch)
				acceptanceErr = errors.Join(
					fmt.Errorf("%w: %v", automation.ErrRetryContinuationCommitAmbiguous, acceptanceErr),
					ambiguousErr,
				)
			}
			cancel()
		},
	})
	if acceptanceErr != nil {
		if err != nil {
			return automation.RunDispatch{}, errors.Join(err, acceptanceErr)
		}
		return automation.RunDispatch{}, acceptanceErr
	}
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

func (s *Service) recordAutomationContinuationMessage(ctx context.Context, taskID, sessionID, turnID, prompt string) {
	if s.messageCreator == nil || prompt == "" || turnID == "" {
		return
	}
	meta := NewUserMessageMeta().WithAutoStart(true)
	if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, turnID, meta.ToMap()); err != nil {
		s.logger.Warn("failed to record automation continuation prompt", zap.String("task_id", taskID), zap.String("turn_id", turnID), zap.Error(err))
	}
}

func (s *Service) retryRunHasExactBinding(ctx context.Context, runID string) bool {
	if runID == "" {
		return true
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return false
	}
	run, err := binding.GetRun(ctx, runID)
	return err == nil && run != nil && run.Status == automation.RunStatusTaskCreated &&
		run.SessionID != "" && run.TurnID != ""
}

func (s *Service) acknowledgeRetryEventAfterBinding(ctx context.Context, evt *automation.AutomationTriggeredEvent) {
	receipt, ok := s.automationService.(automationRetryReceipt)
	if !ok {
		s.logger.Warn("retry event acknowledgement is unavailable",
			zap.String("run_id", evt.RunID))
		return
	}
	if err := receipt.AcknowledgeRetryEvent(ctx, evt.RetryOutboxEventID, evt.RetryOutboxLeaseToken, evt.RunID, evt.SnapshotVersion); err != nil {
		s.logger.Warn("failed to acknowledge automation retry event", zap.Error(err))
	}
}
func (s *Service) autoStartAutomationTask(ctx context.Context, a *automation.Automation, task *models.Task, workflowStepID string) {
	s.autoStartAutomationTaskForRun(ctx, a, task, workflowStepID, "", automation.ThreadActionCreated, "", false, nil)
}

func (s *Service) autoStartAutomationTaskForRun(
	ctx context.Context,
	a *automation.Automation,
	task *models.Task,
	workflowStepID, runID string,
	action automation.ThreadAction,
	reason string,
	preserveTaskOnFailure bool,
	operation *automation.RetryOperation,
) bool {
	var run *automationRunLaunch
	if runID != "" {
		run = &automationRunLaunch{RunID: runID, ThreadAction: action, ThreadReason: reason}
	}
	if s.dispatchAutomationRun(ctx, a.ID, task.ID, "", runID, action, reason, "auto-start", func() (automation.RunDispatch, error) {
		dispatch, err := s.startAutomationTask(ctx, a, task, workflowStepID, run)
		if err != nil {
			return automation.RunDispatch{}, err
		}
		if operation != nil && operation.State == retryOperationCommittedState {
			if err := s.commitRetryContinuationOperation(ctx, runID, operation, dispatch); err != nil {
				return automation.RunDispatch{}, fmt.Errorf("commit exact retry launch identity: %w", err)
			}
		}
		return dispatch, nil
	}, nil, preserveTaskOnFailure) {
		return s.retryRunHasExactBinding(ctx, runID)
	}

	dispatch, err := s.startAutomationTask(ctx, a, task, workflowStepID, run)
	if err != nil {
		s.logger.Error("failed to auto-start automation task",
			zap.String("task_id", task.ID), zap.Error(err))
		if !s.markExactAutomationRunTerminal(ctx, runID, "", "", false, err.Error()) && runID == "" {
			s.markAutomationRunTerminal(ctx, task.ID, false, err.Error())
		}
		return false
	}
	if !s.bindAutomationRun(ctx, runID, dispatch.TaskID, dispatch.SessionID, dispatch.TurnID, action, reason, "auto-start") {
		return false
	}
	s.logger.Info("auto-started automation task",
		zap.String("task_id", task.ID),
		zap.String("automation_id", a.ID))
	return s.retryRunHasExactBinding(ctx, runID)
}

// startAutomationTask starts the task an automation run created. run names
// that run so a start queued by the session ceiling keeps it for the replay.
func (s *Service) startAutomationTask(
	ctx context.Context,
	a *automation.Automation,
	task *models.Task,
	workflowStepID string,
	run *automationRunLaunch,
) (automation.RunDispatch, error) {
	execution, err := s.startTask(
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
		startTaskOptions{AutomationRun: run},
	)
	return automationRunDispatchFor(task.ID, execution, err)
}

// Repository-selector disposition tokens (A5's wire format). A bare token is
// self-explanatory; selectorNoMatch and selectorAmbiguous carry the resolved
// selector value appended as "<token>: <value>" (tokenWithValue).
const (
	repositoryContinuationReused = "repository_continuation_reused"
	repositorySelectorUnresolved = "selector_unresolved"
	repositoryNoneConfigured     = "repository_none_configured"
	repositoryLoadFailed         = "repository_load_failed"
	repositorySelectorAmbiguous  = "selector_ambiguous"
	repositorySelectorNoMatch    = "selector_no_match"
)

// resolveAutomationRepository determines the repositories for an
// automation-triggered task, and the disposition token (if any) to record
// when a declared webhook repository selector produced no binding. For
// github_pr triggers, it always extracts repo info from the trigger data —
// the PR's own repo is the only sensible choice when responding to a PR
// event — and the payload never reaches resolveGitHubPRTriggerRepository for
// any other trigger type: a webhook firing must select only among the
// automation's own already-configured repositories, never an arbitrary
// owner/name resolved from attacker-controlled payload data (the webhook
// route is secured only by a shared secret, not session auth).
func (s *Service) resolveAutomationRepository(
	ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent,
) ([]ReviewTaskRepository, string) {
	if evt.TriggerType == automation.TriggerTypeGitHubPR {
		return s.resolveGitHubPRTriggerRepository(ctx, a.WorkspaceID, evt.TriggerData), ""
	}
	resolved, outcome := s.resolveExplicitRepositories(ctx, configuredAutomationRepositories(a))
	selectorPath, declared := webhookRepositorySelectorPath(a, evt)
	if !declared {
		return resolved, ""
	}
	// A declared selector with an empty path is still a commitment (the
	// WebhookRepositorySelector doc comment): matchRepositoryBySelector's own
	// automation.ResolvePayloadPath("", ...) call already returns ok=false for
	// an empty path, so routing it through the same match path fails closed
	// (selector_unresolved) instead of the "no selector declared" shortcut
	// above, which binds every configured repository.
	return matchRepositoryBySelector(resolved, outcome, selectorPath, evt.TriggerData)
}

// webhookRepositorySelectorPath returns the declared repository.selector_path
// for the exact trigger that fired and whether a selector was declared at
// all (Repository non-nil) — distinct from an empty declared path, which
// must still fail closed rather than being treated as "no selector". Returns
// declared=false when no selector is declared, the trigger config can't be
// read, or the firing trigger type is not webhook (a selector is a
// webhook-only concept).
func webhookRepositorySelectorPath(a *automation.Automation, evt *automation.AutomationTriggeredEvent) (path string, declared bool) {
	if evt.TriggerType != automation.TriggerTypeWebhook {
		return "", false
	}
	for _, t := range a.Triggers {
		if t.ID != evt.TriggerID {
			continue
		}
		var cfg automation.WebhookTriggerConfig
		if err := json.Unmarshal(t.Config, &cfg); err != nil || cfg.Repository == nil {
			return "", false
		}
		return cfg.Repository.SelectorPath, true
	}
	return "", false
}

// matchRepositoryBySelector resolves a declared selector's value against the
// payload and matches it, exactly and case-sensitively, against each already
// -resolved repository's Name. Exactly one match binds; zero or more than one
// binds nothing. Precedence (first-applicable, total): unresolved selector >
// none configured > load failure that leaves nothing certain > ambiguous >
// no match.
func matchRepositoryBySelector(
	resolved []ReviewTaskRepository, outcome repositoryLoadOutcome, selectorPath string, triggerData json.RawMessage,
) ([]ReviewTaskRepository, string) {
	value, ok := automation.ResolvePayloadPath(triggerData, selectorPath)
	if !ok {
		return nil, repositorySelectorUnresolved
	}
	if outcome == outcomeNoneConfigured {
		return nil, repositoryNoneConfigured
	}
	matches := make([]ReviewTaskRepository, 0, 1)
	for _, r := range resolved {
		if r.Name == value {
			matches = append(matches, r)
		}
	}
	if outcome == outcomeStoreUnavailable || (outcome == outcomePartialFailure && len(matches) != 1) {
		return nil, repositoryLoadFailed
	}
	switch len(matches) {
	case 1:
		return matches, ""
	case 0:
		return nil, tokenWithValue(repositorySelectorNoMatch, value)
	default:
		return nil, tokenWithValue(repositorySelectorAmbiguous, value)
	}
}

// tokenWithValue implements A5's wire format for a value-bearing disposition
// token: token, colon, one space, then the trimmed resolved value truncated
// to 200 runes (not bytes, so a truncated multi-byte character can't produce
// invalid UTF-8).
func tokenWithValue(token, value string) string {
	runes := []rune(value)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	return token + ": " + string(runes)
}

// repositoryLoadOutcome reports how resolveExplicitRepositories' load
// attempt went, independent of any selector matching a caller may perform
// against the result.
type repositoryLoadOutcome int

const (
	outcomeOK repositoryLoadOutcome = iota
	outcomeNoneConfigured
	outcomeStoreUnavailable
	outcomePartialFailure
)

// resolveExplicitRepositories loads each configured repository and produces one
// ReviewTaskRepository entry per resolvable pair, in order. Canonical bindings
// use their saved base branch; legacy repository-ID-only rows use the repository
// default. A repository that fails to load is skipped (with a warning) rather
// than aborting the whole firing, so one bad repository does not sink the others.
func (s *Service) resolveExplicitRepositories(
	ctx context.Context, repositories []automation.AutomationRepository,
) ([]ReviewTaskRepository, repositoryLoadOutcome) {
	if len(repositories) == 0 {
		return nil, outcomeNoneConfigured
	}
	store, ok := s.repo.(repoStore)
	if !ok {
		return nil, outcomeStoreUnavailable
	}
	resolved := make([]ReviewTaskRepository, 0, len(repositories))
	anyFailed := false
	for _, configured := range repositories {
		repo, err := store.GetRepository(ctx, configured.RepositoryID)
		if err != nil || repo == nil {
			s.logger.Warn("failed to load explicit automation repository",
				zap.String("repository_id", configured.RepositoryID), zap.Error(err))
			anyFailed = true
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
			Name:           repo.Name,
			BaseBranch:     baseBranch,
			CheckoutBranch: baseBranch,
		})
	}
	if anyFailed {
		return resolved, outcomePartialFailure
	}
	return resolved, outcomeOK
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

//nolint:nestif // Retry failure admission must verify both service and run identity.
func (s *Service) recordFailedRun(ctx context.Context, evt *automation.AutomationTriggeredEvent, errMsg string) {
	if retryService, ok := s.automationService.(automationRetryFailure); ok && evt.RunID != "" {
		if binding, bindOK := s.automationService.(automationRunBinding); bindOK {
			if run, err := binding.GetRun(ctx, evt.RunID); err == nil && run != nil && run.RetryGroupID != "" {
				if _, err := retryService.FinalizeAutomationRetryFailure(ctx, evt.RunID, run.RetryGroupGeneration, errors.New(errMsg), "launch"); err != nil {
					s.logger.Error("failed to finalize automation retry", zap.Error(err))
				}
				return
			}
		}
	}
	if s.markExactAutomationRunTerminal(ctx, evt.RunID, "", "", false, errMsg) {
		return
	}
	run := &automation.AutomationRun{
		AutomationID: evt.AutomationID,
		TriggerID:    evt.TriggerID,
		TriggerType:  evt.TriggerType,
		Status:       automation.RunStatusFailed,
		DedupKey:     automation.PreTaskCreationDedupKey(evt.TriggerType, evt.DedupKey),
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
// A missing lifecycle deleter is a composition error. Skipping deletion is
// safer than falling back to repository deletion, which would strand workspace
// group state and cleanup metadata.
func (s *Service) deleteAbandonedTask(ctx context.Context, automationID, taskID string) {
	s.clearAbandonedContinuation(ctx, automationID, taskID)
	if s.taskLifecycleDeleter == nil {
		s.logger.Error("task lifecycle deleter is not configured for abandoned automation task",
			zap.String("automation_id", automationID),
			zap.String("task_id", taskID))
		return
	}
	if err := s.taskLifecycleDeleter.DeleteTaskWithLifecycle(ctx, taskID); err != nil {
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
	ctx context.Context, evt *automation.AutomationTriggeredEvent, taskID, repositoryReason string,
) error {
	if s.automationService == nil {
		return nil
	}
	if evt.RunID != "" {
		binding, ok := s.automationService.(automationRunBinding)
		if !ok {
			return fmt.Errorf("automation service cannot bind admitted run %s", evt.RunID)
		}
		return binding.BindRunTask(ctx, evt.RunID, taskID, repositoryReason)
	}
	run := &automation.AutomationRun{
		AutomationID:     evt.AutomationID,
		TriggerID:        evt.TriggerID,
		TriggerType:      evt.TriggerType,
		TaskID:           taskID,
		Status:           automation.RunStatusTaskCreated,
		DedupKey:         evt.DedupKey,
		TriggerData:      evt.TriggerData,
		RepositoryReason: repositoryReason,
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
	//nolint:nestif // Exact completion must preserve the run binding fence.
	if run, err := binding.GetRun(ctx, runID); err == nil && run != nil && run.RetryGroupID != "" {
		if (sessionID != "" && sessionID != run.SessionID) || (turnID != "" && turnID != run.TurnID) {
			return false
		}
		if success {
			if terminal, terminalOK := s.automationService.(automationRetrySuccess); terminalOK {
				return terminal.MarkAutomationRetrySucceeded(ctx, runID, run.RetryGroupGeneration) == nil
			}
		} else if failure, failureOK := s.automationService.(automationRetryFailure); failureOK {
			_, finalizeErr := failure.FinalizeAutomationRetryFailure(ctx, runID, run.RetryGroupGeneration, errors.New(errMsg), "completion")
			return finalizeErr == nil
		}
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
		s.pruneAutomationRunWorktrees(ctx, taskID)
		return
	}

	s.markAutomationRunTerminal(ctx, taskID, success, errMsg)
	s.pruneAutomationRunWorktrees(ctx, taskID)
}

func (s *Service) markAutomationRunTerminal(ctx context.Context, taskID string, success bool, errMsg string) {
	if s.automationService == nil {
		return
	}
	if lookup, ok := s.automationService.(automationOpenRunLookup); ok {
		runs, err := lookup.ListOpenRunsByTaskID(ctx, taskID)
		if err != nil || len(runs) != 1 {
			return
		}
		run := runs[0]
		if !s.markExactAutomationRunTerminal(ctx, run.ID, run.SessionID, run.TurnID, success, errMsg) {
			s.logger.Warn("failed to update automation run terminal status",
				zap.String("run_id", run.ID), zap.String("task_id", taskID))
		}
		return
	}
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

//nolint:nestif // Exact session and turn matching must remain in one fence.
func (s *Service) markAutomationRunTerminalForTurn(ctx context.Context, taskID, sessionID, turnID string, success bool, errMsg string) {
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		s.markAutomationRunTerminal(ctx, taskID, success, errMsg)
		return
	}
	if turnID == "" {
		lookup, lookupOK := s.automationService.(automationOpenRunLookup)
		if !lookupOK {
			return
		}
		runs, err := lookup.ListOpenRunsByTaskID(ctx, taskID)
		if err != nil {
			return
		}
		var match *automation.AutomationRun
		for _, run := range runs {
			if run.SessionID == sessionID {
				if match != nil {
					return
				}
				match = run
			}
		}
		if match == nil {
			return
		}
		turnID = match.TurnID
		if !s.markExactAutomationRunTerminal(ctx, match.ID, sessionID, turnID, success, errMsg) {
			s.logger.Warn("failed to update automation run by exact session binding",
				zap.String("run_id", match.ID), zap.String("task_id", taskID))
		}
		return
	}
	lookup, lookupOK := s.automationService.(automationOpenRunLookup)
	if lookupOK {
		runs, lookupErr := lookup.ListOpenRunsByTaskID(ctx, taskID)
		if lookupErr != nil {
			return
		}
		for _, candidate := range runs {
			if candidate.SessionID == sessionID && candidate.TurnID == turnID {
				if !s.markExactAutomationRunTerminal(ctx, candidate.ID, sessionID, turnID, success, errMsg) {
					s.logger.Warn("failed to update automation run by exact turn binding",
						zap.String("run_id", candidate.ID), zap.String("task_id", taskID))
				}
				return
			}
		}
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
