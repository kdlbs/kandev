package routingerr

import "fmt"

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
