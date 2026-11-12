package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const claudeAdmissionTestValue = "sk-ant-backendapp-synthetic"

type claudeAdmissionFixture struct {
	validator taskAgentExecutorCompatibilityValidator
	store     secrets.SecretStore
	actorCtx  context.Context
}

func newClaudeAdmissionFixture(t *testing.T) claudeAdmissionFixture {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.OpenSQLite(filepath.Join(dir, "secrets.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	crypto, err := secrets.NewMasterKeyProvider(dir)
	require.NoError(t, err)
	raw, cleanup, err := secrets.Provide(sqlxDB, sqlxDB, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	claude := agents.NewClaudeACP()
	return claudeAdmissionFixture{
		validator: taskAgentExecutorCompatibilityValidator{
			profiles:      compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: claude.ID()}},
			agentRegistry: newCompatibilityRegistry(t, claude),
			secrets:       secrets.NewUserVisibleStore(raw),
		},
		store:    secrets.NewUserVisibleStore(raw),
		actorCtx: authn.WithIdentity(context.Background(), authn.Identity{UserID: "alice", Role: authn.RoleMember}),
	}
}

func (f claudeAdmissionFixture) createSecret(t *testing.T, ctx context.Context, value string) string {
	t.Helper()
	secret := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "claude key"}, Value: value}
	require.NoError(t, f.store.Create(ctx, secret))
	return secret.ID
}

func (f claudeAdmissionFixture) validate(
	ctx context.Context,
	profileEnv []settingsmodels.ProfileEnvVar,
	config map[string]string,
) error {
	return f.validator.ValidateAgentProfileForExecutor(
		ctx,
		&settingsmodels.AgentProfile{ID: "claude-profile", AgentID: "agent-row", EnvVars: profileEnv},
		&models.Executor{ID: "ssh", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive},
		&models.ExecutorProfile{ID: "ssh-profile", ExecutorID: "ssh", Config: config},
	)
}

func TestClaudeWorkflowAdmission(t *testing.T) {
	fixture := newClaudeAdmissionFixture(t)
	secretID := fixture.createSecret(t, fixture.actorCtx, claudeAdmissionTestValue)
	profileEnv := []settingsmodels.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", SecretID: secretID}}

	t.Run("authorized profile api key is admitted", func(t *testing.T) {
		require.NoError(t, fixture.validate(fixture.actorCtx, profileEnv, nil))
	})

	t.Run("executor api key method is admitted", func(t *testing.T) {
		config := map[string]string{
			"remote_auth_secrets": `{"agent:claude-acp:env:ANTHROPIC_API_KEY":"` + secretID + `"}`,
		}
		require.NoError(t, fixture.validate(fixture.actorCtx, nil, config))
	})

	t.Run("deletion after a successful check is denied on retry", func(t *testing.T) {
		deletableID := fixture.createSecret(t, fixture.actorCtx, claudeAdmissionTestValue)
		env := []settingsmodels.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", SecretID: deletableID}}
		require.NoError(t, fixture.validate(fixture.actorCtx, env, nil))
		require.NoError(t, fixture.store.Delete(fixture.actorCtx, deletableID))
		err := fixture.validate(fixture.actorCtx, env, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), deletableID)
	})

	t.Run("missing actor is denied before credential reads", func(t *testing.T) {
		err := fixture.validate(context.Background(), profileEnv, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), secretID)
		require.NotContains(t, err.Error(), claudeAdmissionTestValue)
	})

	t.Run("different owner is denied", func(t *testing.T) {
		bob := authn.WithIdentity(context.Background(), authn.Identity{UserID: "bob", Role: authn.RoleMember})
		err := fixture.validate(bob, profileEnv, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), secretID)
	})

	t.Run("invalid selected oauth binding denies a valid key", func(t *testing.T) {
		config := map[string]string{
			"remote_auth_secrets": `{"agent:claude-acp:env:CLAUDE_CODE_OAUTH_TOKEN":"missing-oauth"}`,
		}
		require.Error(t, fixture.validate(fixture.actorCtx, profileEnv, config))
	})

	t.Run("broken unrelated required binding denies a valid key", func(t *testing.T) {
		env := append([]settingsmodels.ProfileEnvVar{}, profileEnv...)
		env = append(env, settingsmodels.ProfileEnvVar{Key: "HTTP_PROXY", SecretID: "missing-proxy"})
		require.Error(t, fixture.validate(fixture.actorCtx, env, nil))
	})

	t.Run("empty profile configuration is denied", func(t *testing.T) {
		require.Error(t, fixture.validate(fixture.actorCtx, nil, nil))
	})

	t.Run("malformed executor configuration is denied", func(t *testing.T) {
		require.Error(t, fixture.validate(fixture.actorCtx, profileEnv, map[string]string{"remote_credentials": `{`}))
	})

	t.Run("local execution keeps its existing contract", func(t *testing.T) {
		err := fixture.validator.ValidateAgentProfileForExecutor(
			context.Background(),
			&settingsmodels.AgentProfile{ID: "local-profile", AgentID: "agent-row"},
			&models.Executor{ID: "local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive},
			nil,
		)
		require.NoError(t, err)
	})
}
