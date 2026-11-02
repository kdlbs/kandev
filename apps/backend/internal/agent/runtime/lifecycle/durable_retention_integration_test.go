package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-002.1 AC-PLATFORM-DELIVERY-RETENTION-005.1
func TestDeliveryRetentionIntegrationLostAckAfterCleanup(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "delivery.db")
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	repo, database := openRetentionLifecycleRepository(t, databasePath)
	retained := openRetentionLifecycleJournal(t, journalPath)
	event := seedRetentionIntegrationOutput(t, repo, database, retained)
	before, err := repo.ListMessages(ctx, event.SessionID)
	if err != nil || len(before) == 0 {
		t.Fatalf("canonical positive control: %d %v", len(before), err)
	}
	result, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{})
	if err != nil || result.EventPayloadsPruned != 1 || result.EventRowsRetired != 1 {
		t.Fatalf("actual cleanup: %+v %v", result, err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := retained.Close(); err != nil {
		t.Fatal(err)
	}
	repo, _ = openRetentionLifecycleRepository(t, databasePath)
	retained = openRetentionLifecycleJournal(t, journalPath)
	peer := &retentionLifecycleJournalPeer{journal: retained, sessionID: event.SessionID, incarnationID: event.IncarnationID, harnessGeneration: 2, loseAfter: true}
	execution := &AgentExecution{ID: event.ExecutionID, TaskID: event.TaskID, SessionID: event.SessionID, DeliveryStreamID: event.StreamID, DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: event.IncarnationID, DeliveryHarnessGeneration: 2}
	if _, err := reconcileExpiredAgentDeliveryReplay(ctx, execution, event, repo, peer, peer, nil); !errors.Is(err, errRetentionAckResponseLost) {
		t.Fatalf("lost committed ACK response: %v", err)
	}
	if _, err := reconcileExpiredAgentDeliveryReplay(ctx, execution, event, repo, peer, peer, nil); err != nil {
		t.Fatal(err)
	}
	if peer.ackCalls != 1 {
		t.Fatalf("ACK calls=%d, want one committed ACK", peer.ackCalls)
	}
	after, err := repo.ListMessages(ctx, event.SessionID)
	if err != nil || len(after) != len(before) || after[0].Content != before[0].Content {
		t.Fatalf("saved canonical history changed after cleanup/reopen: %+v %v", after, err)
	}
	cursor, err := repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	if err != nil || cursor.ProjectedSequence != 1 || cursor.RetiredSequence != 1 {
		t.Fatalf("reopened checkpoint: %+v %v", cursor, err)
	}
	remote, err := retained.GetStream(ctx, event.StreamID)
	if err != nil || remote.Acknowledged != 1 {
		t.Fatalf("retained journal ACK: %+v %v", remote, err)
	}
}

func seedRetentionIntegrationOutput(t *testing.T, repo *tasksqlite.Repository, database *sqlx.DB, retained *journal.Journal) *models.AgentDeliveryEvent {
	t.Helper()
	ctx := context.Background()
	const sessionID, taskID, workspaceID = "retention-integration-session", "retention-integration-task", "retention-integration-workspace"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Retention integration"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Retention integration"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, QueueIncarnationID: "retention-integration-incarnation"}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: "retention-integration-execution", Status: models.ExecutorRunningStatusRunning}); err != nil {
		t.Fatal(err)
	}
	if stored, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: sessionID, IncarnationID: "retention-integration-incarnation", Generation: 2, NativeSessionID: "retention-integration-native", CreationReason: "initial", CreatedAt: old, CommittedAt: old}, 0); err != nil || !stored {
		t.Fatalf("commit harness owner: %v %v", stored, err)
	}
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "saved output survives actual cleanup and lost ACK"})
	if err != nil {
		t.Fatal(err)
	}
	event := &models.AgentDeliveryEvent{SessionID: sessionID, IncarnationID: "retention-integration-incarnation", HarnessGeneration: 2, StreamID: "retention-integration-stream", Sequence: 1, EventType: streams.EventTypeMessageChunk, Payload: payload, ReceivedAt: old}
	event.TaskID, event.ExecutionID = taskID, "retention-integration-execution"
	event.OwnerGeneration, event.ExpectedStreamID = 2, event.StreamID
	if _, err := retained.Append(ctx, journal.Event{SessionID: sessionID, IncarnationID: event.IncarnationID, HarnessGeneration: 2, StreamID: event.StreamID, Type: event.EventType, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
		t.Fatalf("receive: %v %v", inserted, err)
	}
	effect := &models.AgentDeliveryEffect{EffectKey: "agent_delivery.event:" + event.StreamID + ":1", StreamID: event.StreamID, Sequence: 1, EffectType: "agent_delivery.event", State: models.DeliveryEffectCompleted, CreatedAt: old, CompletedAt: &old}
	if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, effect); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(database.Rebind(`UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ?`), old, event.StreamID); err != nil {
		t.Fatal(err)
	}
	return event
}
