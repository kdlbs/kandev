package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/logger"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotCoreRecorderWithoutPlugins(t *testing.T) {
	h := newBootStateTestHarness(t)
	require.NoError(t, h.taskRepo.CreateWorkspace(t.Context(), &models.Workspace{ID: "snapshot-workspace", Name: "Workspace"}))
	require.NoError(t, h.taskRepo.CreateTask(t.Context(), &models.Task{ID: "snapshot-task", Title: "Task", WorkspaceID: "snapshot-workspace"}))
	require.NoError(t, h.taskRepo.CreateExecutor(t.Context(), &models.Executor{ID: "snapshot-executor", Name: "Executor", Type: models.ExecutorTypeLocal}))
	require.NoError(t, h.taskRepo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "snapshot-session", TaskID: "snapshot-task", State: models.TaskSessionStateStarting, ExecutorID: "snapshot-executor"}))
	require.NoError(t, h.taskRepo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "snapshot-session", SessionID: "snapshot-session", TaskID: "snapshot-task", AgentExecutionID: "execution", ExecutorID: "snapshot-executor", Status: models.ExecutorRunningStatusStarting}))
	input := models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: "snapshot-session", TaskID: "snapshot-task", ExecutionID: "execution", Agent: launchwrapper.SummarizeCommand([]string{"agent"})}
	execution := &lifecycle.AgentExecution{ID: input.ExecutionID, TaskID: input.TaskID, SessionID: input.SessionID}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	var published []*bus.Event
	_, err = eventBus.Subscribe(">", func(_ context.Context, event *bus.Event) error {
		published = append(published, event)
		return nil
	})
	require.NoError(t, err)
	changed := 0
	recorder := executionSnapshotRecorder{tasks: h.taskRepo, eventBus: eventBus, changed: func() { changed++ }}
	require.NoError(t, recorder.record(t.Context(), execution, input, nil))
	session, err := h.taskRepo.GetTaskSession(t.Context(), input.SessionID)
	require.NoError(t, err)
	require.NotNil(t, session.ExecutionSnapshot)
	require.Equal(t, "snapshot-executor", session.ExecutionSnapshot.ExecutorID)
	require.Nil(t, session.ExecutionSnapshot.Sandbox)
	require.Zero(t, changed, "core wrappers do not invoke the sandbox observer")
	require.Len(t, published, 1, "capture must immediately publish execution details without a lifecycle transition")
	require.Equal(t, "task_session.execution_changed", published[0].Type)
	payload := published[0].Data.(map[string]any)
	require.Equal(t, session.ExecutionSnapshot, payload["execution_snapshot"])
	require.Equal(t, session.QueueIncarnationID, payload["queue_incarnation_id"])
	require.Equal(t, "snapshot-workspace", payload["workspace_id"])
	require.NotContains(t, payload, "new_state")
	require.Contains(t, payload, "execution_observation")
	require.Nil(t, payload["execution_observation"])
	input.AttemptID = "selected-attempt"
	execution.SandboxSelection = &sandbox.SelectionContext{OrgID: "org"}
	require.Error(t, recorder.record(t.Context(), execution, input, nil))
	require.Error(t, recorder.record(t.Context(), execution, input, &contract.Launch{AttemptID: "missing"}))
	session, err = h.taskRepo.GetTaskSession(t.Context(), input.SessionID)
	require.NoError(t, err)
	require.Equal(t, "attempt", session.ExecutionSnapshot.AttemptID)
	require.Len(t, published, 1, "rejected capture must not publish")
}

func verifyPreparedExecutionSnapshot(t *testing.T, tasks *taskrepo.Repository, policies *sandbox.Service, execution *lifecycle.AgentExecution, launch *contract.Launch) {
	t.Helper()
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &models.TaskSession{ID: execution.SessionID, TaskID: execution.TaskID, State: models.TaskSessionStateStarting, ExecutorID: "local"}))
	require.NoError(t, tasks.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: execution.SessionID, SessionID: execution.SessionID, TaskID: execution.TaskID, AgentExecutionID: execution.ID, ExecutorID: "local", Status: models.ExecutorRunningStatusStarting}))
	input := models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: launch.AttemptID, SessionID: execution.SessionID, TaskID: execution.TaskID, ExecutionID: execution.ID, AgentProfileID: execution.AgentProfileID, Agent: launchwrapper.SummarizeCommand(execution.AgentArgs)}
	changed := 0
	recorder := executionSnapshotRecorder{tasks: tasks, policies: policies, changed: func() { changed++ }}
	require.NoError(t, recorder.record(t.Context(), execution, input, launch))
	session, err := tasks.GetTaskSession(t.Context(), execution.SessionID)
	require.NoError(t, err)
	captured := session.ExecutionSnapshot
	require.NotNil(t, captured)
	require.NotNil(t, captured.Sandbox)
	private, err := policies.ReadAttempt(t.Context(), execution.SandboxSelection.OrgID, launch.AttemptID)
	require.NoError(t, err)
	require.Equal(t, private.Snapshot.Provider.RuntimeDigest, captured.Sandbox.RuntimeDigest)
	require.Equal(t, private.Snapshot.Provider.PackageDigest, captured.Sandbox.PackageDigest)
	require.Equal(t, private.Snapshot.Resolved.Revision.ID, captured.Sandbox.RevisionID)
	require.Equal(t, launch.PolicyDigest, captured.Sandbox.PolicyDigest)
	require.Equal(t, launch.Generation, captured.Sandbox.Generation)
	input.AttemptID = "foreign-attempt"
	require.Error(t, recorder.record(t.Context(), execution, input, launch), "public and private attempt identities must agree")
	session, err = tasks.GetTaskSession(t.Context(), execution.SessionID)
	require.NoError(t, err)
	require.Equal(t, captured, session.ExecutionSnapshot)
	require.Equal(t, 1, changed, "only committed sandbox capture wakes the observer")
}
