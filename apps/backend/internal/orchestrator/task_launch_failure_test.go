package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	if len(mc.sessionMessages) != 0 {
		t.Fatalf("launch callback created guidance before the FAILED transition: %d messages", len(mc.sessionMessages))
	}
	if err := repo.UpdateTaskSessionState(ctx, "session1", models.TaskSessionStateFailed, err.Error()); err != nil {
		t.Fatalf("mark session failed: %v", err)
	}
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)
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
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

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

func TestHandleSessionLaunchFailed_SecondaryOpenPRDoesNotUsePrimaryTerminalPR(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", RepositoryID: "repo-primary", HeadBranch: "feature/foo", State: "closed"},
		{TaskID: "task1", RepositoryID: "repo-secondary", HeadBranch: "feature/foo", State: "open"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-secondary", err)

	if len(mc.sessionMessages) != 0 {
		t.Fatalf("expected no missing-branch guidance for the secondary repository's open PR, got %d messages", len(mc.sessionMessages))
	}
}

func TestHandleSessionLaunchFailed_BoundsBlockingPRStateLookup(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	var lookupHadDeadline bool
	svc.SetGitHubService(&mockGitHubService{taskPRListFunc: func(ctx context.Context, _ []string) (map[string][]*github.TaskPR, error) {
		_, lookupHadDeadline = ctx.Deadline()
		if !lookupHadDeadline {
			return nil, errors.New("missing lookup deadline")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}})

	started := time.Now()
	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(context.Background(), "task1", "session1", "repo-a", err)

	if !lookupHadDeadline {
		t.Fatal("expected task PR lookup to have a deadline")
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("blocking task PR lookup took %s, want no more than 1.5s", elapsed)
	}
	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected neutral guidance after task PR lookup timeout, got %d messages", len(mc.sessionMessages))
	}
	if _, ok := mc.sessionMessages[0].metadata["actions"]; ok {
		t.Fatalf("expected no destructive actions after task PR lookup timeout, got %#v", mc.sessionMessages[0].metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_ConflictingSameBranchPRStatesCreateNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

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
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

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
			seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

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
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

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
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

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

func TestHandleSessionLaunchFailed_SingleRepositoryLegacyPRCreatesMissingBranchGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-a", WorkspaceID: "ws1", Name: "repo-a", SourceType: "local",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-a", TaskID: "task1", RepositoryID: "repo-a", Position: 0,
	}); err != nil {
		t.Fatalf("link task repository: %v", err)
	}

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", HeadBranch: "feature/foo", State: "closed"},
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
	if actions, ok := msg.metadata["actions"].([]map[string]interface{}); !ok || len(actions) != 2 {
		t.Fatalf("expected archive/delete actions, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_LegacyPRForDifferentRepositoryCreatesNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-a", WorkspaceID: "ws1", Name: "repo-a", SourceType: "local",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-a", TaskID: "task1", RepositoryID: "repo-a", Position: 0,
	}); err != nil {
		t.Fatalf("link task repository: %v", err)
	}

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", HeadBranch: "feature/foo", State: "closed"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-b", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 neutral fetch-failure guidance message, got %d", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if kind := msg.metadata["failure_kind"]; kind != "branch_fetch_failed" {
		t.Fatalf("expected neutral failure kind, got %#v", kind)
	}
	if _, ok := msg.metadata["actions"]; ok {
		t.Fatalf("expected no destructive actions for a legacy PR linked to another repository, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_MultipleRepositoriesLegacyPRCreatesNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	for position, repositoryID := range []string{"repo-a", "repo-b"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: repositoryID, WorkspaceID: "ws1", Name: repositoryID, SourceType: "local",
		}); err != nil {
			t.Fatalf("create repository %q: %v", repositoryID, err)
		}
		if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "task-repo-" + repositoryID, TaskID: "task1", RepositoryID: repositoryID, Position: position,
		}); err != nil {
			t.Fatalf("link task repository %q: %v", repositoryID, err)
		}
	}

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	svc.SetGitHubService(&mockGitHubService{taskPRs: []*github.TaskPR{
		{TaskID: "task1", HeadBranch: "feature/foo", State: "closed"},
	}})

	err := errors.New("environment preparation failed: branch \"feature/foo\" not found locally or on remote")
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", err)

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 neutral fetch-failure guidance message, got %d", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if kind := msg.metadata["failure_kind"]; kind != "branch_fetch_failed" {
		t.Fatalf("expected neutral failure kind, got %#v", kind)
	}
	if _, ok := msg.metadata["actions"]; ok {
		t.Fatalf("expected no destructive actions for an unscoped multi-repository PR, got %#v", msg.metadata["actions"])
	}
}

func TestHandleSessionLaunchFailed_EmptyRepositoryIDCreatesNeutralGuidance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

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

func TestHandleSessionLaunchFailed_RetriesAfterGuidanceMessageWriteFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	writeErr := errors.New("message store unavailable")
	mc := &mockMessageCreator{sessionMessageErr: writeErr}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc
	err := errors.New("environment preparation failed: fatal: couldn't find remote ref feature/foo")

	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "", err)
	if len(mc.sessionMessages) != 0 {
		t.Fatalf("failed message write persisted %d messages", len(mc.sessionMessages))
	}
	if _, suppressed := svc.suppressToast.Load("session1"); suppressed {
		t.Fatal("failed message write suppressed the fallback toast")
	}
	failed, getErr := repo.GetTaskSession(ctx, "session1")
	if getErr != nil {
		t.Fatalf("get failed session: %v", getErr)
	}
	if _, claimed := failed.Metadata[missingPRBranchRecoveryClaimKey]; claimed {
		t.Fatal("failed message write left the recovery claim persisted")
	}

	mc.sessionMessageErr = nil
	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "", err)
	if len(mc.sessionMessages) != 1 {
		t.Fatalf("retry created %d messages, want 1", len(mc.sessionMessages))
	}
	if _, suppressed := svc.suppressToast.Load("session1"); !suppressed {
		t.Fatal("successful recovery message did not suppress the fallback toast")
	}
}

func TestHandleSessionLaunchFailed_IgnoresUnrelatedErrors(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "repo-a", errors.New("failed to launch container"))

	if len(mc.sessionMessages) != 0 {
		t.Fatalf("expected no session messages for unrelated error, got %d", len(mc.sessionMessages))
	}
}

func TestHandleSessionLaunchFailed_IgnoresMissingBranchWrapperAroundTransportError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	err := errors.New("environment preparation failed: branch \"codex/enhance-prompt-result-delivery\" not found locally or on remote: fatal: unable to access 'https://github.com/kdlbs/kandev.git/': Could not resolve host: github.com")
	if isMissingBranchError(err) {
		t.Fatal("expected transport failure not to be classified as a missing PR branch")
	}

	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "", err)

	if len(mc.sessionMessages) != 0 {
		t.Fatalf("expected no guidance message for transport failure, got %d", len(mc.sessionMessages))
	}
}

func TestHandleSessionLaunchFailed_IgnoresPathspecAfterFetchFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	err := errors.New("environment preparation failed: checkout branch: git command failed: fetch branch failed: exit status 128: fatal: unable to access 'https://github.com/kdlbs/kandev.git/': Could not resolve host: github.com\ncheckout branch failed: error: pathspec 'codex/enhance-prompt-result-delivery' did not match any file(s) known to git")
	if isMissingBranchError(err) {
		t.Fatal("expected pathspec after fetch failure not to be classified as a missing PR branch")
	}

	svc.handleSessionLaunchFailed(ctx, "task1", "session1", "", err)

	if len(mc.sessionMessages) != 0 {
		t.Fatalf("expected no guidance message after fetch failure, got %d", len(mc.sessionMessages))
	}
}
