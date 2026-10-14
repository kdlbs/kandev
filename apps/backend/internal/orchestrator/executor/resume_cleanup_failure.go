package executor

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// ResumeCleanupFailure binds safe failure evidence to the observed session
// revision. A failed predecessor stop never claims a replacement execution.
type ResumeCleanupFailure struct {
	*agentruntime.BootstrapFailure
	ObservedSession *models.TaskSession
}

func (e *ResumeCleanupFailure) Error() string { return e.SafeDetail() }
func (e *ResumeCleanupFailure) Unwrap() error { return e.BootstrapFailure }

func newResumeCleanupFailure(cause error, observed *models.TaskSession) error {
	if errors.Is(cause, context.Canceled) {
		return cause
	}
	promptNotSent := true
	failure := &ResumeCleanupFailure{BootstrapFailure: &agentruntime.BootstrapFailure{
		Operation:     models.AgentErrorCauseOperationResume,
		Code:          models.AgentErrorCauseCodeRuntimeCleanupFailed,
		PromptNotSent: &promptNotSent,
		Cause:         cause,
	}}
	if observed != nil {
		copy := *observed
		copy.Metadata = cloneMetadata(observed.Metadata)
		failure.ObservedSession = &copy
	}
	return failure
}

func (e *Executor) captureResumeCleanupFailureSnapshot(ctx context.Context, err error, sessionID, attemptID string, state models.TaskSessionState) {
	var failure *ResumeCleanupFailure
	if !errors.As(err, &failure) || attemptID == "" {
		return
	}
	session, loadErr := e.repo.GetTaskSession(ctx, sessionID)
	if loadErr != nil || session == nil || session.State != state ||
		models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != attemptID ||
		session.ErrorMessage != failure.SafeDetail() {
		return
	}
	copy := *session
	copy.Metadata = cloneMetadata(session.Metadata)
	failure.ObservedSession = &copy
}
