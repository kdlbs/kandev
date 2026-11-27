package lifecycle

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeRetiresUnknownWithProjectedTerminal(t *testing.T) {
	for _, scenario := range []string{"failed", "completed", "cancelled", "earlier generation", "pruned payload", "automatic", "automatic busy", "wrong hash", "wrong sequence", "wrong stream", "unprojected", "missing terminal", "live peer", "live SQL"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			peer := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "hash", Payload: []byte("original prompt"), State: journal.SubmissionInterruptedUnknown}
			if scenario == "earlier generation" {
				peer.HarnessGeneration, peer.StreamID = 1, "earlier"
			}
			_, err = store.PutSubmission(ctx, peer)
			require.NoError(t, err)
			eventType, state := "error", models.DeliverySubmissionFailed
			if scenario == "cancelled" {
				eventType, state = "complete", models.DeliverySubmissionCancelled
			}
			if scenario == "completed" {
				eventType, state = "complete", models.DeliverySubmissionCompleted
			}
			event, err := store.Append(ctx, journal.Event{SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: peer.HarnessGeneration, StreamID: peer.StreamID, SubmissionID: peer.ID, Type: eventType, Terminal: scenario != "missing terminal", Payload: []byte("terminal")})
			require.NoError(t, err)
			require.NoError(t, store.Acknowledge(ctx, peer.StreamID, event.Sequence))
			row := &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: int64(peer.HarnessGeneration), PayloadHash: peer.Hash, Payload: peer.Payload, State: state, Outcome: string(state), TerminalStreamID: peer.StreamID, TerminalSequence: 1}
			repo := &adoptionDeliveryRepository{cursor: &models.AgentDeliveryCursor{StreamID: "stream", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, ReceivedSequence: 1, ProjectedSequence: 1}, submissions: map[string]*models.AgentDeliverySubmission{peer.ID: row}}
			switch scenario {
			case "wrong hash":
				row.PayloadHash = "foreign"
			case "wrong sequence":
				row.TerminalSequence = 2
			case "wrong stream":
				row.TerminalStreamID = "foreign"
			case "unprojected":
				repo.cursor.ProjectedSequence = 0
			case "live SQL":
				live := *row
				live.ID, live.State = "independent", models.DeliverySubmissionAccepted
				repo.submissions[live.ID] = &live
			case "live peer":
				live := peer
				live.ID, live.State = "independent", journal.SubmissionAccepted
				_, err = store.PutSubmission(ctx, live)
				require.NoError(t, err)
			}

			var repository AgentDeliveryRepository = repo
			if scenario == "earlier generation" {
				cursor := *repo.cursor
				cursor.StreamID, cursor.HarnessGeneration = peer.StreamID, 1
				repo.cursor = nil
				repository = &terminalMetadataTestRepository{adoptionDeliveryRepository: repo, earlier: &cursor}
			}
			if scenario == "pruned payload" {
				at := time.Now().UTC()
				row.PayloadPrunedAt, row.Payload = &at, nil
				repository = &terminalMetadataTestRepository{adoptionDeliveryRepository: repo}
			}
			before := *row
			manager := newTestManager(t)
			manager.SetAgentDeliveryRepository(repository)
			execution := &AgentExecution{ID: "replacement", SessionID: "session", ACPSessionID: "original-native", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream", Status: v1.AgentStatusReady, agentctl: nativeResumeJournalPeer(t, store, false)}
			if scenario == "automatic busy" {
				execution.dispatchedPromptPending.Store(true)
			}
			execution.setSessionInitialized(true)
			require.NoError(t, manager.executionStore.Add(execution))
			if scenario == "automatic" || scenario == "automatic busy" {
				_, err = manager.ReconcileLoadedNativeDelivery(ctx, "session")
			} else {
				err = manager.AcknowledgeNativeResumeDelivery(ctx, "session")
			}
			success := scenario == "failed" || scenario == "completed" || scenario == "cancelled" || scenario == "earlier generation" || scenario == "pruned payload" || scenario == "automatic"
			if success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			stored, err := store.GetSubmission(ctx, peer.ID)
			require.NoError(t, err)
			require.Equal(t, success, stored.Retired)
			require.Equal(t, peer.State, stored.State)
			require.Equal(t, peer.Payload, stored.Payload)
			require.Equal(t, before, *row)
			require.Equal(t, "original-native", execution.ACPSessionID)
			if success {
				proof, err := manager.GetNativeResumeDeliveryEvidence(ctx, "session")
				require.NoError(t, err)
				require.Len(t, proof.Submissions, 1)
				require.Equal(t, row.TerminalSequence, proof.Submissions[0].TerminalSequence)
				require.NoError(t, manager.AcknowledgeNativeResumeDelivery(ctx, "session"), "repeated native restore remains usable")
			}
		})
	}
}

type terminalMetadataTestRepository struct {
	*adoptionDeliveryRepository
	earlier *models.AgentDeliveryCursor
}

func (r *terminalMetadataTestRepository) GetAgentDeliverySubmission(context.Context, string) (*models.AgentDeliverySubmission, error) {
	return nil, fmt.Errorf("prompt payload was pruned")
}
func (r *terminalMetadataTestRepository) GetAgentDeliverySubmissionMetadata(ctx context.Context, id string) (*models.AgentDeliverySubmission, error) {
	return r.adoptionDeliveryRepository.GetAgentDeliverySubmission(ctx, id)
}
func (r *terminalMetadataTestRepository) GetAgentDeliveryCursor(ctx context.Context, stream string) (*models.AgentDeliveryCursor, error) {
	if r.earlier != nil && stream == r.earlier.StreamID {
		cursor := *r.earlier
		return &cursor, nil
	}
	return r.adoptionDeliveryRepository.GetAgentDeliveryCursor(ctx, stream)
}
