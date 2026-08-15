package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

type clarificationSummaryProjectorStub struct {
	err    error
	events []*bus.Event
}

func (s *clarificationSummaryProjectorStub) HandleEvent(_ context.Context, event *bus.Event) error {
	s.events = append(s.events, event)
	return s.err
}

func TestPublishClarificationBundleUpdatesRequiresAcknowledgedProjection(t *testing.T) {
	eventBus := NewMockEventBus()
	svc := NewService(Repos{}, eventBus, logger.Default(), RepositoryDiscoveryConfig{})
	projector := &clarificationSummaryProjectorStub{err: errors.New("projection unavailable")}
	svc.SetTaskStatusSummaryEventProjector(projector)
	message := &models.Message{
		ID: "message-restore", TaskID: "task-restore", TaskSessionID: "session-restore",
		Type: models.MessageTypeClarificationRequest, RequestsInput: true,
		Metadata: map[string]interface{}{"pending_id": "pending-restore", "status": "pending"},
	}

	err := svc.PublishClarificationBundleUpdates(context.Background(), []*models.Message{message})
	if err == nil || !errors.Is(err, projector.err) {
		t.Fatalf("PublishClarificationBundleUpdates error = %v, want projection error", err)
	}
	if len(projector.events) != 1 {
		t.Fatalf("projected events = %d, want 1", len(projector.events))
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("published events = %d, want none before projection acknowledgement", len(eventBus.GetPublishedEvents()))
	}
}

func TestPublishClarificationBundleUpdatesPublishesAfterProjection(t *testing.T) {
	eventBus := NewMockEventBus()
	svc := NewService(Repos{}, eventBus, logger.Default(), RepositoryDiscoveryConfig{})
	projector := &clarificationSummaryProjectorStub{}
	svc.SetTaskStatusSummaryEventProjector(projector)
	message := &models.Message{
		ID: "message-restore", TaskID: "task-restore", TaskSessionID: "session-restore",
		Type: models.MessageTypeClarificationRequest, RequestsInput: true,
		Metadata: map[string]interface{}{"pending_id": "pending-restore", "status": "pending"},
	}

	if err := svc.PublishClarificationBundleUpdates(context.Background(), []*models.Message{message}); err != nil {
		t.Fatalf("PublishClarificationBundleUpdates: %v", err)
	}
	if len(projector.events) != 1 || len(eventBus.GetPublishedEvents()) != 1 {
		t.Fatalf("projected/published events = %d/%d, want 1/1",
			len(projector.events), len(eventBus.GetPublishedEvents()))
	}
}

func TestPublishClarificationBundleUpdatesPublishesTerminalRowsWithoutProjection(t *testing.T) {
	eventBus := NewMockEventBus()
	svc := NewService(Repos{}, eventBus, logger.Default(), RepositoryDiscoveryConfig{})
	message := &models.Message{
		ID: "message-answered", TaskID: "task-answered", TaskSessionID: "session-answered",
		Type:     models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{"pending_id": "pending-answered", "status": "answered"},
	}

	if err := svc.PublishClarificationBundleUpdates(context.Background(), []*models.Message{message}); err != nil {
		t.Fatalf("PublishClarificationBundleUpdates: %v", err)
	}
	if len(eventBus.GetPublishedEvents()) != 1 {
		t.Fatalf("published events = %d, want 1", len(eventBus.GetPublishedEvents()))
	}
}
