package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentruntime"
	lifecycleevents "github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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
	var updateDialCount atomic.Int32
	var recoveryAttemptCount atomic.Int32
	var canceledCandidateClient atomic.Pointer[agentctl.Client]
	updatesDialStarted := make(chan struct{}, 2)
	updatesDialCanceled := make(chan struct{}, 1)
	firstHandshakeFinished := make(chan struct{})
	connectedUpdateStream := make(chan *websocket.Conn, 1)
	releaseFirstHandshake := make(chan struct{})
	var releaseFirstOnce sync.Once
	allowSecondUpdateDial := make(chan struct{})
	var allowSecondOnce sync.Once
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/stream":
			dial := updateDialCount.Add(1)
			updatesDialStarted <- struct{}{}
			if dial == 1 {
				select {
				case <-r.Context().Done():
					updatesDialCanceled <- struct{}{}
				case <-releaseFirstHandshake:
				}
				<-releaseFirstHandshake
				defer close(firstHandshakeFinished)
				conn, upgradeErr := upgrader.Upgrade(w, r, nil)
				if upgradeErr != nil {
					return
				}
				_ = conn.Close()
				return
			}
			select {
			case <-allowSecondUpdateDial:
			case <-r.Context().Done():
				return
			}
			conn, upgradeErr := upgrader.Upgrade(w, r, nil)
			if upgradeErr != nil {
				return
			}
			connectedUpdateStream <- conn
			for {
				if _, _, readErr := conn.ReadMessage(); readErr != nil {
					_ = conn.Close()
					return
				}
			}
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
	mcpPath := filepath.Join(t.TempDir(), ".pi", "mcp.json")
	mcpContent := []byte(`{"mcpServers":{"kandev":{"url":"http://localhost:41006/mcp"}}}`)
	require.NoError(t, os.MkdirAll(filepath.Dir(mcpPath), 0o700))
	require.NoError(t, os.WriteFile(mcpPath, mcpContent, 0o600))
	assertOwnedMCPConfigRetained := func(message string) {
		t.Helper()
		got, readErr := os.ReadFile(mcpPath)
		require.NoError(t, readErr, message)
		require.Equal(t, mcpContent, got, message)
		require.Equal(t, passthroughMCPFingerprint(mcpContent), passthroughMCPFingerprint(got), message)
	}
	row.Metadata = map[string]interface{}{
		MetadataKeyOriginalWorkspacePath: "/workspace",
		metadataKeyPassthroughMCPClaims: []interface{}{map[string]interface{}{
			"path": mcpPath, "fingerprint": passthroughMCPFingerprint(mcpContent), "owned": true,
		}},
	}
	writer := &task08ExecutorInventoryWriter{
		invariantWriter: invariantWriter{prior: row},
		rows:            []*models.ExecutorRunning{row},
	}
	var attemptedOwnersMu sync.Mutex
	var attemptedOwners []*models.ExecutorRunning
	backend := &task08DetailedRecoveryBackend{MockExecutor: &MockExecutor{name: "sprites"}}
	backend.recoveryResult = func(records []*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
		if len(records) != 1 {
			t.Errorf("recovery attempted with %d owner records, want one", len(records))
		} else {
			attemptedOwnersMu.Lock()
			attemptedOwners = append(attemptedOwners, cloneRemoteRecoveryRecord(records[0]))
			attemptedOwnersMu.Unlock()
		}
		client := agentctl.NewClient(host, port, newTestLogger())
		if recoveryAttemptCount.Add(1) == 2 {
			canceledCandidateClient.Store(client)
		}
		return []*ExecutorInstance{{
			InstanceID: row.AgentExecutionID, TaskID: row.TaskID, SessionID: row.SessionID,
			RuntimeName: row.Runtime, WorkspacePath: "/workspace", AgentProfileID: row.ExecutionProfileID,
			Client: client, Metadata: cloneRemoteRecoveryRecord(row).Metadata,
		}}, nil, nil
	}
	mgr, eventBus := task08ManagerWithBackend(t, backend)
	mgr.SetExecutorRunningWriter(writer)
	provider := &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{
		sessionID: task08RemoteSnapshot(row),
	}}
	mgr.workspaceInfoProvider = provider
	repository := &remoteRecoveryReplayRepository{}
	mgr.streamManager.setAgentDeliveryRepository(repository)
	entry := mgr.queueRemoteRecovery(row, provider.snapshots[sessionID])
	require.NotNil(t, entry)
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
	var cancelAttempt context.CancelFunc
	var connectedUpdateConn *websocket.Conn
	t.Cleanup(func() {
		if cancelAttempt != nil {
			cancelAttempt()
		}
		releaseFirstOnce.Do(func() { close(releaseFirstHandshake) })
		allowSecondOnce.Do(func() { close(allowSecondUpdateDial) })
		if connectedUpdateConn == nil {
			select {
			case connectedUpdateConn = <-connectedUpdateStream:
			default:
			}
		}
		if connectedUpdateConn != nil {
			_ = connectedUpdateConn.Close()
		}
		if execution, exists := mgr.executionStore.Get(row.AgentExecutionID); exists {
			if client, releaseClient := execution.AcquireAgentCtlClient(); client != nil {
				releaseClient()
				client.Close()
			}
		}
	})

	var availabilityPayload atomic.Pointer[AgentEventPayload]
	var availabilityCount atomic.Int32
	eventBus.OnPublish = func(subject string, event *bus.Event) {
		if subject != lifecycleevents.AgentBootReady {
			return
		}
		payload, ok := event.Data.(AgentEventPayload)
		if !ok {
			t.Errorf("recovered availability event payload has type %T", event.Data)
			return
		}
		if mgr.pendingRemoteRecoveryForSession(sessionID) != nil {
			t.Error("recovered availability was published while remote admission remained fenced")
		}
		if err := mgr.recoveryGuard.CheckLaunchAllowed(sessionID); err != nil {
			t.Errorf("recovered availability was published before releasing the launch guard: %v", err)
		}
		current, exists := mgr.executionStore.GetBySessionID(sessionID)
		if !exists || current.ID != row.AgentExecutionID {
			t.Error("recovered availability was published without the exact adopted execution")
		} else if client, releaseClient := current.AcquireAgentCtlClient(); client == nil {
			t.Error("recovered availability was published without the adopted agentctl client")
		} else {
			if !client.HasAgentStream() {
				t.Error("recovered availability was published before the updates stream connected")
			}
			releaseClient()
		}
		availabilityCount.Add(1)
		availabilityPayload.Store(&payload)
	}

	mgr.retryRemoteRecovery(context.Background(), entry)
	_, tracked := mgr.executionStore.Get(row.AgentExecutionID)
	require.False(t, tracked, "a timed out partial replay must release the temporary execution")
	require.Same(t, entry, mgr.pendingRemoteRecoveryForSession(sessionID),
		"a partial replay timeout must preserve the retry owner and launch guard")
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(sessionID), ErrSessionRecoveryGuarded)
	assertOwnedMCPConfigRetained("a temporary replay failure must not revoke or replace the retained remote owner's MCP config")
	require.Equal(t, row.Metadata, entry.record.Metadata, "the pending recovery record keeps its original MCP claim")
	require.Equal(t, row.Metadata, writer.prior.Metadata, "failed adoption must preserve persisted owner metadata")
	require.Equal(t, int64(1), repository.cursor.ProjectedSequence,
		"the first event must remain committed so the next adoption can resume after it")

	attemptCtx, cancel := context.WithCancel(context.Background())
	cancelAttempt = cancel
	firstAttemptDone := make(chan struct{})
	go func() {
		mgr.retryRemoteRecovery(attemptCtx, entry)
		close(firstAttemptDone)
	}()
	select {
	case <-updatesDialStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("idle recovery did not start the updates-stream dial")
	}
	_, tracked = mgr.executionStore.Get(row.AgentExecutionID)
	require.True(t, tracked, "the candidate stays registered while its stream handshake is pending")
	require.Same(t, entry, mgr.pendingRemoteRecoveryForSession(sessionID),
		"the owner entry stays pending until the updates stream is attached")
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(sessionID), ErrSessionRecoveryGuarded)
	require.Nil(t, availabilityPayload.Load(), "idle recovery must not publish availability before the stream connects")
	cancelAttempt()
	select {
	case <-firstAttemptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled stream dial did not return from remote recovery")
	}
	select {
	case <-updatesDialCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("discarding the canceled candidate did not cancel its updates-stream handshake")
	}
	releaseFirstOnce.Do(func() { close(releaseFirstHandshake) })
	select {
	case <-firstHandshakeFinished:
	case <-time.After(2 * time.Second):
		t.Fatal("withheld canceled handshake did not finish after the test released it")
	}
	canceledClient := canceledCandidateClient.Load()
	require.NotNil(t, canceledClient, "the canceled recovery attempt must have its own retained client")
	require.False(t, canceledClient.HasAgentStream(),
		"a late server-side upgrade after cancellation must not register a client stream")
	_, tracked = mgr.executionStore.Get(row.AgentExecutionID)
	require.False(t, tracked, "a canceled handshake must remove the temporary candidate")
	require.Same(t, entry, mgr.pendingRemoteRecoveryForSession(sessionID),
		"a canceled handshake must preserve the retry owner and launch guard")
	assertOwnedMCPConfigRetained("canceling a temporary attachment must preserve the remote owner's MCP config bytes")
	require.Equal(t, row.Metadata, writer.prior.Metadata, "canceled adoption must preserve persisted owner metadata")
	require.ErrorIs(t, mgr.recoveryGuard.CheckLaunchAllowed(sessionID), ErrSessionRecoveryGuarded)
	require.Nil(t, availabilityPayload.Load(), "a canceled handshake must not publish availability")
	require.Zero(t, availabilityCount.Load(), "a canceled handshake must not publish availability")

	allowSecondOnce.Do(func() { close(allowSecondUpdateDial) })
	mgr.retryRemoteRecovery(context.Background(), entry)
	select {
	case connectedUpdateConn = <-connectedUpdateStream:
	case <-time.After(2 * time.Second):
		t.Fatal("successful retry did not establish its updates stream")
	}

	_, tracked = mgr.executionStore.Get(row.AgentExecutionID)
	require.True(t, tracked, "a later authenticated attachment must finish replay and become tracked")
	require.Nil(t, mgr.pendingRemoteRecoveryForSession(sessionID), "successful replay completes the pending recovery")
	require.NoError(t, mgr.recoveryGuard.CheckLaunchAllowed(sessionID))
	require.Equal(t, []uint64{0, 1}, replayAfter,
		"the second attachment must resume from the SQL cursor after the partially projected prefix")
	require.Equal(t, int64(2), repository.cursor.ProjectedSequence)
	gotAvailability := availabilityPayload.Load()
	require.NotNil(t, gotAvailability, "idle durable recovery must publish an availability-only boot event")
	require.EqualValues(t, 1, availabilityCount.Load(), "recovery publishes availability exactly once")
	require.True(t, gotAvailability.RecoveredAvailabilityOnly)
	require.Equal(t, row.AgentExecutionID, gotAvailability.AgentExecutionID)
	require.Equal(t, string(v1.AgentStatusReady), gotAvailability.Status)
	require.EqualValues(t, 2, updateDialCount.Load(),
		"the canceled attempt must not register or publish a late socket before the successful retry")
	require.EqualValues(t, 3, recoveryAttemptCount.Load(), "recovery retries exactly once after the canceled attempt")
	attemptedOwnersMu.Lock()
	owners := append([]*models.ExecutorRunning(nil), attemptedOwners...)
	attemptedOwnersMu.Unlock()
	require.Len(t, owners, 3, "the same persisted owner is retried after both temporary failures")
	for _, attempted := range owners {
		require.Equal(t, row.SessionID, attempted.SessionID)
		require.Equal(t, row.TaskID, attempted.TaskID)
		require.Equal(t, row.AgentExecutionID, attempted.AgentExecutionID)
		require.Equal(t, row.Metadata, attempted.Metadata)
	}
	assertOwnedMCPConfigRetained("successful adoption leaves the live owner's MCP config bytes in place")
	require.Equal(t, row.Metadata, writer.prior.Metadata, "successful retry persists the unchanged MCP claim")
	for _, event := range eventBus.PublishedEvents {
		require.NotEqual(t, lifecycleevents.AgentReady, event.Type,
			"idle retained recovery must not publish a turn-completion event")
	}

	execution, exists := mgr.executionStore.Get(row.AgentExecutionID)
	require.True(t, exists)
	mgr.cleanupPassthroughMCPConfig(execution)
	_, err = os.Stat(mcpPath)
	require.ErrorIs(t, err, os.ErrNotExist, "ordinary owner cleanup can still revoke its owned project config")
}

func TestRemoveFailedRemoteReplayFencesStaleCandidates(t *testing.T) {
	const (
		sessionID   = "session-stale-replay-candidate"
		taskID      = "task-stale-replay-candidate"
		executionID = "execution-stale-replay-candidate"
	)

	for _, testCase := range []struct {
		name              string
		stalePendingEntry bool
		staleCandidate    bool
	}{
		{name: "pending generation changed", stalePendingEntry: true},
		{name: "tracked candidate replaced", staleCandidate: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mgr, _ := task08ManagerWithBackend(t, &MockExecutor{name: "sprites"})
			row := &models.ExecutorRunning{
				SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
			}
			entry := &remoteRecoveryRetry{record: row}
			currentEntry := entry
			if testCase.stalePendingEntry {
				currentEntry = &remoteRecoveryRetry{record: cloneRemoteRecoveryRecord(row)}
			}
			staleCandidate := &AgentExecution{ID: executionID, SessionID: sessionID, TaskID: taskID}
			trackedCandidate := staleCandidate
			if testCase.staleCandidate {
				trackedCandidate = &AgentExecution{ID: executionID, SessionID: sessionID, TaskID: taskID}
			}
			require.NoError(t, mgr.executionStore.Add(trackedCandidate))
			mgr.remoteRecoveryPending[sessionID] = currentEntry

			streamCtx, cancelStream := context.WithCancel(context.Background())
			defer cancelStream()
			var cancelCalls atomic.Int32
			cycle := &deliveryReconciliationCycle{
				identity: DeliveryReconciliationIdentity{SessionID: sessionID, ExecutionID: executionID},
				ctx:      streamCtx,
				done:     make(chan struct{}),
				cancel: func() {
					cancelCalls.Add(1)
					cancelStream()
				},
			}
			mgr.streamManager.reconciliationMu.Lock()
			mgr.streamManager.deliveryReconciliations[executionID] = cycle
			mgr.streamManager.reconciliationMu.Unlock()
			t.Cleanup(func() {
				mgr.streamManager.reconciliationMu.Lock()
				if mgr.streamManager.deliveryReconciliations[executionID] == cycle {
					delete(mgr.streamManager.deliveryReconciliations, executionID)
					close(cycle.done)
				}
				mgr.streamManager.reconciliationMu.Unlock()
				cancelStream()
			})

			mgr.removeFailedRemoteReplay(entry, staleCandidate)

			got, exists := mgr.executionStore.Get(executionID)
			require.True(t, exists, "a stale replay failure must not untrack the current execution")
			require.Same(t, trackedCandidate, got, "a stale replay failure must preserve the current candidate")
			select {
			case <-streamCtx.Done():
				t.Fatal("a stale replay failure must not cancel successor reconciliation")
			default:
			}
			require.Zero(t, cancelCalls.Load(), "a stale replay failure must not cancel successor reconciliation")
			mgr.streamManager.reconciliationMu.Lock()
			_, stopped := mgr.streamManager.stoppedReconciliations[executionID]
			mgr.streamManager.reconciliationMu.Unlock()
			require.False(t, stopped, "a stale replay failure must not fence successor reconciliation")
		})
	}
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
