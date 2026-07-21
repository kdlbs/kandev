package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/workflow/models"
)

type recordingWorkflowEventBus struct {
	subject string
	event   *bus.Event
}

func (b *recordingWorkflowEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.subject = subject
	b.event = event
	return nil
}

func (b *recordingWorkflowEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *recordingWorkflowEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *recordingWorkflowEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (b *recordingWorkflowEventBus) Close() {}

func (b *recordingWorkflowEventBus) IsConnected() bool { return true }

func TestPublishWorkflowStepEventPublishesDeletedStepPayload(t *testing.T) {
	eventBus := &recordingWorkflowEventBus{}
	h := NewHandlers(nil, eventBus, logger.Default())

	h.publishWorkflowStepEvent(context.Background(), events.WorkflowStepDeleted, &models.WorkflowStep{
		ID:         "step-deleted",
		WorkflowID: "workflow-1",
		Name:       "Deleted",
		Position:   2,
	})

	if eventBus.subject != events.WorkflowStepDeleted {
		t.Fatalf("subject = %q, want %q", eventBus.subject, events.WorkflowStepDeleted)
	}
	if eventBus.event == nil {
		t.Fatal("expected event to be published")
	}
	if eventBus.event.Source != "workflow-handlers" {
		t.Fatalf("source = %q, want workflow-handlers", eventBus.event.Source)
	}
	data, ok := eventBus.event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map", eventBus.event.Data)
	}
	step, ok := data["step"].(map[string]interface{})
	if !ok {
		t.Fatalf("step data type = %T, want map", data["step"])
	}
	if step["id"] != "step-deleted" || step["workflow_id"] != "workflow-1" {
		t.Fatalf("unexpected step payload: %#v", step)
	}
}
