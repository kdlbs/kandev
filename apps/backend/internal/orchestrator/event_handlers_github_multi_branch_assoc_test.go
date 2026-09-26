package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedGitHubPushAssocFixture creates the repository, task_repository
// (checked out at checkoutBranch), and session wiring shared by
// TestGitHubMultiBranchAssociationsCoexist and
// TestGitHubPushAssociationPassesRepositoryID.
func seedGitHubPushAssocFixture(t *testing.T, repo *sqliterepo.Repository, checkoutBranch string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "kandev",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "kandev",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1", CheckoutBranch: checkoutBranch,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
}

// TestGitHubMultiBranchAssociationsCoexist covers the gap flagged in
// event_handlers_github_multi_branch_test.go: TestIsMultiBranchSubdir only
// pins the pure string-matching helper, not the end-to-end behavior it
// exists to support. Two branches of ONE repository in ONE session (the
// `<repo>-<branch-slug>` sibling-worktree naming) must each get their own
// watch and association — the second branch's push must not be dropped, and
// must not clobber the first branch's already-created rows.
func TestGitHubMultiBranchAssociationsCoexist(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedGitHubPushAssocFixture(t, repo, "main")

	mockClient := github.NewMockClient()
	mockClient.AddPR(&github.PR{
		Number: 1, Title: "Primary", HTMLURL: "https://github.com/myorg/kandev/pull/1",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "main", BaseBranch: "main",
	})
	mockClient.AddPR(&github.PR{
		Number: 2, Title: "Secondary", HTMLURL: "https://github.com/myorg/kandev/pull/2",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "feature-x", BaseBranch: "main",
	})
	ghSvc := &mockGitHubService{client: mockClient}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	// Primary branch push: repositoryName empty (legacy single-repo tag).
	svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "main")
	// Secondary branch push: repositoryName tagged with the multi-branch
	// sibling-worktree subdir naming ("<repo.Name>-<branch-slug>").
	svc.detectPushAndAssociatePR(ctx, "s1", "t1", "kandev-feature-x", "feature-x")

	if ghSvc.createWatchCalls != 2 {
		t.Fatalf("expected 2 CreatePRWatch calls (one per branch), got %d", ghSvc.createWatchCalls)
	}
	if ghSvc.associateCalls != 2 {
		t.Fatalf("expected 2 AssociatePRWithTask calls (one per branch), got %d", ghSvc.associateCalls)
	}
	if len(ghSvc.createWatchLog) != 2 || len(ghSvc.associateLog) != 2 {
		t.Fatalf("call logs incomplete: createWatch=%+v associate=%+v", ghSvc.createWatchLog, ghSvc.associateLog)
	}

	branches := map[string]bool{}
	for _, c := range ghSvc.createWatchLog {
		branches[c.Branch] = true
		// AC17: the multi-repo/multi-branch path must pass a non-empty
		// repositoryID on every call — an empty value here means the push
		// silently fell back to whichever watch already existed instead of
		// creating its own, the exact regression the event_handlers_github.go
		// comment near associatePRFromPushScoped warns about.
		if c.RepositoryID == "" {
			t.Fatalf("CreatePRWatch call for branch %q had empty repositoryID", c.Branch)
		}
	}
	if !branches["main"] || !branches["feature-x"] {
		t.Fatalf("expected watches for both main and feature-x, got %+v", ghSvc.createWatchLog)
	}

	for _, c := range ghSvc.associateLog {
		if c.RepositoryID == "" {
			t.Fatalf("AssociatePRWithTask call for branch %q had empty repositoryID", c.Branch)
		}
	}
}

// TestGitHubPushAssociationPassesRepositoryID pins the AC17 invariant on the
// single-repo path too: even when there is only one repository, push
// detection must resolve and pass its repositoryID rather than an empty
// string, since AssociatePRWithTaskForWorkspace and CreatePRWatchForWorkspace
// use it to scope the row.
func TestGitHubPushAssociationPassesRepositoryID(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedGitHubPushAssocFixture(t, repo, "main")

	mockClient := github.NewMockClient()
	mockClient.AddPR(&github.PR{
		Number: 1, Title: "PR", HTMLURL: "https://github.com/myorg/kandev/pull/1",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "main", BaseBranch: "main",
	})
	ghSvc := &mockGitHubService{client: mockClient}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "main")

	if ghSvc.lastCreateWatchRepositoryID != "repo1" {
		t.Fatalf("CreatePRWatch repositoryID = %q, want repo1", ghSvc.lastCreateWatchRepositoryID)
	}
	if ghSvc.lastAssociateRepositoryID != "repo1" {
		t.Fatalf("AssociatePRWithTask repositoryID = %q, want repo1", ghSvc.lastAssociateRepositoryID)
	}
}

// seedWorkspaceGroup creates the task_workspace_groups /
// task_workspace_group_members rows the workspace-group redirect reads. Those
// tables are created by internal/office/repository/sqlite's schema init, not
// this package's own — office's init never runs against an
// orchestrator-package test DB, so the fixture creates them directly, the
// same way internal/task/repository/sqlite/task_environment_test.go does for
// its own package's group-binding tests. Only the columns the redirect query
// actually reads (or that a foreign key requires) are populated.
func seedWorkspaceGroup(t *testing.T, repo *sqliterepo.Repository, groupID, ownerTaskID string, memberTaskIDs ...string) {
	t.Helper()
	if _, err := repo.DB().Exec(`
		CREATE TABLE IF NOT EXISTS task_workspace_groups (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			owner_task_id TEXT NOT NULL,
			materialized_kind TEXT NOT NULL DEFAULT '',
			materialized_environment_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS task_workspace_group_members (
			workspace_group_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			released_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (workspace_group_id, task_id)
		);
	`); err != nil {
		t.Fatalf("create workspace group tables: %v", err)
	}
	if _, err := repo.DB().Exec(
		`INSERT INTO task_workspace_groups (id, owner_task_id) VALUES (?, ?)`, groupID, ownerTaskID,
	); err != nil {
		t.Fatalf("seed workspace group: %v", err)
	}
	for _, taskID := range memberTaskIDs {
		if _, err := repo.DB().Exec(
			`INSERT INTO task_workspace_group_members (workspace_group_id, task_id) VALUES (?, ?)`,
			groupID, taskID,
		); err != nil {
			t.Fatalf("seed workspace group member %s: %v", taskID, err)
		}
	}
}

// TestGitHubPushAssociationRedirectsToWorkspaceGroupOwner covers the "one PR
// gets bound to several tasks" defect: a subtask sharing its parent's
// worktree via inherit_parent/shared_group workspace modes observes the same
// git-status push as the parent, since they share one physical checkout. Push
// detection must file the resulting watch/association under the group's
// owner task, not the subtask that happened to be running when the push
// landed — otherwise one PR ends up bound to every task in the group.
func TestGitHubPushAssociationRedirectsToWorkspaceGroupOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	// seedSession creates the shared ws1/wf1 fixture plus the subtask's own
	// task+session — its checkout, task_repository row, and
	// session.RepositoryID all key off t1, exactly like a real
	// inherit_parent/shared_group subtask that actually observed the push.
	seedSession(t, repo, "t1", "s1", "step1")

	// The owner task lives in the same workspace/workflow; created directly
	// (not via seedSession/seedTaskWithoutSession, which would try to
	// recreate the ws1/wf1 rows seedSession already inserted above).
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "parent1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Parent", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	seedWorkspaceGroup(t, repo, "group1", "parent1", "parent1", "t1")
	seedGitHubPushAssocFixture(t, repo, "feature-x")
	// The owner shares the same physical checkout as its members (that's the
	// whole premise of inherit_parent/shared_group), so it holds repo1 too —
	// this is what makes the redirect below eligible under the F2 ownership
	// guard, not just the group membership.
	seedGroupOwnerTaskRepository(t, repo, "parent1", "repo1", "feature-x")

	mockClient := github.NewMockClient()
	mockClient.AddPR(&github.PR{
		Number: 7, Title: "Shared worktree PR", HTMLURL: "https://github.com/myorg/kandev/pull/7",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "feature-x", BaseBranch: "main",
	})
	ghSvc := &mockGitHubService{client: mockClient}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	svc.dispatchPushDetection(ctx, "s1", "t1", "", "feature-x")

	if ghSvc.lastCreateWatchTaskID != "parent1" {
		t.Fatalf("CreatePRWatch taskID = %q, want parent1 (the group owner)", ghSvc.lastCreateWatchTaskID)
	}
	if ghSvc.lastAssociateTaskID != "parent1" {
		t.Fatalf("AssociatePRWithTask taskID = %q, want parent1 (the group owner)", ghSvc.lastAssociateTaskID)
	}
	for _, c := range ghSvc.associateLog {
		if c.TaskID == "t1" {
			t.Fatalf("subtask t1 got its own association, want zero: %+v", ghSvc.associateLog)
		}
	}

	// Stable across a second push-detection run: rerunning must not flip the
	// attribution back to the observing subtask.
	svc.dispatchPushDetection(ctx, "s1", "t1", "", "feature-x")
	if ghSvc.lastCreateWatchTaskID != "parent1" {
		t.Fatalf("second run: CreatePRWatch taskID = %q, want parent1", ghSvc.lastCreateWatchTaskID)
	}
	if ghSvc.lastAssociateTaskID != "parent1" {
		t.Fatalf("second run: AssociatePRWithTask taskID = %q, want parent1", ghSvc.lastAssociateTaskID)
	}
	for _, c := range ghSvc.associateLog {
		if c.TaskID == "t1" {
			t.Fatalf("subtask t1 got its own association after second run, want zero: %+v", ghSvc.associateLog)
		}
	}

	// Negative control: a task with no workspace group keeps its own
	// attribution — the redirect must not fire for ordinary, ungrouped tasks.
	// Created directly (not via seedSession, which would try to recreate the
	// shared ws1/wf1 rows already inserted above).
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "solo1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Solo", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create solo task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s-solo", TaskID: "solo1", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create solo session: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-solo", WorkspaceID: "ws1", Name: "kandev",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "kandev",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create solo repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr-solo", TaskID: "solo1", RepositoryID: "repo-solo", CheckoutBranch: "solo-branch",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create solo task repository: %v", err)
	}
	soloSession, _ := repo.GetTaskSession(ctx, "s-solo")
	soloSession.RepositoryID = "repo-solo"
	if err := repo.UpdateTaskSession(ctx, soloSession); err != nil {
		t.Fatalf("update solo session: %v", err)
	}
	mockClient.AddPR(&github.PR{
		Number: 8, Title: "Solo PR", HTMLURL: "https://github.com/myorg/kandev/pull/8",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "solo-branch", BaseBranch: "main",
	})

	svc.dispatchPushDetection(ctx, "s-solo", "solo1", "", "solo-branch")

	if ghSvc.lastCreateWatchTaskID != "solo1" {
		t.Fatalf("CreatePRWatch taskID = %q, want solo1 (no group, no redirect)", ghSvc.lastCreateWatchTaskID)
	}
	if ghSvc.lastAssociateTaskID != "solo1" {
		t.Fatalf("AssociatePRWithTask taskID = %q, want solo1 (no group, no redirect)", ghSvc.lastAssociateTaskID)
	}
}

// seedGroupOwnerTaskRepository gives the workspace-group owner task its own
// task_repositories row for repoID, matching the real inherit_parent/
// shared_group scenario where the owner is checked out at the same physical
// repository its members share. Without this, resolveEffectivePushTaskID's F2
// fallback (the owner doesn't hold the repository) would fire and every
// redirect-focused test below would silently degrade into the ungrouped-task
// no-redirect path instead of exercising the redirect itself.
func seedGroupOwnerTaskRepository(t *testing.T, repo *sqliterepo.Repository, taskID, repoID, checkoutBranch string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTaskRepository(context.Background(), &models.TaskRepository{
		ID: "tr-" + taskID, TaskID: taskID, RepositoryID: repoID, CheckoutBranch: checkoutBranch,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create owner task repository: %v", err)
	}
}

// TestEnsureSessionPRWatchRedirectsToWorkspaceGroupOwner covers Review Round
// 1's F1 finding: resolveEffectivePushTaskID was only ever called from
// dispatchPushDetection. ensureSessionPRWatch runs on every session start (see
// task_operations.go's four call sites) and, before this fix, always created
// its watch under the observing subtask's own taskID — so a shared-worktree
// subtask got its own member-attributed watch the moment its session started,
// independent of whether a push had even happened yet.
func TestEnsureSessionPRWatchRedirectsToWorkspaceGroupOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "parent1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Parent", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	seedWorkspaceGroup(t, repo, "group1", "parent1", "parent1", "t1")
	seedGitHubPushAssocFixture(t, repo, "feature-x")
	seedGroupOwnerTaskRepository(t, repo, "parent1", "repo1", "feature-x")

	ghSvc := &mockGitHubService{client: github.NewMockClient()}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	svc.ensureSessionPRWatch(ctx, "t1", "s1", "feature-x")

	if ghSvc.ensureWatchCalls == 0 {
		t.Fatalf("expected EnsurePRWatchForWorkspace to be called")
	}
	if ghSvc.lastEnsureWatchTaskID != "parent1" {
		t.Fatalf("EnsurePRWatchForWorkspace taskID = %q, want parent1 (the group owner)", ghSvc.lastEnsureWatchTaskID)
	}
	for _, c := range ghSvc.ensureWatchLog {
		if c.TaskID == "t1" {
			t.Fatalf("subtask t1 got its own watch, want zero: %+v", ghSvc.ensureWatchLog)
		}
	}
}

// TestCheckSessionPRRedirectsToWorkspaceGroupOwner covers the second F1 gap:
// CheckSessionPR is the frontend's on-demand alternative to push detection
// (triggered by the user clicking "check for PR"), and before this fix wrote
// both the watch and the association under the observing subtask's own
// taskID, reproducing the multi-binding defect on demand instead of only on a
// push.
func TestCheckSessionPRRedirectsToWorkspaceGroupOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "parent1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Parent", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	seedWorkspaceGroup(t, repo, "group1", "parent1", "parent1", "t1")
	seedGitHubPushAssocFixture(t, repo, "feature-x")
	seedGroupOwnerTaskRepository(t, repo, "parent1", "repo1", "feature-x")

	mockClient := github.NewMockClient()
	mockClient.AddPR(&github.PR{
		Number: 9, Title: "On-demand check PR", HTMLURL: "https://github.com/myorg/kandev/pull/9",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "feature-x", BaseBranch: "main",
	})
	ghSvc := &mockGitHubService{client: mockClient}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	found, err := svc.CheckSessionPR(ctx, "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionPR: %v", err)
	}
	if !found {
		t.Fatalf("CheckSessionPR: expected a PR to be found")
	}

	if ghSvc.lastEnsureWatchTaskID != "parent1" {
		t.Fatalf("EnsurePRWatchForWorkspace taskID = %q, want parent1 (the group owner)", ghSvc.lastEnsureWatchTaskID)
	}
	if ghSvc.lastAssociateTaskID != "parent1" {
		t.Fatalf("AssociatePRWithTask taskID = %q, want parent1 (the group owner)", ghSvc.lastAssociateTaskID)
	}
	for _, c := range ghSvc.associateLog {
		if c.TaskID == "t1" {
			t.Fatalf("subtask t1 got its own association, want zero: %+v", ghSvc.associateLog)
		}
	}
}

// TestPushAssociationFallsBackWhenOwnerLacksRepository covers Review Round 1's
// F2 finding: resolveEffectivePushTaskID must not redirect to a workspace
// group owner that doesn't hold the observing task's repository in its own
// task_repositories, since the downstream validateTaskRepositoryID guard would
// then reject the write and the association would be dropped silently —
// contradicting the "never drop the association silently" design principle.
// The write must fall back to the observing task instead.
func TestPushAssociationFallsBackWhenOwnerLacksRepository(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "t1", "s1", "step1")
	// Parent owns the group but was never seeded with repo1 in its own
	// task_repositories — e.g. it added the group after the member already
	// had its own repository wired up.
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "parent1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Parent", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	seedWorkspaceGroup(t, repo, "group1", "parent1", "parent1", "t1")
	seedGitHubPushAssocFixture(t, repo, "feature-x")

	mockClient := github.NewMockClient()
	mockClient.AddPR(&github.PR{
		Number: 10, Title: "Fallback PR", HTMLURL: "https://github.com/myorg/kandev/pull/10",
		RepoOwner: "myorg", RepoName: "kandev", HeadBranch: "feature-x", BaseBranch: "main",
	})
	ghSvc := &mockGitHubService{client: mockClient}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetGitHubService(ghSvc)

	svc.dispatchPushDetection(ctx, "s1", "t1", "", "feature-x")

	if ghSvc.lastCreateWatchTaskID != "t1" {
		t.Fatalf("CreatePRWatch taskID = %q, want t1 (owner lacks repository, fall back to observing task)",
			ghSvc.lastCreateWatchTaskID)
	}
	if ghSvc.lastAssociateTaskID != "t1" {
		t.Fatalf("AssociatePRWithTask taskID = %q, want t1 (owner lacks repository, fall back to observing task)",
			ghSvc.lastAssociateTaskID)
	}
	if len(ghSvc.associateLog) != 1 {
		t.Fatalf("expected exactly one association (not silently dropped), got %+v", ghSvc.associateLog)
	}
}
