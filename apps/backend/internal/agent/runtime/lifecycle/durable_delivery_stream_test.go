package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

type recordingAgentDeliveryRepository struct {
	cursor        *models.AgentDeliveryCursor
	received      []*models.AgentDeliveryEvent
	projected     []*models.AgentDeliveryEvent
	projectErr    error
	projectedHook func()
}

type immutableDeliveryRepository struct {
	recordingAgentDeliveryRepository
}

type effectReadFailureDeliveryRepository struct {
	recordingAgentDeliveryRepository
	effectErr error
}

func (r *effectReadFailureDeliveryRepository) GetAgentDeliveryEffect(
	context.Context, string,
) (*models.AgentDeliveryEffect, error) {
	return nil, r.effectErr
}

type canonicalProjectionFailureDeliveryRepository struct {
	recordingAgentDeliveryRepository
	projectionErr error
}

func (r *canonicalProjectionFailureDeliveryRepository) ProjectCanonicalAgentDeliveryEvent(
	context.Context, *models.AgentDeliveryEvent, *models.AgentDeliveryEffect,
) (models.CanonicalAgentDeliveryProjectionResult, error) {
	return models.CanonicalAgentDeliveryProjectionResult{}, r.projectionErr
}

type canonicalProjectionDeliveryRepository struct {
	recordingAgentDeliveryRepository
}

type canonicalProjectionWithoutIdentityDeliveryRepository struct {
	recordingAgentDeliveryRepository
}

type canonicalBatchProjectionDeliveryRepository struct {
	recordingAgentDeliveryRepository
	batchCalls int
	emptyIDs   bool
}

func (r *canonicalProjectionDeliveryRepository) ProjectCanonicalAgentDeliveryEvent(
	_ context.Context, event *models.AgentDeliveryEvent, _ *models.AgentDeliveryEffect,
) (models.CanonicalAgentDeliveryProjectionResult, error) {
	r.projected = append(r.projected, event)
	return models.CanonicalAgentDeliveryProjectionResult{CanonicalMessageID: "projected-message"}, nil
}

func (r *canonicalProjectionWithoutIdentityDeliveryRepository) ProjectCanonicalAgentDeliveryEvent(
	_ context.Context, event *models.AgentDeliveryEvent, _ *models.AgentDeliveryEffect,
) (models.CanonicalAgentDeliveryProjectionResult, error) {
	r.projected = append(r.projected, event)
	return models.CanonicalAgentDeliveryProjectionResult{}, nil
}

func (r *canonicalBatchProjectionDeliveryRepository) ProjectCanonicalAgentDeliveryEvents(
	_ context.Context, events []*models.AgentDeliveryEvent, _ []*models.AgentDeliveryEffect,
) ([]models.CanonicalAgentDeliveryProjectionResult, error) {
	r.batchCalls++
	r.projected = append(r.projected, events...)
	results := make([]models.CanonicalAgentDeliveryProjectionResult, len(events))
	for index := range events {
		if !r.emptyIDs {
			results[index] = models.CanonicalAgentDeliveryProjectionResult{CanonicalMessageID: "projected-message", Appended: index > 0}
		}
	}
	return results, nil
}

func (r *immutableDeliveryRepository) ReceiveAgentDeliveryEvent(
	ctx context.Context,
	event *models.AgentDeliveryEvent,
	remoteHighWater int64,
) (bool, error) {
	copyEvent := *event
	copyEvent.Payload = append([]byte(nil), event.Payload...)
	return r.recordingAgentDeliveryRepository.ReceiveAgentDeliveryEvent(ctx, &copyEvent, remoteHighWater)
}

func (r *immutableDeliveryRepository) ProjectAgentDeliveryEvent(
	ctx context.Context,
	event *models.AgentDeliveryEvent,
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	if len(r.received) != 1 || !bytes.Equal(r.received[0].Payload, event.Payload) {
		return false, errors.New("durable delivery payload changed between admission and projection")
	}
	return r.recordingAgentDeliveryRepository.ProjectAgentDeliveryEvent(ctx, event, effect)
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
		r.cursor = &models.AgentDeliveryCursor{
			StreamID: event.StreamID, SessionID: event.SessionID,
			IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration,
		}
	}
	r.cursor.ReceivedSequence = event.Sequence
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

	execution, client, acked := deliveryProjectionFixture(t, event, acknowledger)
	defer sm.Wait()
	if err := sm.processPreparedAgentEvent(context.Background(), execution, client, repository,
		preparedAgentEvent{event: event, durableEvent: durableAgentEvent(execution, event)}, 0); err != nil {
		t.Fatalf("project and acknowledge durable event: %v", err)
	}

	select {
	case <-acked:
	case <-time.After(3 * time.Second):
		t.Fatal("delivery was not acknowledged")
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

	execution, client, _ := deliveryProjectionFixture(t, event, acknowledger)
	defer sm.Wait()
	err := sm.processPreparedAgentEvent(context.Background(), execution, client, repository,
		preparedAgentEvent{event: event, durableEvent: durableAgentEvent(execution, event)}, 0)
	if !errors.Is(err, projectionErr) {
		t.Fatalf("error = %v, want projection error", err)
	}

	sm.ackMu.Lock()
	workers := len(sm.ackWorkers)
	sm.ackMu.Unlock()
	if workers != 0 {
		t.Fatal("failed projection scheduled an acknowledgement")
	}

}

func TestDurableAgentEventEffectReadFailureBlocksProcessing(t *testing.T) {
	effectErr := errors.New("effect store unavailable")
	repository := &effectReadFailureDeliveryRepository{
		recordingAgentDeliveryRepository: recordingAgentDeliveryRepository{},
		effectErr:                        effectErr,
	}
	callbackCalled := false
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { callbackCalled = true },
	}, nil, nil)
	event := agentctl.AgentEvent{
		Type:                      streams.EventTypeComplete,
		DeliveryStreamID:          "stream-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 1,
		DeliverySequence:          1,
	}

	err := sm.processAgentEvent(context.Background(), &AgentExecution{
		SessionID: "session-1", DeliveryMode: DurableDeliveryV1,
		DeliveryStreamID: event.DeliveryStreamID, DeliveryIncarnationID: event.DeliveryIncarnationID,
		DeliveryHarnessGeneration: event.DeliveryHarnessGeneration,
	}, nil, repository, event, 0)
	if !errors.Is(err, effectErr) {
		t.Fatalf("error = %v, want effect read error", err)
	}
	if callbackCalled {
		t.Fatal("agent callback ran after effect read failed")
	}
	if len(repository.projected) != 0 {
		t.Fatalf("projected events = %d, want none", len(repository.projected))
	}
}

func TestDurableAgentEventCanonicalProjectionFailureBlocksProcessing(t *testing.T) {
	projectionErr := errors.New("canonical projection unavailable")
	repository := &canonicalProjectionFailureDeliveryRepository{
		recordingAgentDeliveryRepository: recordingAgentDeliveryRepository{},
		projectionErr:                    projectionErr,
	}
	callbackCalled := false
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { callbackCalled = true },
	}, nil, nil)
	event := agentctl.AgentEvent{
		Type:                      streams.EventTypeMessageChunk,
		Text:                      "hello",
		DeliveryStreamID:          "stream-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 1,
		DeliverySequence:          1,
	}

	err := sm.processAgentEvent(context.Background(), &AgentExecution{
		SessionID: "session-1", DeliveryMode: DurableDeliveryV1,
		DeliveryStreamID: event.DeliveryStreamID, DeliveryIncarnationID: event.DeliveryIncarnationID,
		DeliveryHarnessGeneration: event.DeliveryHarnessGeneration,
	}, nil, repository, event, 0)
	if !errors.Is(err, projectionErr) {
		t.Fatalf("error = %v, want canonical projection error", err)
	}
	if callbackCalled {
		t.Fatal("agent callback ran after canonical projection failed")
	}
}

func TestCanonicalProjectionNotifiesWhenAckTransportFails(t *testing.T) {
	repository := &canonicalProjectionDeliveryRepository{}
	callbackCalled := false
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			callbackCalled = event.CanonicalProjection
		},
	}, nil, nil)
	event := agentctl.AgentEvent{
		Type:                      streams.EventTypeMessageChunk,
		Text:                      "already committed",
		DeliveryStreamID:          "stream-ack-failure",
		DeliveryIncarnationID:     "incarnation-ack-failure",
		DeliveryHarnessGeneration: 1,
		DeliverySequence:          1,
	}

	// A nil client models an ACK transport that is unavailable after the
	// canonical repository commit. The callback must still observe the output.
	if err := sm.processAgentEvent(
		context.Background(), &AgentExecution{
			SessionID: "session-1", DeliveryMode: DurableDeliveryV1,
			DeliveryStreamID: event.DeliveryStreamID, DeliveryIncarnationID: event.DeliveryIncarnationID,
			DeliveryHarnessGeneration: event.DeliveryHarnessGeneration,
		}, nil, repository, event, 0,
	); err != nil {
		t.Fatalf("process canonical event: %v", err)
	}
	if !callbackCalled {
		t.Fatal("canonical output callback did not run after ACK transport failure")
	}
	if len(repository.projected) != 1 {
		t.Fatalf("projected events = %d, want 1", len(repository.projected))
	}
}

func TestCanonicalProjectionDoesNotNotifyWithoutCommittedMessageID(t *testing.T) {
	event := agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: "legacy output",
		DeliveryStreamID: "stream-pruned-legacy", DeliveryIncarnationID: "incarnation-pruned-legacy",
		DeliveryHarnessGeneration: 1, DeliverySequence: 1,
	}
	execution := &AgentExecution{
		SessionID: "session-1", DeliveryMode: DurableDeliveryV1,
		DeliveryStreamID: event.DeliveryStreamID, DeliveryIncarnationID: event.DeliveryIncarnationID,
		DeliveryHarnessGeneration: event.DeliveryHarnessGeneration,
	}
	newRepository := &canonicalProjectionDeliveryRepository{}
	var newCallbacks int
	newManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { newCallbacks++ },
	}, nil, nil)
	if err := newManager.processAgentEvent(context.Background(), execution, nil, newRepository, event, 0); err != nil {
		t.Fatalf("process newly committed event: %v", err)
	}
	if newCallbacks != 1 {
		t.Fatalf("new committed message callbacks = %d, want 1", newCallbacks)
	}

	legacyRepository := &canonicalProjectionWithoutIdentityDeliveryRepository{}
	var legacyCallbacks int
	legacyManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { legacyCallbacks++ },
	}, nil, nil)
	if err := legacyManager.processAgentEvent(context.Background(), execution, nil, legacyRepository, event, 0); err != nil {
		t.Fatalf("process already-projected legacy event: %v", err)
	}
	if legacyCallbacks != 0 {
		t.Fatalf("legacy callbacks = %d, want no notification without a committed message ID", legacyCallbacks)
	}
	if len(legacyRepository.projected) != 1 {
		t.Fatalf("legacy projection calls = %d, want 1", len(legacyRepository.projected))
	}
}

func TestCanonicalProjectionBatchNotifiesOnceWithAccumulatedContent(t *testing.T) {
	repository := &canonicalBatchProjectionDeliveryRepository{}
	var callbacks []agentctl.AgentEvent
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			callbacks = append(callbacks, event)
		},
	}, nil, nil)
	execution := &AgentExecution{SessionID: "session-1", DeliveryStreamID: "stream-1"}
	prepared := []preparedAgentEvent{
		{
			event: agentctl.AgentEvent{
				Type:              streams.EventTypeMessageChunk,
				Text:              "first",
				ProtocolMessageID: "message-1",
				DeliveryStreamID:  "stream-1",
				DeliverySequence:  1,
			},
			durableEvent: &models.AgentDeliveryEvent{StreamID: "stream-1", Sequence: 1},
		},
		{
			event: agentctl.AgentEvent{
				Type:              streams.EventTypeMessageChunk,
				Text:              "second",
				ProtocolMessageID: "message-1",
				DeliveryStreamID:  "stream-1",
				DeliverySequence:  2,
			},
			durableEvent: &models.AgentDeliveryEvent{StreamID: "stream-1", Sequence: 2},
		},
	}

	if err := sm.projectCanonicalAgentEvents(
		context.Background(), execution, prepared, repository, nil, 0,
	); err != nil {
		t.Fatalf("project canonical batch: %v", err)
	}
	if repository.batchCalls != 1 {
		t.Fatalf("batch projection calls = %d, want 1", repository.batchCalls)
	}
	if len(callbacks) != 1 {
		t.Fatalf("callbacks = %d, want 1", len(callbacks))
	}
	callback := callbacks[0]
	if callback.Text != "firstsecond" {
		t.Fatalf("callback text = %q, want accumulated content", callback.Text)
	}
	if !callback.CanonicalProjection || callback.CanonicalMessageAppend {
		t.Fatalf("callback projection metadata = projection:%t append:%t, want true:false", callback.CanonicalProjection, callback.CanonicalMessageAppend)
	}
}

func TestCanonicalProjectionBatchSkipsRowsWithoutCommittedMessageID(t *testing.T) {
	prepared := []preparedAgentEvent{{
		event: agentctl.AgentEvent{
			Type: streams.EventTypeMessageChunk, Text: "already-pruned output",
			DeliveryStreamID: "stream-1", DeliverySequence: 1,
		},
		durableEvent: &models.AgentDeliveryEvent{StreamID: "stream-1", Sequence: 1},
	}}
	execution := &AgentExecution{SessionID: "session-1"}
	positiveRepository := &canonicalBatchProjectionDeliveryRepository{}
	var positiveCallbacks []agentctl.AgentEvent
	positiveManager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			positiveCallbacks = append(positiveCallbacks, event)
		},
	}, nil, nil)
	if err := positiveManager.projectCanonicalAgentEvents(context.Background(), execution, prepared, positiveRepository, nil, 0); err != nil {
		t.Fatalf("project canonical positive-control batch: %v", err)
	}
	if len(positiveCallbacks) != 1 || positiveCallbacks[0].Text != "already-pruned output" {
		t.Fatalf("positive-control callbacks = %+v, want the persisted content", positiveCallbacks)
	}

	repository := &canonicalBatchProjectionDeliveryRepository{emptyIDs: true}
	var callbacks []agentctl.AgentEvent
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
			callbacks = append(callbacks, event)
		},
	}, nil, nil)
	if err := sm.projectCanonicalAgentEvents(context.Background(), execution, prepared, repository, nil, 0); err != nil {
		t.Fatalf("project canonical batch: %v", err)
	}
	if len(callbacks) != 0 {
		t.Fatalf("callbacks = %d, want no notification without a committed message ID", len(callbacks))
	}
	if repository.batchCalls != 1 {
		t.Fatalf("batch projection calls = %d, want 1", repository.batchCalls)
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

	process, err := sm.receiveDurableAgentDeliveryEvent(context.Background(), durableAgentEvent(execution, event), repository)
	if err != nil {
		t.Fatalf("receive durable event: %v", err)
	}
	if !process {
		t.Fatal("first durable event was not admitted")
	}
	if err := sm.projectDurableAgentDeliveryEventWithEffect(context.Background(), durableAgentEvent(execution, event), repository, deliveryEffectForEvent(event)); err != nil {
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
	process, err = sm.receiveDurableAgentDeliveryEvent(context.Background(), durableAgentEvent(execution, event), repository)
	if err != nil {
		t.Fatalf("receive duplicate durable event: %v", err)
	}
	if process {
		t.Fatal("projected duplicate was admitted for a second callback")
	}
}

func TestDurableAgentEventProjectionUsesAdmissionSnapshot(t *testing.T) {
	repository := &immutableDeliveryRepository{}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	execution := &AgentExecution{SessionID: "session-1", promptTurnID: "turn-1"}
	event := agentctl.AgentEvent{
		Type:                 "error",
		DeliveryStreamID:     "session-1",
		DeliverySequence:     1,
		DeliverySubmissionID: "submission-1",
	}
	durableEvent := durableAgentEvent(execution, event)
	process, _, effect, err := sm.prepareDurableAgentDeliveryEvent(
		context.Background(), execution, durableEvent, repository, nil,
	)
	if err != nil {
		t.Fatalf("prepare durable event: %v", err)
	}
	if !process {
		t.Fatal("first durable event was not admitted")
	}

	// Lifecycle failure handling can clear or replace the prompt turn before
	// the stream callback reaches the projection step. The admitted payload
	// must remain the record that gets projected and acknowledged.
	execution.setPromptTurnID("turn-2")
	acknowledger := &recordingAgentDeliveryAcknowledger{}

	boundExecution, client, acked := deliveryProjectionFixture(t, event, acknowledger)
	boundExecution.setPromptTurnID("turn-2")
	defer sm.Wait()
	if err := sm.processPreparedAgentEvent(context.Background(), boundExecution, client, repository,
		preparedAgentEvent{event: event, durableEvent: durableEvent, deliveryEffect: effect}, 0); err != nil {
		t.Fatalf("project and acknowledge durable event: %v", err)
	}

	select {
	case <-acked:
	case <-time.After(3 * time.Second):
		t.Fatal("admitted delivery was not acknowledged")
	}
	if len(acknowledger.acknowledged) != 1 {
		t.Fatal("expected one acknowledgement")
	}

}

func TestDeliveryEffectAllowsSuccessfulRetryAfterTerminalFailure(t *testing.T) {
	streamID := "stream-managed-runtime-retry"
	errorEvent := agentctl.AgentEvent{
		Type:                 streams.EventTypeError,
		TurnID:               "turn-managed-runtime-retry",
		DeliveryStreamID:     streamID,
		DeliverySequence:     1,
		DeliverySubmissionID: "prompt-managed-runtime-retry",
	}
	completeEvent := errorEvent
	completeEvent.Type = streams.EventTypeComplete
	completeEvent.DeliverySequence = 2

	failureEffect := deliveryEffectForEvent(errorEvent)
	successEffect := deliveryEffectForEvent(completeEvent)
	if failureEffect == nil || successEffect == nil {
		t.Fatal("terminal events must produce delivery effects")
	}
	if failureEffect.EffectKey == successEffect.EffectKey {
		t.Fatalf("failure and successful retry share effect key %q", failureEffect.EffectKey)
	}
	if got, want := failureEffect.EffectKey, "agent_delivery.event:"+streamID+":1"; got != want {
		t.Fatalf("failure effect key = %q, want %q", got, want)
	}
	if got, want := successEffect.EffectKey, "agent_delivery.event:"+streamID+":2"; got != want {
		t.Fatalf("success effect key = %q, want %q", got, want)
	}
	storedEvent := &models.AgentDeliveryEvent{
		StreamID: streamID, Sequence: 2, EventType: streams.EventTypeComplete,
		Payload: []byte(`{"type":"complete","turn_id":"turn-managed-runtime-retry"}`),
	}
	if storedEffect := deliveryEffectForDeliveryEvent(storedEvent); storedEffect.EffectKey != successEffect.EffectKey {
		t.Fatalf("persisted completion effect = %q, want %q", storedEffect.EffectKey, successEffect.EffectKey)
	}
}

type terminalSettlingDeliveryRepository struct {
	recordingAgentDeliveryRepository
	outcome models.DeliverySubmissionState
}

func (r *terminalSettlingDeliveryRepository) SettleAgentDeliveryTerminal(_ context.Context, _ string, _ int64, outcome models.DeliverySubmissionState, _ time.Time) (bool, error) {
	r.outcome = outcome
	return true, nil
}

func TestProjectionSettlesTerminalBeforeLifecycleCanSuppressCallback(t *testing.T) {
	for _, outcome := range []models.DeliverySubmissionState{models.DeliverySubmissionCompleted, models.DeliverySubmissionCancelled, models.DeliverySubmissionFailed} {
		t.Run(string(outcome), func(t *testing.T) {
			repo := &terminalSettlingDeliveryRepository{}
			event := &models.AgentDeliveryEvent{StreamID: "stream", Sequence: 1, SubmissionID: "submission", EventType: streams.EventTypeComplete, Terminal: true}
			if outcome == models.DeliverySubmissionFailed {
				event.EventType = streams.EventTypeError
			}
			if outcome == models.DeliverySubmissionCancelled {
				event.Payload = []byte(`{"data":{"stop_reason":"cancelled"}}`)
			}
			if err := (&StreamManager{}).projectDurableAgentDeliveryEventWithEffect(context.Background(), event, repo, nil); err != nil {
				t.Fatal(err)
			}
			if repo.outcome != outcome {
				t.Fatalf("terminal outcome = %q, want %q", repo.outcome, outcome)
			}
		})
	}
}

func deliveryProjectionFixture(t *testing.T, event agentctl.AgentEvent, ack *recordingAgentDeliveryAcknowledger) (*AgentExecution, *agentctl.Client, <-chan struct{}) {
	t.Helper()
	acked := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         "session-1", IncarnationID: "owner", HarnessGeneration: 1, StreamID: event.DeliveryStreamID,
				Stream: &journal.Stream{SessionID: "session-1", IncarnationID: "owner", HarnessGeneration: 1, StreamID: event.DeliveryStreamID, HighWater: event.DeliverySequence},
			})
			return
		}
		var request struct {
			Sequence uint64 `json:"sequence"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if err := ack.AcknowledgeDelivery(r.Context(), event.DeliveryStreamID, request.Sequence); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
		acked <- struct{}{}
	}))
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil {
		t.Fatal(err)
	}
	client := agentctl.NewClient(endpoint.Hostname(), port, newTestLogger())
	execution := &AgentExecution{ID: "projection-test", SessionID: "session-1", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: event.DeliveryStreamID, DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, agentctl: client}
	return execution, client, acked
}
