package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeHistoricalSilentRestoreProof(t *testing.T) {
	testNativeResumeHistoricalSilentRestoreProof(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeHistoricalSilentRestoreProof(t *testing.T) {
	testNativeResumeHistoricalSilentRestoreProof(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func testNativeResumeHistoricalSilentRestoreProof(t *testing.T, open func(*testing.T) *Repository) {
	for _, legacy := range []bool{false, true} {
		name := "exact"
		if legacy {
			name = "legacy empty stream"
		}
		t.Run(name, func(t *testing.T) {
			repo := open(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "historical-proof-"+name, models.TaskSessionStateWaitingForInput)
			commitSilentRestoreFixture(t, repo, fixture, "attempt-candidate-"+name, "candidate-"+name)
			if legacy {
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(
					`UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			}
			session, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
			require.True(t, exists)
			require.Equal(t, models.AgentDeliveryRecoveryRestored, recovery.Phase)

			resolved, err := repo.HasResolvedHistoricalNativeResumeRecovery(ctx, recovery)
			require.NoError(t, err)
			require.True(t, resolved, "silent restore is valid historical native recovery proof")
			strict, err := repo.HasResolvedNativeResumeRecovery(ctx, recovery)
			require.NoError(t, err)
			require.False(t, strict, "historical silent restore does not authorize current explicit Resume verification")
		})
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
func TestNativeResumeAfterSilentRestore(t *testing.T) {
	testNativeResumeAfterSilentRestore(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeAfterSilentRestore(t *testing.T) {
	testNativeResumeAfterSilentRestore(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func testNativeResumeAfterSilentRestore(t *testing.T, open func(*testing.T) *Repository) {
	for _, scenario := range []string{"missing stream", "generic fallback", "matching fences", "contradictory historical stream", "independent fence", "changed native", "broken lineage", "unsupported action", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			repo := open(t)
			ctx := context.Background()
			fixture, proof := prepareRepeatedNativeResume(t, repo, "auth-"+scenario)
			if scenario == "generic fallback" {
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.currentBlock.ID)
				require.NoError(t, err)
				generic := fixture.currentBlock
				generic.Reason = "unresolved_durable_work"
				generic.DeliverySubmissionID, generic.DeliveryStreamID = "", ""
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &generic))
				fixture.currentBlock = generic
			}
			if scenario == "matching fences" || scenario == "contradictory historical stream" || scenario == "independent fence" {
				companion := fixture.currentBlock
				companion.ID += "-companion"
				companion.DeliverySubmissionID = ""
				companion.DeliveryStreamID = ""
				companion.Reason = "unresolved_durable_work"
				if scenario == "independent fence" {
					companion.ConsumerReference = "office"
					companion.Reason = "office_restriction"
				}
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &companion))
			}
			switch scenario {
			case "contradictory historical stream":
				found := false
				for i := range proof.Submissions {
					if proof.Submissions[i].ID == fixture.historicalNotice.SubmissionID {
						proof.Submissions[i].StreamID += "-foreign"
						found = true
						break
					}
				}
				require.True(t, found, "the retained historical submission is present in the proof")
			case "changed native":
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE harness_session_generations
					SET native_session_id = 'replacement-native' WHERE session_id = ? AND generation = ?`),
					fixture.base.session.ID, proof.HarnessGeneration)
				require.NoError(t, err)
				proof.NativeSessionID = "replacement-native"
			case "broken lineage":
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE harness_session_generations
					SET predecessor_generation = 0 WHERE session_id = ? AND generation = ?`),
					fixture.base.session.ID, proof.HarnessGeneration)
				require.NoError(t, err)
			case "unsupported action":
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks
					SET authorized_action = 'continue_from_history' WHERE id = ?`), fixture.base.block.ID)
				require.NoError(t, err)
			case "rollback":
				if repo.db.DriverName() != "sqlite3" {
					t.Skip("SQLite trigger injects the transaction rollback")
				}
				_, err := repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_silent_restore_resume BEFORE UPDATE ON session_recovery_blocks
					WHEN NEW.state = 'resolved' BEGIN SELECT RAISE(ABORT, 'resolution refused'); END`)
				require.NoError(t, err)
			}
			var changed bool
			var err error
			if scenario == "generic fallback" {
				changed, err = repo.ResolveSessionRecoveryBlock(ctx, fixture.currentBlock.ID, nativeResumeRecoveryAction, fixture.completedAt)
			} else {
				changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.currentBlock.ID, proof, fixture.completedAt)
			}
			if scenario == "rollback" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			want := scenario == "missing stream" || scenario == "generic fallback" || scenario == "matching fences"
			require.Equal(t, want, changed)
			block, err := repo.GetSessionRecoveryBlock(ctx, fixture.currentBlock.ID)
			require.NoError(t, err)
			if want {
				require.Equal(t, models.RecoveryBlockResolved, block.State)
				if scenario == "missing stream" {
					require.Equal(t, proof.StreamID, block.DeliveryStreamID)
				}
			} else {
				require.Equal(t, models.RecoveryBlockOpen, block.State)
				require.Empty(t, block.DeliveryStreamID)
			}
			if scenario == "matching fences" || scenario == "contradictory historical stream" || scenario == "independent fence" {
				companion, err := repo.GetSessionRecoveryBlock(ctx, fixture.currentBlock.ID+"-companion")
				require.NoError(t, err)
				require.Equal(t, want, companion.State == models.RecoveryBlockResolved)
			}
			session, err := repo.GetTaskSession(ctx, fixture.base.session.ID)
			require.NoError(t, err)
			recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
			require.True(t, exists)
			require.Equal(t, fixture.historicalNotice, recovery, "historical restore metadata remains unchanged")
			if want && scenario != "generic fallback" {
				require.Equal(t, models.RecoveryNoticeSourceIdentity(models.AgentDeliveryRecovery{
					SessionID: fixture.base.session.ID, IncarnationID: fixture.base.session.QueueIncarnationID,
					HarnessGeneration: fixture.currentSubmission.HarnessGeneration,
					SubmissionID:      fixture.currentSubmission.ID,
				}), session.Metadata[models.SessionMetaKeyRecoveryNoticePending],
					"later incident must have its own notice marker after explicit Resume")
			} else {
				require.NotContains(t, session.Metadata, models.SessionMetaKeyRecoveryNoticePending,
					"failed or non-native recovery must not create a notice marker")
			}
			oldSubmission, err := repo.GetAgentDeliverySubmission(ctx, fixture.base.recovery.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, oldSubmission.State)
			currentSubmission, err := repo.GetAgentDeliverySubmission(ctx, fixture.currentSubmission.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, currentSubmission.State)
		})
	}
}

type repeatedNativeResumeFixture struct {
	base              *silentRestoreFixture
	currentBlock      models.SessionRecoveryBlock
	currentSubmission models.AgentDeliverySubmission
	historicalNotice  models.AgentDeliveryRecovery
	completedAt       time.Time
}

func prepareRepeatedNativeResume(t *testing.T, repo *Repository, suffix string) (*repeatedNativeResumeFixture, *models.NativeResumeDeliveryEvidence) {
	t.Helper()
	ctx := context.Background()
	base := newSilentRestoreFixture(t, repo, suffix, models.TaskSessionStateWaitingForInput)
	candidateID := "candidate-" + suffix
	commitSilentRestoreFixture(t, repo, base, "attempt-candidate-"+suffix, candidateID)
	session, err := repo.GetTaskSession(ctx, base.session.ID)
	require.NoError(t, err)
	historicalNotice, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, exists)
	pending, ok := session.Metadata[models.SessionMetaKeyRecoveryNoticePending].(string)
	require.True(t, ok, "the initial restoration must leave its notice pending")
	cleared, err := repo.ClearSessionMetadataKeyIfStringValue(ctx, session.ID,
		models.SessionMetaKeyRecoveryNoticePending, pending)
	require.NoError(t, err)
	require.True(t, cleared, "the earlier notice has been published before the later incident")
	currentGeneration, err := repo.GetCurrentHarnessSessionGeneration(ctx, base.session.ID, base.session.QueueIncarnationID)
	require.NoError(t, err)
	currentID := "current-submission-" + suffix
	currentStream := "current-stream-" + suffix
	currentSubmission := models.AgentDeliverySubmission{
		ID: currentID, SessionID: base.session.ID, IncarnationID: base.session.QueueIncarnationID,
		HarnessGeneration: currentGeneration.Generation, OwnerGeneration: currentGeneration.Generation,
		PayloadHash: "current-hash-" + suffix, Payload: []byte("distinct current prompt"),
		State: models.DeliverySubmissionInterruptedUnknown,
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &currentSubmission)
	require.NoError(t, err)
	currentBlock := models.SessionRecoveryBlock{
		ID: "current-block-" + suffix, SessionID: base.session.ID, IncarnationID: base.session.QueueIncarnationID,
		ExpectedGeneration: currentGeneration.Generation, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: currentSubmission.ID,
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &currentBlock))
	oldSubmission, err := repo.GetAgentDeliverySubmission(ctx, base.recovery.SubmissionID)
	require.NoError(t, err)
	proof := &models.NativeResumeDeliveryEvidence{
		SessionID: base.session.ID, IncarnationID: base.session.QueueIncarnationID,
		HarnessGeneration: currentGeneration.Generation, NativeSessionID: currentGeneration.NativeSessionID,
		ExecutionID: candidateID, StreamID: currentStream,
		Submissions: []models.NativeResumeSubmissionEvidence{
			{ID: oldSubmission.ID, HarnessGeneration: oldSubmission.HarnessGeneration, PayloadHash: oldSubmission.PayloadHash, StreamID: base.recovery.StreamID},
			{ID: currentSubmission.ID, State: currentSubmission.State, HarnessGeneration: currentSubmission.HarnessGeneration,
				PayloadHash: currentSubmission.PayloadHash, StreamID: currentStream},
		},
	}
	return &repeatedNativeResumeFixture{
		base: base, currentBlock: currentBlock, currentSubmission: currentSubmission,
		historicalNotice: historicalNotice, completedAt: time.Now().UTC(),
	}, proof
}
