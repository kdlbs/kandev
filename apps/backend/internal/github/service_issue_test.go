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

func (s *stubSessionChecker) HasUserAuthoredMessage(_ context.Context, _ string) (bool, error) {
	return s.has, s.err
}

func newCleanupTestService(client Client, log *logger.Logger, checker TaskSessionChecker) *Service {
	return &Service{
		client:               client,
		logger:               log,
		taskSessionChecker:   checker,
		cleanupFailureCounts: make(map[string]int),
	}
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
		svc := newCleanupTestService(&issueStateClient{state: "open"}, log, nil)
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task, CleanupPolicyAuto)
		if del {
			t.Error("expected false for open issue")
		}
	})

	t.Run("closed issue with no user messages returns true", func(t *testing.T) {
		svc := newCleanupTestService(&issueStateClient{state: "closed"}, log, &stubSessionChecker{has: false})
		del, reason := svc.shouldDeleteIssueTask(context.Background(), task, CleanupPolicyAuto)
		if !del {
			t.Error("expected true for closed issue without user messages")
		}
		if reason != "issue_closed" {
			t.Errorf("reason = %q, want %q", reason, "issue_closed")
		}
	})

	t.Run("closed issue with user message returns false under auto", func(t *testing.T) {
		svc := newCleanupTestService(&issueStateClient{state: "closed"}, log, &stubSessionChecker{has: true})
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task, CleanupPolicyAuto)
		if del {
			t.Error("expected false for closed issue when user engaged")
		}
	})

	t.Run("closed issue under always deletes even with user message", func(t *testing.T) {
		svc := newCleanupTestService(&issueStateClient{state: "closed"}, log, &stubSessionChecker{has: true})
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task, CleanupPolicyAlways)
		if !del {
			t.Error("expected true for closed issue under CleanupPolicyAlways")
		}
	})

	t.Run("closed issue under never keeps task", func(t *testing.T) {
		svc := newCleanupTestService(&issueStateClient{state: "closed"}, log, &stubSessionChecker{has: false})
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task, CleanupPolicyNever)
		if del {
			t.Error("expected false under CleanupPolicyNever")
		}
	})

	t.Run("API error returns false", func(t *testing.T) {
		svc := newCleanupTestService(&issueStateClient{err: fmt.Errorf("api error")}, log, nil)
		del, _ := svc.shouldDeleteIssueTask(context.Background(), task, CleanupPolicyAuto)
		if del {
			t.Error("expected false when API returns error")
		}
	})
}

func TestBuildIssueFilter_IncludesStateOpen(t *testing.T) {
	svc := &Service{}
	tests := []struct {
		name  string
		watch *IssueWatch
		want  string
	}{
		{
			name:  "no labels",
			watch: &IssueWatch{},
			want:  "state:open",
		},
		{
			name:  "single label",
			watch: &IssueWatch{Labels: []string{"bug"}},
			want:  "state:open label:bug",
		},
		{
			name:  "label with space is quoted",
			watch: &IssueWatch{Labels: []string{"good first issue"}},
			want:  `state:open label:"good first issue"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.buildIssueFilter(tt.watch); got != tt.want {
				t.Errorf("buildIssueFilter() = %q, want %q", got, tt.want)
			}
		})
	}
}
