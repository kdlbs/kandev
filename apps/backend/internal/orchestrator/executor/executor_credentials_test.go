package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	envGitHubCredentialBrokerURL  = githubauth.CredentialBrokerURLEnv
	envGitHubCredentialLease      = githubauth.CredentialLeaseEnv
	envGitHubCredentialTaskID     = githubauth.CredentialTaskIDEnv
	envGitHubCredentialSessionID  = githubauth.CredentialSessionIDEnv
	envGitHubCredentialRepository = githubauth.CredentialRepositoryEnv
	envGitHubCredentialOwner      = githubauth.CredentialOwnerEnv
	envGitHubCredentialRepo       = githubauth.CredentialRepoEnv
	envGitHubCredentialHost       = githubauth.CredentialHostEnv
	envGitHubCredentialScopes     = githubauth.CredentialScopesEnv
)

type fakeGitHubCredentialLeaseIssuer struct {
	request  GitHubCredentialLeaseRequest
	requests []GitHubCredentialLeaseRequest
	lease    GitHubCredentialLease
	err      error
	calls    int
}

func (f *fakeGitHubCredentialLeaseIssuer) IssueGitHubCredentialLease(
	_ context.Context,
	req GitHubCredentialLeaseRequest,
) (GitHubCredentialLease, error) {
	f.calls++
	f.request = req
	f.requests = append(f.requests, req)
	return f.lease, f.err
}

func TestConfigureGitHubCredentialBroker(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: GitHubCredentialLease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID:       "task-1",
		WorkspaceID:  "workspace-1",
		SessionID:    "session-1",
		ExecutorType: string(models.ExecutorTypeRemoteDocker),
		Env: map[string]string{
			"GIT_CONFIG_COUNT":   "1",
			"GIT_CONFIG_KEY_0":   "http.version",
			"GIT_CONFIG_VALUE_0": "HTTP/1.1",
		},
	}
	info := &repoInfo{
		RepositoryID: "repo-1",
		Repository: &models.Repository{
			Provider:      "github",
			ProviderOwner: "acme",
			ProviderName:  "widgets",
		},
	}

	err := exec.configureGitHubCredentialBroker(context.Background(), req, info)
	if err != nil {
		t.Fatalf("configureGitHubCredentialBroker() error = %v", err)
	}
	if issuer.calls != 1 {
		t.Fatalf("IssueGitHubCredentialLease calls = %d, want 1", issuer.calls)
	}
	if issuer.request.WorkspaceID != "workspace-1" || issuer.request.RepositoryID != "repo-1" {
		t.Fatalf("lease scope = %+v", issuer.request)
	}
	wantEnv := map[string]string{
		envGitHubCredentialBrokerURL:  "https://kandev.example/api/github/credentials/resolve",
		envGitHubCredentialLease:      "opaque-lease",
		envGitHubCredentialTaskID:     "task-1",
		envGitHubCredentialSessionID:  "session-1",
		envGitHubCredentialRepository: "repo-1",
		envGitHubCredentialOwner:      "acme",
		envGitHubCredentialRepo:       "widgets",
		envGitHubCredentialHost:       "github.com",
		"GIT_CONFIG_COUNT":            "3",
		"GIT_CONFIG_KEY_1":            "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1":          "!agentctl git-credential",
		"GIT_CONFIG_KEY_2":            "credential.useHttpPath",
		"GIT_CONFIG_VALUE_2":          "true",
		"GIT_TERMINAL_PROMPT":         "0",
	}
	if got := req.Env[envGitHubCredentialScopes]; !strings.Contains(got, `"repository_id":"repo-1"`) {
		t.Fatalf("credential scopes = %q, want repository scope", got)
	}
	for key, want := range wantEnv {
		if got := req.Env[key]; got != want {
			t.Errorf("Env[%q] = %q, want %q", key, got, want)
		}
	}
	if _, ok := req.Env[envGitHubToken]; ok {
		t.Error("managed credential configuration exposed GITHUB_TOKEN")
	}
	if _, ok := req.Env[envGHToken]; ok {
		t.Error("managed credential configuration exposed GH_TOKEN")
	}
}

func TestConfigureGitHubCredentialBrokerIssuesOneLeasePerRepository(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeRemoteDocker), Env: map[string]string{},
	}
	infos := []*repoInfo{
		{RepositoryID: "repo-1", Repository: &models.Repository{
			Provider: "github", ProviderOwner: "acme", ProviderName: "frontend",
		}},
		{RepositoryID: "repo-2", Repository: &models.Repository{
			Provider: "github", ProviderOwner: "acme", ProviderName: "backend",
		}},
	}
	issuer.lease = GitHubCredentialLease{Token: "opaque-lease"}

	if err := exec.configureGitHubCredentialBrokerForRepositories(context.Background(), req, infos); err != nil {
		t.Fatalf("configureGitHubCredentialBrokerForRepositories() error = %v", err)
	}
	if len(issuer.requests) != 2 {
		t.Fatalf("lease requests = %d, want 2", len(issuer.requests))
	}
	if issuer.requests[0].RepositoryID != "repo-1" || issuer.requests[1].RepositoryID != "repo-2" {
		t.Fatalf("lease scopes = %+v", issuer.requests)
	}
	var scopes []githubCredentialScope
	if err := json.Unmarshal([]byte(req.Env[envGitHubCredentialScopes]), &scopes); err != nil {
		t.Fatalf("decode credential scopes: %v", err)
	}
	if len(scopes) != 2 || scopes[0].Repo != "frontend" || scopes[1].Repo != "backend" {
		t.Fatalf("credential scopes = %+v", scopes)
	}
	if got := req.Env[envGitHubCredentialRepo]; got != "frontend" {
		t.Fatalf("primary gh scope = %q, want frontend", got)
	}
}

func TestConfigureGitHubCredentialBrokerPreservesExplicitProfileToken(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: GitHubCredentialLease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeRemoteDocker),
		Env:          map[string]string{envGHToken: "profile-token"},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
	}}

	if err := exec.configureGitHubCredentialBroker(context.Background(), req, info); err != nil {
		t.Fatalf("configureGitHubCredentialBroker() error = %v", err)
	}
	if issuer.calls != 0 {
		t.Fatalf("IssueGitHubCredentialLease calls = %d, want 0", issuer.calls)
	}
	if got := req.Env[envGHToken]; got != "profile-token" {
		t.Fatalf("GH_TOKEN = %q, want explicit profile value", got)
	}
	if got := req.Env[envGitHubCredentialLease]; got != "" {
		t.Fatalf("broker lease = %q, want none with explicit profile auth", got)
	}
}

func TestConfigureGitHubCredentialBrokerRejectsRemoteLoopbackURL(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: GitHubCredentialLease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "http://127.0.0.1:8080/api/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeSSH), Env: map[string]string{},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
	}}

	err := exec.configureGitHubCredentialBroker(context.Background(), req, info)
	if err == nil || !errors.Is(err, ErrGitHubCredentialBrokerURL) {
		t.Fatalf("configureGitHubCredentialBroker() error = %v, want broker URL error", err)
	}
	if issuer.calls != 0 {
		t.Fatalf("IssueGitHubCredentialLease calls = %d, want 0", issuer.calls)
	}
}

func TestConfigureGitHubCredentialBrokerAllowsLocalLoopbackURL(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: GitHubCredentialLease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "http://localhost:8080/api/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeWorktree), Env: map[string]string{},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
	}}

	if err := exec.configureGitHubCredentialBroker(context.Background(), req, info); err != nil {
		t.Fatalf("configureGitHubCredentialBroker() error = %v", err)
	}
	if issuer.calls != 1 {
		t.Fatalf("IssueGitHubCredentialLease calls = %d, want 1", issuer.calls)
	}
}

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

func TestExecutorNeedsResolvedCredentials(t *testing.T) {
	tests := []struct {
		executorType string
		want         bool
	}{
		{"local_docker", true},
		{"remote_docker", true},
		{"sprites", true},
		{"ssh", true}, // SSH remotes run agentctl off-host; credentials must reach req.Env
		{"local", false},
		{"worktree", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.executorType, func(t *testing.T) {
			got := executorNeedsResolvedCredentials(tt.executorType)
			if got != tt.want {
				t.Errorf("executorNeedsResolvedCredentials(%q) = %v, want %v", tt.executorType, got, tt.want)
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

func TestApplyContainerCredentialsDoesNotInjectGlobalGitHubToken(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	executor.secretStore = &mockSecretStore{
		secrets: map[string]string{"secret-1": "ghp_globaltoken"},
		names:   map[string]string{"secret-1": envGitHubToken},
	}

	req := &LaunchAgentRequest{Env: make(map[string]string)}
	executor.applyContainerCredentials(context.Background(), req, nil)
	if got := req.Env[envGitHubToken]; got != "" {
		t.Fatalf("GITHUB_TOKEN = %q, want no installation-wide fallback", got)
	}
	if got := req.Env[envGHToken]; got != "" {
		t.Fatalf("GH_TOKEN = %q, want no installation-wide fallback", got)
	}
}

func TestApplyContainerCredentialsDoesNotExtractAmbientGitHubCLIAccount(t *testing.T) {
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\nprintf ambient-host-token\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{Env: make(map[string]string)}

	executor.applyContainerCredentials(context.Background(), req, map[string]interface{}{
		profileKeyRemoteCredentials: `["gh_cli_token"]`,
	})

	if got := req.Env[envGitHubToken]; got != "" {
		t.Fatalf("GITHUB_TOKEN = %q, want no ambient gh credential", got)
	}
	if got := req.Env[envGHToken]; got != "" {
		t.Fatalf("GH_TOKEN = %q, want no ambient gh credential", got)
	}
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
