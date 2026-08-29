package workflowsync

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
)

func TestRetryPolicyDelayUsesEqualJitterAndCapsPreJitterDelay(t *testing.T) {
	tests := []struct {
		name     string
		failures int
		jitter   func(time.Duration) time.Duration
		want     time.Duration
	}{
		{name: "first lower bound", failures: 1, jitter: func(time.Duration) time.Duration { return 0 }, want: 30 * time.Second},
		{name: "first upper bound", failures: 1, jitter: func(max time.Duration) time.Duration { return max }, want: time.Minute},
		{name: "second doubles", failures: 2, jitter: func(time.Duration) time.Duration { return 0 }, want: time.Minute},
		{name: "cap lower bound", failures: 20, jitter: func(time.Duration) time.Duration { return 0 }, want: 30 * time.Minute},
		{name: "cap upper bound", failures: 20, jitter: func(max time.Duration) time.Duration { return max }, want: time.Hour},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, retryPolicyDelay(60, test.failures, test.jitter))
		})
	}
}

func TestBuildFailureDirectiveHonorsProviderRetryLowerBound(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	providerRetry := now.Add(17 * time.Minute)
	directive := buildFailureDirective(&Config{
		Provider: ProviderGitHub, IntervalSeconds: 60,
	}, &github.GitHubAPIError{
		StatusCode: 429, FailureKind: github.FailureSecondaryRateLimit,
		RetryAt: providerRetry, RetrySource: github.RetrySourceRetryAfter,
	}, now, func(time.Duration) time.Duration { return 0 })

	require.NotNil(t, directive.nextAttemptAt)
	assert.Equal(t, providerRetry, *directive.nextAttemptAt)
	assert.Equal(t, github.RetrySourceRetryAfter, directive.retrySource)
	assert.Equal(t, string(github.FailureSecondaryRateLimit), directive.class)
}

func TestBuildFailureDirectiveSuspendsOnlyPermanentGitHubFailures(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	for _, kind := range []github.FailureKind{
		github.FailureInvalidCredentials,
		github.FailureMissingResource,
	} {
		directive := buildFailureDirective(&Config{
			Provider: ProviderGitHub, IntervalSeconds: 60,
		}, &github.GitHubAPIError{StatusCode: 403, FailureKind: kind}, now, defaultJitter)
		assert.True(t, directive.suspended, "kind %s", kind)
		assert.Nil(t, directive.nextAttemptAt, "kind %s", kind)
	}

	gitLab := buildFailureDirective(&Config{
		Provider: ProviderGitLab, IntervalSeconds: 60,
	}, errors.New("forbidden"), now, func(time.Duration) time.Duration { return 0 })
	assert.False(t, gitLab.suspended)
	assert.Equal(t, string(github.FailureTransient), gitLab.class)
	assert.NotNil(t, gitLab.nextAttemptAt)
}

func TestEqualJitterSeamCanSpreadWorkspaceRetries(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	seen := map[time.Time]bool{}
	for i := 0; i < 6; i++ {
		offset := time.Duration(i) * time.Second
		directive := buildFailureDirective(&Config{
			Provider: ProviderGitHub, IntervalSeconds: 60,
		}, errors.New("temporary"), now, func(time.Duration) time.Duration { return offset })
		require.NotNil(t, directive.nextAttemptAt)
		seen[*directive.nextAttemptAt] = true
	}
	assert.Len(t, seen, 6)
}
