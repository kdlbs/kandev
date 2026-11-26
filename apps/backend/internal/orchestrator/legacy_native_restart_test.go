package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	runtimeagentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	runtimekind "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type legacyNativeTestManager struct {
	*nativeProofTestManager
	runtimeManager *lifecycle.Manager
	inspect        func(context.Context, agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error)
}

func (m *legacyNativeTestManager) InspectDeliveryRecordEvidence(ctx context.Context, request agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error) {
	return m.inspect(ctx, request)
}
func (m *legacyNativeTestManager) ReadDeliveryRecordSubmission(context.Context, agentruntime.DeliveryRecordSubmissionRequest) (*agentruntime.DeliveryRecordSubmission, error) {
	return nil, errors.New("old prompt must never be read for dispatch")
}
func (m *legacyNativeTestManager) RecoverAgentPromptStreamWithIdentity(ctx context.Context, identity lifecycle.AgentDeliveryRecoveryIdentity) lifecycle.DeliveryReconciliationResult {
	if m.runtimeManager == nil {
		return lifecycle.DeliveryReconciliationResult{Outcome: lifecycle.DeliveryReconciliationBlocked}
	}
	return m.runtimeManager.RecoverAgentPromptStreamWithIdentity(ctx, identity)
}

func TestAutomaticRestartReplaysRetainedCancellation(t *testing.T) {
	t.Run("projects cancellation before native resume", func(t *testing.T) {
		fixture := newRetainedCancellationRestartFixture(t, false)
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)
		restored, err := fixture.service.resumeLegacyNativeRestart(context.Background(), session)
		require.NoError(t, err)
		require.True(t, restored)
		require.NotNil(t, *fixture.launched)
		require.Equal(t, "native-conversation", (*fixture.launched).RequiredNativeConversationID)
		require.Equal(t, "native-conversation", (*fixture.launched).ACPSessionID)
		require.Empty(t, fixture.manager.capturedPromptCalls)
		assertRetainedCancellationSettled(t, fixture)
		pending, err := fixture.queue.ListPendingQueueDispatches(context.Background())
		require.NoError(t, err)
		require.Empty(t, pending, "terminal settlement must retire the matching queue claim")

		againSession, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)
		firstLaunch := *fixture.launched
		again, err := fixture.service.resumeLegacyNativeRestart(context.Background(), againSession)
		require.NoError(t, err)
		require.False(t, again)
		require.Same(t, firstLaunch, *fixture.launched)
		require.Empty(t, fixture.manager.capturedPromptCalls)
		assertRetainedCancellationSettled(t, fixture)
	})

	t.Run("does not launch while a newer independent block remains open", func(t *testing.T) {
		fixture := newRetainedCancellationRestartFixture(t, false)
		bindRetainedCancellationBlockStream(t, fixture, fixture.streamID)
		independent := &models.SessionRecoveryBlock{
			ID: "independent-recovery-block", SessionID: fixture.row.SessionID,
			IncarnationID: fixture.row.IncarnationID, ExpectedGeneration: fixture.row.HarnessGeneration,
			Reason: "independent_recovery", State: models.RecoveryBlockOpen,
			ConsumerReference: "interactive", CreatedAt: fixture.block.CreatedAt.Add(time.Minute),
		}
		require.NoError(t, fixture.repo.UpsertSessionRecoveryBlock(context.Background(), independent))
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)

		restored, err := fixture.service.resumeLegacyNativeRestart(context.Background(), session)
		require.Error(t, err)
		require.False(t, restored)
		require.Nil(t, *fixture.launched)
		assertRetainedCancellationSettled(t, fixture)
		stillOpen, err := fixture.repo.GetSessionRecoveryBlock(context.Background(), independent.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, stillOpen.State)
	})

	t.Run("reconciles the terminal queue claim before native resume", func(t *testing.T) {
		fixture := newRetainedCancellationRestartFixture(t, false)
		bindRetainedCancellationBlockStream(t, fixture, fixture.streamID)
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)

		restored, err := fixture.service.resumeLegacyNativeRestart(context.Background(), session)
		require.NoError(t, err)
		require.True(t, restored)
		pending, err := fixture.queue.ListPendingQueueDispatches(context.Background())
		require.NoError(t, err)
		require.Empty(t, pending, "the exact terminal claim must be retired before resume")
		require.Empty(t, fixture.manager.capturedPromptCalls)
	})

	t.Run("leaves the candidate pending when capacity is full", func(t *testing.T) {
		fixture := newRetainedCancellationRestartFixture(t, false)
		bindRetainedCancellationBlockStream(t, fixture, fixture.streamID)
		fixture.service.sessionCeiling = newSessionCeilingController(1, nil, nil)
		admission := fixture.service.sessionCeiling.admit(context.Background(), admissionRequest{
			taskID: "capacity-task", sessionID: "capacity-session", origin: launchOriginAutomatic, seam: "test",
		})
		require.True(t, admission.admitted)
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)

		restored, err := fixture.service.resumeLegacyNativeRestart(context.Background(), session)
		require.NoError(t, err)
		require.False(t, restored)
		require.Nil(t, *fixture.launched)
		task, err := fixture.repo.GetTask(context.Background(), session.TaskID)
		require.NoError(t, err)
		deferred, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
		require.NotEqual(t, true, deferred[models.CeilingDeferredKey], "capacity refusal must not create a generic deferral")
		stored, err := fixture.repo.GetAgentDeliverySubmission(context.Background(), fixture.row.ID)
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
		block, err := fixture.repo.GetSessionRecoveryBlock(context.Background(), fixture.block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, block.State)
		cursor, err := fixture.repo.GetAgentDeliveryCursor(context.Background(), fixture.streamID)
		require.NoError(t, err)
		require.EqualValues(t, 1, cursor.ProjectedSequence)
		pending, err := fixture.queue.ListPendingQueueDispatches(context.Background())
		require.NoError(t, err)
		require.Len(t, pending, 1)
	})

	t.Run("replays a cancellation received but not projected before restart", func(t *testing.T) {
		fixture := newRetainedCancellationRestartFixtureWithOptions(t, retainedCancellationRestartOptions{
			receiveTerminalWithoutProjection: true,
		})
		bindRetainedCancellationBlockStream(t, fixture, fixture.streamID)
		before, err := fixture.repo.GetAgentDeliveryCursor(context.Background(), fixture.streamID)
		require.NoError(t, err)
		require.EqualValues(t, 2, before.ReceivedSequence)
		require.EqualValues(t, 1, before.ProjectedSequence)
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)

		restored, err := fixture.service.resumeLegacyNativeRestart(context.Background(), session)
		require.NoError(t, err)
		require.True(t, restored)
		require.NotNil(t, *fixture.launched)
		after, err := fixture.repo.GetAgentDeliveryCursor(context.Background(), fixture.streamID)
		require.NoError(t, err)
		require.EqualValues(t, 2, after.ReceivedSequence)
		require.EqualValues(t, 2, after.ProjectedSequence)
		assertRetainedCancellationSettled(t, fixture)
	})

	t.Run("rejects a conflicting populated block stream", func(t *testing.T) {
		fixture := newRetainedCancellationRestartFixture(t, false)
		_, err := fixture.repo.DB().ExecContext(context.Background(),
			`UPDATE session_recovery_blocks SET delivery_stream_id = ? WHERE id = ?`, "foreign-stream", fixture.block.ID)
		require.NoError(t, err)
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)

		restored, err := fixture.service.resumeLegacyNativeRestart(context.Background(), session)
		require.NoError(t, err)
		require.False(t, restored)
		require.Nil(t, *fixture.launched)
		block, err := fixture.repo.GetSessionRecoveryBlock(context.Background(), fixture.block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, block.State)
	})

	t.Run("keeps a mismatched hash blocked", func(t *testing.T) {
		fixture := newRetainedCancellationRestartFixture(t, true)
		bindRetainedCancellationBlockStream(t, fixture, fixture.streamID)
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.row.SessionID)
		require.NoError(t, err)

		restored, err := fixture.service.resumeLegacyNativeRestart(context.Background(), session)
		require.NoError(t, err)
		require.False(t, restored)
		require.Nil(t, *fixture.launched)
		block, err := fixture.repo.GetSessionRecoveryBlock(context.Background(), fixture.block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, block.State)
		cursor, err := fixture.repo.GetAgentDeliveryCursor(context.Background(), fixture.streamID)
		require.NoError(t, err)
		require.EqualValues(t, 1, cursor.ProjectedSequence)
		require.Empty(t, fixture.manager.capturedPromptCalls)
	})
}

type retainedCancellationRestartFixture struct {
	service  *Service
	manager  *legacyNativeTestManager
	repo     *sqliterepo.Repository
	queue    *messagequeue.Service
	block    *models.SessionRecoveryBlock
	row      *models.AgentDeliverySubmission
	launched **executor.LaunchAgentRequest
	streamID string
}

type retainedCancellationRestartOptions struct {
	mismatchHash                     bool
	receiveTerminalWithoutProjection bool
}

func newRetainedCancellationRestartFixture(t *testing.T, mismatchHash bool) *retainedCancellationRestartFixture {
	return newRetainedCancellationRestartFixtureWithOptions(t, retainedCancellationRestartOptions{mismatchHash: mismatchHash})
}

func newRetainedCancellationRestartFixtureWithOptions(t *testing.T, options retainedCancellationRestartOptions) *retainedCancellationRestartFixture {
	t.Helper()
	ctx := context.Background()
	service, manager, block, row, launched, _ := nativeResumeFixtureWithoutRecoveryMetadata(t, "unknown_prompt_outcome", false, false)
	repo := service.repo.(*sqliterepo.Repository)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, row.SessionID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, row.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
	streamID := block.DeliveryStreamID
	queue, queueDB := newRetainedCancellationQueue(t, filepath.Join(t.TempDir(), "queue.db"), "task1", row.SessionID, row.IncarnationID)
	t.Cleanup(func() { _ = queueDB.Close() })
	service.messageQueue = queue
	_, err := queue.QueueMessage(ctx, row.SessionID, "task1", string(row.Payload), "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	claim, ok := queue.ReserveQueued(ctx, row.SessionID)
	require.True(t, ok)
	_, dispatchAttemptID, _ := claim.DeliverySubmission()
	require.NotEmpty(t, dispatchAttemptID)
	payloadHash := journal.SubmissionHash(row.Payload)
	require.NoError(t, queue.SetPendingQueueDispatchDelivery(ctx, claim, messagequeue.DeliveryProtocolV1, dispatchAttemptID, payloadHash))
	submissionID := normalizeDeliverySubmissionID(dispatchAttemptID)
	_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET id = ?, payload_hash = ?, dispatch_attempt_id = ? WHERE id = ?`, submissionID, payloadHash, dispatchAttemptID, row.ID)
	require.NoError(t, err)
	row.ID = submissionID
	row.DispatchAttemptID = dispatchAttemptID
	row.PayloadHash = payloadHash
	require.Error(t, service.deliveryRecoveryError(ctx, row.SessionID, "prompt_dispatch_failed", &agentDeliverySubmissionRuntime{id: row.ID}))
	block, err = service.GetOpenSessionRecoveryBlock(ctx, row.SessionID)
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, row.ID, block.DeliverySubmissionID)
	require.Empty(t, block.DeliveryStreamID, "dispatch failure leaves the stream unbound")
	running, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	running.Runtime = runtimekind.RuntimeStandalone
	running.ExecutorID = models.ExecutorIDWorktree
	running.Status = models.ExecutorRunningStatusStopped
	running.Resumable = true
	running.ResumeToken = "native-conversation"
	running.LocalPID = 0
	if running.Metadata == nil {
		running.Metadata = make(map[string]interface{})
	}
	running.Metadata[models.SessionMetaKeyDeliveryPromptTurnBinding] = models.AgentDeliveryPromptTurnBinding{
		OriginalRuntime:   processidentity.Identity{PID: 42, GroupID: 42, SessionID: 42, BirthToken: "test:owner:1"},
		PromptGeneration:  1,
		TaskID:            "task1",
		SessionID:         row.SessionID,
		ExecutionID:       running.AgentExecutionID,
		IncarnationID:     row.IncarnationID,
		HarnessGeneration: uint64(row.HarnessGeneration),
		StreamID:          streamID,
		SubmissionID:      row.ID,
		TurnID:            "turn-retained-cancellation",
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))

	root := t.TempDir()
	storage := journal.CheckStorage(root, row.SessionID)
	store, err := journal.Open(journal.Config{Path: storage.Path})
	require.NoError(t, err)
	peerHash := row.PayloadHash
	if options.mismatchHash {
		peerHash = "foreign-hash"
	}
	peer := journal.Submission{ID: row.ID, SessionID: row.SessionID, IncarnationID: row.IncarnationID,
		HarnessGeneration: uint64(row.HarnessGeneration), StreamID: streamID,
		Hash: peerHash, Payload: row.Payload, State: journal.SubmissionDispatching}
	_, err = store.PutSubmission(ctx, peer)
	require.NoError(t, err)
	prefix := journal.Event{SessionID: row.SessionID, IncarnationID: row.IncarnationID,
		HarnessGeneration: uint64(row.HarnessGeneration), StreamID: streamID,
		SubmissionID: row.ID, Type: "message_chunk", Payload: []byte(`{"type":"message_chunk","text":"retained prefix"}`)}
	prefix, err = store.Append(ctx, prefix)
	require.NoError(t, err)
	sqlPrefix := &models.AgentDeliveryEvent{SessionID: row.SessionID, IncarnationID: row.IncarnationID,
		HarnessGeneration: row.HarnessGeneration, StreamID: streamID,
		Sequence: int64(prefix.Sequence), SubmissionID: row.ID, EventType: prefix.Type,
		Payload: prefix.Payload}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, sqlPrefix, int64(prefix.Sequence))
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, sqlPrefix, nil)
	require.NoError(t, err)
	require.NoError(t, store.Acknowledge(ctx, streamID, prefix.Sequence))
	cancel := journal.Event{SessionID: row.SessionID, IncarnationID: row.IncarnationID,
		HarnessGeneration: uint64(row.HarnessGeneration), StreamID: streamID,
		SubmissionID: row.ID, Type: "complete", Terminal: true,
		Payload: []byte(`{"type":"complete","data":{"stop_reason":"cancelled"}}`)}
	require.NoError(t, store.CancelSubmission(ctx, row.ID, cancel))
	if options.receiveTerminalWithoutProjection {
		cancelled, err := store.GetSubmission(ctx, row.ID)
		require.NoError(t, err)
		received := &models.AgentDeliveryEvent{SessionID: cancel.SessionID, IncarnationID: cancel.IncarnationID,
			HarnessGeneration: row.HarnessGeneration, StreamID: cancel.StreamID, Sequence: int64(cancelled.TerminalSequence),
			SubmissionID: cancel.SubmissionID, EventType: cancel.Type, Payload: cancel.Payload, Terminal: cancel.Terminal}
		_, err = repo.ReceiveAgentDeliveryEvent(ctx, received, received.Sequence)
		require.NoError(t, err)
	}
	require.NoError(t, store.Close())
	reopened, err := journal.Open(journal.Config{Path: storage.Path})
	require.NoError(t, err)
	cancelled, err := reopened.GetSubmission(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, journal.SubmissionCancelled, cancelled.State)
	require.EqualValues(t, 2, cancelled.TerminalSequence)
	require.NoError(t, reopened.Close())

	service.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
	return configureRetainedCancellationRestart(t, root, service, manager, repo, block, row, launched, streamID, queue)
}

func configureRetainedCancellationRestart(t *testing.T, root string, service *Service, manager *nativeResumeTestManager, repo *sqliterepo.Repository, block *models.SessionRecoveryBlock, row *models.AgentDeliverySubmission, launched **executor.LaunchAgentRequest, streamID string, queue *messagequeue.Service) *retainedCancellationRestartFixture {
	t.Helper()
	runtimeManager := newRetainedCancellationLifecycleManager(t, root, repo)
	proofManager := &nativeProofTestManager{nativeResumeTestManager: manager, proof: func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
		running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return &models.NativeResumeDeliveryEvidence{SessionID: sessionID, IncarnationID: row.IncarnationID,
			HarnessGeneration: row.HarnessGeneration, NativeSessionID: "native-conversation", ExecutionID: running.AgentExecutionID,
			StreamID: streamID, Submissions: []models.NativeResumeSubmissionEvidence{{ID: row.ID,
				HarnessGeneration: row.HarnessGeneration, PayloadHash: row.PayloadHash, StreamID: streamID,
				TerminalStreamID: streamID, TerminalSequence: 2, State: models.DeliverySubmissionCancelled,
				Outcome: string(models.DeliverySubmissionCancelled)}}}, nil
	}}
	wrapper := &legacyNativeTestManager{nativeProofTestManager: proofManager, runtimeManager: runtimeManager}
	wrapper.inspect = func(ctx context.Context, request agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error) {
		evidence, err := runtimeManager.InspectDeliveryRecordEvidence(ctx, request)
		if evidence != nil {
			terminated := true
			evidence.ProcessTerminated = &terminated
		}
		return evidence, err
	}
	manager.acknowledge = func(ctx context.Context, _ string) error {
		settled, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
		if err != nil {
			return err
		}
		if settled.State != models.RecoveryBlockResolved {
			return errors.New("retained cancellation block was not settled before resume")
		}
		return nil
	}
	service.agentManager = wrapper
	return &retainedCancellationRestartFixture{service: service, manager: wrapper, repo: repo, queue: queue,
		block: block, row: row, launched: launched, streamID: streamID}
}

func newRetainedCancellationLifecycleManager(t *testing.T, root string, repo *sqliterepo.Repository) *lifecycle.Manager {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/instances" {
			_ = json.NewEncoder(w).Encode(map[string]any{"instances": []*runtimeagentctl.InstanceInfo{}})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/instances/old-execution" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/delivery/retained" {
			var request journal.RetainedRecoveryRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := journal.ReadRetainedRecovery(r.Context(), request)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/delivery/retained/evidence" {
			var request journal.RetainedReconstructionEvidenceRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := journal.ReadRetainedReconstructionEvidence(r.Context(), request)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/delivery/retained/submission" {
			var request journal.RetainedReconstructionSubmissionRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			result, err := journal.ReadRetainedReconstructionSubmission(r.Context(), request)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(endpoint.Port())
	require.NoError(t, err)
	owner := runtimeagentctl.NewRuntimeOwner(nil, testLogger(), "test")
	t.Cleanup(owner.Stop)
	binding, err := owner.PrepareBinding()
	require.NoError(t, err)
	require.NoError(t, binding.Configure(endpoint.Hostname(), port, "retained-cancellation-token", 0, nil))
	require.NoError(t, binding.Commit())
	manager := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackWarn, root, testLogger())
	manager.SetExecutorRunningWriter(repo)
	manager.SetAgentDeliveryRepository(repo)
	manager.SetRuntimeOwner(owner)
	t.Cleanup(func() { _ = manager.Stop() })
	return manager
}

func assertRetainedCancellationSettled(t *testing.T, fixture *retainedCancellationRestartFixture) {
	t.Helper()
	ctx := context.Background()
	stored, err := fixture.repo.GetAgentDeliverySubmission(ctx, fixture.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCancelled, stored.State)
	require.Equal(t, string(models.DeliverySubmissionCancelled), stored.Outcome)
	block, err := fixture.repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)
	require.Equal(t, fixture.streamID, block.DeliveryStreamID)
	require.EqualValues(t, 2, block.DeliverySequence)
	require.Equal(t, string(models.DeliverySubmissionCancelled), block.DeliveryOutcome)
	effect, err := fixture.repo.GetAgentDeliveryEffect(ctx, "agent_delivery.terminal:"+fixture.streamID+":2")
	require.NoError(t, err)
	require.Equal(t, string(models.DeliverySubmissionCancelled), effect.Outcome)
}

func bindRetainedCancellationBlockStream(t *testing.T, fixture *retainedCancellationRestartFixture, streamID string) {
	t.Helper()
	_, err := fixture.repo.DB().ExecContext(context.Background(),
		`UPDATE session_recovery_blocks SET delivery_stream_id = ? WHERE id = ?`, streamID, fixture.block.ID)
	require.NoError(t, err)
	fixture.block.DeliveryStreamID = streamID
}

func newRetainedCancellationQueue(t *testing.T, path, taskID, sessionID, incarnationID string) (*messagequeue.Service, *sqlx.DB) {
	t.Helper()
	raw, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	for _, statement := range []string{
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at TIMESTAMP NULL, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, queue_incarnation_id TEXT NOT NULL)`,
	} {
		_, err = db.Exec(statement)
		require.NoError(t, err)
	}
	_, err = db.Exec(`INSERT INTO tasks (id) VALUES (?)`, taskID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO task_sessions (id, task_id, queue_incarnation_id) VALUES (?, ?, ?)`, sessionID, taskID, incarnationID)
	require.NoError(t, err)
	repository, err := messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)
	return messagequeue.NewService(repository, messagequeue.DefaultMaxPerSession, testLogger()), db
}

func TestLegacyNativeRestartWithoutRecoveryNotice(t *testing.T) {
	for _, condition := range []string{"dead owner", "backlog", "queue", "retired history", "generic history", "unprojected output", "live owner", "hash mismatch", "retained terminal", "changed owner", "independent work"} {
		t.Run(condition, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, row, launched, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
			repo := service.repo.(*sqliterepo.Repository)
			require.NoError(t, repo.UpdateTaskSessionState(ctx, row.SessionID, models.TaskSessionStateWaitingForInput, ""))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, row.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
			_, err := repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`, block.ID)
			require.NoError(t, err)
			if condition == "queue" {
				_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET id = 'queue:delivery' WHERE id = ?`, row.ID)
				require.NoError(t, err)
				row.ID = "queue:delivery"
				block.DeliverySubmissionID = row.ID
				_, err = repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_submission_id = ? WHERE id = ?`, row.ID, block.ID)
				require.NoError(t, err)
			}
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "journal.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			peer := journal.Submission{ID: row.ID, SessionID: row.SessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1, StreamID: block.DeliveryStreamID, Hash: row.PayloadHash, Payload: row.Payload, State: journal.SubmissionFailed}
			_, err = store.PutSubmission(ctx, peer)
			require.NoError(t, err)
			if condition == "unprojected output" {
				_, err = store.Append(ctx, journal.Event{SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: 1, StreamID: peer.StreamID, SubmissionID: peer.ID, Type: "tool_call", Payload: []byte("unprojected")})
				require.NoError(t, err)
			}
			if condition == "retired history" || condition == "generic history" {
				older := *row
				older.ID = "prior"
				older.PayloadHash = "prior-hash"
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &older)
				require.NoError(t, err)
				retired := peer
				retired.ID = older.ID
				retired.Hash = older.PayloadHash
				retired.Retired = true
				_, err = store.PutSubmission(ctx, retired)
				require.NoError(t, err)
				if condition == "generic history" {
					_, err = repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_submission_id = '', reason = 'unresolved_durable_work' WHERE id = ?`, block.ID)
					require.NoError(t, err)
				}
			}
			calls := 0
			identity := agentruntime.AgentDeliveryRecoveryIdentity{TaskID: "task1", SessionID: row.SessionID, ExecutionID: "old-execution", IncarnationID: row.IncarnationID, HarnessGeneration: 1, StreamID: peer.StreamID, SubmissionID: peer.ID, PromptGeneration: 1, OriginalRuntime: processidentity.Identity{PID: 42, GroupID: 42, SessionID: 42, BirthToken: "linux:boot:1"}}
			wrapper := &legacyNativeTestManager{nativeProofTestManager: &nativeProofTestManager{nativeResumeTestManager: manager}}
			wrapper.inspect = func(ctx context.Context, request agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error) {
				calls++
				d, err := store.RecoveryDescriptor(ctx, row.SessionID, row.IncarnationID, 1, peer.StreamID)
				require.NoError(t, err)
				if request.StoredSubmissionID == "" {
					return &agentruntime.DeliveryRecordEvidence{Descriptor: d}, nil
				}
				p, err := store.GetSubmission(ctx, request.StoredSubmissionID)
				require.NoError(t, err)
				dead := condition != "live owner"
				binding := identity
				switch condition {
				case "hash mismatch":
					p.Hash = "foreign"
				case "retained terminal":
					p.TerminalEventRetained = true
					p.TerminalSequence = 1
				case "changed owner":
					if calls > 1 {
						binding.ExecutionID = "foreign"
					}
				case "independent work":
					d.SubmissionCount = 1
				}
				return &agentruntime.DeliveryRecordEvidence{Descriptor: d, StoredSubmission: &p, ControlIdentity: &binding, ProcessTerminated: &dead}, nil
			}
			originalAck := manager.acknowledge
			manager.acknowledge = func(ctx context.Context, sessionID string) error {
				require.NoError(t, originalAck(ctx, sessionID))
				_, err := store.RetireSubmission(ctx, peer.ID, 1)
				return err
			}
			wrapper.proof = func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
				running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
				require.NoError(t, err)
				retired, err := store.GetSubmission(ctx, peer.ID)
				require.NoError(t, err)
				require.True(t, retired.Retired)
				proof := &models.NativeResumeDeliveryEvidence{SessionID: sessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1, NativeSessionID: "native-conversation", ExecutionID: running.AgentExecutionID, StreamID: peer.StreamID, Submissions: []models.NativeResumeSubmissionEvidence{{ID: peer.ID, HarnessGeneration: 1, PayloadHash: peer.Hash, StreamID: peer.StreamID}}}
				if condition == "retired history" || condition == "generic history" {
					proof.Submissions = append(proof.Submissions, models.NativeResumeSubmissionEvidence{ID: "prior", HarnessGeneration: 1, PayloadHash: "prior-hash", StreamID: peer.StreamID})
				}
				return proof, nil
			}
			service.agentManager = wrapper
			if condition == "backlog" {
				_, err = service.messageQueue.QueueMessage(ctx, row.SessionID, "task1", "queued instruction", "", "user", false, nil)
				require.NoError(t, err)
				require.NoError(t, service.messageQueue.SetAutoRun(ctx, row.SessionID, true))
			}
			session, err := repo.GetTaskSession(ctx, row.SessionID)
			require.NoError(t, err)
			restored, err := service.resumeLegacyNativeRestart(ctx, session)
			if condition == "changed owner" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			want := condition == "dead owner" || condition == "backlog" || condition == "queue" || condition == "retired history" || condition == "generic history"
			require.Equal(t, want, restored)
			if restored {
				require.NotNil(t, *launched)
				saved, err := store.GetSubmission(ctx, peer.ID)
				require.NoError(t, err)
				require.True(t, saved.Retired)
				require.Equal(t, journal.SubmissionFailed, saved.State)
				again, err := service.resumeLegacyNativeRestart(ctx, session)
				require.NoError(t, err)
				require.False(t, again)
			} else {
				require.Nil(t, *launched)
			}
			if condition == "backlog" {
				status := service.messageQueue.GetStatus(ctx, row.SessionID)
				require.False(t, status.AutoRun, "native restoration must not release queued instructions")
				require.Len(t, status.Entries, 1)
			}
			require.Empty(t, manager.capturedPromptCalls)
		})
	}
}

func TestLegacyNativeRestartSkipsPersistedRecovery(t *testing.T) {
	ctx := context.Background()
	service, manager, _, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
	repo := service.repo.(*sqliterepo.Repository)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, submission.SessionID, models.TaskSessionStateWaitingForInput, ""))
	session, err := repo.GetTaskSession(ctx, submission.SessionID)
	require.NoError(t, err)
	_, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, exists)

	var inspectCalls int
	wrapper := &legacyNativeTestManager{
		nativeProofTestManager: &nativeProofTestManager{nativeResumeTestManager: manager},
		inspect: func(context.Context, agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error) {
			inspectCalls++
			return nil, errors.New("durable adoption evidence unavailable")
		},
	}
	service.agentManager = wrapper

	restored, err := service.resumeLegacyNativeRestart(ctx, session)
	require.NoError(t, err)
	require.False(t, restored)
	require.Zero(t, inspectCalls, "persisted recovery must use the restart worker instead of the legacy probe")
}
