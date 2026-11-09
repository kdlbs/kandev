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
)

const (
	combinedDeliveryTaskID    = "task-combined-batch"
	combinedDeliverySessionID = "session-combined-batch"
	combinedDeliveryStreamID  = "stream-combined-batch"
	combinedDeliveryMessageID = "message-combined-batch"
)

func seedCombinedDeliveryBatchSession(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: combinedDeliveryTaskID, Title: "Combined batch"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: combinedDeliverySessionID, TaskID: combinedDeliveryTaskID,
		QueueIncarnationID: "incarnation-combined-batch", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-combined-batch", TaskSessionID: combinedDeliverySessionID,
		TaskID: combinedDeliveryTaskID, StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
}

func combinedDeliveryBatchEvent(t *testing.T, sequence int64, text string) *models.AgentDeliveryEvent {
	t.Helper()
	payload, err := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: text,
		CanonicalMessageBaseID: combinedDeliveryMessageID, CanonicalMessageID: combinedDeliveryMessageID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &models.AgentDeliveryEvent{
		SessionID: combinedDeliverySessionID, IncarnationID: "incarnation-combined-batch",
		HarnessGeneration: 1, StreamID: combinedDeliveryStreamID, Sequence: sequence,
		EventType: streams.EventTypeMessageChunk, Payload: payload, ReceivedAt: time.Now().UTC(),
	}
}

func combinedDeliveryBatchEffects(events []*models.AgentDeliveryEvent) []*models.AgentDeliveryEffect {
	effects := make([]*models.AgentDeliveryEffect, len(events))
	for index, event := range events {
		now := event.ReceivedAt
		effects[index] = &models.AgentDeliveryEffect{
			EffectKey: fmt.Sprintf("agent_delivery.event:%s:%d", event.StreamID, event.Sequence),
			StreamID:  event.StreamID, Sequence: event.Sequence, EffectType: "agent_delivery.event",
			State: models.DeliveryEffectCompleted, CreatedAt: now, CompletedAt: &now,
		}
	}
	return effects
}

func countCombinedDeliveryRows(t *testing.T, repo *Repository, query string, args ...any) int {
	t.Helper()
	var count int
	if err := repo.db.QueryRowx(repo.db.Rebind(query), args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestReceiveAndProjectCanonicalBatchAtomic(t *testing.T) {
	repos := map[string]func(*testing.T) *Repository{
		"sqlite":   newRepoForSessionTests,
		"postgres": openPostgresRepo,
	}
	for name, open := range repos {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			control := open(t)
			seedCombinedDeliveryBatchSession(t, control)
			controlEvents := []*models.AgentDeliveryEvent{
				combinedDeliveryBatchEvent(t, 1, "a"),
				combinedDeliveryBatchEvent(t, 2, "b"),
			}
			controlResults, err := control.ReceiveAndProjectCanonicalAgentDeliveryEvents(
				ctx, controlEvents, combinedDeliveryBatchEffects(controlEvents), 0,
			)
			if err != nil {
				t.Fatalf("uninterrupted batch: %v", err)
			}
			controlMessage, err := control.GetMessage(ctx, combinedDeliveryMessageID)
			if err != nil {
				t.Fatalf("read uninterrupted message: %v", err)
			}

			repo := open(t)
			seedCombinedDeliveryBatchSession(t, repo)
			events := []*models.AgentDeliveryEvent{
				combinedDeliveryBatchEvent(t, 1, "a"),
				combinedDeliveryBatchEvent(t, 2, "b"),
			}
			effects := combinedDeliveryBatchEffects(events)

			repo.agentDeliveryCombinedProjectionFailpoint = func(string) error {
				return errors.New("injected projection failure")
			}
			if _, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, 0); err == nil {
				t.Fatal("combined batch succeeded despite the injected failure")
			}
			repo.agentDeliveryCombinedProjectionFailpoint = nil
			if got := countCombinedDeliveryRows(t, repo,
				`SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ?`, combinedDeliveryStreamID); got != 0 {
				t.Fatalf("inbox rows after failed batch = %d, want 0", got)
			}
			if got := countCombinedDeliveryRows(t, repo,
				`SELECT COUNT(*) FROM agent_delivery_cursors WHERE stream_id = ?`, combinedDeliveryStreamID); got != 0 {
				t.Fatalf("cursor rows after failed batch = %d, want 0", got)
			}
			if got := countCombinedDeliveryRows(t, repo,
				`SELECT COUNT(*) FROM task_session_messages WHERE id = ?`, combinedDeliveryMessageID); got != 0 {
				t.Fatalf("messages after failed batch = %d, want 0", got)
			}

			results, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, 0)
			if err != nil {
				t.Fatalf("retry combined batch: %v", err)
			}
			if len(results) != 2 || results[0].CanonicalMessageID != combinedDeliveryMessageID ||
				results[1].CanonicalMessageID != combinedDeliveryMessageID {
				t.Fatalf("combined results = %+v", results)
			}
			if results[0].AlreadyProjected || results[1].AlreadyProjected {
				t.Fatalf("first commit reported already projected: %+v", results)
			}
			if got := countCombinedDeliveryRows(t, repo,
				`SELECT COUNT(*) FROM task_session_messages WHERE id = ?`, combinedDeliveryMessageID); got != 1 {
				t.Fatalf("messages after retry = %d, want 1", got)
			}
			message, err := repo.GetMessage(ctx, combinedDeliveryMessageID)
			if err != nil {
				t.Fatalf("read canonical message: %v", err)
			}
			if message.Content != controlMessage.Content {
				t.Fatalf("canonical message content = %q, want the uninterrupted %q", message.Content, controlMessage.Content)
			}
			for index := range results {
				if results[index].CanonicalMessageID != controlResults[index].CanonicalMessageID {
					t.Fatalf("retry result %d ID = %q, want uninterrupted %q",
						index, results[index].CanonicalMessageID, controlResults[index].CanonicalMessageID)
				}
			}

			duplicates, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, 0)
			if err != nil {
				t.Fatalf("duplicate combined batch: %v", err)
			}
			for index, result := range duplicates {
				if !result.AlreadyProjected || result.CanonicalMessageID != combinedDeliveryMessageID {
					t.Fatalf("duplicate result %d = %+v", index, result)
				}
			}
			cursor, err := repo.GetAgentDeliveryCursor(ctx, combinedDeliveryStreamID)
			if err != nil {
				t.Fatalf("read cursor: %v", err)
			}
			if cursor.ReceivedSequence != 2 || cursor.ProjectedSequence != 2 {
				t.Fatalf("cursor = %+v, want received and projected at 2", cursor)
			}
		})
	}
}

func TestReceiveAndProjectReceivedButUnprojectedEvent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCombinedDeliveryBatchSession(t, repo)
	ctx := context.Background()
	event := combinedDeliveryBatchEvent(t, 1, "received")
	if _, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 0); err != nil {
		t.Fatalf("receive event: %v", err)
	}
	results, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(
		ctx, []*models.AgentDeliveryEvent{event}, combinedDeliveryBatchEffects([]*models.AgentDeliveryEvent{event}), 0,
	)
	if err != nil {
		t.Fatalf("project received event: %v", err)
	}
	if len(results) != 1 || results[0].AlreadyProjected || results[0].CanonicalMessageID != combinedDeliveryMessageID {
		t.Fatalf("results = %+v, want one freshly projected event", results)
	}
}
