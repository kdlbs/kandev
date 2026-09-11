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
)

type launchFailureClassification struct {
	code    string
	message string
	noRetry bool
}

func classifyLaunchFailure(err error) launchFailureClassification {
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
	return models.LastAgentError{
		Message:          classification.message,
		OccurredAt:       occurredAt,
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
		if operation == "" && fromResume {
			operation = models.AgentErrorCauseOperationResume
		}
		if operation != models.AgentErrorCauseOperationResume &&
			operation != models.AgentErrorCauseOperationRestoreWorkspace {
			return nil, ""
		}
		code := failure.SafeCode()
		detail := failure.SafeDetail()
		return models.NormalizeAgentErrorCauses([]models.AgentErrorCause{{
			Operation: operation,
			Code:      code,
			Detail:    detail,
		}}), code
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
func (e *Executor) bootstrapFailureOwnsSession(
	ctx context.Context,
	sessionID, agentExecutionID string,
) (bool, error) {
	if sessionID == "" || agentExecutionID == "" {
		return true, nil
	}
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("load session before bootstrap failure projection: %w", err)
	}
	if session == nil {
		return false, nil
	}
	if session.AgentExecutionID != "" && session.AgentExecutionID != agentExecutionID {
		return false, nil
	}
	if e.agentManager == nil {
		return true, nil
	}
	liveExecutionID, lookupErr := e.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if lookupErr == nil && liveExecutionID != "" && liveExecutionID != agentExecutionID {
		return false, nil
	}
	return true, nil
}

// bootstrapFailureExpectation captures the final session observation used by
// the atomic bootstrap-failure commit. The repository repeats every guard in
// the same write that stores the error and FAILED state.
func (e *Executor) bootstrapFailureExpectation(
	ctx context.Context,
	sessionID, agentExecutionID string,
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

	current, hasCurrent := models.LoadLastAgentError(session.Metadata)
	stamp := ""
	if hasCurrent {
		stamp = current.Stamp()
	}
	return session, stamp, true, nil
}

// commitBootstrapFailure commits the correlated failure projection and the
// FAILED transition through the atomic orchestrator callback or repository
// capability. Legacy test stores retain the older CAS plus state-transition
// fallback, but the production SQL repository uses the guarded commit.
func (e *Executor) commitBootstrapFailure(
	ctx context.Context,
	taskID, sessionID, agentExecutionID string,
	errorValue models.LastAgentError,
) (bool, models.TaskSessionState, error) {
	session, expectedStamp, owned, err := e.bootstrapFailureExpectation(ctx, sessionID, agentExecutionID)
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

	// Legacy stores cannot atomically fence the executor row and session write.
	// Keep their existing behavior for focused test doubles and older adapters;
	// the production repository always takes the guarded path above.
	writer, supportsCAS := e.repo.(sessionMetadataErrorCASWriter)
	var persisted bool
	if supportsCAS {
		if expectedStamp != "" {
			persisted, err = writer.SetSessionMetadataKeyIfStamp(
				ctx,
				sessionID,
				models.SessionMetaKeyLastAgentError,
				expectedStamp,
				errorValue,
			)
		} else {
			persisted, err = writer.SetSessionMetadataKeyIfAbsent(
				ctx,
				sessionID,
				models.SessionMetaKeyLastAgentError,
				errorValue,
			)
		}
	} else {
		err = e.repo.SetSessionMetadataKey(
			ctx, sessionID, models.SessionMetaKeyLastAgentError, errorValue,
		)
		persisted = err == nil
	}
	if err != nil {
		return false, session.State, err
	}
	if !persisted {
		return false, session.State, nil
	}

	changed, finalState, err := e.transitionSessionState(
		ctx, taskID, sessionID, models.TaskSessionStateFailed, errorValue.Message,
	)
	return changed, finalState, err
}
