package github

import (
	"context"
	"errors"
	"testing"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.2
func TestEnterpriseBrokerScope(t *testing.T) {
	for _, host := range []string{"git.company.example", "acme.ghe.com"} {
		t.Run(host, func(t *testing.T) {
			connection := &WorkspaceConnection{WorkspaceID: "workspace-1", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, GitHubHost: host, CredentialGeneration: 7}
			connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{connection.WorkspaceID: connection}}
			resolver := NewCredentialResolver(connections, fakeAuthSecrets{WorkspacePATSecretKeyForHost(connection.WorkspaceID, host): "workspace-secret"})
			broker := NewCredentialBroker(connections, resolver, &fakeBrokerAuthorizer{})
			issue := brokerLeaseRequest()
			issue.Host = host
			lease, err := broker.Issue(context.Background(), issue)
			if err != nil {
				t.Fatalf("issue Enterprise lease: %v", err)
			}
			request := brokerCredentialRequest(lease.Token)
			request.Host = host
			credential, err := broker.Resolve(context.Background(), request)
			if err != nil || credential.Password != "workspace-secret" {
				t.Fatalf("redeem Enterprise lease: %v", err)
			}
			request.Host = "github.com"
			if _, err = broker.Resolve(context.Background(), request); !errors.Is(err, ErrCredentialScopeDenied) {
				t.Fatalf("foreign host error: %v", err)
			}
			connection.GitHubHost = "other.company.example"
			request.Host = host
			if _, err = broker.Resolve(context.Background(), request); err == nil {
				t.Fatal("old host lease survived replacement")
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
func TestEnterpriseInitialCloneCredentialHost(t *testing.T) {
	connection := &WorkspaceConnection{WorkspaceID: "workspace", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, GitHubHost: "git.company.example", CredentialGeneration: 1}
	resolver := NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{"workspace": connection}}, fakeAuthSecrets{WorkspacePATSecretKeyForHost("workspace", connection.GitHubHost): "workspace-secret"})
	service := &Service{resolver: resolver}
	for _, host := range []string{"git.company.example", "other.company.example", "github.com"} {
		_, password, err := service.ResolveGitCredentialForHost(context.Background(), "workspace", "github", host, "acme", "repo")
		if host == connection.GitHubHost {
			if err != nil || password != "workspace-secret" {
				t.Fatalf("same-host clone failed: %v", err)
			}
		} else if err == nil || password != "" {
			t.Fatal("credential returned for foreign clone host")
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
func TestEnterpriseCloneRejectsConnectionReplacementDuringResolution(t *testing.T) {
	connection := &WorkspaceConnection{WorkspaceID: "workspace", Source: ConnectionSourceGHCLI, Status: ConnectionStatusActive, GitHubHost: "git.company.example", Login: "automation", CredentialGeneration: 1}
	resolver := NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{"workspace": connection}}, nil)
	resolver.ghToken = func(context.Context, string, string) (string, error) {
		connection.CredentialGeneration++
		return "old-token", nil
	}
	service := &Service{resolver: resolver}
	_, password, err := service.ResolveGitCredentialForHost(context.Background(), "workspace", "github", "git.company.example", "acme", "repo")
	if err == nil || password != "" {
		t.Fatal("in-flight credential survived connection replacement")
	}
}
