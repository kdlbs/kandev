package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestInboxAckAfterCommitDeduplicatesMessages(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	event := &models.AgentDeliveryEvent{
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		StreamID:          "stream-1",
		Sequence:          1,
		EventType:         "assistant_message",
		Payload:           []byte("hello"),
	}
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	if err != nil || !inserted {
		t.Fatalf("first inbox insert = %v, err=%v", inserted, err)
	}
	inserted, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	if err != nil || inserted {
		t.Fatalf("duplicate inbox insert = %v, err=%v", inserted, err)
	}
	cursor, err := repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ReceivedSequence != 1 || cursor.ProjectedSequence != 0 {
		t.Fatalf("cursor before projection = %+v", cursor)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, event, &models.AgentDeliveryEffect{
		EffectKey:  "message:stream-1:1",
		StreamID:   event.StreamID,
		Sequence:   event.Sequence,
		EffectType: "message",
	}); err != nil {
		t.Fatal(err)
	}
	projectedEffect, err := repo.GetAgentDeliveryEffect(ctx, "message:stream-1:1")
	if err != nil {
		t.Fatal(err)
	}
	if projectedEffect.State != models.DeliveryEffectPending || projectedEffect.CompletedAt != nil {
		t.Fatalf("projected effect = %+v, want pending intent", projectedEffect)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, event, &models.AgentDeliveryEffect{
		EffectKey:  "message:stream-1:1",
		StreamID:   event.StreamID,
		Sequence:   event.Sequence,
		EffectType: "message",
	}); err != nil {
		t.Fatal(err)
	}
	cursor, err = repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ProjectedSequence != 1 {
		t.Fatalf("cursor after projection = %+v", cursor)
	}
	events, err := repo.ListUnprojectedAgentDeliveryEvents(ctx, event.StreamID, 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("unprojected events = %#v, err=%v", events, err)
	}
}

func TestInboxRejectsConflictingDuplicateSequence(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	event := &models.AgentDeliveryEvent{
		SessionID:         "session-conflict",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 1,
		StreamID:          "stream-conflict",
		Sequence:          1,
		SubmissionID:      "submission-1",
		EventType:         "assistant_message",
		Payload:           []byte("first"),
	}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
		t.Fatalf("first inbox insert = %v, err=%v", inserted, err)
	}
	conflict := *event
	conflict.Payload = []byte("different")
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &conflict, 1); !errors.Is(err, repoerrors.ErrAgentDeliveryEventConflict) || inserted {
		t.Fatalf("conflicting inbox insert = %v, err=%v", inserted, err)
	}
	events, err := repo.ListUnprojectedAgentDeliveryEvents(ctx, event.StreamID, 10)
	if err != nil || len(events) != 1 || string(events[0].Payload) != "first" {
		t.Fatalf("stored event after conflict = %#v, err=%v", events, err)
	}
}

func TestProjectionRejectsMismatchedEventIdentity(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	event := &models.AgentDeliveryEvent{
		SessionID:         "session-projection",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 2,
		StreamID:          "stream-projection",
		Sequence:          1,
		EventType:         "assistant_message",
		Payload:           []byte("authoritative"),
	}
	if _, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil {
		t.Fatal(err)
	}
	mismatched := *event
	mismatched.HarnessGeneration = 3
	if projected, err := repo.ProjectAgentDeliveryEvent(ctx, &mismatched, &models.AgentDeliveryEffect{
		EffectKey: "effect-projection-conflict", StreamID: event.StreamID, Sequence: event.Sequence, EffectType: "message",
	}); !errors.Is(err, repoerrors.ErrAgentDeliveryEventConflict) || projected {
		t.Fatalf("mismatched projection = %v, err=%v", projected, err)
	}
	cursor, err := repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ProjectedSequence != 0 {
		t.Fatalf("projected cursor after conflict = %+v", cursor)
	}
}

func TestCanonicalAgentDeliveryProjectionPersistsNewlineFreeChunksExactlyOnce(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-canonical", "session-canonical", "turn-canonical")

	for sequence, text := range []string{"hello", " world"} {
		sequenceNumber := int64(sequence + 1)
		wire := streams.AgentEvent{
			Type:               streams.EventTypeMessageChunk,
			Text:               text,
			TurnID:             "turn-canonical",
			CanonicalMessageID: "canonical-message-1",
		}
		payload, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		event := &models.AgentDeliveryEvent{
			SessionID:         "session-canonical",
			IncarnationID:     "incarnation-canonical",
			HarnessGeneration: 1,
			StreamID:          "stream-canonical",
			Sequence:          sequenceNumber,
			EventType:         streams.EventTypeMessageChunk,
			Payload:           payload,
		}
		if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, sequenceNumber); err != nil || !inserted {
			t.Fatalf("receive sequence %d = %v, err=%v", sequenceNumber, inserted, err)
		}
		appended, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, &models.AgentDeliveryEffect{
			EffectKey:  "canonical:" + string(rune('0'+sequence)),
			StreamID:   event.StreamID,
			Sequence:   sequenceNumber,
			EffectType: "agent_delivery.event",
		})
		if err != nil {
			t.Fatalf("project sequence %d: %v", sequenceNumber, err)
		}
		if appended != (sequence == 1) {
			t.Fatalf("sequence %d appended=%v", sequenceNumber, appended)
		}
	}

	message, err := repo.GetMessage(ctx, "canonical-message-1")
	if err != nil {
		t.Fatalf("get canonical message: %v", err)
	}
	if message.Content != "hello world" {
		t.Fatalf("canonical content = %q, want %q", message.Content, "hello world")
	}
	var count int
	if err := repo.db.Get(&count, repo.db.Rebind(`
		SELECT COUNT(*) FROM task_session_messages WHERE id = ?`), "canonical-message-1"); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("canonical message rows = %d, want 1", count)
	}
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "stream-canonical")
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ProjectedSequence != 2 {
		t.Fatalf("projected cursor = %d, want 2", cursor.ProjectedSequence)
	}
}

func TestCanonicalAgentDeliveryProjectionLeavesOutOfOrderEventsForReplay(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-canonical-gap", "session-canonical-gap", "turn-canonical-gap")

	makeEvent := func(sequence int64, text string) *models.AgentDeliveryEvent {
		payload, err := json.Marshal(streams.AgentEvent{
			Type:               streams.EventTypeMessageChunk,
			Text:               text,
			TurnID:             "turn-canonical-gap",
			CanonicalMessageID: "canonical-message-gap",
		})
		if err != nil {
			t.Fatal(err)
		}
		return &models.AgentDeliveryEvent{
			SessionID:         "session-canonical-gap",
			IncarnationID:     "incarnation-canonical-gap",
			HarnessGeneration: 1,
			StreamID:          "stream-canonical-gap",
			Sequence:          sequence,
			EventType:         streams.EventTypeMessageChunk,
			Payload:           payload,
		}
	}
	project := func(event *models.AgentDeliveryEvent) error {
		_, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, &models.AgentDeliveryEffect{
			EffectKey:  "canonical-gap:" + fmt.Sprint(event.Sequence),
			StreamID:   event.StreamID,
			Sequence:   event.Sequence,
			EffectType: "agent_delivery.event",
		})
		return err
	}

	second := makeEvent(2, " world")
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, second, 2); err != nil || !inserted {
		t.Fatalf("receive out-of-order sequence = %v, err=%v", inserted, err)
	}
	if err := project(second); err == nil {
		t.Fatal("out-of-order canonical projection unexpectedly succeeded")
	}
	if _, err := repo.GetMessage(ctx, "canonical-message-gap"); err == nil {
		t.Fatal("out-of-order canonical event created a message before its predecessor")
	}
	cursor, err := repo.GetAgentDeliveryCursor(ctx, second.StreamID)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ProjectedSequence != 0 {
		t.Fatalf("projected cursor after gap = %d, want 0", cursor.ProjectedSequence)
	}

	first := makeEvent(1, "hello")
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, first, 2); err != nil || !inserted {
		t.Fatalf("receive predecessor = %v, err=%v", inserted, err)
	}
	if err := project(first); err != nil {
		t.Fatalf("project predecessor: %v", err)
	}
	if err := project(second); err != nil {
		t.Fatalf("replay out-of-order event: %v", err)
	}
	message, err := repo.GetMessage(ctx, "canonical-message-gap")
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "hello world" {
		t.Fatalf("replayed canonical content = %q, want %q", message.Content, "hello world")
	}
}

func TestEffectRejectsConflictingDuplicateKey(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	effect := &models.AgentDeliveryEffect{
		EffectKey:  "effect-conflict",
		StreamID:   "stream-1",
		Sequence:   1,
		EffectType: "message",
	}
	if inserted, err := repo.PutAgentDeliveryEffect(ctx, effect); err != nil || !inserted {
		t.Fatalf("first effect insert = %v, err=%v", inserted, err)
	}
	conflict := *effect
	conflict.Sequence = 2
	if inserted, err := repo.PutAgentDeliveryEffect(ctx, &conflict); !errors.Is(err, repoerrors.ErrAgentDeliveryEffectConflict) || inserted {
		t.Fatalf("conflicting effect insert = %v, err=%v", inserted, err)
	}
}

func TestWorkflowTransitionEffectIsAtomicAndIdempotent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-delivery-effect")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-delivery-effect", WorkspaceID: "workspace-delivery-effect", Name: "Workflow",
	}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-delivery-effect", "step-effect-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-delivery-effect", "step-effect-target", 1)
	task := &models.Task{
		ID: "task-delivery-effect", WorkspaceID: "workspace-delivery-effect", WorkflowID: "workflow-delivery-effect",
		WorkflowStepID: "step-effect-source", Title: "Effect candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	effect := &models.AgentDeliveryEffect{
		EffectKey:  "workflow.on_turn_complete:turn-delivery-effect",
		EffectType: "workflow.on_turn_complete",
		State:      "completed",
	}

	admitted, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionAndEffect(
		ctx, task, "step-effect-target", 0, effect,
	)
	if err != nil {
		t.Fatalf("first transition: %v", err)
	}
	if !admitted || !applied {
		t.Fatalf("first transition = admitted %v, applied %v; want both true", admitted, applied)
	}

	duplicate, duplicateApplied, err := repo.UpdateTaskWithWorkflowStepAdmissionAndEffect(
		ctx, task, "step-effect-target", 0, effect,
	)
	if err != nil {
		t.Fatalf("duplicate transition: %v", err)
	}
	if duplicate || duplicateApplied {
		t.Fatalf("duplicate transition = admitted %v, applied %v; want both false", duplicate, duplicateApplied)
	}

	updated, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updated.WorkflowStepID != "step-effect-target" {
		t.Fatalf("workflow step = %q, want step-effect-target", updated.WorkflowStepID)
	}
	stored, err := repo.GetAgentDeliveryEffect(ctx, effect.EffectKey)
	if err != nil {
		t.Fatalf("GetAgentDeliveryEffect: %v", err)
	}
	if stored.State != "completed" {
		t.Fatalf("effect state = %q, want completed", stored.State)
	}
}

func TestAgentDeliveryProjectionRetainsOrderAcrossGaps(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	for sequence := int64(2); sequence >= 1; sequence-- {
		if _, err := repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{
			SessionID:         "session-2",
			IncarnationID:     "incarnation-1",
			HarnessGeneration: 1,
			StreamID:          "stream-2",
			Sequence:          sequence,
			EventType:         "message",
			Payload:           []byte{byte(sequence)},
		}, 2); err != nil {
			t.Fatal(err)
		}
	}
	events, err := repo.ListUnprojectedAgentDeliveryEvents(ctx, "stream-2", 10)
	if err != nil || len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("events before projection = %#v, err=%v", events, err)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, events[1], &models.AgentDeliveryEffect{
		EffectKey: "effect-2", StreamID: "stream-2", Sequence: 2, EffectType: "message",
	}); err != nil {
		t.Fatal(err)
	}
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "stream-2")
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ProjectedSequence != 0 {
		t.Fatalf("projected cursor crossed a gap: %+v", cursor)
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, events[0], &models.AgentDeliveryEffect{
		EffectKey: "effect-1", StreamID: "stream-2", Sequence: 1, EffectType: "message",
	}); err != nil {
		t.Fatal(err)
	}
	cursor, err = repo.GetAgentDeliveryCursor(ctx, "stream-2")
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ProjectedSequence != 2 {
		t.Fatalf("projected cursor after gap repair: %+v", cursor)
	}
}

func TestAgentDeliverySubmissionHashConflictBeforeDispatch(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	submission := &models.AgentDeliverySubmission{
		ID:                "submission-1",
		SessionID:         "session-1",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 1,
		OwnerGeneration:   3,
		PayloadHash:       "hash-1",
		Payload:           []byte("prompt"),
	}
	created, err := repo.PrepareAgentDeliverySubmission(ctx, submission)
	if err != nil || !created {
		t.Fatalf("prepare = %v, err=%v", created, err)
	}
	created, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: submission.ID, PayloadHash: submission.PayloadHash})
	if err != nil || created {
		t.Fatalf("idempotent prepare = %v, err=%v", created, err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: submission.ID, PayloadHash: "hash-2"}); !errors.Is(err, repoerrors.ErrAgentDeliverySubmissionConflict) {
		t.Fatalf("hash conflict = %v", err)
	}
	changed, err := repo.TransitionAgentDeliverySubmission(ctx, submission.ID, models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted, "", time.Now().UTC())
	if err != nil || !changed {
		t.Fatalf("accepted transition = %v, err=%v", changed, err)
	}
	changed, err = repo.TransitionAgentDeliverySubmission(ctx, submission.ID, models.DeliverySubmissionPrepared, models.DeliverySubmissionDispatching, "", time.Now().UTC())
	if err != nil || changed {
		t.Fatalf("stale transition = %v, err=%v", changed, err)
	}
}
