package automation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type personalBindingValidatorFunc func(context.Context, *Automation, []PersonalCredentialBindingInput) ([]AutomationCredentialChoice, error)

func (f personalBindingValidatorFunc) ValidatePersonalCredentialBindings(
	ctx context.Context,
	a *Automation,
	inputs []PersonalCredentialBindingInput,
) ([]AutomationCredentialChoice, error) {
	return f(ctx, a, inputs)
}

type credentialChoiceRepositoryReader map[string]*taskmodels.Repository

func (r credentialChoiceRepositoryReader) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	return r[id], nil
}

type credentialChoiceSecretStore struct {
	secrets.ScopedSecretStore
	secret *secrets.Secret
	value  string
}

func (s *credentialChoiceSecretStore) GetForWorkspace(ctx context.Context, id, workspaceID string) (*secrets.Secret, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || s.secret == nil || identity.UserID != s.secret.OwnerUserID {
		return nil, secrets.ErrNotFound
	}
	if s.secret == nil || s.secret.ID != id || s.secret.WorkspaceID != workspaceID {
		return nil, secrets.ErrNotFound
	}
	return s.secret, nil
}

func (s *credentialChoiceSecretStore) RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || s.secret == nil || identity.UserID != s.secret.OwnerUserID || s.secret.ID != id || s.secret.WorkspaceID != workspaceID || s.value == "" {
		return "", secrets.ErrNotFound
	}
	return s.value, nil
}

func TestAutomationPersonalCredentialBinding(t *testing.T) {
	ctx := credentialIdentity("creator-1")
	svc, a := credentialChoiceService(t)
	profileID := a.AgentProfileID
	secretID := "creator-secret"
	input := []PersonalCredentialBindingInput{{ProfileID: profileID, Key: "SERVICE_KEY", SecretID: secretID}}
	svc.SetPersonalCredentialBindingValidator(personalBindingValidatorFunc(func(_ context.Context, got *Automation, inputs []PersonalCredentialBindingInput) ([]AutomationCredentialChoice, error) {
		require.Equal(t, profileID, got.AgentProfileID)
		require.Equal(t, input, inputs)
		return []AutomationCredentialChoice{{
			Kind: CredentialChoicePersonal, ProfileID: profileID, Key: "SERVICE_KEY",
			SecretID: secretID, SourceSecretID: "profile-default", State: CredentialChoiceSelected,
			SecretName: "Review key",
		}}, nil
	}))

	_, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{PersonalCredentialBindings: &input})
	require.NoError(t, err)

	got, err := svc.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, got.CredentialChoices, 1)
	require.Equal(t, CredentialChoiceSelected, got.CredentialChoices[0].State)
	require.Equal(t, secretID, got.CredentialChoices[0].SecretID)
	require.Equal(t, "Review key", got.CredentialChoices[0].SecretName)

	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"secret_id":"creator-secret"`)
	require.NotContains(t, string(encoded), "profile-default")

	collaborator, err := svc.GetAutomation(credentialIdentity("collaborator-1"), a.ID)
	require.NoError(t, err)
	require.Empty(t, collaborator.CredentialChoices[0].SecretID)
	require.Empty(t, collaborator.CredentialChoices[0].SecretName)

	updated, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{PersonalCredentialBindings: ptr([]PersonalCredentialBindingInput{})})
	require.NoError(t, err)
	require.Empty(t, updated.CredentialChoices)
}

func TestAutomationCanTriggerNowRequiresCurrentCredentialAuthority(t *testing.T) {
	base := &Automation{
		Enabled: true, CreatedByUserID: "creator-1", TaskMode: TaskModeAutomationRun, RepositoryMode: RepositoryModeSelected,
		AgentProfileID: "agent", ExecutorProfileID: "executor", ContinuationPolicy: ContinuationPolicyNewTask,
		Repositories: []AutomationRepository{{RepositoryID: "repo-1"}},
		CredentialChoices: []CredentialChoiceProjection{{
			Kind: CredentialChoiceWorkspace, RepositoryID: "repo-1", State: CredentialChoiceApproval,
		}},
	}
	ctx := credentialIdentity("creator-1")
	require.False(t, automationCanTriggerNow(ctx, base))

	base.CredentialChoices[0].State = CredentialChoiceGranted
	require.True(t, automationCanTriggerNow(ctx, base))
	base.CredentialChoices[0].State = CredentialChoiceRevoked
	require.False(t, automationCanTriggerNow(ctx, base))
	base.CredentialChoices = []CredentialChoiceProjection{{
		Kind: CredentialChoiceWorkspace, RepositoryID: "repo-removed", State: CredentialChoiceStale,
	}}
	require.True(t, automationCanTriggerNow(ctx, base))
}

func TestAutomationWorkspaceSecretGrant(t *testing.T) {
	t.Run("removed repository grants stay stale while a disabled draft can be rebound", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		repo1 := credentialChoiceRepository(a.WorkspaceID)
		repo2BindingTime := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
		repo2 := &taskmodels.Repository{
			ID: "repo-2", WorkspaceID: a.WorkspaceID, Name: "legal",
			SecretBindings: []taskmodels.RepositorySecretBinding{{
				RepositoryID: "repo-2", Key: "SERVICE_KEY", SecretID: "workspace-secret-2", UpdatedAt: repo2BindingTime,
			}},
		}
		secretStore := &credentialChoiceSecretStore{secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "owner-1", Name: "Billing key",
			Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}, value: "billing-value"}
		svc.SetWorkspaceCredentialGrantDependencies(testWorkspaceOwner, credentialChoiceRepositoryReader{
			"repo-1": repo1, "repo-2": repo2,
		}, secretStore, testCredentialOwnerContext, testCredentialCreatorAccess)
		firstGrant, err := svc.GrantWorkspaceSecret(credentialIdentity("owner-1"), a.ID, workspaceGrantRequest(a, 0))
		require.NoError(t, err)
		require.Equal(t, CredentialChoiceGranted, firstGrant.CredentialChoices[0].State)
		svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{
			"repo-1": a.WorkspaceID, "repo-2": a.WorkspaceID,
		}})

		rebound, err := svc.UpdateAutomation(credentialIdentity("creator-1"), a.ID, &UpdateAutomationRequest{
			Repositories: []AutomationRepository{{RepositoryID: "repo-2", BaseBranch: "main"}},
		})
		require.NoError(t, err)
		require.False(t, rebound.Enabled)
		require.Equal(t, []string{"repo-2"}, rebound.RepositoryIDs)
		secretStore.secret = &secrets.Secret{
			ID: "workspace-secret-2", OwnerUserID: "owner-1", Name: "Legal key",
			Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}

		ownerView, err := svc.GetAutomation(credentialIdentity("owner-1"), a.ID)
		require.NoError(t, err)
		var newRepositoryChoice *CredentialChoiceProjection
		for index := range ownerView.CredentialChoices {
			choice := &ownerView.CredentialChoices[index]
			if choice.RepositoryID == "repo-2" {
				newRepositoryChoice = choice
			}
			if choice.RepositoryID == "repo-1" {
				require.Equal(t, CredentialChoiceStale, choice.State)
			}
		}
		require.NotNil(t, newRepositoryChoice)
		require.Equal(t, CredentialChoiceApproval, newRepositoryChoice.State)
		require.True(t, newRepositoryChoice.CanGrant)

		grantRequest := &WorkspaceCredentialGrantRequest{
			RepositoryID: "repo-2", Key: "SERVICE_KEY", ExpectedAutomationRevision: rebound.ResourceRevision,
			ExpectedBindingUpdatedAt: repo2BindingTime, ExpectedWorkspaceOwnerUpdatedAt: testWorkspaceOwnerUpdatedAt,
		}
		granted, err := svc.GrantWorkspaceSecret(credentialIdentity("owner-1"), a.ID, grantRequest)
		require.NoError(t, err)
		var newChoiceRevision int64
		for _, choice := range granted.CredentialChoices {
			if choice.RepositoryID == "repo-2" {
				require.Equal(t, CredentialChoiceGranted, choice.State)
				newChoiceRevision = choice.Revision
			}
		}
		require.EqualValues(t, 1, newChoiceRevision)

		validated := 0
		svc.SetCredentialValidator(credentialValidatorFunc(func(_ context.Context, _ *Automation, repositoryIDs []string) error {
			validated++
			require.Equal(t, []string{"repo-2"}, repositoryIDs)
			return nil
		}))
		require.NoError(t, svc.EnableAutomation(credentialIdentity("creator-1"), a.ID))
		require.Equal(t, 1, validated)
	})

	t.Run("only the current workspace owner who owns the bound secret can grant", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		request := workspaceGrantRequest(a, 0)
		_, err := svc.GrantWorkspaceSecret(credentialIdentity("creator-1"), a.ID, request)
		require.ErrorIs(t, err, ErrWorkspaceCredentialGrantDenied)

		ownerCtx := credentialIdentity("owner-1")
		store := credentialChoiceSecretStore{secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "different-owner", Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}, value: "value"}
		svc.SetWorkspaceCredentialGrantDependencies(testWorkspaceOwner, credentialChoiceRepositoryReader{
			"repo-1": credentialChoiceRepository(a.WorkspaceID),
		}, &store, testCredentialOwnerContext, testCredentialCreatorAccess)
		_, err = svc.GrantWorkspaceSecret(ownerCtx, a.ID, request)
		require.ErrorIs(t, err, ErrWorkspaceCredentialGrantDenied)
	})

	t.Run("grant projects only safe details and revocation blocks future resolution", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		store := &credentialChoiceSecretStore{secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "owner-1", Name: "Billing key",
			Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}, value: "secret-value"}
		svc.SetWorkspaceCredentialGrantDependencies(testWorkspaceOwner, credentialChoiceRepositoryReader{
			"repo-1": credentialChoiceRepository(a.WorkspaceID),
		}, store, testCredentialOwnerContext, testCredentialCreatorAccess)
		ownerCtx := credentialIdentity("owner-1")
		request := workspaceGrantRequest(a, 0)
		granted, err := svc.GrantWorkspaceSecret(ownerCtx, a.ID, request)
		require.NoError(t, err)
		require.Equal(t, CredentialChoiceGranted, granted.CredentialChoices[0].State)
		require.Equal(t, "Billing key", granted.CredentialChoices[0].SecretName)

		creatorView, err := svc.GetAutomation(credentialIdentity("creator-1"), a.ID)
		require.NoError(t, err)
		require.Equal(t, CredentialChoiceGranted, creatorView.CredentialChoices[0].State)
		require.Empty(t, creatorView.CredentialChoices[0].SecretName)
		encoded, err := json.Marshal(creatorView)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "workspace-secret")
		require.NotContains(t, string(encoded), "Billing key")
		_, validationValue, err := svc.ResolveAutomationCredentialChoice(
			credentialIdentity("creator-1"), a, "repo-1", "SERVICE_KEY",
		)
		require.NoError(t, err)
		require.Equal(t, "secret-value", validationValue)

		run := &AutomationRun{
			AutomationID: a.ID, TriggerType: TriggerTypeManual, TaskID: "task-1",
			ProtectedCreatorUserID: "creator-1", Status: RunStatusTriggered,
		}
		require.NoError(t, svc.store.CreateRun(context.Background(), run))
		protected, allowed, err := svc.ProtectedAutomationRunUseAccessForTask(credentialIdentity("creator-1"), "task-1", "creator-1")
		require.NoError(t, err)
		require.True(t, protected)
		require.True(t, allowed)
		value, err := svc.ResolveAutomationCredentialBindingValue(credentialIdentity("creator-1"), "task-1", a.ID, run.ID, a.Repositories[0].RepositoryID, "SERVICE_KEY")
		require.NoError(t, err)
		require.Equal(t, "secret-value", value)

		choice := granted.CredentialChoices[0]
		revoked, err := svc.RevokeWorkspaceSecret(ownerCtx, a.ID, workspaceGrantRequest(a, choice.Revision))
		require.NoError(t, err)
		require.Equal(t, CredentialChoiceRevoked, revoked.CredentialChoices[0].State)
		_, _, err = svc.ResolveAutomationCredentialChoice(
			credentialIdentity("creator-1"), a, "repo-1", "SERVICE_KEY",
		)
		require.ErrorIs(t, err, ErrCredentialsUnavailable)
		_, err = svc.ResolveAutomationCredentialBindingValue(credentialIdentity("creator-1"), "task-1", a.ID, run.ID, "repo-1", "SERVICE_KEY")
		require.ErrorIs(t, err, ErrCredentialsUnavailable)
		protected, allowed, err = svc.ProtectedAutomationRunUseAccessForTask(credentialIdentity("creator-1"), "task-1", "creator-1")
		require.NoError(t, err)
		require.True(t, protected)
		require.False(t, allowed)
	})

	t.Run("a replaced repository binding does not revive an older grant", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		secret := &credentialChoiceSecretStore{secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "owner-1", Name: "Billing key",
			Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}, value: "secret-value"}
		repository := credentialChoiceRepository(a.WorkspaceID)
		svc.SetWorkspaceCredentialGrantDependencies(testWorkspaceOwner, credentialChoiceRepositoryReader{"repo-1": repository}, secret, testCredentialOwnerContext, testCredentialCreatorAccess)
		ownerCtx := credentialIdentity("owner-1")
		granted, err := svc.GrantWorkspaceSecret(ownerCtx, a.ID, workspaceGrantRequest(a, 0))
		require.NoError(t, err)
		run := &AutomationRun{
			AutomationID: a.ID, TriggerType: TriggerTypeManual, TaskID: "task-2",
			ProtectedCreatorUserID: "creator-1", Status: RunStatusTriggered,
		}
		require.NoError(t, svc.store.CreateRun(context.Background(), run))

		repository.SecretBindings[0].UpdatedAt = repository.SecretBindings[0].UpdatedAt.Add(time.Second)
		_, err = svc.ResolveAutomationCredentialBindingValue(credentialIdentity("creator-1"), "task-2", a.ID, run.ID, "repo-1", "SERVICE_KEY")
		require.ErrorIs(t, err, ErrCredentialsUnavailable)

		stale, err := svc.GetAutomation(ownerCtx, a.ID)
		require.NoError(t, err)
		require.Equal(t, CredentialChoiceStale, stale.CredentialChoices[0].State)
		require.Equal(t, "Billing key", stale.CredentialChoices[0].SecretName)
		require.Equal(t, granted.CredentialChoices[0].Revision, stale.CredentialChoices[0].Revision)
	})

	t.Run("stale resource revisions cannot overwrite a newer grant", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		store := &credentialChoiceSecretStore{secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "owner-1", Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}, value: "value"}
		svc.SetWorkspaceCredentialGrantDependencies(testWorkspaceOwner, credentialChoiceRepositoryReader{"repo-1": credentialChoiceRepository(a.WorkspaceID)}, store, testCredentialOwnerContext, testCredentialCreatorAccess)
		request := workspaceGrantRequest(a, 0)
		request.ExpectedAutomationRevision--
		_, err := svc.GrantWorkspaceSecret(credentialIdentity("owner-1"), a.ID, request)
		require.ErrorIs(t, err, ErrCredentialSettingsConflict)
	})

	t.Run("stale binding confirmation cannot approve a replacement secret", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		repository := credentialChoiceRepository(a.WorkspaceID)
		store := &credentialChoiceSecretStore{secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "owner-1", Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}, value: "value"}
		svc.SetWorkspaceCredentialGrantDependencies(testWorkspaceOwner, credentialChoiceRepositoryReader{"repo-1": repository}, store, testCredentialOwnerContext, testCredentialCreatorAccess)
		request := workspaceGrantRequest(a, 0)
		repository.SecretBindings[0].SecretID = "replacement-secret"
		repository.SecretBindings[0].UpdatedAt = repository.SecretBindings[0].UpdatedAt.Add(time.Second)

		_, err := svc.GrantWorkspaceSecret(credentialIdentity("owner-1"), a.ID, request)
		require.ErrorIs(t, err, ErrCredentialSettingsConflict)
		choices, choiceErr := svc.store.credentialChoices(context.Background(), a.ID)
		require.NoError(t, choiceErr)
		require.Empty(t, choices)
	})

	t.Run("ownership moving away and back does not revive the old grant", func(t *testing.T) {
		svc, a := credentialChoiceService(t)
		currentOwner := "owner-1"
		ownerRevision := testWorkspaceOwnerUpdatedAt
		svc.SetWorkspaceCredentialOwnerRevisionLookup(func(context.Context, string) (time.Time, error) { return ownerRevision, nil })
		secretStore := &credentialChoiceSecretStore{secret: &secrets.Secret{
			ID: "workspace-secret", OwnerUserID: "owner-1", Scope: secrets.ScopeWorkspace, WorkspaceID: a.WorkspaceID,
		}, value: "value"}
		svc.SetWorkspaceCredentialGrantDependencies(
			func(context.Context, string) (string, error) { return currentOwner, nil },
			credentialChoiceRepositoryReader{"repo-1": credentialChoiceRepository(a.WorkspaceID)},
			secretStore, testCredentialOwnerContext, testCredentialCreatorAccess,
		)
		granted, err := svc.GrantWorkspaceSecret(credentialIdentity("owner-1"), a.ID, workspaceGrantRequest(a, 0))
		require.NoError(t, err)

		ownerRevision = ownerRevision.Add(time.Second)
		currentOwner = "owner-2"
		ownerRevision = ownerRevision.Add(time.Second)
		currentOwner = "owner-1"
		_, _, err = svc.ResolveAutomationCredentialChoice(credentialIdentity("creator-1"), a, "repo-1", "SERVICE_KEY")
		require.ErrorIs(t, err, ErrCredentialsUnavailable)

		ownerView, err := svc.GetAutomation(credentialIdentity("owner-1"), a.ID)
		require.NoError(t, err)
		require.Equal(t, CredentialChoiceStale, ownerView.CredentialChoices[0].State)
		require.True(t, ownerView.CredentialChoices[0].CanRevoke)
		_, err = svc.RevokeWorkspaceSecret(credentialIdentity("owner-1"), a.ID, workspaceGrantRequest(a, granted.CredentialChoices[0].Revision))
		require.NoError(t, err)
	})
}

func credentialChoiceService(t *testing.T) (*Service, *Automation) {
	t.Helper()
	svc := newTestService(t)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	svc.SetWorkspaceCredentialOwnerRevisionLookup(testWorkspaceOwnerRevision)
	a := &Automation{
		ID: "automation-credential-choice", WorkspaceID: "workspace-1", CreatedByUserID: "creator-1",
		Name: "credential automation", TaskMode: TaskModeAutomationRun, RepositoryMode: RepositoryModeSelected,
		AgentProfileID: "agent-profile", ExecutorProfileID: "executor-profile", Enabled: false,
		ContinuationPolicy: ContinuationPolicyNewTask,
		Repositories:       []AutomationRepository{{RepositoryID: "repo-1", BaseBranch: "main"}},
	}
	require.NoError(t, svc.store.CreateAutomation(credentialIdentity("creator-1"), a))
	stored, err := svc.store.GetAutomation(context.Background(), a.ID)
	require.NoError(t, err)
	require.Equal(t, "creator-1", stored.CreatedByUserID)
	require.Equal(t, "executor-profile", stored.ExecutorProfileID)
	return svc, stored
}

func credentialChoiceRepository(workspaceID string) *taskmodels.Repository {
	return &taskmodels.Repository{
		ID: "repo-1", WorkspaceID: workspaceID, Name: "billing",
		SecretBindings: []taskmodels.RepositorySecretBinding{{
			RepositoryID: "repo-1", Key: "SERVICE_KEY", SecretID: "workspace-secret",
			CreatedAt: time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC),
		}},
	}
}

func workspaceGrantRequest(a *Automation, choiceRevision int64) *WorkspaceCredentialGrantRequest {
	return &WorkspaceCredentialGrantRequest{
		RepositoryID: "repo-1", Key: "SERVICE_KEY", ExpectedAutomationRevision: a.ResourceRevision,
		ExpectedCredentialRevision:      choiceRevision,
		ExpectedBindingUpdatedAt:        credentialChoiceRepository(a.WorkspaceID).SecretBindings[0].UpdatedAt,
		ExpectedWorkspaceOwnerUpdatedAt: testWorkspaceOwnerUpdatedAt,
	}
}

func credentialIdentity(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

func testWorkspaceOwner(context.Context, string) (string, error) { return "owner-1", nil }

var testWorkspaceOwnerUpdatedAt = time.Date(2026, 11, 26, 1, 0, 0, 0, time.UTC)

func testWorkspaceOwnerRevision(context.Context, string) (time.Time, error) {
	return testWorkspaceOwnerUpdatedAt, nil
}

func testCredentialOwnerContext(ctx context.Context, userID string) (context.Context, error) {
	if userID != "owner-1" {
		return nil, errors.New("owner identity unavailable")
	}
	return authn.WithIdentity(ctx, authn.Identity{UserID: userID, Role: authn.RoleMember}), nil
}

func testCredentialCreatorAccess(_ context.Context, workspaceID, creatorID, profileID string) error {
	if workspaceID != "workspace-1" || creatorID != "creator-1" || profileID != "executor-profile" {
		return ErrCredentialCreatorUnavailable
	}
	return nil
}

func ptr[T any](value T) *T { return &value }
