package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Isolate an actual lock cycle so a regression cannot leak blocked production
// goroutines into the parent test process or hold its SQLite cleanup open.
// @covers AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.2
func TestCeilingReplayBootReadyAndCancellationProgress(t *testing.T) {
	const childKey = "KANDEV_TEST_CEILING_PROGRESS_CHILD"
	if mode := os.Getenv(childKey); mode != "" {
		runCeilingReplayProgress(t, mode == "async")
		return
	}
	for _, mode := range []string{"sync", "async"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			binary, err := os.Executable()
			require.NoError(t, err)
			cmd := exec.CommandContext(ctx, binary,
				"-test.run=^TestCeilingReplayBootReadyAndCancellationProgress$", "-test.timeout=20s")
			cmd.Env = append(os.Environ(), childKey+"="+mode)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
		})
	}
}

type ceilingReplayReadBarrier struct {
	*sqliterepo.Repository
	taskID           string
	entered, release chan struct{}
	once             sync.Once
}

func (r *ceilingReplayReadBarrier) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if id == r.taskID && ceilingEntryAdmissionLockHeld(ctx, id) {
		record, _, err := r.GetTaskDeferredLaunch(ctx, id)
		if err != nil {
			return nil, err
		}
		if _, _, claimed := models.ReadCeilingLaunchClaim(record); claimed {
			r.once.Do(func() { close(r.entered); <-r.release })
		}
	}
	return r.Repository.GetTask(ctx, id)
}

// As with NATS, publication acknowledges delivery scheduling, not completion
// of the local subscriber. Different subjects can execute concurrently.
type ceilingAsyncBus struct {
	bus.EventBus
	wg sync.WaitGroup
}

func (b *ceilingAsyncBus) dispatch(handler bus.EventHandler) bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		copy := *event
		b.wg.Add(1)
		go func() { defer b.wg.Done(); _ = handler(ctx, &copy) }()
		return nil
	}
}

func (b *ceilingAsyncBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return b.EventBus.Subscribe(subject, b.dispatch(handler))
}

func (b *ceilingAsyncBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return b.EventBus.QueueSubscribe(subject, queue, b.dispatch(handler))
}

func runCeilingReplayProgress(t *testing.T, async bool) {
	f := newCeilingDispatchFixture(t)
	ctx := context.Background()
	seedCeilingProgressCompanions(t, f)
	var eventBus bus.EventBus = bus.NewMemoryEventBus(testLogger())
	if async {
		asyncBus := &ceilingAsyncBus{EventBus: eventBus}
		eventBus = asyncBus
		t.Cleanup(asyncBus.wg.Wait)
	}
	t.Cleanup(eventBus.Close)
	f.svc.eventBus = eventBus
	f.svc.taskRepo = newTaskServiceStateRepository(f.repo, eventBus)
	f.svc.turnService = &repoTurnService{repo: f.repo}
	_, err := f.svc.turnService.StartTurn(ctx, "cancel-session")
	require.NoError(t, err)
	f.agent.cancelAgentEntered = make(chan struct{}, 1)
	stream := &blockingToolCallMessageCreator{entered: make(chan struct{}), release: make(chan struct{})}
	f.svc.messageCreator = stream
	replay := &ceilingReplayReadBarrier{Repository: f.repo, taskID: f.task.ID,
		entered: make(chan struct{}), release: make(chan struct{})}
	f.svc.repo = replay
	streamDone, bootDone := make(chan struct{}), make(chan struct{})
	w := watcher.NewWatcher(eventBus, watcher.EventHandlers{
		OnAgentBootReady: func(ctx context.Context, data watcher.AgentEventData) {
			defer close(bootDone)
			f.svc.handleAgentBootReady(ctx, data)
		},
		OnAgentStreamEvent: func(ctx context.Context, data *lifecycle.AgentStreamEventPayload) {
			defer close(streamDone)
			f.svc.handleAgentStreamEvent(ctx, data)
		},
	}, "ceiling-progress", testLogger())
	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { require.NoError(t, w.Stop()) })
	reviewPublished := observeCeilingCancellationReview(t, eventBus)

	streamPublish := make(chan error, 1)
	go func() {
		streamPublish <- eventBus.Publish(ctx, events.BuildAgentStreamSubject(f.route.DestinationID),
			bus.NewEvent(events.AgentStream, "test", lifecycle.AgentStreamEventPayload{
				TaskID: f.task.ID, SessionID: f.route.DestinationID, ExecutionID: "dispatch-execution",
				Data: &lifecycle.AgentStreamEventData{Type: agentEventToolCall, ToolCallID: "progress-tool", ToolTitle: "Read", ToolStatus: "running"},
			}))
	}()
	awaitCeilingProgress(t, stream.entered, "stream owns its cancellation guard")
	sweepDone := make(chan struct{})
	go func() { defer close(sweepDone); f.svc.drainDeferredCeilingLaunches(ctx) }()
	awaitCeilingProgress(t, replay.entered, "claimed replay owns task admission")
	bootPublish := make(chan error, 1)
	go func() {
		bootPublish <- eventBus.Publish(ctx, events.AgentBootReady, bus.NewEvent(events.AgentBootReady, "test",
			watcher.AgentEventData{TaskID: f.task.ID, SessionID: "boot-session", AgentExecutionID: "boot-execution"}))
	}()
	// Only replay and boot-ready can own/request this task's admission here.
	// Observing the second reference proves boot-ready reached the contention.
	require.Eventually(t, func() bool {
		f.svc.ceilingEntryAdmissionLocksMu.Lock()
		defer f.svc.ceilingEntryAdmissionLocksMu.Unlock()
		entry := f.svc.ceilingEntryAdmissionLocks[f.task.ID]
		return entry != nil && entry.refs == 2
	}, 5*time.Second, time.Millisecond, "boot-ready did not reach task admission")
	cancelDone := make(chan error, 1)
	go func() { cancelDone <- f.svc.CancelAgent(ctx, "cancel-session") }()
	awaitCeilingProgress(t, f.agent.cancelAgentEntered, "unrelated cancellation reached runtime")
	close(replay.release)
	close(stream.release)
	awaitCeilingProgress(t, streamDone, "stream completed")
	awaitCeilingProgress(t, bootDone, "boot-ready completed")
	awaitCeilingProgress(t, sweepDone, "sweep reached the second task")
	require.NoError(t, <-cancelDone)
	require.NoError(t, <-streamPublish)
	require.NoError(t, <-bootPublish)
	awaitCeilingProgress(t, reviewPublished, "cancellation published REVIEW")
	assertCeilingProgressSettled(t, f)
}

func seedCeilingProgressCompanions(t *testing.T, f *ceilingDispatchFixture) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "boot-session", TaskID: f.task.ID, State: models.TaskSessionStateStarting, StartedAt: now, UpdatedAt: now,
	}))
	seedTaskAndSession(t, f.repo, "cancel-task", "cancel-session", models.TaskSessionStateRunning)
	seedExecutorRunning(t, f.repo, "cancel-session", "cancel-task", "cancel-execution")
	seedTaskAndSession(t, f.repo, "zz-second-task", "second-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, f.repo, "second-session", "zz-second-task", "second-execution")
	second, err := f.repo.GetTask(ctx, "zz-second-task")
	require.NoError(t, err)
	second.WorkflowStepID = f.task.WorkflowStepID
	require.NoError(t, f.repo.UpdateTask(ctx, second))
	session, err := f.repo.GetTaskSession(ctx, "second-session")
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	require.NoError(t, f.repo.UpdateTaskSession(ctx, session))
	route := f.route
	route.OperationID, route.DestinationID = "second-route", session.ID
	route.EntryIdentity = f.svc.workflowEntryIdentity(ctx, second.ID)
	require.NoError(t, f.repo.SetTaskMetadataKey(ctx, second.ID, models.MetaKeyWorkflowSessionRoute, route))
	binding := f.binding
	binding.RouteOperationID, binding.DestinationSessionID, binding.EntryIdentity = route.OperationID, session.ID, route.EntryIdentity
	deferral := f.deferral
	deferral.Payload = map[string]interface{}{
		metaKeySessionID: session.ID, metaKeyWorkflowStepID: second.WorkflowStepID,
		models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
	}
	require.NoError(t, f.repo.SetTaskMetadataKey(ctx, second.ID, models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(deferral)))
	seedMockTaskState(f.svc.taskRepo.(*mockTaskRepo), second.ID, v1.TaskStateInProgress)
}

func observeCeilingCancellationReview(t *testing.T, eventBus bus.EventBus) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	var once sync.Once
	_, err := eventBus.Subscribe(events.TaskStateChanged, func(_ context.Context, event *bus.Event) error {
		data, err := json.Marshal(event.Data)
		if err != nil {
			return err
		}
		var update watcher.TaskEventData
		if err := json.Unmarshal(data, &update); err != nil {
			return err
		}
		if update.TaskID == "cancel-task" && update.NewState != nil && *update.NewState == v1.TaskStateReview {
			once.Do(func() { close(done) })
		}
		return nil
	})
	require.NoError(t, err)
	return done
}

func assertCeilingProgressSettled(t *testing.T, f *ceilingDispatchFixture) {
	t.Helper()
	ctx := context.Background()
	task, err := f.repo.GetTask(ctx, "cancel-task")
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, task.State)
	session, err := f.repo.GetTaskSession(ctx, "cancel-session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	turn, err := f.svc.turnService.GetActiveTurn(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, turn)
	require.False(t, f.svc.CancellationPending(session.ID))
	f.agent.mu.Lock()
	prompts := append([]string(nil), f.agent.capturedPrompts...)
	f.agent.mu.Unlock()
	require.Equal(t, []string{"continue"}, prompts, "only the second queued task dispatches")
	record, _, err := f.repo.GetTaskDeferredLaunch(ctx, "zz-second-task")
	require.NoError(t, err)
	_, err = models.ReadCeilingDeferral(record)
	require.Error(t, err, "accepted second task must be removed from the durable queue")
}

func awaitCeilingProgress(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("no progress: %s", what)
	}
}
