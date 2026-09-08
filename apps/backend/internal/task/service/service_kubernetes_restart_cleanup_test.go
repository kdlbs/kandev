package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	agentexecutor "github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCleanupTaskResourcesRecoversPersistedKubernetesRuntimeAfterRestart(t *testing.T) {
	tests := []struct {
		name        string
		stopErr     error
		wantDeleted bool
	}{
		{name: "exact cleanup succeeds", wantDeleted: true},
		{name: "exact cleanup failure stays retryable", stopErr: errors.New("RBAC denied exact cleanup")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedPersistedKubernetesCleanupFixture(t, ctx, repo)

			backend := &persistedKubernetesCleanupBackend{stopErr: test.stopErr}
			registry := lifecycle.NewExecutorRegistry(svc.logger)
			registry.Register(backend)
			manager := lifecycle.NewManager(
				nil, svc.eventBus, registry, nil, nil, nil,
				lifecycle.ExecutorFallbackWarn, "", svc.logger,
			)
			manager.SetExecutorRunningWriter(repo)
			t.Cleanup(func() { _ = manager.Stop() })
			svc.SetExecutionStopper(&lifecycleManagerTaskStopper{manager: manager})
			svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

			quickChatRoot := t.TempDir()
			svc.SetQuickChatDir(quickChatRoot)
			sessionDir := filepath.Join(quickChatRoot, "session-k8s")
			if err := os.MkdirAll(sessionDir, 0o755); err != nil {
				t.Fatalf("create session dir: %v", err)
			}

			svc.CleanupTaskResources(ctx, "task-k8s", false)
			waitForCleanupDone(t, svc)

			_, rowErr := repo.GetExecutorRunningBySessionID(ctx, "session-k8s")
			if test.wantDeleted {
				if !backend.podDeleted || !backend.pvcDeleted {
					t.Fatalf("exact Kubernetes cleanup = pod:%t pvc:%t, want both", backend.podDeleted, backend.pvcDeleted)
				}
				if !errors.Is(rowErr, models.ErrExecutorRunningNotFound) {
					t.Fatalf("executor row remains after exact cleanup: %v", rowErr)
				}
				if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
					t.Fatalf("session dir remains after exact cleanup: %v", err)
				}
				return
			}
			if rowErr != nil {
				t.Fatalf("failed cleanup lost authoritative executor row: %v", rowErr)
			}
			if backend.podDeleted || backend.pvcDeleted {
				t.Fatalf("failed cleanup reported deletion = pod:%t pvc:%t", backend.podDeleted, backend.pvcDeleted)
			}
			if _, err := os.Stat(sessionDir); err != nil {
				t.Fatalf("failed cleanup removed retryable session resources: %v", err)
			}
		})
	}
}

func seedPersistedKubernetesCleanupFixture(
	t *testing.T,
	ctx context.Context,
	repo interface {
		CreateWorkspace(context.Context, *models.Workspace) error
		CreateWorkflow(context.Context, *models.Workflow) error
		CreateTask(context.Context, *models.Task) error
		CreateTaskSession(context.Context, *models.TaskSession) error
		CreateExecutor(context.Context, *models.Executor) error
		UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
	},
) {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-k8s", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-k8s", WorkspaceID: "workspace-k8s", Name: "WF"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-k8s", WorkspaceID: "workspace-k8s", WorkflowID: "workflow-k8s",
		WorkflowStepID: "step-k8s", Title: "Kubernetes", Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-k8s", TaskID: "task-k8s", State: models.TaskSessionStateCancelled,
		AgentExecutionID: "execution-k8s",
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-k8s", Name: "Kubernetes", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: map[string]string{
			lifecycle.MetadataKeyKubernetesAuthMode:              "in_cluster",
			lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
			lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "45",
		},
	}); err != nil {
		t.Fatalf("create executor: %v", err)
	}
	metadata := map[string]interface{}{
		"executor_id":                                        "executor-k8s",
		lifecycle.MetadataKeyExecutorProfileID:               "profile-k8s",
		lifecycle.MetadataKeyKubernetesAuthMode:              "kubeconfig",
		lifecycle.MetadataKeyKubernetesKubeconfigPath:        "/stale/kubeconfig",
		lifecycle.MetadataKeyKubernetesKubeContext:           "stale-context",
		lifecycle.MetadataKeyKubernetesConfigNamespace:       "kandev-agents",
		lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds: "10",
		lifecycle.MetadataKeyKubernetesNamespace:             "kandev-agents",
		lifecycle.MetadataKeyKubernetesPodName:               "pod-k8s",
		lifecycle.MetadataKeyKubernetesPodUID:                "pod-uid",
		lifecycle.MetadataKeyKubernetesMainContainer:         "kandev-agent",
		lifecycle.MetadataKeyKubernetesPlatform:              "linux/amd64",
		lifecycle.MetadataKeyKubernetesRuntimeWorkspaceMode:  "managed_pvc",
		lifecycle.MetadataKeyKubernetesPVCName:               "pvc-k8s",
		lifecycle.MetadataKeyKubernetesPVCUID:                "pvc-uid",
		lifecycle.MetadataKeyKubernetesPVCCreated:            true,
		lifecycle.MetadataKeyKubernetesAgentctlRemotePort:    "41001",
		lifecycle.MetadataKeyKubernetesAgentctlInstanceID:    "resource-k8s",
		lifecycle.MetadataKeyKubernetesResourceInstanceID:    "resource-k8s",
		lifecycle.MetadataKeyKubernetesResourceExecutorID:    "executor-k8s",
		lifecycle.MetadataKeyKubernetesResourceProfileID:     "profile-k8s",
		lifecycle.MetadataKeyKubernetesResourceTaskID:        "task-k8s",
		lifecycle.MetadataKeyKubernetesResourceSessionID:     "session-k8s",
		lifecycle.MetadataKeyKubernetesResourceEnvironmentID: "environment-k8s",
		lifecycle.MetadataKeyKubernetesProfileSnapshot:       `{}`,
		lifecycle.MetadataKeyIsRemote:                        true,
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-k8s", SessionID: "session-k8s", TaskID: "task-k8s",
		ExecutorID: "executor-k8s", ExecutionProfileID: "agent-profile-k8s",
		AgentExecutionID: "execution-k8s", Runtime: agentruntime.RuntimeKubernetes,
		Status: models.ExecutorRunningStatusRunning, Metadata: metadata,
	}); err != nil {
		t.Fatalf("upsert executor running: %v", err)
	}
}

type lifecycleManagerTaskStopper struct {
	manager *lifecycle.Manager
}

func (*lifecycleManagerTaskStopper) StopTask(context.Context, string, string, bool) error { return nil }
func (*lifecycleManagerTaskStopper) StopSession(context.Context, string, string, bool) error {
	return errors.New("session-only stop is not expected")
}
func (s *lifecycleManagerTaskStopper) StopExecution(
	ctx context.Context,
	executionID string,
	reason string,
	force bool,
) error {
	return s.manager.StopAgentWithReason(ctx, executionID, reason, force)
}
func (*lifecycleManagerTaskStopper) RegisterExecutionStopOwner(string, string, bool) {}

type persistedKubernetesCleanupBackend struct {
	stopErr    error
	podDeleted bool
	pvcDeleted bool
}

func (*persistedKubernetesCleanupBackend) Name() agentexecutor.Name {
	return agentexecutor.NameKubernetes
}
func (*persistedKubernetesCleanupBackend) HealthCheck(context.Context) error { return nil }
func (*persistedKubernetesCleanupBackend) CreateInstance(
	context.Context,
	*lifecycle.ExecutorCreateRequest,
) (*lifecycle.ExecutorInstance, error) {
	return nil, errors.New("create is not expected")
}
func (b *persistedKubernetesCleanupBackend) StopInstance(
	_ context.Context,
	instance *lifecycle.ExecutorInstance,
	force bool,
) error {
	if instance == nil || instance.TaskID != "task-k8s" || instance.SessionID != "session-k8s" ||
		instance.InstanceID != "execution-k8s" || !force ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesPodName] != "pod-k8s" ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesPodUID] != "pod-uid" ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesPVCName] != "pvc-k8s" ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesPVCUID] != "pvc-uid" ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesAuthMode] != "in_cluster" ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesKubeconfigPath] != "" ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesKubeContext] != "" ||
		instance.Metadata[lifecycle.MetadataKeyKubernetesRequestTimeoutSeconds] != "45" {
		return errors.New("reconstructed Kubernetes cleanup instance is not exact")
	}
	if b.stopErr != nil {
		return b.stopErr
	}
	b.podDeleted = true
	b.pvcDeleted = true
	return nil
}
func (*persistedKubernetesCleanupBackend) RecoverInstances(context.Context) ([]*lifecycle.ExecutorInstance, error) {
	return nil, nil
}
func (*persistedKubernetesCleanupBackend) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}
func (*persistedKubernetesCleanupBackend) RequiresCloneURL() bool          { return true }
func (*persistedKubernetesCleanupBackend) ShouldApplyPreferredShell() bool { return false }
func (*persistedKubernetesCleanupBackend) IsAlwaysResumable() bool         { return true }
