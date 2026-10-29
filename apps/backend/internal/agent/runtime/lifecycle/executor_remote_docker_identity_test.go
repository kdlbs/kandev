package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerAdoptedAgentctlIdentityIsDurableAndSessionScoped(t *testing.T) {
	require.True(t, ShouldPersistMetadataKey(MetadataKeyRemoteDockerAgentctlInstanceID))
	require.True(t, IsSessionScopedMetadataKey(MetadataKeyRemoteDockerAgentctlInstanceID),
		"a sibling session must not inherit another session's agentctl identity")

	filtered := FilterPersistentMetadata(map[string]interface{}{
		MetadataKeyRemoteDockerAgentctlInstanceID: "adopted-instance",
		"transient_request_value":                 "discard",
	})
	require.Equal(t, "adopted-instance", filtered[MetadataKeyRemoteDockerAgentctlInstanceID])
	require.NotContains(t, filtered, "transient_request_value")

	request := &ExecutorCreateRequest{
		InstanceID: "backend-execution",
		Metadata:   map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "adopted-instance"},
	}
	require.Equal(t, "adopted-instance", remoteDockerAgentctlInstanceID(request))
	require.Equal(t, "backend-execution", remoteDockerAgentctlInstanceID(&ExecutorCreateRequest{InstanceID: "backend-execution"}))
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerAdoptedIdentitySurvivesPersistenceAndRecoveryRequest(t *testing.T) {
	instance := &ExecutorInstance{
		InstanceID: "backend-new", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true,
		Metadata: map[string]interface{}{
			MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-old",
			MetadataKeyContainerID:                    "container-1",
		},
	}
	execution := instance.ToAgentExecution(&ExecutorCreateRequest{
		InstanceID: "backend-new", TaskID: "task-1", SessionID: "session-1",
		ExecutorType: "remote_docker",
	})
	require.Equal(t, "agentctl-old", execution.MetadataSnapshot()[MetadataKeyRemoteDockerAgentctlInstanceID])

	persisted := FilterPersistentMetadata(execution.MetadataSnapshot())
	require.Equal(t, "agentctl-old", persisted[MetadataKeyRemoteDockerAgentctlInstanceID])
	req, containerID, ok := remoteDockerRecoveryRequest(&models.ExecutorRunning{
		AgentExecutionID: execution.ID, SessionID: execution.SessionID, TaskID: execution.TaskID,
		Runtime: agentruntime.RuntimeRemoteDocker, ContainerID: "container-1", Metadata: persisted,
	})
	require.True(t, ok)
	require.Equal(t, "container-1", containerID)
	require.Equal(t, "backend-new", req.InstanceID)
	require.Equal(t, "agentctl-old", remoteDockerAgentctlInstanceID(req))
}
