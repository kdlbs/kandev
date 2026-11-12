package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.9
func TestHistoricalTerminalRecovery(t *testing.T) {
	testHistoricalTerminalRecovery(t, newRepoForSessionTests)
}

func TestPostgresHistoricalTerminalRecovery(t *testing.T) {
	testHistoricalTerminalRecovery(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func testHistoricalTerminalRecovery(t *testing.T, open func(*testing.T) *Repository) {
	t.Helper()
	cases := []struct {
		name                 string
		staleNotice          bool
		staleAttempt         bool
		completeTerminal     bool
		allocatedAttempt     bool
		changeAttemptNative  bool
		omitTerminalEvidence bool
		changeTerminalHash   bool
		independentWork      bool
		rollback             bool
		want                 bool
	}{
		{name: "clean control", want: true},
		{name: "stale terminal notice", staleNotice: true, completeTerminal: true, want: true},
		{name: "stale prepared attempt", staleAttempt: true, completeTerminal: true, want: true},
		{name: "stale notice and attempt", staleNotice: true, staleAttempt: true, completeTerminal: true, want: true},
		{name: "terminal completion races resume", staleNotice: true, completeTerminal: false, want: true},
		{name: "missing terminal evidence", staleNotice: true, completeTerminal: true, omitTerminalEvidence: true},
		{name: "changed terminal payload", staleNotice: true, completeTerminal: true, changeTerminalHash: true},
		{name: "allocated candidate remains a guard", staleAttempt: true, completeTerminal: true, allocatedAttempt: true},
		{name: "changed attempt native identity remains a guard", staleAttempt: true, completeTerminal: true, changeAttemptNative: true},
		{name: "independent live work remains a guard", staleNotice: true, completeTerminal: true, independentWork: true},
		{name: "rollback preserves stale attempt", staleNotice: true, staleAttempt: true, completeTerminal: true, rollback: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			repo := open(t)
			ctx := t.Context()
			if test.name == "clean control" {
				fixture := newSilentRestoreFixture(t, repo, "terminal-clean", models.TaskSessionStateWaitingForInput)
				proof := nativeResumeProofForFixture(t, repo, fixture, fixture.recovery.AgentExecutionID)
				changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, proof, time.Now().UTC())
				require.NoError(t, err)
				require.True(t, changed)
				return
			}

			fixture := newSilentRestoreFixture(t, repo, "terminal-"+test.name, models.TaskSessionStateWaitingForInput)
			var attempt *models.RestoreAttempt
			terminal := settleHistoricalTerminalSource(t, repo, fixture, test.completeTerminal)
			current := createHistoricalCurrentSubmission(t, repo, fixture, "current-"+test.name)
			if test.staleAttempt && !test.allocatedAttempt {
				attempt = fixture.attempt("attempt-historical-" + test.name)
				attempt.Authorized = true
				if test.changeAttemptNative {
					attempt.Checkpoint.SourceGeneration.NativeSessionID = "foreign-native-conversation"
				}
				require.NoError(t, repo.CreateRestoreAttempt(ctx, attempt))
			}
			if test.staleNotice {
				notice := fixture.recovery
				notice.Phase = models.AgentDeliveryRecoverySettled
				require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, notice))
			} else {
				require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
			}
			if test.staleAttempt && test.allocatedAttempt {
				attempt = fixture.attempt("attempt-allocated-" + test.name)
				attempt.Authorized = true
				attempt.Reason = "provider_unavailable"
				attempt.Checkpoint.Stage = models.SilentRestoreStageCandidateLaunching
				attempt.Checkpoint.CandidateExecutionID = "allocated-candidate"
				require.NoError(t, repo.CreateRestoreAttempt(ctx, attempt))
				storedAttempt, err := repo.GetRestoreAttempt(ctx, attempt.ID)
				require.NoError(t, err)
				require.Equal(t, attempt.Checkpoint.CandidateExecutionID, storedAttempt.Checkpoint.CandidateExecutionID)
			}

			proof := nativeResumeProofForHistoricalSource(t, repo, fixture, terminal, current, "current-execution-"+current.ID)
			if test.omitTerminalEvidence {
				proof.Submissions = proof.Submissions[1:]
			}
			if test.changeTerminalHash {
				proof.Submissions[0].PayloadHash = "changed-terminal-payload"
			}
			if test.independentWork {
				currentSubmission, err := repo.GetAgentDeliverySubmission(ctx, current.ID)
				require.NoError(t, err)
				independent := *currentSubmission
				independent.ID = "independent-" + test.name
				independent.PayloadHash = "independent-payload-" + test.name
				independent.Payload = []byte("independent live prompt")
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &independent)
				require.NoError(t, err)
				block := models.SessionRecoveryBlock{
					ID: "independent-block-" + test.name, SessionID: fixture.session.ID,
					IncarnationID: fixture.session.QueueIncarnationID, ExpectedGeneration: 1,
					Reason: deliveryUnknownPromptReason, State: models.RecoveryBlockOpen,
					ConsumerReference: reconstructionDeliveryConsumer, DeliverySubmissionID: independent.ID,
				}
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &block))
			}
			if test.rollback {
				installNativeResumeRollbackTrigger(t, repo)
			}

			changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, current.Block.ID, proof, time.Now().UTC())
			if test.rollback {
				require.Error(t, err)
				changed = false
			} else {
				require.NoError(t, err)
			}
			if test.name == "terminal completion races resume" {
				require.False(t, changed, "an unsettled source cannot retire historical bookkeeping")
				putRetentionWorkflowOutcome(t, repo, terminal, "not_required", "historical-terminal-turn")
				completed, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx, terminalRetentionEffectKey(terminal), time.Now().UTC())
				require.NoError(t, err)
				require.True(t, completed)
				proof = nativeResumeProofForHistoricalSource(t, repo, fixture, terminal, current, "current-execution-"+current.ID)
				changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, current.Block.ID, proof, time.Now().UTC())
				require.NoError(t, err)
			}
			require.Equal(t, test.want, changed)
			if test.want && test.staleNotice {
				session, err := repo.GetTaskSession(ctx, fixture.session.ID)
				require.NoError(t, err)
				restored, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
				require.True(t, exists)
				require.Equal(t, fixture.recovery.AgentExecutionID, restored.AgentExecutionID,
					"later recovery retains the interrupted prompt's historical execution owner")
				require.NotEqual(t, proof.ExecutionID, restored.AgentExecutionID)
			}

			currentBlock, err := repo.GetSessionRecoveryBlock(ctx, current.Block.ID)
			require.NoError(t, err)
			require.Equal(t, test.want, currentBlock.State == models.RecoveryBlockResolved)
			oldBlock, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockResolved, oldBlock.State)
			oldSubmission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionFailed, oldSubmission.State)
			require.Equal(t, "failed", oldSubmission.Outcome)
			require.Equal(t, []byte("old immutable prompt"), oldSubmission.Payload)
			currentSubmission, err := repo.GetAgentDeliverySubmission(ctx, current.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, currentSubmission.State)
			if attempt != nil {
				stored, err := repo.GetRestoreAttempt(ctx, attempt.ID)
				require.NoError(t, err)
				if test.want {
					require.Equal(t, "blocked", stored.Outcome)
					require.Equal(t, models.SilentRestoreStageTerminal, stored.Checkpoint.Stage)
					require.Equal(t, "source_terminal_settled", stored.Reason)
					require.NotNil(t, stored.CompletedAt)
				} else {
					require.Equal(t, models.SilentRestoreOutcomePending, stored.Outcome)
					require.Nil(t, stored.CompletedAt)
				}
			}
		})
	}

	t.Run("terminal settlement retires only its prepared attempt", func(t *testing.T) {
		repo := open(t)
		fixture := newSilentRestoreFixture(t, repo, "terminal-settlement-attempt", models.TaskSessionStateWaitingForInput)
		attempt := createHistoricalTerminalAttempt(t, repo, fixture, "attempt-settlement-history", "original failure")
		terminal := settleHistoricalTerminalSource(t, repo, fixture, false)
		putRetentionWorkflowOutcome(t, repo, terminal, "not_required", "historical-terminal-turn")
		completed, err := repo.CompleteAgentDeliveryTerminalSettlement(t.Context(), terminalRetentionEffectKey(terminal), time.Now().UTC())
		require.NoError(t, err)
		require.True(t, completed)
		stored, err := repo.GetRestoreAttempt(t.Context(), attempt.ID)
		require.NoError(t, err)
		require.Equal(t, "blocked", stored.Outcome)
		require.Equal(t, models.SilentRestoreStageTerminal, stored.Checkpoint.Stage)
		require.Equal(t, "original failure", stored.Reason, "terminal settlement preserves an existing failure reason")
		require.NotNil(t, stored.CompletedAt)
	})

	t.Run("settled notice for the current unknown prompt remains blocked", func(t *testing.T) {
		repo := open(t)
		fixture := newSilentRestoreFixture(t, repo, "terminal-current-unknown", models.TaskSessionStateWaitingForInput)
		notice := fixture.recovery
		notice.Phase = models.AgentDeliveryRecoverySettled
		require.NoError(t, repo.SetSessionMetadataKey(t.Context(), fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, notice))
		proof := nativeResumeProofForFixture(t, repo, fixture, fixture.recovery.AgentExecutionID)
		changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), fixture.block.ID, proof, time.Now().UTC())
		require.NoError(t, err)
		require.False(t, changed)
		block, err := repo.GetSessionRecoveryBlock(t.Context(), fixture.block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, block.State)
	})
}

type historicalCurrentSubmission struct {
	ID    string
	Block models.SessionRecoveryBlock
}

func createHistoricalTerminalAttempt(t *testing.T, repo *Repository, fixture *silentRestoreFixture, id, reason string) *models.RestoreAttempt {
	t.Helper()
	attempt := fixture.attempt(id)
	attempt.Authorized = true
	attempt.Reason = reason
	require.NoError(t, repo.CreateRestoreAttempt(t.Context(), attempt))
	return attempt
}

func settleHistoricalTerminalSource(t *testing.T, repo *Repository, fixture *silentRestoreFixture, complete bool) models.AgentDeliveryEvent {
	t.Helper()
	ctx := t.Context()
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions SET state = ? WHERE id = ?`), models.DeliverySubmissionDispatching, fixture.recovery.SubmissionID)
	require.NoError(t, err)
	event := models.AgentDeliveryEvent{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID, HarnessGeneration: fixture.generation.Generation,
		StreamID: fixture.recovery.StreamID, Sequence: 1, SubmissionID: fixture.recovery.SubmissionID,
		EventType: "error", Payload: []byte(`{"turn_id":"historical-terminal-turn"}`), Terminal: true,
	}
	projectDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionFailed, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, settled)
	if complete {
		putRetentionWorkflowOutcome(t, repo, event, "not_required", "historical-terminal-turn")
		completed, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx, terminalRetentionEffectKey(event), time.Now().UTC())
		require.NoError(t, err)
		require.True(t, completed)
	}
	return event
}

func createHistoricalCurrentSubmission(t *testing.T, repo *Repository, fixture *silentRestoreFixture, id string) historicalCurrentSubmission {
	t.Helper()
	ctx := t.Context()
	submission := models.AgentDeliverySubmission{
		ID: id, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: fixture.generation.Generation, OwnerGeneration: fixture.generation.Generation,
		PayloadHash: "hash-" + id, Payload: []byte("distinct current prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	}
	_, err := repo.PrepareAgentDeliverySubmission(ctx, &submission)
	require.NoError(t, err)
	block := models.SessionRecoveryBlock{
		ID: "block-" + id, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		ExpectedGeneration: fixture.generation.Generation, Reason: deliveryUnknownPromptReason, State: models.RecoveryBlockOpen,
		ConsumerReference: reconstructionDeliveryConsumer, DeliverySubmissionID: submission.ID,
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &block))
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE executors_running SET agent_execution_id = ? WHERE session_id = ?`), "current-execution-"+id, fixture.session.ID)
	require.NoError(t, err)
	return historicalCurrentSubmission{ID: submission.ID, Block: block}
}

func nativeResumeProofForFixture(t *testing.T, repo *Repository, fixture *silentRestoreFixture, executionID string) *models.NativeResumeDeliveryEvidence {
	t.Helper()
	row, err := repo.GetAgentDeliverySubmission(t.Context(), fixture.recovery.SubmissionID)
	require.NoError(t, err)
	return &models.NativeResumeDeliveryEvidence{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: fixture.generation.Generation, NativeSessionID: fixture.generation.NativeSessionID,
		ExecutionID: executionID, StreamID: fixture.recovery.StreamID,
		Submissions: []models.NativeResumeSubmissionEvidence{{
			ID: row.ID, State: row.State, Outcome: row.Outcome, HarnessGeneration: row.HarnessGeneration,
			PayloadHash: row.PayloadHash, StreamID: fixture.recovery.StreamID,
		}},
	}
}

func nativeResumeProofForHistoricalSource(
	t *testing.T,
	repo *Repository,
	fixture *silentRestoreFixture,
	terminal models.AgentDeliveryEvent,
	current historicalCurrentSubmission,
	executionID string,
) *models.NativeResumeDeliveryEvidence {
	t.Helper()
	ctx := t.Context()
	old, err := repo.GetAgentDeliverySubmission(ctx, terminal.SubmissionID)
	require.NoError(t, err)
	latest, err := repo.GetAgentDeliverySubmission(ctx, current.ID)
	require.NoError(t, err)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, terminal.StreamID)
	require.NoError(t, err)
	return &models.NativeResumeDeliveryEvidence{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: fixture.generation.Generation, NativeSessionID: fixture.generation.NativeSessionID,
		ExecutionID: executionID, StreamID: terminal.StreamID, Cursor: cursor,
		Submissions: []models.NativeResumeSubmissionEvidence{
			{ID: old.ID, State: old.State, Outcome: old.Outcome, HarnessGeneration: old.HarnessGeneration,
				PayloadHash: old.PayloadHash, StreamID: terminal.StreamID, TerminalStreamID: old.TerminalStreamID, TerminalSequence: old.TerminalSequence},
			{ID: latest.ID, State: latest.State, Outcome: latest.Outcome, HarnessGeneration: latest.HarnessGeneration,
				PayloadHash: latest.PayloadHash, StreamID: terminal.StreamID},
		},
	}
}

func installNativeResumeRollbackTrigger(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	if repo.db.DriverName() == "sqlite3" {
		_, err := repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_historical_native_resume BEFORE UPDATE ON session_recovery_blocks
			WHEN NEW.state = 'resolved' BEGIN SELECT RAISE(ABORT, 'historical native resume refused'); END`)
		require.NoError(t, err)
		return
	}
	_, err := repo.DB().ExecContext(ctx, `CREATE FUNCTION reject_historical_native_resume() RETURNS trigger AS $$
		BEGIN
			IF NEW.state = 'resolved' THEN
				RAISE EXCEPTION 'historical native resume refused';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_historical_native_resume BEFORE UPDATE ON session_recovery_blocks
		FOR EACH ROW EXECUTE FUNCTION reject_historical_native_resume()`)
	require.NoError(t, err)
}
