package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

type automationRetryLaunchOwner interface {
	GetRetryLaunch(context.Context, string) (*automation.RetryLaunch, error)
	ReserveRetryLaunch(context.Context, string, string, string, automation.ThreadAction, string) (*automation.RetryLaunch, error)
	AdmitRetryLaunch(context.Context, *automation.RetryLaunch, string) error
	VerifyRetryLaunch(context.Context, *automation.RetryLaunch, string) error
	AcceptRetryLaunch(context.Context, *automation.RetryLaunch, string) error
}

type automationRetryLaunchContextKey struct{}

func (s *Service) reserveAutomationRetryLaunch(ctx context.Context, runID, taskID, sessionID string, action automation.ThreadAction, reason string) error {
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return nil
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return automation.ErrRetryLaunchUncertain
	}
	run, err := binding.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run == nil {
		return automation.ErrRetryGenerationMismatch
	}
	if run.RetryGroupID == "" {
		return nil
	}
	_, err = owner.ReserveRetryLaunch(ctx, runID, taskID, sessionID, action, reason)
	return err
}

func (s *Service) retryLaunchContext(ctx context.Context, run *automationRunLaunch, taskID string) (context.Context, error) {
	if run == nil {
		return ctx, nil
	}
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return ctx, nil
	}
	launch, err := owner.GetRetryLaunch(ctx, run.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		ordinary, readErr := s.retryLaunchIsOrdinaryRun(ctx, run.RunID)
		if readErr != nil {
			return ctx, readErr
		}
		if ordinary {
			return ctx, nil
		}
	}
	if err != nil {
		return ctx, errors.Join(automation.ErrRetryLaunchUncertain, err)
	}
	if !retryLaunchClaimMatches(launch, taskID) {
		return ctx, automation.ErrRetryLaunchUncertain
	}
	if run.Launch != nil && !sameRetryLaunchIdentity(run.Launch, launch) {
		return ctx, automation.ErrRetryGenerationMismatch
	}
	run.Launch = launch
	ctx = models.WithReservedLaunchIdentity(ctx, models.ReservedLaunchIdentity{TaskID: launch.TaskID, SessionID: launch.SessionID, TurnID: launch.TurnID, SubmissionID: launch.SubmissionID})
	return context.WithValue(ctx, automationRetryLaunchContextKey{}, launch), nil
}

func sameRetryLaunchIdentity(left, right *automation.RetryLaunch) bool {
	return left.RunID == right.RunID && left.Generation == right.Generation && left.TaskID == right.TaskID && left.SessionID == right.SessionID && left.TurnID == right.TurnID && left.SubmissionID == right.SubmissionID
}

func retryLaunchInContext(ctx context.Context) *automation.RetryLaunch {
	launch, _ := ctx.Value(automationRetryLaunchContextKey{}).(*automation.RetryLaunch)
	return launch
}

func (s *Service) configureRetryInitialLaunch(ctx context.Context, sessionID, turnID string, opts *executor.LaunchOptions) error {
	launch := retryLaunchInContext(ctx)
	if launch == nil {
		return nil
	}
	if launch.SessionID != sessionID || launch.TurnID != turnID {
		return automation.ErrRetryGenerationMismatch
	}
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return automation.ErrRetryLaunchUncertain
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return automation.ErrRetryLaunchUncertain
	}
	opts.BeforeInitialPromptDispatch = func(admissionCtx context.Context, executionID, _ string, _ []v1.MessageAttachment) error {
		return owner.VerifyRetryLaunch(admissionCtx, launch, executionID)
	}
	s.configureInitialDeliveryAdmission(session, startCreatedSessionOptions{deliverySubmissionID: launch.SubmissionID}, opts)
	bindIdentity := opts.BeforeAgentStart
	admitPrepared := opts.BeforeInitialPromptDispatch
	opts.BeforeAgentStart = func(admissionCtx context.Context, executionID, dispatchPrompt string) error {
		if err := owner.AdmitRetryLaunch(admissionCtx, launch, executionID); err != nil {
			return err
		}
		return bindIdentity(admissionCtx, executionID, dispatchPrompt)
	}
	opts.BeforeInitialPromptDispatch = func(admissionCtx context.Context, executionID, finalPrompt string, finalAttachments []v1.MessageAttachment) error {
		receiptCtx, cancel := context.WithTimeout(admissionCtx, 5*time.Second)
		defer cancel()
		if err := admitPrepared(receiptCtx, executionID, finalPrompt, finalAttachments); err != nil {
			return err
		}
		return s.transitionInitialSubmissionToDispatching(receiptCtx, sessionID, executionID, launch.SubmissionID)
	}
	opts.OnInitialPromptAccepted = func(executionID string) {
		receiptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		// Retain launch acceptance even if the session projection cannot be updated.
		launchErr := owner.AcceptRetryLaunch(receiptCtx, launch, executionID)
		projectionErr := s.transitionInitialSubmissionToAccepted(receiptCtx, sessionID, executionID, launch.SubmissionID)
		if err := errors.Join(launchErr, projectionErr); err != nil {
			s.logger.Error("retry launch acceptance projection remains pending", zap.String("run_id", launch.RunID), zap.Error(err))
		}
	}
	return nil
}

func (s *Service) recoverReservedRetryLaunch(ctx context.Context, evt *automation.AutomationTriggeredEvent, run *automation.AutomationRun) bool {
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return false
	}
	launch, err := owner.GetRetryLaunch(ctx, run.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		s.logger.Warn("retry launch ownership read failed", zap.Error(err))
		return true
	}
	if launch.Generation != run.RetryGroupGeneration || launch.TaskID != run.TaskID || launch.SessionID != run.SessionID || launch.TurnID != run.TurnID {
		s.logger.Error("retry launch ownership conflicts with its run", zap.String("run_id", run.ID))
		return true
	}
	if launch.State == automation.RetryLaunchAccepted || launch.State == automation.RetryLaunchStopped {
		s.acknowledgeRetryEventAfterBinding(ctx, evt)
		return true
	}
	if launch.State != automation.RetryLaunchPrepared {
		return true
	}
	return s.dispatchReservedRetryLaunch(ctx, run, launch)
}

func (s *Service) retryContinuationAdmission(ctx context.Context, sessionID string) error {
	launch := retryLaunchInContext(ctx)
	if launch == nil {
		return nil
	}
	if launch.SessionID != sessionID {
		return automation.ErrRetryGenerationMismatch
	}
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return automation.ErrRetryLaunchUncertain
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load retry continuation execution: %w", err)
	}
	if session == nil {
		return automation.ErrRetryLaunchUncertain
	}
	if session.AgentExecutionID == "" {
		return automation.ErrRetryLaunchUncertain
	}
	if s.turnService == nil {
		return automation.ErrRetryLaunchUncertain
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		return err
	}
	if turn == nil || turn.ID != launch.TurnID {
		return automation.ErrRetryGenerationMismatch
	}
	return owner.AdmitRetryLaunch(ctx, launch, session.AgentExecutionID)
}

// Durable automation attempts own recovery; generic provider retries cannot resend them.
func (s *Service) automationOwnsRetrySession(ctx context.Context, taskID, sessionID string) bool {
	lookup, ok := s.automationService.(automationOpenRunLookup)
	if !ok {
		return false
	}
	runs, err := lookup.ListOpenRunsByTaskID(ctx, taskID)
	if err != nil {
		return true
	}
	for _, run := range runs {
		if run.RetryGroupID != "" && (sessionID == "" || run.SessionID == sessionID || run.SessionID == "") {
			return true
		}
	}
	return s.automationOwnsUnboundRetryTask(ctx, taskID)
}

// Task creation persists the run owner before the run can acquire its task binding.
func (s *Service) automationOwnsUnboundRetryTask(ctx context.Context, taskID string) bool {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return true
	}
	runID, _ := task.Metadata[retryAutomationRunIDMetadataKey].(string)
	if runID == "" {
		return false
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return true
	}
	run, err := binding.GetRun(ctx, runID)
	if err != nil {
		return true
	}
	if run == nil {
		owner, ok := s.automationService.(interface {
			RetryAdmissionUnresolved(context.Context, string) (bool, error)
		})
		if !ok {
			return true
		}
		pending, err := owner.RetryAdmissionUnresolved(ctx, runID)
		return err != nil || pending
	}
	return run.RetryGroupID != "" && (run.Status == automation.RunStatusTriggered || run.Status == automation.RunStatusTaskCreated)
}

func (s *Service) observeAutomationRetryTurnTerminal(ctx context.Context, taskID, sessionID, turnID, executionID string) bool {
	if turnID == "" {
		return true
	}
	owner, ok := s.automationService.(interface {
		ObserveRetryLaunchTerminal(context.Context, string, string, string, string) error
	})
	if !ok {
		return true
	}
	if err := owner.ObserveRetryLaunchTerminal(ctx, taskID, sessionID, turnID, executionID); err != nil {
		s.logger.Error("retry terminal evidence persistence failed", zap.Error(err))
		return false
	}
	return true
}

func (s *Service) releaseRetryTurnFailureOwnership(ctx context.Context, data watcher.AgentEventData) bool {
	lookup, ok := s.automationService.(automationOpenRunLookup)
	if !ok || data.TurnID == "" {
		return false
	}
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return false
	}
	runs, err := lookup.ListOpenRunsByTaskID(ctx, data.TaskID)
	if err != nil {
		return false
	}
	for _, run := range runs {
		if run.RetryGroupID == "" || run.SessionID != data.SessionID || run.TurnID != data.TurnID {
			continue
		}
		launch, err := owner.GetRetryLaunch(ctx, run.ID)
		if err != nil || launch.TaskID != data.TaskID || launch.SessionID != data.SessionID ||
			launch.TurnID != data.TurnID || launch.ExecutionID != data.AgentExecutionID {
			return false
		}
		return s.observeAutomationRetryTurnTerminal(ctx, data.TaskID, data.SessionID, data.TurnID, data.AgentExecutionID)
	}
	return false
}

// StopAutomationRunDefinitively waits for teardown of the observed execution.
// Logical cancellation alone cannot release durable launch ownership.

func (s *Service) StopAutomationRunDefinitively(ctx context.Context, taskID, sessionID, turnID string) (bool, error) {
	if taskID == "" || sessionID == "" || turnID == "" || s.turnService == nil || s.executor == nil {
		return false, nil
	}
	endCancel := s.beginCancelInFlight(sessionID)
	defer endCancel()
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	lock.Lock()
	executionID, err := s.retryStopExecutionIdentity(ctx, taskID, sessionID, turnID)
	if err != nil || executionID == "" {
		lock.Unlock()
		release()
		return false, err
	}
	result, _, err := s.stopTaskSessionForCoordinatorLocked(ctx, taskID, sessionID)
	claim, claimed := s.executionTeardownClaimFor(sessionID, executionID)
	lock.Unlock()
	release()
	endCancel()
	if err != nil {
		return false, err
	}
	if result.ExecutionID != "" && result.ExecutionID != executionID {
		return false, automation.ErrRetryGenerationMismatch
	}
	if err := s.executor.StopExecution(ctx, executionID, coordinatorMCPStopReason, false); err != nil {
		return false, err
	}
	if s.lspLeases != nil {
		s.lspLeases.StopLSPLeasesForSession(sessionID)
	}
	s.clearDynamicUnclassifiedStreakForStop(ctx, sessionID)
	if claimed {
		s.completeExecutionTeardownClaim(sessionID, executionID, claim)
	}
	return true, nil
}

func (s *Service) retryStopExecutionIdentity(ctx context.Context, taskID, sessionID, turnID string) (string, error) {
	session, err := s.retryStopSession(ctx, taskID, sessionID, turnID)
	if err != nil || session == nil {
		return "", err
	}
	lookup, ok := s.automationService.(automationOpenRunLookup)
	if !ok {
		return "", automation.ErrRetryLaunchUncertain
	}
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return "", automation.ErrRetryLaunchUncertain
	}
	runs, err := lookup.ListOpenRunsByTaskID(ctx, taskID)
	if err != nil {
		return "", err
	}
	for _, run := range runs {
		if run.SessionID != sessionID || run.TurnID != turnID {
			continue
		}
		launch, err := owner.GetRetryLaunch(ctx, run.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return session.AgentExecutionID, nil
		}
		if err != nil {
			return "", err
		}
		return retryStopOwnedExecution(session, launch, taskID, sessionID, turnID)
	}
	return "", automation.ErrRetryLaunchUncertain
}

func (s *Service) retryLaunchIsOrdinaryRun(ctx context.Context, runID string) (bool, error) {
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return false, automation.ErrRetryLaunchUncertain
	}
	firing, err := binding.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	return firing != nil && firing.RetryGroupID == "", nil
}

func retryLaunchClaimMatches(launch *automation.RetryLaunch, taskID string) bool {
	return launch.RunID != "" && launch.Generation > 0 && launch.SessionID != "" && launch.TurnID != "" &&
		launch.SubmissionID != "" && launch.Token != "" && launch.TaskID == taskID &&
		launch.State == automation.RetryLaunchPreparing && !launch.CancelRequested
}

func (s *Service) dispatchReservedRetryLaunch(ctx context.Context, run *automation.AutomationRun, launch *automation.RetryLaunch) bool {
	snapshot, err := automation.DecodeRetryLaunchConfigSnapshot(run.RetryLaunchConfigSnapshot, run.RetryLaunchConfigVersion)
	if err != nil {
		return true
	}
	task, err := s.repo.GetTask(ctx, launch.TaskID)
	if err != nil || task == nil {
		return true
	}
	a := automationFromRetrySnapshot(snapshot)
	if run.ThreadAction == automation.ThreadActionResumed {
		session, err := s.repo.GetTaskSession(ctx, launch.SessionID)
		if err != nil || session == nil {
			return true
		}
		prompt := snapshot.ResolvedPrompt
		s.dispatchAutomationRun(ctx, a.ID, task.ID, session.ID, run.ID, run.ThreadAction, run.ThreadReason, "recovered continuation", func() (automation.RunDispatch, error) {
			return s.promptAutomationContinuation(ctx, task, session, prompt, run.ID, nil)
		}, nil, true)
		return true
	}
	s.autoStartAutomationTaskForRun(ctx, a, task, task.WorkflowStepID, run.ID, run.ThreadAction, run.ThreadReason, true, nil)
	return true
}

func (s *Service) retryStopSession(ctx context.Context, taskID, sessionID, turnID string) (*models.TaskSession, error) {
	session, valid, err := s.loadAutomationCoordinatorSession(ctx, taskID, sessionID, false)
	if err != nil || !valid {
		return nil, err
	}
	active, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if active != nil && active.ID != turnID {
		return nil, automation.ErrRetryGenerationMismatch
	}
	turn, err := s.turnService.GetTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}
	if turn == nil || turn.TaskID != taskID || turn.TaskSessionID != sessionID {
		return nil, automation.ErrRetryGenerationMismatch
	}
	return session, nil
}

func retryStopOwnedExecution(session *models.TaskSession, launch *automation.RetryLaunch, taskID, sessionID, turnID string) (string, error) {
	if !launch.CancelRequested || launch.TaskID != taskID || launch.SessionID != sessionID || launch.TurnID != turnID {
		return "", automation.ErrRetryGenerationMismatch
	}
	if session.AgentExecutionID != "" && session.AgentExecutionID != launch.ExecutionID {
		return "", automation.ErrRetryGenerationMismatch
	}
	return launch.ExecutionID, nil
}
