package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestCheckSessionMRRedirectsToWorkspaceGroupOwner covers Review Round 3's F-B
// finding: CheckSessionMR is GitLab's on-demand counterpart to CheckSessionPR
// (redirected in an earlier round), but was never itself redirected. Before
// this fix it always wrote the MR watch and association under the observing
// subtask's own taskID, reproducing the "one PR/MR gets bound to several
// tasks" defect on demand for GitLab in exactly the same way CheckSessionPR
// once did for GitHub.
func TestCheckSessionMRRedirectsToWorkspaceGroupOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	// seedSession creates the shared ws1/wf1 fixture plus the subtask's own
	// task+session, checked out at repo1/feat/a, exactly like a real
	// inherit_parent/shared_group subtask sharing its parent's worktree.
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "parent1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Parent", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	seedWorkspaceGroup(t, repo, "group1", "parent1", "parent1", "t1")

	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myproj",
		SourceType: "provider", Provider: gitlabProviderName,
		ProviderOwner: "group", ProviderName: "myproj",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1", CheckoutBranch: "feat/a",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	// The owner shares the same physical checkout as its members (the whole
	// premise of inherit_parent/shared_group), so it holds repo1 too — this is
	// what makes the redirect eligible under resolveEffectivePushTaskID's
	// ownership guard, not just group membership.
	seedGroupOwnerTaskRepository(t, repo, "parent1", "repo1", "feat/a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	fake.autoLinkFunc = func(_ context.Context, _, _, taskID, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		tm := &gitlab.TaskMR{TaskID: taskID, RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 3, HeadBranch: branch}
		fake.mu.Lock()
		fake.taskMRs[taskID] = append(fake.taskMRs[taskID], tm)
		fake.mu.Unlock()
		return tm, nil
	}
	svc.SetGitLabMRLinkService(fake)

	found, err := svc.CheckSessionMR(ctx, "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatalf("CheckSessionMR: expected a merge request to be found")
	}

	if fake.lastEnsureWatchTaskID != "parent1" {
		t.Fatalf("EnsureMRWatch taskID = %q, want parent1 (the group owner)", fake.lastEnsureWatchTaskID)
	}
	if fake.lastAutoLinkTaskID != "parent1" {
		t.Fatalf("AutoLinkMRForBranch taskID = %q, want parent1 (the group owner)", fake.lastAutoLinkTaskID)
	}
	if len(fake.taskMRs["t1"]) != 0 {
		t.Fatalf("subtask t1 got its own association, want zero: %+v", fake.taskMRs["t1"])
	}
}

// TestCheckSessionMR_NoGroupKeepsOwnAttribution is the negative control for
// TestCheckSessionMRRedirectsToWorkspaceGroupOwner: an ordinary, ungrouped
// task must keep writing its watch/association under its own taskID — the
// redirect must only fire for actual workspace-group members.
func TestCheckSessionMR_NoGroupKeepsOwnAttribution(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.autoLinkFunc = func(_ context.Context, _, _, taskID, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		return &gitlab.TaskMR{TaskID: taskID, RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 4, HeadBranch: branch}, nil
	}

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatalf("CheckSessionMR: expected a merge request to be found")
	}

	if fake.lastEnsureWatchTaskID != "t1" {
		t.Fatalf("EnsureMRWatch taskID = %q, want t1 (no group, no redirect)", fake.lastEnsureWatchTaskID)
	}
	if fake.lastAutoLinkTaskID != "t1" {
		t.Fatalf("AutoLinkMRForBranch taskID = %q, want t1 (no group, no redirect)", fake.lastAutoLinkTaskID)
	}
}
