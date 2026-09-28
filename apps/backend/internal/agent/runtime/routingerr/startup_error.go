package routingerr

import (
	"context"
	"errors"
	"fmt"
)

// ManagedRuntimeStartupError carries the final normalized classification from
// a managed-runtime startup attempt through the lifecycle and orchestrator
// error wrappers. Details are already sanitized and bounded by the caller.
type ManagedRuntimeStartupError struct {
	Code    Code
	Details string
	Cause   error
}

func (e *ManagedRuntimeStartupError) Error() string {
	if e == nil {
		return ""
	}
	if e.Details == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Details)
}

func (e *ManagedRuntimeStartupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// AgentStartupFailure marks an error from the concrete agent-process or ACP
// session-initialization boundary. Preparation, executor, and task errors must
// not be wrapped with this type.
type AgentStartupFailure struct {
	Phase      Phase
	ProviderID string
	Diagnostic string
	Cause      error
}

func (e *AgentStartupFailure) Error() string {
	if e == nil {
		return "agent startup failed"
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Diagnostic
}

func (e *AgentStartupFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewAgentStartupFailure creates trusted stage evidence only for process
// start and ACP session initialization. Cancellation and managed-runtime
// policy failures retain their original types and cannot enter this path.
func NewAgentStartupFailure(phase Phase, providerID string, cause error) error {
	if cause == nil || (phase != PhaseProcessStart && phase != PhaseSessionInit) ||
		errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	var managedRuntime *ManagedRuntimeStartupError
	if errors.As(cause, &managedRuntime) {
		return cause
	}
	return &AgentStartupFailure{
		Phase: phase, ProviderID: providerID, Diagnostic: cause.Error(), Cause: cause,
	}
}
