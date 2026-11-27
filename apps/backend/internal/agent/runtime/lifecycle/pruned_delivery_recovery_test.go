//go:build linux

package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
func TestPrunedDeliveryWithoutExecutionUsesGuardedJournalProof(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	payload := []byte(`{"text":"original prompt"}`)
	j, err := journal.Open(journal.Config{Path: journal.CheckStorage(root, "session").Path})
	require.NoError(t, err)
	_, err = j.PutSubmission(ctx, journal.Submission{ID: "prompt", SessionID: "session", IncarnationID: "session",
		HarnessGeneration: 1, StreamID: "stream", Hash: journal.SubmissionHash(payload), Payload: payload, State: journal.SubmissionInterruptedUnknown})
	require.NoError(t, err)
	for range 6 {
		_, err = j.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1,
			StreamID: "stream", SubmissionID: "prompt", Type: "context_window", Payload: []byte(`{}`)})
		require.NoError(t, err)
	}
	require.NoError(t, j.Acknowledge(ctx, "stream", 6))
	require.NoError(t, j.Close())
	var calls atomic.Int32
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/delivery/retained" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		calls.Add(1)
		var request journal.RetainedRecoveryRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Zero(t, request.Acknowledge)
		require.EqualValues(t, 1, request.After)
		result, err := journal.ReadRetainedRecovery(r.Context(), request)
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(result))
	})
	manager := newTestManager(t)
	manager.dataDir = root
	manager.SetRuntimeOwner(runtimeOwnerForTestClient(t, client))
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Guarded gap recovery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "session"}))
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "session",
		HarnessGeneration: 1, StreamID: "stream", Sequence: 1, EventType: "context_window", Payload: []byte(`{}`)}, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "session",
		HarnessGeneration: 1, StreamID: "stream", Sequence: 1, EventType: "context_window", Payload: []byte(`{}`)}, &models.AgentDeliveryEffect{EffectKey: "context:stream:1", StreamID: "stream", Sequence: 1, EffectType: "context_window"})
	require.NoError(t, err)
	manager.SetAgentDeliveryRepository(repo)
	identity := AgentDeliveryRecoveryIdentity{TaskID: "task", SessionID: "session", ExecutionID: "execution", SubmissionID: "prompt",
		StreamID: "stream", IncarnationID: "session", HarnessGeneration: 1, PromptGeneration: 1,
		OriginalRuntime: processidentity.Identity{PID: 2147483647, GroupID: 2147483647, SessionID: 2147483647, BirthToken: "linux:prior-boot:1"},
		HistoryGap:      &models.AgentDeliveryHistoryGap{FirstSequence: 2, LastSequence: 6, ReceivedSequence: 1, ProjectedSequence: 1}, PayloadHash: journal.SubmissionHash(payload)}
	result := manager.RecoverAgentPromptStreamWithIdentity(ctx, identity)
	require.Equal(t, DeliveryReconciliationUncertain, result.Outcome)
	require.Equal(t, "delivery_history_gap", result.Reason)
	require.True(t, result.ProcessTerminated)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "stream")
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ReceivedSequence)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	require.EqualValues(t, 1, calls.Load())
	_, exists := manager.GetExecutionBySessionID("session")
	require.False(t, exists, "proof collection must not launch or prompt a harness")
}
