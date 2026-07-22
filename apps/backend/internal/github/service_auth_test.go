package github

import (
	"context"
	"testing"
)

const testWorkspaceID = "workspace-test"

type testConnectionReader struct {
	workspaces map[string]*WorkspaceConnection
}

func (r testConnectionReader) GetWorkspaceConnection(
	_ context.Context,
	workspaceID string,
) (*WorkspaceConnection, error) {
	return r.workspaces[workspaceID], nil
}

func (testConnectionReader) GetUserConnection(
	context.Context,
	string,
	string,
) (*UserConnection, error) {
	return nil, nil
}

type testAutomationCredentialProvider struct {
	client Client
}

func (p testAutomationCredentialProvider) ResolveAutomation(
	_ context.Context,
	connection *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return &ResolvedCredential{
		Client:       p.client,
		Capabilities: allTokenCapabilities(),
		Principal: AuthPrincipal{
			Kind:   AuthPrincipalHuman,
			Source: ConnectionSourcePAT,
			Login:  connection.Login,
		},
	}, nil
}

// configureTestWorkspaceAuth gives legacy service fixtures an explicit,
// workspace-owned PAT principal while preserving their injected mock client.
func configureTestWorkspaceAuth(
	t *testing.T,
	service *Service,
	client Client,
	workspaceIDs ...string,
) {
	t.Helper()
	connections := make(map[string]*WorkspaceConnection, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		connections[workspaceID] = &WorkspaceConnection{
			WorkspaceID:          workspaceID,
			Source:               ConnectionSourcePAT,
			GitHubHost:           defaultGitHubHost,
			Login:                "test-user",
			Status:               ConnectionStatusActive,
			CredentialGeneration: 1,
		}
	}
	service.resolver = NewCredentialResolver(testConnectionReader{workspaces: connections}, nil)
	service.resolver.SetAutomationProvider(testAutomationCredentialProvider{client: client})
}

func newWorkspaceAuthenticatedTestService(
	t *testing.T,
	client Client,
	store *Store,
	workspaceIDs ...string,
) *Service {
	t.Helper()
	service := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	configureTestWorkspaceAuth(t, service, client, workspaceIDs...)
	return service
}

func withTestWorkspace(watch *PRWatch) *PRWatch {
	if watch.WorkspaceID == "" {
		watch.WorkspaceID = testWorkspaceID
	}
	return watch
}

func testAutomationScope(t *testing.T, service *Service, workspaceID string) string {
	t.Helper()
	resolved, err := service.resolveAutomationClient(context.Background(), workspaceID, "", "")
	if err != nil {
		t.Fatalf("resolve test automation client: %v", err)
	}
	return resolved.CacheScope
}
