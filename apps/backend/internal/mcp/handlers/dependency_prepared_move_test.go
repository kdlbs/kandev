package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	runtimeagentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-TASK-DEPENDENCIES-001.1
func TestMoveTaskAfterDependencyPreparedLaunch(t *testing.T) {
	t.Run("unlimited", func(t *testing.T) { testMoveTaskAfterDependencyPreparedLaunch(t, false) })
	t.Run("full_ceiling", func(t *testing.T) { testMoveTaskAfterDependencyPreparedLaunch(t, true) })
}

func testMoveTaskAfterDependencyPreparedLaunch(t *testing.T, fullCeiling bool) {
	ctx := context.Background()
	taskSvc, repo, wfCtrl, wfRepo, database, eventBus := newTestTaskServiceWithWorkflowDBAndEventBus(t)
	blockers, err := officesqlite.NewWithDB(database, database, testLogger(t))
	require.NoError(t, err)
	taskSvc.SetBlockerRepository(blockers)
	defaultExecutor := models.ExecutorIDWorktree
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-dep", Name: "Dependencies", DefaultExecutorID: &defaultExecutor, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-dep", WorkspaceID: "ws-dep", Name: "Chain", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "predecessor", WorkspaceID: "ws-dep", WorkflowID: "wf-dep", Title: "A", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "dependent", WorkspaceID: "ws-dep", WorkflowID: "wf-dep", WorkflowStepID: "src-dep", Title: "B", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			"intent": "start", "agent_profile_id": "profile-dep", "prompt": "dependency launch brief", models.DeferredLaunchStartWhenUnblockedKey: true,
		}},
	}))
	require.NoError(t, taskSvc.AddDependency(ctx, "dependent", "predecessor"))
	source := &wfmodels.WorkflowStep{ID: "src-dep", WorkflowID: "wf-dep", Name: "Work", Position: 0, CreatedAt: now, UpdatedAt: now, Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}}
	destination := &wfmodels.WorkflowStep{ID: "dst-dep", WorkflowID: "wf-dep", Name: "Done", Position: 1, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, wfRepo.CreateStep(ctx, source))
	require.NoError(t, wfRepo.CreateStep(ctx, destination))
	mgr := &dependencyMoveAgentManager{repo: repo, workspacePath: t.TempDir(), prepared: make(chan struct{}), started: make(chan struct{})}
	log := testLogger(t)
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(func(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
		session, readErr := repo.GetTaskSession(ctx, sessionID)
		if readErr != nil {
			return messagequeue.QueueSessionIdentity{}, readErr
		}
		if session == nil || session.TaskID != taskID || session.QueueIncarnationID == "" {
			return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
		}
		return messagequeue.QueueSessionIdentity{TaskID: taskID, SessionID: sessionID, SessionIncarnationID: session.QueueIncarnationID}, nil
	})
	q := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	config := orchestrator.DefaultServiceConfig()
	if fullCeiling {
		config.SessionCapacity = 1
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "occupier", TaskID: "predecessor", AgentProfileID: "profile-dep", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}))
	}
	orch := orchestrator.NewService(config, eventBus, mgr, &mcpReadinessSchedulerTaskRepo{repo: repo}, repo, nil, nil, q, log)
	orch.SetWorkflowStepGetter(&peerMessageWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{"src-dep": source, "dst-dep": destination}})
	orch.SetTaskDependencyReader(taskSvc)
	orch.SetTurnService(taskSvc)
	require.NoError(t, orch.StartEventWatcher(ctx))
	t.Cleanup(func() { require.NoError(t, orch.StopEventWatcher()) })
	prepared, err := orch.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{TaskID: "dependent", Intent: orchestrator.IntentStart, AgentProfileID: "profile-dep", AutoStart: true})
	require.NoError(t, err)
	require.Equal(t, string(models.TaskSessionStateCreated), prepared.State)
	select {
	case <-mgr.prepared:
	case <-time.After(3 * time.Second):
		t.Fatal("workspace preparation did not reach runtime")
	}
	if fullCeiling {
		observation, readErr := orch.CurrentSessionCeilingObservation(ctx)
		require.NoError(t, readErr)
		require.True(t, observation.Known)
		require.Equal(t, 1, observation.InUse)
		require.Equal(t, 1, observation.Limit)
	}
	require.NoError(t, repo.UpdateTaskState(ctx, "predecessor", v1.TaskStateCompleted))
	require.NoError(t, eventBus.Publish(ctx, events.TaskStateChanged, bus.NewEvent(events.TaskStateChanged, "test", map[string]interface{}{"task_id": "predecessor", "new_state": v1.TaskStateCompleted})))
	select {
	case <-mgr.started:
	case <-time.After(3 * time.Second):
		t.Fatal("resolved dependency did not start agent")
	}
	h := &Handlers{taskSvc: taskSvc, taskRepo: repo, workflowCtrl: wfCtrl, messageQueue: q, logger: log.WithFields()}
	response, err := h.handleMoveTask(ctx, makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "dependent", "workflow_id": "wf-dep", "workflow_step_id": "dst-dep", "position": 0,
	}))
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, response.Type, "running dependency recipient must defer its move: %s", response.Payload)
	pending, exists, readErr := q.GetPendingMoveWithError(ctx, prepared.SessionID)
	require.NoError(t, readErr)
	require.True(t, exists)
	require.Equal(t, "dependent", pending.TaskID)
	require.Equal(t, "dst-dep", pending.WorkflowStepID)
	sessions, err := repo.ListTaskSessions(ctx, "dependent")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, sessions[0].QueueIncarnationID, pending.SessionIncarnationID)
	activeTurn, err := taskSvc.GetActiveTurn(ctx, prepared.SessionID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	task, err := repo.GetTask(ctx, "dependent")
	require.NoError(t, err)
	require.Equal(t, "src-dep", task.WorkflowStepID)
	if fullCeiling {
		require.NotNil(t, sessions[0].Metadata["ceiling_manual_override"])
		require.False(t, models.HasCeilingDeferredIntent(task), "dependency consumer keeps manual admission at capacity")
		require.False(t, models.HasStartWhenUnblockedIntent(task), "admitted launch must consume its intent")
	}
}

type dependencyMoveAgentManager struct {
	executor.AgentManagerClient
	repo          *sqliterepo.Repository
	workspacePath string
	prepared      chan struct{}
	started       chan struct{}
}

func (m *dependencyMoveAgentManager) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	err := m.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: req.SessionID, TaskID: req.TaskID, SessionID: req.SessionID, Status: "ready", AgentExecutionID: "exec-" + req.SessionID})
	if err != nil {
		return nil, err
	}
	if !req.StartAgent {
		close(m.prepared)
	}
	return &executor.LaunchAgentResponse{AgentExecutionID: "exec-" + req.SessionID, Status: v1.AgentStatusStarting, WorkspacePath: m.workspacePath}, nil
}
func (m *dependencyMoveAgentManager) StartAgentProcess(context.Context, string) error {
	close(m.started)
	return nil
}
func (m *dependencyMoveAgentManager) GetExecutionIDForSession(ctx context.Context, sessionID string) (string, error) {
	running, err := m.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if running == nil {
		return "", fmt.Errorf("no execution")
	}
	return running.AgentExecutionID, nil
}
func (*dependencyMoveAgentManager) ResolveAgentProfile(_ context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{ProfileID: profileID}, nil
}
func (*dependencyMoveAgentManager) IsAgentRunningForSession(context.Context, string) bool {
	return false
}
func (*dependencyMoveAgentManager) IsPassthroughSession(context.Context, string) bool { return false }
func (*dependencyMoveAgentManager) SetExecutionDescription(context.Context, string, string) error {
	return nil
}

func (*dependencyMoveAgentManager) WaitForAgentctlReady(context.Context, string) error { return nil }
func (*dependencyMoveAgentManager) GetGitStatus(context.Context, string) (*runtimeagentctl.GitStatusResult, error) {
	return nil, nil
}
func (*dependencyMoveAgentManager) GetGitStatusFresh(context.Context, string) (*runtimeagentctl.GitStatusResult, error) {
	return nil, nil
}

func (*dependencyMoveAgentManager) SetExecutionEnv(context.Context, string, map[string]string) error {
	return nil
}
func (*dependencyMoveAgentManager) SetMcpMode(context.Context, string, string) error { return nil }
func (*dependencyMoveAgentManager) IsAgentCommandConfigured(string) bool             { return true }
