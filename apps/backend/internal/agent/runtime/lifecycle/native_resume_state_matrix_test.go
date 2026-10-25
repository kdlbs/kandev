package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// The table describes admission after native restoration with no retained
// terminal. R retires unknown history, K preserves settled history, B blocks.
// A dispatch label alone is never evidence that an external call did not occur.
func TestNativeResumeCrossStoreStateDisposition(t *testing.T) {
	states := []string{"prepared", "accepted", "dispatching", "interrupted_unknown", "completed", "failed", "cancelled"}
	model := []string{
		"BBBBBBB", // SQL prepared
		"BBBBBBB", // SQL accepted
		"BBBRRRR", // SQL dispatching: matching unproven history becomes unknown
		"BBBRRRR", // SQL unknown: retain historical labels while retiring
		"BBBBBKK", // SQL completed: no terminal journal evidence may be invented
		"BBBBBKK", // SQL failed
		"BBBBBKK", // SQL cancelled
	}
	for sqlIndex, sqlState := range states {
		for peerIndex, peerState := range states {
			t.Run(sqlState+"/"+peerState, func(t *testing.T) {
				ctx := context.Background()
				store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
				require.NoError(t, err)
				t.Cleanup(func() { _ = store.Close() })
				peer := journal.Submission{ID: "prompt:queue:interrupted", SessionID: "session", IncarnationID: "incarnation",
					HarnessGeneration: 2, StreamID: "stream", Hash: "immutable-hash", Payload: []byte("old instruction"), State: journal.SubmissionState(peerState)}
				_, err = store.PutSubmission(ctx, peer)
				require.NoError(t, err)
				repo, _ := runOwnerTestRepository(t)
				require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "State model"}))
				require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID,
					IncarnationID: peer.IncarnationID, HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: peer.Hash, Payload: peer.Payload, State: models.DeliverySubmissionState(sqlState)})
				require.NoError(t, err)
				manager := newTestManager(t)
				manager.SetAgentDeliveryRepository(repo)
				execution := &AgentExecution{ID: "replacement", SessionID: "session", ACPSessionID: "saved-native", DeliveryIncarnationID: "incarnation",
					DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream", agentctl: nativeResumeJournalPeer(t, store, false)}
				execution.setSessionInitialized(true)
				require.NoError(t, manager.executionStore.Add(execution))
				err = manager.AcknowledgeNativeResumeDelivery(ctx, "session")
				disposition := model[sqlIndex][peerIndex]
				if disposition == 'B' {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				stored, err := store.GetSubmission(ctx, peer.ID)
				require.NoError(t, err)
				require.Equal(t, disposition == 'R', stored.Retired)
				require.Equal(t, peer.State, stored.State)
				require.Equal(t, peer.Payload, stored.Payload)
				require.Equal(t, peer.Hash, stored.Hash)
				require.Zero(t, stored.TerminalSequence)
				row, err := repo.GetAgentDeliverySubmission(ctx, peer.ID)
				require.NoError(t, err)
				if disposition == 'R' {
					require.Equal(t, models.DeliverySubmissionInterruptedUnknown, row.State)
				} else {
					require.Equal(t, models.DeliverySubmissionState(sqlState), row.State)
				}
				require.Zero(t, row.TerminalSequence)
				if disposition == 'R' {
					require.NoError(t, manager.AcknowledgeNativeResumeDelivery(ctx, "session"), "repeated recovery must converge")
					status, err := execution.agentctl.GetDeliveryStatus(ctx, "stream")
					require.NoError(t, err)
					require.NoError(t, newAdoptionManager(repo).restoreRecoveredSubmission(ctx, execution, status, repo, execution.agentctl))
					require.Empty(t, execution.deliverySubmissionIDSnapshot(), "old queue instruction cannot become current work")
				}
			})
		}
	}
}
