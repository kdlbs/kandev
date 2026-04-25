package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// recordingTaskEventPublisher records calls to PublishTaskUpdated so tests
// can assert that the sidebar-driving event fired.
type recordingTaskEventPublisher struct {
	mu     sync.Mutex
	taskID []string
}

func (p *recordingTaskEventPublisher) PublishTaskUpdated(_ context.Context, task *models.Task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if task != nil {
		p.taskID = append(p.taskID, task.ID)
	}
}

// recordingEventBus records published events for assertions.
type recordingEventBus struct {
	events []recordedEvent
}

type recordedEvent struct {
	subject string
	event   *bus.Event
}

func (b *recordingEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.events = append(b.events, recordedEvent{subject: subject, event: event})
	return nil
}
func (b *recordingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *recordingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *recordingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (b *recordingEventBus) Close()            {}
func (b *recordingEventBus) IsConnected() bool { return true }

func TestHandleSessionModeEvent(t *testing.T) {
	t.Run("publishes plan mode", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: "plan"},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("publishes default mode without available modes (mode exit)", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: "default"},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("publishes default mode with available modes (initial state)", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data: &lifecycle.AgentStreamEventData{
				CurrentModeID: "default",
				AvailableModes: []streams.SessionModeInfo{
					{ID: "default", Name: "Default"},
					{ID: "plan", Name: "Plan"},
				},
			},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("publishes empty mode (mode exit)", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			AgentID:   "a1",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: ""},
		})

		require.Len(t, eb.events, 1)
	})

	t.Run("skips when session ID is empty", func(t *testing.T) {
		eb := &recordingEventBus{}
		svc := &Service{logger: testLogger(), eventBus: eb}

		svc.handleSessionModeEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "",
			Data:      &lifecycle.AgentStreamEventData{CurrentModeID: "plan"},
		})

		require.Empty(t, eb.events)
	})
}

// TestUpdateTaskSessionState_PromotesNonPrimaryRunningSession is the
// regression test for the sidebar bug: when a task already has a primary
// session that is idle (WAITING_FOR_INPUT) and the user opens a second chat
// tab, the new session is non-primary. As it transitions to RUNNING the
// sidebar must reflect "in progress" — which only happens if the running
// session becomes primary (the sidebar reads primary_session_state) and a
// task.updated event fires.
func TestUpdateTaskSessionState_PromotesNonPrimaryRunningSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	// Seed workspace, workflow, task.
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", CreatedAt: now, UpdatedAt: now}))

	// sessionA: existing primary, idle (WAITING_FOR_INPUT, "Turn Finished").
	sessionA := &models.TaskSession{ID: "sA", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, IsPrimary: true, StartedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTaskSession(ctx, sessionA))
	require.NoError(t, repo.SetSessionPrimary(ctx, "sA"))

	// sessionB: newly opened chat tab, non-primary, just started.
	sessionB := &models.TaskSession{ID: "sB", TaskID: "t1", State: models.TaskSessionStateStarting, StartedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTaskSession(ctx, sessionB))

	publisher := &recordingTaskEventPublisher{}
	svc := &Service{logger: testLogger(), repo: repo, taskEvents: publisher}

	// The new tab's session transitions to RUNNING.
	svc.updateTaskSessionState(ctx, "t1", "sB", models.TaskSessionStateRunning, "", true)

	// sessionB must now be primary so the sidebar's primary_session_state
	// reflects RUNNING ("in progress" bucket).
	updatedB, err := repo.GetTaskSession(ctx, "sB")
	require.NoError(t, err)
	require.True(t, updatedB.IsPrimary, "the running session should be promoted to primary")

	updatedA, err := repo.GetTaskSession(ctx, "sA")
	require.NoError(t, err)
	require.False(t, updatedA.IsPrimary, "the previously-primary idle session should lose primary")

	// task.updated must fire so the sidebar receives the new state.
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	require.Contains(t, publisher.taskID, "t1", "task.updated should be published for the task")
}

// TestUpdateTaskSessionState_DoesNotStealFromActivePrimary guards the inverse:
// if the current primary is itself RUNNING, a second session also reaching
// RUNNING must not steal primary. Otherwise concurrent sessions would
// flip-flop the primary indicator.
func TestUpdateTaskSessionState_DoesNotStealFromActivePrimary(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", CreatedAt: now, UpdatedAt: now}))

	sessionA := &models.TaskSession{ID: "sA", TaskID: "t1", State: models.TaskSessionStateRunning, IsPrimary: true, StartedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTaskSession(ctx, sessionA))
	require.NoError(t, repo.SetSessionPrimary(ctx, "sA"))

	sessionB := &models.TaskSession{ID: "sB", TaskID: "t1", State: models.TaskSessionStateStarting, StartedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTaskSession(ctx, sessionB))

	svc := &Service{logger: testLogger(), repo: repo, taskEvents: &recordingTaskEventPublisher{}}

	svc.updateTaskSessionState(ctx, "t1", "sB", models.TaskSessionStateRunning, "", true)

	updatedA, err := repo.GetTaskSession(ctx, "sA")
	require.NoError(t, err)
	require.True(t, updatedA.IsPrimary, "active primary must not be replaced by another running session")

	updatedB, err := repo.GetTaskSession(ctx, "sB")
	require.NoError(t, err)
	require.False(t, updatedB.IsPrimary)
}
