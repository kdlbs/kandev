package executor

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
)

func TestIsContainerizedExecutor(t *testing.T) {
	tests := []struct {
		executorType string
		want         bool
	}{
		{"local_docker", true},
		{"remote_docker", true},
		{"sprites", true},
		{"local", false},
		{"worktree", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.executorType, func(t *testing.T) {
			got := isContainerizedExecutor(tt.executorType)
			if got != tt.want {
				t.Errorf("isContainerizedExecutor(%q) = %v, want %v", tt.executorType, got, tt.want)
			}
		})
	}
}

func TestMethodIDToEnvVar(t *testing.T) {
	tests := []struct {
		methodID string
		want     string
	}{
		{"gh_cli_env", envGitHubToken},
		{"agent:claude_code:env:ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY"},
		{"agent:openai:env:OPENAI_API_KEY", "OPENAI_API_KEY"},
		{"unknown_method", ""},
		{"", ""},
		{"agent:invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.methodID, func(t *testing.T) {
			got := methodIDToEnvVar(tt.methodID)
			if got != tt.want {
				t.Errorf("methodIDToEnvVar(%q) = %q, want %q", tt.methodID, got, tt.want)
			}
		})
	}
}

func TestResolveAuthSecrets(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	// Set up mock secret store
	executor.secretStore = &mockSecretStore{
		secrets: map[string]string{
			"secret-gh": "ghp_testtoken123",
			"secret-ai": "sk-test456",
		},
	}

	tests := []struct {
		name           string
		metadata       map[string]interface{}
		existingEnv    map[string]string
		wantEnv        map[string]string
		wantGHTokenSet bool
	}{
		{
			name:     "no remote_auth_secrets",
			metadata: map[string]interface{}{},
			wantEnv:  map[string]string{},
		},
		{
			name: "gh_cli_env secret resolved",
			metadata: map[string]interface{}{
				"remote_auth_secrets": `{"gh_cli_env": "secret-gh"}`,
			},
			wantEnv: map[string]string{
				envGitHubToken: "ghp_testtoken123",
				envGHToken:     "ghp_testtoken123",
			},
			wantGHTokenSet: true,
		},
		{
			name: "agent env var resolved",
			metadata: map[string]interface{}{
				"remote_auth_secrets": `{"agent:test:env:CUSTOM_KEY": "secret-ai"}`,
			},
			wantEnv: map[string]string{
				"CUSTOM_KEY": "sk-test456",
			},
		},
		{
			name: "skip if already set",
			metadata: map[string]interface{}{
				"remote_auth_secrets": `{"gh_cli_env": "secret-gh"}`,
			},
			existingEnv: map[string]string{
				envGitHubToken: "existing-token",
			},
			wantEnv: map[string]string{
				envGitHubToken: "existing-token", // Not overwritten
			},
		},
		{
			name: "invalid JSON ignored",
			metadata: map[string]interface{}{
				"remote_auth_secrets": `{invalid json}`,
			},
			wantEnv: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &LaunchAgentRequest{
				Env: tt.existingEnv,
			}
			if req.Env == nil {
				req.Env = make(map[string]string)
			}

			executor.resolveAuthSecrets(context.Background(), req, tt.metadata)

			for key, wantVal := range tt.wantEnv {
				if gotVal := req.Env[key]; gotVal != wantVal {
					t.Errorf("env[%q] = %q, want %q", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestResolveGHCLIToken(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	tests := []struct {
		name        string
		metadata    map[string]interface{}
		existingEnv map[string]string
		expectSkip  bool
	}{
		{
			name:       "no remote_credentials",
			metadata:   map[string]interface{}{},
			expectSkip: true,
		},
		{
			name: "gh_cli_token not in list",
			metadata: map[string]interface{}{
				"remote_credentials": `["other_credential"]`,
			},
			expectSkip: true,
		},
		{
			name: "skip if GITHUB_TOKEN already set",
			metadata: map[string]interface{}{
				"remote_credentials": `["gh_cli_token"]`,
			},
			existingEnv: map[string]string{
				envGitHubToken: "existing",
			},
			expectSkip: true,
		},
		{
			name: "invalid JSON ignored",
			metadata: map[string]interface{}{
				"remote_credentials": `{invalid}`,
			},
			expectSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &LaunchAgentRequest{
				Env: tt.existingEnv,
			}
			if req.Env == nil {
				req.Env = make(map[string]string)
			}
			initialToken := req.Env[envGitHubToken]

			executor.resolveGHCLIToken(req, tt.metadata)

			if tt.expectSkip {
				if req.Env[envGitHubToken] != initialToken {
					t.Errorf("expected GITHUB_TOKEN to remain %q, got %q", initialToken, req.Env[envGitHubToken])
				}
			}
		})
	}
}

func TestInjectGitHubToken(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	executor.secretStore = &mockSecretStore{
		secrets: map[string]string{"secret-1": "ghp_globaltoken"},
		names:   map[string]string{"secret-1": envGitHubToken},
	}

	t.Run("injects token when not set", func(t *testing.T) {
		req := &LaunchAgentRequest{Env: make(map[string]string)}
		executor.injectGitHubToken(context.Background(), req)
		if req.Env[envGitHubToken] != "ghp_globaltoken" {
			t.Errorf("expected GITHUB_TOKEN to be set, got %q", req.Env[envGitHubToken])
		}
		if req.Env[envGHToken] != "ghp_globaltoken" {
			t.Errorf("expected GH_TOKEN to be set, got %q", req.Env[envGHToken])
		}
	})

	t.Run("skips if GITHUB_TOKEN already set", func(t *testing.T) {
		req := &LaunchAgentRequest{Env: map[string]string{envGitHubToken: "existing-token"}}
		executor.injectGitHubToken(context.Background(), req)
		if req.Env[envGitHubToken] != "existing-token" {
			t.Errorf("expected GITHUB_TOKEN to remain unchanged, got %q", req.Env[envGitHubToken])
		}
	})

	t.Run("skips if GH_TOKEN already set", func(t *testing.T) {
		req := &LaunchAgentRequest{Env: map[string]string{envGHToken: "existing-gh-token"}}
		executor.injectGitHubToken(context.Background(), req)
		if req.Env[envGitHubToken] != "" {
			t.Errorf("expected GITHUB_TOKEN to not be injected when GH_TOKEN is set, got %q", req.Env[envGitHubToken])
		}
	})
}

func TestInjectGitHubTokenFromCLI(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	t.Run("skips if GITHUB_TOKEN already set", func(t *testing.T) {
		req := &LaunchAgentRequest{Env: map[string]string{envGitHubToken: "existing-token"}}
		executor.injectGitHubTokenFromCLI(context.Background(), req)
		if req.Env[envGitHubToken] != "existing-token" {
			t.Errorf("expected GITHUB_TOKEN to remain unchanged, got %q", req.Env[envGitHubToken])
		}
	})

	t.Run("skips if GH_TOKEN already set", func(t *testing.T) {
		req := &LaunchAgentRequest{Env: map[string]string{envGHToken: "existing-gh-token"}}
		executor.injectGitHubTokenFromCLI(context.Background(), req)
		if req.Env[envGHToken] != "existing-gh-token" {
			t.Errorf("expected GH_TOKEN to remain unchanged, got %q", req.Env[envGHToken])
		}
	})
}

// mockSecretStore implements secrets.SecretStore for testing
type mockSecretStore struct {
	secrets map[string]string // id -> value
	names   map[string]string // id -> name
}

func (m *mockSecretStore) Create(_ context.Context, secret *secrets.SecretWithValue) error {
	if m.secrets == nil {
		m.secrets = make(map[string]string)
	}
	if m.names == nil {
		m.names = make(map[string]string)
	}
	m.secrets[secret.ID] = secret.Value
	m.names[secret.ID] = secret.Name
	return nil
}

func (m *mockSecretStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	if name, ok := m.names[id]; ok {
		return &secrets.Secret{ID: id, Name: name}, nil
	}
	return nil, fmt.Errorf("secret not found")
}

func (m *mockSecretStore) Reveal(_ context.Context, id string) (string, error) {
	if val, ok := m.secrets[id]; ok {
		return val, nil
	}
	return "", fmt.Errorf("secret not found")
}

func (m *mockSecretStore) Update(_ context.Context, id string, req *secrets.UpdateSecretRequest) error {
	if _, ok := m.secrets[id]; !ok {
		return fmt.Errorf("secret not found")
	}
	if req.Value != nil {
		m.secrets[id] = *req.Value
	}
	if req.Name != nil {
		m.names[id] = *req.Name
	}
	return nil
}

func (m *mockSecretStore) Delete(_ context.Context, id string) error {
	delete(m.secrets, id)
	delete(m.names, id)
	return nil
}

func (m *mockSecretStore) List(_ context.Context) ([]*secrets.SecretListItem, error) {
	var items []*secrets.SecretListItem
	for id, name := range m.names {
		items = append(items, &secrets.SecretListItem{
			ID:       id,
			Name:     name,
			HasValue: m.secrets[id] != "",
		})
	}
	return items, nil
}

func (m *mockSecretStore) Close() error {
	return nil
}
