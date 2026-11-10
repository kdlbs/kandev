package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type nativeProjectionRepository struct {
	*adoptionDeliveryRepository
	reads       int
	advance     bool
	cancel      context.CancelFunc
	changeOwner bool
	advanceAck  bool
}

func (r *nativeProjectionRepository) GetAgentDeliveryCursor(ctx context.Context, streamID string) (*models.AgentDeliveryCursor, error) {
	r.reads++
	if r.advance && r.reads >= 3 {
		r.cursor.ReceivedSequence = 5
		r.cursor.ProjectedSequence = 5
	}
	if r.cancel != nil && r.reads >= 2 {
		r.cancel()
	}
	return r.adoptionDeliveryRepository.GetAgentDeliveryCursor(ctx, streamID)
}

func nativeProjectionManager(t *testing.T, repository *nativeProjectionRepository) *Manager {
	t.Helper()
	status := durableAdoptionStatus()
	var statusReads atomic.Int32
	client := processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("native proof attempted a mutation: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected mutation", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v1/agent/delivery":
			current := *status
			if statusReads.Add(1) > 1 {
				if repository.changeOwner {
					current.IncarnationID = "foreign"
				}
				if repository.advanceAck {
					stream := *current.Stream
					stream.Acknowledged++
					current.Stream = &stream
				}
			}
			if err := json.NewEncoder(w).Encode(current); err != nil {
				t.Error(err)
			}
		case "/api/v1/agent/submissions":
			_, err := w.Write([]byte("[]"))
			if err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected native proof request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	manager := newTestManager(t)
	manager.streamManager.setAgentDeliveryRepository(repository)
	execution := &AgentExecution{ID: "execution", SessionID: status.SessionID, ACPSessionID: "native-1",
		DeliveryIncarnationID: status.IncarnationID, DeliveryHarnessGeneration: status.HarnessGeneration,
		DeliveryStreamID: status.StreamID, agentctl: client}
	execution.setSessionInitialized(true)
	require.NoError(t, manager.executionStore.Add(execution))
	return manager
}

func TestNativeResumeProofWaitsForCapturedStreamProjection(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		received int64
	}{
		{name: "peer ahead of inbox", received: 2},
		{name: "received but not projected", received: 5},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repository := &nativeProjectionRepository{adoptionDeliveryRepository: durableAdoptionRepository(), advance: true}
			repository.cursor.ReceivedSequence = scenario.received
			manager := nativeProjectionManager(t, repository)
			proof, err := manager.GetNativeResumeDeliveryEvidence(context.Background(), "session-1")
			require.NoError(t, err)
			require.EqualValues(t, 5, proof.Cursor.ProjectedSequence)
			require.EqualValues(t, 5, proof.Cursor.ReceivedSequence)
		})
	}
}

func TestNativeResumeProofDoesNotRetryChangedOwner(t *testing.T) {
	repository := &nativeProjectionRepository{adoptionDeliveryRepository: durableAdoptionRepository(), changeOwner: true}
	repository.cursor.ReceivedSequence = 5
	repository.cursor.ProjectedSequence = 5
	manager := nativeProjectionManager(t, repository)
	proof, err := manager.GetNativeResumeDeliveryEvidence(context.Background(), "session-1")
	require.Nil(t, proof)
	require.ErrorIs(t, err, ErrDeliveryRecordEvidenceStale)
	require.Equal(t, 2, repository.reads)
}

func TestNativeResumeProofRevalidatesAdvancingAcknowledgment(t *testing.T) {
	repository := &nativeProjectionRepository{adoptionDeliveryRepository: durableAdoptionRepository(), advance: true, advanceAck: true}
	repository.cursor.ReceivedSequence = 5
	manager := nativeProjectionManager(t, repository)
	proof, err := manager.GetNativeResumeDeliveryEvidence(context.Background(), "session-1")
	require.NoError(t, err)
	require.EqualValues(t, 5, proof.Cursor.ProjectedSequence)
	require.Equal(t, 4, repository.reads)
}

func TestNativeResumeProofWaitsForInitialStreamProgress(t *testing.T) {
	repository := &nativeProjectionRepository{adoptionDeliveryRepository: durableAdoptionRepository(), advanceAck: true}
	repository.cursor.ReceivedSequence = 5
	repository.cursor.ProjectedSequence = 5
	manager := nativeProjectionManager(t, repository)
	proof, err := manager.GetNativeResumeDeliveryEvidence(t.Context(), "session-1")
	require.NoError(t, err)
	require.EqualValues(t, 5, proof.Cursor.ProjectedSequence)
	require.Equal(t, 4, repository.reads)
}

func TestNativeResumeProofProjectionWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repository := &nativeProjectionRepository{adoptionDeliveryRepository: durableAdoptionRepository(), cancel: cancel}
	repository.cursor.ReceivedSequence = 5
	manager := nativeProjectionManager(t, repository)
	proof, err := manager.GetNativeResumeDeliveryEvidence(ctx, "session-1")
	require.Nil(t, proof)
	require.ErrorIs(t, err, context.Canceled)
}
