package process

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCreatePRWithRetryAfterPushClearsTransientResult(t *testing.T) {
	operator := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	operator.prCreateRetryAttempts = 2
	operator.prCreateRetryBaseDelay = 0
	result := &PRCreateResult{
		Provider:     string(prProviderGitHub),
		BranchPushed: true,
	}
	attempts := 0

	got, err := operator.createPRWithRetryAfterPush(
		context.Background(), result, "title", "body",
		func() (*PRCreateResult, error) {
			attempts++
			if attempts == 1 {
				result.Error = "no commits between base and head"
				result.Output = "provider failure"
				return result, nil
			}
			result.Success = true
			result.PRURL = "https://github.com/acme/repo/pull/1"
			result.Output = "created"
			return result, nil
		},
	)
	if err != nil {
		t.Fatalf("createPRWithRetryAfterPush returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if !got.Success || got.Error != "" || got.Output != "created" {
		t.Fatalf("result = %+v, want clean successful result", got)
	}
}

func TestCreatePRWithRetryAfterPushDoesNotRetryPermanentFailure(t *testing.T) {
	operator := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	operator.prCreateRetryAttempts = 3
	operator.prCreateRetryBaseDelay = 0
	result := &PRCreateResult{
		Provider:     string(prProviderGitHub),
		BranchPushed: true,
	}
	attempts := 0

	got, err := operator.createPRWithRetryAfterPush(
		context.Background(), result, "title", "body",
		func() (*PRCreateResult, error) {
			attempts++
			result.Error = "authentication failed"
			return result, nil
		},
	)
	if err != nil {
		t.Fatalf("createPRWithRetryAfterPush returned error: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 for permanent failure", attempts)
	}
	if got.Success || !got.BranchPushed || got.Error != "branch was pushed; retry pull request creation" {
		t.Fatalf("result = %+v, want generic partial result", got)
	}
}

func TestCreatePRWithRetryAfterPushExhaustsTransientFailures(t *testing.T) {
	operator := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	operator.prCreateRetryAttempts = 3
	operator.prCreateRetryBaseDelay = 0
	result := &PRCreateResult{
		Provider:     string(prProviderGitHub),
		BranchPushed: true,
	}
	attempts := 0

	got, err := operator.createPRWithRetryAfterPush(
		context.Background(), result, "title", "body",
		func() (*PRCreateResult, error) {
			attempts++
			result.Error = "no commits between base and head"
			return result, nil
		},
	)
	if err != nil {
		t.Fatalf("createPRWithRetryAfterPush returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if got.Success || !got.BranchPushed || got.Error != "branch was pushed; retry pull request creation" {
		t.Fatalf("result = %+v, want generic partial result", got)
	}
}

func TestCreatePRWithRetryAfterPushStopsWhenContextCanceledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	operator := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	operator.prCreateRetryAttempts = 3
	operator.prCreateRetryBaseDelay = time.Hour
	result := &PRCreateResult{
		Provider:     string(prProviderGitHub),
		BranchPushed: true,
	}
	attempts := 0

	got, err := operator.createPRWithRetryAfterPush(
		ctx, result, "title", "body",
		func() (*PRCreateResult, error) {
			attempts++
			result.Error = "no commits between base and head"
			cancel()
			return result, nil
		},
	)
	if err != nil {
		t.Fatalf("createPRWithRetryAfterPush returned error: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 after cancellation", attempts)
	}
	if got.Error != "branch was pushed; retry pull request creation" {
		t.Fatalf("result = %+v, want generic partial result", got)
	}
}

func TestPRCreateRetryDelayUsesLinearBackoff(t *testing.T) {
	for _, test := range []struct {
		name       string
		base       time.Duration
		retryIndex int
		want       time.Duration
	}{
		{name: "first attempt", base: 2 * time.Second, retryIndex: 0, want: 0},
		{name: "first retry", base: 2 * time.Second, retryIndex: 1, want: 2 * time.Second},
		{name: "second retry", base: 2 * time.Second, retryIndex: 2, want: 4 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := prCreateRetryDelay(test.base, test.retryIndex); got != test.want {
				t.Fatalf("delay = %s, want %s", got, test.want)
			}
		})
	}
}

func TestCreatePRWithRetryAfterPushPreservesPushMetadata(t *testing.T) {
	operator := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	operator.prCreateRetryAttempts = 1
	result := &PRCreateResult{
		Provider:     string(prProviderGitHub),
		BranchPushed: true,
	}

	got, err := operator.createPRWithRetryAfterPush(
		context.Background(), result, "title", "body",
		func() (*PRCreateResult, error) {
			return &PRCreateResult{
				Success: true,
				PRURL:   "https://github.com/acme/repo/pull/1",
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("createPRWithRetryAfterPush returned error: %v", err)
	}
	if !got.Success || !got.BranchPushed || got.Provider != string(prProviderGitHub) {
		t.Fatalf("result = %+v, want successful result with push metadata", got)
	}
}

func TestCreatePRWithRetryAfterPushClassifiesProviderOutput(t *testing.T) {
	operator := NewGitOperator(t.TempDir(), newTestLogger(t), nil)
	operator.prCreateRetryAttempts = 2
	operator.prCreateRetryBaseDelay = 0
	result := &PRCreateResult{
		Provider:     string(prProviderGitHub),
		BranchPushed: true,
	}
	attempts := 0

	got, err := operator.createPRWithRetryAfterPush(
		context.Background(), result, "title", "body",
		func() (*PRCreateResult, error) {
			attempts++
			if attempts == 1 {
				result.Error = "provider rejected pull request creation"
				result.Output = "no commits between base and head"
				return result, nil
			}
			result.Success = true
			result.PRURL = "https://github.com/acme/repo/pull/1"
			return result, nil
		},
	)
	if err != nil {
		t.Fatalf("createPRWithRetryAfterPush returned error: %v", err)
	}
	if attempts != 2 || !got.Success {
		t.Fatalf("attempts = %d, result = %+v, want retry and success", attempts, got)
	}
}

func TestCreatePRWithRetryAfterPushSanitizesRetryWarning(t *testing.T) {
	log, observed := newObservedTestLogger(t)
	operator := NewGitOperator(t.TempDir(), log, nil)
	operator.prCreateRetryAttempts = 2
	operator.prCreateRetryBaseDelay = 0
	result := &PRCreateResult{
		Provider:     string(prProviderGitHub),
		BranchPushed: true,
	}
	attempts := 0

	_, err := operator.createPRWithRetryAfterPush(
		context.Background(), result, "Sensitive title", "body",
		func() (*PRCreateResult, error) {
			attempts++
			if attempts == 1 {
				result.Error = "no commits between base and head: https://oauth2:embedded-token@git.example/repo Sensitive title"
				return result, nil
			}
			result.Success = true
			result.PRURL = "https://github.com/acme/repo/pull/1"
			return result, nil
		},
	)
	if err != nil {
		t.Fatalf("createPRWithRetryAfterPush returned error: %v", err)
	}
	entries := observed.FilterMessage("PR creation attempt failed after push; retrying").All()
	if len(entries) != 1 {
		t.Fatalf("retry warning entries = %d, want 1", len(entries))
	}
	loggedError := fmt.Sprint(entries[0].ContextMap()["error"])
	for _, secret := range []string{"embedded-token", "Sensitive title"} {
		if strings.Contains(loggedError, secret) {
			t.Fatalf("retry warning contains %q: %q", secret, loggedError)
		}
	}
}
