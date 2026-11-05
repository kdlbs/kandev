package github

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type hostBoundaryWatchClient struct {
	repositoryVisibilityClient
	calls atomic.Int32
}

var errHostBoundaryProvider = errors.New("host boundary provider reached")

func (c *hostBoundaryWatchClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	c.calls.Add(1)
	return nil, errHostBoundaryProvider
}

func (c *hostBoundaryWatchClient) FindPRByBranch(context.Context, string, string, string) (*PR, error) {
	c.calls.Add(1)
	return nil, errHostBoundaryProvider
}

func (c *hostBoundaryWatchClient) ExecuteGraphQL(context.Context, string, map[string]any, any) error {
	c.calls.Add(1)
	return errHostBoundaryProvider
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestWatchAndBatchHostReplacementDuringResolution(t *testing.T) {
	for _, operation := range []string{"watch", "detection", "status sync", "watch batch", "status batch"} {
		for _, replacementHost := range []string{"git.example.com", "other.example.com"} {
			t.Run(operation+"/"+replacementHost, func(t *testing.T) {
				service, store := setupWatchServiceTest(t)
				t.Cleanup(service.Stop)
				initial := WorkspaceConnection{
					WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com",
					Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1,
				}
				replacement := initial
				replacement.GitHubHost = replacementHost
				replacement.CredentialGeneration++
				service.resolver = NewCredentialResolver(&replacingHostConnectionReader{initial: initial, replacement: replacement}, nil)
				client := &hostBoundaryWatchClient{repositoryVisibilityClient: repositoryVisibilityClient{
					Client: &NoopClient{}, repos: map[string]GitHubRepo{"acme/demo": testGitHubRepo("acme", "demo")},
				}}
				service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
				watch := &PRWatch{ID: "watch", WorkspaceID: "ws-1", GitHubHost: initial.GitHubHost, Owner: "acme", Repo: "demo", PRNumber: 7, Branch: "feature"}
				if createErr := store.CreatePRWatch(t.Context(), watch); createErr != nil {
					t.Fatalf("seed PR watch: %v", createErr)
				}
				var err error
				switch operation {
				case "watch":
					_, _, err = service.CheckPRWatchForWorkspace(t.Context(), watch)
				case "detection":
					watch.PRNumber = 0
					_, err = service.triggerPRDetectionWithOptions(t.Context(), watch, "task", true)
				case "status sync":
					_, err = service.triggerPRStatusSyncWithOptions(t.Context(), watch, "task", true)
				case "watch batch":
					_, err = service.SyncWorkspaceWatchesBatched(t.Context(), "ws-1", []*PRWatch{watch})
				case "status batch":
					_, err = service.GetPRStatusesBatchForWorkspace(t.Context(), "ws-1", "", []PRRef{{ExpectedHost: initial.GitHubHost, Owner: "acme", Repo: "demo", Number: 7}})
				}
				if replacementHost != initial.GitHubHost {
					if !errors.Is(err, ErrGitHubHostMismatch) || client.calls.Load() != 0 {
						t.Fatalf("replacement host used: err=%v provider calls=%d", err, client.calls.Load())
					}
				} else if client.calls.Load() == 0 {
					t.Fatalf("same-host control did not reach provider: %v", err)
				}
			})
		}
	}
}

func TestPermanentlyMissingWatchesKeepTheirHost(t *testing.T) {
	service := NewService(nil, AuthMethodNone, nil, nil, nil, testLogger(t))
	t.Cleanup(service.Stop)
	connection := &WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "other.example.com",
		Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 2,
	}
	service.resolver = NewCredentialResolver(&fakeConnectionReader{
		workspaces: map[string]*WorkspaceConnection{"ws-1": connection},
	}, nil)
	service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: &NoopClient{}})
	resolved, err := service.resolveAutomationClient(t.Context(), "ws-1", "acme", "demo")
	if err != nil {
		t.Fatal(err)
	}
	service.markRepoAsMissingForScope(resolved.CacheScope, "acme", "demo", service.repoErrorGenSnapshot())
	for _, host := range []string{"git.example.com", connection.GitHubHost} {
		watch := &PRWatch{WorkspaceID: "ws-1", GitHubHost: host, Owner: "acme", Repo: "demo"}
		got := service.areAllWatchesPermanentlyMissing(t.Context(), []*PRWatch{watch})
		if want := host == connection.GitHubHost; got != want {
			t.Fatalf("host=%s permanently missing=%v, want %v", host, got, want)
		}
	}
}
