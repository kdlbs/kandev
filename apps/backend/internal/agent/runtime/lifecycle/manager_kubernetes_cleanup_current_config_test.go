package lifecycle

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestStopAgentWithReasonUsesCurrentKubernetesConnectionOnRetry(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &currentConnectionStopBackend{
		MockExecutor: MockExecutor{name: executor.NameKubernetes},
		staleErr:     errors.New("stale Kubernetes credentials"),
		transientErr: errors.New("temporary Kubernetes API failure"),
	}
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(&restartKubernetesInventoryStore{executors: map[string]*models.Executor{
		"executor-1": {
			ID: "executor-1", Type: models.ExecutorTypeKubernetes,
			Config: map[string]string{
				MetadataKeyKubernetesAuthMode:              "in_cluster",
				MetadataKeyKubernetesConfigNamespace:       "new-agents",
				MetadataKeyKubernetesRequestTimeoutSeconds: "45",
			},
		},
	}})
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: executor.NameKubernetes, Status: v1.AgentStatusRunning,
		metadata: map[string]interface{}{
			"executor_id":                              "executor-1",
			MetadataKeyKubernetesAuthMode:              "kubeconfig",
			MetadataKeyKubernetesKubeconfigPath:        "/stale/kubeconfig",
			MetadataKeyKubernetesKubeContext:           "stale-context",
			MetadataKeyKubernetesConfigNamespace:       "old-agents",
			MetadataKeyKubernetesRequestTimeoutSeconds: "10",
			MetadataKeyKubernetesNamespace:             "old-agents",
		},
	}))

	err := mgr.StopAgentWithReason(context.Background(), "execution-1", StopReasonTaskDeleted, true)
	require.ErrorIs(t, err, backend.transientErr)
	_, exists := mgr.GetExecution("execution-1")
	require.True(t, exists, "transient cleanup failure must retain the execution")

	require.NoError(t, mgr.StopAgentWithReason(
		context.Background(), "execution-1", StopReasonTaskDeleted, true,
	))
	require.Len(t, backend.calls, 2)
	for _, metadata := range backend.calls {
		require.Equal(t, "in_cluster", metadata[MetadataKeyKubernetesAuthMode])
		require.Empty(t, metadata[MetadataKeyKubernetesKubeconfigPath])
		require.Empty(t, metadata[MetadataKeyKubernetesKubeContext])
		require.Equal(t, "new-agents", metadata[MetadataKeyKubernetesConfigNamespace])
		require.Equal(t, "old-agents", metadata[MetadataKeyKubernetesNamespace])
	}
}

func TestKubernetesStopUsesCurrentCleanupClientInsteadOfLaunchSessionClient(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	launchResources := &fakeKubernetesResources{}
	executorBackend := newFakeKubernetesExecutor(
		t, launchResources, &recordingKubernetesExec{}, map[uint16]uint16{
			uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
			41001:                                    instancePort,
		},
	)
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	instance, err := executorBackend.CreateInstance(context.Background(), req)
	require.NoError(t, err)

	launchResources.mu.Lock()
	currentResources := &fakeKubernetesResources{
		pod: launchResources.pod.DeepCopy(), pvc: launchResources.pvc.DeepCopy(),
	}
	launchResources.getPodErr = apierrors.NewUnauthorized("launch credentials expired")
	launchResources.mu.Unlock()
	var cleanupConfig kubeexecutor.ExecutorConfig
	executorBackend.clientFactory = func(config kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		cleanupConfig = config
		return &kubernetesRuntimeClient{resources: currentResources}, nil
	}
	instance.Metadata[MetadataKeyKubernetesAuthMode] = "in_cluster"
	instance.Metadata[MetadataKeyKubernetesKubeconfigPath] = ""
	instance.Metadata[MetadataKeyKubernetesKubeContext] = ""
	instance.Metadata[MetadataKeyKubernetesConfigNamespace] = "new-agents"
	instance.Metadata[MetadataKeyKubernetesRequestTimeoutSeconds] = "45"
	instance.StopReason = StopReasonTaskDeleted

	require.NoError(t, executorBackend.StopInstance(context.Background(), instance, true))
	require.Equal(t, kubeexecutor.AuthModeInCluster, cleanupConfig.AuthMode)
	require.Equal(t, "new-agents", cleanupConfig.Namespace)
	require.Equal(t, []string{"pod", "pvc"}, currentResources.deletionOrder)
}

type currentConnectionStopBackend struct {
	MockExecutor
	staleErr        error
	transientErr    error
	currentAttempts int
	calls           []map[string]interface{}
}

func (b *currentConnectionStopBackend) StopInstance(
	_ context.Context,
	instance *ExecutorInstance,
	_ bool,
) error {
	metadata := cloneKubernetesMetadata(instance.Metadata)
	b.calls = append(b.calls, metadata)
	if getMetadataString(metadata, MetadataKeyKubernetesAuthMode) != "in_cluster" ||
		getMetadataString(metadata, MetadataKeyKubernetesKubeconfigPath) != "" ||
		getMetadataString(metadata, MetadataKeyKubernetesKubeContext) != "" ||
		getMetadataString(metadata, MetadataKeyKubernetesConfigNamespace) != "new-agents" {
		return b.staleErr
	}
	b.currentAttempts++
	if b.currentAttempts == 1 {
		return b.transientErr
	}
	return nil
}
