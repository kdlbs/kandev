package remoteauth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const admissionTestSecretValue = "sk-ant-synthetic-value"

func admissionTestActor() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
}

func newAdmissionSecretStore(t *testing.T) secrets.SecretStore {
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
	return secrets.NewUserVisibleStore(raw)
}

func createAdmissionSecret(t *testing.T, store secrets.SecretStore, ctx context.Context, scope secrets.SecretScope, value string) string {
	t.Helper()
	secret := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "claude credential", Scope: scope}, Value: value}
	if scope == secrets.ScopeWorkspace {
		secret.WorkspaceID = "workspace-1"
	}
	require.NoError(t, store.Create(ctx, secret))
	return secret.ID
}

func claudeCatalog() Catalog {
	return BuildCatalogForHost([]agents.Agent{agents.NewClaudeACP()}, "linux", "")
}

func claudeInput(profileEnv []models.ProfileEnvVar, config map[string]string, store secrets.SecretStore) AdmissionInput {
	return AdmissionInput{
		AgentID:        "claude-acp",
		ProfileEnvVars: profileEnv,
		ExecutorConfig: config,
		Catalog:        claudeCatalog(),
		SecretStore:    store,
	}
}

func apiKeyProfileEnv(secretID string) []models.ProfileEnvVar {
	return []models.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", SecretID: secretID}}
}

func TestRemoteAuthAdmissionAcceptsAuthorizedProfileAPIKey(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)

	err := ValidateCredentialAdmission(ctx, claudeInput(apiKeyProfileEnv(secretID), nil, store))

	require.NoError(t, err)
}

func TestRemoteAuthAdmissionAcceptsExecutorAPIKeyMethod(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	config := map[string]string{
		"remote_auth_secrets": `{"agent:claude-acp:env:ANTHROPIC_API_KEY":"` + secretID + `"}`,
	}

	err := ValidateCredentialAdmission(ctx, claudeInput(nil, config, store))

	require.NoError(t, err)
}

func TestRemoteAuthAdmissionAcceptsSelectedEnvMethodInCredentialList(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	config := map[string]string{
		"remote_credentials":  `["agent:claude-acp:env:ANTHROPIC_API_KEY"]`,
		"remote_auth_secrets": `{"agent:claude-acp:env:ANTHROPIC_API_KEY":"` + secretID + `"}`,
	}

	err := ValidateCredentialAdmission(ctx, claudeInput(nil, config, store))

	require.NoError(t, err)
}

func TestRemoteAuthAdmissionAcceptsLiteralProfileAPIKey(t *testing.T) {
	err := ValidateCredentialAdmission(
		context.Background(),
		claudeInput([]models.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", Value: admissionTestSecretValue}}, nil, nil),
	)

	require.NoError(t, err)
}

func TestRemoteAuthAdmissionIgnoresEmptyDeclarations(t *testing.T) {
	err := ValidateCredentialAdmission(
		context.Background(),
		claudeInput([]models.ProfileEnvVar{
			{},
			{Key: "UNFILLED_ROW"},
			{Key: "ANTHROPIC_API_KEY", Value: admissionTestSecretValue},
		}, nil, nil),
	)

	require.NoError(t, err)
}

func TestRemoteAuthAdmissionAcceptsIndexedGitEnvironment(t *testing.T) {
	apiKey := models.ProfileEnvVar{Key: "ANTHROPIC_API_KEY", Value: admissionTestSecretValue}
	tests := []struct {
		name     string
		profile  []models.ProfileEnvVar
		executor []models.ProfileEnvVar
	}{
		{
			name:    "zero count",
			profile: []models.ProfileEnvVar{apiKey, {Key: "GIT_CONFIG_COUNT", Value: "0"}},
		},
		{
			name: "ignored leftover indexes",
			profile: []models.ProfileEnvVar{
				apiKey,
				{Key: "GIT_CONFIG_COUNT", Value: "1"},
				{Key: "GIT_CONFIG_KEY_0", Value: "http.extraHeader"},
				{Key: "GIT_CONFIG_VALUE_0", Value: "profile-value"},
				{Key: "GIT_CONFIG_KEY_7", Value: "ignored"},
				{Key: "GIT_CONFIG_VALUE_7", Value: "ignored"},
			},
		},
		{
			name: "composed profile and executor blocks",
			profile: []models.ProfileEnvVar{
				apiKey,
				{Key: "GIT_CONFIG_COUNT", Value: "1"},
				{Key: "GIT_CONFIG_KEY_0", Value: "http.extraHeader"},
				{Key: "GIT_CONFIG_VALUE_0", Value: "profile-value"},
			},
			executor: []models.ProfileEnvVar{
				{Key: "GIT_CONFIG_COUNT", Value: "1"},
				{Key: "GIT_CONFIG_KEY_0", Value: "credential.helper"},
				{Key: "GIT_CONFIG_VALUE_0", Value: "executor-value"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := claudeInput(tc.profile, nil, nil)
			input.ExecutorEnvVars = tc.executor
			require.NoError(t, ValidateCredentialAdmission(context.Background(), input))
		})
	}
}

func TestRemoteAuthAdmissionRejectsPaddedReferences(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	methodID := "agent:claude-acp:env:ANTHROPIC_API_KEY"

	tests := []struct {
		name   string
		config map[string]string
	}{
		{name: "padded secret id", config: map[string]string{
			"remote_auth_secrets": `{"` + methodID + `":" ` + secretID + ` "}`,
		}},
		{name: "padded method id", config: map[string]string{
			"remote_auth_secrets": `{" ` + methodID + ` ":"` + secretID + `"}`,
		}},
		{name: "padded file method id", config: map[string]string{
			"remote_credentials": `[" agent:claude-acp:files:0 "]`,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCredentialAdmission(ctx, claudeInput(apiKeyProfileEnv(secretID), tc.config, store))
			require.ErrorIs(t, err, ErrAdmissionRefused)
			require.NotContains(t, err.Error(), secretID)
			require.NotContains(t, err.Error(), admissionTestSecretValue)
		})
	}
}

func TestRemoteAuthAdmissionRejectsCanceledLiteralAndFileRoutes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("literal route", func(t *testing.T) {
		err := ValidateCredentialAdmission(ctx, claudeInput(
			[]models.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", Value: admissionTestSecretValue}}, nil, nil,
		))
		require.ErrorIs(t, err, ErrAdmissionRefused)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("file route", func(t *testing.T) {
		err := ValidateCredentialAdmission(ctx, AdmissionInput{
			AgentID:        "codex-acp",
			ExecutorConfig: map[string]string{"remote_credentials": `["agent:codex-acp:files:0"]`},
			Catalog:        BuildCatalogForHost([]agents.Agent{agents.NewCodexACP()}, "linux", ""),
		})
		require.ErrorIs(t, err, ErrAdmissionRefused)
		require.ErrorIs(t, err, context.Canceled)
	})
}

type cancelOnRevealStore struct {
	secrets.SecretStore
	cancel context.CancelFunc
	value  string
}

func (s *cancelOnRevealStore) Reveal(context.Context, string) (string, error) {
	s.cancel()
	return s.value, nil
}

func TestRemoteAuthAdmissionRejectsCancellationDuringResolution(t *testing.T) {
	realStore := newAdmissionSecretStore(t)
	ctx, cancel := context.WithCancel(admissionTestActor())
	secretID := createAdmissionSecret(t, realStore, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	store := &cancelOnRevealStore{SecretStore: realStore, cancel: cancel, value: admissionTestSecretValue}
	config := map[string]string{
		"remote_auth_secrets": `{"agent:claude-acp:env:ANTHROPIC_API_KEY":"` + secretID + `"}`,
	}

	err := ValidateCredentialAdmission(ctx, claudeInput(nil, config, store))

	require.ErrorIs(t, err, ErrAdmissionRefused)
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), secretID)
	require.NotContains(t, err.Error(), admissionTestSecretValue)
}

func TestRemoteAuthAdmissionRejectsCancellationDuringEnvironmentResolution(t *testing.T) {
	realStore := newAdmissionSecretStore(t)
	ctx, cancel := context.WithCancel(admissionTestActor())
	secretID := createAdmissionSecret(t, realStore, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	store := &cancelOnRevealStore{SecretStore: realStore, cancel: cancel, value: admissionTestSecretValue}

	err := ValidateCredentialAdmission(ctx, claudeInput(apiKeyProfileEnv(secretID), nil, store))

	require.ErrorIs(t, err, ErrAdmissionRefused)
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), secretID)
	require.NotContains(t, err.Error(), admissionTestSecretValue)
}

func TestRemoteAuthAdmissionAcceptsFileRoute(t *testing.T) {
	codexCatalog := BuildCatalogForHost([]agents.Agent{agents.NewCodexACP()}, "linux", "")
	err := ValidateCredentialAdmission(context.Background(), AdmissionInput{
		AgentID:        "codex-acp",
		ExecutorConfig: map[string]string{"remote_credentials": `["agent:codex-acp:files:0"]`},
		Catalog:        codexCatalog,
	})

	require.NoError(t, err)
}

func TestRemoteAuthAdmissionAllowsMethodsLessAgent(t *testing.T) {
	err := ValidateCredentialAdmission(context.Background(), AdmissionInput{
		AgentID: "mock-agent",
		Catalog: BuildCatalogForHost([]agents.Agent{agents.NewMockAgent()}, "linux", ""),
	})

	require.NoError(t, err)
}

func TestRemoteAuthAdmissionValidatesMethodsLessEnvironments(t *testing.T) {
	err := ValidateCredentialAdmission(context.Background(), AdmissionInput{
		AgentID: "mock-agent",
		ProfileEnvVars: []models.ProfileEnvVar{
			{Key: "DUPLICATE", Value: "one"},
			{Key: "DUPLICATE", Value: "two"},
		},
		Catalog: BuildCatalogForHost([]agents.Agent{agents.NewMockAgent()}, "linux", ""),
	})

	require.ErrorIs(t, err, ErrAdmissionRefused)
}

func TestRemoteAuthAdmissionRejectsConflictingAuthenticationBindings(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	profileSecretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	executorSecretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, "another-synthetic-value")
	methodID := "agent:claude-acp:env:ANTHROPIC_API_KEY"
	config := map[string]string{
		"remote_auth_secrets": `{"` + methodID + `":"` + executorSecretID + `"}`,
	}

	err := ValidateCredentialAdmission(ctx, claudeInput(apiKeyProfileEnv(profileSecretID), config, store))
	require.ErrorIs(t, err, ErrAdmissionRefused)
	require.NotContains(t, err.Error(), profileSecretID)
	require.NotContains(t, err.Error(), executorSecretID)

	sharedConfig := map[string]string{
		"remote_auth_secrets": `{"` + methodID + `":"` + profileSecretID + `"}`,
	}
	require.NoError(t, ValidateCredentialAdmission(ctx, claudeInput(apiKeyProfileEnv(profileSecretID), sharedConfig, store)))
}

func TestRemoteAuthAdmissionRejectsNULSecretValues(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, "sk-ant-\x00invalid")
	methodID := "agent:claude-acp:env:ANTHROPIC_API_KEY"

	for _, input := range []AdmissionInput{
		claudeInput(apiKeyProfileEnv(secretID), nil, store),
		claudeInput(nil, map[string]string{
			"remote_auth_secrets": `{"` + methodID + `":"` + secretID + `"}`,
		}, store),
	} {
		err := ValidateCredentialAdmission(ctx, input)
		require.ErrorIs(t, err, ErrAdmissionRefused)
		require.NotContains(t, err.Error(), secretID)
	}
}

func TestRemoteAuthAdmissionRejectsUnusableBindings(t *testing.T) {
	ownerCtx := admissionTestActor()
	store := newAdmissionSecretStore(t)
	validID := createAdmissionSecret(t, store, ownerCtx, secrets.ScopeGlobal, admissionTestSecretValue)
	workspaceID := createAdmissionSecret(t, store, ownerCtx, secrets.ScopeWorkspace, admissionTestSecretValue)
	emptyID := createAdmissionSecret(t, store, ownerCtx, secrets.ScopeGlobal, " ")
	deletedID := createAdmissionSecret(t, store, ownerCtx, secrets.ScopeGlobal, admissionTestSecretValue)
	require.NoError(t, store.Delete(ownerCtx, deletedID))

	tests := []struct {
		name    string
		ctx     context.Context
		profile []models.ProfileEnvVar
		config  map[string]string
	}{
		{name: "missing secret", ctx: ownerCtx, profile: apiKeyProfileEnv("missing-secret")},
		{name: "wrong owner", ctx: authn.WithIdentity(context.Background(), authn.Identity{UserID: "bob"}), profile: apiKeyProfileEnv(validID)},
		{name: "workspace scope", ctx: ownerCtx, profile: apiKeyProfileEnv(workspaceID)},
		{name: "empty value", ctx: ownerCtx, profile: apiKeyProfileEnv(emptyID)},
		{name: "deleted", ctx: ownerCtx, profile: apiKeyProfileEnv(deletedID)},
		{name: "missing identity", ctx: context.Background(), profile: apiKeyProfileEnv(validID)},
		{
			name:    "invalid selected oauth binding",
			ctx:     ownerCtx,
			profile: apiKeyProfileEnv(validID),
			config:  map[string]string{"remote_auth_secrets": `{"agent:claude-acp:env:CLAUDE_CODE_OAUTH_TOKEN":"missing-oauth"}`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCredentialAdmission(tc.ctx, claudeInput(tc.profile, tc.config, store))
			require.ErrorIs(t, err, ErrAdmissionRefused)
			require.NotContains(t, err.Error(), admissionTestSecretValue)
			require.NotContains(t, err.Error(), validID)
		})
	}
}

func TestRemoteAuthAdmissionRejectsBrokenRequiredProfileBinding(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	apiKeyID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	profileEnv := []models.ProfileEnvVar{
		{Key: "ANTHROPIC_API_KEY", SecretID: apiKeyID},
		{Key: "HTTP_PROXY", SecretID: "missing-proxy-secret"},
	}

	err := ValidateCredentialAdmission(ctx, claudeInput(profileEnv, nil, store))

	require.ErrorIs(t, err, ErrAdmissionRefused)
	require.NotContains(t, err.Error(), apiKeyID)
	require.NotContains(t, err.Error(), "missing-proxy-secret")
}

func TestRemoteAuthAdmissionRejectsMalformedOrUnknownConfiguration(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)

	tests := []struct {
		name   string
		config map[string]string
	}{
		{name: "malformed credentials", config: map[string]string{"remote_credentials": `{`}},
		{name: "malformed secrets", config: map[string]string{"remote_auth_secrets": `[]`}},
		{name: "unknown selected method", config: map[string]string{"remote_auth_secrets": `{"agent:claude-acp:env:NOT_REAL":"` + secretID + `"}`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCredentialAdmission(ctx, claudeInput(apiKeyProfileEnv(secretID), tc.config, store))
			require.ErrorIs(t, err, ErrAdmissionRefused)
			require.NotContains(t, err.Error(), secretID)
		})
	}
}

func TestRemoteAuthAdmissionPreservesCancellation(t *testing.T) {
	store := newAdmissionSecretStore(t)
	actorCtx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, actorCtx, secrets.ScopeGlobal, admissionTestSecretValue)
	canceled, cancel := context.WithCancel(actorCtx)
	cancel()

	err := ValidateCredentialAdmission(canceled, claudeInput(apiKeyProfileEnv(secretID), nil, store))

	require.ErrorIs(t, err, ErrAdmissionRefused)
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), admissionTestSecretValue)
	require.NotContains(t, err.Error(), secretID)
}

func TestRemoteAuthAdmissionRejectsInvalidProfileShape(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	secretID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)

	tests := []struct {
		name    string
		profile []models.ProfileEnvVar
	}{
		{name: "duplicate keys", profile: []models.ProfileEnvVar{
			{Key: "ANTHROPIC_API_KEY", SecretID: secretID},
			{Key: "ANTHROPIC_API_KEY", SecretID: secretID},
		}},
		{name: "both value and secret", profile: []models.ProfileEnvVar{
			{Key: "ANTHROPIC_API_KEY", Value: "literal", SecretID: secretID},
		}},
		{name: "reserved key", profile: []models.ProfileEnvVar{
			{Key: "KANDEV_TASK_ID", Value: "x"},
			{Key: "ANTHROPIC_API_KEY", SecretID: secretID},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCredentialAdmission(ctx, claudeInput(tc.profile, nil, store))
			require.ErrorIs(t, err, ErrAdmissionRefused)
			require.NotContains(t, err.Error(), secretID)
		})
	}
}

func TestRemoteAuthAdmissionRejectsConflictingBindings(t *testing.T) {
	store := newAdmissionSecretStore(t)
	ctx := admissionTestActor()
	firstID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, admissionTestSecretValue)
	secondID := createAdmissionSecret(t, store, ctx, secrets.ScopeGlobal, "another-synthetic-value")
	input := claudeInput(apiKeyProfileEnv(firstID), nil, store)
	input.ExecutorEnvVars = []models.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", SecretID: secondID}}

	err := ValidateCredentialAdmission(ctx, input)

	require.ErrorIs(t, err, ErrAdmissionRefused)
	require.NotContains(t, err.Error(), firstID)
	require.NotContains(t, err.Error(), secondID)
	require.NotContains(t, strings.ToLower(err.Error()), "sk-ant")
}
