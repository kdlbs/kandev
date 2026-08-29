package workflowsync

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/kandev/kandev/internal/github"
)

const maxRetryDelay = time.Hour

type failureDirective struct {
	class            string
	consecutive      int
	nextAttemptAt    *time.Time
	suspended        bool
	suspensionReason string
	retrySource      github.RetrySource
}

func defaultJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(max) + 1))
}

func buildFailureDirective(
	cfg *Config,
	syncErr error,
	now time.Time,
	jitter func(time.Duration) time.Duration,
) failureDirective {
	kind := failureKind(cfg.Provider, syncErr)
	directive := failureDirective{
		class:       string(kind),
		consecutive: cfg.ConsecutiveFailures + 1,
	}
	if kind == github.FailureInvalidCredentials || kind == github.FailureMissingResource {
		directive.suspended = true
		directive.suspensionReason = syncErr.Error()
		return directive
	}

	policyDelay := retryPolicyDelay(cfg.IntervalSeconds, directive.consecutive, jitter)
	retryAt := now.Add(policyDelay).UTC()
	var apiErr *github.GitHubAPIError
	if errors.As(syncErr, &apiErr) {
		directive.retrySource = apiErr.RetrySource
		if apiErr.RetryAt.After(retryAt) {
			retryAt = apiErr.RetryAt.UTC()
		}
	}
	directive.nextAttemptAt = &retryAt
	return directive
}

func failureKind(provider string, syncErr error) github.FailureKind {
	if provider == ProviderGitLab {
		return github.FailureTransient
	}
	if errors.Is(syncErr, github.ErrNoClient) {
		return github.FailureInvalidCredentials
	}
	kind := github.FailureKindOf(syncErr)
	if kind == github.FailureUnknown || kind == "" {
		return github.FailureTransient
	}
	return kind
}

func retryPolicyDelay(
	intervalSeconds int,
	consecutiveFailures int,
	jitter func(time.Duration) time.Duration,
) time.Duration {
	delay := time.Duration(intervalSeconds) * time.Second
	if delay < time.Minute {
		delay = time.Minute
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
