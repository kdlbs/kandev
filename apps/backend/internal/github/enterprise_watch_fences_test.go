package github

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type enterpriseWatchClient struct {
	NoopClient
	admitted, release chan struct{}
}

func (c *enterpriseWatchClient) wait(ctx context.Context) error {
	if c.admitted == nil {
		return nil
	}
	close(c.admitted)
	select {
	case <-c.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *enterpriseWatchClient) ListIssues(ctx context.Context, _, _ string) ([]*Issue, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	return []*Issue{{Number: 7, RepoOwner: "acme", RepoName: "demo"}}, nil
}
func (c *enterpriseWatchClient) ListReviewRequestedPRs(ctx context.Context, _, _, _ string) ([]*PR, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	return []*PR{{Number: 7, RepoOwner: "acme", RepoName: "demo"}}, nil
}

type enterpriseWatchCredentials struct{ client Client }

func (p enterpriseWatchCredentials) ResolveAutomation(_ context.Context, c *WorkspaceConnection, _ ResolveCredentialRequest) (*ResolvedCredential, error) {
	return &ResolvedCredential{Client: p.client, GitHubHost: c.GitHubHost, Principal: AuthPrincipal{GitHubHost: c.GitHubHost, Kind: AuthPrincipalHuman}, Capabilities: allTokenCapabilities()}, nil
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseWatchDelayedResultsFenceHostAndGeneration(t *testing.T) {
	for _, kind := range []string{"issue", "review"} {
		for _, replacement := range []string{"control", "host", "generation"} {
			t.Run(kind+"/"+replacement, func(t *testing.T) {
				service, store := setupWatchServiceTest(t)
				connection := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com", Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}
				if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
					t.Fatal(err)
				}
				client := &enterpriseWatchClient{}
				if replacement != "control" {
					client.admitted = make(chan struct{})
					client.release = make(chan struct{})
				}
				service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
				issueWatch := &IssueWatch{WorkspaceID: "ws-1", GitHubHost: connection.GitHubHost, Enabled: true, WorkflowID: "wf-1", WorkflowStepID: "step-1"}
				reviewWatch := &ReviewWatch{WorkspaceID: "ws-1", GitHubHost: connection.GitHubHost, Enabled: true, WorkflowID: "wf-1", WorkflowStepID: "step-1", ReviewScope: ReviewScopeUser}
				if kind == "issue" {
					if err := store.CreateIssueWatch(context.Background(), issueWatch); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := store.CreateReviewWatch(context.Background(), reviewWatch); err != nil {
						t.Fatal(err)
					}
				}
				type result struct {
					count int
					err   error
				}
				done := make(chan result, 1)
				go func() {
					if kind == "issue" {
						items, err := service.CheckIssueWatch(context.Background(), issueWatch)
						done <- result{len(items), err}
					} else {
						items, err := service.CheckReviewWatch(context.Background(), reviewWatch)
						done <- result{len(items), err}
					}
				}()
				if replacement != "control" {
					<-client.admitted
					next := *connection
					next.CredentialGeneration++
					if replacement == "host" {
						next.GitHubHost = "other.example.com"
					}
					if err := store.UpsertWorkspaceConnection(context.Background(), &next); err != nil {
						close(client.release)
						t.Fatal(err)
					}
					close(client.release)
				}
				got := <-done
				if replacement == "control" {
					if got.err != nil || got.count != 1 {
						t.Fatalf("positive poll control: %+v", got)
					}
					return
				}
				if !errors.Is(got.err, ErrGitHubHostMismatch) || got.count != 0 {
					t.Fatalf("stale provider results accepted: %+v", got)
				}
				if kind == "issue" {
					stored, err := store.GetIssueWatch(context.Background(), issueWatch.ID)
					if err != nil || stored.LastPolledAt != nil {
						t.Fatalf("stale issue poll persisted: %+v, error %v", stored, err)
					}
				} else {
					stored, err := store.GetReviewWatch(context.Background(), reviewWatch.ID)
					if err != nil || stored.LastPolledAt != nil {
						t.Fatalf("stale review poll persisted: %+v, error %v", stored, err)
					}
				}
			})
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseWatchPublicationUsesCapturedGeneration(t *testing.T) {
	for _, kind := range []string{"issue", "review"} {
		t.Run(kind, func(t *testing.T) {
			service, store := setupWatchServiceTest(t)
			events := &mockEventBus{}
			service.eventBus = events
			connection := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com", Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}
			if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
				t.Fatal(err)
			}
			service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: &enterpriseWatchClient{}})
			var publish func()
			if kind == "issue" {
				watch := &IssueWatch{WorkspaceID: "ws-1", GitHubHost: connection.GitHubHost}
				items, err := service.CheckIssueWatch(context.Background(), watch)
				if err != nil || len(items) != 1 {
					t.Fatalf("issue poll %+v, error %v", items, err)
				}
				publish = func() { service.publishNewIssueEvent(context.Background(), watch, items[0]) }
			} else {
				watch := &ReviewWatch{WorkspaceID: "ws-1", GitHubHost: connection.GitHubHost, ReviewScope: ReviewScopeUser}
				items, err := service.CheckReviewWatch(context.Background(), watch)
				if err != nil || len(items) != 1 {
					t.Fatalf("review poll %+v, error %v", items, err)
				}
				publish = func() { service.publishNewReviewPREvent(context.Background(), watch, items[0]) }
			}
			publish()
			if len(events.events) != 1 {
				t.Fatal("positive publication control failed")
			}
			connection.CredentialGeneration++
			if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
				t.Fatal(err)
			}
			publish()
			if len(events.events) != 1 {
				t.Fatal("old generation published after connection replacement")
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseSerializedWatchEventReservationRejectsStaleAuthority(t *testing.T) {
	for _, kind := range []string{"issue", "review"} {
		for _, replacement := range []string{"host", "generation"} {
			t.Run(kind+"/"+replacement, func(t *testing.T) {
				service, store := setupWatchServiceTest(t)
				connection := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com", Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}
				if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
					t.Fatal(err)
				}
				var reserve func(context.Context, int) (bool, error)
				if kind == "issue" {
					watch := &IssueWatch{WorkspaceID: "ws-1", GitHubHost: connection.GitHubHost}
					if err := store.CreateIssueWatch(context.Background(), watch); err != nil {
						t.Fatal(err)
					}
					reserve = func(ctx context.Context, number int) (bool, error) {
						return service.ReserveIssueWatchTask(ctx, watch.ID, "acme", "demo", number, "https://git.example.com/acme/demo/issues/7")
					}
				} else {
					watch := &ReviewWatch{WorkspaceID: "ws-1", GitHubHost: connection.GitHubHost}
					if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
						t.Fatal(err)
					}
					reserve = func(ctx context.Context, number int) (bool, error) {
						return service.ReserveReviewPRTask(ctx, watch.ID, "acme", "demo", number, "https://git.example.com/acme/demo/pull/7")
					}
				}
				raw, err := json.Marshal(&NewIssueEvent{GitHubHost: connection.GitHubHost, CredentialGeneration: connection.CredentialGeneration, WorkspaceID: "ws-1"})
				if err != nil {
					t.Fatal(err)
				}
				var event NewIssueEvent
				if err := json.Unmarshal(raw, &event); err != nil {
					t.Fatal(err)
				}
				eventCtx := WithGitHubWatchConnection(context.Background(), event.GitHubHost, event.CredentialGeneration)
				admitted, err := reserve(eventCtx, 1)
				if err != nil || !admitted {
					t.Fatalf("reservation control failed: %t, error %v", admitted, err)
				}
				connection.CredentialGeneration++
				if replacement == "host" {
					connection.GitHubHost = "other.example.com"
				}
				if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
					t.Fatal(err)
				}
				admitted, err = reserve(eventCtx, 2)
				if !errors.Is(err, ErrGitHubHostMismatch) || admitted {
					t.Fatalf("stale serialized authority reserved task: %t, error %v", admitted, err)
				}
			})
		}
	}
}

type enterpriseCleanupClient struct {
	NoopClient
	calls             int
	admitted, release chan struct{}
}

func (c *enterpriseCleanupClient) GetPR(ctx context.Context, _, _ string, _ int) (*PR, error) {
	c.calls++
	if c.admitted != nil {
		close(c.admitted)
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &PR{State: prStateClosed}, nil
}
func (c *enterpriseCleanupClient) GetIssueState(ctx context.Context, _, _ string, _ int) (string, error) {
	c.calls++
	if c.admitted != nil {
		close(c.admitted)
		select {
		case <-c.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "closed", nil
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseCleanupRejectsHistoricalHostBeforeProviderAccess(t *testing.T) {
	for _, kind := range []string{"issue", "review"} {
		t.Run(kind, func(t *testing.T) {
			service, store := setupWatchServiceTest(t)
			connection := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com", Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}
			if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
				t.Fatal(err)
			}
			client := &enterpriseCleanupClient{}
			service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
			resolved, err := service.resolveAutomationClient(context.Background(), "ws-1", "", "")
			if err != nil {
				t.Fatal(err)
			}
			ctx := withGitHubOperationFence(context.Background(), service, "ws-1", resolved)
			var should bool
			if kind == "issue" {
				should, _, err = service.shouldDeleteIssueTaskWithClient(ctx, client, &IssueWatchTask{GitHubHost: "github.com", RepoOwner: "acme", RepoName: "demo", IssueNumber: 7}, CleanupPolicyAlways)
			} else {
				should, _, err = service.shouldDeleteReviewTaskWithClient(ctx, client, &ReviewPRTask{GitHubHost: "github.com", RepoOwner: "acme", RepoName: "demo", PRNumber: 7}, CleanupPolicyAlways)
			}
			if !errors.Is(err, ErrGitHubHostMismatch) || should || client.calls != 0 {
				t.Fatalf("historical cleanup reached new provider: delete %t, error %v, calls %d", should, err, client.calls)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterpriseWatchCreationStoresSelectedHost(t *testing.T) {
	for _, kind := range []string{"issue", "review"} {
		t.Run(kind, func(t *testing.T) {
			service, store := setupWatchServiceTest(t)
			if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com", Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}); err != nil {
				t.Fatal(err)
			}
			service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: &enterpriseWatchClient{}})
			if kind == "issue" {
				watch, err := service.CreateIssueWatchForWorkspace(context.Background(), &CreateIssueWatchRequest{WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
				if err != nil || watch.GitHubHost != "git.example.com" {
					t.Fatalf("issue watch authority = %+v, error %v", watch, err)
				}
			} else {
				watch, err := service.CreateReviewWatch(context.Background(), &CreateReviewWatchRequest{WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
				if err != nil || watch.GitHubHost != "git.example.com" {
					t.Fatalf("review watch authority = %+v, error %v", watch, err)
				}
			}
		})
	}
}

type enterprisePersonalCredentialStub struct{}

func (enterprisePersonalCredentialStub) ResolveUser(_ context.Context, c *UserConnection, _ ResolveCredentialRequest) (*ResolvedCredential, error) {
	return &ResolvedCredential{Client: &NoopClient{}, GitHubHost: "git.example.com", Principal: AuthPrincipal{GitHubHost: "git.example.com", Kind: AuthPrincipalHuman, Source: ConnectionSourceGitHubAppUser, Login: c.Login}, Capabilities: allTokenCapabilities()}, nil
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestEnterprisePersonalOperationUsesWorkspaceGeneration(t *testing.T) {
	service, store := setupWatchServiceTest(t)
	connection := &WorkspaceConnection{WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com", Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1}
	if err := store.UpsertWorkspaceConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	reader := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{"ws-1": connection}, users: map[string]*UserConnection{"ws-1:alice": {WorkspaceID: "ws-1", UserID: "alice", AppRegistrationID: "registration", GitHubUserID: 7, Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 7}}}
	service.resolver = NewCredentialResolver(reader, nil)
	service.resolver.SetUserProvider(enterprisePersonalCredentialStub{})
	resolved, err := service.resolvePersonalReadClient(context.Background(), "ws-1", "alice", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.requireCurrentHost(context.Background(), "ws-1", resolved); err != nil {
		t.Fatalf("current personal operation rejected due distinct user generation: %v", err)
	}
	reader.workspaces["ws-1"].CredentialGeneration++
	if err := service.requireCurrentHost(context.Background(), "ws-1", resolved); !errors.Is(err, ErrGitHubHostMismatch) {
		t.Fatalf("old workspace generation accepted: %v", err)
	}
}
