package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// An authorized context continuation starts generation two before the
// orchestrator commits it in SQL. Setup events retained in that window must be
// replayed from zero instead of failing the stream with a missing checkpoint.
func TestDeliveryPreflightReplaysContinuationSetupBeforeGenerationCommit(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Context continuation"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 1, NativeSessionID: "predecessor-native", CreationReason: "native_started",
	}))
	descriptor := agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "incarnation:g2",
		Stream: &journal.Stream{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "incarnation:g2", HighWater: 1, FirstRetained: 1},
	}
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/agent/delivery", r.URL.Path)
		require.NoError(t, json.NewEncoder(w).Encode(descriptor))
	})
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(sm.Wait)
	execution := &AgentExecution{SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2,
		DeliveryStreamID: "incarnation:g2", ForceContextContinuation: true}

	after, err := sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.NoError(t, err, "retained continuation setup must not fail the stream before its generation commit")
	require.Zero(t, after, "replay the retained setup event; do not fabricate a receipt")
	_, err = repo.GetAgentDeliveryCursor(ctx, execution.DeliveryStreamID)
	require.Error(t, err, "preflight must leave the absent SQL receipt absent")

	execution.ForceContextContinuation, execution.ReplacesNativeConversation = false, true
	after, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.NoError(t, err, "an explicit fresh start shares the successor setup replay")
	require.Zero(t, after)
	execution.ReplacesNativeConversation = false
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.Error(t, err, "an unauthorized successor generation cannot bypass a missing checkpoint")
	execution.ForceContextContinuation = true
	descriptor.SubmissionCount = 1
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.Error(t, err, "retained prompt work must be reconciled")
	descriptor.SubmissionCount = 0
	descriptor.Stream.Acknowledged = 1
	descriptor.Stream.FirstRetained = 2
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.ErrorIs(t, err, ErrAgentDeliveryHistoryGap, "continuation cannot bypass pruned history")
	descriptor.Stream.Acknowledged = 0
	descriptor.Stream.FirstRetained = 1
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 2, PredecessorGeneration: 1,
		NativeSessionID: "continued-native", CreationReason: "context_continued",
	}))
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.Error(t, err, "a committed continuation generation cannot bypass a missing checkpoint")
}

// The replayed setup event must wait for the continuation commit before it is
// received, exactly like a fresh-start successor.
func TestContinuationSetupEventWaitsForGenerationCommit(t *testing.T) {
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
	execution, event := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	execution.DeliveryHarnessGeneration, execution.ForceContextContinuation = 2, true
	event.DeliveryHarnessGeneration = 2
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: execution.TaskID, Title: "Continuation SQL wait"}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: execution.SessionID, TaskID: execution.TaskID, QueueIncarnationID: execution.DeliveryIncarnationID}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{SessionID: execution.SessionID, TaskID: execution.TaskID,
		AgentExecutionID: execution.ID, ResumeToken: "continued-native", Status: models.ExecutorRunningStatusRunning}))
	predecessor := retentionBootstrapGeneration()
	committed, err := repo.CommitHarnessSessionGeneration(t.Context(), predecessor, 0)
	require.NoError(t, err)
	require.True(t, committed)
	reader := &freshStartSQLReader{Repository: repo, firstRead: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	done, result := make(chan struct{}), make(chan error, 1)
	go func() {
		defer close(done)
		_, err := manager.prepareAgentEvent(ctx, execution, nil, reader, event)
		result <- err
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-reader.firstRead:
	case <-ctx.Done():
		t.Fatal("event did not inspect the real SQL predecessor")
	}
	select {
	case err := <-result:
		t.Fatalf("continuation setup event returned before its generation commit: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	var receipts int
	require.NoError(t, repo.DB().QueryRow("SELECT count(*) FROM agent_delivery_inbox").Scan(&receipts))
	require.Zero(t, receipts, "no receipt may precede the continuation commit")

	successor := *predecessor
	successor.Generation, successor.PredecessorGeneration = 2, 1
	successor.NativeSessionID, successor.CreationReason = "continued-native", "context_continued"
	committed, err = repo.CommitHarnessSessionGeneration(t.Context(), &successor, 1)
	require.NoError(t, err)
	require.True(t, committed)
	select {
	case err = <-result:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("continuation waiter did not settle")
	}
	<-done
	require.NoError(t, repo.DB().QueryRow("SELECT count(*) FROM agent_delivery_inbox").Scan(&receipts))
	require.Equal(t, 1, receipts)
}
