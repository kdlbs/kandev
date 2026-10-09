package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExecutorDisconnectUnsupportedRuntimeKeepsExistingPath(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeSSH, agentruntime.RuntimeSprites, agentruntime.RuntimePluginRemote} {
		t.Run(string(runtime), func(t *testing.T) {
			mgr := newRemoteStatusManager(t, &statusProviderExecutor{MockExecutor: MockExecutor{name: runtime}, status: &RemoteStatus{State: "running"}})
			mgr.SetExecutorObservationHandler(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) error { return nil })
			execution := &AgentExecution{ID: "execution", SessionID: "session", RuntimeName: runtime}
			execution.setSessionInitialized(true)
			require.False(t, mgr.inspectManagedDisconnect(execution, 0, 0))
		})
	}
}

func TestExecutorDeletingPodRemainsStoppingWhenContainersNotReady(t *testing.T) {
	now := metav1.NewTime(time.Now())
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}, Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"}}}}
	state, _, ready, _, _, _ := kubernetesRemotePodState(pod, "agent")
	require.Equal(t, "stopping", state)
	require.False(t, ready)
}

func TestExecutorDockerNotFoundIsMissingButAPIFailureIsUnknown(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.Header().Set("API-Version", "1.54")
					w.WriteHeader(http.StatusOK)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"message":"fixture failure"}`))
			}))
			t.Cleanup(daemon.Close)
			client, err := docker.NewClient(config.DockerConfig{Host: daemon.URL}, newResolverTestLogger(t))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			status, err := dockerExecutorRemoteStatus(t.Context(), client, &ExecutorInstance{TaskID: "task", ContainerID: "owned", RuntimeName: agentruntime.RuntimeDocker}, "docker")
			if code == http.StatusNotFound {
				require.NoError(t, err)
				require.NotNil(t, status)
				require.Equal(t, models.ExecutorOutcomeMissing, status.Observation.Outcome)
				require.Equal(t, "owned", status.Observation.ResourceKey)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestExecutorRemoteDockerRetainedInspectionUsesRecordedConnection(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("API-Version", "1.54")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":"owned","Config":{"Labels":{"kandev.task_id":"task"}},"State":{"Status":"running","Running":true}}`))
	}))
	t.Cleanup(daemon.Close)
	client, err := docker.NewClient(config.DockerConfig{Host: daemon.URL}, newResolverTestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	remote := NewRemoteDockerExecutor(newResolverTestLogger(t))
	connected := false
	remote.connect = func(_ context.Context, req *ExecutorCreateRequest) (*remoteDockerSession, error) {
		connected = true
		require.Equal(t, "worker.example.invalid", getMetadataString(req.Metadata, MetadataKeySSHHost))
		return &remoteDockerSession{dockerClient: client}, nil
	}
	status, err := remote.GetRemoteStatus(t.Context(), &ExecutorInstance{InstanceID: "environment", TaskID: "task", ContainerID: "owned", RuntimeName: agentruntime.RuntimeRemoteDocker, Metadata: map[string]any{MetadataKeySSHHost: "worker.example.invalid"}})
	require.NoError(t, err)
	require.True(t, connected)
	require.NotNil(t, status.Observation)
	require.Equal(t, models.ExecutorOutcomeHealthy, status.Observation.Outcome)
	require.Empty(t, remote.sessions, "inspection must not register a launch or recovery session")
}

func TestExecutorDisconnectMissingAuthorityRetainsUnverifiedStatus(t *testing.T) {
	mgr := newRemoteStatusManager(t, &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{State: "running"}})
	calls := 0
	mgr.SetExecutorObservationHandler(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) error {
		calls++
		return nil
	})
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", TaskEnvironmentID: "unavailable-authority", RuntimeName: agentruntime.RuntimeKubernetes, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	require.NoError(t, mgr.executionStore.Add(execution))
	mgr.classifyExecutorDisconnect(execution, 0, 0)
	status, ok := mgr.GetRemoteStatusBySession("session")
	require.True(t, ok)
	require.Equal(t, models.ExecutorOutcomeUnknown, status.State)
	require.Zero(t, calls)
	require.Empty(t, execution.promptDoneCh, "missing authority cannot prove process death or successful completion")
}

func TestExecutorFailureUnverifiedLaunchDoesNotClaimLiveAgent(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: models.ExecutorOutcomeUnknown, ResourceKey: "pod", ObservedAt: time.Now().UTC()}}}
	mgr := newRemoteStatusManager(t, provider)
	mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeKubernetes}
	execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
	err := mgr.existingExecutorLaunchError(t.Context(), execution)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrAgentAlreadyRunning, "unverified inventory must not authorize duplicate cleanup or prompt queuing")
	var unavailable *ExecutorUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Equal(t, "StatusUnverified", unavailable.Observation.Reason)
}

func TestExecutorFailureUnverifiedAuthorityLaunchDoesNotClaimLiveAgent(t *testing.T) {
	mgr := newRemoteStatusManager(t, &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}})
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", TaskEnvironmentID: "unavailable-authority", RuntimeName: agentruntime.RuntimeKubernetes}
	err := mgr.existingExecutorLaunchError(t.Context(), execution)
	require.NotErrorIs(t, err, ErrAgentAlreadyRunning, "unreadable ownership cannot prove a live agent")
	var unavailable *ExecutorUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Equal(t, "StatusUnverified", unavailable.Observation.Reason)
}

func TestExecutorFailureTerminalLocalAgentKeepsControllerAuthorityForRecovery(t *testing.T) {
	for _, outcome := range []string{models.ExecutorOutcomeHealthy, models.ExecutorOutcomeTerminated, models.ExecutorOutcomeUnknown} {
		t.Run(outcome, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.standaloneHostPID.Store(42)
			mgr.SetExecutorRunningWriter(&synchronizedRunningWriter{running: &models.ExecutorRunning{TaskID: "task", SessionID: "session", Runtime: agentruntime.RuntimeStandalone, AgentExecutionID: "execution", Status: models.ExecutorRunningStatusFailed, UpdatedAt: time.Now().UTC()}})
			mgr.SetLocalExecutorInspector(func(target models.ExecutorObservationTarget) *models.ExecutorObservation {
				require.EqualValues(t, 42, target.LocalPID)
				require.Equal(t, "local-pid:42", target.ResourceKey)
				return &models.ExecutorObservation{Outcome: outcome, ResourceKey: target.ResourceKey}
			})
			execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", RuntimeName: agentruntime.RuntimeStandalone, Status: v1.AgentStatusFailed}
			err := mgr.existingExecutorLaunchError(t.Context(), execution)
			if outcome == models.ExecutorOutcomeHealthy {
				require.ErrorIs(t, err, ErrAgentAlreadyRunning, "healthy controller permits the existing explicit stale-agent cleanup path")
			} else {
				require.NotErrorIs(t, err, ErrAgentAlreadyRunning, "unknown or lost controller must still block cleanup/relaunch")
				var unavailable *ExecutorUnavailableError
				require.ErrorAs(t, err, &unavailable)
			}
		})
	}
}

func TestExecutorFailureLocalControllerFallbackRejectsLiveOrRotatedAgentRows(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       v1.AgentStatus
		rowExecution string
		runtime      agentruntime.Runtime
	}{{"live_agent", v1.AgentStatusReady, "execution", agentruntime.RuntimeStandalone}, {"rotated_agent", v1.AgentStatusFailed, "replacement", agentruntime.RuntimeStandalone}, {"remote_row", v1.AgentStatusFailed, "execution", agentruntime.RuntimeKubernetes}} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.standaloneHostPID.Store(42)
			mgr.SetExecutorRunningWriter(&synchronizedRunningWriter{running: &models.ExecutorRunning{TaskID: "task", SessionID: "session", Runtime: tc.runtime, AgentExecutionID: tc.rowExecution, Status: models.ExecutorRunningStatusFailed, UpdatedAt: time.Now().UTC()}})
			mgr.SetLocalExecutorInspector(func(models.ExecutorObservationTarget) *models.ExecutorObservation {
				t.Fatal("unowned controller evidence must not be read")
				return nil
			})
			err := mgr.existingExecutorLaunchError(t.Context(), &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", RuntimeName: agentruntime.RuntimeStandalone, Status: tc.status})
			require.NotErrorIs(t, err, ErrAgentAlreadyRunning)
			var unavailable *ExecutorUnavailableError
			require.ErrorAs(t, err, &unavailable)
		})
	}
}
