package executor

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestSwitchModelFallbackReusesAuthoritativeKubernetesRuntime(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-k8s"] = &models.Task{
		ID: "task-k8s", WorkspaceID: "workspace-1", Title: "Kubernetes task",
	}
	repo.sessions["session-k8s"] = &models.TaskSession{
		ID: "session-k8s", TaskID: "task-k8s", AgentProfileID: "agent-profile",
		ExecutorID: "repointed-executor", ExecutorProfileID: "edited-profile",
		State: models.TaskSessionStateRunning,
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyKubernetesPodUID:          "hostile-pod-uid",
			lifecycle.MetadataKeyKubernetesProfileSnapshot: "hostile-snapshot",
		},
	}
	wantConfig := map[string]string{
		lifecycle.MetadataKeyKubernetesAuthMode:              "kubeconfig",
		lifecycle.MetadataKeyKubernetesKubeconfigPath:        "/etc/kandev/current-kubeconfig",
		lifecycle.MetadataKeyKubernetesKubeContext:           "current-context",
		lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
		lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "45",
	}
	repo.executors["recorded-kubernetes-executor"] = &models.Executor{
		ID: "recorded-kubernetes-executor", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: wantConfig,
	}
	repo.executors["repointed-executor"] = &models.Executor{
		ID: "repointed-executor", Type: models.ExecutorTypeLocal,
		Status: models.ExecutorStatusActive,
	}
	repo.executorsRunning["session-k8s"] = &models.ExecutorRunning{
		ID: "running-k8s", SessionID: "session-k8s", TaskID: "task-k8s",
		ExecutorID: "recorded-kubernetes-executor", Runtime: agentruntime.RuntimeKubernetes,
		AgentExecutionID: "recorded-execution", ResumeToken: "resume-token",
		Metadata: recordedKubernetesResumeMetadataFor("task-k8s", "session-k8s", "recorded-execution"),
	}
	var launched *LaunchAgentRequest
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "recorded-execution", nil
		},
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launched = req
			return &LaunchAgentResponse{AgentExecutionID: "replacement-execution"}, nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	_, err := exec.SwitchModel(
		context.Background(), "task-k8s", "session-k8s", "new-model", "continue",
	)

	if err != nil {
		t.Fatalf("SwitchModel() error = %v", err)
	}
	if launched == nil {
		t.Fatal("Kubernetes model-switch fallback did not launch")
	}
	if launched.ExecutorType != string(models.ExecutorTypeKubernetes) {
		t.Fatalf("ExecutorType = %q, want k8s", launched.ExecutorType)
	}
	if launched.PreviousExecutionID != "recorded-execution" {
		t.Fatalf("PreviousExecutionID = %q, want recorded-execution", launched.PreviousExecutionID)
	}
	if got := launched.Metadata[lifecycle.MetadataKeyKubernetesPodUID]; got != "recorded-pod-uid" {
		t.Fatalf("Pod UID = %v, want recorded-pod-uid", got)
	}
	if got := launched.Metadata[lifecycle.MetadataKeyKubernetesProfileSnapshot]; got != repo.executorsRunning["session-k8s"].Metadata[lifecycle.MetadataKeyKubernetesProfileSnapshot] {
		t.Fatalf("profile snapshot = %v, want recorded workload snapshot", got)
	}
	if fmt.Sprint(launched.ExecutorConfig) != fmt.Sprint(wantConfig) {
		t.Fatalf("ExecutorConfig = %#v, want %#v", launched.ExecutorConfig, wantConfig)
	}
}

func TestSwitchModelFallbackStopsOnKubernetesTeardownFailure(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-k8s"] = &models.Task{ID: "task-k8s", WorkspaceID: "workspace-1"}
	repo.sessions["session-k8s"] = &models.TaskSession{
		ID: "session-k8s", TaskID: "task-k8s", AgentProfileID: "agent-profile",
		ExecutorID: "recorded-kubernetes-executor",
		State:      models.TaskSessionStateRunning,
	}
	repo.executors["recorded-kubernetes-executor"] = &models.Executor{
		ID: "recorded-kubernetes-executor", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: map[string]string{
			lifecycle.MetadataKeyKubernetesAuthMode:              "in_cluster",
			lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
			lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "30",
		},
	}
	repo.executorsRunning["session-k8s"] = &models.ExecutorRunning{
		SessionID: "session-k8s", TaskID: "task-k8s", Runtime: agentruntime.RuntimeKubernetes,
		ExecutorID: "recorded-kubernetes-executor", AgentExecutionID: "recorded-execution",
		Metadata: recordedKubernetesResumeMetadataFor("task-k8s", "session-k8s", "recorded-execution"),
	}
	teardownErr := errors.New("close Kubernetes forward failed")
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "recorded-execution", nil
		},
		stopAgentFunc: func(context.Context, string, bool) error { return teardownErr },
	}
	exec := newTestExecutor(t, manager, repo)

	_, err := exec.SwitchModel(
		context.Background(), "task-k8s", "session-k8s", "new-model", "continue",
	)

	if !errors.Is(err, teardownErr) {
		t.Fatalf("SwitchModel() error = %v, want teardown cause", err)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("LaunchAgent calls = %d, want zero after failed Kubernetes teardown", manager.launchAgentCallCount)
	}
}

func TestSwitchModelFallbackValidatesKubernetesAuthorityBeforeStop(t *testing.T) {
	readErr := errors.New("running inventory unavailable")
	tests := []struct {
		name   string
		mutate func(*mockRepository)
	}{
		{
			name: "running row read failure",
			mutate: func(repo *mockRepository) {
				repo.getExecutorRunningFunc = func(context.Context, string) (*models.ExecutorRunning, error) {
					return nil, readErr
				}
			},
		},
		{name: "missing Kubernetes row", mutate: func(repo *mockRepository) { delete(repo.executorsRunning, "session-k8s") }},
		{name: "task mismatch", mutate: func(repo *mockRepository) { repo.executorsRunning["session-k8s"].TaskID = "other-task" }},
		{name: "session mismatch", mutate: func(repo *mockRepository) { repo.executorsRunning["session-k8s"].SessionID = "other-session" }},
		{name: "execution mismatch", mutate: func(repo *mockRepository) { repo.executorsRunning["session-k8s"].AgentExecutionID = "other-execution" }},
		{name: "wrong recorded runtime", mutate: func(repo *mockRepository) { repo.executorsRunning["session-k8s"].Runtime = agentruntime.RuntimeSSH }},
		{name: "missing recorded executor", mutate: func(repo *mockRepository) { repo.executorsRunning["session-k8s"].ExecutorID = "deleted-executor" }},
		{name: "recorded executor changed type", mutate: func(repo *mockRepository) {
			repo.executors["recorded-kubernetes-executor"].Type = models.ExecutorTypeLocal
		}},
		{name: "incomplete workload snapshot", mutate: func(repo *mockRepository) {
			delete(repo.executorsRunning["session-k8s"].Metadata, lifecycle.MetadataKeyKubernetesProfileSnapshot)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks["task-k8s"] = &models.Task{ID: "task-k8s", WorkspaceID: "workspace-1"}
			repo.sessions["session-k8s"] = &models.TaskSession{
				ID: "session-k8s", TaskID: "task-k8s", AgentProfileID: "agent-profile",
				ExecutorID: "recorded-kubernetes-executor", State: models.TaskSessionStateRunning,
			}
			repo.executors["recorded-kubernetes-executor"] = &models.Executor{
				ID: "recorded-kubernetes-executor", Type: models.ExecutorTypeKubernetes,
				Status: models.ExecutorStatusActive,
				Config: map[string]string{
					lifecycle.MetadataKeyKubernetesAuthMode:              "in_cluster",
					lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
					lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "30",
				},
			}
			repo.executorsRunning["session-k8s"] = &models.ExecutorRunning{
				ID: "running-k8s", SessionID: "session-k8s", TaskID: "task-k8s",
				ExecutorID: "recorded-kubernetes-executor", Runtime: agentruntime.RuntimeKubernetes,
				AgentExecutionID: "recorded-execution", ResumeToken: "resume-token",
				Metadata: recordedKubernetesResumeMetadataFor("task-k8s", "session-k8s", "recorded-execution"),
			}
			test.mutate(repo)
			stopCalls := 0
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
					return "recorded-execution", nil
				},
				stopAgentFunc: func(context.Context, string, bool) error { stopCalls++; return nil },
			}
			exec := newTestExecutor(t, manager, repo)

			_, err := exec.SwitchModel(context.Background(), "task-k8s", "session-k8s", "new-model", "continue")

			if err == nil {
				t.Fatal("SwitchModel() error = nil, want fail-closed Kubernetes authority error")
			}
			if stopCalls != 0 {
				t.Fatalf("StopAgent calls = %d, want zero before Kubernetes request validation", stopCalls)
			}
			if manager.launchAgentCallCount != 0 {
				t.Fatalf("LaunchAgent calls = %d, want zero", manager.launchAgentCallCount)
			}
		})
	}
}

func TestSwitchModelFallbackKeepsLegacyBehaviorWhenNonKubernetesInventoryReadFails(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-local"] = &models.Task{ID: "task-local", WorkspaceID: "workspace-1"}
	repo.sessions["session-local"] = &models.TaskSession{
		ID: "session-local", TaskID: "task-local", AgentProfileID: "agent-profile",
		ExecutorID: "local-executor", State: models.TaskSessionStateRunning,
	}
	repo.executors["local-executor"] = &models.Executor{
		ID: "local-executor", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
	}
	repo.getExecutorRunningFunc = func(context.Context, string) (*models.ExecutorRunning, error) {
		return nil, errors.New("running inventory unavailable")
	}
	stopCalls := 0
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "local-execution", nil
		},
		stopAgentFunc: func(context.Context, string, bool) error { stopCalls++; return nil },
	}
	exec := newTestExecutor(t, manager, repo)

	_, err := exec.SwitchModel(context.Background(), "task-local", "session-local", "new-model", "continue")

	if err != nil {
		t.Fatalf("SwitchModel() error = %v", err)
	}
	if stopCalls != 1 || manager.launchAgentCallCount != 1 {
		t.Fatalf("stop/launch calls = %d/%d, want 1/1", stopCalls, manager.launchAgentCallCount)
	}
}

// Reviewer-requested contract coverage: request construction remains before
// destructive Kubernetes stop, so any late resolver error leaves live runtime intact.
func TestSwitchModelFallbackBuildFailureLeavesKubernetesAgentRunning(t *testing.T) {
	repo := newMockRepository()
	task := &models.Task{ID: "task-k8s", WorkspaceID: "workspace-1"}
	repo.tasks[task.ID] = task
	repo.sessions["session-k8s"] = &models.TaskSession{
		ID: "session-k8s", TaskID: task.ID, AgentProfileID: "agent-profile",
		ExecutorID: "recorded-kubernetes-executor", State: models.TaskSessionStateRunning,
	}
	repo.executors["recorded-kubernetes-executor"] = &models.Executor{
		ID: "recorded-kubernetes-executor", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: map[string]string{
			lifecycle.MetadataKeyKubernetesAuthMode:              "in_cluster",
			lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
			lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "30",
		},
	}
	repo.executorsRunning["session-k8s"] = &models.ExecutorRunning{
		ID: "running-k8s", SessionID: "session-k8s", TaskID: task.ID,
		ExecutorID: "recorded-kubernetes-executor", Runtime: agentruntime.RuntimeKubernetes,
		AgentExecutionID: "recorded-execution", ResumeToken: "resume-token",
		Metadata: recordedKubernetesResumeMetadataFor(task.ID, "session-k8s", "recorded-execution"),
	}
	buildErr := errors.New("MCP request projection unavailable")
	taskReads := 0
	repo.getTaskFunc = func(context.Context, string) (*models.Task, error) {
		taskReads++
		if taskReads > 1 {
			return nil, buildErr
		}
		return task, nil
	}
	stopCalls := 0
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "recorded-execution", nil
		},
		stopAgentFunc: func(context.Context, string, bool) error { stopCalls++; return nil },
	}
	exec := newTestExecutor(t, manager, repo)

	_, err := exec.SwitchModel(context.Background(), task.ID, "session-k8s", "new-model", "continue")

	if !errors.Is(err, buildErr) {
		t.Fatalf("SwitchModel() error = %v, want request-build cause", err)
	}
	if stopCalls != 0 {
		t.Fatalf("StopAgent calls = %d, want zero before complete request build", stopCalls)
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("LaunchAgent calls = %d, want zero", manager.launchAgentCallCount)
	}
}
