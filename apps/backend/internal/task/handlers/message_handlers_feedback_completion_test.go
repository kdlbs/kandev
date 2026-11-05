package handlers

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type feedbackCompletionCoordinator struct {
	*feedbackAdmissionCoordinator
	mu          sync.Mutex
	pending     bool
	prompts     chan string
	completed   chan error
	done        chan struct{}
	holdFirst   chan struct{}
	releaseOnce sync.Once
}

func (o *feedbackCompletionCoordinator) MarkInitialTaskBriefDispatchPending(string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pending = true
}

func (o *feedbackCompletionCoordinator) InitialTaskBriefDispatchPending(string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.pending
}

func (o *feedbackCompletionCoordinator) CompleteInitialTaskBriefDispatch(ctx context.Context, taskID, sessionID string) {
	defer close(o.done)
	o.mu.Lock()
	o.pending = false
	o.mu.Unlock()
	o.completed <- o.drainFeedback(ctx, taskID, sessionID)
}

func (o *feedbackCompletionCoordinator) NotifyQueuedUserPrompt(ctx context.Context, taskID, sessionID string) {
	if !o.InitialTaskBriefDispatchPending(sessionID) {
		_ = o.drainFeedback(ctx, taskID, sessionID)
	}
}

func (o *feedbackCompletionCoordinator) PromptTask(
	_ context.Context, _, _, prompt, _ string, _ bool, _ []v1.MessageAttachment, _ bool,
) (*orchestrator.PromptResult, error) {
	o.prompts <- prompt
	if strings.Contains(prompt, "Winning first message") {
		<-o.holdFirst
	}
	return &orchestrator.PromptResult{}, nil
}

func (o *feedbackCompletionCoordinator) drainFeedback(ctx context.Context, taskID, sessionID string) error {
	identity, err := o.queue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		return err
	}
	queued, found, _, err := o.queue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !found {
		return err
	}
	if _, err := o.PromptTask(ctx, taskID, sessionID, queued.Content, queued.Model, queued.PlanMode, nil, false); err != nil {
		return err
	}
	return o.queue.AcknowledgeQueuedForSession(ctx, identity, queued)
}

func newFeedbackCompletionHarness(t *testing.T, waitForCompletion bool) (feedbackAdmissionFixture, *feedbackCompletionCoordinator, *MessageHandlers) {
	t.Helper()
	fixture := newFeedbackAdmissionFixture(t, "Keep the initial task objective")
	coordinator := &feedbackCompletionCoordinator{
		feedbackAdmissionCoordinator: fixture.coordinator,
		prompts:                      make(chan string, 2), completed: make(chan error, 1),
		done: make(chan struct{}), holdFirst: make(chan struct{}),
	}
	t.Cleanup(func() {
		coordinator.releaseOnce.Do(func() { close(coordinator.holdFirst) })
		if !waitForCompletion {
			return
		}
		select {
		case <-coordinator.done:
		case <-time.After(5 * time.Second):
			t.Error("initial brief completion did not finish")
		}
	})
	return fixture, coordinator, NewMessageHandlers(fixture.handler.service, coordinator, fixture.handler.logger)
}

func receiveFeedbackPrompt(t *testing.T, coordinator *feedbackCompletionCoordinator) string {
	t.Helper()
	select {
	case prompt := <-coordinator.prompts:
		return prompt
	case <-time.After(5 * time.Second):
		t.Fatal("accepted feedback did not reach the prompt boundary")
		return ""
	}
}

func assertFeedbackCompletion(t *testing.T, fixture feedbackAdmissionFixture, coordinator *feedbackCompletionCoordinator) {
	t.Helper()
	select {
	case err := <-coordinator.completed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not complete the initial brief dispatch")
	}
	require.False(t, coordinator.InitialTaskBriefDispatchPending(feedbackSessionID))
	require.Zero(t, fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID).Count)
}

// @covers AC-TASKS-PLAN-COMMENTS-002.2
func TestWSAddMessagePlanCommentAdmissionNotifiesReceiptDelivery(t *testing.T) {
	fixture, coordinator, handler := newFeedbackCompletionHarness(t, false)

	response, err := handler.wsAddMessage(t.Context(), feedbackAdmissionRequest(t, 1))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))

	require.Contains(t, receiveFeedbackPrompt(t, coordinator), "Apply this change")
	require.False(t, coordinator.InitialTaskBriefDispatchPending(feedbackSessionID))
	require.Zero(t, fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID).Count)
}

// @covers AC-TASKS-PLAN-COMMENTS-002.2
func TestWSAddMessagePlanCommentInitialBriefCompletionDrainsDeferredContender(t *testing.T) {
	fixture, coordinator, handler := newFeedbackCompletionHarness(t, true)
	winning, err := ws.NewRequest("winning-request", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": feedbackTaskID, "session_id": feedbackSessionID,
		"client_message_id": "winning-message", "content": "Winning first message",
	})
	require.NoError(t, err)
	response, err := handler.wsAddMessage(t.Context(), winning)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	require.Contains(t, receiveFeedbackPrompt(t, coordinator), "Winning first message")
	require.True(t, coordinator.InitialTaskBriefDispatchPending(feedbackSessionID))

	response, err = handler.wsAddMessage(t.Context(), feedbackAdmissionRequest(t, 1))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	require.Equal(t, 1, fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID).Count)
	select {
	case prompt := <-coordinator.prompts:
		t.Fatalf("feedback overtook the initial brief: %q", prompt)
	default:
	}

	coordinator.releaseOnce.Do(func() { close(coordinator.holdFirst) })
	require.Contains(t, receiveFeedbackPrompt(t, coordinator), "Apply this change")
	assertFeedbackCompletion(t, fixture, coordinator)
}
