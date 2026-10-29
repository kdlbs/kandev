package main

import (
	"context"
	"errors"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// checkpointingHost is a Host whose executor-provider callbacks record the
// checkpoints a provider issues, refusing them when asked.
type checkpointingHost struct {
	pluginsdk.Host
	checkpoints  []*pluginsdk.CheckpointExecutorResourceRequest
	refuse       error
	onCheckpoint func()
}

func (h *checkpointingHost) ExecutorProvider() pluginsdk.ExecutorProviderHostAPI { return h }

func (h *checkpointingHost) CheckpointExecutorResource(_ context.Context, req *pluginsdk.CheckpointExecutorResourceRequest) (*pluginsdk.CheckpointExecutorResourceResponse, error) {
	h.checkpoints = append(h.checkpoints, req)
	if h.onCheckpoint != nil {
		h.onCheckpoint()
	}
	if h.refuse != nil {
		return nil, h.refuse
	}
	return &pluginsdk.CheckpointExecutorResourceResponse{Accepted: true}, nil
}

func (h *checkpointingHost) ReportExecutorProgress(context.Context, *pluginsdk.ReportExecutorProgressRequest) (*pluginsdk.ReportExecutorProgressResponse, error) {
	return &pluginsdk.ReportExecutorProgressResponse{}, nil
}

func (h *checkpointingHost) ReadExecutorRuntimeArtifact(context.Context, *pluginsdk.ReadExecutorRuntimeArtifactRequest, func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error {
	return nil
}

func provisionForCleanup(t *testing.T, plugin *fixturePlugin) (*pluginsdk.ExecutorProviderRequestContext, *pluginsdk.ExecutorResourceDescriptor) {
	t.Helper()
	profile := &pluginsdk.ExecutorProfileSnapshot{ProfileId: "profile-1", Config: map[string]string{"region": "eu-west-1"}}
	ctx := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "kandev-plugin-e2e", InstallationId: "installation-1", ProviderKey: executorProviderKey,
		ContractVersion: 1, EnvironmentId: "environment-1", OperationId: "operation-1", InputDigest: "digest-1",
	}
	provisioned, err := plugin.ProvisionExecutorEnvironment(context.Background(), &pluginsdk.ProvisionExecutorEnvironmentRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)
	return ctx, provisioned.GetResource()
}

// A destroy under a cleanup claim checkpoints cleanup_progress through the host
// before it mutates anything, as a real provider must.
func TestFixtureDestroyCheckpointsCleanupProgressBeforeMutating(t *testing.T) {
	plugin := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
	ctx, resource := provisionForCleanup(t, plugin)
	host := &checkpointingHost{}
	plugin.SetHost(host)
	host.onCheckpoint = func() {
		_, found, err := plugin.fixtureResourceByHandle(resource.GetResourceHandle())
		require.NoError(t, err)
		require.True(t, found, "the resource was destroyed before the checkpoint")
	}

	destroyed, err := plugin.DestroyExecutorEnvironment(context.Background(), &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: ctx, Resource: resource, CleanupClaim: "operation-1:cleanup",
	})
	require.NoError(t, err)
	require.True(t, destroyed.GetConfirmedAbsent())
	require.Len(t, host.checkpoints, 1)
	require.Equal(t, "cleanup_progress", host.checkpoints[0].GetPhase())
	require.EqualValues(t, 1, host.checkpoints[0].GetSequence())
	require.Equal(t, resource.GetResourceHandle(), host.checkpoints[0].GetResource().GetResourceHandle())
	_, found, err := plugin.fixtureResourceByHandle(resource.GetResourceHandle())
	require.NoError(t, err)
	require.False(t, found)
}

// A refused checkpoint fails the destroy closed: nothing is mutated and the
// resource is not reported absent.
func TestFixtureDestroyFailsClosedWhenTheHostRefusesTheCheckpoint(t *testing.T) {
	plugin := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
	ctx, resource := provisionForCleanup(t, plugin)
	plugin.SetHost(&checkpointingHost{refuse: errors.New("cleanup claim not found")})

	destroyed, err := plugin.DestroyExecutorEnvironment(context.Background(), &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: ctx, Resource: resource, CleanupClaim: "operation-1:cleanup",
	})
	require.NoError(t, err)
	require.False(t, destroyed.GetConfirmedAbsent())
	require.Equal(t, "cleanup_checkpoint_refused", destroyed.GetError().GetCode())
	_, found, err := plugin.fixtureResourceByHandle(resource.GetResourceHandle())
	require.NoError(t, err)
	require.True(t, found, "the resource was mutated without an accepted checkpoint")
}
