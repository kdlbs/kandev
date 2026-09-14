package lifecycle

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// BootstrapFailure carries safe, operation-boundary evidence for a failure
// before the agent becomes ready. Cause remains available to backend logging
// and errors.Is/errors.As callers; user-facing projections must use Code and
// Detail instead of Error().
type BootstrapFailure struct {
	Operation string
	Code      string
	Detail    string
	Cause     error
}

func (e *BootstrapFailure) Error() string {
	if e == nil {
		return "agent bootstrap failed"
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Detail != "" {
		return e.Detail
	}
	return "agent bootstrap failed"
}

func (e *BootstrapFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *BootstrapFailure) safeCode() string {
	if e == nil || !isKnownBootstrapCauseCode(e.Code) {
		return models.AgentErrorCauseCodeUnknown
	}
	return e.Code
}

// SafeCode returns the allowlisted operation-boundary reason for user-facing
// recovery projections. It never exposes the wrapped provider or transport
// error.
func (e *BootstrapFailure) SafeCode() string {
	return e.safeCode()
}

func (e *BootstrapFailure) safeDetail() string {
	if e == nil || e.Detail == "" {
		return ""
	}
	return e.Detail
}

// SafeDetail returns the operation-boundary detail. Callers must use this
// field instead of Error when building durable or user-facing records.
func (e *BootstrapFailure) SafeDetail() string {
	return e.safeDetail()
}

func isKnownBootstrapCauseCode(code string) bool {
	switch code {
	case models.AgentErrorCauseCodeAuthenticationRequired,
		models.AgentErrorCauseCodePermissionDenied,
		models.AgentErrorCauseCodeDestinationInvalid,
		models.AgentErrorCauseCodeSourceBranchMissing,
		models.AgentErrorCauseCodeTransportUnavailable,
		models.AgentErrorCauseCodeTimeout,
		models.AgentErrorCauseCodeUnknown:
		return true
	default:
		return false
	}
}

func bootstrapOperation(execution *AgentExecution) string {
	if execution != nil && execution.isResumedSession {
		return models.AgentErrorCauseOperationResume
	}
	return ""
}

func bootstrapFailureFor(execution *AgentExecution, err error) *BootstrapFailure {
	if err == nil {
		return nil
	}
	var existing *BootstrapFailure
	if errors.As(err, &existing) {
		return existing
	}
	code := models.AgentErrorCauseCodeUnknown
	if errors.Is(err, context.DeadlineExceeded) {
		code = models.AgentErrorCauseCodeTimeout
	}
	return &BootstrapFailure{
		Operation: bootstrapOperation(execution),
		Code:      code,
		Detail:    bootstrapFailureDetail(code),
		Cause:     err,
	}
}

func bootstrapFailureDetail(code string) string {
	switch code {
	case models.AgentErrorCauseCodeAuthenticationRequired:
		return "Agent authentication is required."
	case models.AgentErrorCauseCodePermissionDenied:
		return "The required contribution access was denied."
	case models.AgentErrorCauseCodeDestinationInvalid:
		return "The contribution destination is not valid."
	case models.AgentErrorCauseCodeSourceBranchMissing:
		return "The contribution source branch is not available."
	case models.AgentErrorCauseCodeTransportUnavailable:
		return "The contribution service could not be reached."
	case models.AgentErrorCauseCodeTimeout:
		return "The bootstrap operation timed out."
	default:
		return "The bootstrap operation could not be completed."
	}
}

func wrapBootstrapFailure(execution *AgentExecution, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, ErrSessionTerminal) {
		return err
	}
	failure := bootstrapFailureFor(execution, err)
	if failure == nil {
		return err
	}
	return failure
}
