package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	ownerRevision := testWorkspaceOwnerUpdatedAt
	require.NoError(t, store.SaveWorkspaceCredentialGrant(context.Background(), AutomationCredentialChoice{
		AutomationID: a.ID, Kind: CredentialChoiceWorkspace, RepositoryID: "repo-1", Key: "SERVICE_KEY",
		SecretID: "workspace-secret", SourceSecretID: "workspace-secret", SourceUpdatedAt: credentialChoiceRepository(a.WorkspaceID).SecretBindings[0].UpdatedAt,
		WorkspaceOwnerUpdatedAt: &ownerRevision, CreatorUserID: a.CreatedByUserID, GrantorUserID: "owner-1",
		State: CredentialChoiceGranted,
	}, a.ResourceRevision, 0))
}
