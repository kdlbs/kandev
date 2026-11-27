package automation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateRunClearsProtectionWithoutCredentialChoices(t *testing.T) {
	svc, a := credentialChoiceService(t)
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		ProtectedCreatorUserID: "stale-creator", CredentialChoicesSnapshot: "stale-snapshot",
	}

	require.NoError(t, svc.store.CreateRun(context.Background(), run))

	require.Empty(t, run.ProtectedCreatorUserID)
	require.Equal(t, "[]", run.CredentialChoicesSnapshot)
	saved, err := svc.store.GetRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Empty(t, saved.ProtectedCreatorUserID)
	require.Equal(t, "[]", saved.CredentialChoicesSnapshot)
}

func TestCredentialAuthorityIsRecheckedAtRunCommit(t *testing.T) {
	svc, a := credentialChoiceService(t)
	ctx := context.Background()
	hasChoices, err := svc.store.HasCredentialChoices(ctx, a.ID)
	require.NoError(t, err)
	require.False(t, hasChoices)
	storeWorkspaceCredentialChoice(t, svc.store, a)
	enabled := true
	require.NoError(t, svc.store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Enabled: &enabled}))

	initiator := withAutomationRunInitiator(credentialIdentity("collaborator-1"))
	run := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered}
	err = svc.store.CreateRun(initiator, run)

	require.ErrorIs(t, err, ErrCredentialCreatorRequired)
	saved, readErr := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, readErr)
	require.Nil(t, saved)
}

func TestCredentialAuthorityIsRecheckedAtRetryCommit(t *testing.T) {
	svc, a := credentialChoiceService(t)
	ctx := context.Background()
	hasChoices, err := svc.store.HasCredentialChoices(ctx, a.ID)
	require.NoError(t, err)
	require.False(t, hasChoices)
	storeWorkspaceCredentialChoice(t, svc.store, a)
	enabled := true
	require.NoError(t, svc.store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Enabled: &enabled}))

	initiator := withAutomationRunInitiator(credentialIdentity("collaborator-1"))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: "retry-group-credential-authority", RetryGroupGeneration: 1,
	}
	group := &RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	err = svc.store.CreateRetryAdmission(initiator, run, group)

	require.ErrorIs(t, err, ErrCredentialCreatorRequired)
	saved, readErr := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, readErr)
	require.Nil(t, saved)
}

func TestCredentialAuthorityIsRecheckedAtAutomationUpdateCommit(t *testing.T) {
	svc, a := credentialChoiceService(t)
	ctx := context.Background()
	hasChoices, err := svc.store.HasCredentialChoices(ctx, a.ID)
	require.NoError(t, err)
	require.False(t, hasChoices)
	storeWorkspaceCredentialChoice(t, svc.store, a)
	newPrompt := "changed after first grant"
	_, err = svc.store.updateAutomationReturning(credentialIdentity("collaborator-1"), a.ID, &UpdateAutomationRequest{Prompt: &newPrompt})
	require.ErrorIs(t, err, ErrCredentialCreatorRequired)
	saved, err := svc.store.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Empty(t, saved.Prompt)
}

func storeWorkspaceCredentialChoice(t *testing.T, store *Store, a *Automation) {
	t.Helper()
	ownerRevision := testWorkspaceOwnerRevisionValue
	require.NoError(t, store.SaveWorkspaceCredentialGrant(context.Background(), AutomationCredentialChoice{
		AutomationID: a.ID, Kind: CredentialChoiceWorkspace, RepositoryID: "repo-1", Key: "SERVICE_KEY",
		SecretID: "workspace-secret", SourceSecretID: "workspace-secret", SourceUpdatedAt: credentialChoiceRepository(a.WorkspaceID).SecretBindings[0].UpdatedAt,
		WorkspaceOwnerRevision: &ownerRevision, CreatorUserID: a.CreatedByUserID, GrantorUserID: "owner-1",
		State: CredentialChoiceGranted,
	}, a.ResourceRevision, 0, ownerRevision))
}

func storePersonalCredentialChoice(t *testing.T, store *Store, a *Automation) {
	t.Helper()
	now := time.Now().UTC()
	_, err := store.db.Exec(`INSERT INTO automation_credential_choices (
		automation_id, kind, profile_id, env_key, secret_id, source_secret_id, source_updated_at,
		creator_user_id, state, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, CredentialChoicePersonal, a.AgentProfileID, "SERVICE_KEY", "personal-secret",
		"profile-default", now, a.CreatedByUserID, CredentialChoiceSelected, now, now)
	require.NoError(t, err)
}
