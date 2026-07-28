package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

type cacheCountingReviewRequestClient struct {
	*MockClient
	feedbackCalls int
	statusCalls   int
	err           error
}

type reviewRequestInstallationProvider struct {
	client       Client
	capabilities map[GitHubAppCapability]bool
}

func (p reviewRequestInstallationProvider) ResolveInstallation(
	context.Context,
	*WorkspaceConnection,
	ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return &ResolvedCredential{
		Client: p.client,
		Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
		},
		Capabilities: p.capabilities,
	}, nil
}

func newAppReviewRequestService(t *testing.T, client Client, capabilities map[GitHubAppCapability]bool) *Service {
	t.Helper()
	installationID := int64(42)
	service := newTestService(nil)
	service.resolver = NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace-a": {
			WorkspaceID: "workspace-a", Source: ConnectionSourceGitHubAppInstallation,
			InstallationID: &installationID, AppRegistrationID: "registration-a",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}, nil)
	service.resolver.SetInstallationProvider(reviewRequestInstallationProvider{
		client: client, capabilities: capabilities,
	})
	return service
}

func TestService_RequestReviewers_InvalidatesOnlyAffectedInFlightPRCaches(t *testing.T) {
	client := &cacheCountingReviewRequestClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	key := scopedCacheKey("legacy", prStatusCacheKey("o", "r", 1))
	unrelatedKey := scopedCacheKey("legacy", prStatusCacheKey("o", "r", 2))

	for name, cache := range map[string]*ttlCache{
		"feedback": svc.prFeedbackCache,
		"status":   svc.prStatusCache,
	} {
		t.Run(name, func(t *testing.T) {
			targetStarted := make(chan struct{})
			unrelatedStarted := make(chan struct{})
			releaseTarget := make(chan struct{})
			releaseUnrelated := make(chan struct{})
			targetDone := make(chan any, 1)
			unrelatedDone := make(chan any, 1)

			go func() {
				value, _ := cache.doOrFetch(key, func() (any, error) {
					close(targetStarted)
					<-releaseTarget
					return "stale", nil
				})
				targetDone <- value
			}()
			go func() {
				value, _ := cache.doOrFetch(unrelatedKey, func() (any, error) {
					close(unrelatedStarted)
					<-releaseUnrelated
					return "unrelated", nil
				})
				unrelatedDone <- value
			}()

			<-targetStarted
			<-unrelatedStarted
			if err := svc.RequestReviewers(context.Background(), "o", "r", 1, []string{"octocat"}); err != nil {
				t.Fatalf("RequestReviewers: %v", err)
			}

			freshDone := make(chan any, 1)
			go func() {
				value, _ := cache.doOrFetch(key, func() (any, error) {
					return "fresh", nil
				})
				freshDone <- value
			}()
			select {
			case value := <-freshDone:
				if value != "fresh" {
					t.Fatalf("post-invalidation fetch = %v, want fresh", value)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("post-invalidation fetch joined the stale in-flight call")
			}

			close(releaseTarget)
			close(releaseUnrelated)
			<-targetDone
			<-unrelatedDone

			if value, ok := cache.get(key); !ok || value != "fresh" {
				t.Fatalf("affected cache = %v, %t; want fresh, true", value, ok)
			}
			if value, ok := cache.get(unrelatedKey); !ok || value != "unrelated" {
				t.Fatalf("unrelated cache = %v, %t; want unrelated, true", value, ok)
			}
		})
	}
}

func (c *cacheCountingReviewRequestClient) GetPRFeedback(context.Context, string, string, int) (*PRFeedback, error) {
	c.feedbackCalls++
	return &PRFeedback{}, nil
}

func (c *cacheCountingReviewRequestClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	c.statusCalls++
	return &PRStatus{}, nil
}

func (c *cacheCountingReviewRequestClient) RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error {
	if c.err != nil {
		return c.err
	}
	return c.MockClient.RequestReviewers(ctx, owner, repo, number, reviewers)
}

func TestService_RequestReviewers_EvictsOnlyAffectedCachesOnSuccess(t *testing.T) {
	client := &cacheCountingReviewRequestClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	for _, number := range []int{1, 2} {
		if _, err := svc.GetPRFeedback(ctx, "o", "r", number); err != nil {
			t.Fatalf("prime feedback: %v", err)
		}
		if _, err := svc.GetPRStatus(ctx, "o", "r", number); err != nil {
			t.Fatalf("prime status: %v", err)
		}
	}
	if err := svc.RequestReviewers(ctx, "o", "r", 1, []string{"octocat"}); err != nil {
		t.Fatalf("RequestReviewers: %v", err)
	}
	for _, number := range []int{1, 2} {
		if _, err := svc.GetPRFeedback(ctx, "o", "r", number); err != nil {
			t.Fatalf("fetch feedback: %v", err)
		}
		if _, err := svc.GetPRStatus(ctx, "o", "r", number); err != nil {
			t.Fatalf("fetch status: %v", err)
		}
	}
	if client.feedbackCalls != 3 || client.statusCalls != 3 {
		t.Fatalf("calls feedback/status = %d/%d, want 3/3", client.feedbackCalls, client.statusCalls)
	}
}

func TestService_RequestReviewers_KeepsCachesOnFailure(t *testing.T) {
	client := &cacheCountingReviewRequestClient{MockClient: NewMockClient(), err: errors.New("GitHub rejected request")}
	svc := newTestService(client)
	ctx := context.Background()
	if _, err := svc.GetPRFeedback(ctx, "o", "r", 1); err != nil {
		t.Fatalf("prime feedback: %v", err)
	}
	if _, err := svc.GetPRStatus(ctx, "o", "r", 1); err != nil {
		t.Fatalf("prime status: %v", err)
	}
	if err := svc.RequestReviewers(ctx, "o", "r", 1, []string{"octocat"}); err == nil {
		t.Fatal("expected request error")
	}
	if _, err := svc.GetPRFeedback(ctx, "o", "r", 1); err != nil {
		t.Fatalf("fetch feedback: %v", err)
	}
	if _, err := svc.GetPRStatus(ctx, "o", "r", 1); err != nil {
		t.Fatalf("fetch status: %v", err)
	}
	if client.feedbackCalls != 1 || client.statusCalls != 1 {
		t.Fatalf("calls feedback/status = %d/%d, want 1/1", client.feedbackCalls, client.statusCalls)
	}
	if requests := client.RequestedReviews(); len(requests) != 0 {
		t.Fatalf("failed request recorded mock pending state: %#v", requests)
	}
}

func TestService_RequestReviewersForWorkspace_InvalidatesCorrespondingPersonalReadCaches(t *testing.T) {
	client := &cacheCountingReviewRequestClient{MockClient: NewMockClient()}
	client.AddRepos("test-user", []GitHubRepo{{Owner: "o", Name: "r", FullName: "o/r"}})
	service := newTestService(client)
	configureTestWorkspaceAuth(t, service, client, "workspace-a")
	ctx := context.Background()

	if _, err := service.GetPRFeedbackForWorkspace(ctx, "workspace-a", "user-a", "o", "r", 1); err != nil {
		t.Fatalf("prime feedback cache: %v", err)
	}
	if _, err := service.GetPRStatusForWorkspace(ctx, "workspace-a", "user-a", "o", "r", 1); err != nil {
		t.Fatalf("prime status cache: %v", err)
	}

	principal, err := service.RequestReviewersForWorkspace(
		ctx, "workspace-a", "user-a", "o", "r", 1, []string{"octocat"},
	)
	if err != nil {
		t.Fatalf("RequestReviewersForWorkspace: %v", err)
	}
	if principal.WorkspaceID != "workspace-a" || principal.Kind != AuthPrincipalHuman {
		t.Fatalf("principal = %+v", principal)
	}
	if requests := client.RequestedReviews(); len(requests) != 1 || requests[0].Owner != "o" {
		t.Fatalf("review requests = %#v", requests)
	}

	if _, err := service.GetPRFeedbackForWorkspace(ctx, "workspace-a", "user-a", "o", "r", 1); err != nil {
		t.Fatalf("refetch feedback cache: %v", err)
	}
	if _, err := service.GetPRStatusForWorkspace(ctx, "workspace-a", "user-a", "o", "r", 1); err != nil {
		t.Fatalf("refetch status cache: %v", err)
	}
	if client.feedbackCalls != 2 || client.statusCalls != 2 {
		t.Fatalf("calls feedback/status = %d/%d, want 2/2", client.feedbackCalls, client.statusCalls)
	}
}

func TestService_RequestReviewersForWorkspace_RejectsMissingAppWriteCapability(t *testing.T) {
	service := newAppReviewRequestService(t, &stubClient{}, nil)
	_, err := service.RequestReviewersForWorkspace(
		context.Background(), "workspace-a", "user-a", "o", "r", 1, []string{"octocat"},
	)
	if !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("RequestReviewersForWorkspace error = %v, want capability denied", err)
	}
}

func TestService_RequestReviewersForWorkspace_RejectsRepositoryOutsideWorkspaceScope(t *testing.T) {
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-a")
	if err := store.UpsertWorkspaceSettings(context.Background(), &WorkspaceSettings{
		WorkspaceID: "workspace-a", RepoScopeMode: RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{{Owner: "o", Name: "allowed"}},
	}); err != nil {
		t.Fatalf("configure workspace repository scope: %v", err)
	}
	service := NewService(NewMockClient(), AuthMethodPAT, nil, store, nil, testLogger(t))
	configureTestWorkspaceAuth(t, service, NewMockClient(), "workspace-a")
	_, err := service.RequestReviewersForWorkspace(
		context.Background(), "workspace-a", "user-a", "o", "outside", 1, []string{"octocat"},
	)
	if !errors.Is(err, ErrRepoNotResolvable) {
		t.Fatalf("RequestReviewersForWorkspace error = %v, want repository scope denial", err)
	}
}
