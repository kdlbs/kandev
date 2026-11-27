package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleActivityNormalPersistenceToManagerStart(t *testing.T) {
	ctx := context.Background()
	repo := newSettingsSourceRecoveryRepository(t, "task-idle", "session-idle")
	writer := newTestManager(t)
	writer.SetExecutorRunningWriter(repo)
	e := createTestExecution("execution-idle", "task-idle", "session-idle")
	e.AgentProfileID = recoveryTestAgentProfileID
	e.RuntimeName = agentruntime.RuntimeStandalone
	old := time.Now().UTC().Add(-time.Hour)
	e.lastActivityAt = old
	require.NoError(t, writer.persistExecutorRunningResult(ctx, e))
	updateIdleActivityPassiveMessage(t, repo, e.TaskID, e.SessionID, old)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, e.SessionID, "passive", "updated"))
	row, err := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
	require.NoError(t, err)
	row.Metadata["passive"] = "updated"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctl.InstanceInfo{{ID: e.ID, SessionID: e.SessionID, Port: control.executor(t).port}}
	mgr := newIdleActivityStartupManager(t, repo, control)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
	require.NoError(t, mgr.Start(ctx))
	recovered, ok := mgr.GetExecutionBySessionID(e.SessionID)
	require.True(t, ok, "real standalone recovery must adopt the persisted execution")
	at, _, _ := recovered.promptActivitySnapshot()
	require.True(t, at.Equal(old), "passive persistence must not restart the interval")
	recovered.markRecoveredAgentActivity(nil)
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, recovered))
	cleared, err := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
	require.NoError(t, err)
	require.NotContains(t, cleared.Metadata, idleActivityMetadataKey)
	mgr.executionStore.Remove(e.ID)
	require.NoError(t, mgr.Stop())
	mgr = newIdleActivityStartupManager(t, repo, control)
	require.NoError(t, mgr.Start(ctx))
	next, ok := mgr.GetExecutionBySessionID(e.SessionID)
	require.True(t, ok, "a second Manager.Start must adopt the cleared persisted row")
	at, _, _ = next.promptActivitySnapshot()
	require.True(t, at.IsZero(), "prior metadata must not resurrect a removed clock")
}

func newIdleActivityStartupManager(t *testing.T, repo *tasksqlite.Repository, control *standaloneControlServer) *Manager {
	t.Helper()
	registry := NewExecutorRegistry(newTestLogger())
	registry.Register(control.executor(t))
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", newTestLogger())
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(repo)
	mgr.SetAgentSurvivalEnabled(true)
	return mgr
}

func updateIdleActivityPassiveMessage(t *testing.T, repo *tasksqlite.Repository, taskID, sessionID string, old time.Time) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-idle", TaskID: taskID, TaskSessionID: sessionID, StartedAt: old}))
	message := &models.Message{ID: "message-idle", TaskID: taskID, TaskSessionID: sessionID, TurnID: "turn-idle",
		Content: "settled output", CreatedAt: old, UpdatedAt: old}
	require.NoError(t, repo.CreateMessage(ctx, message))
	require.NoError(t, repo.CompleteTurn(ctx, message.TurnID))
	message.Metadata = map[string]interface{}{"passive": "updated"}
	require.NoError(t, repo.UpdateMessage(ctx, message))
	times, err := repo.GetLastMessageTimeBySessionIDs(ctx, []string{sessionID})
	require.NoError(t, err)
	require.True(t, times[sessionID].After(old), "positive control: transcript metadata really refreshed MAX(updated_at)")
}

// @covers AC-EXECUTORS-IDLE-PARKING-001.11
func TestIdleActivityRemoteConstructorUsesPersistedOwnerClock(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{
		agentruntime.RuntimeSSH, agentruntime.RuntimeSprites, agentruntime.RuntimeRemoteDocker,
		agentruntime.RuntimeKubernetes, agentruntime.RuntimePluginRemote,
	} {
		t.Run(string(runtime), func(t *testing.T) {
			ctx := context.Background()
			repo := newSettingsSourceRecoveryRepository(t, "task", "session")
			mgr := newTestManager(t)
			registerRecoveryTestAgentProfile(t, mgr)
			mgr.SetExecutorRunningWriter(repo)
			e := createTestExecution("execution", "task", "session")
			e.RuntimeName, e.AgentProfileID = runtime, recoveryTestAgentProfileID
			old := time.Now().UTC().Add(-time.Hour)
			e.lastActivityAt = old
			require.NoError(t, mgr.persistExecutorRunningResult(ctx, e))
			row, err := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
			require.NoError(t, err)
			ri := &ExecutorInstance{InstanceID: e.ID, TaskID: e.TaskID, SessionID: e.SessionID,
				RuntimeName: runtime, AgentProfileID: recoveryTestAgentProfileID,
				Metadata: map[string]interface{}{"connection_only": true}}
			recovered, err := mgr.prepareRecoveredRemoteExecution(ctx, row, task08RemoteSnapshot(row), ri)
			require.NoError(t, err)
			at, _, _ := recovered.promptActivitySnapshot()
			require.True(t, at.Equal(old), "adapter connection metadata must not replace the lifecycle clock")
			require.NoError(t, mgr.persistExecutorRunningResult(ctx, recovered))
			next, err := repo.GetExecutorRunningBySessionID(ctx, e.SessionID)
			require.NoError(t, err)
			require.Equal(t, row.Metadata[idleActivityMetadataKey], next.Metadata[idleActivityMetadataKey])
		})
	}
}

func TestIdleActivityMetadataIsIsolatedAndRemovalIsAuthoritative(t *testing.T) {
	e := createTestExecution("execution", "task", "session")
	e.RuntimeName = executor.NameStandalone
	stale := map[string]interface{}{"execution_id": e.ID, "activity_at": time.Now().Add(-time.Hour).Format(time.RFC3339Nano)}
	e.metadata = map[string]interface{}{idleActivityMetadataKey: stale}
	prior := &models.ExecutorRunning{Metadata: map[string]interface{}{idleActivityMetadataKey: stale}}
	row := buildRunningFromExecution(e, prior)
	require.NotContains(t, row.Metadata, idleActivityMetadataKey)
	require.Contains(t, e.MetadataSnapshot(), idleActivityMetadataKey, "mapping must not mutate execution metadata")
	e.markAgentActivity()
	row = buildRunningFromExecution(e, prior)
	require.Contains(t, row.Metadata, idleActivityMetadataKey)
	require.Contains(t, FilterPersistentMetadata(row.Metadata), idleActivityMetadataKey)
	require.True(t, IsSessionScopedMetadataKey(idleActivityMetadataKey),
		"sibling sessions must not inherit the activity clock")
}
