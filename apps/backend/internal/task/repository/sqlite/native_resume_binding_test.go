package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

type nativeEvidenceResolver interface {
	ResolveSessionRecoveryBlockWithNativeEvidence(context.Context, string, *models.NativeResumeDeliveryEvidence, time.Time) (bool, error)
}

func TestNativeResumeAuthenticatedMissingBinding(t *testing.T) {
	testNativeResumeAuthenticatedMissingBinding(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeAuthenticatedMissingBinding(t *testing.T) {
	testNativeResumeAuthenticatedMissingBinding(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func testNativeResumeAuthenticatedMissingBinding(t *testing.T, open func(*testing.T) *Repository) {
	for _, scenario := range []string{"absent notice", "partial notice", "resolved earlier notice", "matching fences", "independent fence", "foreign native", "hash changed", "active work", "unmatched unknown", "foreign stream", "foreign submission stream", "missing submission stream", "reconstructed unknown", "known outcome", "prepared attempt", "independent attempt", "legacy error", "independent error", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			repo := open(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "bound-native", models.TaskSessionStateWaitingForInput)
			submission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
			require.NoError(t, err)
			metadata := map[string]any{"acp": map[string]any{"session_id": fixture.generation.NativeSessionID}}
			if scenario == "resolved earlier notice" {
				changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
				require.NoError(t, err)
				require.True(t, changed)
				previousSession, err := repo.GetTaskSession(ctx, fixture.session.ID)
				require.NoError(t, err)
				previous, exists := models.LoadAgentDeliveryRecovery(previousSession.Metadata)
				require.True(t, exists)
				metadata[models.SessionMetaKeyAgentDeliveryRecovery] = previous
				older := *submission
				older.ID = "new-current-submission"
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &older)
				require.NoError(t, err)
				fixture.block.ID = "current-block"
				fixture.block.State = models.RecoveryBlockOpen
				fixture.block.ResolvedAt = nil
				fixture.block.AuthorizedAction = ""
				fixture.block.DeliverySubmissionID = older.ID
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &fixture.block))
			}
			if scenario == "matching fences" || scenario == "independent fence" {
				companion := fixture.block
				companion.ID = "companion"
				companion.Reason = "unresolved_durable_work"
				companion.DeliverySubmissionID, companion.DeliveryStreamID = "", ""
				if scenario == "independent fence" {
					companion.ConsumerReference = "office"
				}
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &companion))
			}

			if scenario == "partial notice" {
				partial := fixture.recovery
				partial.StreamID = ""
				metadata[models.SessionMetaKeyAgentDeliveryRecovery] = partial
			}
			if scenario == "legacy error" || scenario == "independent error" {
				details := submission.ID
				if scenario == "independent error" {
					details = "independent"
				}
				metadata[models.SessionMetaKeyLastAgentError] = models.LastAgentError{Code: durableDeliveryUncertainErrorCode, Message: "uncertain", Details: details, AgentExecutionID: "original"}
			}
			raw, err := json.Marshal(metadata)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET metadata = ? WHERE id = ?`), string(raw), fixture.session.ID)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`), fixture.block.ID)
			require.NoError(t, err)
			proof := &models.NativeResumeDeliveryEvidence{SessionID: fixture.session.ID, IncarnationID: fixture.recovery.IncarnationID,
				HarnessGeneration: 1, NativeSessionID: fixture.generation.NativeSessionID, StreamID: fixture.recovery.StreamID,
				ExecutionID: fixture.recovery.AgentExecutionID,
				Submissions: []models.NativeResumeSubmissionEvidence{{ID: submission.ID, HarnessGeneration: 1, PayloadHash: submission.PayloadHash, StreamID: fixture.recovery.StreamID}}}
			if scenario == "resolved earlier notice" {
				current, err := repo.GetAgentDeliverySubmission(ctx, fixture.block.DeliverySubmissionID)
				require.NoError(t, err)
				proof.Submissions = append(proof.Submissions, models.NativeResumeSubmissionEvidence{ID: current.ID, HarnessGeneration: 1, PayloadHash: current.PayloadHash, StreamID: proof.StreamID})
			}
			switch scenario {
			case "reconstructed unknown", "known outcome":
				outcome := "unknown"
				if scenario == "known outcome" {
					outcome = "completed"
				}
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_outcome = ? WHERE id = ?`), outcome, fixture.block.ID)
				require.NoError(t, err)
			case "foreign submission stream":
				proof.Submissions[0].StreamID = "foreign"
			case "missing submission stream":
				proof.Submissions[0].StreamID = ""
			case "prepared attempt", "independent attempt":
				attempt := fixture.attempt("pending-native-attempt")
				attempt.Authorized = true
				attempt.Checkpoint.SourceRecovery.AgentExecutionID = "original-owner"
				if scenario == "independent attempt" {
					attempt.Checkpoint.Stage = models.SilentRestoreStageCandidateLaunching
					attempt.Checkpoint.CandidateExecutionID = "independent"
				}
				require.NoError(t, repo.CreateRestoreAttempt(ctx, attempt))
			case "foreign native":
				proof.NativeSessionID = "foreign"
			case "hash changed":
				proof.Submissions[0].PayloadHash = "changed"
			case "foreign stream":
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_stream_id = 'foreign-stream' WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			case "active work", "unmatched unknown":
				state := models.DeliverySubmissionAccepted
				if scenario == "unmatched unknown" {
					state = models.DeliverySubmissionInterruptedUnknown
				}
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "independent", SessionID: proof.SessionID, IncarnationID: proof.IncarnationID,
					HarnessGeneration: 1, OwnerGeneration: 1, State: state, PayloadHash: "independent", Payload: []byte("independent")})
				require.NoError(t, err)
			case "rollback":
				if repo.db.DriverName() == "sqlite3" {
					_, err = repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_bound_resume BEFORE UPDATE ON session_recovery_blocks WHEN NEW.state = 'resolved' BEGIN SELECT RAISE(ABORT, 'reject resolution'); END`)
					require.NoError(t, err)
				} else {
					t.Skip("SQLite write-boundary injection; PostgreSQL transactional controls have separate coverage")
				}
			}
			resolver, ok := any(repo).(nativeEvidenceResolver)
			require.True(t, ok, "authenticated retirement must support guarded missing binding recovery")
			changed, err := resolver.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, proof, time.Now().UTC())
			if scenario == "rollback" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			want := scenario == "legacy error" || scenario == "independent error" || scenario == "prepared attempt" || scenario == "reconstructed unknown" || scenario == "absent notice" || scenario == "partial notice" || scenario == "resolved earlier notice" || scenario == "matching fences"
			require.Equal(t, want, changed)
			if scenario == "prepared attempt" || scenario == "independent attempt" {
				attempt, err := repo.GetRestoreAttempt(ctx, "pending-native-attempt")
				require.NoError(t, err)
				require.Equal(t, scenario == "prepared attempt", attempt.CompletedAt != nil)
			}
			block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
			require.NoError(t, err)
			if want {
				require.Equal(t, models.RecoveryBlockResolved, block.State)
				require.Equal(t, proof.StreamID, block.DeliveryStreamID)
				if scenario == "matching fences" {
					companion, err := repo.GetSessionRecoveryBlock(ctx, "companion")
					require.NoError(t, err)
					require.Equal(t, models.RecoveryBlockResolved, companion.State)
				}
			} else {
				require.Equal(t, models.RecoveryBlockOpen, block.State)
				if scenario != "foreign stream" {
					require.Empty(t, block.DeliveryStreamID)
				}
			}
			if scenario == "legacy error" || scenario == "independent error" {
				current, err := repo.GetTaskSession(ctx, proof.SessionID)
				require.NoError(t, err)
				require.Equal(t, scenario == "legacy error", current.Metadata[models.SessionMetaKeyLastAgentError] == nil)
			}
			stored, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, err)
			require.Equal(t, submission.State, stored.State)
			require.Equal(t, submission.PayloadHash, stored.PayloadHash)
			require.Zero(t, stored.TerminalSequence)
		})
	}
}
