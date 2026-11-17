package backendapp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officedashboard "github.com/kandev/kandev/internal/office/dashboard"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/pkg/api/v1"
)

type officeStateEventCaptureWriter struct {
	shared.TaskStateWriter
	mu      sync.Mutex
	receipt taskmodels.TaskStateChange
}

func (w *officeStateEventCaptureWriter) CommitTaskState(
	ctx context.Context, taskID string, write taskmodels.TaskStateWrite,
) (taskmodels.TaskStateChange, error) {
	change, err := w.TaskStateWriter.CommitTaskState(ctx, taskID, write)
	if err == nil {
		w.mu.Lock()
		w.receipt = change
		w.mu.Unlock()
	}
	return change, err
}

func (w *officeStateEventCaptureWriter) lastReceipt() taskmodels.TaskStateChange {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.receipt
}

type officeStateEventCounts struct {
	mu     sync.Mutex
	counts map[string]int
}

type officeStateEventWorkflowGetter struct {
	svc *workflowservice.Service
}

func (g officeStateEventWorkflowGetter) GetStep(ctx context.Context, stepID string) (*models.WorkflowStep, error) {
	return g.svc.GetStep(ctx, stepID)
}

func (g officeStateEventWorkflowGetter) GetNextStepByPosition(
	ctx context.Context, workflowID string, position int,
) (*models.WorkflowStep, error) {
	return g.svc.GetNextStepByPosition(ctx, workflowID, position)
}

func (g officeStateEventWorkflowGetter) GetPreviousStepByPosition(
	ctx context.Context, workflowID string, position int,
) (*models.WorkflowStep, error) {
	return g.svc.GetPreviousStepByPosition(ctx, workflowID, position)
}

func (g officeStateEventWorkflowGetter) GetWorkflowMeta(
	ctx context.Context, workflowID string,
) (orchestrator.WorkflowMeta, error) {
	meta, err := g.svc.GetWorkflowMeta(ctx, workflowID)
	if err != nil {
		return orchestrator.WorkflowMeta{}, err
	}
	return orchestrator.WorkflowMeta{AgentProfileID: meta.AgentProfileID, Prompt: meta.Prompt}, nil
}

func (c *officeStateEventCounts) record(_ context.Context, event *bus.Event) error {
	var data struct {
		TaskID string `json:"task_id"`
	}
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return err
	}
	c.mu.Lock()
	c.counts[data.TaskID]++
	c.mu.Unlock()
	return nil
}

func (c *officeStateEventCounts) count(taskID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[taskID]
}

func TestOfficeStateEventIntegration(t *testing.T) {
	ctx := context.Background()
	h := newPluginMoveErrorBindingHarness(t)
	taskSvc, ok := h.adapter.svc.(*taskservice.Service)
	require.True(t, ok, "the integration harness must use the real task service")
	log := logger.Default()
	h.workflowSvc.SetWorkflowProvider(&workflowProviderAdapter{svc: taskSvc})
	officeRepo, err := officesqlite.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	_, err = h.db.Exec(`UPDATE workspaces SET office_workflow_id = ? WHERE id = ?`, "wf-home", "ws-1")
	require.NoError(t, err)

	// One dashboard mutation and the task-service publisher feed every consumer
	// below through the same production-shaped event bus.
	writer := &officeStateEventCaptureWriter{TaskStateWriter: h.repo}
	dashboard := officedashboard.NewDashboardService(officeRepo, log, nil, nil, nil)
	dashboard.SetTaskStateWriter(writer)
	dashboard.SetTaskStateChangePublisher(taskSvc)
	dashboard.SetTaskLifecyclePublisher(taskSvc)
	dashboard.SetEventBus(h.eventBus)
	taskSvc.SetTaskStateActivityLogger(dashboard)
	counts := &officeStateEventCounts{counts: make(map[string]int)}
	stateSub, err := h.eventBus.Subscribe(events.TaskStateChanged, counts.record)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stateSub.Unsubscribe() })

	// Parent completion is consumed by the orchestrator's real event watcher.
	require.NoError(t, h.workflowSvc.CreateStep(ctx, &models.WorkflowStep{
		ID: "state-parent-wait", WorkflowID: "wf-home", Name: "Wait for children", Position: 1,
		Events: models.StepEvents{OnChildrenCompleted: []models.GenericAction{{Type: models.GenericActionMoveToNext}}},
	}))
	require.NoError(t, h.workflowSvc.CreateStep(ctx, &models.WorkflowStep{
		ID: "state-parent-done", WorkflowID: "wf-home", Name: "Children complete", Position: 2,
	}))
	require.NoError(t, h.workflowSvc.CreateStep(ctx, &models.WorkflowStep{
		ID: "state-office-done", WorkflowID: "wf-home", Name: "Office complete", Position: 3,
	}))
	orchestratorSvc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), h.eventBus, newStubAgentManager(),
		&taskRepositoryAdapter{repo: h.repo, svc: taskSvc}, h.repo,
		nil, nil, nil, log,
	)
	orchestratorSvc.SetWorkflowStepGetter(officeStateEventWorkflowGetter{svc: h.workflowSvc})
	taskSvc.SetBlockerRepository(officeRepo)
	orchestratorSvc.SetTaskDependencyReader(taskSvc)
	require.NoError(t, orchestratorSvc.StartEventWatcher(ctx))
	t.Cleanup(func() { require.NoError(t, orchestratorSvc.StopEventWatcher()) })

	createOfficeStateEventTask(t, h, "parent-task", "wf-home", "state-parent-wait", v1.TaskStateInProgress, nil)
	require.NoError(t, h.repo.CreateTask(ctx, &taskmodels.Task{
		ID: "child-task", WorkspaceID: "ws-1", WorkflowID: "wf-target", WorkflowStepID: "step-target",
		ParentID: "parent-task", Title: "child-task", State: v1.TaskStateInProgress,
	}))
	require.NoError(t, h.repo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "parent-session", TaskID: "parent-task", State: taskmodels.TaskSessionStateRunning, IsPrimary: true,
	}))

	// The Office subscriber owns the durable handoff report. Registration uses
	// the same service subscriber path as backendapp wiring.
	handoffSource := map[string]interface{}{
		"source_task_id": "handoff-source", "source_workspace_id": "ws-1",
		"source_session_id": "handoff-session", "source_agent_profile_id": "handoff-agent",
		"handed_off_at": "2026-11-12T00:00:00Z",
	}
	createOfficeStateEventTask(t, h, "handoff-source", "wf-home", "step-home", v1.TaskStateInProgress, nil)
	createOfficeStateEventTask(t, h, "handoff-delivery", "wf-target", "step-target", v1.TaskStateInProgress,
		map[string]interface{}{taskmodels.MetaKeyHandoffSource: handoffSource})
	officeSvc := officeservice.NewService(officeservice.ServiceOptions{Repo: officeRepo, EventBus: h.eventBus, Logger: log})
	officeSvc.SetHandoffCompletionPolicy(func(context.Context, officeservice.HandoffCompletionSource) (officeservice.HandoffCompletionAuthorization, error) {
		return officeservice.HandoffCompletionAuthorization{Report: true}, nil
	})
	officeSvc.SetSyncHandlers(true)
	require.NoError(t, officeSvc.RegisterEventSubscribers(h.eventBus))

	// The coordinator wake recorder consumes the same committed event and
	// derives ownership and completion from its stores.
	createOfficeStateEventTask(t, h, "coordinator-task", "wf-home", "state-office-done", v1.TaskStateInProgress, nil)
	coordinatorStore, err := coordinator.NewStore(h.db, h.db)
	require.NoError(t, err)
	coord := &coordinator.Coordinator{ID: "state-event-coordinator", WorkspaceID: "ws-1", Name: "State event coordinator", AgentProfileID: "agent", ExecutorProfileID: "executor"}
	require.NoError(t, coordinatorStore.CreateCoordinator(ctx, coord))
	_, err = h.db.Exec(`UPDATE coordinators SET autonomy_enabled = 1 WHERE id = ?`, coord.ID)
	require.NoError(t, err)
	proposal := &coordinator.Proposal{
		CoordinatorID: coord.ID, WorkspaceID: "ws-1", Kind: "create_task",
		Spec: coordinator.ProposalSpec{Title: "Owned task", WorkflowID: "wf-home", StepID: "state-office-done"},
	}
	require.NoError(t, coordinatorStore.InsertProposal(ctx, proposal, true))
	_, err = h.db.Exec(`UPDATE coordinator_proposals SET status = ?, task_id = ? WHERE id = ?`,
		string(coordinator.ProposalStatusApproved), "coordinator-task", proposal.ID)
	require.NoError(t, err)
	coordinatorSvc := coordinator.NewService(coordinatorStore, nil, nil, log, coordinator.WithPhase3(true))
	coordinatorSvc.StartWakeRecorder(h.eventBus, &coordinatorWakeReader{tasks: taskSvc})
	t.Cleanup(coordinatorSvc.StopWakeRecorder)

	// The plugin provider registers its production wildcard bridge from the
	// internal event bus into the scoped web-app event hub.
	createOfficeStateEventTask(t, h, "plugin-task", "wf-target", "step-target", v1.TaskStateInProgress, nil)
	pluginSvc, pluginCleanup, pluginStoreErrors := plugins.ProvideWithStoreErrors(
		&config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{Canvases: true}},
		db.NewPool(h.db, h.db), nil, h.eventBus, log,
	)
	require.NotNil(t, pluginSvc)
	t.Cleanup(func() { require.NoError(t, pluginCleanup()) })
	require.NoError(t, pluginStoreErrors["plugin-instances"])
	require.NoError(t, pluginSvc.Instances().Create(ctx, instances.Instance{
		ID: "state-event-canvas", PluginID: "test-plugin", SourceKind: instances.SourceLocalCanvas,
		ScopeKind: instances.ScopeTask, WorkspaceID: "ws-1", TaskID: "plugin-task", Status: instances.StatusActive,
	}))
	pluginEvents, err := pluginSvc.WebAppEventHub().Subscribe(ctx, webapp.EventSubscriptionRequest{
		InstanceID: "state-event-canvas", UserID: "state-event-user",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pluginEvents.Close() })

	complete := func(taskID string) taskmodels.TaskStateChange {
		t.Helper()
		require.NoError(t, dashboard.UpdateTaskStatus(ctx, officedashboard.TaskStatusUpdateRequest{
			TaskID: taskID, NewStatus: "done",
		}))
		change := writer.lastReceipt()
		require.True(t, change.Matched, "dashboard state writer must commit task %s", taskID)
		require.True(t, change.Changed, "dashboard state writer must transition task %s", taskID)
		require.Equal(t, 1, counts.count(taskID), "one canonical event must follow the dashboard transition")
		return change
	}
	replay := func(change taskmodels.TaskStateChange) {
		t.Helper()
		taskSvc.PublishCommittedTaskStateChange(ctx, change, true)
	}

	childReceipt := complete("child-task")
	var parent *taskmodels.Task
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		parent, err = h.repo.GetTask(ctx, "parent-task")
		if err == nil && parent.WorkflowStepID == "state-parent-done" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil || parent == nil || parent.WorkflowStepID != "state-parent-done" {
		var storedParentID, childState string
		_ = h.db.Get(&storedParentID, `SELECT COALESCE(parent_id, '') FROM tasks WHERE id = ?`, "child-task")
		_ = h.db.Get(&childState, `SELECT state FROM tasks WHERE id = ?`, "child-task")
		step, stepErr := h.workflowSvc.GetStep(ctx, "state-parent-wait")
		t.Fatalf("parent did not advance: parent=%+v err=%v child_parent=%q child_state=%q wait_step=%+v step_err=%v",
			parent, err, storedParentID, childState, step, stepErr)
	}
	require.NoError(t, err)
	parentTransitionID := parent.WorkflowStepTransitionID
	replay(childReceipt)
	parent, err = h.repo.GetTask(ctx, "parent-task")
	require.NoError(t, err)
	require.Equal(t, "state-parent-done", parent.WorkflowStepID)
	require.Equal(t, parentTransitionID, parent.WorkflowStepTransitionID, "duplicate delivery must not repeat the parent transition")
	require.Equal(t, 2, counts.count("child-task"))

	handoffReceipt := complete("handoff-delivery")
	comments, err := officeRepo.ListTaskComments(ctx, "handoff-source")
	require.NoError(t, err)
	require.Len(t, comments, 1, "handoff completion subscriber must persist one report")
	replay(handoffReceipt)
	comments, err = officeRepo.ListTaskComments(ctx, "handoff-source")
	require.NoError(t, err)
	require.Len(t, comments, 1, "duplicate delivery must not duplicate the handoff report")
	require.Equal(t, 2, counts.count("handoff-delivery"))

	coordinatorReceipt := complete("coordinator-task")
	assertCoordinatorCompletionWake(t, h, coordinatorStore, coord.ID)
	replay(coordinatorReceipt)
	assertCoordinatorCompletionWake(t, h, coordinatorStore, coord.ID)
	require.Equal(t, 2, counts.count("coordinator-task"))

	pluginReceipt := complete("plugin-task")
	firstPluginEvent := receiveOfficeStatePluginEvent(t, pluginEvents.Events())
	require.Equal(t, events.TaskStateChanged, firstPluginEvent.Type)
	require.Equal(t, "ws-1", firstPluginEvent.Scope.WorkspaceID)
	require.Equal(t, "plugin-task", firstPluginEvent.Scope.TaskID)
	var firstData map[string]interface{}
	require.NoError(t, json.Unmarshal(firstPluginEvent.Data, &firstData))
	require.Equal(t, "COMPLETED", firstData["state"])
	replay(pluginReceipt)
	secondPluginEvent := receiveOfficeStatePluginEvent(t, pluginEvents.Events())
	require.Equal(t, events.TaskStateChanged, secondPluginEvent.Type)
	require.Greater(t, secondPluginEvent.Sequence, firstPluginEvent.Sequence, "each bus delivery must be forwarded in sequence")
	require.Equal(t, 2, counts.count("plugin-task"))
}

func createOfficeStateEventTask(
	t *testing.T,
	h *pluginMoveErrorBindingHarness,
	id, workflowID, stepID string,
	state v1.TaskState,
	metadata map[string]interface{},
) {
	t.Helper()
	require.NoError(t, h.repo.CreateTask(context.Background(), &taskmodels.Task{
		ID: id, WorkspaceID: "ws-1", WorkflowID: workflowID, WorkflowStepID: stepID,
		Title: id, State: state, Metadata: metadata,
	}))
}

func assertCoordinatorCompletionWake(
	t *testing.T,
	h *pluginMoveErrorBindingHarness,
	store *coordinator.Store,
	coordinatorID string,
) {
	t.Helper()
	var count int
	require.NoError(t, h.db.Get(&count, `SELECT COUNT(*) FROM coordinator_wakes WHERE coordinator_id = ? AND task_id = ? AND kind = ? AND episode_key = ?`,
		coordinatorID, "coordinator-task", string(coordinator.WakeKindCompleted), "completed"))
	require.Equal(t, 1, count, "the real coordinator recorder must store one completion episode")
	result, err := store.RecordWake(context.Background(), coordinatorID, "coordinator-task", coordinator.WakeKindCompleted, "completed")
	require.NoError(t, err)
	require.Equal(t, coordinator.WakeExists, result.Outcome, "the wake must be durable before its canonical event is observed")
}

func receiveOfficeStatePluginEvent(t *testing.T, eventCh <-chan webapp.Event) webapp.Event {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		select {
		case event, ok := <-eventCh:
			require.True(t, ok, "plugin event stream must remain open")
			if event.Type == events.TaskStateChanged {
				return event
			}
		case <-timeout:
			t.Fatal("timed out waiting for scoped plugin task-state event")
			return webapp.Event{}
		}
	}
}
