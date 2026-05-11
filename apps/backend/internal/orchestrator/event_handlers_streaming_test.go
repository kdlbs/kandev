package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

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

// TestToolEventsWakeSessionAndTaskTogether locks in the fix for the
// REVIEW + RUNNING split: when an out-of-turn tool event (e.g. a Monitor
// watcher firing after on_turn_complete moved the task to REVIEW) wakes
// the session from WAITING_FOR_INPUT, the task must flip to IN_PROGRESS
// in lockstep instead of being left at REVIEW.
func TestToolEventsWakeSessionAndTaskTogether(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		fire func(*Service)
	}{
		{
			name: "tool_call event",
			fire: func(svc *Service) {
				svc.handleToolCallEvent(ctx, &lifecycle.AgentStreamEventPayload{
					TaskID:    "t1",
					SessionID: "s1",
					Data: &lifecycle.AgentStreamEventData{
						ToolCallID: "tc1",
						ToolStatus: "running",
					},
				})
			},
		},
		{
			name: "tool_update completion event",
			fire: func(svc *Service) {
				svc.handleToolUpdateEvent(ctx, &lifecycle.AgentStreamEventPayload{
					TaskID:    "t1",
					SessionID: "s1",
					Data: &lifecycle.AgentStreamEventData{
						ToolCallID: "tc1",
						ToolStatus: agentEventComplete,
					},
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			// Simulate post-on_turn_complete state: session WAITING, task REVIEW.
			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateWaitingForInput
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			taskRepo := newMockTaskRepo()
			svc := createTestService(repo, newMockStepGetter(), taskRepo)
			svc.messageCreator = &mockMessageCreator{}

			tc.fire(svc)

			updatedSession, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateRunning, updatedSession.State,
				"session should be woken to RUNNING")
			require.Equal(t, v1.TaskStateInProgress, taskRepo.updatedStates["t1"],
				"task must move to IN_PROGRESS in lockstep — leaving it at REVIEW is the bug")
		})

		t.Run(tc.name+" does not clobber terminal session", func(t *testing.T) {
			// Inverse edge case: a buffered tool event arriving after the
			// session is already terminal must NOT silently flip tasks.state
			// to IN_PROGRESS while the session itself stays terminal.
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")

			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateCancelled
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			taskRepo := newMockTaskRepo()
			svc := createTestService(repo, newMockStepGetter(), taskRepo)
			svc.messageCreator = &mockMessageCreator{}

			tc.fire(svc)

			updatedSession, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateCancelled, updatedSession.State,
				"terminal session must not be revived by a stale tool event")
			_, taskWritten := taskRepo.updatedStates["t1"]
			require.False(t, taskWritten,
				"task state must not be clobbered when session is terminal")
		})
	}
}
