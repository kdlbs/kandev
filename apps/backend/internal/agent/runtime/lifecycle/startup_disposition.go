package lifecycle

import (
	"errors"
	"fmt"
)

// AgentStartupDisposition records whether startup created a process or
// attached to one already owned by a live executor.
type AgentStartupDisposition string

const (
	AgentStartupCreatedByAttempt   AgentStartupDisposition = "created_by_attempt"
	AgentStartupReattachedExisting AgentStartupDisposition = "reattached_existing"
)

var ErrAgentReattachment = errors.New("existing agent reattachment failed")

// AgentReattachmentFailure keeps cleanup ownership attached to the failed
// operation. Its cause remains available for logging and error classification.
type AgentReattachmentFailure struct {
	ExecutionID     string
	SessionID       string
	ResumeAttemptID string
	Cause           error
}

func (e *AgentReattachmentFailure) Error() string {
	if e == nil {
		return ErrAgentReattachment.Error()
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s for execution %q", ErrAgentReattachment, e.ExecutionID)
	}
	return fmt.Sprintf("%s for execution %q: %v", ErrAgentReattachment, e.ExecutionID, e.Cause)
}

func (e *AgentReattachmentFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *AgentReattachmentFailure) Is(target error) bool {
	return target == ErrAgentReattachment
}

func startupDispositionFromMetadata(metadata map[string]interface{}) AgentStartupDisposition {
	if reuse, _ := metadata[MetadataKeyReuseExistingProcess].(bool); reuse {
		return AgentStartupReattachedExisting
	}
	return AgentStartupCreatedByAttempt
}

func (e *AgentExecution) startupDispositionSnapshot() AgentStartupDisposition {
	if e == nil {
		return AgentStartupCreatedByAttempt
	}
	if e.startupDisposition != "" {
		return e.startupDisposition
	}
	if e.metadataBool(MetadataKeyReuseExistingProcess) {
		return AgentStartupReattachedExisting
	}
	return AgentStartupCreatedByAttempt
}

// StartupDisposition returns the immutable process ownership decision for an
// execution. Callers use it to avoid applying new-process cleanup to a peer.
func (m *Manager) StartupDisposition(executionID string) AgentStartupDisposition {
	if m == nil || m.executionStore == nil {
		return AgentStartupCreatedByAttempt
	}
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return AgentStartupCreatedByAttempt
	}
	return execution.startupDispositionSnapshot()
}
