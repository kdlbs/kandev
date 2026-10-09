package lifecycle

import "fmt"

type SessionInitializationPhase string

const SessionInitializationPhaseACPInitialize SessionInitializationPhase = "acp_initialize"

// SessionInitializationPhaseError identifies which remote startup operation
// failed without labeling later session work as an initialize failure.
type SessionInitializationPhaseError struct {
	Phase SessionInitializationPhase
	Cause error
}

func (e *SessionInitializationPhaseError) Error() string {
	return fmt.Sprintf("%s failed: %v", e.Phase, e.Cause)
}

func (e *SessionInitializationPhaseError) Unwrap() error { return e.Cause }
