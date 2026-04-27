package handlers

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "debug",
		Format:     "console",
		OutputPath: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return log
}

func TestResolveFreshBranchName(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		taskTitle string
		assert    func(t *testing.T, got string)
	}{
		{
			name:      "uses raw name when provided",
			raw:       "feature/x",
			taskTitle: "ignored",
			assert: func(t *testing.T, got string) {
				if got != "feature/x" {
					t.Fatalf("expected feature/x, got %q", got)
				}
			},
		},
		{
			name:      "trims whitespace from raw name",
			raw:       "  feature/y  ",
			taskTitle: "ignored",
			assert: func(t *testing.T, got string) {
				if got != "feature/y" {
					t.Fatalf("expected feature/y, got %q", got)
				}
			},
		},
		{
			name:      "derives from title when raw is empty",
			raw:       "",
			taskTitle: "Fix login bug",
			assert: func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "fix-login-bug_") {
					t.Fatalf("expected fix-login-bug_ prefix, got %q", got)
				}
			},
		},
		{
			name:      "title with only special chars falls back to suffix only",
			raw:       "",
			taskTitle: "!!!",
			assert: func(t *testing.T, got string) {
				// SemanticWorktreeName returns just the suffix (3 chars from
				// the alphabet) when the sanitized title is empty.
				if len(got) != 3 {
					t.Fatalf("expected 3-char suffix, got %q (len %d)", got, len(got))
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, resolveFreshBranchName(tc.raw, tc.taskTitle))
		})
	}
}

func TestAssociatePRFromRepoInputs(t *testing.T) {
	log := newTestLogger(t)

	t.Run("calls callback when repo input contains PR URL", func(t *testing.T) {
		var mu sync.Mutex
		var called bool
		var gotTaskID, gotSessionID, gotPRURL, gotBranch string

		h := &TaskHandlers{logger: log}
		h.SetOnTaskCreatedWithPR(func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			mu.Lock()
			defer mu.Unlock()
			called = true
			gotTaskID = taskID
			gotSessionID = sessionID
			gotPRURL = prURL
			gotBranch = branch
		})

		// The callback runs in a goroutine, so we need a channel to sync
		done := make(chan struct{})
		h.onTaskCreatedWithPR = func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			defer close(done)
			mu.Lock()
			defer mu.Unlock()
			called = true
			gotTaskID = taskID
			gotSessionID = sessionID
			gotPRURL = prURL
			gotBranch = branch
		}

		h.associatePRFromRepoInputs("task-1", "session-1", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/123",
				CheckoutBranch: "feature-branch",
			},
		})

		<-done

		mu.Lock()
		defer mu.Unlock()
		require.True(t, called)
		assert.Equal(t, "task-1", gotTaskID)
		assert.Equal(t, "session-1", gotSessionID)
		assert.Equal(t, "https://github.com/owner/repo/pull/123", gotPRURL)
		assert.Equal(t, "feature-branch", gotBranch)
	})

	t.Run("does not call callback for plain repo URLs", func(t *testing.T) {
		called := false
		h := &TaskHandlers{logger: log}
		h.SetOnTaskCreatedWithPR(func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			called = true
		})

		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo",
				CheckoutBranch: "main",
			},
		})

		assert.False(t, called)
	})

	t.Run("does not call callback when no onTaskCreatedWithPR set", func(t *testing.T) {
		h := &TaskHandlers{logger: log}
		// Should not panic
		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/123",
				CheckoutBranch: "feature-branch",
			},
		})
	})

	t.Run("passes empty session ID when no session created", func(t *testing.T) {
		done := make(chan struct{})
		var gotSessionID string

		h := &TaskHandlers{logger: log}
		h.onTaskCreatedWithPR = func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			defer close(done)
			gotSessionID = sessionID
		}

		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/456",
				CheckoutBranch: "fix-branch",
			},
		})

		<-done
		assert.Equal(t, "", gotSessionID)
	})

	t.Run("only processes first PR URL when multiple repos have PRs", func(t *testing.T) {
		var count int
		var mu sync.Mutex
		done := make(chan struct{})

		h := &TaskHandlers{logger: log}
		h.onTaskCreatedWithPR = func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			defer close(done)
			mu.Lock()
			defer mu.Unlock()
			count++
		}

		h.associatePRFromRepoInputs("task-1", "", []httpTaskRepositoryInput{
			{
				GitHubURL:      "https://github.com/owner/repo/pull/1",
				CheckoutBranch: "branch-1",
			},
			{
				GitHubURL:      "https://github.com/owner/repo/pull/2",
				CheckoutBranch: "branch-2",
			},
		})

		<-done
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 1, count)
	})
}
