package backendapp

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type fakeContributorForkService struct {
	policy         github.TaskGitCredentialPolicy
	policyErr      error
	resolution     github.ContributionForkResolution
	resolveErr     error
	calls          int
	create         bool
	resolveStarted chan struct{}
	resolveRelease <-chan struct{}
}

func (f *fakeContributorForkService) DescribeTaskGitCredentialPolicy(
	context.Context, string,
) (github.TaskGitCredentialPolicy, error) {
	return f.policy, f.policyErr
}

func (f *fakeContributorForkService) ResolveContributionForkForWorkspace(
	_ context.Context, _, _, _ string, create bool,
) (github.ContributionForkResolution, error) {
	f.calls++
	f.create = create
	if f.resolveStarted != nil {
		close(f.resolveStarted)
	}
	if f.resolveRelease != nil {
		<-f.resolveRelease
	}
	return f.resolution, f.resolveErr
}

type fakeContributorForkTaskRepository struct {
	task       *taskmodels.Task
	repository *taskmodels.Repository
	links      []*taskmodels.TaskRepository
	updates    int
}

type fakeContributorForkRepositoryUpdater struct {
	calls          int
	repositoryID   string
	providerRepoID string
	err            error
}

func (u *fakeContributorForkRepositoryUpdater) UpdateRepository(
	_ context.Context, repositoryID string, request *taskservice.UpdateRepositoryRequest,
) (*taskmodels.Repository, error) {
	u.calls++
	u.repositoryID = repositoryID
	if request != nil && request.ProviderRepoID != nil {
		u.providerRepoID = *request.ProviderRepoID
	}
	return nil, u.err
}

func (r *fakeContributorForkTaskRepository) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return r.task, nil
}

func (r *fakeContributorForkTaskRepository) GetRepository(context.Context, string) (*taskmodels.Repository, error) {
	return r.repository, nil
}

func (r *fakeContributorForkTaskRepository) ListTaskRepositories(
	context.Context, string,
) ([]*taskmodels.TaskRepository, error) {
	return r.links, nil
}

func (r *fakeContributorForkTaskRepository) BindTaskRepositoryContributionDestination(
	_ context.Context,
	id, taskID, repositoryID string,
	destination *taskmodels.ContributionDestination,
) (*taskmodels.TaskRepository, bool, error) {
	current := r.links[0]
	if current.ID != id || current.TaskID != taskID || current.RepositoryID != repositoryID {
		return nil, false, context.Canceled
	}
	bound, err := hasContributorPublicationBinding(current.Metadata)
	if err != nil || bound {
		return current, false, err
	}
	updated := *current
	updated.Metadata = maps.Clone(current.Metadata)
	if updated.Metadata == nil {
		updated.Metadata = make(map[string]interface{})
	}
	if err := taskmodels.PutContributionDestination(updated.Metadata, destination); err != nil {
		return nil, false, err
	}
	r.updates++
	r.links[0] = &updated
	return &updated, true, nil
}

func TestPrepareContributorForkLeaseBindsExistingWorkspaceForkForOrdinaryTask(t *testing.T) {
	destination := testBoundContributorForkDestination()
	provider := &fakeContributorForkService{
		policy: github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged},
		resolution: github.ContributionForkResolution{
			Status: github.ContributionForkStatusReady, Destination: &destination,
		},
	}
	repo := &fakeContributorForkTaskRepository{
		task: &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		repository: &taskmodels.Repository{
			ID: "repo-1", WorkspaceID: "workspace-1", Provider: "github",
			ProviderHost: "https://github.com", ProviderOwner: "kdlbs", ProviderName: "kandev",
			ProviderRepoID: "100",
		},
		links: []*taskmodels.TaskRepository{{
			ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", Metadata: map[string]interface{}{},
		}},
	}
	preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

	if err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1"); err != nil {
		t.Fatalf("PrepareContributorForkLease() error = %v", err)
	}
	if provider.calls != 1 || provider.create {
		t.Fatalf("provider calls = %d, create = %v; want one read-only resolution", provider.calls, provider.create)
	}
	if repo.updates != 1 {
		t.Fatalf("task repository updates = %d, want 1", repo.updates)
	}
	got, ok, err := taskmodels.LoadContributionDestination(repo.links[0].Metadata)
	if err != nil || !ok || got != destination {
		t.Fatalf("stored destination = %#v, %v, %v; want %#v, true, nil", got, ok, err, destination)
	}
}

func TestPrepareContributorForkLeasePreservesConcurrentAttachmentChanges(t *testing.T) {
	destination := testBoundContributorForkDestination()
	started := make(chan struct{})
	release := make(chan struct{})
	provider := &fakeContributorForkService{
		policy: github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged},
		resolution: github.ContributionForkResolution{
			Status: github.ContributionForkStatusReady, Destination: &destination,
		},
		resolveStarted: started,
		resolveRelease: release,
	}
	repo := ordinaryContributorTaskRepository(canonicalContributorRepository())
	repo.links[0].BaseBranch = "main"
	preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

	done := make(chan error, 1)
	go func() {
		done <- preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1")
	}()
	<-started
	concurrent := *repo.links[0]
	concurrent.BaseBranch = "release/concurrent"
	concurrent.Metadata = map[string]interface{}{"concurrent": "preserved"}
	repo.links[0] = &concurrent
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("PrepareContributorForkLease() error = %v", err)
	}

	if repo.links[0].BaseBranch != "release/concurrent" || repo.links[0].Metadata["concurrent"] != "preserved" {
		t.Fatalf("concurrent attachment change was overwritten: %#v", repo.links[0])
	}
	if _, ok, err := taskmodels.LoadContributionDestination(repo.links[0].Metadata); err != nil || !ok {
		t.Fatalf("destination missing after concurrent update: ok=%v err=%v metadata=%#v", ok, err, repo.links[0].Metadata)
	}
}

func TestPrepareContributorForkLeaseRejectsCrossWorkspaceTask(t *testing.T) {
	provider := &fakeContributorForkService{policy: github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged}}
	repo := &fakeContributorForkTaskRepository{task: &taskmodels.Task{ID: "task-1", WorkspaceID: "other-workspace"}}
	preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

	err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1")
	if err == nil || !strings.Contains(err.Error(), "does not belong to workspace") {
		t.Fatalf("PrepareContributorForkLease() error = %v, want workspace denial", err)
	}
	if provider.calls != 0 || repo.updates != 0 {
		t.Fatalf("cross-workspace request reached provider or storage: calls=%d updates=%d", provider.calls, repo.updates)
	}
}

func TestPrepareContributorForkLeaseDoesNotWidenUnmanagedOrUnrelatedTasks(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		policyMode string
		repository *taskmodels.Repository
	}{
		{
			name: "executor credentials", policyMode: github.TaskGitCredentialsModeExecutor,
			repository: canonicalContributorRepository(),
		},
		{
			name: "arbitrary GitHub repository", policyMode: github.TaskGitCredentialsModeManaged,
			repository: &taskmodels.Repository{
				ID: "repo-1", WorkspaceID: "workspace-1", Provider: "github", ProviderHost: "https://github.com",
				ProviderOwner: "someone", ProviderName: "kandev", ProviderRepoID: "100",
			},
		},
		{
			name: "lookalike provider host", policyMode: github.TaskGitCredentialsModeManaged,
			repository: &taskmodels.Repository{
				ID: "repo-1", WorkspaceID: "workspace-1", Provider: "github", ProviderHost: "https://github.example",
				ProviderOwner: "kdlbs", ProviderName: "kandev", ProviderRepoID: "100",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := &fakeContributorForkService{
				policy:     github.TaskGitCredentialPolicy{Mode: testCase.policyMode},
				resolution: github.ContributionForkResolution{Destination: pointerToDestination(testBoundContributorForkDestination())},
			}
			repo := ordinaryContributorTaskRepository(testCase.repository)
			preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

			if err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1"); err != nil {
				t.Fatalf("PrepareContributorForkLease() error = %v", err)
			}
			if provider.calls != 0 || repo.updates != 0 {
				t.Fatalf("provider calls = %d, updates = %d; want no widened scope", provider.calls, repo.updates)
			}
		})
	}
}

func TestPrepareContributorForkLeasePreservesExistingServerBindings(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		metadata map[string]interface{}
	}{
		{name: "contribution destination", metadata: metadataWithDestination(t, testBoundContributorForkDestination())},
		{name: "remote contribution", metadata: metadataWithRemoteContribution(t)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := &fakeContributorForkService{policy: github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged}}
			repo := ordinaryContributorTaskRepository(canonicalContributorRepository())
			repo.links[0].Metadata = testCase.metadata
			preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

			if err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1"); err != nil {
				t.Fatalf("PrepareContributorForkLease() error = %v", err)
			}
			if provider.calls != 0 || repo.updates != 0 {
				t.Fatalf("existing binding was replaced: calls=%d updates=%d", provider.calls, repo.updates)
			}
		})
	}
}

func TestPrepareContributorForkLeaseRequiresMatchingCanonicalProviderIdentity(t *testing.T) {
	destination := testBoundContributorForkDestination()
	provider := &fakeContributorForkService{
		policy:     github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged},
		resolution: github.ContributionForkResolution{Destination: &destination},
	}
	repository := canonicalContributorRepository()
	repository.ProviderRepoID = "999"
	repo := ordinaryContributorTaskRepository(repository)
	preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

	err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1")
	if err == nil || !strings.Contains(err.Error(), "provider identity changed") {
		t.Fatalf("PrepareContributorForkLease() error = %v, want identity denial", err)
	}
	if repo.updates != 0 {
		t.Fatalf("task repository updates = %d, want 0", repo.updates)
	}
}

func TestPrepareContributorForkLeaseRejectsUnboundProviderDestination(t *testing.T) {
	destination := testBoundContributorForkDestination()
	destination.CredentialBinding = nil
	provider := &fakeContributorForkService{
		policy:     github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged},
		resolution: github.ContributionForkResolution{Destination: &destination},
	}
	repo := ordinaryContributorTaskRepository(canonicalContributorRepository())
	preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

	err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1")
	if err == nil || !strings.Contains(err.Error(), "credential binding") {
		t.Fatalf("PrepareContributorForkLease() error = %v, want credential-binding denial", err)
	}
	if repo.updates != 0 {
		t.Fatalf("task repository updates = %d, want 0", repo.updates)
	}
}

func TestPrepareContributorForkLeaseBackfillsVerifiedCanonicalProviderIdentity(t *testing.T) {
	destination := testBoundContributorForkDestination()
	provider := &fakeContributorForkService{
		policy:     github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged},
		resolution: github.ContributionForkResolution{Destination: &destination},
	}
	repository := canonicalContributorRepository()
	repository.ProviderRepoID = ""
	repo := ordinaryContributorTaskRepository(repository)
	updater := &fakeContributorForkRepositoryUpdater{}
	preparer := &githubContributionDestinationPreparer{service: provider, taskSvc: updater, repo: repo}

	if err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1"); err != nil {
		t.Fatalf("PrepareContributorForkLease() error = %v", err)
	}
	if updater.calls != 1 || updater.repositoryID != "repo-1" || updater.providerRepoID != "100" {
		t.Fatalf("provider identity backfill = %d (%q, %q), want 1 (repo-1, 100)",
			updater.calls, updater.repositoryID, updater.providerRepoID)
	}
	if repo.updates != 1 {
		t.Fatalf("task repository updates = %d, want 1", repo.updates)
	}
}

func TestPrepareImproveKandevDestinationUsesCanonicalIdentityFromBoundFork(t *testing.T) {
	destination := testBoundContributorForkDestination()
	provider := &fakeContributorForkService{
		policy: github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged},
		resolution: github.ContributionForkResolution{
			Status: github.ContributionForkStatusReady,
			Repository: &github.GitHubRepository{
				ID: 200, FullName: "yattdev/kandev", Fork: true, ParentID: 100, ParentFullName: "kdlbs/kandev",
			},
			Destination: &destination,
		},
	}
	repository := canonicalContributorRepository()
	repository.ProviderRepoID = ""
	updater := &fakeContributorForkRepositoryUpdater{}
	preparer := &githubContributionDestinationPreparer{service: provider, taskSvc: updater}
	templateID := "improve-kandev"
	request := &taskservice.CreateTaskRequest{
		WorkspaceID:  "workspace-1",
		Repositories: []taskservice.TaskRepositoryInput{{RepositoryID: "repo-1"}},
	}

	if err := preparer.PrepareContributionDestination(
		context.Background(), request, &taskmodels.Workflow{WorkflowTemplateID: &templateID}, []*taskmodels.Repository{repository},
	); err != nil {
		t.Fatalf("PrepareContributionDestination() error = %v", err)
	}
	if request.Repositories[0].ProviderRepoID != "100" || request.Repositories[0].ContributionDestination != &destination {
		t.Fatalf("prepared repository = %#v, want canonical provider ID 100 and bound destination", request.Repositories[0])
	}
}

func TestPrepareContributorForkLeaseDoesNotCreateMissingFork(t *testing.T) {
	provider := &fakeContributorForkService{
		policy:     github.TaskGitCredentialPolicy{Mode: github.TaskGitCredentialsModeManaged},
		resolution: github.ContributionForkResolution{Status: github.ContributionForkStatusCreatable},
	}
	repo := ordinaryContributorTaskRepository(canonicalContributorRepository())
	preparer := &githubContributionDestinationPreparer{service: provider, repo: repo}

	if err := preparer.PrepareContributorForkLease(context.Background(), "workspace-1", "task-1"); err != nil {
		t.Fatalf("PrepareContributorForkLease() error = %v", err)
	}
	if provider.calls != 1 || provider.create || repo.updates != 0 {
		t.Fatalf("missing fork handling: calls=%d create=%v updates=%d", provider.calls, provider.create, repo.updates)
	}
}

func canonicalContributorRepository() *taskmodels.Repository {
	return &taskmodels.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", Provider: "github", ProviderHost: "https://github.com",
		ProviderOwner: "kdlbs", ProviderName: "kandev", ProviderRepoID: "100",
	}
}

func ordinaryContributorTaskRepository(repository *taskmodels.Repository) *fakeContributorForkTaskRepository {
	return &fakeContributorForkTaskRepository{
		task:       &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		repository: repository,
		links: []*taskmodels.TaskRepository{{
			ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", Metadata: map[string]interface{}{},
		}},
	}
}

func pointerToDestination(destination taskmodels.ContributionDestination) *taskmodels.ContributionDestination {
	return &destination
}

func metadataWithDestination(t *testing.T, destination taskmodels.ContributionDestination) map[string]interface{} {
	t.Helper()
	metadata := map[string]interface{}{}
	if err := taskmodels.PutContributionDestination(metadata, &destination); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func metadataWithRemoteContribution(t *testing.T) map[string]interface{} {
	t.Helper()
	binding := taskmodels.RemoteContribution{
		Version: taskmodels.RemoteContributionVersion, Provider: taskmodels.RemoteContributionProviderGitHub,
		Kind: taskmodels.RemoteContributionKindPullRequest, CanonicalURL: "https://github.com/kdlbs/kandev/pull/7",
		Number: 7, State: taskmodels.RemoteContributionStateOpen, BaseBranch: "main", HeadBranch: "feature/task",
		HeadSHA: strings.Repeat("a", 40), CollaborationAllowed: true,
		SourceRepository: taskmodels.RemoteContributionRepository{
			Host: "github.com", Path: "yattdev/kandev", ProviderID: "200", RemoteURL: "https://github.com/yattdev/kandev.git",
		},
	}
	metadata := map[string]interface{}{}
	if err := taskmodels.PutRemoteContribution(metadata, &binding); err != nil {
		t.Fatal(err)
	}
	return metadata
}

func testBoundContributorForkDestination() taskmodels.ContributionDestination {
	return taskmodels.ContributionDestination{
		Version: taskmodels.ContributionDestinationVersion, Provider: taskmodels.ContributionDestinationProviderGitHub,
		SourceRepository: taskmodels.ContributionDestinationRepository{
			Host: "github.com", Path: "kdlbs/kandev", ProviderID: "100", RemoteURL: "https://github.com/kdlbs/kandev.git",
		},
		TargetRepository: taskmodels.ContributionDestinationRepository{
			Host: "github.com", Path: "yattdev/kandev", ProviderID: "200", RemoteURL: "https://github.com/yattdev/kandev.git",
		},
		CredentialBinding: &taskmodels.ContributionDestinationCredentialBinding{
			Source: "gh_cli", Login: "yattdev", CredentialGeneration: 7,
		},
	}
}
