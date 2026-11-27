package executor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-RESUME-REMOTE-AUTH-001.1
// @covers AC-EXECUTORS-RESUME-REMOTE-AUTH-001.2
// @covers AC-EXECUTORS-RESUME-REMOTE-AUTH-001.3
func TestKubernetesResumeResolvesSelectedRemoteCredentials(t *testing.T) {
	for _, mode := range []string{taskGitCredentialsModeExecutor, taskGitCredentialsModeManaged} {
		t.Run(mode, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			attachManagedGitHubRepositoryForResume(t, repo)
			session := repo.sessions["sess-1"]
			session.ExecutorID = "repointed-executor"
			session.ExecutorProfileID = "repointed-profile"
			session.Metadata = map[string]interface{}{
				profileKeyRemoteAuthSecrets: `{"gh_cli_env":"secret-gh","agent:test:env:CUSTOM_KEY":"secret-agent"}`,
			}
			recorded := recordedKubernetesResumeMetadata()
			repo.executors["recorded-kubernetes-executor"] = &models.Executor{
				ID: "recorded-kubernetes-executor", Type: models.ExecutorTypeKubernetes,
				Config: map[string]string{"auth_mode": "kubeconfig", "kubeconfig_path": "/tmp/test-config", "namespace": "kandev-agents"},
			}
			repo.executorsRunning[session.ID] = &models.ExecutorRunning{
				ID: session.ID, SessionID: session.ID, TaskID: session.TaskID,
				ExecutorID: "recorded-kubernetes-executor", Runtime: agentruntime.RuntimeKubernetes, Metadata: recorded,
			}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.secretStore = &mockSecretStore{secrets: map[string]string{
				"secret-gh": "selected-github-token", "secret-agent": "selected-agent-token",
			}}
			exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
				policy: TaskGitCredentialPolicy{Mode: mode},
			})
			issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "unused-lease"}}
			exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/git/credentials/resolve")

			req := buildRemoteCredentialResumeRequest(t, t.Context(), exec, session)
			require.Equal(t, "selected-github-token", req.Env[envGitHubToken])
			require.Equal(t, "selected-github-token", req.Env[envGHToken])
			require.Equal(t, "selected-agent-token", req.Env["CUSTOM_KEY"])
			resolved, _, err := runtimeenv.Resolve(t.Context(), req.EnvironmentDefinitions, nil)
			require.NoError(t, err)
			require.Equal(t, req.Env[envGitHubToken], resolved[envGitHubToken])
			require.Equal(t, req.Env[envGHToken], resolved[envGHToken])
			require.Equal(t, req.Env["CUSTOM_KEY"], resolved["CUSTOM_KEY"])
			require.Empty(t, req.Env[githubauth.CredentialLeaseEnv])
			require.Zero(t, issuer.calls)
			require.Equal(t, string(models.ExecutorTypeKubernetes), req.ExecutorType)
			require.Equal(t, "recorded-kubernetes-executor", session.ExecutorID)
			require.Equal(t, "recorded-profile", session.ExecutorProfileID)
			for key, value := range recorded {
				require.Equal(t, value, req.Metadata[key], "recorded metadata %s", key)
			}
			require.Equal(t, session.Metadata[profileKeyRemoteAuthSecrets], req.Metadata[profileKeyRemoteAuthSecrets])
		})
	}
}

// @covers AC-EXECUTORS-RESUME-REMOTE-AUTH-001.1
// @covers AC-EXECUTORS-RESUME-REMOTE-AUTH-001.3
func TestResumeRemoteCredentialEligibility(t *testing.T) {
	for _, executorType := range []models.ExecutorType{
		models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker,
		models.ExecutorTypeSprites, models.ExecutorTypeSSH,
		models.ExecutorTypeLocal, models.ExecutorTypeWorktree,
	} {
		t.Run(string(executorType), func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			session := repo.sessions["sess-1"]
			session.ExecutorID = "executor-1"
			session.Metadata = map[string]interface{}{profileKeyRemoteAuthSecrets: `{"agent:test:env:CUSTOM_KEY":"secret-agent"}`}
			repo.executors[session.ExecutorID] = &models.Executor{ID: session.ExecutorID, Type: executorType}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.secretStore = &mockSecretStore{secrets: map[string]string{"secret-agent": "selected-agent-token"}}

			req := buildRemoteCredentialResumeRequest(t, t.Context(), exec, session)
			if executorType == models.ExecutorTypeLocal || executorType == models.ExecutorTypeWorktree {
				require.Empty(t, req.Env["CUSTOM_KEY"])
				return
			}
			require.Equal(t, "selected-agent-token", req.Env["CUSTOM_KEY"])
		})
	}
}

// @covers AC-EXECUTORS-RESUME-REMOTE-AUTH-001.3
func TestResumeRemoteCredentialsRespectSecretVisibility(t *testing.T) {
	store := newResumeCredentialStore(t)
	owner := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	other := authn.WithIdentity(t.Context(), authn.Identity{UserID: "other", Role: authn.RoleMember})
	selected := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "resume-token"}, Value: "owner-token"}
	require.NoError(t, store.Create(owner, selected))
	workspaceSecret := &secrets.SecretWithValue{
		Secret: secrets.Secret{Name: "workspace-token", Scope: secrets.ScopeWorkspace, WorkspaceID: "workspace-1"},
		Value:  "workspace-only-token",
	}
	require.NoError(t, store.Create(owner, workspaceSecret))
	for _, tc := range []struct {
		name     string
		ctx      context.Context
		secretID string
		want     string
	}{
		{name: "owner", ctx: owner, secretID: selected.ID, want: "owner-token"},
		{name: "another user", ctx: other, secretID: selected.ID},
		{name: "missing secret", ctx: owner, secretID: "missing"},
		{name: "workspace secret", ctx: owner, secretID: workspaceSecret.ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			session := repo.sessions["sess-1"]
			session.ExecutorID = "executor-1"
			session.Metadata = map[string]interface{}{profileKeyRemoteAuthSecrets: `{"gh_cli_env":"` + tc.secretID + `"}`}
			repo.executors[session.ExecutorID] = &models.Executor{ID: session.ExecutorID, Type: models.ExecutorTypeSSH}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.secretStore = store

			req := buildRemoteCredentialResumeRequest(t, tc.ctx, exec, session)
			require.Equal(t, tc.want, req.Env[envGitHubToken])
			require.Equal(t, tc.want, req.Env[envGHToken])
		})
	}
}

func buildRemoteCredentialResumeRequest(t *testing.T, ctx context.Context, exec *Executor, session *models.TaskSession) *LaunchAgentRequest {
	t.Helper()
	req, _, _, _, _, err := exec.buildResumeRequest(ctx, &v1.Task{ID: session.TaskID, WorkspaceID: "workspace-1"}, session, true)
	require.NoError(t, err)
	return req
}

func newResumeCredentialStore(t *testing.T) secrets.SecretStore {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.OpenSQLite(filepath.Join(dir, "secrets.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	crypto, err := secrets.NewMasterKeyProvider(dir)
	require.NoError(t, err)
	store, cleanup, err := secrets.Provide(database, database, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	return secrets.NewUserVisibleStore(store)
}
