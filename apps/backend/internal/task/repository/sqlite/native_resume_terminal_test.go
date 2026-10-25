package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeTerminalProof(t *testing.T) {
	testNativeResumeTerminalProof(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeTerminalProof(t *testing.T) {
	testNativeResumeTerminalProof(t, func(t *testing.T) *Repository {
		db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(db, db, nil)
		require.NoError(t, err)
		return repo
	})
}

func testNativeResumeTerminalProof(t *testing.T, open func(*testing.T) *Repository) {
	for _, scenario := range []string{"generic fence", "bound fence", "changed hash", "changed outcome", "changed state", "changed sequence", "unprojected", "independent active"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			repo := open(t)
			f := newSilentRestoreFixture(t, repo, "terminal", models.TaskSessionStateWaitingForInput)
			row, err := repo.GetAgentDeliverySubmission(ctx, f.recovery.SubmissionID)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions SET state = 'failed', outcome = 'failed', terminal_stream_id = ?, terminal_sequence = 1 WHERE id = ?`), f.recovery.StreamID, row.ID)
			require.NoError(t, err)
			require.NoError(t, repo.SetSessionMetadataKey(ctx, f.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
			if scenario != "bound fence" {
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET reason = 'unresolved_durable_work', delivery_submission_id = '', delivery_stream_id = '' WHERE id = ?`), f.block.ID)
				require.NoError(t, err)
			}
			event := &models.AgentDeliveryEvent{StreamID: f.recovery.StreamID, SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: 1, Sequence: 1, SubmissionID: row.ID, EventType: "error", Terminal: true, Payload: []byte(`{}`)}
			_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
			require.NoError(t, err)
			if scenario != "unprojected" {
				_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
				require.NoError(t, err)
			}
			cursor, err := repo.GetAgentDeliveryCursor(ctx, f.recovery.StreamID)
			require.NoError(t, err)
			proof := &models.NativeResumeDeliveryEvidence{SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: 1, NativeSessionID: f.generation.NativeSessionID, ExecutionID: f.recovery.AgentExecutionID, StreamID: f.recovery.StreamID, Cursor: cursor,
				Submissions: []models.NativeResumeSubmissionEvidence{{ID: row.ID, HarnessGeneration: 1, PayloadHash: row.PayloadHash, StreamID: f.recovery.StreamID, TerminalStreamID: f.recovery.StreamID, TerminalSequence: 1, State: models.DeliverySubmissionFailed, Outcome: "failed"}}}
			switch scenario {
			case "changed hash":
				proof.Submissions[0].PayloadHash = "foreign"
			case "changed outcome":
				proof.Submissions[0].Outcome = "completed"
			case "changed state":
				proof.Submissions[0].State = models.DeliverySubmissionCompleted
			case "changed sequence":
				proof.Submissions[0].TerminalSequence = 2
			case "independent active":
				active := *row
				active.ID, active.State = "independent", models.DeliverySubmissionAccepted
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &active)
				require.NoError(t, err)
			}
			resolved, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, f.block.ID, proof, time.Now().UTC())
			require.NoError(t, err)
			success := scenario == "generic fence" || scenario == "bound fence"
			require.Equal(t, success, resolved)
			block, err := repo.GetSessionRecoveryBlock(ctx, f.block.ID)
			require.NoError(t, err)
			require.Equal(t, success, block.State == models.RecoveryBlockResolved)
			retained, err := repo.GetAgentDeliverySubmission(ctx, row.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionFailed, retained.State)
			require.Equal(t, "failed", retained.Outcome)
			require.Equal(t, row.Payload, retained.Payload)
		})
	}
}
