package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNativeResumePendingTerminalProof(t *testing.T) {
	testNativeResumePendingTerminalProof(t, newRepoForSessionTests)
}

func TestPostgresNativeResumePendingTerminalProof(t *testing.T) {
	testNativeResumePendingTerminalProof(t, func(t *testing.T) *Repository {
		db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(db, db, nil)
		require.NoError(t, err)
		return repo
	})
}

func pendingTerminalProof(t *testing.T, repo *Repository) (*silentRestoreFixture, *models.NativeResumeDeliveryEvidence) {
	t.Helper()
	ctx := t.Context()
	f := newSilentRestoreFixture(t, repo, "pending-terminal", models.TaskSessionStateWaitingForInput)
	row, err := repo.GetAgentDeliverySubmission(ctx, f.recovery.SubmissionID)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{StreamID: f.recovery.StreamID, SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: 1, Sequence: 1, SubmissionID: row.ID, EventType: "error", Terminal: true, Payload: []byte(`{"turn_id":"turn"}`)}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, 1, models.DeliverySubmissionFailed, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, settled)
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET state = 'open', resolved_at = NULL, delivery_sequence = 0, delivery_turn_id = '', delivery_outcome = '', authorized_action = '' WHERE id = ?`), f.block.ID)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, f.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
	cursor, err := repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	require.NoError(t, err)
	return f, &models.NativeResumeDeliveryEvidence{SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: 1, NativeSessionID: f.generation.NativeSessionID, ExecutionID: f.recovery.AgentExecutionID, StreamID: event.StreamID, Cursor: cursor,
		Submissions: []models.NativeResumeSubmissionEvidence{{ID: row.ID, HarnessGeneration: 1, PayloadHash: row.PayloadHash, StreamID: event.StreamID, TerminalStreamID: event.StreamID, TerminalSequence: 1, State: models.DeliverySubmissionFailed, Outcome: "failed"}}}
}

// The proof was assembled before these changes. Admission must re-read SQL
// under its locks, and never release a fence on stale terminal evidence.
func testNativeResumePendingTerminalProof(t *testing.T, open func(*testing.T) *Repository) {
	mutations := map[string]string{
		"bound fence":             "",
		"generic fence":           `UPDATE session_recovery_blocks SET reason = 'unresolved_durable_work', delivery_submission_id = '', delivery_stream_id = ''`,
		"missing effect":          `DELETE FROM agent_delivery_effects`,
		"pending effect":          `UPDATE agent_delivery_effects SET state = 'pending'`,
		"wrong effect type":       `UPDATE agent_delivery_effects SET effect_type = 'foreign'`,
		"wrong effect owner":      `UPDATE agent_delivery_effects SET incarnation_id = 'foreign'`,
		"wrong effect generation": `UPDATE agent_delivery_effects SET harness_generation = 2`,
		"wrong effect outcome":    `UPDATE agent_delivery_effects SET outcome = 'completed'`,
		"changed hash":            `UPDATE agent_delivery_submissions SET payload_hash = 'foreign'`,
		"partial binding":         `UPDATE agent_delivery_submissions SET terminal_stream_id = 'foreign'`,
		"partial turn binding":    `UPDATE agent_delivery_submissions SET terminal_turn_id = 'turn'`,
		"settled marker":          `UPDATE agent_delivery_submissions SET fully_settled_at = CURRENT_TIMESTAMP`,
		"pruned marker":           `UPDATE agent_delivery_submissions SET payload_pruned_at = CURRENT_TIMESTAMP`,
		"unprojected":             `UPDATE agent_delivery_cursors SET received_sequence = 2`,
		"invalid retired cursor":  `UPDATE agent_delivery_cursors SET retired_sequence = 2`,
		"superseded": `INSERT INTO agent_delivery_effects (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at)
 SELECT 'newer', stream_id, 2, effect_type, 'pending', session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at FROM agent_delivery_effects`,
	}
	for scenario, mutation := range mutations {
		t.Run(scenario, func(t *testing.T) {
			repo := open(t)
			f, proof := pendingTerminalProof(t, repo)
			if mutation != "" {
				_, err := repo.DB().ExecContext(t.Context(), mutation)
				require.NoError(t, err)
			}
			before, err := repo.GetAgentDeliverySubmissionMetadata(t.Context(), proof.Submissions[0].ID)
			require.NoError(t, err)
			resolved, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), f.block.ID, proof, time.Now().UTC())
			require.NoError(t, err)
			success := scenario == "bound fence" || scenario == "generic fence"
			require.Equal(t, success, resolved)
			block, err := repo.GetSessionRecoveryBlock(t.Context(), f.block.ID)
			require.NoError(t, err)
			require.Equal(t, success, block.State == models.RecoveryBlockResolved)
			after, err := repo.GetAgentDeliverySubmissionMetadata(t.Context(), proof.Submissions[0].ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
