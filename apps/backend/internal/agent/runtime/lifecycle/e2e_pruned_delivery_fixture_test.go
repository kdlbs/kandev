package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func seedPrunedDeliveryIncident(t *testing.T, repo *tasksqlite.Repository, root, sessionID string) {
	t.Helper()
	require.Equal(t, "linux", runtime.GOOS, "pruned recovery fixture uses Linux isolated-session death proof")

	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	incarnation := session.QueueIncarnationID
	if incarnation == "" {
		incarnation = session.ID
	}
	current, err := repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnation)
	if err != nil {
		require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: sessionID, IncarnationID: incarnation, Generation: 1, NativeSessionID: "native-e2e-record-" + sessionID, CreationReason: "initial"}))
		current, err = repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnation)
	}
	require.NoError(t, err)
	require.NotEmpty(t, current.NativeSessionID)
	generation := current.Generation + 1
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: sessionID, IncarnationID: incarnation, Generation: generation, NativeSessionID: current.NativeSessionID, CurrentWorkspace: current.CurrentWorkspace, CreationReason: "restore"}))
	streamID := fmt.Sprintf("%s:g%d", incarnation, generation)
	messageID := "message-e2e-record-" + sessionID + "-1"
	executionID := "dead-pruned-owner-" + sessionID
	payload := []byte(`{"text":"Inspect the saved work and continue from the same conversation"}`)
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "prompt:" + messageID, SessionID: sessionID, IncarnationID: incarnation, HarnessGeneration: generation, OwnerGeneration: generation, DispatchAttemptID: messageID, PayloadHash: journal.SubmissionHash(payload), Payload: payload, State: models.DeliverySubmissionDispatching})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-e2e-record-" + sessionID, TaskID: session.TaskID, TaskSessionID: sessionID}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: messageID, TaskID: session.TaskID, TaskSessionID: sessionID, TurnID: "turn-e2e-record-" + sessionID, AuthorType: models.MessageAuthorUser, Content: "Inspect the saved work and continue from the same conversation", Metadata: map[string]interface{}{models.MessageMetaKeyDeliveryStatus: models.MessageDeliveryStatusBlocked}}))
	event := &models.AgentDeliveryEvent{SessionID: sessionID, IncarnationID: incarnation, HarnessGeneration: generation, StreamID: streamID, Sequence: 1, EventType: "context_window", Payload: []byte(`{}`)}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, &models.AgentDeliveryEffect{EffectKey: streamID + ":context:1", StreamID: streamID, Sequence: 1, EffectType: "context_window"})
	require.NoError(t, err)
	capability := journal.CheckStorage(root+"/.kandev", sessionID)
	retained, err := journal.Open(journal.Config{Path: capability.Path})
	require.NoError(t, err)
	_, err = retained.PutSubmission(ctx, journal.Submission{ID: "prompt:" + messageID, SessionID: sessionID, IncarnationID: incarnation, HarnessGeneration: uint64(generation), StreamID: streamID, Hash: journal.SubmissionHash(payload), Payload: payload, State: journal.SubmissionInterruptedUnknown})
	require.NoError(t, err)
	for range 6 {
		_, err = retained.Append(ctx, journal.Event{SessionID: sessionID, IncarnationID: incarnation, HarnessGeneration: uint64(generation), StreamID: streamID, SubmissionID: "prompt:" + messageID, Type: "context_window", Payload: []byte(`{}`)})
		require.NoError(t, err)
	}
	require.NoError(t, retained.Acknowledge(ctx, streamID, 6))
	require.NoError(t, retained.Close())
	owner := processidentity.Identity{PID: 2147483647, GroupID: 2147483647, SessionID: 2147483647, BirthToken: "linux:prior-boot:1"}
	binding := deliveryPromptTurnBinding{OriginalRuntime: owner, PromptGeneration: 1, TaskID: session.TaskID, SessionID: sessionID, ExecutionID: executionID, IncarnationID: incarnation, HarnessGeneration: uint64(generation), StreamID: streamID, SubmissionID: "prompt:" + messageID, TurnID: "turn-e2e-record-" + sessionID}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: executionID, TaskID: session.TaskID, SessionID: sessionID, AgentExecutionID: executionID, Runtime: "standalone", LocalPID: owner.PID, Status: models.ExecutorRunningStatusRunning, Metadata: map[string]interface{}{deliveryPromptTurnBindingMetadataKey: binding}}))
	session.AgentExecutionID = executionID
	session.State = models.TaskSessionStateWaitingForInput
	if session.Metadata == nil {
		session.Metadata = map[string]interface{}{}
	}
	session.Metadata["acp"] = map[string]interface{}{"session_id": current.NativeSessionID}
	delete(session.Metadata, models.SessionMetaKeyAgentDeliveryRecovery)
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	fmt.Println("KANDEV_E2E_RECORD_FIXTURE:seeded")
}

func assertPrunedDeliveryRestored(t *testing.T, repo *tasksqlite.Repository, sessionID string) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	raw, err := json.Marshal(session.Metadata[models.SessionMetaKeyAgentDeliveryHistoryGaps])
	require.NoError(t, err)
	var records map[string]models.AgentDeliveryHistoryGapRecord
	require.NoError(t, json.Unmarshal(raw, &records))
	var originalRecord models.AgentDeliveryHistoryGapRecord
	for _, record := range records {
		originalRecord = record
	}
	streamID := originalRecord.StreamID
	cursor, err := repo.GetAgentDeliveryCursor(ctx, streamID)
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ReceivedSequence, "the missing five events must not become fictional SQL receipts")
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	original, err := repo.GetAgentDeliverySubmission(ctx, "prompt:message-e2e-record-"+sessionID+"-1")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, original.State)
	require.Equal(t, "unknown", original.Outcome)
	require.Zero(t, original.TerminalSequence)
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, original.IncarnationID)
	require.NoError(t, err)
	require.Greater(t, generation.Generation, original.HarnessGeneration)
	require.Equal(t, originalRecord.NativeSessionID, generation.NativeSessionID)
	require.Equal(t, models.AgentDeliveryHistoryGap{FirstSequence: 2, LastSequence: 6, ReceivedSequence: 1, ProjectedSequence: 1}, records[streamID].Gap)
	require.Equal(t, generation.NativeSessionID, records[streamID].NativeSessionID)
	blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, sessionID, original.IncarnationID, original.HarnessGeneration)
	require.NoError(t, err)
	for _, block := range blocks {
		require.NotEqual(t, models.RecoveryBlockOpen, block.State)
	}
	fmt.Println("KANDEV_E2E_RECORD_FIXTURE:pruned_restored")
}

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
func TestPrunedDeliveryIncidentFixturePinsExactDeadOwnerAndCursor(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux isolated-session proof")
	}
	ctx := context.Background()
	root := t.TempDir()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Pruned delivery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput, QueueIncarnationID: "session", WorkspacePath: t.TempDir()}))
	seedPrunedDeliveryIncident(t, repo, root, "session")
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "session:g2")
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ReceivedSequence)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	require.Nil(t, session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery])
	manager := newTestManager(t)
	manager.runningWriter = repo
	capability := journal.CheckStorage(root+"/.kandev", "session")
	retained, err := journal.Open(journal.Config{Path: capability.Path})
	require.NoError(t, err)
	descriptor, err := retained.RecoveryDescriptor(ctx, "session", "session", 2, "")
	require.NoError(t, err)
	require.NoError(t, retained.Close())
	identity := manager.persistedDeliveryRecordControlIdentity(ctx, DeliveryRecordEvidenceRequest{TaskID: "task", SessionID: "session"}, descriptor.Submissions[0])
	require.NotNil(t, identity)
	result, err := journal.ReadRetainedRecovery(ctx, journal.RetainedRecoveryRequest{Root: root + "/.kandev", SessionID: "session", ExecutionID: identity.ExecutionID, IncarnationID: "session", HarnessGeneration: 2, StreamID: cursor.StreamID, SubmissionID: identity.SubmissionID, OriginalRuntime: identity.OriginalRuntime, After: 1, Limit: 4, PrunedThrough: 6, ExpectedHash: descriptor.Submissions[0].Hash})
	require.NoError(t, err)
	require.True(t, result.HistoryGapVerified)
	require.Equal(t, journal.SubmissionInterruptedUnknown, result.Submission.State)
	require.False(t, result.Submission.Retired)
}
