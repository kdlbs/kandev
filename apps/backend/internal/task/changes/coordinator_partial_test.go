package changes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorPreservesHealthyCheckoutWithFailedSibling(t *testing.T) {
	for _, phase := range []string{"start_scope", "end_scope", "end_capture"} {
		t.Run(phase, func(t *testing.T) {
			store := &partialCoordinatorStore{newCoordinatorStore()}
			policy := &coordinatorPolicy{result: turnchanges.CapturePolicy{Enabled: true}}
			client := &partialCheckpointClient{coordinatorCheckpointClient: coordinatorCheckpointClient{scopes: []string{"frontend", "backend"}}}
			coordinator := NewCoordinator(store, coordinatorTurnReader{}, policy, NewContentService(store, nil), nil)
			admission := coordinatorAdmission()
			admission.Checkouts = append(admission.Checkouts, Checkout{ID: "checkout-two", RepositorySubpath: "backend", RepositorySubpathKnown: true})
			if phase == "start_scope" {
				client.scopes = []string{"frontend"}
			}
			err := coordinator.Admit(context.Background(), admission, client)
			if phase == "start_scope" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if phase == "end_scope" {
				client.scopes = []string{"frontend"}
			}
			if phase == "end_capture" {
				client.failedCheckout = "checkout-two"
			}
			terminal := Terminal{Admission: admission, At: time.Now()}
			require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), terminal, client))
			// A later retry must not fabricate a new endpoint for the failed checkout.
			client.failedCheckout = ""
			client.scopes = []string{"frontend", "backend"}
			require.NoError(t, coordinator.CaptureTerminalEndpoints(context.Background(), terminal, client))
			require.NoError(t, coordinator.ProcessTerminal(context.Background(), terminal, client))
			set := store.changeSets[turnChangeSetID(admission.TurnID)]
			require.EqualValues(t, 1, set.FileCount)
			require.Equal(t, models.TurnChangeAvailabilityReady, set.Availability)
			require.False(t, set.Complete)
			require.Equal(t, models.TurnChangeAvailabilityReady, store.repositoryRows[set.ID][0].Availability)
			failed := store.repositoryRows[set.ID][1]
			require.Equal(t, models.TurnChangeAvailabilityUnavailable, failed.Availability)
			require.Empty(t, failed.EndCommitOID)
			require.Equal(t, 1, client.exportCalls)
		})
	}
}

type partialCheckpointClient struct {
	coordinatorCheckpointClient
	failedCheckout string
}

func (c *partialCheckpointClient) CaptureTurnCheckpoint(ctx context.Context, request turnchanges.CheckpointRequest) (*turnchanges.CheckpointResult, error) {
	if request.Boundary == turnchanges.CheckpointEnd && request.CheckoutID == c.failedCheckout {
		return nil, errors.New("checkout disappeared")
	}
	return c.coordinatorCheckpointClient.CaptureTurnCheckpoint(ctx, request)
}

func (s *coordinatorStore) SetTurnRepositoryEndUnavailable(_ context.Context, changeSetID, repositoryChangeID, startCommitOID, startTreeOID string, reason models.TurnChangeReason) (bool, error) {
	for _, row := range s.repositoryRows[changeSetID] {
		if row.ID != repositoryChangeID || row.StartCommitOID != startCommitOID || row.StartTreeOID != startTreeOID {
			continue
		}
		if row.EndCommitOID != "" {
			return true, nil
		}
		row.Availability, row.Reason = models.TurnChangeAvailabilityUnavailable, reason
		return true, nil
	}
	return false, nil
}

type partialCoordinatorStore struct{ *coordinatorStore }

func (s *partialCoordinatorStore) FinalizeTurnChangeSet(ctx context.Context, id string, revision int64, finalization models.TurnChangeSetFinalization) (bool, error) {
	accepted, err := s.coordinatorStore.FinalizeTurnChangeSet(ctx, id, revision, finalization)
	if err != nil || !accepted {
		return accepted, err
	}
	for index := range finalization.Repositories {
		row := finalization.Repositories[index]
		for existing, stored := range s.repositoryRows[id] {
			if stored.ID == row.ID {
				s.repositoryRows[id][existing] = &row
			}
		}
	}
	return true, nil
}
