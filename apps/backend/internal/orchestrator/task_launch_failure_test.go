package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestIsMissingMergedPRBranchError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "remote ref missing",
			err:  errors.New("environment preparation failed: fatal: couldn't find remote ref feature/foo"),
			want: true,
		},
		{
			name: "branch not found locally or remote",
			err:  errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote"),
			want: true,
		},
		{
			name: "pathspec did not match (local executor)",
			err:  errors.New("environment preparation failed: checkout branch: git command failed: error: pathspec 'feature/foo' did not match any file(s) known to git"),
			want: true,
		},
		{
			name: "unrelated launch error",
			err:  errors.New("failed to launch container"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMissingMergedPRBranchError(tt.err); got != tt.want {
				t.Fatalf("isMissingMergedPRBranchError()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractMissingBranchName(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "quoted branch form",
			err:  errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote"),
			want: "feature/foo",
		},
		{
			name: "remote ref form",
			err:  errors.New("fatal: couldn't find remote ref hotfix/bar"),
			want: "hotfix/bar",
		},
		{
			name: "pathspec form (local executor)",
			err:  errors.New("checkout branch: git command failed: error: pathspec 'feature/baz' did not match any file(s) known to git"),
			want: "feature/baz",
		},
		{
			name: "no branch available",
			err:  errors.New("failed to launch container"),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractMissingBranchName(tt.err); got != tt.want {
				t.Fatalf("extractMissingBranchName()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleSessionLaunchFailed_CreatesGuidanceMessageForMissingPRBranch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote: fatal: couldn't find remote ref feature/foo")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 session message, got %d", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if msg.taskID != "task1" || msg.sessionID != "session1" {
		t.Fatalf("unexpected message target: task=%s session=%s", msg.taskID, msg.sessionID)
	}
	if msg.messageType != string(v1.MessageTypeStatus) {
		t.Fatalf("expected status message type, got %q", msg.messageType)
	}
	if msg.turnID != "" {
		t.Fatalf("expected empty turn ID for pre-start failure, got %q", msg.turnID)
	}
	if !strings.Contains(msg.content, "feature/foo") || !strings.Contains(strings.ToLower(msg.content), "merged") {
		t.Fatalf("expected content with branch name and merged hint, got %q", msg.content)
	}
	if kind, ok := msg.metadata["failure_kind"].(string); !ok || kind != "missing_pr_branch" {
		t.Fatalf("expected failure_kind metadata, got %#v", msg.metadata["failure_kind"])
	}
	if branch, ok := msg.metadata["missing_branch"].(string); !ok || branch != "feature/foo" {
		t.Fatalf("expected missing_branch metadata, got %#v", msg.metadata["missing_branch"])
	}
	// Verify actions array is present with archive and delete actions
	actions, ok := msg.metadata["actions"].([]map[string]interface{})
	if !ok || len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %#v", msg.metadata["actions"])
	}
	if actions[0]["type"] != "archive_task" {
		t.Fatalf("expected first action type archive_task, got %v", actions[0]["type"])
	}
	if actions[1]["type"] != "delete_task" {
		t.Fatalf("expected second action type delete_task, got %v", actions[1]["type"])
	}
}

func TestHandleSessionLaunchFailed_IgnoresUnrelatedErrors(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	svc.handleSessionLaunchFailed(ctx, "task1", "session1", errors.New("failed to launch container"))

	if len(mc.sessionMessages) != 0 {
		t.Fatalf("expected no session messages for unrelated error, got %d", len(mc.sessionMessages))
	}
}
