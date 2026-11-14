package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
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

func legacyContinuationSetupFixture(t *testing.T) (*tasksqlite.Repository, *AgentExecution, agentctl.DeliveryStatus) {
	t.Helper()
	ctx := t.Context()
	repo, _ := runOwnerTestRepository(t)
	execution := &AgentExecution{
		ID: "legacy-execution", TaskID: "legacy-task", SessionID: "legacy-session",
		DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "legacy-incarnation",
		DeliveryHarnessGeneration: 1, DeliveryStreamID: "legacy-incarnation:g1",
		ForceContextContinuation: true, ACPSessionID: "legacy-new-native",
	}
	execution.deliveryBootstrap.Store(true)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: execution.TaskID, Title: "Legacy continuation"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: execution.SessionID, TaskID: execution.TaskID, QueueIncarnationID: execution.DeliveryIncarnationID,
		State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: execution.SessionID, TaskID: execution.TaskID, AgentExecutionID: execution.ID,
		ResumeToken: execution.ACPSessionID, Status: models.ExecutorRunningStatusRunning,
	}))
	attempt := &models.RestoreAttempt{ID: "legacy-attempt", SessionID: execution.SessionID,
		IncarnationID: execution.DeliveryIncarnationID, ExpectedGeneration: 0,
		Action: "continue_from_history", Outcome: "blocked", Authorized: true}
	require.NoError(t, repo.CreateRestoreAttempt(ctx, attempt))
	require.NoError(t, repo.CreateContinuationSnapshot(ctx, &models.ContinuationSnapshot{
		ID: "legacy-snapshot", AttemptID: attempt.ID, SessionID: execution.SessionID,
		TargetGeneration: 1, SubmissionID: "legacy-submission", Content: "retained setup context",
		Status: models.ContinuitySnapshotPrepared,
	}))
	descriptor := agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		HarnessGeneration: 1, StreamID: execution.DeliveryStreamID,
		Stream: &journal.Stream{SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
			HarnessGeneration: 1, StreamID: execution.DeliveryStreamID, HighWater: 1, FirstRetained: 1},
	}
	return repo, execution, descriptor
}

func TestDeliveryPreflightReplaysAuthorizedLegacyContinuationSetup(t *testing.T) {
	for _, resumed := range []bool{false, true} {
		t.Run(map[bool]string{false: "native bound", true: "resumed"}[resumed], func(t *testing.T) {
			repo, execution, descriptor := legacyContinuationSetupFixture(t)
			execution.isResumedSession = resumed
			client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/agent/delivery", r.URL.Path)
				require.NoError(t, json.NewEncoder(w).Encode(descriptor))
			})
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			after, err := sm.initialDeliveryReplayCursor(t.Context(), execution, repo, client, execution.DeliveryStreamID)
			require.NoError(t, err, "authorized generation-zero continuation must replay retained setup")
			require.Zero(t, after)
			_, err = repo.GetAgentDeliveryCursor(t.Context(), execution.DeliveryStreamID)
			require.Error(t, err, "setup admission cannot fabricate SQL receipt authority")
			_, err = repo.GetCurrentHarnessSessionGeneration(t.Context(), execution.SessionID, execution.DeliveryIncarnationID)
			require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
		})
	}
}

type failedLegacyContinuationReader struct{ *tasksqlite.Repository }

func (*failedLegacyContinuationReader) HasPreparedLegacyContinuationBootstrap(context.Context, string, string, string) (bool, error) {
	return false, errors.New("legacy continuation query failed")
}

func TestLegacyContinuationSetupAdmissionRefusals(t *testing.T) {
	for _, query := range []string{
		"UPDATE session_restore_attempts SET authorized = 0",
		"UPDATE session_restore_attempts SET action = 'fresh_start'",
		"UPDATE session_restore_attempts SET expected_generation = 1",
		"UPDATE session_restore_attempts SET completed_at = CURRENT_TIMESTAMP",
		"UPDATE session_restore_attempts SET outcome = 'restored'",
		"UPDATE session_continuation_snapshots SET target_generation = 2",
		"UPDATE session_continuation_snapshots SET status = 'consumed'",
		"UPDATE session_continuation_snapshots SET content = '', byte_count = 0",
		"UPDATE executors_running SET agent_execution_id = 'foreign-execution'",
		"UPDATE task_sessions SET queue_incarnation_id = 'foreign-incarnation'",
		"UPDATE task_sessions SET state = 'CANCELLED'",
		"INSERT INTO harness_session_generations (session_id, incarnation_id, generation, native_session_id, created_at, committed_at) VALUES ('legacy-session', 'legacy-incarnation', 1, 'other-native', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"INSERT INTO agent_delivery_cursors (stream_id, session_id, incarnation_id, harness_generation, updated_at) VALUES ('legacy-stream', 'legacy-session', 'legacy-incarnation', 1, CURRENT_TIMESTAMP)",
		"INSERT INTO agent_delivery_submissions (id, session_id, incarnation_id, harness_generation, owner_generation, payload_hash, payload, state, created_at, updated_at) VALUES ('unknown-work', 'legacy-session', 'legacy-incarnation', 1, 1, 'existing-evidence', '{}', 'unknown', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
	} {
		t.Run(query, func(t *testing.T) {
			repo, execution, descriptor := legacyContinuationSetupFixture(t)
			_, err := repo.DB().ExecContext(t.Context(), query)
			require.NoError(t, err)
			valid, err := repo.HasPreparedLegacyContinuationBootstrap(t.Context(), execution.SessionID, execution.DeliveryIncarnationID, execution.ID)
			require.NoError(t, err)
			require.False(t, valid, "contradictory durable state cannot authorize legacy setup")
			require.False(t, canReplaySuccessorSetupEvents(t.Context(), execution, repo, &descriptor))
		})
	}
	for _, kind := range []string{"ordinary start", "not bootstrap", "unresolved journal", "acknowledged", "truncated submissions", "query error"} {
		t.Run(kind, func(t *testing.T) {
			repo, execution, descriptor := legacyContinuationSetupFixture(t)
			var reader AgentDeliveryRepository = repo
			switch kind {
			case "ordinary start":
				execution.ForceContextContinuation = false
			case "not bootstrap":
				execution.deliveryBootstrap.Store(false)
			case "unresolved journal":
				descriptor.Unresolved = true
			case "acknowledged":
				descriptor.Stream.Acknowledged = 1
			case "truncated submissions":
				descriptor.SubmissionsTruncated = true
			case "query error":
				reader = &failedLegacyContinuationReader{Repository: repo}
			}
			require.False(t, canReplaySuccessorSetupEvents(t.Context(), execution, reader, &descriptor))
		})
	}
}

func TestLegacyContinuationSetupWaitsForFirstCommitOrCancellation(t *testing.T) {
	for _, commit := range []bool{true, false} {
		t.Run(map[bool]string{true: "commit", false: "cancel"}[commit], func(t *testing.T) {
			repo, execution, _ := legacyContinuationSetupFixture(t)
			reader := &freshStartSQLReader{Repository: repo, firstRead: make(chan struct{})}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			_, event := retentionBootstrapOwner()
			event.DeliveryStreamID = execution.DeliveryStreamID
			event.DeliveryIncarnationID = execution.DeliveryIncarnationID
			result, done := make(chan error, 1), make(chan struct{})
			go func() {
				defer close(done)
				_, err := sm.prepareAgentEvent(ctx, execution, nil, reader, event)
				result <- err
			}()
			t.Cleanup(func() { cancel(); <-done })
			select {
			case <-reader.firstRead:
			case <-ctx.Done():
				t.Fatal("legacy setup did not inspect canonical generation")
			}
			select {
			case err := <-result:
				t.Fatalf("legacy setup returned before first generation commit: %v", err)
			case <-time.After(100 * time.Millisecond): // existing 25ms bootstrap poll must remain waiting
			}
			var receipts int
			require.NoError(t, repo.DB().QueryRow("SELECT count(*) FROM agent_delivery_inbox").Scan(&receipts))
			require.Zero(t, receipts)
			if commit {
				ok, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
					SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
					Generation: 1, NativeSessionID: execution.ACPSessionID, CreationReason: "context_continued",
				}, 0)
				require.NoError(t, err)
				require.True(t, ok)
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if commit {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, context.Canceled)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("legacy generation waiter did not settle")
			}
			<-done
			require.NoError(t, repo.DB().QueryRow("SELECT count(*) FROM agent_delivery_inbox").Scan(&receipts))
			require.Equal(t, map[bool]int{true: 1, false: 0}[commit], receipts)
		})
	}
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
