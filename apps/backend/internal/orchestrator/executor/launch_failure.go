package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// RecordEarlyLaunchFailure preserves typed recovery details for a prepared
// session whose launch failed before entering LaunchPreparedSession. The state
// transition is conditional, so an already settled session keeps its history.
func (e *Executor) RecordEarlyLaunchFailure(ctx context.Context, taskID, sessionID string, launchErr error) error {
	return e.handleEarlyLaunchFailure(ctx, taskID, sessionID, "", launchErr)
}

type quickChatStartFailureCommitter interface {
	CommitQuickChatStartFailureIfCurrentAttempt(
		context.Context,
		string,
		string,
		string,
		string,
		models.TaskSessionState,
		models.LastAgentError,
	) (bool, time.Time, error)
}

func (e *Executor) RecordEarlyQuickChatStartFailure(
	ctx context.Context,
	taskID, sessionID, quickChatAttemptID, agentAttemptID string,
	launchErr error,
) (bool, models.TaskSessionState, time.Time, error) {
	safeErr := routingerr.SanitizeError(launchErr)
	if safeErr == nil || taskID == "" || sessionID == "" || quickChatAttemptID == "" ||
		errors.Is(launchErr, ErrExecutionAlreadyRunning) {
		return false, "", time.Time{}, safeErr
	}
	committer, ok := e.repo.(quickChatStartFailureCommitter)
	if !ok {
		return false, "", time.Time{}, safeErr
	}
	failCtx := context.WithoutCancel(ctx)
	session, owned := e.loadQuickChatFailureOwner(
		failCtx, taskID, sessionID, quickChatAttemptID, agentAttemptID,
	)
	if !owned {
		if session == nil {
			return false, "", time.Time{}, safeErr
		}
		return false, session.State, time.Time{}, safeErr
	}
	errorValue := e.buildLastAgentError(failCtx, taskID, "", launchErr)
	changed, updatedAt, err := committer.CommitQuickChatStartFailureIfCurrentAttempt(
		failCtx,
		taskID,
		sessionID,
		quickChatAttemptID,
		agentAttemptID,
		session.State,
		errorValue,
	)
	if err != nil {
		e.logger.Warn("failed to commit Quick Chat launch failure for its owning attempt",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return false, session.State, time.Time{}, safeErr
	}
	if !changed {
		return false, session.State, time.Time{}, safeErr
	}
	e.notifyQuickChatStartFailure(failCtx, taskID, sessionID, safeErr)
	return true, session.State, updatedAt, safeErr
}

func (e *Executor) loadQuickChatFailureOwner(
	ctx context.Context,
	taskID, sessionID, quickChatAttemptID, agentAttemptID string,
) (*models.TaskSession, bool) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		if err != nil {
			e.logger.Warn("failed to load session before recording Quick Chat launch failure",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		}
		return nil, false
	}
	if models.StringFromAny(session.Metadata[models.SessionMetaKeyQuickChatStartAttemptID]) != quickChatAttemptID ||
		models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != agentAttemptID {
		return session, false
	}
	return session, isQuickChatStartFailureState(session.State)
}

func isQuickChatStartFailureState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

func (e *Executor) notifyQuickChatStartFailure(ctx context.Context, taskID, sessionID string, safeErr error) {
	if e.onEarlyLaunchTaskStateReconcile != nil {
		if err := e.onEarlyLaunchTaskStateReconcile(ctx, taskID, sessionID, v1.TaskStateFailed); err != nil {
			e.logger.Warn("failed to reconcile task after Quick Chat launch failure",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		}
	}
	if e.onLaunchFailed != nil {
		e.onLaunchFailed(ctx, taskID, sessionID, "", safeErr)
	}
}

type launchFailureClassification struct {
	code    string
	message string
	noRetry bool
}

func classifyLaunchFailure(err error) launchFailureClassification {
	var relocationErr *worktree.ManagedCloneRelocationRequiredError
	if errors.As(err, &relocationErr) {
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryManagedCloneRelocationRequired,
			message: "The task workspace contains local changes and needs explicit relocation.",
		}
	}
	var recoveryErr *worktree.WorktreeRecoveryError
	if errors.As(err, &recoveryErr) {
		return launchFailureClassification{
			code: models.LaunchErrorCategoryGenericLaunchFailure,
			message: fmt.Sprintf("Worktree recovery is required for task %s at %s: %s",
				recoveryErr.TaskID, recoveryErr.Checkout, recoveryErr.Reason),
			noRetry: true,
		}
	}
	switch {
	case errors.Is(err, worktree.ErrWorkspaceCheckoutFailed):
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryWorkspaceCheckoutFailed,
			message: "The workspace could not be prepared for this launch.",
		}
	case errors.Is(err, worktree.ErrInvalidBaseBranch):
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryBaseBranchMissing,
			message: "The selected base branch is not available.",
		}
	case errors.Is(err, worktree.ErrRemoteDefaultUnresolved):
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryDefaultBranchUnresolved,
			message: "The repository default branch could not be resolved.",
		}
	default:
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryGenericLaunchFailure,
			message: "The agent could not start.",
		}
	}
}

func launchFailureRecoveryActions(category, taskRepositoryID string, markReviewDone bool) []string {
	actions := make([]string, 0, 3)
	if strings.TrimSpace(taskRepositoryID) != "" {
		switch category {
		case models.LaunchErrorCategoryBaseBranchMissing:
			actions = append(actions,
				models.RecoveryActionRetryDefault,
				models.RecoveryActionPickBaseBranch,
			)
		case models.LaunchErrorCategoryDefaultBranchUnresolved:
			actions = append(actions, models.RecoveryActionPickBaseBranch)
		}
	}
	if category == models.LaunchErrorCategoryWorkspaceCheckoutFailed || category == models.LaunchErrorCategoryGenericLaunchFailure {
		actions = append(actions, models.RecoveryActionRetryLaunch)
	}
	if category == models.LaunchErrorCategoryManagedCloneRelocationRequired {
		actions = append(actions, models.RecoveryActionRelocateAndResume)
	}
	if category == models.LaunchErrorCategoryPRAlreadyClosed && markReviewDone {
		actions = append(actions, models.RecoveryActionMarkReviewDone)
	}
	return models.NormalizeRecoveryActionsForCategory(category, actions)
}
func (e *Executor) buildLastAgentError(
	ctx context.Context,
	taskID, taskRepositoryID string,
	launchErr error,
) models.LastAgentError {
	classification := classifyLaunchFailure(launchErr)
	markReviewDone := false
	if e.launchFailureReviewEligibility != nil {
		eligible, err := e.launchFailureReviewEligibility(ctx, taskID)
		if err != nil {
			if e.logger != nil {
				e.logger.Debug("launch failure review eligibility lookup failed",
					zap.String("task_id", taskID), zap.Error(err))
			}
		} else {
			markReviewDone = eligible
		}
	}
	occurredAt := time.Now().UTC()
	details := ""
	if launchErr != nil {
		details = routingerr.Sanitize(launchErr.Error())
	}
	return models.LastAgentError{
		Message:    classification.message,
		OccurredAt: occurredAt,
		Scope:      models.ErrorScopeSession,
		Phase:      models.LaunchErrorPhaseBootstrap,
		Code:       classification.code,
		Details:    details,
		RecoveryActions: func() []string {
			if classification.noRetry {
				return nil
			}
			return launchFailureRecoveryActions(classification.code, taskRepositoryID, markReviewDone)
		}(),
		TaskRepositoryID: taskRepositoryID,
		StampValue: models.StableLaunchErrorStamp(
			taskID, classification.code, taskRepositoryID, occurredAt.Format(time.RFC3339Nano),
		),
	}
}

// buildBootstrapLastAgentError creates the durable projection for an
// asynchronous process-start failure. The wrapped error is useful for logs and
// errors.Is callers, but it is deliberately absent from every persisted field.
func (e *Executor) buildBootstrapLastAgentError(
	ctx context.Context,
	taskID, sessionID, agentExecutionID string,
	launchErr error,
	fromResume bool,
) models.LastAgentError {
	const operation = "agent_bootstrap"
	classification := launchFailureClassification{
		code:    models.LaunchErrorCategoryGenericLaunchFailure,
		message: "The agent could not start.",
	}
	markReviewDone := false
	if e.launchFailureReviewEligibility != nil {
		eligible, err := e.launchFailureReviewEligibility(ctx, taskID)
		if err != nil {
			if e.logger != nil {
				e.logger.Debug("launch failure review eligibility lookup failed",
					zap.String("task_id", taskID), zap.Error(err))
			}
		} else {
			markReviewDone = eligible
		}
	}

	var preparationErr *agentruntime.RepositoryPreparationError
	taskRepositoryID := ""
	if errors.As(launchErr, &preparationErr) && preparationErr != nil {
		taskRepositoryID = preparationErr.TaskRepositoryID
	}

	causes, safeReason := bootstrapFailureCause(launchErr, fromResume)
	details := operation
	if safeReason != "" {
		details += "; cause=" + safeReason
	}
	if len(causes) > 0 && causes[0].Detail != "" {
		details += "; " + causes[0].Detail
	}
	details = models.NormalizeAgentErrorDetails(details, causes)
	occurredAt := time.Now().UTC()
	attemptID := agentExecutionID
	if contextAttemptID := ResumeAttemptIDFromContext(ctx); contextAttemptID != "" {
		attemptID = contextAttemptID
	}
	return models.LastAgentError{
		Message:          classification.message,
		OccurredAt:       occurredAt,
		Scope:            models.ErrorScopeSession,
		AgentExecutionID: agentExecutionID,
		ExecutionID:      agentExecutionID,
		Phase:            models.LaunchErrorPhaseBootstrap,
		AttemptID:        attemptID,
		Causes:           causes,
		Code:             classification.code,
		Details:          details,
		RecoveryActions:  launchFailureRecoveryActions(classification.code, taskRepositoryID, markReviewDone),
		TaskRepositoryID: taskRepositoryID,
		StampValue: models.StableLaunchErrorStamp(
			taskID, sessionID, agentExecutionID, attemptID,
			classification.code, occurredAt.Format(time.RFC3339Nano),
		),
	}
}

func bootstrapFailureCause(launchErr error, fromResume bool) ([]models.AgentErrorCause, string) {
	var failure *agentruntime.BootstrapFailure
	if errors.As(launchErr, &failure) && failure != nil {
		operation := failure.Operation
		if operation == "" {
			operation = models.AgentErrorCauseOperationStart
			if fromResume {
				operation = models.AgentErrorCauseOperationResume
			}
		}
		if operation != models.AgentErrorCauseOperationStart &&
			operation != models.AgentErrorCauseOperationResume &&
			operation != models.AgentErrorCauseOperationRestoreWorkspace {
			return nil, ""
		}
		cause, ok := failure.SafeAgentErrorCause(operation)
		if !ok {
			return nil, ""
		}
		return models.NormalizeAgentErrorCauses([]models.AgentErrorCause{cause}), cause.Code
	}
	if !fromResume {
		return nil, ""
	}
	code := models.AgentErrorCauseCodeUnknown
	return []models.AgentErrorCause{{
		Operation: models.AgentErrorCauseOperationResume,
		Code:      code,
	}}, code
}

func (e *Executor) persistLastAgentError(
	ctx context.Context,
	sessionID string,
	errorValue models.LastAgentError,
) {
	if err := e.repo.SetSessionMetadataKey(
		ctx, sessionID, models.SessionMetaKeyLastAgentError, errorValue,
	); err != nil {
		e.logger.Warn("failed to persist typed launch error",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

// bootstrapFailureOwnsSession verifies that the failed asynchronous start is
// still the session's active execution. The durable session identity is the
// primary fence; the runtime lookup catches the smaller window in which a
// successor has been registered but its session row has not been refreshed in
// this process yet.
func (e *Executor) loadBootstrapFailureSession(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	for attempt := 0; attempt < 3; attempt++ {
		session, err := e.repo.GetTaskSession(ctx, sessionID)
		if err == nil {
			return session, nil
		}
		if attempt == 2 {
			return nil, fmt.Errorf("load session before bootstrap failure projection: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil, ctx.Err()
}

func (e *Executor) bootstrapFailureOwnsSession(
	ctx context.Context,
	sessionID, agentExecutionID string,
	expectedStartAttemptID string,
) (bool, bool, error) {
	if sessionID == "" || agentExecutionID == "" {
		return true, false, nil
	}
	session, err := e.loadBootstrapFailureSession(ctx, sessionID)
	if err != nil {
		return false, false, err
	}
	if session == nil {
		return false, false, nil
	}
	if session.AgentExecutionID != "" && session.AgentExecutionID != agentExecutionID {
		return false, true, nil
	}
	if expectedStartAttemptID != "" &&
		models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != expectedStartAttemptID {
		return false, false, nil
	}
	if e.agentManager == nil {
		return true, false, nil
	}
	liveExecutionID, lookupErr := e.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if lookupErr == nil && liveExecutionID != "" && liveExecutionID != agentExecutionID {
		return false, true, nil
	}
	return true, false, nil
}

// bootstrapFailureExpectation captures the final session observation used by
// the atomic bootstrap-failure commit. The repository repeats every guard in
// the same write that stores the error and FAILED state.
func (e *Executor) bootstrapFailureExpectation(
	ctx context.Context,
	sessionID, agentExecutionID string,
	expectedStartAttemptID string,
) (*models.TaskSession, string, bool, error) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, "", false, fmt.Errorf("reload session before bootstrap failure commit: %w", err)
	}
	if session == nil {
		return nil, "", false, fmt.Errorf("reload session before bootstrap failure commit: session %q is nil", sessionID)
	}
	if session.AgentExecutionID != "" && session.AgentExecutionID != agentExecutionID {
		return session, "", false, nil
	}
	if expectedStartAttemptID != "" &&
		models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != expectedStartAttemptID {
		return session, "", false, nil
	}

	current, hasCurrent := models.LoadLastAgentError(session.Metadata)
	stamp := ""
	if hasCurrent {
		stamp = current.Stamp()
	}
	return session, stamp, true, nil
}

// commitBootstrapFailure commits the correlated failure projection and the
// FAILED transition through the atomic orchestrator callback or repository
// capability. Stores without an execution-fenced commit fail closed so a
// bootstrap failure cannot overwrite a successor execution.
func (e *Executor) commitBootstrapFailure(
	ctx context.Context,
	taskID, sessionID, agentExecutionID string,
	expectedStartAttemptID string,
	errorValue models.LastAgentError,
) (bool, models.TaskSessionState, error) {
	session, expectedStamp, owned, err := e.bootstrapFailureExpectation(ctx, sessionID, agentExecutionID, expectedStartAttemptID)
	if err != nil {
		return false, "", err
	}
	if !owned {
		return false, session.State, nil
	}

	if e.onBootstrapFailureTransition != nil {
		return e.onBootstrapFailureTransition(
			ctx,
			taskID,
			sessionID,
			agentExecutionID,
			session.State,
			expectedStamp,
			expectedStartAttemptID,
			errorValue,
		)
	}
	if expectedStartAttemptID != "" {
		return e.commitBootstrapFailureIfCurrentAttempt(
			ctx,
			taskID,
			sessionID,
			agentExecutionID,
			session.State,
			expectedStamp,
			expectedStartAttemptID,
			errorValue,
		)
	}

	if committer, ok := e.repo.(bootstrapFailureCommitter); ok {
		changed, _, err := committer.CommitBootstrapFailureIfCurrentExecution(
			ctx,
			taskID,
			sessionID,
			agentExecutionID,
			session.State,
			expectedStamp,
			errorValue,
		)
		if err != nil {
			return false, session.State, err
		}
		if !changed {
			return false, session.State, nil
		}
		return true, models.TaskSessionStateFailed, nil
	}

	return false, session.State, fmt.Errorf(
		"bootstrap failure requires an execution-fenced repository commit",
	)
}

func (e *Executor) commitBootstrapFailureIfCurrentAttempt(
	ctx context.Context,
	taskID, sessionID, agentExecutionID string,
	expectedState models.TaskSessionState,
	expectedStamp, expectedStartAttemptID string,
	errorValue models.LastAgentError,
) (bool, models.TaskSessionState, error) {
	committer, ok := e.repo.(bootstrapFailureAttemptCommitter)
	if !ok {
		return false, expectedState, fmt.Errorf("bootstrap failure requires a startup-attempt-fenced repository commit")
	}
	changed, _, err := committer.CommitBootstrapFailureIfCurrentAttempt(
		ctx, taskID, sessionID, agentExecutionID, expectedState, expectedStamp, expectedStartAttemptID, errorValue,
	)
	if err != nil {
		return false, expectedState, err
	}
	if !changed {
		return false, expectedState, nil
	}
	return true, models.TaskSessionStateFailed, nil
}
