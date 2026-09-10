package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestHandleAgentReadyPassthroughPublishesStatusAfterReservation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-passthrough-status", "session-passthrough-status", "step1")
	agentMgr := &mockAgentManager{
		isPassthrough:          true,
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-passthrough-status", v1.TaskStateReview)
	steps := newMockStepGetter()
	steps.steps["step1"] = &models.WorkflowStep{ID: "step1", WorkflowID: "wf1"}
	svc := createTestServiceWithAgent(repo, steps, taskRepo, agentMgr)
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	if _, err := svc.messageQueue.QueueMessage(
		ctx,
		"session-passthrough-status",
		"task-passthrough-status",
		"prompt",
		"",
		messagequeue.QueuedByUser,
		false,
		nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}

	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID:    "task-passthrough-status",
		SessionID: "session-passthrough-status",
	})

	var queueStatusEvents int
	for _, event := range recorded.events {
		if event.subject == events.MessageQueueStatusChanged {
			queueStatusEvents++
		}
	}
	if queueStatusEvents != 1 {
		t.Fatalf("queue status events after passthrough reservation = %d, want 1", queueStatusEvents)
	}
}
func TestPublishQueueStatusDetachesCancelledRequest(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-cancelled-status", "session-cancelled-status", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.PublishQueueStatusEvent(ctx, "session-cancelled-status")

	if len(recorded.events) != 1 {
		t.Fatalf("queue status events = %d, want 1", len(recorded.events))
	}
	if err := recorded.events[0].ctx.Err(); err != nil {
		t.Fatalf("queue status event context error = %v, want nil", err)
	}
}
