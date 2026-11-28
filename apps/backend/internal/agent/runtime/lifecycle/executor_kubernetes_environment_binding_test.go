package lifecycle

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/executor"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestKubernetesSharedEnvironmentWritesOmitBuiltinBinding(t *testing.T) {
	ctx := context.Background()
	const binding = "session-owned-binding"

	t.Run("new task environment", func(t *testing.T) {
		f := newTaskPodFixture(t)
		req := taskPodRequest(1)
		req.Metadata[mcpprofile.BuiltinBindingMetadataKey] = binding
		_, err := f.runtime.CreateInstance(ctx, req)
		require.NoError(t, err)
		record, err := f.repo.GetKubernetesEnvironment(ctx, "environment-1")
		require.NoError(t, err)
		require.NotContains(t, record.Metadata, mcpprofile.BuiltinBindingMetadataKey)
	})

	t.Run("legacy inventory adoption", func(t *testing.T) {
		f := newTaskPodFixture(t)
		created := f.launch(t, 1)
		metadata := cloneKubernetesMetadata(created.Metadata)
		metadata[mcpprofile.BuiltinBindingMetadataKey] = binding
		running := buildRunningFromExecution(&AgentExecution{
			ID: created.InstanceID, TaskID: created.TaskID, SessionID: created.SessionID,
			RuntimeName: executor.NameKubernetes, metadata: metadata,
		}, nil)
		require.NoError(t, f.repo.UpsertExecutorRunning(ctx, running))

		record := &models.KubernetesEnvironment{EnvironmentID: "environment-1"}
		require.NoError(t, f.runtime.adoptKubernetesEnvironment(ctx, taskPodRequest(2), record))
		require.NotContains(t, record.Metadata, mcpprofile.BuiltinBindingMetadataKey)
	})

	t.Run("runtime checkpoint", func(t *testing.T) {
		f := newTaskPodFixture(t)
		f.launch(t, 1)
		taskEnv, err := f.repo.GetTaskEnvironment(ctx, "environment-1")
		require.NoError(t, err)
		record, err := f.repo.ClaimKubernetesEnvironment(
			ctx, taskEnv.ID, taskEnv.TaskID, taskEnv.OwnershipGeneration, uuid.NewString(),
		)
		require.NoError(t, err)
		req, err := f.runtime.taskKubernetesCreateRequest(ctx, taskPodRequest(2), record)
		require.NoError(t, err)
		metadata := cloneKubernetesMetadata(req.Metadata)
		metadata[mcpprofile.BuiltinBindingMetadataKey] = binding
		require.NoError(t, req.CheckpointRuntimeInventory(ctx, metadata))
		saved, err := f.repo.GetKubernetesEnvironment(ctx, "environment-1")
		require.NoError(t, err)
		require.NotContains(t, saved.Metadata, mcpprofile.BuiltinBindingMetadataKey)
	})
}
