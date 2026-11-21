package lifecycle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

const repeatedNativeResumeFixtureMarker = "KANDEV_REPEATED_NATIVE_RESUME_FIXTURE:"

func openRepeatedNativeResumeFixtureJournal(ctx context.Context, path string) (*journal.Journal, error) {
	const attemptTimeout = 50 * time.Millisecond
	const retryInterval = 50 * time.Millisecond
	// Session stop is asynchronous. Wait for the journal itself to become
	// available instead of treating the cancelled session state as owner release.
	deadline := time.Now().Add(35 * time.Second)
	for {
		retained, err := journal.Open(journal.Config{Path: path, OpenTimeout: attemptTimeout})
		if err == nil {
			return retained, nil
		}
		var openErr *journal.OpenError
		if !errors.As(err, &openErr) || openErr.Reason() != journal.OpenReasonLockTimeout {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("timed out waiting for the existing journal owner to release its lock: %w", err)
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("waiting for the existing journal owner to release its lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func TestRepeatedNativeResumeFixtureWaitsForJournalRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	owner, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	release := time.AfterFunc(200*time.Millisecond, func() {
		_ = owner.Close()
	})
	t.Cleanup(func() {
		release.Stop()
		_ = owner.Close()
	})

	fixture, err := openRepeatedNativeResumeFixtureJournal(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, fixture.Close())
}

type repeatedNativeResumeFixtureResult struct {
	SubmissionID           string `json:"submission_id"`
	BoundBlockID           string `json:"bound_block_id"`
	GenericBlockID         string `json:"generic_block_id"`
	IndependentBlockID     string `json:"independent_block_id,omitempty"`
	HistoricalSubmissionID string `json:"historical_submission_id"`
	HistoricalBlockID      string `json:"historical_block_id"`
	HistoricalStreamID     string `json:"historical_stream_id"`
	HistoricalPayload      string `json:"historical_payload"`
	HistoricalPayloadHash  string `json:"historical_payload_hash"`
	IncarnationID          string `json:"incarnation_id"`
	Generation             int64  `json:"generation"`
	StreamID               string `json:"stream_id"`
	Payload                string `json:"payload"`
	PayloadHash            string `json:"payload_hash"`
}

// TestRepeatedNativeResumeFixture seeds complete generation-owned evidence for
// the isolated browser regression, including a projected journal stream.
func TestRepeatedNativeResumeFixture(t *testing.T) {
	root := os.Getenv("KANDEV_E2E_REPEAT_RESUME_ROOT")
	if root == "" {
		t.Skip("used only by the isolated managed browser fixture")
	}
	require.True(t, strings.HasPrefix(filepath.Base(root), "kandev-e2e-"))
	sessionID := os.Getenv("KANDEV_E2E_REPEAT_RESUME_SESSION")
	require.NotEmpty(t, sessionID)
	shape := os.Getenv("KANDEV_E2E_REPEAT_RESUME_SHAPE")
	require.Contains(t, []string{"matching", "independent"}, shape)
	result := seedRepeatedNativeResumeFixture(t, root, sessionID, shape)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	fmt.Printf("%s%s\n", repeatedNativeResumeFixtureMarker, encoded)
}

func seedRepeatedNativeResumeFixture(t *testing.T, root, sessionID, shape string) repeatedNativeResumeFixtureResult {
	t.Helper()
	ctx := context.Background()
	database, err := db.OpenSQLite(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	connection := sqlx.NewDb(database, "sqlite3")
	repo, err := tasksqlite.NewWithDB(connection, connection, nil)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	require.NoError(t, err)
	require.NotNil(t, generation)
	require.GreaterOrEqual(t, generation.Generation, int64(1))
	require.NotEmpty(t, generation.NativeSessionID)
	task, err := repo.GetTask(ctx, session.TaskID)
	require.NoError(t, err)
	workspace, err := repo.GetWorkspace(ctx, task.WorkspaceID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""))

	submissionID := os.Getenv("KANDEV_E2E_REPEAT_RESUME_SUBMISSION")
	boundBlockID := os.Getenv("KANDEV_E2E_REPEAT_RESUME_BOUND_BLOCK")
	genericBlockID := os.Getenv("KANDEV_E2E_REPEAT_RESUME_GENERIC_BLOCK")
	historicalSubmissionID := os.Getenv("KANDEV_E2E_REPEAT_RESUME_HISTORICAL_SUBMISSION")
	historicalBlockID := os.Getenv("KANDEV_E2E_REPEAT_RESUME_HISTORICAL_BLOCK")
	independentBlockID := os.Getenv("KANDEV_E2E_REPEAT_RESUME_INDEPENDENT_BLOCK")
	payload, err := base64.StdEncoding.DecodeString(os.Getenv("KANDEV_E2E_REPEAT_RESUME_PAYLOAD"))
	require.NoError(t, err)
	require.NotEmpty(t, submissionID)
	require.NotEmpty(t, boundBlockID)
	require.NotEmpty(t, genericBlockID)
	require.NotEmpty(t, historicalSubmissionID)
	require.NotEmpty(t, historicalBlockID)
	require.NotEmpty(t, payload)
	now := time.Now().UTC()

	journalRoot := filepath.Join(root, ".kandev")
	capability := journal.CheckStorage(journalRoot, sessionID)
	retained, err := openRepeatedNativeResumeFixtureJournal(ctx, capability.Path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, retained.Close()) })

	// Commit a real silent-restore repository transition before introducing the
	// distinct generation-two interruption exercised by the browser flow.
	historicalStreamID := fmt.Sprintf("%s:g%d", incarnationID, generation.Generation)
	historicalPayload := []byte("An older instruction with an unknown outcome.")
	historicalPayloadHash := journal.SubmissionHash(historicalPayload)
	_, err = retained.PutSubmission(ctx, journal.Submission{
		ID: historicalSubmissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: uint64(generation.Generation), StreamID: historicalStreamID,
		Hash: historicalPayloadHash, Payload: historicalPayload,
		State: journal.SubmissionInterruptedUnknown, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	historicalEvent, err := retained.Append(ctx, journal.Event{
		SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: uint64(generation.Generation), StreamID: historicalStreamID,
		Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	historicalDeliveryEvent := &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: generation.Generation, StreamID: historicalStreamID,
		Sequence: int64(historicalEvent.Sequence), EventType: historicalEvent.Type, Payload: historicalEvent.Payload,
	}
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, historicalDeliveryEvent, int64(historicalEvent.Sequence))
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, historicalDeliveryEvent, nil)
	require.NoError(t, err)
	require.NoError(t, retained.Acknowledge(ctx, historicalStreamID, historicalEvent.Sequence))

	inserted, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: historicalSubmissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: generation.Generation, OwnerGeneration: generation.Generation,
		PayloadHash: historicalPayloadHash, Payload: historicalPayload,
		State: models.DeliverySubmissionInterruptedUnknown, Outcome: "prompt_dispatch_failed",
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	require.True(t, inserted)
	historicalRecovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, Revision: 1, SessionID: sessionID,
		AgentExecutionID: "repeated-native-resume-source-execution", SubmissionID: historicalSubmissionID,
		StreamID: historicalStreamID, IncarnationID: incarnationID,
		HarnessGeneration: generation.Generation, PromptGeneration: 1, UpdatedAt: now,
	}
	historicalBlock := &models.SessionRecoveryBlock{
		ID: historicalBlockID, SessionID: sessionID, IncarnationID: incarnationID,
		ExpectedGeneration: generation.Generation, Reason: "unknown_prompt_outcome",
		State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		DeliverySubmissionID: historicalSubmissionID, DeliveryStreamID: historicalStreamID,
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyAgentDeliveryRecovery, historicalRecovery))
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, historicalBlock))
	loadedSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	historicalRecovery, stored := models.LoadAgentDeliveryRecovery(loadedSession.Metadata)
	require.True(t, stored)
	historicalBlock, err = repo.GetSessionRecoveryBlock(ctx, historicalBlockID)
	require.NoError(t, err)

	attemptID := "repeated-native-resume-restore-" + historicalSubmissionID
	attempt := &models.RestoreAttempt{
		ID: attemptID, SessionID: sessionID, IncarnationID: incarnationID,
		ExpectedGeneration: generation.Generation, Action: models.SilentRestoreAction,
		Outcome: models.SilentRestoreOutcomePending, TargetWorkspace: workspace.ID, CreatedAt: now,
		Checkpoint: &models.SilentRestoreCheckpoint{
			Version: 1, Stage: models.SilentRestoreStagePrepared,
			SourceRecovery: historicalRecovery, SourceGeneration: *generation, SourceBlock: *historicalBlock,
			TaskID: task.ID, WorkspaceID: workspace.ID, WorkspaceOwnerID: workspace.OwnerID,
			WorkspaceOrgID: workspace.OrgID,
		},
	}
	storedAttempt, _, err := repo.EnsureRestoreAttempt(ctx, attempt)
	require.NoError(t, err)
	require.NotNil(t, storedAttempt.Checkpoint)
	allocated := *storedAttempt.Checkpoint
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	allocated.CandidateExecutionID = "repeated-native-resume-candidate-" + historicalSubmissionID
	changed, err := repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, *storedAttempt.Checkpoint, allocated)
	require.NoError(t, err)
	require.True(t, changed)
	launching := allocated
	launching.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, allocated, launching)
	require.NoError(t, err)
	require.True(t, changed)
	running := &models.ExecutorRunning{
		ID: "repeated-native-resume-running-" + sessionID, SessionID: sessionID, TaskID: task.ID,
		ExecutionProfileID: session.ExecutionProfileID, ExecutorID: session.ExecutorID,
		AgentExecutionID: launching.CandidateExecutionID, ResumeToken: generation.NativeSessionID,
		Status: "running",
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	commit := &models.SilentRestoreCommit{
		AttemptID: attemptID, Checkpoint: launching,
		Generation: models.HarnessSessionGeneration{
			SessionID: sessionID, IncarnationID: incarnationID,
			Generation: generation.Generation + 1, PredecessorGeneration: generation.Generation,
			NativeSessionID: generation.NativeSessionID, AgentType: generation.AgentType,
			AdapterVersion: generation.AdapterVersion, OriginalWorkspace: generation.OriginalWorkspace,
			CurrentWorkspace: generation.CurrentWorkspace, NativeStateReference: generation.NativeStateReference,
			CreationReason: models.SilentRestoreAction,
		},
	}
	changed, err = repo.CommitSilentRestore(ctx, commit)
	require.NoError(t, err)
	var commitEvidence struct {
		State            string `db:"state"`
		IncarnationID    string `db:"queue_incarnation_id"`
		RouteState       string `db:"route_state"`
		OpenBlockCount   int    `db:"open_block_count"`
		CandidateCount   int    `db:"candidate_count"`
		SubmissionState  string `db:"submission_state"`
		SourceBlockState string `db:"source_block_state"`
	}
	require.NoError(t, connection.GetContext(ctx, &commitEvidence, `SELECT ts.state, ts.queue_incarnation_id,
		ts.route_state, (SELECT COUNT(*) FROM session_recovery_blocks b WHERE b.session_id = ts.id AND b.incarnation_id = ? AND b.state = 'open') AS open_block_count,
		(SELECT COUNT(*) FROM executors_running e WHERE e.session_id = ts.id AND e.task_id = ? AND e.agent_execution_id = ?) AS candidate_count,
		(SELECT state FROM agent_delivery_submissions WHERE id = ?) AS submission_state,
		(SELECT state FROM session_recovery_blocks WHERE id = ?) AS source_block_state
		FROM task_sessions ts WHERE ts.id = ?`, incarnationID, task.ID, launching.CandidateExecutionID,
		historicalSubmissionID, historicalBlockID, sessionID))
	require.True(t, changed, "silent restore commit rejected: %+v", commitEvidence)

	generation, err = repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	require.NoError(t, err)
	require.Equal(t, commit.Generation.Generation, generation.Generation)
	streamID := fmt.Sprintf("%s:g%d", incarnationID, generation.Generation)
	payloadHash := journal.SubmissionHash(payload)

	_, err = retained.PutSubmission(ctx, journal.Submission{
		ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: uint64(generation.Generation), StreamID: streamID,
		Hash: payloadHash, Payload: payload, State: journal.SubmissionInterruptedUnknown,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	event, err := retained.Append(ctx, journal.Event{
		SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: uint64(generation.Generation), StreamID: streamID,
		Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	deliveryEvent := &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: generation.Generation, StreamID: streamID,
		Sequence: int64(event.Sequence), EventType: event.Type, Payload: event.Payload,
	}
	inserted, err = repo.ReceiveAgentDeliveryEvent(ctx, deliveryEvent, int64(event.Sequence))
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, deliveryEvent, nil)
	require.NoError(t, err)
	require.NoError(t, retained.Acknowledge(ctx, streamID, event.Sequence))

	inserted, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: generation.Generation, OwnerGeneration: generation.Generation,
		PayloadHash: payloadHash, Payload: payload,
		State: models.DeliverySubmissionInterruptedUnknown, Outcome: "prompt_dispatch_failed",
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	require.True(t, inserted)
	for _, block := range []*models.SessionRecoveryBlock{
		{
			ID: boundBlockID, SessionID: sessionID, IncarnationID: incarnationID,
			ExpectedGeneration: generation.Generation, Reason: "unknown_prompt_outcome",
			State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
			DeliverySubmissionID: submissionID, DeliveryStreamID: streamID,
		},
		{
			ID: genericBlockID, SessionID: sessionID, IncarnationID: incarnationID,
			ExpectedGeneration: generation.Generation, Reason: "unresolved_durable_work",
			State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		},
	} {
		require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	}
	if shape == "independent" {
		require.NotEmpty(t, independentBlockID)
		require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
			ID: independentBlockID, SessionID: sessionID, IncarnationID: incarnationID,
			ExpectedGeneration: generation.Generation, Reason: "office_restriction",
			State: models.RecoveryBlockOpen, ConsumerReference: "office",
		}))
	}
	require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed,
		"A successor-generation prompt was interrupted."))
	return repeatedNativeResumeFixtureResult{
		SubmissionID: submissionID, BoundBlockID: boundBlockID, GenericBlockID: genericBlockID,
		IndependentBlockID: independentBlockID, IncarnationID: incarnationID,
		HistoricalSubmissionID: historicalSubmissionID, HistoricalBlockID: historicalBlockID,
		HistoricalStreamID:    historicalStreamID,
		HistoricalPayload:     base64.StdEncoding.EncodeToString(historicalPayload),
		HistoricalPayloadHash: historicalPayloadHash,
		Generation:            generation.Generation, StreamID: streamID,
		Payload: base64.StdEncoding.EncodeToString(payload), PayloadHash: payloadHash,
	}
}
