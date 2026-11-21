package reposync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

func TestRetryPolicyDelayUsesEqualJitterAndCapsPreJitterDelay(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		failures int
		jitter   func(time.Duration) time.Duration
		want     time.Duration
	}{
		{name: "first lower bound", interval: 60, failures: 1, jitter: func(time.Duration) time.Duration { return 0 }, want: 30 * time.Second},
		{name: "first upper bound", interval: 60, failures: 1, jitter: func(max time.Duration) time.Duration { return max }, want: time.Minute},
		{name: "second doubles", interval: 60, failures: 2, jitter: func(time.Duration) time.Duration { return 0 }, want: time.Minute},
		{name: "cap lower bound", interval: 60, failures: 20, jitter: func(time.Duration) time.Duration { return 0 }, want: 30 * time.Minute},
		{name: "cap upper bound", interval: 60, failures: 20, jitter: func(max time.Duration) time.Duration { return max }, want: time.Hour},
		{name: "oversized interval lower bound", interval: 30 * 24 * 60 * 60, failures: 1, jitter: func(time.Duration) time.Duration { return 0 }, want: 30 * time.Minute},
		{name: "oversized interval upper bound", interval: 30 * 24 * 60 * 60, failures: 1, jitter: func(max time.Duration) time.Duration { return max }, want: time.Hour},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, retryPolicyDelay(test.interval, test.failures, test.jitter))
		})
	}
}

func TestBuildFailureDirectiveHonorsProviderRetryLowerBound(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	providerRetry := now.Add(17 * time.Minute)
	directive := BuildFailureDirective(FailureInput{Provider: ProviderGitHub, IntervalSeconds: 60}, &github.GitHubAPIError{
		StatusCode: 429, FailureKind: github.FailureSecondaryRateLimit,
		RetryAt: providerRetry, RetrySource: github.RetrySourceRetryAfter,
	}, now, func(time.Duration) time.Duration { return 0 })

	require.NotNil(t, directive.NextAttemptAt)
	assert.Equal(t, providerRetry, *directive.NextAttemptAt)
	assert.Equal(t, github.RetrySourceRetryAfter, directive.RetrySource)
	assert.Equal(t, string(github.FailureSecondaryRateLimit), directive.Class)
}

func TestBuildFailureDirectiveHonorsCanceledAdmissionRetryLowerBound(t *testing.T) {
	now := time.Date(2026, 9, 9, 7, 0, 0, 0, time.UTC)
	providerRetry := now.Add(17 * time.Minute)
	directive := BuildFailureDirective(FailureInput{Provider: ProviderGitHub, IntervalSeconds: 60}, &github.AdmissionWaitError{
		Resource: github.ResourceCore, RetryAt: providerRetry,
		RetrySource: github.RetrySourceRetryAfter,
		Reason:      "observed_secondary_rate_limit", Cause: context.Canceled,
	}, now, func(time.Duration) time.Duration { return 0 })

	require.NotNil(t, directive.NextAttemptAt)
	assert.Equal(t, providerRetry, *directive.NextAttemptAt)
	assert.Equal(t, github.RetrySourceRetryAfter, directive.RetrySource)
	assert.Equal(t, string(github.FailureTransient), directive.Class)
}

func TestBuildFailureDirectiveSuspendsOnlyPermanentGitHubFailures(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	for _, kind := range []github.FailureKind{
		github.FailureInvalidCredentials,
		github.FailureMissingResource,
	} {
		directive := BuildFailureDirective(FailureInput{Provider: ProviderGitHub, IntervalSeconds: 60}, &github.GitHubAPIError{StatusCode: 403, FailureKind: kind}, now, DefaultJitter)
		assert.True(t, directive.Suspended, "kind %s", kind)
		assert.Nil(t, directive.NextAttemptAt, "kind %s", kind)
	}

	gitLab := BuildFailureDirective(FailureInput{Provider: ProviderGitLab, IntervalSeconds: 60}, errors.New("forbidden"), now, func(time.Duration) time.Duration { return 0 })
	assert.False(t, gitLab.Suspended)
	assert.Equal(t, string(github.FailureTransient), gitLab.Class)
	assert.NotNil(t, gitLab.NextAttemptAt)
}

func TestBuildFailureDirectiveKeepsGitLabAuthCircuitRetry(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	directive := BuildFailureDirective(FailureInput{Provider: ProviderGitLab, IntervalSeconds: 60}, &gitlab.APIError{StatusCode: 401, Endpoint: "/projects/x"}, now,
		func(time.Duration) time.Duration { return 0 })

	assert.False(t, directive.Suspended)
	assert.Equal(t, authcircuit.FailureClassAuth, directive.CircuitClass)
	assert.Equal(t, string(authcircuit.FailureClassAuth), directive.Class)
	require.NotNil(t, directive.NextAttemptAt)
	assert.False(t, directive.NextAttemptAt.Before(now.Add(authcircuit.PermanentBackoff.Base)))
	assert.False(t, directive.NextAttemptAt.After(now.Add(3*time.Minute)))
}

func TestFailureKindTreatsMissingAndInvalidGitHubConnectionsAsPermanent(t *testing.T) {
	for _, err := range []error{github.ErrGitHubNotConfigured, github.ErrGitHubConnectionInvalid} {
		if got := failureKind(ProviderGitHub, err); got != github.FailureInvalidCredentials {
			t.Errorf("failureKind(%v) = %q, want invalid_credentials", err, got)
		}
	}
}

func TestEqualJitterSeamCanSpreadWorkspaceRetries(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	seen := map[time.Time]bool{}
	for i := 0; i < 6; i++ {
		offset := time.Duration(i) * time.Second
		directive := BuildFailureDirective(FailureInput{Provider: ProviderGitHub, IntervalSeconds: 60}, errors.New("temporary"), now, func(time.Duration) time.Duration { return offset })
		require.NotNil(t, directive.NextAttemptAt)
		seen[*directive.NextAttemptAt] = true
	}
	assert.Len(t, seen, 6)
}
