package github

import (
	"context"
	"testing"
	"time"
)

const (
	forkOwner        = "alice"
	forkRepo         = "kandev"
	parentOwner      = "kdlbs"
	parentRepo       = "kandev"
	forkPRBranch     = "feature/fork-pr-association"
	forkPRNumber     = 2652
	forkRepositoryID = 200
)

func seedForkNetwork(client *MockClient) {
	client.SetRepositoryDetails(GitHubRepository{
		ID:       100,
		FullName: parentOwner + "/" + parentRepo,
		Owner:    parentOwner,
		Name:     parentRepo,
	})
	client.SetRepositoryDetails(GitHubRepository{
		ID:             forkRepositoryID,
		FullName:       forkOwner + "/" + forkRepo,
		Owner:          forkOwner,
		Name:           forkRepo,
		Fork:           true,
		ParentID:       100,
		ParentFullName: parentOwner + "/" + parentRepo,
	})
}

func forkNetworkPR() *PR {
	return &PR{
		Number:        forkPRNumber,
		Title:         "Contributor change",
		State:         "open",
		URL:           "https://github.com/kdlbs/kandev/pull/2652",
		HeadBranch:    forkPRBranch,
		HeadRepoOwner: forkOwner,
		HeadRepoName:  forkRepo,
		RepoOwner:     parentOwner,
		RepoName:      parentRepo,
		BaseRepoOwner: parentOwner,
		BaseRepoName:  parentRepo,
	}
}

func TestFindPRByBranchForWorkspace_ForkFallsBackToParentWithExactHead(t *testing.T) {
	_, service, client, _ := setupPollerTest(t)
	seedForkNetwork(client)
	client.AddPR(forkNetworkPR())

	pr, err := service.FindPRByBranchForWorkspace(
		context.Background(), testWorkspaceID, forkOwner, forkRepo, forkPRBranch,
	)
	if err != nil {
		t.Fatalf("FindPRByBranchForWorkspace: %v", err)
	}
	if pr == nil {
		t.Fatal("FindPRByBranchForWorkspace returned nil, want the parent PR")
	}
	if pr.Number != forkPRNumber || pr.RepoOwner != parentOwner || pr.RepoName != parentRepo {
		t.Fatalf("PR = %#v, want #%d in %s/%s", pr, forkPRNumber, parentOwner, parentRepo)
	}
}

func TestFindPRByBranchForWorkspace_DoesNotUseAnotherForkWithSameBranch(t *testing.T) {
	_, service, client, _ := setupPollerTest(t)
	seedForkNetwork(client)
	otherForkPR := forkNetworkPR()
	otherForkPR.HeadRepoOwner = "another-user"
	otherForkPR.HeadRepoName = forkRepo
	client.AddPR(otherForkPR)

	pr, err := service.FindPRByBranchForWorkspace(
		context.Background(), testWorkspaceID, forkOwner, forkRepo, forkPRBranch,
	)
	if err != nil {
		t.Fatalf("FindPRByBranchForWorkspace: %v", err)
	}
	if pr != nil {
		t.Fatalf("PR = %#v, want nil because the parent PR has another head repository", pr)
	}
}

func TestPollerDetectPRForWatch_RebindsForkWatchToParentPRRepository(t *testing.T) {
	poller, _, client, store := setupPollerTest(t)
	seedTask(t, store, "task-1", false)
	seedForkNetwork(client)
	client.AddPR(forkNetworkPR())

	watch := &PRWatch{
		WorkspaceID:  testWorkspaceID,
		SessionID:    "session-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        forkOwner,
		Repo:         forkRepo,
		PRNumber:     0,
		Branch:       forkPRBranch,
	}
	if err := store.CreatePRWatch(context.Background(), watch); err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}

	poller.detectPRForWatch(context.Background(), watch)

	updated, err := store.GetPRWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("GetPRWatch: %v", err)
	}
	if updated == nil {
		t.Fatal("GetPRWatch returned nil")
	}
	if updated.Owner != parentOwner || updated.Repo != parentRepo {
		t.Fatalf("watch repository = %s/%s, want %s/%s", updated.Owner, updated.Repo, parentOwner, parentRepo)
	}
	if updated.PRNumber != forkPRNumber {
		t.Fatalf("watch PR number = %d, want %d", updated.PRNumber, forkPRNumber)
	}
}

func TestSyncWorkspaceWatchesBatched_ForkFallsBackToParent(t *testing.T) {
	_, service, client, store := setupBatchedPollerTest(t)
	seedTask(t, store, "task-1", false)
	seedForkNetwork(client.MockClient)
	client.AddPR(forkNetworkPR())
	client.branchResponses = []string{`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`}

	watch := &PRWatch{
		WorkspaceID:  testWorkspaceID,
		SessionID:    "session-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        forkOwner,
		Repo:         forkRepo,
		PRNumber:     0,
		Branch:       forkPRBranch,
	}
	if err := store.CreatePRWatch(context.Background(), watch); err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}

	if _, err := service.SyncWorkspaceWatchesBatched(context.Background(), testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("SyncWorkspaceWatchesBatched: %v", err)
	}

	updated, err := store.GetPRWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("GetPRWatch: %v", err)
	}
	if updated.Owner != parentOwner || updated.Repo != parentRepo || updated.PRNumber != forkPRNumber {
		t.Fatalf("watch = %#v, want parent repository and PR #%d", updated, forkPRNumber)
	}
}

func TestSyncWorkspaceWatchesBatched_ForkRetryReleasesOriginalHealthTarget(t *testing.T) {
	_, service, client, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-fork-health", false)
	seedForkNetwork(client.MockClient)
	client.AddPR(forkNetworkPR())

	fixedNow := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	service.prDiscoveryHealth.setNow(func() time.Time { return fixedNow })
	scope := testAutomationScope(t, service, testWorkspaceID)
	client.branchResponses = []string{
		`{"errors":[{"type":"GRAPHQL_VALIDATION_FAILED","message":"Field 'cloneUrl' doesn't exist on type 'Repository'"}]}`,
		`{"data":{"b0":{"pullRequests":{"nodes":[]}}}}`,
	}
	watch := &PRWatch{
		WorkspaceID: testWorkspaceID, SessionID: "session-fork-health", TaskID: "task-fork-health",
		RepositoryID: "repo-fork", Owner: forkOwner, Repo: forkRepo, Branch: forkPRBranch,
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create fork health watch: %v", err)
	}

	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("initial failed fork discovery: %v", err)
	}
	failed := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if failed.State != PRDiscoveryHealthDegraded || failed.FailedTargetCount != 1 {
		t.Fatalf("failed fork health = %+v, want one degraded fork target", failed)
	}

	fixedNow = fixedNow.Add(PRDiscoveryRetryBase)
	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("successful fork retry: %v", err)
	}
	updated, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil || updated == nil {
		t.Fatalf("reload fork watch: %v, %#v", err, updated)
	}
	if updated.Owner != parentOwner || updated.Repo != parentRepo || updated.PRNumber != forkPRNumber {
		t.Fatalf("fork watch = %#v, want parent repository and PR #%d", updated, forkPRNumber)
	}
	recovered := service.prDiscoveryHealth.snapshot(testWorkspaceID, scope, 0)
	if recovered.State != PRDiscoveryHealthHealthy || recovered.FailedTargetCount != 0 {
		t.Fatalf("fork health after retry = %+v, want healthy with original target released", recovered)
	}

	originalTarget := prDiscoveryHealthTarget{Owner: forkOwner, Repo: forkRepo, Branch: forkPRBranch}
	service.prDiscoveryHealth.mu.Lock()
	originalState := service.prDiscoveryHealth.scopes[prDiscoveryScopeKey(testWorkspaceID, scope)].targets[originalTarget.key()]
	service.prDiscoveryHealth.mu.Unlock()
	if originalState != nil {
		t.Fatalf("original fork target state = %#v, want released after rebind", originalState)
	}
}
