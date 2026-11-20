package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeAbandonsUncommittedSuccessorStream(t *testing.T) {
	ctx := t.Context()
	store := openRetentionLifecycleJournal(t, filepath.Join(t.TempDir(), "delivery.bbolt"))
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Orphan stream"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session", TaskID: "task", QueueIncarnationID: "incarnation", State: models.TaskSessionStateWaitingForInput,
		Metadata: map[string]any{"acp": map[string]any{"session_id": "native-conversation"}},
	}))
	committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 1,
		NativeSessionID: "native-conversation", CreationReason: "initial",
	}, 0)
	require.NoError(t, err)
	require.True(t, committed)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session", SessionID: "session", TaskID: "task", AgentExecutionID: "execution",
		Status: models.ExecutorRunningStatusRunning,
	}))

	_, err = store.Append(ctx, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "incarnation:g1", Type: "usage_update", Payload: []byte(`{"generation":1}`),
	})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		TaskID: "task", ExecutionID: "execution", OwnerGeneration: 1,
		ExpectedStreamID: "incarnation:g1", StreamID: "incarnation:g1", Sequence: 1,
		EventType: "usage_update", Payload: []byte(`{"generation":1}`),
	}
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.NoError(t, store.Acknowledge(ctx, "incarnation:g1", 1))

	_, err = store.Append(ctx, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "incarnation:g2", Type: "usage_update", Payload: []byte(`{"generation":2}`),
	})
	require.NoError(t, err)
	peer := journal.Submission{
		ID: "interrupted", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "incarnation:g1", Hash: "prompt-hash",
		Payload: []byte("original prompt"), State: journal.SubmissionInterruptedUnknown,
	}
	_, err = store.PutSubmission(ctx, peer)
	require.NoError(t, err)
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: peer.Hash,
		Payload: peer.Payload, State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "unresolved-block", SessionID: "session", IncarnationID: "incarnation",
		ExpectedGeneration: 1, State: models.RecoveryBlockOpen,
		Reason: "unresolved_durable_work", ConsumerReference: "agent_delivery",
	}))

	var abandoned int
	var successorInventories int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/delivery":
			status, statusErr := store.RecoveryDescriptor(r.Context(), "session", "incarnation", 1, "incarnation:g1")
			if statusErr != nil {
				http.Error(w, "status unavailable", http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(status)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/submissions":
			submissions, listErr := store.ListSubmissions(r.Context(), "session")
			if listErr != nil {
				http.Error(w, "submissions unavailable", http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(submissions)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/interrupted/retire"):
			if _, retireErr := store.RetireSubmission(r.Context(), peer.ID, 1); retireErr != nil {
				http.Error(w, "retirement rejected", http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/delivery/successors":
			successorInventories++
			streams, listErr := store.ListSuccessorStreams(r.Context(), "session", "incarnation", 1)
			if listErr != nil {
				http.Error(w, "successor streams unavailable", http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(streams)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/delivery/stream/abandon":
			var expected journal.Stream
			if decodeErr := json.NewDecoder(r.Body).Decode(&expected); decodeErr != nil {
				http.Error(w, "invalid stream", http.StatusBadRequest)
				return
			}
			if abandonErr := store.AbandonStream(r.Context(), expected); abandonErr != nil {
				http.Error(w, "abandon rejected", http.StatusConflict)
				return
			}
			abandoned++
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
	client := agentctl.NewClient(host, port, newTestLogger())

	manager := newTestManager(t)
	manager.SetAgentDeliveryRepository(repo)
	execution := &AgentExecution{
		ID: "execution", SessionID: "session", ACPSessionID: "native-conversation",
		DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliveryStreamID: "incarnation:g1",
		Status: v1.AgentStatusReady, agentctl: client,
	}
	execution.setSessionInitialized(true)
	require.NoError(t, manager.executionStore.Add(execution))

	require.NoError(t, manager.AcknowledgeNativeResumeDelivery(ctx, "session"))
	require.Equal(t, 1, successorInventories)
	require.Equal(t, 1, abandoned)
	proof, err := manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.NoError(t, err)
	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, "unresolved-block", proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed)
	block, err := repo.GetSessionRecoveryBlock(ctx, "unresolved-block")
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)
	resolved, err := store.HasUnresolvedWork(ctx)
	require.NoError(t, err)
	require.False(t, resolved)
	_, err = store.GetStream(ctx, "incarnation:g2")
	require.ErrorIs(t, err, journal.ErrStreamNotFound)
	require.Equal(t, 1, successorInventories)
	require.Equal(t, 1, abandoned)
}

type orphanCursorRepository struct {
	*tasksqlite.Repository
	cursor *models.AgentDeliveryCursor
}

func (r *orphanCursorRepository) GetAgentDeliveryCursor(ctx context.Context, streamID string) (*models.AgentDeliveryCursor, error) {
	if r.cursor != nil && r.cursor.StreamID == streamID {
		copy := *r.cursor
		return &copy, nil
	}
	return r.Repository.GetAgentDeliveryCursor(ctx, streamID)
}

func TestFailedSuccessorBootstrapAbandonsOnlyUncommittedCursorlessStream(t *testing.T) {
	for _, scenario := range []string{"orphan", "committed generation", "SQL cursor"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			store := openRetentionLifecycleJournal(t, filepath.Join(t.TempDir(), "delivery.bbolt"))
			repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Bootstrap cleanup"}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "session", TaskID: "task", QueueIncarnationID: "incarnation", State: models.TaskSessionStateWaitingForInput,
			}))
			committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
				SessionID: "session", IncarnationID: "incarnation", Generation: 1,
				NativeSessionID: "native-one", CreationReason: "initial",
			}, 0)
			require.NoError(t, err)
			require.True(t, committed)
			if scenario == "committed generation" {
				committed, err = repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
					SessionID: "session", IncarnationID: "incarnation", Generation: 2,
					PredecessorGeneration: 1, NativeSessionID: "native-two", CreationReason: "context_continued",
				}, 1)
				require.NoError(t, err)
				require.True(t, committed)
			}
			_, err = store.Append(ctx, journal.Event{
				SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
				StreamID: "incarnation:g2", Identity: "orphan-event", Type: "usage_update", Payload: []byte(`{}`),
			})
			require.NoError(t, err)

			var abandoned int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agent/delivery/stream/abandon-bootstrap" {
					http.Error(w, "unexpected operation", http.StatusBadRequest)
					return
				}
				var request struct {
					StreamID          string `json:"stream_id"`
					HarnessGeneration uint64 `json:"harness_generation"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				stream, err := store.GetStream(r.Context(), request.StreamID)
				if err != nil || request.HarnessGeneration != 2 {
					http.Error(w, "stream unavailable", http.StatusConflict)
					return
				}
				if err := store.AbandonStream(r.Context(), stream); err != nil {
					http.Error(w, "stream abandonment rejected", http.StatusConflict)
					return
				}
				abandoned++
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			client := agentctl.NewClient(host, port, newTestLogger())
			manager := newTestManager(t)
			var delivery AgentDeliveryRepository = repo
			if scenario == "SQL cursor" {
				delivery = &orphanCursorRepository{Repository: repo, cursor: &models.AgentDeliveryCursor{
					StreamID: "incarnation:g2", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
				}}
			}
			manager.SetAgentDeliveryRepository(delivery)
			execution := &AgentExecution{
				ID: "successor", SessionID: "session", DeliveryIncarnationID: "incarnation",
				DeliveryHarnessGeneration: 2, DeliveryStreamID: "incarnation:g2", ForceContextContinuation: true,
				agentctl: client,
			}

			err = manager.abandonFailedSuccessorBootstrapStream(ctx, execution)
			require.NoError(t, err)
			shouldAbandon := scenario == "orphan"
			wantAbandoned := 0
			if shouldAbandon {
				wantAbandoned = 1
				_, err = store.GetStream(ctx, "incarnation:g2")
				require.ErrorIs(t, err, journal.ErrStreamNotFound)
				retried, appendErr := store.Append(ctx, journal.Event{
					SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
					StreamID: "incarnation:g2", Identity: "orphan-event", Type: "usage_update", Payload: []byte(`{}`),
				})
				require.NoError(t, appendErr)
				require.EqualValues(t, 1, retried.Sequence)
				committed, commitErr := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
					SessionID: "session", IncarnationID: "incarnation", Generation: 2,
					PredecessorGeneration: 1, NativeSessionID: "native-two", CreationReason: "context_continued",
				}, 1)
				require.NoError(t, commitErr)
				require.True(t, committed)
			} else {
				stream, streamErr := store.GetStream(ctx, "incarnation:g2")
				require.NoError(t, streamErr)
				require.False(t, stream.Sealed)
			}
			require.Equal(t, wantAbandoned, abandoned)
		})
	}
}
