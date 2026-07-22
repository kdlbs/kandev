package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestIsMissingBranchError(t *testing.T) {
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
			if got := isMissingBranchError(tt.err); got != tt.want {
				t.Fatalf("isMissingBranchError()=%v, want %v", got, tt.want)
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

func TestHandleSessionLaunchFailed_UnavailablePRStateCreatesNeutralFetchGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRErr: errors.New("persisted PR state unavailable")})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote: fatal: couldn't find remote ref feature/foo")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

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
	lowerContent := strings.ToLower(msg.content)
	if !strings.Contains(msg.content, "feature/foo") || !strings.Contains(lowerContent, "retry") || strings.Contains(lowerContent, "merged") {
		t.Fatalf("expected actionable content with branch name and no unverified merge claim, got %q", msg.content)
	}
	if kind, ok := msg.metadata["failure_kind"].(string); !ok || kind != "branch_fetch_failed" {
		t.Fatalf("expected failure_kind metadata, got %#v", msg.metadata["failure_kind"])
	}
	if branch, ok := msg.metadata["missing_branch"].(string); !ok || branch != "feature/foo" {
		t.Fatalf("expected missing_branch metadata, got %#v", msg.metadata["missing_branch"])
	}
	if _, ok := msg.metadata["actions"]; ok {
		t.Fatalf("expected neutral guidance without archive/delete actions, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_OpenMatchingPRDoesNotCreateMissingBranchGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: "open"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

	if len(mc.sessionMessages) != 0 {
		t.Fatalf("expected no persistent missing-branch guidance for an open PR, got %d messages", len(mc.sessionMessages))
	}
	if _, ok := svc.suppressToast.Load("session1"); ok {
		t.Fatal("expected the ordinary launch error path to remain available")
	}
}

func TestHandleSessionLaunchFailed_ConflictingSameBranchPRStatesCreateNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: "open"},
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: "closed"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected neutral guidance for ambiguous PR state, got %d messages", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if !strings.Contains(strings.ToLower(msg.content), "retry") || strings.Contains(msg.content, "no longer exists") {
		t.Fatalf("expected neutral fetch-failure guidance, got %q", msg.content)
	}
	if kind := msg.metadata["failure_kind"]; kind != "branch_fetch_failed" {
		t.Fatalf("expected neutral failure kind, got %#v", kind)
	}
	if _, ok := msg.metadata["actions"]; ok {
		t.Fatalf("expected no destructive actions for ambiguous PR state, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_MultipleTerminalMatchingPRsCreateNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: "closed"},
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: "merged"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected neutral guidance for ambiguous terminal PRs, got %d messages", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if !strings.Contains(strings.ToLower(msg.content), "retry") || strings.Contains(msg.content, "no longer exists") {
		t.Fatalf("expected neutral fetch-failure guidance, got %q", msg.content)
	}
	if kind := msg.metadata["failure_kind"]; kind != "branch_fetch_failed" {
		t.Fatalf("expected neutral failure kind, got %#v", kind)
	}
	if _, ok := msg.metadata["actions"]; ok {
		t.Fatalf("expected no destructive actions for ambiguous terminal PRs, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_ClosedOrMergedMatchingPRCreatesMissingBranchGuidance(t *testing.T) {
	for _, prState := range []string{"closed", "merged"} {
		t.Run(prState, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

			mc := &mockMessageCreator{}
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			svc.messageCreator = mc
			svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
				{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: prState},
			}})

			err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
			svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

			if len(mc.sessionMessages) != 1 {
				t.Fatalf("expected 1 missing-branch guidance message, got %d", len(mc.sessionMessages))
			}
			msg := mc.sessionMessages[0]
			if !strings.Contains(msg.content, "feature/foo") || !strings.Contains(msg.content, "no longer exists") {
				t.Fatalf("expected authoritative missing-branch guidance, got %q", msg.content)
			}
			actions, ok := msg.metadata["actions"].([]map[string]interface{})
			if !ok || len(actions) != 2 {
				t.Fatalf("expected archive/delete actions, got %#v", msg.metadata["actions"])
			}
			if suppressed, ok := svc.suppressToast.Load("session1"); !ok || suppressed != true {
				t.Fatalf("expected duplicate launch toast to be suppressed, got ok=%v value=%v", ok, suppressed)
			}
		})
	}
}

func TestHandleSessionLaunchFailed_UnrelatedOpenPRDoesNotSuppressNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/other", State: "open"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected neutral guidance for the unmatched branch, got %d messages", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if !strings.Contains(strings.ToLower(msg.content), "retry") || strings.Contains(msg.content, "no longer exists") {
		t.Fatalf("expected neutral fetch-failure guidance, got %q", msg.content)
	}
	if kind := msg.metadata["failure_kind"]; kind != "branch_fetch_failed" {
		t.Fatalf("expected neutral failure kind, got %#v", kind)
	}
}

func TestHandleSessionLaunchFailed_ClosedPRInAnotherRepositoryCreatesNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: "closed"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-b", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected neutral guidance when repo-b launch matches only repo-a's closed PR, got %d messages", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if !strings.Contains(strings.ToLower(msg.content), "retry") || strings.Contains(msg.content, "no longer exists") {
		t.Fatalf("expected neutral fetch-failure guidance, got %q", msg.content)
	}
	if kind := msg.metadata["failure_kind"]; kind != "branch_fetch_failed" {
		t.Fatalf("expected neutral failure kind, got %#v", kind)
	}
	if _, ok := msg.metadata["actions"]; ok {
		t.Fatalf("expected no destructive actions without a repository-scoped PR match, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_EmptyRepositoryIDCreatesNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", RepositoryID: "repo-a", HeadBranch: "feature/foo", State: "closed"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected neutral guidance without a failed repository ID, got %d messages", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if !strings.Contains(strings.ToLower(msg.content), "retry") || strings.Contains(msg.content, "no longer exists") {
		t.Fatalf("expected neutral fetch-failure guidance, got %q", msg.content)
	}
	if kind := msg.metadata["failure_kind"]; kind != "branch_fetch_failed" {
		t.Fatalf("expected neutral failure kind, got %#v", kind)
	}
	if _, ok := msg.metadata["actions"]; ok {
		t.Fatalf("expected no destructive actions without a repository-scoped PR match, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_IgnoresUnrelatedErrors(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", errors.New("failed to launch container"))

	if len(mc.sessionMessages) != 0 {
		t.Fatalf("expected no session messages for unrelated error, got %d", len(mc.sessionMessages))
	}
}
