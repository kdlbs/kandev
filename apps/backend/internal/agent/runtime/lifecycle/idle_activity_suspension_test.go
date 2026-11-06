package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestIdleActivityRecoveryKeepsPromptAndOwnershipFences(t *testing.T) {
	for _, tc := range []struct {
		name       string
		invalidate func(*AgentExecution)
	}{
		{"active prompt", func(e *AgentExecution) { e.dispatchedPromptPending.Store(true) }},
		{"running turn", func(e *AgentExecution) { e.Status = v1.AgentStatusRunning }},
		{"successor prompt", func(e *AgentExecution) { e.promptGeneration++ }},
		{"new activity", func(e *AgentExecution) { e.markAgentActivity() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr, e, backend, identity := newIdleActivitySuspensionFixture(t)
			tc.invalidate(e)
			require.ErrorIs(t, mgr.SuspendIdle(t.Context(), identity), ErrIdleSuspensionRejected)
			require.Empty(t, backend.stoppedID, "the runtime must remain untouched behind an invalidated fence")
			e.dispatchedPromptPending.Store(false)
			e.Status = v1.AgentStatusReady
			identity.PromptGeneration = e.promptGenerationSnapshot()
			identity.ActivityEpoch = e.promptActivityEpochSnapshot()
			require.NoError(t, mgr.SuspendIdle(t.Context(), identity), "positive control: the settled current owner can suspend")
			require.Equal(t, e.ID, backend.stoppedID)
		})
	}
}

func newIdleActivitySuspensionFixture(t *testing.T) (*Manager, *AgentExecution, *runOwnerRecoveryExecutor, IdleSuspensionIdentity) {
	t.Helper()
	ctx := t.Context()
	repo := newSettingsSourceRecoveryRepository(t, "task", "session")
	workspace, err := repo.GetWorkspace(ctx, "workspace-task")
	require.NoError(t, err)
	workspace.ACPIdleSuspensionEnabled = true
	require.NoError(t, repo.UpdateWorkspace(ctx, workspace))
	workspace, err = repo.GetWorkspace(ctx, workspace.ID)
	require.NoError(t, err)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	e := createTestExecution("execution", "task", "session")
	e.RuntimeName, e.Status, e.standaloneInstanceID = executor.NameStandalone, v1.AgentStatusReady, e.ID
	e.promptGeneration, e.promptActivityEpoch = 3, 5
	require.NoError(t, mgr.executionStore.Add(e))
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, e))
	row, err := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
	require.NoError(t, err)
	row.ResumeToken = "preserved-conversation"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	return mgr, e, backend, IdleSuspensionIdentity{ExecutionID: e.ID, SessionID: e.SessionID,
		WorkspaceID: workspace.ID, PolicyUpdatedAt: workspace.UpdatedAt, PromptGeneration: 3, ActivityEpoch: 5}
}
