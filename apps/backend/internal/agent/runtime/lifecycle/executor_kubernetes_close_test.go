package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestKubernetesExecutorCloseDropsForwardsWithoutDeletingResources(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	}}
	executor := newFakeKubernetesExecutorWithForwarder(resources, &recordingKubernetesExec{}, forwards)
	_, err := executor.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)

	closer, ok := interface{}(executor).(Closeable)
	require.True(t, ok, "Kubernetes executor must close process-local forwards during registry shutdown")
	if !ok {
		return
	}
	require.NoError(t, closer.Close())

	require.True(t, forwards.lastSession().isClosed())
	require.Empty(t, resources.deletedPods)
	require.Empty(t, resources.deletedPVCs)
}
