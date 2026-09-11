package acp

import (
	"errors"
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"
)

const (
	SessionRestoreReasonNativeStateMissing      = "native_state_missing"
	SessionRestoreReasonNativeResumeUnsupported = "native_resume_unsupported"
	SessionRestoreReasonUnknown                 = "unknown_failure"
)

// SessionRestoreError retains the ACP evidence needed by the host recovery
// policy. Generic adapter errors must not be interpreted as missing state.
type SessionRestoreError struct {
	Reason string
	Cause  error
}

func (e *SessionRestoreError) Error() string {
	if e == nil || e.Cause == nil {
		return "session restore failed"
	}
	return fmt.Sprintf("session restore failed: %v", e.Cause)
}

func (e *SessionRestoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func sessionRestoreReason(err error) string {
	var requestErr *acpsdk.RequestError
	if !errors.As(err, &requestErr) {
		return SessionRestoreReasonUnknown
	}
	switch requestErr.Code {
	case -32002:
		return SessionRestoreReasonNativeStateMissing
	case -32601:
		return SessionRestoreReasonNativeResumeUnsupported
	default:
		return SessionRestoreReasonUnknown
	}
}
