package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type nativeDeliveryResumer interface {
	AcknowledgeNativeResumeDelivery(context.Context, string) error
}

func nativeResumeJournalPeer(t *testing.T, store *journal.Journal, rejectRetirement bool) *agentctl.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/agent/delivery":
			status, err := store.RecoveryDescriptor(r.Context(), "session", "incarnation", 2, "stream")
			if err != nil {
				http.Error(w, "unavailable", http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(status)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/submissions":
			submissions, err := store.ListSubmissions(r.Context(), "session")
			if err != nil {
				http.Error(w, "unavailable", http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(submissions)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/submissions/interrupted/retire":
			if rejectRetirement {
				http.Error(w, "retirement rejected", http.StatusConflict)
				return
			}
			if _, err := store.RetireSubmission(r.Context(), "interrupted", 2); err != nil {
				http.Error(w, "retirement rejected", http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected operation", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return agentctl.NewClient(host, port, newTestLogger())
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
func TestNativeResumeAcknowledgesRealJournalBeforeNewWork(t *testing.T) {
	for _, condition := range []string{"interrupted", "earlier generation", "live work", "retirement rejected", "uninitialized", "wrong incarnation"} {
		t.Run(condition, func(t *testing.T) {
			ctx := context.Background()
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			original := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "hash", Payload: []byte("original prompt"), State: journal.SubmissionInterruptedUnknown}
			if condition == "earlier generation" {
				original.HarnessGeneration = 1
			}
			_, err = store.PutSubmission(ctx, original)
			require.NoError(t, err)
			if condition == "live work" {
				live := original
				live.ID, live.State = "live", journal.SubmissionAccepted
				_, err = store.PutSubmission(ctx, live)
				require.NoError(t, err)
			}
			mgr := newTestManager(t)
			repository := &adoptionDeliveryRepository{submissions: map[string]*models.AgentDeliverySubmission{
				original.ID: {ID: original.ID, SessionID: original.SessionID, IncarnationID: original.IncarnationID,
					HarnessGeneration: int64(original.HarnessGeneration), PayloadHash: original.Hash, State: models.DeliverySubmissionInterruptedUnknown},
			}}
			mgr.streamManager.setAgentDeliveryRepository(repository)
			execution := &AgentExecution{ID: "execution", SessionID: "session", ACPSessionID: "native-conversation", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream", agentctl: nativeResumeJournalPeer(t, store, condition == "retirement rejected")}
			execution.setSessionInitialized(condition != "uninitialized")
			if condition == "wrong incarnation" {
				execution.DeliveryIncarnationID = "other"
			}
			require.NoError(t, mgr.executionStore.Add(execution))
			resumer, ok := any(mgr).(nativeDeliveryResumer)
			require.True(t, ok, "explicit native resume must reconcile the retained journal")
			err = resumer.AcknowledgeNativeResumeDelivery(ctx, execution.SessionID)
			if condition == "interrupted" || condition == "earlier generation" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			stored, err := store.GetSubmission(ctx, original.ID)
			require.NoError(t, err)
			require.Equal(t, condition == "interrupted" || condition == "earlier generation", stored.Retired)
			require.Equal(t, original.State, stored.State)
			require.Equal(t, original.Payload, stored.Payload)
			require.Equal(t, "native-conversation", execution.ACPSessionID)
			if stored.Retired {
				status, err := execution.agentctl.GetDeliveryStatus(ctx, execution.DeliveryStreamID)
				require.NoError(t, err)
				require.NoError(t, newAdoptionManager(repository).restoreRecoveredSubmission(ctx, execution, status, repository, execution.agentctl))
				require.Empty(t, execution.deliverySubmissionIDSnapshot())
			}
		})
	}
}

func TestAcknowledgedUnknownHistorySurvivesDurableAdoption(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	repository := &adoptionDeliveryRepository{submissions: make(map[string]*models.AgentDeliverySubmission)}
	for _, id := range []string{"earlier", "interrupted"} {
		generation := uint64(2)
		if id == "earlier" {
			generation = 1
		}
		_, err = store.PutSubmission(ctx, journal.Submission{ID: id, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: generation, Hash: "hash", Payload: []byte("prompt"), State: journal.SubmissionInterruptedUnknown, Retired: true})
		require.NoError(t, err)
		repository.submissions[id] = &models.AgentDeliverySubmission{ID: id, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: int64(generation), PayloadHash: "hash", State: models.DeliverySubmissionInterruptedUnknown}
	}
	client := nativeResumeJournalPeer(t, store, false)
	status, err := client.GetDeliveryStatus(ctx, "stream")
	require.NoError(t, err)
	manager := newAdoptionManager(repository)
	execution := &AgentExecution{SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream"}
	require.NoError(t, manager.restoreRecoveredSubmission(ctx, execution, status, repository, client))
	require.Empty(t, execution.deliverySubmissionIDSnapshot())
}

func TestDurableAdoptionRejectsMismatchedRetirementEvidence(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.PutSubmission(ctx, journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, Hash: "peer-hash", State: journal.SubmissionInterruptedUnknown, Retired: true})
	require.NoError(t, err)
	repository := &adoptionDeliveryRepository{submissions: map[string]*models.AgentDeliverySubmission{
		"interrupted": {ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, PayloadHash: "different-sql-hash", State: models.DeliverySubmissionInterruptedUnknown},
	}}
	client := nativeResumeJournalPeer(t, store, false)
	status, err := client.GetDeliveryStatus(ctx, "stream")
	require.NoError(t, err)
	manager := newAdoptionManager(repository)
	execution := &AgentExecution{SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream"}
	require.ErrorIs(t, manager.restoreRecoveredSubmission(ctx, execution, status, repository, client), ErrDurableAdoptionBlocked)
}
