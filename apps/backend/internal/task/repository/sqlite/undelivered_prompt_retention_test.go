package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-001.3
func TestUnsentPromptRetention(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			for _, scenario := range []string{"expired", "recent", "blocked", "stream block", "unsettled", "unknown", "terminal effect", "byte limit"} {
				t.Run(scenario, func(t *testing.T) {
					repo := newRetentionTestRepo(t, engine)
					ctx := t.Context()
					now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
					repo.clockNow = func() time.Time { return now }
					require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Unsent prompt"}))
					require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
					row := &models.AgentDeliverySubmission{
						ID: "prompt:unsent", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1,
						DispatchAttemptID: "attempt", PayloadHash: "hash", Payload: []byte("prompt and attachments"), State: models.DeliverySubmissionDispatching,
					}
					created, err := repo.PrepareAgentDeliverySubmission(ctx, row)
					require.NoError(t, err)
					require.True(t, created)
					settled := now.Add(-2 * time.Hour)
					if scenario == "recent" {
						settled = now
					}
					if scenario == "unsettled" || scenario == "unknown" {
						state := models.DeliverySubmissionFailed
						if scenario == "unknown" {
							state = models.DeliverySubmissionInterruptedUnknown
						}
						_, err = repo.TransitionAgentDeliverySubmission(ctx, row.ID, models.DeliverySubmissionDispatching, state, string(state), settled)
						require.NoError(t, err)
					} else {
						changed, err := repo.FailUnsentAgentDeliverySubmission(ctx, row, settled)
						require.NoError(t, err)
						require.True(t, changed)
					}
					if scenario == "unknown" {
						_, err := repo.db.ExecContext(ctx, repo.db.Rebind("UPDATE agent_delivery_submissions SET fully_settled_at = ? WHERE id = ?"), settled, row.ID)
						require.NoError(t, err)
					}
					if scenario == "terminal effect" {
						_, err := repo.PutAgentDeliveryEffect(ctx, &models.AgentDeliveryEffect{EffectKey: "terminal", StreamID: "stream", Sequence: 1, EffectType: "agent_delivery.terminal_settlement", State: "pending", SessionID: row.SessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1, SubmissionID: row.ID})
						require.NoError(t, err)
					}
					if scenario == "blocked" || scenario == "stream block" {
						streamID := ""
						if scenario == "stream block" {
							streamID = "other-stream"
						}
						require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
							ID: "block", DeliveryStreamID: streamID, SessionID: row.SessionID, IncarnationID: row.IncarnationID, ExpectedGeneration: 1,
							Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery", DeliverySubmissionID: row.ID,
						}))
					}
					options := models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 100, WorkTimeout: 5 * time.Second}
					if scenario == "byte limit" {
						options.MaxPayloadBytes = 1
					}
					pruned := 0
					for range 2 {
						result, err := repo.RunAgentDeliveryRetentionBatch(ctx, options)
						require.NoError(t, err)
						pruned += result.PromptPayloadsPruned
					}
					metadata, err := repo.GetAgentDeliverySubmissionMetadata(ctx, row.ID)
					require.NoError(t, err)
					require.Equal(t, row.PayloadHash, metadata.PayloadHash)
					require.Equal(t, row.DispatchAttemptID, metadata.DispatchAttemptID)
					require.Zero(t, metadata.TerminalSequence)
					var payloadLength int
					require.NoError(t, repo.db.Get(&payloadLength, repo.db.Rebind("SELECT LENGTH(payload) FROM agent_delivery_submissions WHERE id = ?"), row.ID))
					if scenario == "expired" {
						require.Equal(t, 1, pruned)
						require.Zero(t, payloadLength)
						require.NotNil(t, metadata.PayloadPrunedAt)
						require.Equal(t, models.DeliverySubmissionFailed, metadata.State)
					} else {
						require.Zero(t, pruned)
						require.Equal(t, len(row.Payload), payloadLength)
						require.Nil(t, metadata.PayloadPrunedAt)
					}
				})
			}
		})
	}
}
