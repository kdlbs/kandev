package github

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// --- shouldDeleteIssueTask tests ---

// issueStateClient is a minimal Client stub for shouldDeleteIssueTask tests.
type issueStateClient struct {
	NoopClient
	state string
	err   error
}

func (c *issueStateClient) GetIssueState(_ context.Context, _, _ string, _ int) (string, error) {
	return c.state, c.err
}

// stubSessionChecker implements TaskSessionChecker for tests.
type stubSessionChecker struct {
	has bool
	err error
}

func (s *stubSessionChecker) HasTaskSessions(_ context.Context, _ string) (bool, error) {
	return s.has, s.err
}

func TestShouldDeleteIssueTask(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})

	task := &IssueWatchTask{
		ID:           "iwt-1",
		IssueWatchID: "iw-1",
		RepoOwner:    "acme",
		RepoName:     "widget",
		IssueNumber:  7,
		TaskID:       "task-1",
	}

	t.Run("open issue returns false", func(t *testing.T) {
		svc := &Service{client: &issueStateClient{state: "open"}, logger: log}
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task)
		if del {
			t.Error("expected false for open issue")
		}
	})

	t.Run("closed issue without sessions returns true", func(t *testing.T) {
		svc := &Service{
			client:             &issueStateClient{state: "closed"},
			logger:             log,
			taskSessionChecker: &stubSessionChecker{has: false},
		}
		del, reason := svc.shouldDeleteIssueTask(context.Background(), task)
		if !del {
			t.Error("expected true for closed issue without sessions")
		}
		if reason != "issue_closed" {
			t.Errorf("reason = %q, want %q", reason, "issue_closed")
		}
	})

	t.Run("closed issue with sessions returns false", func(t *testing.T) {
		svc := &Service{
			client:             &issueStateClient{state: "closed"},
			logger:             log,
			taskSessionChecker: &stubSessionChecker{has: true},
		}
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task)
		if del {
			t.Error("expected false for closed issue with active sessions")
		}
	})

	t.Run("API error returns false", func(t *testing.T) {
		svc := &Service{
			client: &issueStateClient{err: fmt.Errorf("api error")},
			logger: log,
		}
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task)
		if del {
			t.Error("expected false when API returns error")
		}
	})
}
