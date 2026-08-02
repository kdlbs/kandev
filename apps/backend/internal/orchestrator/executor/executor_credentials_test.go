package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

type fakeTaskGitCredentialPolicyResolver struct {
	policy TaskGitCredentialPolicy
	err    error
}

func (r fakeTaskGitCredentialPolicyResolver) ResolveTaskGitCredentialPolicy(
	context.Context,
	string,
) (TaskGitCredentialPolicy, error) {
	return r.policy, r.err
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
		"GIT_CONFIG_COUNT":            "4",
		"GIT_CONFIG_KEY_1":            "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1":          "",
		"GIT_CONFIG_KEY_2":            "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_2":          gitHubCredentialHelper,
		"GIT_CONFIG_KEY_3":            "credential.useHttpPath",
		"GIT_CONFIG_VALUE_3":          "true",
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

func TestApplyGitCredentialSnapshotOmitsManagedSourceWithoutBroker(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: "managed", WorkspaceMethod: "gh_cli"},
	})
	session := &models.TaskSession{ID: "session-1"}
	req := &LaunchAgentRequest{WorkspaceID: "workspace-1", Env: map[string]string{}}

	if err := exec.applyGitCredentialSnapshot(context.Background(), req, session); err != nil {
		t.Fatalf("applyGitCredentialSnapshot() error = %v", err)
	}
	if _, ok := session.Metadata[models.SessionMetaKeyGitCredentialSnapshot]; ok {
		t.Fatalf("snapshot = %#v, want no managed snapshot without broker", session.Metadata)
	}
}

func TestApplyGitCredentialSnapshotUsesExecutorSources(t *testing.T) {
	for _, tc := range []struct {
		name        string
		policy      TaskGitCredentialPolicy
		env         map[string]string
		wantSource  string
		wantActor   string
		wantTransit string
	}{
		{name: "profile token", env: map[string]string{envGHToken: "token"}, wantSource: "executor_profile", wantTransit: "profile_token"},
		{name: "executor inheritance", policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}, env: map[string]string{}, wantSource: "executor", wantTransit: "executor_selected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: tc.policy})
			session := &models.TaskSession{ID: "session-1"}
			if err := exec.applyGitCredentialSnapshot(context.Background(), &LaunchAgentRequest{WorkspaceID: "workspace-1", Env: tc.env}, session); err != nil {
				t.Fatalf("applyGitCredentialSnapshot() error = %v", err)
			}
			snapshot, ok := session.Metadata[models.SessionMetaKeyGitCredentialSnapshot].(models.GitCredentialSnapshot)
			if !ok || snapshot.Source != tc.wantSource || snapshot.Actor != tc.wantActor || snapshot.Transport != tc.wantTransit {
				t.Fatalf("snapshot = %#v", session.Metadata[models.SessionMetaKeyGitCredentialSnapshot])
			}
		})
	}
}

func TestApplyGitCredentialSnapshotUsesManagedWorkspaceIdentity(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: "managed", WorkspaceMethod: "github_app_installation", WorkspaceActor: "acme-bot"},
	})
	session := &models.TaskSession{ID: "session-1"}
	req := &LaunchAgentRequest{WorkspaceID: "workspace-1", Env: map[string]string{githubauth.CredentialBrokerURLEnv: "http://broker"}}

	if err := exec.applyGitCredentialSnapshot(context.Background(), req, session); err != nil {
		t.Fatalf("applyGitCredentialSnapshot() error = %v", err)
	}
	snapshot := session.Metadata[models.SessionMetaKeyGitCredentialSnapshot].(models.GitCredentialSnapshot)
	if snapshot.Source != "workspace" || snapshot.Transport != "managed_https" || snapshot.WorkspaceMethod != "github_app_installation" || snapshot.Actor != "acme-bot" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestApplyGitCredentialSnapshotReturnsResolverError(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{err: errors.New("resolver unavailable")})
	err := exec.applyGitCredentialSnapshot(context.Background(), &LaunchAgentRequest{WorkspaceID: "workspace-1"}, &models.TaskSession{})
	if err == nil || !strings.Contains(err.Error(), "resolver unavailable") {
		t.Fatalf("applyGitCredentialSnapshot() error = %v", err)
	}
}

func TestConfigureGitHubCredentialBrokerHelperSurvivesPathReset(t *testing.T) {
	helperDir := filepath.Join(t.TempDir(), "managed github helper")
	if err := os.MkdirAll(helperDir, 0o700); err != nil {
		t.Fatalf("create helper directory: %v", err)
	}
	helper := filepath.Join(helperDir, "agentctl")
	const helperScript = `#!/bin/sh
if [ "$1" != "git-credential" ] || [ "$2" != "get" ]; then
  exit 2
fi
printf 'username=x-access-token\npassword=fake-token\n'
`
	if err := os.WriteFile(helper, []byte(helperScript), 0o700); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	issuer := &fakeGitHubCredentialLeaseIssuer{lease: GitHubCredentialLease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
	exec.SetAgentctlBinaryPath(helper)
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

	env := make(map[string]string, len(os.Environ())+len(req.Env)+1)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && !strings.HasPrefix(key, "GIT_CONFIG_") && key != githubauth.CredentialCLIShimDirEnv {
			env[key] = value
		}
	}
	for key, value := range req.Env {
		env[key] = value
	}
	env[githubauth.CredentialBrokerURLEnv] = "https://kandev.example/api/github/credentials/resolve"
	env["PATH"] = "/usr/bin:/bin"
	commandEnv := make([]string, 0, len(env))
	for key, value := range env {
		commandEnv = append(commandEnv, key+"="+value)
	}

	command := osExec.Command("git", "credential", "fill")
	command.Env = commandEnv
	command.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets\n\n")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git credential fill failed with PATH reset: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "username=x-access-token") ||
		!strings.Contains(string(output), "password=fake-token") {
		t.Fatalf("credential output = %q, want fake helper credentials", output)
	}
}

func TestConfigureGitHubCredentialBrokerPublishesLocalHelperBeforePreparation(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "managed agentctl")
	for _, executorType := range []models.ExecutorType{models.ExecutorTypeLocal, models.ExecutorTypeWorktree} {
		t.Run(string(executorType), func(t *testing.T) {
			issuer := &fakeGitHubCredentialLeaseIssuer{lease: GitHubCredentialLease{Token: "opaque-lease"}}
			exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
			exec.SetAgentctlBinaryPath(helperPath)
			req := &LaunchAgentRequest{
				TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
				ExecutorType: string(executorType), Env: map[string]string{},
			}
			info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
				Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
			}}

			if err := exec.configureGitHubCredentialBroker(context.Background(), req, info); err != nil {
				t.Fatalf("configureGitHubCredentialBroker() error = %v", err)
			}
			if got := req.Env[githubauth.CredentialHelperPathEnv]; got != helperPath {
				t.Fatalf("credential helper path = %q, want %q before preparation", got, helperPath)
			}
		})
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

func TestConfigureGitHubCredentialBrokerSkipsExecutorInheritedPolicy(t *testing.T) {
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: GitHubCredentialLease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: "executor"},
	})
	req := &LaunchAgentRequest{WorkspaceID: "workspace-1", Env: map[string]string{
		githubauth.CredentialBrokerURLEnv:  "http://broker.example/resolve",
		githubauth.CredentialHelperPathEnv: "/opt/kandev/agentctl",
		githubauth.CredentialLeaseEnv:      "lease",
		githubauth.CredentialTaskIDEnv:     "task-1",
		githubauth.CredentialSessionIDEnv:  "session-1",
		githubauth.CredentialRepositoryEnv: "repo-1",
		githubauth.CredentialOwnerEnv:      "acme",
		githubauth.CredentialRepoEnv:       "widgets",
		githubauth.CredentialHostEnv:       "github.com",
		githubauth.CredentialScopesEnv:     `[{"repo":"widgets"}]`,
		"GIT_CONFIG_COUNT":                 "5",
		"GIT_CONFIG_KEY_0":                 "core.hooksPath",
		"GIT_CONFIG_VALUE_0":               "/work/hooks",
		"GIT_CONFIG_KEY_1":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1":               "",
		"GIT_CONFIG_KEY_2":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_2":               gitHubCredentialHelper,
		"GIT_CONFIG_KEY_3":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_3":               legacyShimGitCredentialHelper,
		"GIT_CONFIG_KEY_4":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_4":               legacyGitHubCredentialHelper,
	}}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
	}}

	if err := exec.configureGitHubCredentialBroker(context.Background(), req, info); err != nil {
		t.Fatalf("configureGitHubCredentialBroker() error = %v", err)
	}
	if issuer.calls != 0 {
		t.Fatalf("IssueGitHubCredentialLease calls = %d, want 0", issuer.calls)
	}
	if got := req.Env[githubauth.CredentialBrokerURLEnv]; got != "" {
		t.Fatalf("broker URL = %q, want none for executor inheritance", got)
	}
	for _, key := range []string{githubauth.CredentialHelperPathEnv, githubauth.CredentialLeaseEnv, githubauth.CredentialTaskIDEnv, githubauth.CredentialSessionIDEnv, githubauth.CredentialRepositoryEnv, githubauth.CredentialOwnerEnv, githubauth.CredentialRepoEnv, githubauth.CredentialHostEnv, githubauth.CredentialScopesEnv} {
		if _, ok := req.Env[key]; ok {
			t.Fatalf("Env[%q] still contains a managed credential value", key)
		}
	}
	if got, want := req.Env["GIT_CONFIG_COUNT"], "1"; got != want {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want %q", got, want)
	}
	if got, want := req.Env["GIT_CONFIG_KEY_0"], "core.hooksPath"; got != want {
		t.Fatalf("GIT_CONFIG_KEY_0 = %q, want %q", got, want)
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

type fakeGitLabCredentialResolver struct {
	byWorkspace map[string]struct{ host, token string }
}

func (f *fakeGitLabCredentialResolver) ResolveGitLabExecutionCredentials(_ context.Context, workspaceID string) (string, string, error) {
	entry, ok := f.byWorkspace[workspaceID]
	if !ok {
		return "", "", fmt.Errorf("not configured")
	}
	return entry.host, entry.token, nil
}

func TestInjectGitLabWorkspaceCredentialsIsolatesWorkspaces(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{
		"workspace-a": {host: "https://gitlab.a.example", token: "token-a"},
		"workspace-b": {host: "http://gitlab.b.internal", token: "token-b"},
	}})

	requestA := &LaunchAgentRequest{WorkspaceID: "workspace-a", Env: map[string]string{envGitLabToken: "stale-token"}}
	exec.injectGitLabWorkspaceCredentials(context.Background(), requestA)
	if requestA.Env[envGitLabToken] != "token-a" || requestA.Env[envKandevGitLabHost] != "https://gitlab.a.example" {
		t.Fatalf("workspace A env = %#v", requestA.Env)
	}
	requestB := &LaunchAgentRequest{WorkspaceID: "workspace-b"}
	exec.injectGitLabWorkspaceCredentials(context.Background(), requestB)
	if requestB.Env[envGitLabToken] != "token-b" || requestB.Env[envKandevGitLabHost] != "http://gitlab.b.internal" {
		t.Fatalf("workspace B env = %#v", requestB.Env)
	}
	if strings.Contains(fmt.Sprint(requestB.Env), "token-a") {
		t.Fatalf("workspace B env leaked workspace A token: %#v", requestB.Env)
	}
}

func TestInjectGitLabWorkspaceCredentialsClearsUnscopedInheritedToken(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{}})
	req := &LaunchAgentRequest{WorkspaceID: "unconfigured", Env: map[string]string{
		envGitLabToken:      "global-token",
		envKandevGitLabHost: "https://wrong.example",
	}}
	exec.injectGitLabWorkspaceCredentials(context.Background(), req)
	if req.Env[envGitLabToken] != "" || req.Env[envKandevGitLabHost] != "" {
		t.Fatalf("unconfigured workspace retained GitLab credentials: %#v", req.Env)
	}
}

func TestInjectGitLabWorkspaceCredentialsAddsExactOriginHelperWithoutToken(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{
		"workspace-a": {host: "http://gitlab.internal:8080", token: "glpat-secret"},
	}})
	req := &LaunchAgentRequest{WorkspaceID: "workspace-a", Env: map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "safe.directory",
		"GIT_CONFIG_VALUE_0": "*",
	}}

	exec.injectGitLabWorkspaceCredentials(context.Background(), req)

	if got := req.Env["GIT_CONFIG_KEY_1"]; got != "credential.http://gitlab.internal:8080.helper" {
		t.Fatalf("credential helper key = %q", got)
	}
	if got := req.Env["GIT_CONFIG_VALUE_1"]; got != gitLabCredentialHelper {
		t.Fatalf("credential helper = %q", got)
	}
	if strings.Contains(req.Env["GIT_CONFIG_VALUE_1"], "glpat-secret") {
		t.Fatal("credential helper persisted the token")
	}
}

func TestStartAgentOnExistingWorkspaceRefreshesWorkspaceGitLabCredentials(t *testing.T) {
	var captured map[string]string
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "execution-1", nil },
		setExecutionEnvFunc: func(_ context.Context, _ string, env map[string]string) error {
			captured = cloneStringMap(env)
			return nil
		},
	}
	repo := newMockRepository()
	exec := newTestExecutor(t, manager, repo)
	exec.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{
		"workspace-a": {host: "https://gitlab.a.example", token: "prepared-token"},
	}})
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-a"}
	session := &models.TaskSession{ID: "session-1", TaskID: task.ID, AgentProfileID: "profile-1"}
	repo.sessions[session.ID] = session

	_, _ = exec.startAgentOnExistingWorkspace(context.Background(), task, session, "prompt", true, "", map[string]string{
		envGitLabToken: "stale-token",
	})
	if captured[envGitLabToken] != "prepared-token" || captured[envKandevGitLabHost] != "https://gitlab.a.example" {
		t.Fatalf("prepared workspace env = %#v", captured)
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
