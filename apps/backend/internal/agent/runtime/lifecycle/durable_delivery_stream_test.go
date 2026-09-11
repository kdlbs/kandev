package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type recordingAgentDeliveryRepository struct {
	cursor        *models.AgentDeliveryCursor
	received      []*models.AgentDeliveryEvent
	projected     []*models.AgentDeliveryEvent
	projectErr    error
	projectedHook func()
}

func (r *recordingAgentDeliveryRepository) ReceiveAgentDeliveryEvent(_ context.Context, event *models.AgentDeliveryEvent, _ int64) (bool, error) {
	for _, received := range r.received {
		if received.StreamID == event.StreamID && received.Sequence == event.Sequence {
			return false, nil
		}
	}
	r.received = append(r.received, event)
	return true, nil
}

func (r *recordingAgentDeliveryRepository) GetAgentDeliveryCursor(_ context.Context, _ string) (*models.AgentDeliveryCursor, error) {
	if r.cursor == nil {
		return &models.AgentDeliveryCursor{}, nil
	}
	return r.cursor, nil
}

func (r *recordingAgentDeliveryRepository) ProjectAgentDeliveryEvent(_ context.Context, event *models.AgentDeliveryEvent, _ *models.AgentDeliveryEffect) (bool, error) {
	r.projected = append(r.projected, event)
	if r.projectedHook != nil {
		r.projectedHook()
	}
	if r.projectErr != nil {
		return false, r.projectErr
	}
	if r.cursor == nil {
		r.cursor = &models.AgentDeliveryCursor{}
	}
	r.cursor.ProjectedSequence = event.Sequence
	return true, nil
}

type recordingAgentDeliveryAcknowledger struct {
	acknowledged  []string
	ackErr        error
	onAcknowledge func()
}

func (r *recordingAgentDeliveryAcknowledger) AcknowledgeDelivery(_ context.Context, streamID string, sequence uint64) error {
	if r.onAcknowledge != nil {
		r.onAcknowledge()
	}
	r.acknowledged = append(r.acknowledged, streamID+":"+fmt.Sprint(sequence))
	return r.ackErr
}

func TestDurableAgentEventAcknowledgesOnlyAfterInboxProjection(t *testing.T) {
	order := make([]string, 0, 2)
	repository := &recordingAgentDeliveryRepository{
		projectedHook: func() { order = append(order, "project") },
	}
	acknowledger := &recordingAgentDeliveryAcknowledger{
		onAcknowledge: func() { order = append(order, "ack") },
	}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	event := agentctl.AgentEvent{Type: "complete", DeliveryStreamID: "stream-1", DeliverySequence: 3}

	if err := sm.projectAndAcknowledgeDurableAgentEvent(
		context.Background(), &AgentExecution{SessionID: "session-1"}, event, repository, acknowledger,
	); err != nil {
		t.Fatalf("project and acknowledge durable event: %v", err)
	}
	if got, want := fmt.Sprint(order), "[project ack]"; got != want {
		t.Fatalf("operation order = %s, want %s", got, want)
	}
	if got, want := acknowledger.acknowledged, []string{"stream-1:3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("acknowledged = %v, want %v", got, want)
	}
}

func TestDurableAgentEventProjectionFailureDoesNotAcknowledge(t *testing.T) {
	projectionErr := errors.New("projection unavailable")
	repository := &recordingAgentDeliveryRepository{projectErr: projectionErr}
	acknowledger := &recordingAgentDeliveryAcknowledger{}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	event := agentctl.AgentEvent{Type: "complete", DeliveryStreamID: "stream-1", DeliverySequence: 3}

	err := sm.projectAndAcknowledgeDurableAgentEvent(
		context.Background(), &AgentExecution{SessionID: "session-1"}, event, repository, acknowledger,
	)
	if !errors.Is(err, projectionErr) {
		t.Fatalf("error = %v, want projection error", err)
	}
	if len(acknowledger.acknowledged) != 0 {
		t.Fatalf("acknowledged = %v, want no acknowledgments", acknowledger.acknowledged)
	}
}

func TestDeliveryReplayCursorUsesProjectedSequence(t *testing.T) {
	repository := &recordingAgentDeliveryRepository{
		cursor: &models.AgentDeliveryCursor{StreamID: "session-1", ProjectedSequence: 7},
	}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)

	got, err := sm.deliveryReplayCursor(context.Background(), repository, "session-1")
	if err != nil {
		t.Fatalf("replay cursor error = %v", err)
	}
	if got != 7 {
		t.Fatalf("replay cursor = %d, want 7", got)
	}
}

func TestDurableAgentEventAdmissionProjectsOpaquePayload(t *testing.T) {
	repository := &recordingAgentDeliveryRepository{}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	execution := &AgentExecution{SessionID: "session-1"}
	event := agentctl.AgentEvent{
		Type:                 "message_chunk",
		Text:                 "hello",
		DeliveryStreamID:     "session-1",
		DeliverySequence:     1,
		DeliverySubmissionID: "submission-1",
	}

	process, err := sm.receiveDurableAgentEvent(context.Background(), execution, event, repository)
	if err != nil {
		t.Fatalf("receive durable event: %v", err)
	}
	if !process {
		t.Fatal("first durable event was not admitted")
	}
	if err := sm.projectDurableAgentEvent(context.Background(), execution, event, repository); err != nil {
		t.Fatalf("project durable event: %v", err)
	}
	if len(repository.received) != 1 || len(repository.projected) != 1 {
		t.Fatalf("received/projected = %d/%d, want 1/1", len(repository.received), len(repository.projected))
	}
	projected := repository.projected[0]
	if projected.SessionID != "session-1" || projected.StreamID != "session-1" || projected.Sequence != 1 {
		t.Fatalf("projected identity = %+v", projected)
	}
	var payload agentctl.AgentEvent
	if err := json.Unmarshal(projected.Payload, &payload); err != nil {
		t.Fatalf("decode opaque payload: %v", err)
	}
	if payload.DeliverySequence != 0 || payload.DeliveryStreamID != "" {
		t.Fatalf("payload contains transport cursor: %+v", payload)
	}

	repository.cursor.UpdatedAt = time.Now()
	process, err = sm.receiveDurableAgentEvent(context.Background(), execution, event, repository)
	if err != nil {
		t.Fatalf("receive duplicate durable event: %v", err)
	}
	if process {
		t.Fatal("projected duplicate was admitted for a second callback")
	}
}
