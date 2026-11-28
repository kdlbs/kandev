package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteAllRunsRejectsForeignProtectedRunBeforeSideEffects(t *testing.T) {
	svc, automation := credentialChoiceService(t)
	storeWorkspaceCredentialChoice(t, svc.store, automation)
	run := &AutomationRun{
		AutomationID: automation.ID, TriggerType: TriggerTypeManual,
		TaskID: "protected-bulk-delete-task", Status: RunStatusTriggered,
	}
	require.NoError(t, svc.store.CreateRun(credentialIdentity("creator-1"), run))
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	stopCalls := 0
	svc.SetRunStopper(runStopperStub{stopped: true, onStop: func() { stopCalls++ }})

	err := svc.DeleteAllRuns(credentialIdentity("collaborator-1"), automation.ID)
	require.ErrorIs(t, err, ErrCredentialCreatorRequired)
	require.Zero(t, stopCalls)
	require.Empty(t, deleter.deleted)
	saved, err := svc.store.GetRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
}
