package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeReconcilesJournalUnknownSQLDispatch(t *testing.T) {
	for _, scenario := range []string{"abandoned", "queue origin", "hash conflict", "peer still dispatching", "independent work"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			peer := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "original-hash", Payload: []byte("original prompt"), State: journal.SubmissionInterruptedUnknown}
			if scenario == "queue origin" {
				peer.ID = "prompt:queue:interrupted"
			}
			if scenario == "peer still dispatching" {
				peer.State = journal.SubmissionDispatching
			}
			_, err = store.PutSubmission(ctx, peer)
			require.NoError(t, err)
			repo, _ := runOwnerTestRepository(t)
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Native recovery"}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
			hash := peer.Hash
			if scenario == "hash conflict" {
				hash = "different-hash"
			}
			_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: hash, Payload: peer.Payload, State: models.DeliverySubmissionDispatching})
			require.NoError(t, err)
			if scenario == "independent work" {
				_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "independent", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "independent", Payload: []byte("independent"), State: models.DeliverySubmissionAccepted})
				require.NoError(t, err)
			}
			manager := newTestManager(t)
			manager.SetAgentDeliveryRepository(repo)
			execution := &AgentExecution{ID: "replacement", SessionID: "session", ACPSessionID: "original-native", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream", agentctl: nativeResumeJournalPeer(t, store, false)}
			execution.setSessionInitialized(true)
			require.NoError(t, manager.executionStore.Add(execution))
			err = manager.AcknowledgeNativeResumeDelivery(ctx, "session")
			if scenario == "abandoned" || scenario == "queue origin" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			submission, err := repo.GetAgentDeliverySubmission(ctx, peer.ID)
			require.NoError(t, err)
			expected := models.DeliverySubmissionDispatching
			if scenario == "abandoned" || scenario == "queue origin" {
				expected = models.DeliverySubmissionInterruptedUnknown
				require.Equal(t, "unknown", submission.Outcome)
			}
			require.Equal(t, expected, submission.State)
			require.Zero(t, submission.TerminalSequence)
			require.Equal(t, peer.Payload, submission.Payload)
			stored, err := store.GetSubmission(ctx, peer.ID)
			require.NoError(t, err)
			require.Equal(t, scenario == "abandoned" || scenario == "queue origin", stored.Retired)
			require.Equal(t, "original-native", execution.ACPSessionID)
		})
	}
}
