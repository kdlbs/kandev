package lifecycle

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type repeatedNativeResumeLifecycleFixture struct {
	manager           *Manager
	execution         *AgentExecution
	repo              *tasksqlite.Repository
	store             *journal.Journal
	databasePath      string
	journalPath       string
	oldSubmission     journal.Submission
	currentSubmission journal.Submission
	currentBlockIDs   []string
	historicalNotice  models.AgentDeliveryRecovery
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
func TestRepeatedNativeResumeAfterSilentRestoreJournal(t *testing.T) {
	for _, scenario := range []string{"matching duplicates after SQL rollback", "generic fence", "missing stream fence", "independent restriction"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRepeatedNativeResumeLifecycleFixture(t, scenario)
			require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
			requireRetiredUnknownHistory(t, f)

			if scenario == "matching duplicates after SQL rollback" {
				_, err := f.repo.DB().Exec(`CREATE TRIGGER reject_repeated_native_resume BEFORE UPDATE OF state ON session_recovery_blocks
					WHEN NEW.session_id = 'session' AND NEW.expected_generation = 2 AND NEW.state = 'resolved'
					BEGIN SELECT RAISE(ABORT, 'resolution refused'); END`)
				require.NoError(t, err)
				proof, err := f.manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
				require.NoError(t, err)
				changed, err := f.repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), f.currentBlockIDs[0], proof, time.Now().UTC())
				require.Error(t, err)
				require.False(t, changed)
				assertRepeatedRecoveryBlocksOpen(t, f, 2)
			}

			reopenRepeatedNativeResumeStores(t, f)
			require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"), "retained retirement must be reusable after reopening the journal")
			proof, err := f.manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
			require.NoError(t, err)
			require.Equal(t, int64(2), proof.HarnessGeneration)
			require.Equal(t, "native-conversation", proof.NativeSessionID)
			require.Len(t, proof.Submissions, 2)

			if scenario == "matching duplicates after SQL rollback" {
				_, err := f.repo.DB().Exec(`DROP TRIGGER reject_repeated_native_resume`)
				require.NoError(t, err)
			}

			changed, err := f.repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), f.currentBlockIDs[0], proof, time.Now().UTC())
			if scenario == "independent restriction" {
				require.NoError(t, err)
				require.False(t, changed)
				assertRepeatedRecoveryBlocksOpen(t, f, 3)
			} else {
				require.NoError(t, err)
				require.True(t, changed)
				assertRepeatedRecoveryBlocksResolved(t, f)
			}
			requireRetiredUnknownHistory(t, f)
			assertRepeatedHistoricalNotice(t, f)
		})
	}
}

func newRepeatedNativeResumeLifecycleFixture(t *testing.T, scenario string) *repeatedNativeResumeLifecycleFixture {
	t.Helper()
	ctx := t.Context()
	f := &repeatedNativeResumeLifecycleFixture{
		journalPath:  filepath.Join(t.TempDir(), "delivery.bbolt"),
		databasePath: filepath.Join(t.TempDir(), "delivery.db"),
		oldSubmission: journal.Submission{
			ID: "historical-prompt", SessionID: "session", IncarnationID: "incarnation",
			HarnessGeneration: 1, StreamID: "stream", Hash: "historical-hash",
			Payload: []byte("original historical prompt"), State: journal.SubmissionInterruptedUnknown,
		},
		currentSubmission: journal.Submission{
			ID: "current-prompt", SessionID: "session", IncarnationID: "incarnation",
			HarnessGeneration: 2, StreamID: "stream", Hash: "current-hash",
			Payload: []byte("distinct current instruction"), State: journal.SubmissionInterruptedUnknown,
		},
	}
	f.store = openRetentionLifecycleJournal(t, f.journalPath)
	f.repo, _ = openRetentionLifecycleRepository(t, f.databasePath)
	t.Cleanup(func() {
		_ = f.store.Close()
		_ = f.repo.DB().Close()
	})
	require.NoError(t, f.repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Repeated restore", OwnerID: "owner", OrgID: "org"}))
	require.NoError(t, f.repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Repeated restore"}))
	require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session", TaskID: "task", QueueIncarnationID: "incarnation", State: models.TaskSessionStateFailed,
		Metadata: map[string]any{"acp": map[string]any{"session_id": "native-conversation"}},
	}))
	sourceGeneration := models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 1,
		NativeSessionID: "native-conversation", AgentType: "test", CreationReason: "initial",
	}
	require.NoError(t, f.repo.CreateHarnessSessionGeneration(ctx, &sourceGeneration))
	storedGeneration, err := f.repo.GetCurrentHarnessSessionGeneration(ctx, "session", "incarnation")
	require.NoError(t, err)
	sourceGeneration = *storedGeneration
	require.NoError(t, f.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session", SessionID: "session", TaskID: "task", AgentExecutionID: "old-execution",
		ResumeToken: "native-conversation", Status: models.ExecutorRunningStatusRunning,
	}))
	_, err = f.store.PutSubmission(ctx, f.oldSubmission)
	require.NoError(t, err)
	_, err = f.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: f.oldSubmission.ID, SessionID: f.oldSubmission.SessionID, IncarnationID: f.oldSubmission.IncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: f.oldSubmission.Hash,
		Payload: f.oldSubmission.Payload, State: models.DeliverySubmissionInterruptedUnknown,
		Outcome: "prompt_dispatch_failed",
	})
	require.NoError(t, err)
	recovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, Revision: 1, SessionID: "session",
		AgentExecutionID: "old-execution", SubmissionID: f.oldSubmission.ID, StreamID: f.oldSubmission.StreamID,
		IncarnationID: "incarnation", HarnessGeneration: 1, PromptGeneration: 1, UpdatedAt: time.Now().UTC(),
	}
	sourceBlock := &models.SessionRecoveryBlock{
		ID: "historical-block", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 1,
		Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		DeliverySubmissionID: f.oldSubmission.ID, DeliveryStreamID: f.oldSubmission.StreamID,
	}
	stored, err := f.repo.UpsertAgentDeliveryRecovery(ctx, &recovery, sourceBlock)
	require.NoError(t, err)
	require.True(t, stored)
	session, err := f.repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	recovery, _ = models.LoadAgentDeliveryRecovery(session.Metadata)
	sourceBlock, err = f.repo.GetSessionRecoveryBlock(ctx, sourceBlock.ID)
	require.NoError(t, err)
	checkpoint := models.SilentRestoreCheckpoint{
		Version: 1, Stage: models.SilentRestoreStagePrepared, SourceRecovery: recovery,
		SourceGeneration: sourceGeneration, SourceBlock: *sourceBlock, TaskID: "task", WorkspaceID: "workspace",
		WorkspaceOwnerID: "owner", WorkspaceOrgID: "org",
	}
	attempt := &models.RestoreAttempt{
		ID: "restore-attempt", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 1,
		Action: models.SilentRestoreAction, Outcome: models.SilentRestoreOutcomePending,
		TargetWorkspace: "workspace", CreatedAt: time.Now().UTC(), Checkpoint: &checkpoint,
	}
	storedAttempt, _, err := f.repo.EnsureRestoreAttempt(ctx, attempt)
	require.NoError(t, err)
	allocated := *storedAttempt.Checkpoint
	allocated.Stage = models.SilentRestoreStageCandidateAllocated
	allocated.CandidateExecutionID = "execution"
	changed, err := f.repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, *storedAttempt.Checkpoint, allocated)
	require.NoError(t, err)
	require.True(t, changed)
	launching := allocated
	launching.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err = f.repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, allocated, launching)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, f.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session", SessionID: "session", TaskID: "task", AgentExecutionID: "execution",
		ResumeToken: "native-conversation", Status: models.ExecutorRunningStatusRunning,
	}))
	commit := &models.SilentRestoreCommit{
		AttemptID: attempt.ID, Checkpoint: launching, CompletedAt: time.Now().UTC(),
		Generation: models.HarnessSessionGeneration{
			SessionID: "session", IncarnationID: "incarnation", Generation: 2, PredecessorGeneration: 1,
			NativeSessionID: "native-conversation", AgentType: "test", CreationReason: models.SilentRestoreAction,
		},
	}
	changed, err = f.repo.CommitSilentRestore(ctx, commit)
	require.NoError(t, err)
	require.True(t, changed, "the fixture must commit a real silent restore transaction")
	storedSession, err := f.repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	f.historicalNotice, _ = models.LoadAgentDeliveryRecovery(storedSession.Metadata)
	require.Equal(t, models.AgentDeliveryRecoveryRestored, f.historicalNotice.Phase)

	for sequence := int64(1); sequence <= 2; sequence++ {
		_, err = f.store.Append(ctx, journal.Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`),
		})
		require.NoError(t, err)
		event := &models.AgentDeliveryEvent{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, TaskID: "task",
			ExecutionID: "execution", OwnerGeneration: 2, ExpectedStreamID: "stream", StreamID: "stream",
			Sequence: sequence, EventType: "usage_update", Payload: []byte(`{}`),
		}
		inserted, err := f.repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
		require.NoError(t, err)
		require.True(t, inserted)
		_, err = f.repo.ProjectAgentDeliveryEvent(ctx, event, nil)
		require.NoError(t, err)
	}
	require.NoError(t, f.store.Acknowledge(ctx, "stream", 2))
	_, err = f.store.PutSubmission(ctx, f.currentSubmission)
	require.NoError(t, err)
	_, err = f.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: f.currentSubmission.ID, SessionID: f.currentSubmission.SessionID, IncarnationID: f.currentSubmission.IncarnationID,
		HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: f.currentSubmission.Hash,
		Payload: f.currentSubmission.Payload, State: models.DeliverySubmissionInterruptedUnknown,
		Outcome: "prompt_dispatch_failed",
	})
	require.NoError(t, err)
	boundBlock := &models.SessionRecoveryBlock{
		ID: "current-bound-block", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 2,
		Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		DeliverySubmissionID: f.currentSubmission.ID, DeliveryStreamID: "stream",
	}
	if scenario == "missing stream fence" {
		boundBlock.DeliveryStreamID = ""
	}
	if scenario == "generic fence" {
		boundBlock = &models.SessionRecoveryBlock{
			ID: "current-generic-block", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 2,
			Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		}
	}
	require.NoError(t, f.repo.UpsertSessionRecoveryBlock(ctx, boundBlock))
	f.currentBlockIDs = append(f.currentBlockIDs, boundBlock.ID)
	if scenario == "matching duplicates after SQL rollback" || scenario == "independent restriction" {
		generic := &models.SessionRecoveryBlock{
			ID: "current-generic-companion", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 2,
			Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		}
		require.NoError(t, f.repo.UpsertSessionRecoveryBlock(ctx, generic))
		if scenario == "independent restriction" {
			independent := &models.SessionRecoveryBlock{
				ID: "independent-restriction", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 2,
				Reason: "office_restriction", State: models.RecoveryBlockOpen, ConsumerReference: "office",
			}
			require.NoError(t, f.repo.UpsertSessionRecoveryBlock(ctx, independent))
		}
	}
	if scenario == "matching duplicates after SQL rollback" || scenario == "independent restriction" {
		f.currentBlockIDs = append(f.currentBlockIDs, "current-generic-companion")
	}

	f.manager = newTestManager(t)
	f.manager.SetAgentDeliveryRepository(f.repo)
	f.execution = &AgentExecution{
		ID: "execution", SessionID: "session", TaskID: "task", ACPSessionID: "native-conversation",
		DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream",
		Status:   v1.AgentStatusReady,
		agentctl: nativeResumeJournalPeer(t, f.store, false),
	}
	f.execution.setSessionInitialized(true)
	require.NoError(t, f.manager.executionStore.Add(f.execution))
	return f
}

func reopenRepeatedNativeResumeStores(t *testing.T, f *repeatedNativeResumeLifecycleFixture) {
	t.Helper()
	require.NoError(t, f.store.Close())
	require.NoError(t, f.repo.DB().Close())
	f.store = openRetentionLifecycleJournal(t, f.journalPath)
	f.repo, _ = openRetentionLifecycleRepository(t, f.databasePath)
	f.manager.SetAgentDeliveryRepository(f.repo)
	f.execution.agentctl = nativeResumeJournalPeer(t, f.store, false)
}

func requireRetiredUnknownHistory(t *testing.T, f *repeatedNativeResumeLifecycleFixture) {
	t.Helper()
	for _, peer := range []journal.Submission{f.oldSubmission, f.currentSubmission} {
		retained, err := f.store.GetSubmission(t.Context(), peer.ID)
		require.NoError(t, err)
		require.True(t, retained.Retired)
		require.Equal(t, peer.State, retained.State)
		require.Equal(t, peer.Payload, retained.Payload)
		sqlSubmission, err := f.repo.GetAgentDeliverySubmission(t.Context(), peer.ID)
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionInterruptedUnknown, sqlSubmission.State)
		require.Equal(t, "prompt_dispatch_failed", sqlSubmission.Outcome)
		require.Zero(t, sqlSubmission.TerminalSequence)
		require.Empty(t, sqlSubmission.TerminalStreamID)
	}
}

func assertRepeatedRecoveryBlocksOpen(t *testing.T, f *repeatedNativeResumeLifecycleFixture, count int) {
	t.Helper()
	blocks, err := f.repo.ListOpenSessionRecoveryBlocks(t.Context(), "session", "incarnation", 2)
	require.NoError(t, err)
	require.Len(t, blocks, count)
	for _, block := range blocks {
		require.Equal(t, models.RecoveryBlockOpen, block.State)
	}
}

func assertRepeatedRecoveryBlocksResolved(t *testing.T, f *repeatedNativeResumeLifecycleFixture) {
	t.Helper()
	blocks, err := f.repo.ListOpenSessionRecoveryBlocks(t.Context(), "session", "incarnation", 2)
	require.NoError(t, err)
	require.Empty(t, blocks)
	for _, id := range f.currentBlockIDs {
		block, err := f.repo.GetSessionRecoveryBlock(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockResolved, block.State)
		require.Equal(t, "resume", block.AuthorizedAction)
	}
}

func assertRepeatedHistoricalNotice(t *testing.T, f *repeatedNativeResumeLifecycleFixture) {
	t.Helper()
	session, err := f.repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, exists)
	require.Equal(t, f.historicalNotice, recovery)
}
