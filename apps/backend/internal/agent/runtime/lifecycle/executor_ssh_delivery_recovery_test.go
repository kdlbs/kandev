package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
