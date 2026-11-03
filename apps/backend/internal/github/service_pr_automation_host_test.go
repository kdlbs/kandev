package github

import (
	"context"
	"testing"
)

type sharedPRClient struct {
	NoopClient
	pr *PR
}

func (c *sharedPRClient) GetPR(context.Context, string, string, int) (*PR, error) {
	return c.pr, nil
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1
func TestGetPRForAutomationStampsResolvedHost(t *testing.T) {
	connection := WorkspaceConnection{
		WorkspaceID: "ws-1", Source: ConnectionSourcePAT, GitHubHost: "git.example.com:8443",
		Login: "alice", Status: ConnectionStatusActive, CredentialGeneration: 1,
	}
	connections := &replacingHostConnectionReader{initial: connection, replacement: connection}
	shared := &PR{RepoOwner: "acme", RepoName: "demo", Number: 7}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(enterpriseWatchCredentials{client: &sharedPRClient{pr: shared}})
	service := &Service{resolver: resolver}

	pr, err := service.GetPRForAutomation(context.Background(), "ws-1", "acme", "demo", 7)
	if err != nil {
		t.Fatalf("GetPRForAutomation() error: %v", err)
	}
	if pr.GitHubHost != "git.example.com:8443" || pr.Number != 7 {
		t.Fatalf("PR = %+v, want GitHubHost git.example.com:8443", pr)
	}
	if pr == shared || shared.GitHubHost != "" {
		t.Fatalf("GetPRForAutomation() mutated the provider-returned PR: %+v", shared)
	}
}
