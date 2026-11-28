package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.4
func TestProtectedCreatorRemainsScopedWhileCleanupJobSurvivesDeletedRun(t *testing.T) {
	svc, definition := credentialChoiceService(t)
	storeWorkspaceCredentialChoice(t, svc.store, definition)
	taskID := "protected-cleanup-task"
	run := &AutomationRun{
		AutomationID: definition.ID, TriggerType: TriggerTypeManual,
		TaskID: taskID, Status: RunStatusTaskCreated,
	}
	require.NoError(t, svc.store.CreateRun(credentialIdentity("creator-1"), run))
	creatorID, err := svc.ProtectedCreatorForRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, "creator-1", creatorID)

	jobs, err := svc.store.DeleteAutomationWithCleanup(context.Background(), definition.ID, []string{taskID})
	require.NoError(t, err)
	require.Equal(t, []string{taskID}, jobs)
	// Unit-test SQLite does not enable foreign keys, so remove the cascaded run
	// explicitly while retaining the cleanup job written by the real delete path.
	require.NoError(t, svc.store.DeleteRun(context.Background(), run.ID))
	remainingRuns, err := svc.store.ListRuns(context.Background(), definition.ID, 10)
	require.NoError(t, err)
	require.Empty(t, remainingRuns)
	cleanupJobs, err := svc.store.ListCleanupJobs(context.Background())
	require.NoError(t, err)
	require.Len(t, cleanupJobs, 1)
	require.Equal(t, "creator-1", cleanupJobs[0].ProtectedCreatorUserID)

	protected, allowed, err := svc.ProtectedAutomationRunAccessForTask(context.Background(), taskID, "creator-1")
	require.NoError(t, err)
	require.True(t, protected)
	require.True(t, allowed, "the creator retains task access for cleanup and Stop")
	protected, allowed, err = svc.ProtectedAutomationRunUseAccessForTask(context.Background(), taskID, "creator-1")
	require.NoError(t, err)
	require.True(t, protected)
	require.False(t, allowed, "new prompt and runtime attachment admission stays closed after run-row deletion")
	protected, allowed, err = svc.ProtectedAutomationRunUseAccessForTask(context.Background(), taskID, "collaborator-1")
	require.NoError(t, err)
	require.True(t, protected)
	require.False(t, allowed)
}
