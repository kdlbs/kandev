package lifecycle

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
)

// StopReasonAgentBootstrapFailed lets the lifecycle manager reclaim a fresh
// failed launch while preserving an already-retained Kubernetes runtime.
const StopReasonAgentBootstrapFailed = "agent bootstrap failed"

// BootstrapFailure carries safe, operation-boundary evidence for a failure
// before the agent becomes ready. Cause remains available to backend logging
// and errors.Is/errors.As callers; user-facing projections must use Code and
// Detail instead of Error().
type BootstrapFailure struct {
	Operation      string
	Code           string
	Reason         string
	Detail         string
	RequestedModel string
	EffectiveModel string
	AttemptedModel string
	RequestedMode  string
	EffectiveMode  string
	PromptNotSent  *bool
	Cause          error
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
	if e == nil {
		return ""
	}
	if e.Detail == "" || isSelectionBootstrapCause(e.safeCode()) {
		return bootstrapFailureDetail(e.safeCode())
	}
	return routingerr.Sanitize(e.Detail)
}

// SafeDetail returns the operation-boundary detail. Callers must use this
// field instead of Error when building durable or user-facing records.
func (e *BootstrapFailure) SafeDetail() string {
	return e.safeDetail()
}

// SafeAgentErrorCause returns the bounded structured projection used by
// persistence and transport. Selector values are rejected when the routing
// sanitizer changes them; arbitrary provider errors remain only in Cause.
func (e *BootstrapFailure) SafeAgentErrorCause(operation string) (models.AgentErrorCause, bool) {
	if e == nil {
		return models.AgentErrorCause{}, false
	}
	if operation == "" {
		operation = e.Operation
	}
	cause := models.AgentErrorCause{
		Operation:      operation,
		Code:           e.SafeCode(),
		Detail:         e.SafeDetail(),
		Reason:         e.Reason,
		RequestedModel: safeBootstrapSelector(e.RequestedModel),
		EffectiveModel: safeBootstrapSelector(e.EffectiveModel),
		AttemptedModel: safeBootstrapSelector(e.AttemptedModel),
		RequestedMode:  safeBootstrapSelector(e.RequestedMode),
		EffectiveMode:  safeBootstrapSelector(e.EffectiveMode),
	}
	if e.PromptNotSent != nil {
		value := *e.PromptNotSent
		cause.PromptNotSent = &value
	}
	normalized := models.NormalizeAgentErrorCauses([]models.AgentErrorCause{cause})
	if len(normalized) == 0 {
		return models.AgentErrorCause{}, false
	}
	return normalized[0], true
}

func safeBootstrapSelector(value string) string {
	if len(value) > 256 || strings.TrimSpace(value) != value {
		return ""
	}
	if routingerr.SanitizeFullUnbounded(value) != value {
		return ""
	}
	return value
}

func isSelectionBootstrapCause(code string) bool {
	switch code {
	case models.AgentErrorCauseCodeModelUnavailable,
		models.AgentErrorCauseCodeModelSelectionFailed,
		models.AgentErrorCauseCodePermissionModeFailed,
		models.AgentErrorCauseCodePermissionModeUnconfirmed,
		models.AgentErrorCauseCodePermissionModeMismatch:
		return true
	default:
		return false
	}
}

func isKnownBootstrapCauseCode(code string) bool {
	switch code {
	case models.AgentErrorCauseCodeAuthenticationRequired,
		models.AgentErrorCauseCodePermissionDenied,
		models.AgentErrorCauseCodeDestinationInvalid,
		models.AgentErrorCauseCodeSourceBranchMissing,
		models.AgentErrorCauseCodeTransportUnavailable,
		models.AgentErrorCauseCodeTimeout,
		models.AgentErrorCauseCodeModelUnavailable,
		models.AgentErrorCauseCodeModelSelectionFailed,
		models.AgentErrorCauseCodePermissionModeFailed,
		models.AgentErrorCauseCodePermissionModeUnconfirmed,
		models.AgentErrorCauseCodePermissionModeMismatch,
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
	return models.AgentErrorCauseOperationStart
}

func bootstrapFailureFor(execution *AgentExecution, err error) *BootstrapFailure {
	if err == nil {
		return nil
	}
	var existing *BootstrapFailure
	if errors.As(err, &existing) {
		if existing != nil && existing.Operation == "" {
			copy := *existing
			copy.Operation = bootstrapOperation(execution)
			return &copy
		}
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
	case models.AgentErrorCauseCodeModelUnavailable:
		return "The requested model is unavailable."
	case models.AgentErrorCauseCodeModelSelectionFailed:
		return "The requested model could not be selected."
	case models.AgentErrorCauseCodePermissionModeFailed:
		return "The requested permission mode could not be applied."
	case models.AgentErrorCauseCodePermissionModeUnconfirmed:
		return "The requested permission mode was not confirmed."
	case models.AgentErrorCauseCodePermissionModeMismatch:
		return "The applied permission mode did not match the request."
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
