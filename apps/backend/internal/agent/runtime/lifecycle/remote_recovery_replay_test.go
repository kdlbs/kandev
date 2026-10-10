package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type remoteRecoveryReplayRepository struct {
	recordingAgentDeliveryRepository
	failedSecondProjection bool
}

func (*remoteRecoveryReplayRepository) GetCurrentHarnessSessionGeneration(
	context.Context,
	string,
	string,
) (*models.HarnessSessionGeneration, error) {
	return &models.HarnessSessionGeneration{
		SessionID: "session-replay-retry", IncarnationID: "incarnation-replay-retry",
		Generation: 3, NativeSessionID: "native-replay-retry",
	}, nil
}

func (*remoteRecoveryReplayRepository) ListAgentDeliverySubmissions(
	context.Context,
	string,
) ([]*models.AgentDeliverySubmission, error) {
	return nil, nil
}

func (r *remoteRecoveryReplayRepository) ProjectAgentDeliveryEvent(
	ctx context.Context,
	event *models.AgentDeliveryEvent,
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	if event.Sequence == 2 && !r.failedSecondProjection {
		r.failedSecondProjection = true
		return false, context.DeadlineExceeded
	}
	return r.recordingAgentDeliveryRepository.ProjectAgentDeliveryEvent(ctx, event, effect)
}

func TestRemoteRecoveryRetriesAfterPartialReplayTimeout(t *testing.T) {
	const sessionID = "session-replay-retry"
	stream := journal.Stream{
		SessionID: sessionID, IncarnationID: "incarnation-replay-retry", HarnessGeneration: 3,
		StreamID: "stream-replay-retry", HighWater: 2, FirstRetained: 1,
	}
	events := []journal.Event{
		remoteReplayEvent(t, stream, 1, "first"),
		remoteReplayEvent(t, stream, 2, "second"),
	}
	var replayAfter []uint64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/session":
			_ = json.NewEncoder(w).Encode(agentctl.AgentSessionAssociation{
				InstanceID: "execution-replay-retry", SessionID: sessionID,
				IncarnationID: stream.IncarnationID, HarnessGeneration: stream.HarnessGeneration,
				NativeSessionID: "native-replay-retry", AgentStatus: agentProcessStatusRunning,
			})
		case "/api/v1/agent/delivery":
			_ = json.NewEncoder(w).Encode(agentctl.DeliveryStatus{
				StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
				SessionID:         sessionID, IncarnationID: stream.IncarnationID,
				HarnessGeneration: stream.HarnessGeneration, StreamID: stream.StreamID, Stream: &stream,
			})
		case "/api/v1/agent/delivery/stream":
			after, parseErr := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
			if parseErr != nil {
				t.Errorf("parse replay cursor: %v", parseErr)
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
			replayAfter = append(replayAfter, after)
			page := make([]journal.Event, 0, len(events))
			for _, event := range events {
				if event.Sequence > after {
					page = append(page, event)
				}
			}
			_ = json.NewEncoder(w).Encode(struct {
				Events []journal.Event `json:"events"`
				Stream journal.Stream  `json:"stream"`
			}{Events: page, Stream: stream})
		case "/api/v1/agent/delivery/stream/ack":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	row := &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: "task-replay-retry",
		AgentExecutionID: "execution-replay-retry", Runtime: agentruntime.RuntimeSprites,
		ExecutionProfileID: "profile-replay-retry", Status: models.ExecutorRunningStatusReady,
	}
	backend := &task08DetailedRecoveryBackend{MockExecutor: &MockExecutor{name: "sprites"}}
	backend.recoveryResult = func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
		return []*ExecutorInstance{{
			InstanceID: row.AgentExecutionID, TaskID: row.TaskID, SessionID: row.SessionID,
			RuntimeName: row.Runtime, WorkspacePath: "/workspace", AgentProfileID: row.ExecutionProfileID,
			Client: agentctl.NewClient(host, port, newTestLogger()),
		}}, nil, nil
	}
	mgr, _ := task08ManagerWithBackend(t, backend)
	provider := &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
		sessionID: task08RemoteSnapshot(row),
	}}
	mgr.workspaceInfoProvider = provider
	repository := &remoteRecoveryReplayRepository{}
	mgr.streamManager.setAgentDeliveryRepository(repository)
	entry := mgr.queueRemoteRecovery(row, provider.snapshots[sessionID])
	require.NotNil(t, entry)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })

	mgr.retryRemoteRecovery(context.Background(), entry)
	_, tracked := mgr.executionStore.Get(row.AgentExecutionID)
	require.False(t, tracked, "a timed out partial replay must release the temporary execution")
	require.Same(t, entry, mgr.pendingRemoteRecoveryForSession(sessionID),
		"a partial replay timeout must preserve the retry owner and launch guard")
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(sessionID), ErrSessionRecoveryGuarded)
	require.Equal(t, int64(1), repository.cursor.ProjectedSequence,
		"the first event must remain committed so the next adoption can resume after it")

	mgr.retryRemoteRecovery(context.Background(), entry)

	_, tracked = mgr.executionStore.Get(row.AgentExecutionID)
	require.True(t, tracked, "a later authenticated attachment must finish replay and become tracked")
	require.Nil(t, mgr.pendingRemoteRecoveryForSession(sessionID), "successful replay completes the pending recovery")
	require.NoError(t, mgr.recoveryGuard.CheckLaunchAllowed(sessionID))
	require.Equal(t, []uint64{0, 1}, replayAfter,
		"the second attachment must resume from the SQL cursor after the partially projected prefix")
	require.Equal(t, int64(2), repository.cursor.ProjectedSequence)
}

func remoteReplayEvent(t *testing.T, stream journal.Stream, sequence uint64, text string) journal.Event {
	t.Helper()
	payload, err := json.Marshal(agentctl.AgentEvent{Type: "plan", Text: text})
	require.NoError(t, err)
	return journal.Event{
		SessionID: stream.SessionID, IncarnationID: stream.IncarnationID,
		HarnessGeneration: stream.HarnessGeneration, StreamID: stream.StreamID,
		Sequence: sequence, Type: "plan", Payload: payload,
	}
}
