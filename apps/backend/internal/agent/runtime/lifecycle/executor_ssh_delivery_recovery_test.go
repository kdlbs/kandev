package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type sshDeliveryReconnector interface {
	ReconnectDeliveryInstance(context.Context, *ExecutorInstance) (*RemoteInstanceRefresh, error)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4
func TestSSHLiveDeliveryRecoveryReplacesLostTransport(t *testing.T) {
	rt, record, server := sshDeliveryRecoveryFixture(t)
	instances, _, err := rt.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Len(t, instances, 1)
	prior := instances[0]
	rt.mu.Lock()
	old := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	rt.transportTeardown(record.AgentExecutionID, old, "test_disconnect", 0)
	_, err = prior.Client.GetStatus(context.Background())
	require.Error(t, err)
	reconnector, ok := interface{}(rt).(sshDeliveryReconnector)
	require.True(t, ok, "SSH delivery recovery must restore its transport before reconciling")
	refresh, err := reconnector.ReconnectDeliveryInstance(context.Background(), prior)
	require.NoError(t, err)
	require.NotNil(t, refresh)
	t.Cleanup(refresh.Abort)
	require.False(t, refresh.ProcessRestarted)
	published := false
	require.NoError(t, refresh.Commit(func() { published = true }))
	require.True(t, published)
	require.Equal(t, prior.InstanceID, refresh.Instance.InstanceID)
	require.Equal(t, prior.WorkspacePath, refresh.Instance.WorkspacePath)
	status, err := refresh.Instance.Client.GetStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, "running", status.AgentStatus)
	again, err := reconnector.ReconnectDeliveryInstance(context.Background(), refresh.Instance)
	require.NoError(t, err)
	require.Nil(t, again, "a healthy recovered transport must be reused")
	for _, command := range server.commands() {
		require.False(t, strings.Contains(command, "kill ") || strings.Contains(command, "mkdir") || strings.Contains(command, "agentctl start"), command)
	}
}

func sshDeliveryRecoveryFixture(t *testing.T) (*SSHExecutor, *models.ExecutorRunning, *fakeSSHServer) {
	t.Helper()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer refreshed-auth-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/v1/instances/native-execution-id":
			if err := json.NewEncoder(w).Encode(agentctl.InstanceInfo{ID: "native-execution-id", Port: 41234, Status: "running", WorkspacePath: "/remote/task", SessionID: "session-ssh", TaskID: "task-ssh"}); err != nil {
				t.Error(err)
			}
		case "/api/v1/agent/delivery":
			if err := json.NewEncoder(w).Encode(agentctl.DeliveryStatus{StorageCapability: journal.StorageCapability{Version: 1, Durable: true}, SessionID: "session-ssh", IncarnationID: "incarnation-ssh", HarnessGeneration: 1, StreamID: "stream-ssh"}); err != nil {
				t.Error(err)
			}
		case "/api/v1/agent/submissions/prompt-ssh":
			if err := json.NewEncoder(w).Encode(journal.Submission{ID: "prompt-ssh", SessionID: "session-ssh", IncarnationID: "incarnation-ssh", HarnessGeneration: 1, StreamID: "stream-ssh", State: journal.SubmissionCompleted, TerminalEventRetained: true, TerminalSequence: 1}); err != nil {
				t.Error(err)
			}
		case "/api/v1/status":
			if err := json.NewEncoder(w).Encode(agentctl.StatusResponse{AgentStatus: "running"}); err != nil {
				t.Error(err)
			}
		case "/api/v1/workspace/stream":
			conn, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(control.Close)
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "ps -p 4242 -o command="):
			return sshExecResult{Stdout: "/opt/kandev/bin/agentctl --workdir /remote/task"}
		case command == "cat -- '/remote/session/agentctl.pid'":
			return sshExecResult{Stdout: "4242"}
		default:
			return sshExecResult{Stderr: "unexpected mutation or probe", ExitCode: 127}
		}
	})
	server.forwardTo(control.Listener.Addr().String())
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
	metadata[MetadataKeySSHRemoteControlPort] = "41235"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	metadata[MetadataKeySSHAgentctlInstanceID] = "native-execution-id"
	rt := NewSSHExecutor(nil, nil, nil, newTestLogger())
	t.Cleanup(func() { require.NoError(t, rt.Close()) })
	return rt, &models.ExecutorRunning{ID: "session-ssh", SessionID: "session-ssh", TaskID: "task-ssh", AgentExecutionID: "execution-ssh", Runtime: agentruntime.RuntimeSSH, PID: 4242, TransientAuthToken: "refreshed-auth-token", Metadata: metadata}, server
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4
func TestSSHLiveDeliveryRecoveryManagerReconcilesOriginalOwner(t *testing.T) {
	rt, record, _ := sshDeliveryRecoveryFixture(t)
	instances, _, err := rt.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Len(t, instances, 1)
	instance := instances[0]
	stale, err := instance.Client.StreamWorkspace(context.Background(), agentctl.WorkspaceStreamCallbacks{})
	require.NoError(t, err)
	t.Cleanup(func() { stale.Close(); stale.Wait() })
	rt.mu.Lock()
	old := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	rt.transportTeardown(record.AgentExecutionID, old, "test_disconnect", 0)
	mgr := newTestManager(t)
	mgr.executorRegistry = NewExecutorRegistry(newTestLogger())
	mgr.executorRegistry.Register(rt)
	mgr.secretStore = &mockSecretStore{store: map[string]string{"test-auth": "refreshed-auth-token"}}
	writer := &captureExecutorRunningWriter{prior: record}
	mgr.SetExecutorRunningWriter(writer)
	execution := &AgentExecution{ID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID, RuntimeName: record.Runtime,
		WorkspacePath: "/remote/task", ACPSessionID: "native-conversation-ssh", agentctl: instance.Client,
		DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "incarnation-ssh", DeliveryHarnessGeneration: 1, DeliveryStreamID: "stream-ssh", promptGeneration: 1}
	execution.metadata = cloneSSHMetadata(instance.Metadata)
	execution.setMetadataValue(MetadataKeyAuthTokenSecret, "test-auth")
	execution.setDeliverySubmissionID("prompt-ssh")
	execution.SetWorkspaceStream(stale)
	require.NoError(t, mgr.executionStore.Add(execution))
	clock := newReconciliationTestClock()
	mgr.streamManager.reconciliationNow = clock.Now
	mgr.streamManager.reconciliationWait = clock.Wait
	mgr.streamManager.deliverySubmissionSettler = func(_ context.Context, e *AgentExecution, id DeliveryReconciliationIdentity, s *journal.Submission) (bool, error) {
		require.Same(t, execution, e)
		require.Equal(t, "prompt-ssh", s.ID)
		require.Equal(t, "stream-ssh", id.StreamID)
		return s.TerminalEventRetained && s.State == journal.SubmissionCompleted, nil
	}
	identity := AgentDeliveryRecoveryIdentity{TaskID: record.TaskID, SessionID: record.SessionID, ExecutionID: record.AgentExecutionID, SubmissionID: "prompt-ssh", StreamID: "stream-ssh", IncarnationID: "incarnation-ssh", HarnessGeneration: 1, PromptGeneration: 1}
	result := mgr.RecoverAgentPromptStreamWithIdentity(context.Background(), identity)
	require.Equal(t, DeliveryReconciliationTerminalSettled, result.Outcome, "%+v", result)
	require.Equal(t, "native-conversation-ssh", execution.ACPSessionID)
	current, release := execution.AcquireAgentCtlClient()
	release()
	require.NotSame(t, instance.Client, current)
	require.Equal(t, execution.ID, writer.running.AgentExecutionID)
	require.Equal(t, "stream-ssh", execution.DeliveryStreamID)
	require.Eventually(t, func() bool { ws := execution.GetWorkspaceStream(); return ws != nil && ws != stale },
		5*time.Second, 10*time.Millisecond, "the workspace stream must follow the replacement transport")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHLiveDeliveryRecoveryAbsentExecutionKeepsRemoteOwnerGuarded(t *testing.T) {
	_, record, _ := sshDeliveryRecoveryFixture(t)
	mgr := newTestManager(t)
	mgr.executorRegistry = NewExecutorRegistry(newTestLogger())
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: record})
	task08SetSnapshots(mgr, []*models.ExecutorRunning{record})
	result := mgr.RecoverAgentPromptStreamWithIdentity(context.Background(), AgentDeliveryRecoveryIdentity{TaskID: record.TaskID, SessionID: record.SessionID, ExecutionID: record.AgentExecutionID, SubmissionID: "prompt-ssh", StreamID: "stream-ssh", IncarnationID: "incarnation-ssh", HarnessGeneration: 1, PromptGeneration: 1})
	require.Equal(t, DeliveryReconciliationTransportUnavailable, result.Outcome)
	require.True(t, mgr.HasPendingRemoteRecovery(record.SessionID, record.AgentExecutionID), "a saved SSH owner must remain guarded for remote recovery rather than falling back to the local journal")
	_, exists := mgr.GetExecutionBySessionID(record.SessionID)
	require.False(t, exists)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHLiveDeliveryRecoveryRejectsStaleCandidate(t *testing.T) {
	rt, record, _ := sshDeliveryRecoveryFixture(t)
	instances, _, err := rt.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Len(t, instances, 1)
	rt.mu.Lock()
	prior := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	rt.transportTeardown(record.AgentExecutionID, prior, "test_disconnect", 0)
	winner, err := rt.ReconnectDeliveryInstance(context.Background(), instances[0])
	require.NoError(t, err)
	require.NotNil(t, winner)
	t.Cleanup(winner.Abort)
	loser, err := rt.ReconnectDeliveryInstance(context.Background(), instances[0])
	require.NoError(t, err)
	require.NotNil(t, loser)
	t.Cleanup(loser.Abort)
	require.NoError(t, winner.Commit(func() {}))
	published := false
	require.ErrorIs(t, loser.Commit(func() { published = true }), ErrDeliveryOwnerMismatch)
	require.False(t, published)
	loser.Abort()
	_, err = winner.Instance.Client.GetStatus(context.Background())
	require.NoError(t, err, "loser cleanup must leave the winning owner connected")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHLiveDeliveryRecoveryCancellationAllowsLaterRetry(t *testing.T) {
	rt, record, _ := sshDeliveryRecoveryFixture(t)
	instances, _, err := rt.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Len(t, instances, 1)
	rt.mu.Lock()
	prior := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	rt.transportTeardown(record.AgentExecutionID, prior, "test_disconnect", 0)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	candidate, err := rt.ReconnectDeliveryInstance(ctx, instances[0])
	require.NoError(t, err)
	require.NotNil(t, candidate)
	t.Cleanup(candidate.Abort)
	cancel()
	published := false
	require.ErrorIs(t, candidate.Commit(func() { published = true }), context.Canceled)
	require.False(t, published)
	candidate.Abort()
	retry, err := rt.ReconnectDeliveryInstance(context.Background(), instances[0])
	require.NoError(t, err)
	require.NotNil(t, retry)
	t.Cleanup(retry.Abort)
	require.NoError(t, retry.Commit(func() {}))
	_, err = retry.Instance.Client.GetStatus(context.Background())
	require.NoError(t, err)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHLiveDeliveryRecoveryRejectsChangedRemoteOwner(t *testing.T) {
	rt, record, _ := sshDeliveryRecoveryFixture(t)
	instances, _, err := rt.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Len(t, instances, 1)
	rt.mu.Lock()
	prior := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	rt.transportTeardown(record.AgentExecutionID, prior, "test_disconnect", 0)
	foreign := *instances[0]
	foreign.SessionID = "foreign-session"
	candidate, err := rt.ReconnectDeliveryInstance(context.Background(), &foreign)
	require.Error(t, err)
	require.Nil(t, candidate)
	rt.mu.Lock()
	current := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	require.Same(t, prior, current)
	retry, err := rt.ReconnectDeliveryInstance(context.Background(), instances[0])
	require.NoError(t, err)
	require.NotNil(t, retry)
	t.Cleanup(retry.Abort)
	require.NoError(t, retry.Commit(func() {}))
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHLiveDeliveryRecoveryCallbackDoesNotBlockExecutor(t *testing.T) {
	rt, record, _ := sshDeliveryRecoveryFixture(t)
	instances, _, err := rt.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Len(t, instances, 1)
	original := instances[0]
	rt.mu.Lock()
	prior := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	rt.transportTeardown(record.AgentExecutionID, prior, "test_disconnect", 0)
	refresh, err := rt.ReconnectDeliveryInstance(context.Background(), original)
	require.NoError(t, err)
	require.NotNil(t, refresh)
	t.Cleanup(refresh.Abort)
	execution := &AgentExecution{ID: record.AgentExecutionID, SessionID: record.SessionID, agentctl: original.Client}
	manager := newTestManager(t)
	publishing := make(chan struct{})
	commit := refresh.Commit
	refresh.Commit = func(publish func()) error {
		return commit(func() { close(publishing); publish() })
	}
	execution.agentctlSourceMu.RLock()
	done := make(chan error, 1)
	go func() {
		_, err := manager.commitTrackedRemoteRefresh(context.Background(), execution, refresh, "", func(context.Context) error { return nil })
		done <- err
	}()
	select {
	case <-publishing:
	case <-time.After(100 * time.Millisecond):
	}
	available := rt.mu.TryLock()
	if available {
		rt.mu.Unlock()
	}
	execution.agentctlSourceMu.RUnlock()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not finish after callback release")
	}
	require.True(t, available, "a pinned workspace callback must not stall other SSH executor operations")
	client, release := execution.AcquireAgentCtlClient()
	release()
	require.Same(t, refresh.Instance.Client, client)
	rt.mu.Lock()
	current := rt.sessions[record.AgentExecutionID]
	rt.mu.Unlock()
	require.NotSame(t, prior, current)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
func TestSSHLiveDeliveryRecoveryFailedPublicationRestoresURL(t *testing.T) {
	original := agentctl.NewClient("127.0.0.1", 12345, newTestLogger())
	candidate := agentctl.NewClient("127.0.0.1", 12346, newTestLogger())
	execution := &AgentExecution{ID: "original-execution", agentctl: original}
	refresh := &RemoteInstanceRefresh{Instance: &ExecutorInstance{Client: candidate}, Commit: func(func()) error { return ErrDeliveryOwnerMismatch }}
	manager := newTestManager(t)
	committed, err := manager.commitTrackedRemoteRefresh(context.Background(), execution, refresh, "", func(context.Context) error { return nil })
	require.ErrorIs(t, err, ErrDeliveryOwnerMismatch)
	require.False(t, committed)
	execution.agentctlSourceMu.Lock()
	url := execution.AgentctlURL()
	execution.agentctlSourceMu.Unlock()
	require.Equal(t, original.BaseURL(), url, "a losing refresh must not persist the discarded candidate URL")
	client, release := execution.AcquireAgentCtlClient()
	release()
	require.Same(t, original, client)
}
