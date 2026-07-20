package github

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockAuthState supplies workspace-scoped principals while delegating API
// behavior to the existing shared MockClient data set.
type MockAuthState struct {
	mu           sync.RWMutex
	client       *MockClient
	capabilities map[string]map[GitHubAppCapability]bool
	cliAccounts  []GHAccount
}

func NewMockAuthState(client *MockClient) *MockAuthState {
	return &MockAuthState{client: client, capabilities: make(map[string]map[GitHubAppCapability]bool)}
}

func (m *MockAuthState) ResolveAutomation(
	_ context.Context,
	connection *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if m == nil || m.client == nil || connection == nil {
		return nil, ErrGitHubNotConfigured
	}
	login := connection.Login
	kind := AuthPrincipalHuman
	installationID := int64(0)
	if connection.Source == ConnectionSourceGitHubAppInstallation {
		kind = AuthPrincipalApp
		login = connection.InstallationAccountLogin
		if connection.InstallationID != nil {
			installationID = *connection.InstallationID
		}
	} else if connection.Source == ConnectionSourceLegacyShared && login == "" {
		login, _ = m.client.GetAuthenticatedUser(context.Background())
	}
	return &ResolvedCredential{
		Client: &mockPrincipalClient{MockClient: m.client, login: login},
		Principal: AuthPrincipal{
			Kind: kind, Source: connection.Source, Login: login, InstallationID: installationID,
		},
		Capabilities: m.workspaceCapabilities(connection.WorkspaceID),
		RateTracker:  NewRateTracker(nil, nil),
		credential:   "mock-token:" + connection.WorkspaceID,
	}, nil
}

func (m *MockAuthState) ResolveUser(
	_ context.Context,
	connection *UserConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	if m == nil || m.client == nil || connection == nil {
		return nil, ErrGitHubPersonalRequired
	}
	return &ResolvedCredential{
		Client: &mockPrincipalClient{MockClient: m.client, login: connection.Login},
		Principal: AuthPrincipal{
			Kind: AuthPrincipalHuman, Source: ConnectionSourceGitHubAppUser,
			Login: connection.Login, UserID: connection.UserID,
		},
		Capabilities: allTokenCapabilities(),
		RateTracker:  NewRateTracker(nil, nil),
		credential:   "mock-user-token:" + connection.WorkspaceID + ":" + connection.UserID,
	}, nil
}

func (m *MockAuthState) SetWorkspaceCapabilities(
	workspaceID string,
	capabilities map[GitHubAppCapability]bool,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capabilities[workspaceID] = cloneCapabilities(capabilities)
}

func (m *MockAuthState) DeleteWorkspace(workspaceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.capabilities, workspaceID)
}

func (m *MockAuthState) workspaceCapabilities(workspaceID string) map[GitHubAppCapability]bool {
	m.mu.RLock()
	capabilities, exists := m.capabilities[workspaceID]
	m.mu.RUnlock()
	if !exists {
		return allTokenCapabilities()
	}
	return cloneCapabilities(capabilities)
}

func (m *MockAuthState) SetCLIAccounts(accounts []GHAccount) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cliAccounts = append([]GHAccount(nil), accounts...)
}

func (m *MockAuthState) ListCLIAccounts(context.Context) ([]GHAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]GHAccount(nil), m.cliAccounts...), nil
}

func (m *MockAuthState) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capabilities = make(map[string]map[GitHubAppCapability]bool)
	m.cliAccounts = nil
}

func cloneCapabilities(source map[GitHubAppCapability]bool) map[GitHubAppCapability]bool {
	result := make(map[GitHubAppCapability]bool, len(source))
	for capability, allowed := range source {
		result[capability] = allowed
	}
	return result
}

// mockPrincipalClient overrides identity while sharing every seeded GitHub
// object with the installation-global MockClient.
type mockPrincipalClient struct {
	*MockClient
	login string
}

func (c *mockPrincipalClient) IsAuthenticated(context.Context) (bool, error) { return true, nil }

func (c *mockPrincipalClient) GetAuthenticatedUser(context.Context) (string, error) {
	return c.login, nil
}

func (s *Service) enableMockAuth(client *MockClient) *MockAuthState {
	if s.mockAuth != nil {
		return s.mockAuth
	}
	state := NewMockAuthState(client)
	s.mockAuth = state
	s.ghAccountLister = state.ListCLIAccounts
	if s.resolver != nil {
		s.resolver.SetAutomationProvider(state)
		s.resolver.SetUserProvider(state)
		s.resolver.ghToken = func(_ context.Context, host, login string) (string, error) {
			if strings.TrimSpace(login) == "" {
				return "", fmt.Errorf("GitHub login is required")
			}
			return "mock-gh:" + host + ":" + login, nil
		}
	}
	s.tokenClientFactory = func(token string) Client {
		login := strings.TrimSpace(strings.TrimPrefix(token, "mock-pat:"))
		if strings.HasPrefix(token, "mock-gh:") {
			parts := strings.Split(token, ":")
			login = parts[len(parts)-1]
		}
		if login == "" || login == token {
			login, _ = client.GetAuthenticatedUser(context.Background())
		}
		return &mockPrincipalClient{MockClient: client, login: login}
	}
	return state
}

func (s *Service) setMockAppAvailable(available bool) {
	s.mu.Lock()
	s.appAvailable = available
	s.mu.Unlock()
}

func (s *Service) setMockDeploymentAppStatus(status *DeploymentAppRegistrationStatus) {
	s.mu.Lock()
	if status == nil {
		s.mockDeploymentAppStatus = nil
	} else {
		cloned := cloneDeploymentAppRegistrationStatus(*status)
		s.mockDeploymentAppStatus = &cloned
	}
	s.mu.Unlock()
}

// ResetMockAuth clears in-memory mock identity state for one workspace, or
// all workspaces when workspaceID is empty.
func (s *Service) ResetMockAuth(workspaceID string) {
	if s == nil {
		return
	}
	s.setMockDeploymentAppStatus(nil)
	s.setMockAppAvailable(false)
	if s.mockAuth == nil {
		return
	}
	if workspaceID == "" {
		s.mockAuth.Reset()
	} else {
		s.mockAuth.DeleteWorkspace(workspaceID)
		s.mockAuth.SetCLIAccounts(nil)
	}
	if s.resolver != nil {
		if workspaceID == "" {
			s.resolver.InvalidateAll()
		} else {
			s.resolver.InvalidateWorkspace(workspaceID)
		}
	}
}
