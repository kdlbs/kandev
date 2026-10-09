package lifecycle

import (
	"testing"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutorFailureInspectsPersistedTaskPodInventory(t *testing.T) {
	f := newTaskPodFixture(t)
	f.launch(t, 1)
	f.resources.mu.Lock()
	f.resources.pod = nil
	f.resources.mu.Unlock()
	f.restartBackend(t)

	targets, err := f.repo.ListExecutorObservationTargets(t.Context(), "", 100)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	observation, err := f.manager.InspectExecutor(t.Context(), targets[0])
	require.NoError(t, err)
	require.Equal(t, models.ExecutorOutcomeMissing, observation.Outcome)
	require.Equal(t, "PodNotFound", observation.Reason)
	_, changed, err := f.repo.ObserveExecutorFailure(t.Context(), targets[0], observation)
	require.NoError(t, err)
	require.True(t, changed)
	require.Empty(t, f.resources.deletedPVCs)
}

func TestExecutorFailureInspectionUsesCurrentConnectionAndRecordedIdentity(t *testing.T) {
	for _, identity := range []string{"current", "resource-only", "conflicting"} {
		t.Run(identity, func(t *testing.T) {
			f := newTaskPodFixture(t)
			f.launch(t, 1)
			f.restartBackend(t)
			targets, err := f.repo.ListExecutorObservationTargets(t.Context(), "", 100)
			require.NoError(t, err)
			require.Len(t, targets, 1)
			target := targets[0]
			target.Metadata = cloneKubernetesMetadata(target.Metadata)
			if identity == "resource-only" {
				delete(target.Metadata, "executor_id")
			}
			if identity == "conflicting" {
				target.Metadata["executor_id"] = "other-executor"
				require.NoError(t, f.repo.CreateExecutor(t.Context(), &models.Executor{ID: "other-executor", Name: "other cluster", Type: models.ExecutorTypeKubernetes, Config: map[string]string{"auth_mode": "in_cluster", "namespace": "kandev-agents", "request_timeout_seconds": "30"}}))
			}
			original := cloneKubernetesMetadata(target.Metadata)
			current, err := f.repo.GetExecutor(t.Context(), "executor-1")
			require.NoError(t, err)
			current.Config = map[string]string{"auth_mode": "kubeconfig", "kubeconfig_path": "/test/current-kubeconfig", "kube_context": "current-context", "namespace": "new-default", "request_timeout_seconds": "17"}
			require.NoError(t, f.repo.UpdateExecutor(t.Context(), current))
			created, active, handshakes := f.control.snapshot()
			f.resources.getPodRequests = nil
			factory := f.runtime.clientFactory
			called := false
			f.runtime.clientFactory = func(config kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
				called = true
				require.Equal(t, kubeexecutor.AuthModeKubeconfig, config.AuthMode)
				require.Equal(t, "/test/current-kubeconfig", config.KubeconfigPath)
				require.Equal(t, "current-context", config.KubeContext)
				require.Equal(t, "new-default", config.Namespace)
				require.Equal(t, 17, config.RequestTimeoutSeconds)
				return factory(config)
			}
			observation, err := f.manager.InspectExecutor(t.Context(), target)
			if identity == "conflicting" {
				require.Error(t, err)
				require.False(t, called)
				require.Empty(t, f.resources.getPodRequests)
				require.Equal(t, models.ExecutorOutcomeUnknown, observation.Outcome)
			} else {
				require.NoError(t, err)
				require.True(t, called)
				require.Equal(t, target.ResourceKey, observation.ResourceKey)
				require.NotEqual(t, models.ExecutorOutcomeMissing, observation.Outcome)
				require.Equal(t, []string{"kandev-agents/" + getMetadataString(original, MetadataKeyKubernetesPodName)}, f.resources.getPodRequests)
			}
			require.Equal(t, original, target.Metadata)
			afterCreated, afterActive, afterHandshakes := f.control.snapshot()
			require.Equal(t, created, afterCreated)
			require.Equal(t, active, afterActive)
			require.Equal(t, handshakes, afterHandshakes)
			require.Empty(t, f.resources.deletedPods)
			require.Empty(t, f.resources.deletedPVCs)
		})
	}
}
