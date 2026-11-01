package lifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type projectionSQLRepository struct {
	*tasksqlite.Repository
	reads      int
	beforeRead func(int)
	afterRead  func(int)
}

func (r *projectionSQLRepository) GetAgentDeliveryCursor(ctx context.Context, stream string) (*models.AgentDeliveryCursor, error) {
	r.reads++
	r.beforeRead(r.reads)
	cursor, err := r.Repository.GetAgentDeliveryCursor(ctx, stream)
	r.afterRead(r.reads)
	return cursor, err
}

func seedProjectionResumeOwner(t *testing.T, repo *tasksqlite.Repository) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Projection recovery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation", State: models.TaskSessionStateWaitingForInput,
		Metadata: map[string]any{"acp": map[string]any{"session_id": "native-conversation"}}}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "session", SessionID: "session", TaskID: "task", AgentExecutionID: "execution", Status: models.ExecutorRunningStatusRunning}))
	now := time.Now().UTC()
	committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: "session", IncarnationID: "incarnation", Generation: 2, NativeSessionID: "native-conversation", CreationReason: "initial", CreatedAt: now, CommittedAt: now}, 0)
	require.NoError(t, err)
	require.True(t, committed)
}

func projectionSQLReceipt(t *testing.T, repo *tasksqlite.Repository, sequence int64) *models.AgentDeliveryEvent {
	t.Helper()
	event := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		TaskID: "task", ExecutionID: "execution", OwnerGeneration: 2, ExpectedStreamID: "stream", StreamID: "stream", Sequence: sequence, EventType: "usage_update", Payload: []byte(`{}`)}
	inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	return event
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
func TestNativeResumeProjectionWaitResolvesRealSQLAndJournal(t *testing.T) {
	manager, execution, _, store := newProjectionWaitFixture(t)
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
	seedProjectionResumeOwner(t, repo)
	for sequence := int64(1); sequence <= 2; sequence++ {
		_, err := repo.ProjectAgentDeliveryEvent(t.Context(), projectionSQLReceipt(t, repo, sequence), nil)
		require.NoError(t, err)
	}
	original := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "hash", Payload: []byte("original instruction"), State: journal.SubmissionInterruptedUnknown}
	_, err := store.PutSubmission(t.Context(), original)
	require.NoError(t, err)
	_, err = repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: original.ID, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: original.Hash, Payload: original.Payload, State: models.DeliverySubmissionInterruptedUnknown})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertSessionRecoveryBlock(t.Context(), &models.SessionRecoveryBlock{ID: "block", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 2, State: models.RecoveryBlockOpen, Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery", DeliverySubmissionID: original.ID, DeliveryStreamID: "stream"}))
	manager.SetAgentDeliveryRepository(repo)
	require.NoError(t, manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
	var pending *models.AgentDeliveryEvent
	wrapped := &projectionSQLRepository{Repository: repo}
	wrapped.beforeRead = func(read int) {
		if read != 1 {
			return
		}
		_, err := store.Append(t.Context(), journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`)})
		require.NoError(t, err)
		pending = projectionSQLReceipt(t, repo, 3)
	}
	wrapped.afterRead = func(read int) {
		if read != 2 {
			return
		}
		_, err := repo.ProjectAgentDeliveryEvent(t.Context(), pending, nil)
		require.NoError(t, err)
		require.NoError(t, store.Acknowledge(t.Context(), "stream", 3))
	}
	manager.SetAgentDeliveryRepository(wrapped)
	proof, err := manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
	require.NoError(t, err)
	require.GreaterOrEqual(t, wrapped.reads, 4)
	resolved, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), "block", proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, resolved)
	resolved, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), "block", proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, resolved, "repeated proof must not release work twice")
	row, err := repo.GetAgentDeliverySubmission(t.Context(), original.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, row.State)
	require.Equal(t, original.Payload, row.Payload)
	retained, err := store.GetSubmission(t.Context(), original.ID)
	require.NoError(t, err)
	require.True(t, retained.Retired)
	require.Equal(t, original.State, retained.State)
	require.Empty(t, execution.deliverySubmissionIDSnapshot())
	require.Equal(t, "native-conversation", proof.NativeSessionID)
}
