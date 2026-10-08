package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

func TestInspectRemoteRepositorySelectionDoesNotWrite(t *testing.T) {
	svc, _, repo := createTestService(t)
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)

	verified, err := svc.InspectRemoteRepositorySelection(context.Background(), "ws-1", TaskRepositoryInput{
		Provider: "fixture-source-control", RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
	})
	if err != nil {
		t.Fatalf("InspectRemoteRepositorySelection: %v", err)
	}
	if verified.RemoteURL != "https://bitbucket.example.test/scm/TEAM/fixture.git" ||
		verified.ProviderRepoID != "repo-42" || verified.DefaultBranch != "main" {
		t.Fatalf("verified descriptor = %+v", verified)
	}
	repositories, err := repo.ListRepositories(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("inspection persisted repositories: %+v", repositories)
	}
}

func TestRegisterRemoteRepositorySelectionReverifiesAndDeduplicates(t *testing.T) {
	svc, _, repo := createTestService(t)
	createRepositorySelectionWorkspace(t, repo)
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)
	input := TaskRepositoryInput{
		Provider: "fixture-source-control", RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
	}

	first, err := svc.RegisterRemoteRepositorySelection(context.Background(), "ws-1", input)
	if err != nil {
		t.Fatalf("first RegisterRemoteRepositorySelection: %v", err)
	}
	second, err := svc.RegisterRemoteRepositorySelection(context.Background(), "ws-1", input)
	if err != nil {
		t.Fatalf("retry RegisterRemoteRepositorySelection: %v", err)
	}
	if first.ID != second.ID || first.RemoteURL != "https://bitbucket.example.test/scm/TEAM/fixture.git" ||
		first.ProviderScope != "workspace-a" || first.ProviderRepoID != "repo-42" || first.DefaultBranch != "main" {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if len(resolver.calls) != 2 {
		t.Fatalf("resolver calls = %d, want re-verification for both requests", len(resolver.calls))
	}
	repositories, err := repo.ListRepositories(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 1 {
		t.Fatalf("registered %d records after retry, want one", len(repositories))
	}
}

func TestRegisterRemoteRepositorySelectionConcurrentRetryConverges(t *testing.T) {
	svc, _, repo := createTestService(t)
	createRepositorySelectionWorkspace(t, repo)
	const attempts = 12
	resolver := &concurrentRepositorySelectionResolver{attempts: attempts, ready: make(chan struct{})}
	svc.SetRepositorySelectionResolver(resolver)
	input := TaskRepositoryInput{
		Provider: "fixture-source-control", RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
	}
	start := make(chan struct{})
	ids := make(chan string, attempts)
	errs := make(chan error, attempts)
	var wait sync.WaitGroup
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			repository, err := svc.RegisterRemoteRepositorySelection(context.Background(), "ws-1", input)
			if err != nil {
				errs <- err
				return
			}
			ids <- repository.ID
		}()
	}
	close(start)
	wait.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent registration: %v", err)
	}
	var canonicalID string
	for id := range ids {
		if canonicalID == "" {
			canonicalID = id
		} else if id != canonicalID {
			t.Errorf("concurrent IDs differ: %q and %q", canonicalID, id)
		}
	}
	if canonicalID == "" {
		t.Fatal("no registration succeeded")
	}
	repositories, err := repo.ListRepositories(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 1 || repositories[0].ID != canonicalID {
		t.Fatalf("registered repositories = %+v, want one row with ID %q", repositories, canonicalID)
	}
	if got := resolver.calls.Load(); got != attempts {
		t.Fatalf("provider verification calls = %d, want %d", got, attempts)
	}
}

type concurrentRepositorySelectionResolver struct {
	attempts int
	calls    atomic.Int32
	ready    chan struct{}
	once     sync.Once
}

func (r *concurrentRepositorySelectionResolver) ResolveRepositorySelection(
	_ context.Context, _ string, input TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	if int(r.calls.Add(1)) == r.attempts {
		r.once.Do(func() { close(r.ready) })
	}
	<-r.ready
	return authoritativeRepositoryInput(input)
}

func TestInspectRemoteRepositorySelectionRejectsMismatchedHints(t *testing.T) {
	svc, _, repo := createTestService(t)
	createRepositorySelectionWorkspace(t, repo)
	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{resolve: authoritativeRepositoryInput})

	_, err := svc.InspectRemoteRepositorySelection(context.Background(), "ws-1", TaskRepositoryInput{
		Provider: "fixture-source-control", RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
		ProviderOwner: "attacker",
	})
	assertRepositorySelectionError(t, err, RepositorySelectionErrorInvalid, "")
}

func TestRegisterRemoteRepositorySelectionRequiresRepositoryManage(t *testing.T) {
	svc, _, repo := createTestService(t)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "Workspace", OwnerID: "owner"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
		WorkspaceID: "ws-1", UserID: "reader", Role: string(authz.WorkspaceRoleViewer),
	}); err != nil {
		t.Fatalf("UpsertWorkspaceMember: %v", err)
	}
	resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
	svc.SetRepositorySelectionResolver(resolver)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "reader", Role: authn.RoleMember})

	_, err := svc.RegisterRemoteRepositorySelection(ctx, "ws-1", TaskRepositoryInput{
		Provider: "fixture-source-control", RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("RegisterRemoteRepositorySelection error = %v, want forbidden", err)
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("resolver called %d times before authorization", len(resolver.calls))
	}
}
