package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.1
func TestExecutorFailurePreservesPodEviction(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodFailed, Reason: "Evicted", Message: `Usage of EmptyDir volume "docker-data" exceeds the limit "12Gi"`,
		ContainerStatuses: []corev1.ContainerStatus{{Name: "kandev-agent", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Reason: "Error"}}}},
	}}
	state, _, _, _, reason, message := kubernetesRemotePodState(pod, "kandev-agent")
	require.Equal(t, "failed", state)
	require.Equal(t, "Evicted", reason)
	require.Contains(t, message, "12Gi")
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.12
func TestExecutorFailureRunningPodCrashLoop(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
		{Name: "docker", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
		{Name: "kandev-agent", RestartCount: 126, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 2, Reason: "Error"}}},
	}}}
	state, _, ready, restarts, reason, _ := kubernetesRemotePodState(pod, "kandev-agent")
	require.Equal(t, "failed", state)
	require.False(t, ready)
	require.EqualValues(t, 126, restarts)
	require.Equal(t, "CrashLoopBackOff", reason)
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.6
func TestExecutorFailureRejectsEvictedPodBeforeReconnect(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	execs := &recordingKubernetesExec{}
	executor := newFakeKubernetesExecutor(t, resources, execs, map[uint16]uint16{8765: controlPort, 41001: instancePort})
	created, err := executor.CreateInstance(t.Context(), validKubernetesCreateRequest())
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod.Status.Phase = corev1.PodFailed
	resources.pod.Status.Reason = "Evicted"
	resources.pod.Status.Message = `Usage of EmptyDir volume "docker-data" exceeds the limit "12Gi"`
	resources.mu.Unlock()
	execs.mu.Lock()
	execs.requests = nil
	execs.mu.Unlock()
	_, err = executor.CreateInstance(t.Context(), kubernetesReconnectRequest(created))
	require.ErrorContains(t, err, "Evicted")
	var unavailable interface {
		ExecutorObservation() *models.ExecutorObservation
	}
	require.True(t, errors.As(err, &unavailable), "reconnect error must retain typed primary resource evidence")
	require.Equal(t, "Evicted", unavailable.ExecutorObservation().Reason)
	execs.mu.Lock()
	defer execs.mu.Unlock()
	require.Empty(t, execs.requests)
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.1, AC-EXECUTORS-FAILURE-VISIBILITY-001.10
func TestExecutorFailureEvidenceSurvivesStatusSerialization(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{8765: controlPort, 41001: instancePort})
	created, err := executor.CreateInstance(t.Context(), validKubernetesCreateRequest())
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod.Status.Phase = corev1.PodFailed
	resources.pod.Status.Reason = "Evicted"
	resources.pod.Status.Message = `Usage of EmptyDir volume "docker-data" exceeds the limit "12Gi" token=supersecret`
	resources.pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{Name: "kandev-agent", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Reason: "Error"}}},
		{Name: "docker", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Reason: "Error"}}},
	}
	resources.mu.Unlock()
	status, err := executor.GetRemoteStatus(t.Context(), created)
	require.NoError(t, err)
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	var projected map[string]any
	require.NoError(t, json.Unmarshal(encoded, &projected))
	observation, ok := projected["observation"].(map[string]any)
	require.True(t, ok, "typed executor observation must survive status serialization")
	require.Equal(t, "terminated", observation["outcome"])
	require.Equal(t, "Evicted", observation["reason"])
	require.Len(t, observation["containers"], 2)
	require.NotContains(t, string(encoded), "supersecret")
	require.NotContains(t, string(encoded), "OOMKilled")
}

func TestExecutorFailureRefreshRejectsEvictedPod(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{8765: controlPort, 41001: instancePort})
	req := validKubernetesCreateRequest()
	created, err := executor.CreateInstance(t.Context(), req)
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod.Status.Phase = corev1.PodFailed
	resources.pod.Status.Reason = "Evicted"
	resources.mu.Unlock()
	_, err = executor.RefreshRemoteInstance(t.Context(), kubernetesRefreshInstance(created, req.Metadata))
	require.ErrorContains(t, err, "Evicted")
}

type failedRefreshStatusExecutor struct{ statusProviderExecutor }

func (e *failedRefreshStatusExecutor) RefreshRemoteInstance(context.Context, *ExecutorInstance) (*RemoteInstanceRefresh, error) {
	return nil, errors.New("cleanup authentication failed")
}

func TestExecutorFailureInspectionSurvivesRefreshFailure(t *testing.T) {
	provider := &failedRefreshStatusExecutor{statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{State: "failed", Details: map[string]any{"reason": "Evicted"}}}}
	mgr := newRemoteStatusManager(t, provider)
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", RuntimeName: agentruntime.RuntimeKubernetes}
	require.NoError(t, mgr.executionStore.Add(execution))
	mgr.pollOneRemoteStatus(t.Context(), execution)
	status, ok := mgr.GetRemoteStatusBySession("session")
	require.True(t, ok)
	require.Equal(t, "failed", status.State)
	require.Equal(t, "Evicted", status.Details["reason"])
	require.Contains(t, status.ErrorMessage, "cleanup authentication failed")
}

func TestExecutorFailureDockerExit137IsNotOOM(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":"owned-container","Config":{"Labels":{"kandev.task_id":"task"}},"State":{"Status":"exited","ExitCode":137,"OOMKilled":false}}`))
	}))
	t.Cleanup(daemon.Close)
	backend := NewDockerExecutor(config.DockerConfig{Host: "tcp://" + strings.TrimPrefix(daemon.URL, "http://")}, "", newTestDockerLogger())
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	provider, ok := any(backend).(RemoteStatusProvider)
	require.True(t, ok, "Docker must expose authoritative container inspection")
	status, err := provider.GetRemoteStatus(t.Context(), &ExecutorInstance{ContainerID: "owned-container", TaskID: "task"})
	require.NoError(t, err)
	require.Equal(t, "terminated", status.Observation.Outcome)
	require.NotEqual(t, "OOMKilled", status.Observation.Reason)
	require.EqualValues(t, 137, *status.Observation.Containers[0].ExitCode)
}

func TestExecutorFailureDockerExplicitOOM(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":"owned-container","Config":{"Labels":{"kandev.task_id":"task"}},"State":{"Status":"exited","ExitCode":137,"OOMKilled":true}}`))
	}))
	t.Cleanup(daemon.Close)
	backend := NewDockerExecutor(config.DockerConfig{Host: "tcp://" + strings.TrimPrefix(daemon.URL, "http://")}, "", newTestDockerLogger())
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	provider, ok := any(backend).(RemoteStatusProvider)
	require.True(t, ok, "Docker must expose authoritative container inspection")
	status, err := provider.GetRemoteStatus(t.Context(), &ExecutorInstance{ContainerID: "owned-container", TaskID: "task"})
	require.NoError(t, err)
	require.Equal(t, "terminated", status.Observation.Outcome)
	require.Equal(t, "OOMKilled", status.Observation.Reason)
	require.EqualValues(t, 137, *status.Observation.Containers[0].ExitCode)
}

func TestExecutorFailureCachedEvidenceIsImmutable(t *testing.T) {
	mgr := newRemoteStatusManager(t, &statusProviderExecutor{})
	obs := &models.ExecutorObservation{Outcome: "terminated", Containers: []models.ExecutorContainerEvidence{{Name: "agent"}}}
	mgr.storeRemoteStatus("session", &RemoteStatus{Observation: obs})
	obs.Containers[0].Name = "provider mutation"
	first, _ := mgr.GetRemoteStatusBySession("session")
	require.Equal(t, "agent", first.Observation.Containers[0].Name)
	first.Observation.Containers[0].Name = "caller mutation"
	second, _ := mgr.GetRemoteStatusBySession("session")
	require.Equal(t, "agent", second.Observation.Containers[0].Name)
}

func TestExecutorFailureEvidenceIsBounded(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Status.Phase = corev1.PodFailed
	pod.Status.Reason = strings.Repeat("x ", 3000)
	pod.Status.Message = strings.Repeat("m ", 3000)
	for range 12 {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{Name: strings.Repeat("n ", 3000), State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: strings.Repeat("r ", 3000)}}})
	}
	obs := kubernetesExecutorObservation(pod, "agent", time.Now())
	raw, err := json.Marshal(obs)
	require.NoError(t, err)
	require.LessOrEqual(t, len(raw), 4096)
	require.Len(t, obs.Containers, 8)
}

func TestExecutorFailureSucceededPodDominatesStaleContainer(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded, ContainerStatuses: []corev1.ContainerStatus{{Name: "agent", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}}
	obs := kubernetesExecutorObservation(pod, "agent", time.Now())
	require.Equal(t, "terminated", obs.Outcome)
}

func TestExecutorFailureUnsupportedEvidenceRemainsUnknown(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameSSH}, status: &RemoteStatus{State: "disconnected"}}
	mgr := newRemoteStatusManager(t, provider)
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", RuntimeName: agentruntime.RuntimeSSH}
	mgr.pollOneRemoteStatus(t.Context(), execution)
	status, _ := mgr.GetRemoteStatusBySession("session")
	require.NotNil(t, status.Observation)
	require.Equal(t, "unknown", status.Observation.Outcome)
}

func TestExecutorFailureMissingRecordedPodPreservesIdentity(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	backend := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{8765: controlPort, 41001: instancePort})
	created, err := backend.CreateInstance(t.Context(), validKubernetesCreateRequest())
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod = nil
	resources.mu.Unlock()
	status, err := backend.GetRemoteStatus(t.Context(), created)
	require.NoError(t, err)
	require.NotNil(t, status.Observation)
	require.Equal(t, "missing", status.Observation.Outcome)
	require.Equal(t, created.Metadata[MetadataKeyKubernetesPodUID], status.Observation.ResourceKey)
}

func TestExecutorFailureReadOnlyInspectionNeverRefreshes(t *testing.T) {
	provider := &failedRefreshStatusExecutor{statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "pod", Reason: "Evicted"}}}}
	mgr := newRemoteStatusManager(t, provider)
	inspector, ok := any(mgr).(interface {
		InspectExecutor(context.Context, models.ExecutorObservationTarget) (*models.ExecutorObservation, error)
	})
	require.True(t, ok, "recheck must use a read-only executor inspector")
	obs, err := inspector.InspectExecutor(t.Context(), models.ExecutorObservationTarget{TaskID: "task", EnvironmentID: "env", Runtime: "k8s", ResourceKey: "pod"})
	require.NoError(t, err)
	require.Equal(t, "Evicted", obs.Reason)
}

func TestExecutorFailurePromptWaitReturnsInterruption(t *testing.T) {
	sm := &SessionManager{logger: newTestDockerLogger()}
	execution := &AgentExecution{ID: "execution", promptDoneCh: make(chan PromptCompletionSignal, 1)}
	execution.promptDoneCh <- PromptCompletionSignal{IsError: true, ExecutorInterrupted: true, Error: "executor unavailable"}
	_, err := sm.waitForPromptDone(t.Context(), execution, 0)
	require.ErrorIs(t, err, ErrExecutorInterrupted)
}

func TestExecutorFailureRetiresOnlyObservedExecution(t *testing.T) {
	mgr := newRemoteStatusManager(t, &statusProviderExecutor{})
	execution := &AgentExecution{ID: "lost", SessionID: "session", TaskID: "task", ContainerID: "owned", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeDocker, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	require.NoError(t, mgr.executionStore.Add(execution))
	retirer, ok := any(mgr).(interface {
		RetireExecutorLoss(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) (bool, error)
	})
	require.True(t, ok, "confirmed resource loss needs exact retirement without resource deletion")
	target := models.ExecutorObservationTarget{TaskID: "task", EnvironmentID: "env", SessionID: "session", ExecutionID: "lost", ResourceKey: "owned", ContainerID: "owned", Runtime: "docker"}
	unknown := &models.ExecutorObservation{Outcome: "unknown", ResourceKey: "owned"}
	retired, err := retirer.RetireExecutorLoss(t.Context(), target, unknown)
	require.NoError(t, err)
	require.False(t, retired)
	_, tracked := mgr.executionStore.GetBySessionID("session")
	require.True(t, tracked)
	unknown.Outcome = "terminated"
	retired, err = retirer.RetireExecutorLoss(t.Context(), target, unknown)
	require.NoError(t, err)
	require.True(t, retired)
	_, tracked = mgr.executionStore.GetBySessionID("session")
	require.False(t, tracked)
	signal := <-execution.promptDoneCh
	require.True(t, signal.ExecutorInterrupted)
	successor := &AgentExecution{ID: "successor", SessionID: "session", TaskID: "task", ContainerID: "owned", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeDocker}
	require.NoError(t, mgr.executionStore.Add(successor))
	retired, err = retirer.RetireExecutorLoss(t.Context(), target, unknown)
	require.NoError(t, err)
	require.False(t, retired)
	current, _ := mgr.executionStore.GetBySessionID("session")
	require.Same(t, successor, current)
}

type failureEnvironmentWriter struct{ captureExecutorRunningWriter }

func (w *failureEnvironmentWriter) GetTaskEnvironment(context.Context, string) (*models.TaskEnvironment, error) {
	return &models.TaskEnvironment{ID: "env", TaskID: "task", OwnershipGeneration: 1}, nil
}

func (w *failureEnvironmentWriter) GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error) {
	return &models.KubernetesEnvironment{TaskID: "task", OwnershipGeneration: 1, Revision: 1, Metadata: map[string]interface{}{MetadataKeyKubernetesPodUID: "pod"}}, nil
}

func TestExecutorFailureDisconnectInspectsBeforePromptSignal(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "pod", ObservedAt: time.Now().UTC(), Reason: "Evicted"}}}
	mgr := newRemoteStatusManager(t, provider)
	mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
	setter, ok := any(mgr).(interface {
		SetExecutorObservationHandler(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) error)
	})
	require.True(t, ok, "managed stream loss must be classified outside startup locks")
	observed := make(chan *models.ExecutorObservation, 1)
	setter.SetExecutorObservationHandler(func(ctx context.Context, target models.ExecutorObservationTarget, obs *models.ExecutorObservation) error {
		observed <- obs
		return nil
	})
	execution := &AgentExecution{ID: "lost", SessionID: "session", TaskID: "task", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeKubernetes, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	execution.setSessionInitialized(true)
	execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
	require.NoError(t, mgr.executionStore.Add(execution))
	mgr.handleStreamDisconnectWithStartupGeneration(execution, errors.New("connection closed"), 0, 0)
	select {
	case obs := <-observed:
		require.Equal(t, "Evicted", obs.Reason)
	case <-time.After(5 * time.Second):
		t.Fatal("executor was not inspected")
	}
	select {
	case <-execution.promptDoneCh:
		t.Fatal("classification cannot signal ordinary prompt completion")
	default:
	}
}

func TestExecutorFailureRestartRequiresRecordedBaseline(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	backend := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{8765: controlPort, 41001: instancePort})
	created, err := backend.CreateInstance(t.Context(), validKubernetesCreateRequest())
	require.NoError(t, err)
	resources.mu.Lock()
	for i := range resources.pod.Status.ContainerStatuses {
		if resources.pod.Status.ContainerStatuses[i].Name == "kandev-agent" {
			resources.pod.Status.ContainerStatuses[i].RestartCount = 1
		}
	}
	resources.mu.Unlock()
	created.Metadata[MetadataKeyKubernetesContainerRestartCount] = "0"
	status, err := backend.GetRemoteStatus(t.Context(), created)
	require.NoError(t, err)
	require.Equal(t, "restarted", status.Observation.Outcome)
	require.Equal(t, "ContainerRestarted", status.Observation.Reason)
	delete(created.Metadata, MetadataKeyKubernetesContainerRestartCount)
	status, err = backend.GetRemoteStatus(t.Context(), created)
	require.NoError(t, err)
	require.Equal(t, "healthy", status.Observation.Outcome, "absence of baseline must not invent restart history")
}

func TestExecutorFailureConversationOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		missing       bool
	}{{"restored", "restored", false}, {"missing rollout preserves conversation and blocks recovery", "unknown", true}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.handler = func(msg ws.Message) *ws.Message {
				if tc.missing && msg.Action == "agent.session.load" {
					response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, `{"code":-32603,"message":"Internal error","data":{"details":"no rollout found for thread id saved-session"}}`, nil)
					return response
				}
				return mock.defaultHandler(msg)
			}
			sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
			client := createTestClient(t, mock.server.URL)
			t.Cleanup(client.Close)
			require.NoError(t, client.StreamUpdates(t.Context(), func(agentctl.AgentEvent) {}, nil, nil))
			waitForWSConnected(t, mock)
			config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP, SessionConfig: agents.SessionConfig{NativeSessionResume: true}}}
			result, err := sm.InitializeSession(t.Context(), nil, client, config, "saved-session", "/workspace", nil)
			if tc.missing {
				require.Error(t, err)
				require.Nil(t, result)
				var required *RestoreRequiredError
				require.ErrorAs(t, err, &required)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.outcome, result.ConversationOutcome, "successful initialization must report the actual load/create result")
		})
	}
}

func TestExecutorFailureRetirementRejectsReplacedRowWithSameExecutionID(t *testing.T) {
	mgr := newRemoteStatusManager(t, &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}})
	now := time.Now().UTC()
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{AgentExecutionID: "same", UpdatedAt: now.Add(time.Second)}})
	execution := &AgentExecution{ID: "same", TaskID: "task", SessionID: "session", ContainerID: "container", RuntimeName: agentruntime.RuntimeDocker}
	require.NoError(t, mgr.executionStore.Add(execution))
	retired, err := mgr.RetireExecutorLoss(t.Context(), models.ExecutorObservationTarget{TaskID: "task", SessionID: "session", ExecutionID: "same", ResourceKey: "container", ExpectedExecutorUpdatedAt: now}, &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "container"})
	require.NoError(t, err)
	require.False(t, retired, "a replaced persistence row must invalidate retirement even when execution ID was reused")
	_, tracked := mgr.executionStore.GetBySessionID("session")
	require.True(t, tracked)
}

func TestExecutorFailureProviderOutcomeCannotLeakIntoUnrelatedSessionEvent(t *testing.T) {
	log := newSessionTestLogger()
	eventBus := bus.NewMemoryEventBus(log)
	pub := NewEventPublisher(eventBus, log)
	received := make(chan *bus.Event, 1)
	sub, err := eventBus.Subscribe(events.AgentACPSessionCreated, func(_ context.Context, event *bus.Event) error { received <- event; return nil })
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	execution.setMetadataValue("provider_conversation_recovery", "fresh")
	pub.PublishACPSessionCreatedWithAttempt(execution, "user-reset-provider", "")
	select {
	case event := <-received:
		raw, err := json.Marshal(event.Data)
		require.NoError(t, err)
		require.NotContains(t, string(raw), `"conversation_outcome":"fresh"`, "only the actual initialization boundary may report recovery")
	case <-time.After(time.Second):
		t.Fatal("provider event missing")
	}
}

func TestExecutorFailureLocalControllerLossUsesExitEvidence(t *testing.T) {
	mgr := newRemoteStatusManager(t, &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}})
	setter, ok := any(mgr).(interface {
		SetLocalExecutorInspector(func(models.ExecutorObservationTarget) *models.ExecutorObservation)
	})
	require.True(t, ok, "local controller exit proof must reach task-owned failure reconciliation")
	setter.SetLocalExecutorInspector(func(target models.ExecutorObservationTarget) *models.ExecutorObservation {
		if target.ResourceKey != "local-pid:42" {
			return nil
		}
		return &models.ExecutorObservation{Outcome: "terminated", Runtime: "standalone", ResourceKey: target.ResourceKey, ObservedAt: time.Now().UTC(), Reason: "ControllerExited", Workspace: "unknown"}
	})
	obs, err := mgr.InspectExecutor(t.Context(), models.ExecutorObservationTarget{TaskID: "task", Runtime: "standalone", ResourceKey: "local-pid:42"})
	require.NoError(t, err)
	require.Equal(t, "terminated", obs.Outcome)
	require.Equal(t, "ControllerExited", obs.Reason)
	require.Empty(t, obs.Containers, "unavailable exit codes cannot be invented")
}

func TestExecutorFailureLaunchDuplicatePreservesPrimaryResourceCause(t *testing.T) {
	mgr := newTestManager(t)
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "pod", ObservedAt: time.Now().UTC(), Reason: "Evicted", Message: "EmptyDir docker-data exceeds 12Gi"}}}
	mgr.executorRegistry = NewExecutorRegistry(newTestRegistryLogger())
	mgr.executorRegistry.Register(provider)
	mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
	execution := &AgentExecution{ID: "lost", SessionID: "session", TaskID: "task", TaskEnvironmentID: "env", AgentProfileID: "profile", AgentCommand: "auggie --acp", RuntimeName: agentruntime.RuntimeKubernetes}
	execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
	require.NoError(t, mgr.executionStore.Add(execution))
	_, err := mgr.Launch(t.Context(), &LaunchRequest{TaskID: "task", SessionID: "session", AgentProfileID: "profile"})
	require.ErrorContains(t, err, "Evicted", "duplicate inventory and cleanup errors must not hide proven resource loss")
	require.NotErrorIs(t, err, ErrAgentAlreadyRunning, "a dead resource cannot trigger duplicate cleanup and automatic relaunch")
	var cause interface {
		ExecutorObservation() *models.ExecutorObservation
	}
	require.True(t, errors.As(err, &cause))
	require.Equal(t, "terminated", cause.ExecutorObservation().Outcome)
}

func TestExecutorFailureTransientHealthyDisconnectDoesNotSettle(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: "healthy", ResourceKey: "pod", ObservedAt: time.Now().UTC()}}}
	mgr := newRemoteStatusManager(t, provider)
	mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
	calls := 0
	mgr.SetExecutorObservationHandler(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) error {
		calls++
		return nil
	})
	execution := &AgentExecution{ID: "live", SessionID: "session", TaskID: "task", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeKubernetes, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
	require.NoError(t, mgr.executionStore.Add(execution))
	mgr.classifyExecutorDisconnect(execution, 0, 0)
	require.Zero(t, calls, "transient disconnect must not create an incident")
	current, ok := mgr.executionStore.GetBySessionID("session")
	require.True(t, ok)
	require.Same(t, execution, current)
	require.Empty(t, execution.promptDoneCh, "transport reconnect cannot complete or replay the prompt")
}

func TestExecutorFailureUnknownDisconnectPreservesOwnership(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, err: errors.New("API temporarily unreachable")}
		mgr := newRemoteStatusManager(t, provider)
		mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
		calls := 0
		mgr.SetExecutorObservationHandler(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) error {
			calls++
			return nil
		})
		execution := &AgentExecution{ID: "uncertain", SessionID: "session", TaskID: "task", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeKubernetes, promptDoneCh: make(chan PromptCompletionSignal, 1)}
		execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
		require.NoError(t, mgr.executionStore.Add(execution))
		mgr.classifyExecutorDisconnect(execution, 0, 0)
		require.Zero(t, calls, "temporary API uncertainty cannot become confirmed executor failure")
		status, ok := mgr.GetRemoteStatusBySession("session")
		require.True(t, ok)
		require.Equal(t, "unknown", status.State)
		current, ok := mgr.executionStore.GetBySessionID("session")
		require.True(t, ok)
		require.Same(t, execution, current)
		require.Empty(t, execution.promptDoneCh)
	})
}

func TestExecutorFailureStartupStatusOwnsCopyAndSanitizesErrors(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{}}
	mgr := newRemoteStatusManager(t, provider)
	records := []RemoteStatusPollRecord{{TaskID: "task", SessionID: "session", Runtime: agentruntime.RuntimeKubernetes, AgentExecutionID: "exec"}}
	mgr.PollRemoteStatusForRecords(t.Context(), records)
	require.True(t, provider.status.LastCheckedAt.IsZero(), "startup hydration cannot mutate shared provider snapshots")
	provider.err = errors.New("token=supersecret " + strings.Repeat("x", 2000))
	mgr.PollRemoteStatusForRecords(t.Context(), records)
	cached, ok := mgr.GetRemoteStatusBySession("session")
	require.True(t, ok)
	require.NotContains(t, cached.ErrorMessage, "supersecret")
	require.LessOrEqual(t, len(cached.ErrorMessage), 768)
}

func TestExecutorFailureCleanupRetainsPrimaryAndSecondaryCause(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes, stopInstanceErr: context.DeadlineExceeded}, status: &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: "terminated", ResourceKey: "pod", ObservedAt: time.Now().UTC(), Reason: "Evicted", Message: "EmptyDir exceeds 12Gi"}}}
	mgr := newRemoteStatusManager(t, provider)
	mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
	observed := make(chan *models.ExecutorObservation, 1)
	mgr.SetExecutorObservationHandler(func(_ context.Context, _ models.ExecutorObservationTarget, obs *models.ExecutorObservation) error {
		observed <- obs
		return nil
	})
	execution := &AgentExecution{ID: "lost", SessionID: "session", TaskID: "task", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeKubernetes}
	execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
	require.NoError(t, mgr.executionStore.Add(execution))
	err := mgr.CleanupStaleExecutionBySessionID(t.Context(), "session")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "Evicted", "secondary timeout must not replace the physical cause")
	var cause interface {
		ExecutorObservation() *models.ExecutorObservation
	}
	require.True(t, errors.As(err, &cause))
	raw, encodeErr := json.Marshal(cause.ExecutorObservation())
	require.NoError(t, encodeErr)
	require.Contains(t, string(raw), "cleanup_timeout")
	select {
	case durable := <-observed:
		encoded, _ := json.Marshal(durable)
		require.Contains(t, string(encoded), "cleanup_timeout")
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup failure evidence not admitted")
	}
	current, ok := mgr.executionStore.GetBySessionID("session")
	require.True(t, ok)
	require.Same(t, execution, current, "reporting does not bypass failed cleanup")
}

func TestExecutorFailureLegacyTargetCapturesSourceRevision(t *testing.T) {
	mgr := newRemoteStatusManager(t, &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}})
	now := time.Now().UTC()
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{TaskID: "task", AgentExecutionID: "same", ContainerID: "container", UpdatedAt: now}})
	execution := &AgentExecution{ID: "same", TaskID: "task", SessionID: "session", RuntimeName: agentruntime.RuntimeDocker, ContainerID: "container"}
	target, err := mgr.executorObservationTarget(t.Context(), execution)
	require.NoError(t, err)
	require.Equal(t, now, target.ExpectedExecutorUpdatedAt, "legacy observations must reject a same-ID successor row")
}
