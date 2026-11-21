package reposync

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/github"
)

const maxRetryDelay = time.Hour

// FailureInput is the persisted recovery state a failure directive builds on.
type FailureInput struct {
	Provider            string
	IntervalSeconds     int
	ConsecutiveFailures int
	// Circuit is the current authcircuit state; its NextRetryAt is a lower
	// bound for the next attempt.
	Circuit authcircuit.State
	// GenericMessage is the feature's sanitized fallback failure summary.
	GenericMessage string
}

// FailureDirective is the recovery decision for one failed sync attempt.
type FailureDirective struct {
	Class            string
	Consecutive      int
	NextAttemptAt    *time.Time
	Suspended        bool
	SuspensionReason string
	RetrySource      github.RetrySource
	CircuitClass     authcircuit.FailureClass
}

// DefaultJitter returns a uniformly random duration in [0, max].
func DefaultJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(max) + 1))
}

// BuildFailureDirective decides how a failed attempt is retried: definite
// GitHub credential or missing-resource failures suspend polling, GitLab
// non-transient failures follow the authcircuit schedule, and everything
// else backs off with equal jitter, never earlier than a provider retry time.
func BuildFailureDirective(
	input FailureInput,
	syncErr error,
	now time.Time,
	jitter func(time.Duration) time.Duration,
) FailureDirective {
	kind := failureKind(input.Provider, syncErr)
	directive := FailureDirective{
		Class:        string(kind),
		Consecutive:  input.ConsecutiveFailures + 1,
		CircuitClass: classifyError(syncErr),
	}
	if input.Provider == ProviderGitHub &&
		(kind == github.FailureInvalidCredentials || kind == github.FailureMissingResource) {
		directive.Suspended = true
		directive.SuspensionReason = SafeErrorMessage(syncErr, input.GenericMessage)
		return directive
	}
	if input.Provider == ProviderGitLab {
		directive.Class = string(directive.CircuitClass)
		if directive.CircuitClass != authcircuit.FailureClassTransient {
			state := input.Circuit
			state.RecordFailure(now, directive.CircuitClass, nil)
			directive.NextAttemptAt = state.NextRetryAt
			return directive
		}
	}

	retryAt := now.Add(retryPolicyDelay(input.IntervalSeconds, directive.Consecutive, jitter)).UTC()
	if next := input.Circuit.NextRetryAt; next != nil && next.After(retryAt) {
		retryAt = next.UTC()
	}
	var apiErr *github.GitHubAPIError
	if errors.As(syncErr, &apiErr) {
		directive.RetrySource = apiErr.RetrySource
		if apiErr.RetryAt.After(retryAt) {
			retryAt = apiErr.RetryAt.UTC()
		}
	}
	var admissionWait *github.AdmissionWaitError
	if errors.As(syncErr, &admissionWait) {
		directive.RetrySource = admissionWait.RetrySource
		if admissionWait.RetryAt.After(retryAt) {
			retryAt = admissionWait.RetryAt.UTC()
		}
	}
	directive.NextAttemptAt = &retryAt
	return directive
}

func failureKind(provider string, syncErr error) github.FailureKind {
	if provider == ProviderGitLab {
		return github.FailureTransient
	}
	if errors.Is(syncErr, github.ErrNoClient) ||
		errors.Is(syncErr, ErrGitHubClientNotConfigured) ||
		errors.Is(syncErr, github.ErrGitHubNotConfigured) ||
		errors.Is(syncErr, github.ErrGitHubConnectionInvalid) {
		return github.FailureInvalidCredentials
	}
	kind := github.FailureKindOf(syncErr)
	if kind == github.FailureUnknown || kind == "" {
		return github.FailureTransient
	}
	return kind
}

// retryPolicyDelay is the equal-jitter exponential backoff: the base is the
// larger of the interval and one minute, each consecutive failure doubles it,
// and the pre-jitter delay stops growing at one hour.
func retryPolicyDelay(
	intervalSeconds int,
	consecutiveFailures int,
	jitter func(time.Duration) time.Duration,
) time.Duration {
	delay := time.Duration(intervalSeconds) * time.Second
	if delay < time.Minute {
		delay = time.Minute
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	for i := 1; i < consecutiveFailures && delay < maxRetryDelay; i++ {
		delay *= 2
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}
	}
	half := delay / 2
	extra := jitter(half)
	if extra < 0 {
		extra = 0
	}
	if extra > half {
		extra = half
	}
	return half + extra
}
