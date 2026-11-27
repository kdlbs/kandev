package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type automationCredentialProfileResolver struct {
	info *AgentProfileInfo
	err  error
}

func (r automationCredentialProfileResolver) ResolveProfile(context.Context, string) (*AgentProfileInfo, error) {
	return r.info, r.err
}

type automationCredentialRepositoryReader struct {
	*fakeExecutorProfileReader
	repositories map[string]*models.Repository
	executor     *models.Executor
	executorErr  error
	executorIDs  []string
}

type automationCredentialSecretStore struct {
	*inMemorySecretStore
	revealedGlobalIDs []string
}

func (*automationCredentialSecretStore) CopyScoped(context.Context, string, string, secrets.SecretScope, string, *string, func(context.Context) error) (*secrets.Secret, error) {
	return nil, nil
}

func (*automationCredentialSecretStore) MoveScoped(context.Context, string, string, secrets.SecretScope, string, *string, func(context.Context) error) (*secrets.Secret, error) {
	return nil, nil
}

func (s *automationCredentialSecretStore) RevealGlobal(ctx context.Context, id string) (string, error) {
	s.revealedGlobalIDs = append(s.revealedGlobalIDs, id)
	return s.inMemorySecretStore.RevealGlobal(ctx, id)
}

func (r *automationCredentialRepositoryReader) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	return r.repositories[id], nil
}

func (r *automationCredentialRepositoryReader) GetExecutor(_ context.Context, id string) (*models.Executor, error) {
	r.executorIDs = append(r.executorIDs, id)
	return r.executor, r.executorErr
}

func TestAutomationCredentialEnvironment(t *testing.T) {
	ctx := context.Background()
	store := newInMemorySecretStore()
	for _, secret := range []*secrets.SecretWithValue{
		{Secret: secrets.Secret{ID: "agent-secret"}, Value: "agent-value"},
		{Secret: secrets.Secret{ID: "executor-secret"}, Value: "executor-value"},
		{Secret: secrets.Secret{ID: "workspace-secret", Scope: secrets.ScopeWorkspace, WorkspaceID: "workspace-1"}, Value: "workspace-value"},
	} {
		require.NoError(t, store.Create(ctx, secret))
	}

	reader := &automationCredentialRepositoryReader{
		fakeExecutorProfileReader: &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
			"executor-profile": {ID: "executor-profile", EnvVars: []models.ProfileEnvVar{
				{Key: "EXECUTOR_KEY", SecretID: "executor-secret"},
				{Key: "SHARED_SETTING", Value: "executor-value"},
			}},
		}},
		repositories: map[string]*models.Repository{
			"repo-1": {ID: "repo-1", WorkspaceID: "workspace-1", Name: "app", SecretBindings: []models.RepositorySecretBinding{
				{Key: "REPOSITORY_KEY", SecretID: "workspace-secret"},
			}},
		},
	}
	mgr := newTestManager(t)
	mgr.profileResolver = automationCredentialProfileResolver{info: &AgentProfileInfo{
		EnvVars: []models.ProfileEnvVar{
			{Key: "AGENT_KEY", SecretID: "agent-secret"},
			{Key: "SHARED_SETTING", Value: "agent-value"},
		},
	}}
	mgr.SetExecutorProfileReader(reader)
	mgr.SetSecretStore(&automationCredentialSecretStore{inMemorySecretStore: store})

	req := AutomationCredentialRequest{
		WorkspaceID: "workspace-1", AgentProfileID: "agent-profile", ExecutorProfileID: "executor-profile",
		RepositoryIDs: []string{"repo-1"},
	}
	require.NoError(t, mgr.ValidateAutomationCredentialEnvironment(ctx, req))

	reader.repositories["repo-1"].SecretBindings[0].SecretID = "missing-workspace-secret"
	err := mgr.ValidateAutomationCredentialEnvironment(ctx, req)
	var credentialErr *AutomationCredentialEnvironmentError
	require.ErrorAs(t, err, &credentialErr)
	require.Equal(t, "REPOSITORY_KEY", credentialErr.Field)
	require.Equal(t, "repository app", credentialErr.Source)
	require.NotContains(t, err.Error(), "missing-workspace-secret")

	reader.repositories["repo-1"].SecretBindings[0].SecretID = "workspace-secret"
	mgr.profileResolver = automationCredentialProfileResolver{info: &AgentProfileInfo{
		ProviderKind: settingsmodels.ProviderKindOpenAICompatible, ProviderAPIKeySecretID: "missing-provider-secret",
	}}
	err = mgr.ValidateAutomationCredentialEnvironment(ctx, req)
	require.ErrorAs(t, err, &credentialErr)
	require.Equal(t, "provider API key", credentialErr.Field)
	require.Equal(t, "Agent profile", credentialErr.Source)
	require.NotContains(t, err.Error(), "missing-provider-secret")
}

func TestAutomationCredentialEnvironmentRemoteAdmissionUsesPersonalBinding(t *testing.T) {
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "creator", Role: authn.RoleMember})
	store := &automationCredentialSecretStore{inMemorySecretStore: newInMemorySecretStore()}
	require.NoError(t, store.Create(ctx, &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "personal-secret", Scope: secrets.ScopeGlobal}, Value: "personal-api-key",
	}))
	reader := &automationCredentialRepositoryReader{
		fakeExecutorProfileReader: &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
			"executor-profile": {
				ID: "executor-profile", ExecutorID: "remote-executor", EnvVars: []models.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", SecretID: "shared-default-secret"}},
			},
		}},
		executor: &models.Executor{ID: "remote-executor", Type: models.ExecutorTypeRemoteDocker},
	}
	mgr := newTestManager(t)
	mgr.profileResolver = automationCredentialProfileResolver{info: &AgentProfileInfo{
		ProfileID: "agent-profile", AgentName: agents.NewClaudeACP().ID(),
	}}
	mgr.SetExecutorProfileReader(reader)
	mgr.SetSecretStore(store)

	err := mgr.ValidateAutomationCredentialEnvironment(ctx, AutomationCredentialRequest{
		WorkspaceID: "workspace-1", AgentProfileID: "agent-profile", ExecutorProfileID: "executor-profile",
		CredentialBindings: []AutomationCredentialBinding{{
			Kind: "personal", ProfileID: "executor-profile", Key: "ANTHROPIC_API_KEY", SecretID: "personal-secret",
			SourceSecretID: "shared-default-secret", State: "selected",
		}},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"remote-executor"}, reader.executorIDs)
	require.NotEmpty(t, store.revealedGlobalIDs)
	require.Contains(t, store.revealedGlobalIDs, "personal-secret")
	require.NotContains(t, store.revealedGlobalIDs, "shared-default-secret")
}

func TestAutomationCredentialEnvironmentExecutorLookupFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		executor  *models.Executor
		lookupErr error
		wantErr   error
	}{
		{name: "storage failure", lookupErr: errors.New("database unavailable"), wantErr: ErrAutomationCredentialStorage},
		{name: "canceled lookup", lookupErr: context.Canceled, wantErr: context.Canceled},
		{name: "missing executor", wantErr: ErrAutomationCredentialStorage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &automationCredentialRepositoryReader{
				fakeExecutorProfileReader: &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
					"executor-profile": {ID: "executor-profile", ExecutorID: "remote-executor"},
				}},
				executor:    tc.executor,
				executorErr: tc.lookupErr,
			}
			mgr := newTestManager(t)
			mgr.profileResolver = automationCredentialProfileResolver{info: &AgentProfileInfo{
				ProfileID: "agent-profile", AgentName: agents.NewClaudeACP().ID(),
			}}
			mgr.SetExecutorProfileReader(reader)
			mgr.SetSecretStore(newInMemorySecretStore())

			err := mgr.ValidateAutomationCredentialEnvironment(context.Background(), AutomationCredentialRequest{
				AgentProfileID: "agent-profile", ExecutorProfileID: "executor-profile",
			})

			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, []string{"remote-executor"}, reader.executorIDs)
		})
	}
}

func TestAutomationCredentialEnvironmentSkipsRemoteAdmissionAfterLocalExecutorLookup(t *testing.T) {
	reader := &automationCredentialRepositoryReader{
		fakeExecutorProfileReader: &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
			"executor-profile": {
				ID: "executor-profile", ExecutorID: "local-executor",
				Config: map[string]string{"remote_credentials": "invalid local-only config"},
			},
		}},
		executor: &models.Executor{ID: "local-executor", Type: models.ExecutorTypeLocal},
	}
	mgr := newTestManager(t)
	mgr.profileResolver = automationCredentialProfileResolver{info: &AgentProfileInfo{
		ProfileID: "agent-profile", AgentName: agents.NewClaudeACP().ID(),
	}}
	mgr.SetExecutorProfileReader(reader)
	mgr.SetSecretStore(newInMemorySecretStore())

	err := mgr.ValidateAutomationCredentialEnvironment(context.Background(), AutomationCredentialRequest{
		AgentProfileID: "agent-profile", ExecutorProfileID: "executor-profile",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"local-executor"}, reader.executorIDs)
}

func TestAutomationCredentialEnvironmentRequiresExecutorLookupCapability(t *testing.T) {
	reader := &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
		"executor-profile": {ID: "executor-profile", ExecutorID: "remote-executor"},
	}}
	mgr := newTestManager(t)
	mgr.profileResolver = automationCredentialProfileResolver{info: &AgentProfileInfo{
		ProfileID: "agent-profile", AgentName: agents.NewClaudeACP().ID(),
	}}
	mgr.SetExecutorProfileReader(reader)
	mgr.SetSecretStore(newInMemorySecretStore())

	err := mgr.ValidateAutomationCredentialEnvironment(context.Background(), AutomationCredentialRequest{
		AgentProfileID: "agent-profile", ExecutorProfileID: "executor-profile",
	})

	require.ErrorIs(t, err, ErrAutomationCredentialStorage)
}
