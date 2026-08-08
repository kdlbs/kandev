package routingerr

import "time"

// ExecutionContext selects the owner of recovery. Kanban replays one user
// turn; Office owns unattended route selection and may use the long-horizon
// provider-health scheduler after the short retry phase.
type ExecutionContext string

const (
	ContextKanban ExecutionContext = "kanban"
	ContextOffice ExecutionContext = "office"
)

// RecoveryDecision is deliberately small and policy-level. Adapters classify
// evidence; workspace owners decide whether that evidence can be replayed.
type RecoveryDecision string

const (
	DecisionManual     RecoveryDecision = "manual"
	DecisionShortRetry RecoveryDecision = "short_retry"
	DecisionLongRetry  RecoveryDecision = "long_retry"
	DecisionFallback   RecoveryDecision = "fallback"
	DecisionPark       RecoveryDecision = "park"
)

// Decide maps a normalized error to the workspace recovery action. Unknown or
// hard user-action errors fail closed and never enter an automatic retry loop.
func Decide(context ExecutionContext, e *Error, now time.Time) RecoveryDecision {
	if e == nil {
		return DecisionManual
	}
	if isShortRetryable(e, now) {
		return DecisionShortRetry
	}

	switch context {
	case ContextOffice:
		switch e.Code {
		case CodeRateLimited, CodeQuotaLimited, CodeProviderUnavailable, CodeProviderOverloaded, CodeModelCapacity:
			return DecisionLongRetry
		case CodeAuthRequired, CodeMissingCredentials, CodeSubscriptionRequired,
			CodeModelUnavailable, CodeProviderNotConfigured:
			return DecisionFallback
		default:
			return DecisionPark
		}
	default:
		return DecisionManual
	}
}

func isShortRetryable(e *Error, now time.Time) bool {
	switch e.Code {
	case CodeNetworkUnavailable, CodeProviderUnavailable, CodeProviderOverloaded, CodeModelCapacity:
		return e.Confidence == ConfHigh || e.Confidence == ConfMedium
	case CodeRateLimited:
		if e.ResetHint == nil || e.ResetHint.IsZero() {
			return false
		}
		if now.IsZero() {
			now = time.Now()
		}
		return !e.ResetHint.Before(now) && e.ResetHint.Sub(now) <= time.Minute
	default:
		return false
	}
}

// IsShortRetryable exposes the shared short-retry predicate to workspace
// owners that already have their own decision table.
func IsShortRetryable(context ExecutionContext, e *Error, now time.Time) bool {
	if context != ContextKanban && context != ContextOffice {
		return false
	}
	return Decide(context, e, now) == DecisionShortRetry
}
