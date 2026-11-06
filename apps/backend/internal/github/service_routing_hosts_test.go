package github

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type replacingHostConnectionReader struct {
	fakeConnectionReader
	initial, replacement WorkspaceConnection
	reads                int
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestPastedResourceURLsFenceHostReplacement(t *testing.T) {
	for _, kind := range []string{"issues", "pull"} {
		for _, replacementHost := range []string{"git.example.com", "other.example.com"} {
			t.Run(kind+"/"+replacementHost, func(t *testing.T) {
				initial := WorkspaceConnection{
					WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com",
					Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1,
				}
				replacement := initial
				replacement.GitHubHost = replacementHost
				replacement.CredentialGeneration++
				connections := &replacingHostConnectionReader{initial: initial, replacement: replacement}
				calls := 0
				providerErr := errors.New("provider reached")
				client := repositoryVisibilityClient{
					Client: &stubClient{
						getPRFunc: func(context.Context, string, string, int) (*PR, error) {
							calls++
							return nil, providerErr
						},
						getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
							calls++
							return nil, providerErr
						},
					},
					repos: map[string]GitHubRepo{"acme/demo": testGitHubRepo("acme", "demo")},
				}
				service := NewService(nil, AuthMethodNone, nil, nil, nil, testLogger(t))
				t.Cleanup(service.Stop)
				service.resolver = NewCredentialResolver(connections, nil)
				service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
				service.SetTaskIssueStore(&fakeTaskIssueStore{task: &taskmodels.Task{ID: "task", WorkspaceID: "ws-1"}})
				resourceURL := "https://git.example.com/acme/demo/" + kind + "/7"
				var err error
				if kind == "issues" {
					_, err = service.LinkTaskIssueForUser(t.Context(), "", "task", LinkTaskIssueRequest{Issue: resourceURL})
				} else {
					err = service.AssociatePRByURLForWorkspace(t.Context(), "ws-1", "", "", "task", "", resourceURL, "")
				}
				wantErr, wantCalls := providerErr, 1
				if replacementHost != initial.GitHubHost {
					wantErr, wantCalls = ErrGitHubHostMismatch, 0
				}
				if !errors.Is(err, wantErr) || calls != wantCalls {
					t.Fatalf("pasted URL: err=%v provider calls=%d; want err=%v calls=%d", err, calls, wantErr, wantCalls)
				}
			})
		}
	}
}

func (r *replacingHostConnectionReader) GetWorkspaceConnection(context.Context, string) (*WorkspaceConnection, error) {
	r.reads++
	connection := r.initial
	if r.reads > 1 {
		connection = r.replacement
	}
	return &connection, nil
}

type hostRoutedPRClient struct {
	NoopClient
	calls int
}

func (c *hostRoutedPRClient) GetPR(_ context.Context, owner, repo string, number int) (*PR, error) {
	c.calls++
	return &PR{RepoOwner: owner, RepoName: repo, Number: number}, nil
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestGetPRForAutomationFencesHostReplacementDuringResolution(t *testing.T) {
	for _, tt := range []struct {
		name, expected, replacement string
		wantMismatch                bool
	}{
		{name: "unchanged", expected: "git.example.com", replacement: "git.example.com"},
		{name: "normalized origin", expected: "https://git.example.com", replacement: "git.example.com"},
		{name: "replacement", expected: "git.example.com", replacement: "other.example.com", wantMismatch: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			initial := WorkspaceConnection{
				WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com",
				Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1,
			}
			replacement := initial
			replacement.GitHubHost = tt.replacement
			replacement.CredentialGeneration++
			connections := &replacingHostConnectionReader{initial: initial, replacement: replacement}
			client := &hostRoutedPRClient{}
			resolver := NewCredentialResolver(connections, nil)
			resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
			service := &Service{resolver: resolver}
			ctx := withExpectedGitHubHost(context.Background(), tt.expected)

			pr, err := service.GetPRForAutomation(ctx, "ws-1", "acme", "demo", 7)
			if tt.wantMismatch {
				if !errors.Is(err, ErrGitHubHostMismatch) || pr != nil || client.calls != 0 {
					t.Fatalf("replacement host used: pr=%+v err=%v provider calls=%d", pr, err, client.calls)
				}
				return
			}
			if err != nil || pr == nil || pr.Number != 7 || client.calls != 1 {
				t.Fatalf("same-host request failed: pr=%+v err=%v provider calls=%d", pr, err, client.calls)
			}
		})
	}
}
