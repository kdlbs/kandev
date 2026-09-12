package lifecycle

import (
	"errors"
	"fmt"
	"strings"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// RestoreAction identifies an operation that may change the native harness
// conversation. Replacement actions are never inferred from a failed load.
type RestoreAction string

const (
	RestoreActionNativeResume        RestoreAction = "native_resume"
	RestoreActionContinueFromHistory RestoreAction = "continue_from_history"
	RestoreActionResumeNewBranch     RestoreAction = "resume_new_branch"
)

// RestoreOutcome describes the continuity guarantee that the lifecycle can
// make for one restore attempt.
type RestoreOutcome string

const (
	RestoreOutcomeNativeResumed    RestoreOutcome = "native_resumed"
	RestoreOutcomeReattached       RestoreOutcome = "reattached"
	RestoreOutcomeContextContinued RestoreOutcome = "context_continued"
	RestoreOutcomeBlocked          RestoreOutcome = "blocked"
)

// RestoreReason is a bounded classification of native restore evidence. The
// reason deliberately excludes provider error text and private native paths.
type RestoreReason string

const (
	RestoreReasonNone                    RestoreReason = ""
	RestoreReasonNativeStateMissing      RestoreReason = "native_state_missing"
	RestoreReasonNativeResumeUnsupported RestoreReason = "native_resume_unsupported"
	RestoreReasonWorkspaceIncompatible   RestoreReason = "workspace_incompatible"
	RestoreReasonTransport               RestoreReason = "transport_failure"
	RestoreReasonAuthentication          RestoreReason = "authentication_failure"
	RestoreReasonConfiguration           RestoreReason = "configuration_failure"
	RestoreReasonPermission              RestoreReason = "permission_failure"
	RestoreReasonUnknown                 RestoreReason = "unknown_failure"
	RestoreReasonBranchUnrecoverable     RestoreReason = "branch_unrecoverable"
)

// RestoreIdentity names the durable Kandev and native identities involved in
// a restore. NativeSessionID remains the previously committed identity until
// a replacement generation is fully configured and committed.
type RestoreIdentity struct {
	SessionID            string
	IncarnationID        string
	HarnessGeneration    uint64
	NativeSessionID      string
	OriginalWorkspace    string
	TargetWorkspace      string
	NativeStateReference string
}

// RestoreCapabilities contains optional adapter evidence. Unknown values are
// conservative and must not authorize a replacement conversation.
type RestoreCapabilities struct {
	SupportsNativeLoad    bool
	SupportsNativeResume  bool
	SupportsDirectoryMove bool
	RequiresNativeState   bool
}

// RestoreFailure carries classified adapter evidence for a failed restore.
// Detail is diagnostic-only and must not be shown as user-facing copy.
type RestoreFailure struct {
	Reason RestoreReason
	Detail string
}

// RestoreRequest is evaluated by every restore entry point before it can
// create or select a native conversation.
type RestoreRequest struct {
	Identity              RestoreIdentity
	Capabilities          RestoreCapabilities
	Failure               RestoreFailure
	Action                RestoreAction
	ExplicitAuthorization bool
}

// RestoreDecision is intentionally conservative. PreserveNativeIdentity is
// true even for an authorized context continuation until its replacement
// generation is configured and committed.
type RestoreDecision struct {
	Outcome                   RestoreOutcome
	Reason                    RestoreReason
	PreserveNativeIdentity    bool
	AllowsContextContinuation bool
	ActionAuthorized          bool
}

// DecideRestore converts native evidence and an explicit action into a typed
// policy decision. A missing action never authorizes replacement.
func DecideRestore(request RestoreRequest) RestoreDecision {
	reason := request.Failure.Reason
	decision := RestoreDecision{
		Outcome:                RestoreOutcomeBlocked,
		Reason:                 reason,
		PreserveNativeIdentity: true,
	}

	if reason == RestoreReasonNone {
		switch request.Action {
		case RestoreActionNativeResume:
			decision.Outcome = RestoreOutcomeNativeResumed
			decision.ActionAuthorized = true
		case RestoreActionContinueFromHistory:
			if request.ExplicitAuthorization {
				decision.Outcome = RestoreOutcomeContextContinued
				decision.ActionAuthorized = true
			}
		}
		return decision
	}

	decision.AllowsContextContinuation = allowsContextContinuation(reason)
	if request.Action == RestoreActionContinueFromHistory &&
		request.ExplicitAuthorization && decision.AllowsContextContinuation {
		decision.Outcome = RestoreOutcomeContextContinued
		decision.ActionAuthorized = true
		return decision
	}
	if request.Action == RestoreActionResumeNewBranch &&
		request.ExplicitAuthorization && reason == RestoreReasonBranchUnrecoverable {
		decision.ActionAuthorized = true
	}
	if request.Action == RestoreActionResumeNewBranch {
		decision.AllowsContextContinuation = false
	}
	return decision
}

func allowsContextContinuation(reason RestoreReason) bool {
	switch reason {
	case RestoreReasonNativeStateMissing, RestoreReasonNativeResumeUnsupported, RestoreReasonWorkspaceIncompatible:
		return true
	default:
		return false
	}
}

// RestoreRequiredError reports a blocked native restore without exposing
// provider error text through its public message.
type RestoreRequiredError struct {
	Decision RestoreDecision
	Cause    error
}

func (e *RestoreRequiredError) Error() string {
	if e == nil {
		return "session restore is required"
	}
	return fmt.Sprintf("session restore blocked: %s", e.Decision.Reason)
}

func (e *RestoreRequiredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// RecoveryReason exposes only the bounded policy reason to a higher-level
// admission owner. Provider error text remains available through Unwrap for
// diagnostics inside the lifecycle boundary and is never part of this value.
func (e *RestoreRequiredError) RecoveryReason() string {
	if e == nil {
		return string(RestoreReasonUnknown)
	}
	return string(e.Decision.Reason)
}

func newRestoreRequiredError(identity RestoreIdentity, err error) *RestoreRequiredError {
	reason := classifyRestoreFailure(err)
	return &RestoreRequiredError{
		Decision: DecideRestore(RestoreRequest{
			Identity: identity,
			Failure:  RestoreFailure{Reason: reason, Detail: errString(err)},
			Action:   RestoreActionNativeResume,
		}),
		Cause: err,
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func classifyRestoreFailure(err error) RestoreReason {
	if err == nil {
		return RestoreReasonNone
	}
	if reason, typed := classifyTypedRestoreFailure(err); typed {
		return reason
	}
	if isTransportDeadErr(err) {
		return RestoreReasonTransport
	}
	if isSessionUnknownErr(err) {
		return RestoreReasonNativeStateMissing
	}
	if isMethodNotFoundErr(err) || strings.Contains(err.Error(), "LoadSession capability is false") {
		return RestoreReasonNativeResumeUnsupported
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "authentication") || strings.Contains(message, "auth required"):
		return RestoreReasonAuthentication
	case strings.Contains(message, "permission") || strings.Contains(message, "forbidden"):
		return RestoreReasonPermission
	case strings.Contains(message, "configuration") || strings.Contains(message, "config option"):
		return RestoreReasonConfiguration
	default:
		return RestoreReasonUnknown
	}
}

func classifyTypedRestoreFailure(err error) (RestoreReason, bool) {
	var typed *agentctl.SessionRestoreOperationError
	if !errors.As(err, &typed) {
		return RestoreReasonNone, false
	}
	reason, _ := typed.Details["reason"].(string)
	switch reason {
	case string(RestoreReasonNativeStateMissing):
		return RestoreReasonNativeStateMissing, true
	case string(RestoreReasonNativeResumeUnsupported):
		return RestoreReasonNativeResumeUnsupported, true
	case string(RestoreReasonWorkspaceIncompatible):
		return RestoreReasonWorkspaceIncompatible, true
	default:
		return RestoreReasonUnknown, true
	}
}
